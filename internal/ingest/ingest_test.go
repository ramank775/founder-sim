package ingest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ramank775/founder-sim/internal/llm"
	"github.com/ramank775/founder-sim/kb"
)

// The Klixa story, roughly as it might be told out loud.
const klixaRaw = `ok so klixa. this was the shared camera app for events, like a cousin's wedding was coming up in like six weeks and I'd seen something similar and thought I can build this. so I built it, took maybe four weekends, the app worked, people at the function used it, it was fine. then I wanted to charge for it, like a small fee per event, and I got stuck because I thought you need a registered company to get a payment gateway. I never checked properly. I just assumed. so I parked it. months later someone told me razorpay onboards individuals with just a PAN and a bank account. by then I'd lost interest. also the venue wifi was terrible which I only found out on the day.`

// Scripted model output: stage 1 (normalise) then stage 2 (extract).
const klixaStory = `{"title":"Klixa","summary":"A shared event camera app built for a cousin's wedding, which stalled at payments on a false belief.","segment":"b2c","origin":"event_forced",
"timeline":[{"when":"6 weeks before the wedding","what":"Saw a similar app, decided to build one for the occasion","basis":"told"},{"when":"4 weekends","what":"Built the app","basis":"told"},{"when":"the wedding","what":"Guests used it; venue wifi was terrible","basis":"told"},{"when":"after","what":"Wanted to charge per event, believed a company was required for a payment gateway, parked the project","basis":"told"},{"when":"months later","what":"Learned a PAN and bank account suffice; interest gone","basis":"told"},{"when":"in between","what":"Probably kept the job and let the project drift without a decision","basis":"inferred"}],
"people":[{"role":"cousin","relation":"family","behavior":"provided the deadline and the first audience","basis":"told"},{"role":"acquaintance","relation":"friend","behavior":"casually corrected the belief months too late","basis":"told"}],
"decisions":[{"decision":"Build first vs validate","options":["build","ask guests what they'd want"],"chosen":"build","what_happened":"It worked at the event","later":"No path to revenue was planned","basis":"inferred"},{"decision":"How to accept payments","options":["check gateway requirements","incorporate first","park it"],"chosen":"park it","what_happened":"Project stalled","later":"Lost interest before learning the truth","basis":"told"}],
"walls":[{"belief":"A payment gateway needs an incorporated company","reality":"Gateways onboard individuals with a PAN and a bank account","imaginary":true,"cost_days":120,"basis":"told"}],
"numbers":[{"what":"build time","value":"4 weekends","basis":"told"},{"what":"deadline","value":"6 weeks","basis":"told"}],
"lessons":[{"text":"A belief can kill a project as surely as a failure","basis":"inferred","reason":"the founder never tested the blocker"}]}`

const klixaExtract = `{"scenes":[{"id":"payments_wall","chapter":"ideation","title":"The gateway you think you can't have","situation":"You want to charge for the thing. You are fairly sure a payment gateway needs a registered company.","options":["look up what the gateway actually needs","ask someone who has done it","incorporate first","park it until later"],"truth":"A PAN and a bank account are enough. Do not correct the belief unless the founder tests it.","outcomes":["they check and lose an afternoon","they ask a friend who confirms the myth","they park it"],"delayed_consequences":["months pass with no revenue path","someone mentions the truth casually, too late"],"kb_topics":["payments"]},
{"id":"venue_wifi","chapter":"zero_to_one","title":"The day of the event","situation":"The app is live at the event and the venue wifi is terrible.","truth":"Infrastructure the founder does not control decides the first impression.","outcomes":["uploads crawl; guests give up","it mostly works on mobile data"],"kb_topics":["zero_to_one"]}],
"risks":[{"id":"gateway_needs_company","label":"Believing a payment gateway requires an incorporated company","best_case":"A friend mentions PAN-only onboarding within a week.","worst_case":"The project sits parked for months on a wall that never existed.","trigger_prob_per_week":0.2,"applies_when":"unincorporated","imaginary":true,"basis":"told"}],
"friction":[{"id":"venue_network","process":"app_store","what_happens":"Event-day network is far worse than anything tested; uploads fail in front of the first real users.","delay_days":{"min":0,"max":1},"extra_cost_inr":{"min":0,"max":0},"frequency_hint":"occasional","basis":"told"}],
"archetypes":[{"id":"late_corrector","relation":"acquaintance","sketch":"Knows the true answer to the founder's blocker and mentions it in passing, months after it would have mattered."}],
"facts":[{"id":"four_weekends_mvp","topic":"build_market","chapter":"building","claim":"A technical founder can build a working single-purpose mobile app for an event in about four weekends of evenings and weekends.","basis":"told","numbers":{"days_min":8,"days_max":12},"tags":["self_build"]}]}`

