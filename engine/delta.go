package engine

import (
	"fmt"
	"strings"
)

// A Proposal is what an untrusted party (the LLM, or a plugin) sends back
// after looking at a player action or an engine hook. It never mutates
// anything. Apply validates every piece against the source's rights, rolls
// where a roll is needed, and applies only what survives.
//
// This is the one write path into a Run. Keep it that way.
type Proposal struct {
	// Narration is prose for the player. Ignored from plugins except as a
	// system note (plugins do not narrate; the LLM does).
	Narration string `json:"narration,omitempty"`

	// SlotCost is how much of the day the action consumed. The engine clamps.
	SlotCost int `json:"slot_cost,omitempty"`

	// Outcomes are mutually exclusive alternatives. The engine picks one by
	// weight using the run's seeded RNG. Empty means "no branching".
	Outcomes []Outcome `json:"outcomes,omitempty"`

	// Deltas apply regardless of which outcome is chosen.
	Deltas []Delta `json:"deltas,omitempty"`

	Todos   []string          `json:"todos,omitempty"`
	Pending []PendingProposal `json:"pending,omitempty"`
	NPCs    []NPCProposal     `json:"npcs,omitempty"`
}

type Outcome struct {
	Weight    float64 `json:"weight"`
	Narration string  `json:"narration"`
	Deltas    []Delta `json:"deltas,omitempty"`
}

type PendingProposal struct {
	InDays      int     `json:"in_days"`
	Description string  `json:"description"`
	Probability float64 `json:"probability"`
}

type NPCProposal struct {
	Name        string `json:"name"`
	Relation    string `json:"relation"`
	Description string `json:"description"`
}

// Delta is one typed change. Op decides which fields matter.
//
//	money_add        Amount (negative = spend; positive is capped per source)
//	users_add        Amount
//	metric_set       Key, Amount
//	pin              Key  ("money" or a metric name)
//	unpin            Key
//	entity_incorp    Text (entity type)
//	gstin            (no args)
//	build_phase_add  Amount
//	hidden_issue_add Text
//	known_bug_add    Text
//	cap_table_set    Key (holder), Percent
//	npc_disposition  Key (npc id), Percent (delta, -1..1)
//	risk_tick        Key (risk id)
//	risk_discover    Key (risk id)
//	exit             Text (sold|shutdown|walked_away)
type Delta struct {
	Op      string  `json:"op"`
	Key     string  `json:"key,omitempty"`
	Amount  int64   `json:"amount,omitempty"`
	Percent float64 `json:"percent,omitempty"`
	Text    string  `json:"text,omitempty"`
}

// Source identifies who proposed something and therefore what it may do.
type Source struct {
	Kind string // "llm" | "plugin" | "engine"
	Name string // plugin name, or empty
}

func (s Source) String() string {
	if s.Name == "" {
		return s.Kind
	}
	return s.Kind + ":" + s.Name
}

// Rejection explains a discarded piece of a proposal. These are logged, and
// surfaced in dev mode, so misbehaving models and plugins are visible.
type Rejection struct {
	What   string `json:"what"`
	Reason string `json:"reason"`
}

// Result is what Apply did.
type Result struct {
	Narration     string      `json:"narration"`
	ChosenOutcome int         `json:"chosen_outcome"` // -1 if none
	SlotCost      int         `json:"slot_cost"`
	Applied       []Delta     `json:"applied"`
	Rejected      []Rejection `json:"rejected"`
	DayEnded      bool        `json:"day_ended"`
}

// MaxLLMIncomePerTurn caps positive money from a single LLM turn. The model
// cannot hand the player a fortune; real income should come through events
// and metrics the engine tracks. Tune per chapter later.
const MaxLLMIncomePerTurn int64 = 50_000

