// Package engine owns the hard state of a run. It is the single source of
// truth: money, time, obligations, cap table, NPCs, the hidden risk checklist
// and pending events. Nothing outside this package mutates state directly;
// the LLM and plugins *propose* Deltas, and Apply validates and rolls them.
package engine

// Segment is the internal classification of an idea. Never shown to the player.
type Segment string

const (
	SegmentB2C      Segment = "b2c"
	SegmentB2BSaaS  Segment = "b2b_saas"
	SegmentDeepTech Segment = "deep_tech"
)

// Player is what the player told us about themselves at session start.
type Player struct {
	JobDescription     string       `json:"job_description"`
	Idea               string       `json:"idea"`
	Age                int          `json:"age"`
	MaritalStatus      string       `json:"marital_status"`
	Kids               string       `json:"kids"` // none / some / expecting
	Savings            int64        `json:"savings"`
	MonthlyObligations []Obligation `json:"monthly_obligations"`
	Technical          bool         `json:"technical"`
	Segment            Segment      `json:"segment"`
}

// Obligation is a recurring monthly cost that never pauses.
type Obligation struct {
	Label  string `json:"label"`
	Amount int64  `json:"amount"`
}

// World is the mutable world state. Money is in whole rupees.
type World struct {
	Money         int64          `json:"money"`
	PinnedMetrics []string       `json:"pinned_metrics"`
	Entity        Entity         `json:"entity"`
	CapTable      []CapTableRow  `json:"cap_table"`
	Build         Build          `json:"build"`
	Users         int            `json:"users"`
	Metrics       map[string]int `json:"metrics"` // player-chosen metrics, by name
}

type Entity struct {
	Incorporated bool   `json:"incorporated"`
	Type         string `json:"type,omitempty"`
	GSTIN        bool   `json:"gstin"`
}

type CapTableRow struct {
	Holder  string  `json:"holder"`
	Percent float64 `json:"percent"`
	Vesting string  `json:"vesting,omitempty"`
}

type Build struct {
	Phase        int      `json:"phase"`
	HiddenIssues []string `json:"hidden_issues"`
	KnownBugs    []string `json:"known_bugs"`
	Agency       string   `json:"agency,omitempty"`
}

// RiskItem is one entry on the hidden compliance / risk checklist.
type RiskItem struct {
	ID                 string  `json:"id"`
	Label              string  `json:"label"`
	Ticked             bool    `json:"ticked"`
	BestCase           string  `json:"best_case"`
	WorstCase          string  `json:"worst_case"`
	TriggerProbPerWeek float64 `json:"trigger_prob_per_week"`
	Imaginary          bool    `json:"imaginary,omitempty"` // a wall that isn't real
	// AppliesWhen gates the roll: "" (always), unincorporated, incorporated,
	// has_customers, has_gstin, agency_build.
	AppliesWhen      string `json:"applies_when,omitempty"`
	Discovered       bool   `json:"discovered"` // player has learned this item exists
	LastTriggeredDay int    `json:"last_triggered_day,omitempty"`
}

// NPC is a character from the player's real life, populated lazily.
type NPC struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Relation          string  `json:"relation"` // father, friend, cofounder, agency, clerk...
	PlayerDescription string  `json:"player_description"`
	Disposition       float64 `json:"disposition"` // -1..1, drifts over time
	Source            string  `json:"source"`      // "player" | "engine" | plugin name
}

// PendingEvent is something the engine has scheduled to surface on DueDay.
type PendingEvent struct {
	ID          string  `json:"id"`
	DueDay      int     `json:"due_day"`
	Source      string  `json:"source"` // risk item id, npc id, plugin name, "llm"...
	Description string  `json:"description"`
	Probability float64 `json:"probability"` // chance it actually fires when due; 1 = certain
	Fired       bool    `json:"fired"`
}

// Todo is a player-visible item that came up. The engine surfaces up to ~10/day.
type Todo struct {
	ID        string `json:"id"`
	Day       int    `json:"day"`
	Text      string `json:"text"`
	Source    string `json:"source"`
	Done      bool   `json:"done"`
	Dismissed bool   `json:"dismissed"`
}

// MissedThing is something the player ignored. It persists in hidden state
// and resurfaces later as an event.
type MissedThing struct {
	Day        int    `json:"day"`
	What       string `json:"what"`
	Resurfaced bool   `json:"resurfaced"`
}

// Exit records how the run ended. Only the founder ends a run.
type Exit struct {
	Type  string `json:"type"` // sold | shutdown | walked_away
	Day   int    `json:"day"`
	Money int64  `json:"money"`
	Note  string `json:"note,omitempty"`
}

