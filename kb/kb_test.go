package kb

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ramank775/founder-sim/engine"
)

// ContentDir is the repo's content tree, used by tests across packages.
const ContentDir = "../content"

func TestContentTreeIsValid(t *testing.T) {
	b, err := Load(ContentDir)
	if err != nil {
		t.Fatal(err)
	}
	if b.B.Hash == "" || b.B.Manifest.Version == "" {
		t.Fatal("bundle missing hash or manifest")
	}
	risks := b.RisksFor(engine.SegmentB2BSaaS)
	if len(risks) < 10 {
		t.Fatalf("expected a dozen-ish risks for b2b_saas, got %d", len(risks))
	}
	imaginary := 0
	for _, r := range risks {
		if r.Imaginary {
			imaginary++
		}
	}
	if imaginary == 0 {
		t.Fatal("content should contain at least one imaginary wall")
	}
	// b2b-only risk must not leak into b2c
	if len(b.RisksFor(engine.SegmentB2C)) >= len(risks) {
		t.Fatal("segment filtering not applied")
	}
	facts := b.FactsFor(engine.SegmentB2BSaaS, "compliance", []string{"gst"}, 3)
	if len(facts) != 3 || facts[0].Topic != "gst" {
		t.Fatalf("FactsFor ranking wrong: %+v", facts)
	}
	if len(b.ScenesFor(engine.SegmentB2BSaaS, "ideation", 0)) < 3 {
		t.Fatal("expected ideation scenes")
	}
	if len(b.FrictionFor("gst_registration")) == 0 {
		t.Fatal("expected friction templates for gst_registration")
	}
}

func TestValidateCatchesBadRecords(t *testing.T) {
	b := &Bundle{Manifest: Manifest{Name: "x", Version: "1", AsOf: "2026-09"}}
	b.Facts = []Fact{
		{ID: "a", Topic: "gst", Claim: "x", Provenance: "real"},     // real without source
		{ID: "a", Topic: "gst", Claim: "x", Provenance: "invented"}, // duplicate id
		{ID: "b", Topic: "gst", Claim: "x", Provenance: "guess"},    // bad provenance
		{ID: "c", Topic: "gst", Claim: "x", Provenance: "invented", UsedBy: []string{"scene.nope"}},
	}
	b.Risks = []Risk{{ID: "r", Label: "l", BestCase: "b", WorstCase: "w", TriggerProbPerWeek: 2, AppliesWhen: "sometimes", Provenance: "invented"}}
	errs := Validate(b)
	if len(errs) != 6 {
		t.Fatalf("expected 6 errors, got %d: %v", len(errs), errs)
	}
}

func TestCompiledBundleRoundTrips(t *testing.T) {
	dirB, err := LoadDir(ContentDir)
	if err != nil {
		t.Fatal(err)
	}
	Compile(dirB)
	raw, _ := json.Marshal(dirB)
	p := filepath.Join(t.TempDir(), "content.bundle.json")
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	fromBundle, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if fromBundle.B.Hash != dirB.Hash || len(fromBundle.B.Facts) != len(dirB.Facts) || len(fromBundle.B.Scenes) != len(dirB.Scenes) {
		t.Fatal("bundle does not round-trip")
	}
	// Same content, same hash, every time.
	again, _ := LoadDir(ContentDir)
	if Compile(again).Hash != dirB.Hash {
		t.Fatal("hash not deterministic")
	}
}
