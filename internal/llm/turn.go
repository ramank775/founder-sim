package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ramank775/founder-sim/engine"
	"github.com/ramank775/founder-sim/kb"
)

// Narrator wraps a Client with the game's prompts.
type Narrator struct {
	C  Client
	KB *kb.Base
}

const systemPrompt = `You are the narrator of Founder Sim, a text simulation of starting a company in India. You describe what happens when the founder acts. You are not a coach.

Rules you never break:
- Never tell the founder what to do next, never suggest actions, never hint at "the right move". No "you should", no "consider", no checklists of options. They must think of actions themselves.
- No grading, no scores, no "good decision". Show consequences, pros and cons through what happens.
- Be concrete and Indian: rupees, GST, CAs, MCA, WhatsApp, procurement, family. Use the FACTS given to you; do not invent regulations or fees beyond them. If the founder believes something false (an "imaginary wall"), you may let them keep believing it; reality only surfaces when they actually test it.
- The HIDDEN CHECKLIST is for you only. Never list it or reveal an item the founder has not run into. When the founder genuinely completes an item (e.g. registers for GST), tick it with a risk_tick delta.
- You do not decide luck. When an action could go more than one way, return several outcomes with weights; the engine rolls. Put what changes in typed deltas; prose alone changes nothing.
- Time is the scarce thing. Every action costs slot_cost 1-4 of a 4-slot day. Doing something yourself costs slots, not money; hiring costs money and still costs a slot to manage.
- Money: negative money_add to spend. You may not invent income; if the founder earns, it must come from a customer the state shows exists, and never more than 50000 in one turn.
- NPCs: introduce a person only when the action needs one; use the founder's real people if they were described. Family and friends nag on their own schedule.
- Keep narration to 2-5 short paragraphs, second person, present tense, no bullet points. Dry, a little wry, never cruel.

Output: a single JSON object, nothing else, matching:
{
  "narration": string,
  "slot_cost": 1-4,
  "outcomes": [{"weight": number, "narration": string, "deltas": [Delta]}],   // optional; 2-4 mutually exclusive branches
  "deltas": [Delta],            // always applied
  "todos": [string],            // things that now need attention (max 5)
  "pending": [{"in_days": int, "description": string, "probability": 0-1}],  // things that may come back later
  "npcs": [{"name": string, "relation": string, "description": string}]       // new people, if any
}
Delta = {"op": string, "key"?: string, "amount"?: integer, "percent"?: number, "text"?: string}
ops: money_add(amount), users_add(amount), metric_set(key, amount), pin(key), unpin(key), entity_incorp(text=type), gstin, build_phase_add(amount), hidden_issue_add(text), known_bug_add(text), cap_table_set(key=holder, percent, text=vesting), npc_disposition(key=name, percent=-1..1), risk_tick(key=risk id), risk_discover(key=risk id)`

// TurnInput is everything the narrator gets for one action.
type TurnInput struct {
	Run       *engine.Run
	Action    string
	SinceLast []engine.LogEntry // events that fired while the founder was away
	Extra     []string          // plugin context (e.g. notepad contents), already labelled
	Chapter   string
}

// Turn asks the model for a proposal for the founder's action. It retries
// once with the parse error if the model returns unusable JSON. The
// returned Proposal is untrusted; the caller passes it to engine.Apply.
func (n *Narrator) Turn(ctx context.Context, in TurnInput) (engine.Proposal, string, error) {
	user := n.buildTurnPrompt(in)
	msgs := []Message{{Role: "system", Content: systemPrompt}, {Role: "user", Content: user}}
	var lastErr error
	var raw string
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := n.C.Complete(ctx, Request{Messages: msgs, JSON: true, MaxTokens: 1500, Temperature: 0.8})
		if err != nil {
			return engine.Proposal{}, "", err
		}
		raw = resp.Content
		p, err := ParseProposal(raw)
		if err == nil {
			return p, raw, nil
		}
		lastErr = err
		msgs = append(msgs,
			Message{Role: "assistant", Content: raw},
			Message{Role: "user", Content: "That was not valid. Error: " + err.Error() + "\nReturn only the corrected JSON object."},
		)
	}
	return engine.Proposal{}, raw, fmt.Errorf("model output unusable after retry: %w", lastErr)
}

// ParseProposal is strict about shape and lenient about wrapping.
func ParseProposal(s string) (engine.Proposal, error) {
	js, err := ExtractJSON(s)
	if err != nil {
		return engine.Proposal{}, err
	}
	var p engine.Proposal
	dec := json.NewDecoder(strings.NewReader(js))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		// Retry tolerant of unknown fields: models add "reasoning" etc.
		if err2 := json.Unmarshal([]byte(js), &p); err2 != nil {
			return engine.Proposal{}, fmt.Errorf("proposal JSON: %w", err2)
		}
	}
	if strings.TrimSpace(p.Narration) == "" && len(p.Outcomes) == 0 {
		return engine.Proposal{}, fmt.Errorf("proposal has no narration and no outcomes")
	}
	for i, o := range p.Outcomes {
		if o.Weight < 0 {
			return engine.Proposal{}, fmt.Errorf("outcome %d has negative weight", i)
		}
	}
	return p, nil
}