// Apply validates and applies a proposal. It is deterministic given the run's
// RNG sequence.
func (r *Run) Apply(p Proposal, src Source) Result {
	res := Result{ChosenOutcome: -1}
	if r.Status != "active" {
		res.Rejected = append(res.Rejected, Rejection{"proposal", "run is not active"})
		return res
	}

	// Slot cost. Plugins cannot spend the player's time; only the engine
	// decides that (from the LLM's proposal, clamped).
	if src.Kind == "llm" {
		cost := p.SlotCost
		if cost < 1 {
			cost = 1
		}
		if cost > r.SlotsLeft {
			cost = r.SlotsLeft
		}
		res.SlotCost = cost
		r.SlotsLeft -= cost
		if r.SlotsLeft <= 0 {
			res.DayEnded = true
		}
	} else if p.SlotCost != 0 {
		res.Rejected = append(res.Rejected, Rejection{"slot_cost", src.String() + " may not spend time"})
	}

	// Narration: LLM only.
	if src.Kind == "llm" {
		res.Narration = strings.TrimSpace(p.Narration)
	} else if p.Narration != "" {
		res.Rejected = append(res.Rejected, Rejection{"narration", src.String() + " may not narrate"})
	}

	// Pick an outcome.
	deltas := append([]Delta(nil), p.Deltas...)
	if len(p.Outcomes) > 0 {
		w := make([]float64, len(p.Outcomes))
		for i, o := range p.Outcomes {
			w[i] = o.Weight
		}
		idx := r.Pick(w)
		res.ChosenOutcome = idx
		chosen := p.Outcomes[idx]
		if src.Kind == "llm" && strings.TrimSpace(chosen.Narration) != "" {
			if res.Narration != "" {
				res.Narration += "\n\n"
			}
			res.Narration += strings.TrimSpace(chosen.Narration)
		}
		deltas = append(deltas, chosen.Deltas...)
	}

	// Apply deltas one by one.
	for _, d := range deltas {
		if err := r.applyDelta(d, src); err != nil {
			res.Rejected = append(res.Rejected, Rejection{d.Op + " " + d.Key, err.Error()})
			continue
		}
		res.Applied = append(res.Applied, d)
	}

	// Todos: anyone may add player-visible to-dos. Capped per turn.
	for i, t := range p.Todos {
		if i >= 10 {
			res.Rejected = append(res.Rejected, Rejection{"todo", "too many todos in one proposal"})
			break
		}
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		r.Todos = append(r.Todos, Todo{
			ID: fmt.Sprintf("todo-%d-%d-%s", r.CurrentDay, len(r.Todos), src.Kind), Day: r.CurrentDay,
			Text: t, Source: src.String(),
		})
	}

	// Pending events: anyone may schedule; the engine owns the roll.
	for _, pe := range p.Pending {
		if pe.InDays < 1 || pe.InDays > 365 {
			res.Rejected = append(res.Rejected, Rejection{"pending", "in_days must be 1..365"})
			continue
		}
		if strings.TrimSpace(pe.Description) == "" {
			continue
		}
		r.Pending = append(r.Pending, PendingEvent{
			ID:          fmt.Sprintf("ev-%d-%d", r.CurrentDay, len(r.Pending)),
			DueDay:      r.CurrentDay + pe.InDays,
			Source:      src.String(),
			Description: strings.TrimSpace(pe.Description),
			Probability: clamp01(pe.Probability),
		})
	}

	// NPCs: LLM and plugins may introduce characters. Never duplicates by name.
	for _, np := range p.NPCs {
		name := strings.TrimSpace(np.Name)
		if name == "" {
			continue
		}
		if r.findNPC(name) != nil {
			continue
		}
		r.NPCs = append(r.NPCs, NPC{
			ID: fmt.Sprintf("npc-%d", len(r.NPCs)+1), Name: name,
			Relation: np.Relation, PlayerDescription: np.Description,
			Disposition: 0, Source: src.String(),
		})
	}

	if res.Narration != "" {
		r.Log = append(r.Log, LogEntry{Day: r.CurrentDay, Kind: "narration", Text: res.Narration, Source: src.String()})
	}
	return res
}

func (r *Run) findNPC(name string) *NPC {
	for i := range r.NPCs {
		if strings.EqualFold(r.NPCs[i].Name, name) {
			return &r.NPCs[i]
		}
	}
	return nil
}

func (r *Run) findRisk(id string) *RiskItem {
	for i := range r.Risks {
		if r.Risks[i].ID == id {
			return &r.Risks[i]
		}
	}
	return nil
}

// coreOps are the operations that touch world state. Only the LLM (via a
// validated turn) and the engine itself may use them. Plugins may not.
var coreOps = map[string]bool{
	"money_add": true, "users_add": true, "entity_incorp": true, "gstin": true,
	"build_phase_add": true, "hidden_issue_add": true, "known_bug_add": true,
	"cap_table_set": true, "risk_tick": true, "risk_discover": true, "exit": true,
}