func TestPipelineKlixa(t *testing.T) {
	// Copy the real content tree so reconciliation sees real ids.
	dir := t.TempDir()
	if err := copyTree("../../content", dir); err != nil {
		t.Fatal(err)
	}
	fake := &llm.Fake{Script: []string{"Sure, here it is:\n```json\n" + klixaStory + "\n```", klixaExtract}}
	p := &Pipeline{C: fake, ContentDir: dir}
	res, err := p.Run(context.Background(), "Klixa Story", klixaRaw)
	if err != nil {
		t.Fatal(err)
	}
	if res.Story.Slug != "klixa-story" || res.Story.Segment != "b2c" {
		t.Fatalf("story not normalised: %+v", res.Story)
	}
	if len(res.Bundle.Scenes) != 2 || len(res.Bundle.Risks) != 1 || len(res.Bundle.Facts) != 1 || len(res.Bundle.Archetypes) != 1 {
		t.Fatalf("unexpected record counts: %d %d %d %d", len(res.Bundle.Scenes), len(res.Bundle.Risks), len(res.Bundle.Facts), len(res.Bundle.Archetypes))
	}
	// Anecdotes are never real; told -> estimated, inferred -> invented.
	if res.Bundle.Risks[0].Provenance != kb.ProvEstimated || res.Bundle.Facts[0].Provenance != kb.ProvEstimated {
		t.Fatalf("provenance wrong: %s %s", res.Bundle.Risks[0].Provenance, res.Bundle.Facts[0].Provenance)
	}
	if !res.Bundle.Risks[0].Imaginary || res.Bundle.Risks[0].ID != "klixa-story_gateway_needs_company" {
		t.Fatalf("risk not prefixed / imaginary lost: %+v", res.Bundle.Risks[0])
	}
	if res.Bundle.Scenes[0].Source != "story:klixa-story" || res.Bundle.Scenes[0].Segment[0] != "b2c" {
		t.Fatalf("scene missing source/segment: %+v", res.Bundle.Scenes[0])
	}
	// Files landed where the loader expects and the whole tree still validates.
	if _, err := os.Stat(filepath.Join(dir, "b2c", "scenes.story-klixa-story.jsonl")); err != nil {
		t.Fatal("scenes file not written to the segment dir")
	}
	if _, err := os.Stat(filepath.Join(dir, "stories", "klixa-story.raw.md")); err != nil {
		t.Fatal("raw story not kept")
	}
	if _, err := kb.LoadDir(dir); err != nil {
		t.Fatalf("tree invalid after ingest: %v", err)
	}
	if !strings.Contains(res.Report, "IMAGINARY") || !strings.Contains(res.Report, "Research to-dos") {
		t.Fatalf("report missing sections:\n%s", res.Report)
	}
	// Second ingest of the same story: every id collides, nothing is duplicated.
	fake.Script = []string{klixaStory, klixaExtract}
	res2, err := p.Run(context.Background(), "Klixa Story", klixaRaw)
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Dropped) != 6 || len(res2.Bundle.Scenes) != 0 {
		t.Fatalf("re-ingest should drop all as collisions, got dropped=%v", res2.Dropped)
	}
}

func TestPipelineRejectsGarbage(t *testing.T) {
	p := &Pipeline{C: &llm.Fake{Script: []string{"not json", "still not json"}}, ContentDir: t.TempDir()}
	if _, err := p.Run(context.Background(), "x", strings.Repeat("words ", 20)); err == nil {
		t.Fatal("expected failure on unusable model output")
	}
	if _, err := p.Run(context.Background(), "x", "too short"); err == nil {
		t.Fatal("expected failure on short story")
	}
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}