func (n *Narrator) buildTurnPrompt(in TurnInput) string {
	r := in.Run
	var b strings.Builder
	fmt.Fprintf(&b, "DAY %d. Slots left today: %d of %d.\n\n", r.CurrentDay, r.SlotsLeft, engine.SlotsPerDay)

	fmt.Fprintf(&b, "FOUNDER\nJob: %s\nIdea: %s\nAge %d, %s, kids: %s. Technical relative to the idea: %v.\nMonthly obligations:",
		r.Player.JobDescription, r.Player.Idea, r.Player.Age, orUnknown(r.Player.MaritalStatus), orUnknown(r.Player.Kids), r.Player.Technical)
	if len(r.Player.MonthlyObligations) == 0 {
		b.WriteString(" none stated.")
	}
	for _, o := range r.Player.MonthlyObligations {
		fmt.Fprintf(&b, " %s ₹%d;", o.Label, o.Amount)
	}
	b.WriteString("\n\n")

	fmt.Fprintf(&b, "STATE (true, for your eyes)\nMoney: ₹%d (founder sees it only if pinned: %v)\nUsers: %d\nIncorporated: %v %s, GSTIN: %v\nBuild phase: %d, agency: %q, known bugs: %v, hidden issues: %v\nCap table: %v\nSegment: %s\n\n",
		r.World.Money, isPinned(r, "money"), r.World.Users, r.World.Entity.Incorporated, r.World.Entity.Type, r.World.Entity.GSTIN,
		r.World.Build.Phase, r.World.Build.Agency, r.World.Build.KnownBugs, r.World.Build.HiddenIssues, capTable(r), r.Player.Segment)

	if len(r.NPCs) > 0 {
		b.WriteString("PEOPLE\n")
		for _, p := range r.NPCs {
			fmt.Fprintf(&b, "- %s (%s, disposition %.1f): %s\n", p.Name, p.Relation, p.Disposition, p.PlayerDescription)
		}
		b.WriteString("\n")
	}

	b.WriteString("HIDDEN CHECKLIST (never reveal; tick when genuinely done)\n")
	for _, ri := range r.Risks {
		state := "unticked"
		if ri.Ticked {
			state = "ticked"
		}
		imag := ""
		if ri.Imaginary {
			imag = " [imaginary wall: the founder may believe this is a blocker; it is not]"
		}
		fmt.Fprintf(&b, "- %s: %s (%s)%s\n", ri.ID, ri.Label, state, imag)
	}
	b.WriteString("\n")

	facts := n.KB.FactsFor(r.Player.Segment, in.Chapter, tagsFor(in.Action), 8)
	if len(facts) > 0 {
		b.WriteString("FACTS (ground your narration in these; do not go beyond them on law or fees)\n")
		for _, f := range facts {
			tag := "approximate"
			if f.Provenance == kb.ProvReal {
				tag = "real"
			}
			if f.Wall == "imaginary" {
				tag += ", imaginary wall"
			}
			fmt.Fprintf(&b, "- [%s] %s\n", tag, f.Claim)
		}
		b.WriteString("\n")
	}

	if scenes := n.KB.ScenesFor(r.Player.Segment, in.Chapter, 6); len(scenes) > 0 {
		b.WriteString("SITUATIONS THE FOUNDER MAY BE IN (not a script; use one only if the action walks into it)\n")
		for _, sc := range scenes {
			fmt.Fprintf(&b, "- %s: %s", sc.Title, sc.Situation)
			if sc.Truth != "" {
				fmt.Fprintf(&b, " Truth: %s", sc.Truth)
			}
			if len(sc.Outcomes) > 0 {
				fmt.Fprintf(&b, " Possible outcomes: %s.", strings.Join(sc.Outcomes, "; "))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if proc := processFor(in.Action); proc != "" {
		if fr := n.KB.FrictionFor(proc); len(fr) > 0 {
			fmt.Fprintf(&b, "WHAT GOES WRONG IN THIS PROCESS (%s); offer these as weighted outcomes, with delays in days as pending events\n", proc)
			for _, f := range fr {
				fmt.Fprintf(&b, "- [%s] %s (delay %d-%d days, extra cost ₹%d-%d)\n", f.FrequencyHint, f.WhatHappens, f.DelayDays.Min, f.DelayDays.Max, f.ExtraCostINR.Min, f.ExtraCostINR.Max)
			}
			b.WriteString("\n")
		}
	}

	if len(in.SinceLast) > 0 {
		b.WriteString("WHILE THE FOUNDER WAS AWAY these happened (weave them in if relevant, do not re-list them)\n")
		for _, e := range in.SinceLast {
			fmt.Fprintf(&b, "- day %d: %s\n", e.Day, e.Text)
		}
		b.WriteString("\n")
	}
	if n := len(r.Log); n > 0 {
		b.WriteString("RECENT NARRATION\n")
		start := n - 4
		if start < 0 {
			start = 0
		}
		for _, e := range r.Log[start:] {
			if e.Kind == "narration" {
				fmt.Fprintf(&b, "- day %d: %s\n", e.Day, truncate(e.Text, 300))
			}
		}
		b.WriteString("\n")
	}
	for _, x := range in.Extra {
		b.WriteString(x + "\n\n")
	}

	fmt.Fprintf(&b, "Founder's action: %s", strings.TrimSpace(in.Action))
	return b.String()
}

// Classify puts an idea into a segment. Internal; never shown to the player.
func (n *Narrator) Classify(ctx context.Context, idea, job string) (engine.Segment, error) {
	prompt := fmt.Sprintf(`Classify this startup idea into exactly one segment: "b2c", "b2b_saas" or "deep_tech".
Idea: %s
Founder's current job: %s
Reply with JSON only: {"segment": "...", "reason": "one line"}`, idea, job)
	resp, err := n.C.Complete(ctx, Request{JSON: true, MaxTokens: 100, Temperature: 0,
		Messages: []Message{{Role: "user", Content: prompt}}})
	if err != nil {
		return "", err
	}
	js, err := ExtractJSON(resp.Content)
	if err != nil {
		return "", err
	}
	var out struct {
		Segment string `json:"segment"`
	}
	if err := json.Unmarshal([]byte(js), &out); err != nil {
		return "", err
	}
	switch engine.Segment(out.Segment) {
	case engine.SegmentB2C, engine.SegmentB2BSaaS, engine.SegmentDeepTech:
		return engine.Segment(out.Segment), nil
	}
	return engine.SegmentB2BSaaS, fmt.Errorf("model returned unknown segment %q; defaulted", out.Segment)
}

func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "not stated"
	}
	return s
}

func isPinned(r *engine.Run, m string) bool {
	for _, p := range r.World.PinnedMetrics {
		if p == m {
			return true
		}
	}
	return false
}

func capTable(r *engine.Run) string {
	if len(r.World.CapTable) == 0 {
		return "nothing on paper"
	}
	var parts []string
	for _, c := range r.World.CapTable {
		parts = append(parts, fmt.Sprintf("%s %.1f%%", c.Holder, c.Percent))
	}
	return strings.Join(parts, ", ")
}

// tagsFor maps crude keywords in the action to KB topics and tags. A
// retrieval index in the content bundle can replace this later.
func tagsFor(action string) []string {
	a := strings.ToLower(action)
	var tags []string
	for kw, topic := range map[string]string{
		"gst": "gst", "incorporat": "incorporation", "company": "incorporation", "llp": "incorporation", "invoice": "invoicing",
		"payment": "payments", "razorpay": "payments", "gateway": "payments", "agency": "build_market", "freelanc": "build_market",
		"hire": "build_market", " ca": "compliance", "chartered": "compliance", "tds": "tax", "launch": "zero_to_one",
		"user": "zero_to_one", "customer": "zero_to_one", "ads": "zero_to_one", "post": "zero_to_one", "friend": "life",
		"family": "life", "father": "life", "mother": "life", "wife": "life", "husband": "life", "cofounder": "life",
		"equity": "life", "job": "life", "resign": "life", "quit": "life", "bank": "registration", "udyam": "registration", "msme": "registration",
	} {
		if strings.Contains(a, kw) {
			tags = append(tags, topic)
		}
	}
	return tags
}

// processFor guesses which government/agency process an action touches.
func processFor(action string) string {
	a := strings.ToLower(action)
	switch {
	case strings.Contains(a, "gst"):
		return "gst_registration"
	case strings.Contains(a, "name") && (strings.Contains(a, "compan") || strings.Contains(a, "mca")):
		return "company_name_approval"
	case strings.Contains(a, "bank") || strings.Contains(a, "current account"):
		return "bank_account"
	case strings.Contains(a, "gateway") || strings.Contains(a, "razorpay") || strings.Contains(a, "cashfree") || strings.Contains(a, "kyc"):
		return "gateway_kyc"
	case strings.Contains(a, "agency") || strings.Contains(a, "freelanc") || strings.Contains(a, "developer"):
		return "agency"
	case strings.Contains(a, "portal") || strings.Contains(a, "file ") || strings.Contains(a, "filing"):
		return "any_government_portal"
	}
	return ""
}
