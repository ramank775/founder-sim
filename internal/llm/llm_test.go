package llm

import (
	"context"
	"strings"
	"testing"

	"github.com/ramank775/founder-sim/engine"
	"github.com/ramank775/founder-sim/kb"
)

func TestExtractJSON(t *testing.T) {
	cases := map[string]string{
		`{"a":1}`: `{"a":1}`,
		"Sure! Here you go:\n```json\n{\"a\":{\"b\":\"}\"}}\n```\nDone.": `{"a":{"b":"}"}}`,
		`prefix {"x":"esc\"aped"} suffix`:                                `{"x":"esc\"aped"}`,
	}
	for in, want := range cases {
		got, err := ExtractJSON(in)
		if err != nil || got != want {
			t.Errorf("ExtractJSON(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := ExtractJSON("no json here"); err == nil {
		t.Error("expected error for no JSON")
	}
}

func TestParseProposalRejectsGarbage(t *testing.T) {
	if _, err := ParseProposal(`{"todos":["x"]}`); err == nil {
		t.Fatal("proposal without narration or outcomes should fail")
	}
	p, err := ParseProposal(`{"narration":"ok","reasoning":"models add this","slot_cost":2}`)
	if err != nil || p.SlotCost != 2 {
		t.Fatalf("unknown fields should be tolerated: %v", err)
	}
}

func TestTurnRetriesOnceThenApplies(t *testing.T) {
	base, err := kb.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	f := &Fake{Script: []string{"this is not json", `{"narration":"Second try.","slot_cost":9,"deltas":[{"op":"money_add","amount":-1000}]}`}}
	n := &Narrator{C: f, KB: base}
	run := engine.NewRun("t", 1, 0, engine.Player{Idea: "x", Savings: 5000, Segment: engine.SegmentB2BSaaS})
	run.Risks = base.RisksFor(run.Player.Segment)

	p, _, err := n.Turn(context.Background(), TurnInput{Run: run, Action: "email three agencies for quotes"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Calls) != 2 {
		t.Fatalf("expected one retry, got %d calls", len(f.Calls))
	}
	if !strings.Contains(f.Calls[1].Messages[len(f.Calls[1].Messages)-1].Content, "not valid") {
		t.Fatal("retry did not carry the error back to the model")
	}
	// Prompt carries the hidden checklist and facts, and the action.
	prompt := f.Calls[0].Messages[1].Content
	for _, want := range []string{"HIDDEN CHECKLIST", "pg_needs_company", "imaginary wall", "FACTS", "Founder's action: email three agencies"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	res := run.Apply(p, engine.Source{Kind: "llm"})
	if res.SlotCost != engine.SlotsPerDay || run.World.Money != 4000 || res.Narration != "Second try." {
		t.Fatalf("apply result wrong: %+v money=%d", res, run.World.Money)
	}
}

func TestClassifyWithFake(t *testing.T) {
	n := &Narrator{C: &Fake{}}
	seg, err := n.Classify(context.Background(), "invoicing for freelancers", "backend dev")
	if err != nil || seg != engine.SegmentB2BSaaS {
		t.Fatalf("classify = %q, %v", seg, err)
	}
}