func (r *Run) applyDelta(d Delta, src Source) error {
	if src.Kind == "plugin" && coreOps[d.Op] {
		return fmt.Errorf("plugins may not change core state")
	}
	switch d.Op {
	case "money_add":
		if d.Amount > 0 && src.Kind == "llm" && d.Amount > MaxLLMIncomePerTurn {
			return fmt.Errorf("income %d exceeds per-turn cap %d", d.Amount, MaxLLMIncomePerTurn)
		}
		if d.Amount < 0 && r.World.Money+d.Amount < 0 {
			// No credit in v1. The model must narrate "you can't afford it".
			return fmt.Errorf("insufficient funds: balance %d, spend %d", r.World.Money, -d.Amount)
		}
		r.World.Money += d.Amount
	case "users_add":
		if r.World.Users+int(d.Amount) < 0 {
			return fmt.Errorf("users cannot go negative")
		}
		r.World.Users += int(d.Amount)
	case "metric_set":
		key := strings.TrimSpace(d.Key)
		if key == "" || key == "money" {
			return fmt.Errorf("invalid metric key")
		}
		if src.Kind == "plugin" && !strings.HasPrefix(key, src.Name+".") {
			return fmt.Errorf("plugin metrics must be namespaced as %s.<name>", src.Name)
		}
		if r.World.Metrics == nil {
			r.World.Metrics = map[string]int{}
		}
		r.World.Metrics[key] = int(d.Amount)
	case "pin":
		if src.Kind == "plugin" {
			return fmt.Errorf("only the player pins metrics")
		}
		for _, m := range r.World.PinnedMetrics {
			if m == d.Key {
				return nil
			}
		}
		r.World.PinnedMetrics = append(r.World.PinnedMetrics, d.Key)
	case "unpin":
		if src.Kind == "plugin" {
			return fmt.Errorf("only the player pins metrics")
		}
		out := r.World.PinnedMetrics[:0]
		for _, m := range r.World.PinnedMetrics {
			if m != d.Key {
				out = append(out, m)
			}
		}
		r.World.PinnedMetrics = out
	case "entity_incorp":
		if r.World.Entity.Incorporated {
			return fmt.Errorf("already incorporated")
		}
		r.World.Entity.Incorporated = true
		r.World.Entity.Type = d.Text
	case "gstin":
		r.World.Entity.GSTIN = true
	case "build_phase_add":
		if d.Amount < 0 {
			return fmt.Errorf("build phase cannot go backwards")
		}
		r.World.Build.Phase += int(d.Amount)
	case "hidden_issue_add":
		r.World.Build.HiddenIssues = append(r.World.Build.HiddenIssues, d.Text)
	case "known_bug_add":
		r.World.Build.KnownBugs = append(r.World.Build.KnownBugs, d.Text)
	case "cap_table_set":
		if d.Percent < 0 || d.Percent > 100 {
			return fmt.Errorf("percent out of range")
		}
		total := d.Percent
		found := false
		for i := range r.World.CapTable {
			if r.World.CapTable[i].Holder == d.Key {
				r.World.CapTable[i].Percent = d.Percent
				found = true
				continue
			}
			total += r.World.CapTable[i].Percent
		}
		if total > 100.0001 {
			return fmt.Errorf("cap table would exceed 100%%")
		}
		if !found {
			r.World.CapTable = append(r.World.CapTable, CapTableRow{Holder: d.Key, Percent: d.Percent, Vesting: d.Text})
		}
	case "npc_disposition":
		var n *NPC
		for i := range r.NPCs {
			if r.NPCs[i].ID == d.Key || strings.EqualFold(r.NPCs[i].Name, d.Key) {
				n = &r.NPCs[i]
			}
		}
		if n == nil {
			return fmt.Errorf("unknown npc %q", d.Key)
		}
		if d.Percent < -1 || d.Percent > 1 {
			return fmt.Errorf("disposition delta out of range")
		}
		n.Disposition += d.Percent
		if n.Disposition > 1 {
			n.Disposition = 1
		}
		if n.Disposition < -1 {
			n.Disposition = -1
		}
	case "risk_tick":
		ri := r.findRisk(d.Key)
		if ri == nil {
			return fmt.Errorf("unknown risk %q", d.Key)
		}
		ri.Ticked = true
		ri.Discovered = true
	case "risk_discover":
		ri := r.findRisk(d.Key)
		if ri == nil {
			return fmt.Errorf("unknown risk %q", d.Key)
		}
		ri.Discovered = true
	case "exit":
		switch d.Text {
		case "sold", "shutdown", "walked_away":
		default:
			return fmt.Errorf("invalid exit type %q", d.Text)
		}
		r.Status = "exited"
		r.Exit = &Exit{Type: d.Text, Day: r.CurrentDay, Money: r.World.Money}
	default:
		return fmt.Errorf("unknown op %q", d.Op)
	}
	return nil
}

// CheckBalance is the player deliberately looking at their money. It costs
// a slot unless money is pinned.
func (r *Run) CheckBalance() (int64, bool) {
	for _, m := range r.World.PinnedMetrics {
		if m == "money" {
			return r.World.Money, false
		}
	}
	if r.SlotsLeft > 0 {
		r.SlotsLeft--
	}
	return r.World.Money, true
}

// PlayerExit ends the run. Only the founder can do this.
func (r *Run) PlayerExit(kind, note string) error {
	err := r.applyDelta(Delta{Op: "exit", Text: kind}, Source{Kind: "engine"})
	if err == nil && r.Exit != nil {
		r.Exit.Note = note
	}
	return err
}
