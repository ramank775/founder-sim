package engine

import (
	"encoding/json"
	"testing"
)

func newTestRun() *Run {
	return NewRun("r1", 42, 1_000_000, Player{
		Idea: "invoicing for freelancers", Age: 30, Savings: 500_000, Technical: true,
		Segment:            SegmentB2BSaaS,
		MonthlyObligations: []Obligation{{Label: "rent", Amount: 30_000}, {Label: "EMI", Amount: 20_000}},
	})
}

func TestRNGIsReplayable(t *testing.T) {
	a := newTestRun()
	b := newTestRun()
	for i := 0; i < 100; i++ {
		if a.Float() != b.Float() {
			t.Fatalf("draw %d differs", i)
		}
	}
	// Save/load mid-sequence and continue identically.
	raw, _ := json.Marshal(a)
	var c Run
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		if a.Float() != c.Float() {
			t.Fatalf("post-reload draw %d differs", i)
		}
	}
}

func TestDifferentSeedsDiffer(t *testing.T) {
	a := NewRun("a", 1, 0, Player{})
	b := NewRun("b", 2, 0, Player{})
	same := 0
	for i := 0; i < 100; i++ {
		if a.IntN(1000) == b.IntN(1000) {
			same++
		}
	}
	if same > 5 {
		t.Fatalf("seeds 1 and 2 agreed on %d/100 draws", same)
	}
}

func TestPickHonoursWeights(t *testing.T) {
	r := newTestRun()
	counts := [3]int{}
	for i := 0; i < 10_000; i++ {
		counts[r.Pick([]float64{0.1, 0.3, 0.6})]++
	}
	if counts[2] < counts[1] || counts[1] < counts[0] {
		t.Fatalf("weights not honoured: %v", counts)
	}
	if r.Pick([]float64{0, 0, 0}) != 0 {
		t.Fatal("all-zero weights should pick 0")
	}
}

func TestCatchUpResolvesDaysAndBills(t *testing.T) {
	r := newTestRun()
	r.TimeScaleMs = 1000
	reps := r.CatchUp(r.StartedAt + 30*1000)
	if len(reps) != 30 || r.CurrentDay != 30 {
		t.Fatalf("expected 30 days, got %d (day %d)", len(reps), r.CurrentDay)
	}
	if r.World.Money != 500_000-50_000 {
		t.Fatalf("obligations not deducted: %d", r.World.Money)
	}
	if reps[29].Bills != 50_000 {
		t.Fatalf("day 30 bills = %d", reps[29].Bills)
	}
	if r.SlotsLeft != SlotsPerDay {
		t.Fatal("slots not reset")
	}
}

func TestCatchUpIsCapped(t *testing.T) {
	r := newTestRun()
	r.TimeScaleMs = 1000
	r.CatchUp(r.StartedAt + 10_000*1000)
	if r.CurrentDay != MaxCatchUpDays {
		t.Fatalf("expected cap at %d, got %d", MaxCatchUpDays, r.CurrentDay)
	}
	// After re-anchoring, a second call at the same time adds nothing.
	if n := len(r.CatchUp(r.StartedAt + 10_000*1000)); n != 0 {
		t.Fatalf("re-anchor failed, resolved %d more days", n)
	}
}

func TestSetTimeScaleDoesNotJumpDay(t *testing.T) {
	r := newTestRun()
	r.TimeScaleMs = 1000
	now := r.StartedAt + 5*1000
	r.CatchUp(now)
	if err := r.SetTimeScale(now, 60_000); err != nil {
		t.Fatal(err)
	}
	if r.DayAt(now+59_000) != 5 || r.DayAt(now+60_000) != 6 {
		t.Fatalf("time scale change jumped the day: %d / %d", r.DayAt(now+59_000), r.DayAt(now+60_000))
	}
}

func TestPendingEventsRollAndSurface(t *testing.T) {
	r := newTestRun()
	r.TimeScaleMs = 1000
	r.Apply(Proposal{Pending: []PendingProposal{
		{InDays: 2, Description: "Customer asks for a GST invoice", Probability: 1},
		{InDays: 2, Description: "never happens", Probability: 0},
	}}, Source{Kind: "llm"})
	reps := r.CatchUp(r.StartedAt + 3*1000)
	var fired []string
	for _, rep := range reps {
		for _, ev := range rep.Fired {
			fired = append(fired, ev.Description)
		}
	}
	if len(fired) != 1 || fired[0] != "Customer asks for a GST invoice" {
		t.Fatalf("fired = %v", fired)
	}
	if len(r.View().Todos) != 1 {
		t.Fatalf("expected the event to become a todo, got %d", len(r.View().Todos))
	}
}