// Run is the whole persisted state of one playthrough.
type Run struct {
	ID        string `json:"id"`
	Seed      int64  `json:"seed"`
	StartedAt int64  `json:"started_at"` // unix ms, wall clock
	// TimeScaleMs is real milliseconds per in-game day. Default 1 hour.
	TimeScaleMs int64  `json:"time_scale_ms"`
	AnchorDay   int    `json:"anchor_day"`   // day at AnchorAtMs; re-set when time scale changes
	AnchorAtMs  int64  `json:"anchor_at_ms"` // 0 means StartedAt
	CurrentDay  int    `json:"current_day"`
	SlotsLeft   int    `json:"slots_left"` // attention slots remaining today
	Status      string `json:"status"`     // active | exited
	Exit        *Exit  `json:"exit,omitempty"`

	Player  Player         `json:"player"`
	World   World          `json:"world"`
	Risks   []RiskItem     `json:"risks"`
	NPCs    []NPC          `json:"npcs"`
	Pending []PendingEvent `json:"pending"`
	Todos   []Todo         `json:"todos"`
	Missed  []MissedThing  `json:"missed"`
	Log     []LogEntry     `json:"log"`
	RollSeq uint64         `json:"roll_seq"` // number of RNG draws so far; makes the RNG replayable
	Plugins []string       `json:"plugins"`  // plugins enabled for this run (opt-in per run)
}

// LogEntry is a narrated moment. This is what the player reads.
type LogEntry struct {
	Day    int    `json:"day"`
	Kind   string `json:"kind"` // narration | event | system
	Text   string `json:"text"`
	Source string `json:"source,omitempty"`
}

// SlotsPerDay is how many attention slots a day has. Every action costs >= 1.
const SlotsPerDay = 4

// DefaultTimeScaleMs is 1 in-game day = 1 real hour.
const DefaultTimeScaleMs int64 = 60 * 60 * 1000

// NewRun creates a fresh active run at day 0 from a player profile.
func NewRun(id string, seed int64, startedAt int64, p Player) *Run {
	return &Run{
		ID:          id,
		Seed:        seed,
		StartedAt:   startedAt,
		TimeScaleMs: DefaultTimeScaleMs,
		CurrentDay:  0,
		SlotsLeft:   SlotsPerDay,
		Status:      "active",
		Player:      p,
		World: World{
			Money:   p.Savings,
			Metrics: map[string]int{},
		},
	}
}

// PlayerView is what the player (and plugins) are allowed to see. It
// deliberately omits the hidden checklist, unresolved pending events and
// missed things. Money is present only if pinned; otherwise the player
// spends a slot to check it.
type PlayerView struct {
	RunID         string         `json:"run_id"`
	Day           int            `json:"day"`
	SlotsLeft     int            `json:"slots_left"`
	Status        string         `json:"status"`
	Idea          string         `json:"idea"`
	Technical     bool           `json:"technical"`
	PinnedMetrics map[string]int `json:"pinned_metrics"`
	Money         *int64         `json:"money,omitempty"`
	Entity        Entity         `json:"entity"`
	Users         int            `json:"users"`
	KnownNPCs     []NPC          `json:"known_npcs"`
	Todos         []Todo         `json:"todos"`
	KnownRisks    []string       `json:"known_risks"` // labels of discovered checklist items
	Log           []LogEntry     `json:"log"`
}

// View projects the run into what the player may see.
func (r *Run) View() PlayerView {
	v := PlayerView{
		RunID:         r.ID,
		Day:           r.CurrentDay,
		SlotsLeft:     r.SlotsLeft,
		Status:        r.Status,
		Idea:          r.Player.Idea,
		Technical:     r.Player.Technical,
		PinnedMetrics: map[string]int{},
		Entity:        r.World.Entity,
		Users:         r.World.Users,
		KnownNPCs:     r.NPCs,
		Log:           r.Log,
	}
	for _, m := range r.World.PinnedMetrics {
		if m == "money" {
			money := r.World.Money
			v.Money = &money
			continue
		}
		v.PinnedMetrics[m] = r.World.Metrics[m]
	}
	for _, t := range r.Todos {
		if !t.Done && !t.Dismissed {
			v.Todos = append(v.Todos, t)
		}
	}
	for _, ri := range r.Risks {
		if ri.Discovered {
			v.KnownRisks = append(v.KnownRisks, ri.Label)
		}
	}
	return v
}