func TestLLMCannotMintMoney(t *testing.T) {
	r := newTestRun()
	res := r.Apply(Proposal{Narration: "A VC wires you money.", Deltas: []Delta{{Op: "money_add", Amount: 10_000_000}}}, Source{Kind: "llm"})
	if len(res.Rejected) != 1 || r.World.Money != 500_000 {
		t.Fatalf("mint not rejected: %+v money=%d", res.Rejected, r.World.Money)
	}
	res = r.Apply(Proposal{Deltas: []Delta{{Op: "money_add", Amount: -600_000}}}, Source{Kind: "llm"})
	if len(res.Rejected) != 1 || r.World.Money != 500_000 {
		t.Fatalf("overspend not rejected: %+v", res.Rejected)
	}
}

func TestPluginsCannotTouchCoreState(t *testing.T) {
	r := newTestRun()
	src := Source{Kind: "plugin", Name: "notepad"}
	res := r.Apply(Proposal{
		SlotCost:  2,
		Narration: "plugins do not narrate",
		Deltas: []Delta{
			{Op: "money_add", Amount: -1},
			{Op: "users_add", Amount: 5},
			{Op: "metric_set", Key: "signups", Amount: 3},       // not namespaced
			{Op: "metric_set", Key: "notepad.notes", Amount: 3}, // ok
		},
		Todos:   []string{"Call the CA"},
		Pending: []PendingProposal{{InDays: 1, Description: "Alarm: follow up with agency", Probability: 1}},
		NPCs:    []NPCProposal{{Name: "Ravi", Relation: "friend"}},
	}, src)
	if r.World.Money != 500_000 || r.World.Users != 0 || r.SlotsLeft != SlotsPerDay {
		t.Fatal("plugin changed core state")
	}
	if len(res.Applied) != 1 || res.Applied[0].Key != "notepad.notes" {
		t.Fatalf("applied = %+v", res.Applied)
	}
	if len(res.Rejected) != 5 { // slot_cost, narration, money, users, metric
		t.Fatalf("expected 5 rejections, got %d: %+v", len(res.Rejected), res.Rejected)
	}
	if len(r.Todos) != 1 || len(r.Pending) != 1 || len(r.NPCs) != 1 {
		t.Fatal("plugin additions not applied")
	}
}

func TestOutcomeChoiceIsSeeded(t *testing.T) {
	p := Proposal{Outcomes: []Outcome{
		{Weight: 1, Narration: "agency ghosts you"},
		{Weight: 1, Narration: "agency delivers", Deltas: []Delta{{Op: "build_phase_add", Amount: 1}}},
	}}
	a := newTestRun()
	b := newTestRun()
	for i := 0; i < 20; i++ {
		ra := a.Apply(p, Source{Kind: "llm"})
		rb := b.Apply(p, Source{Kind: "llm"})
		if ra.ChosenOutcome != rb.ChosenOutcome {
			t.Fatalf("turn %d: same seed chose different outcomes", i)
		}
		a.SlotsLeft, b.SlotsLeft = SlotsPerDay, SlotsPerDay
	}
}

func TestSlotsClampAndDayEnds(t *testing.T) {
	r := newTestRun()
	res := r.Apply(Proposal{SlotCost: 99}, Source{Kind: "llm"})
	if res.SlotCost != SlotsPerDay || !res.DayEnded || r.SlotsLeft != 0 {
		t.Fatalf("clamp failed: %+v slots=%d", res, r.SlotsLeft)
	}
}

func TestViewHidesChecklistUntilDiscovered(t *testing.T) {
	r := newTestRun()
	r.Risks = []RiskItem{{ID: "gst", Label: "No GSTIN"}}
	if len(r.View().KnownRisks) != 0 {
		t.Fatal("hidden risk leaked")
	}
	r.Apply(Proposal{Deltas: []Delta{{Op: "risk_discover", Key: "gst"}}}, Source{Kind: "llm"})
	if len(r.View().KnownRisks) != 1 {
		t.Fatal("discovered risk not shown")
	}
	if r.View().Money != nil {
		t.Fatal("money shown without pin")
	}
	_, cost := r.CheckBalance()
	if !cost {
		t.Fatal("checking balance should cost a slot")
	}
}

func TestOnlyFounderEndsRun(t *testing.T) {
	r := newTestRun()
	if err := r.PlayerExit("shutdown", "burnt out"); err != nil {
		t.Fatal(err)
	}
	if r.Status != "exited" || r.Exit.Note != "burnt out" {
		t.Fatal("exit not recorded")
	}
	res := r.Apply(Proposal{Narration: "x"}, Source{Kind: "llm"})
	if len(res.Rejected) == 0 {
		t.Fatal("exited run accepted a proposal")
	}
}
