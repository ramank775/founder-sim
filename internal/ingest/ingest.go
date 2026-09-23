// Package ingest turns a founder's story, told in any form (voice
// transcript, chat dump, bullet notes, a blog post), into game content.
//
// Pipeline:
//
//  1. Normalise: raw text -> Story (timeline, people, decisions, walls,
//     numbers, lessons). The model fills connective tissue but must mark
//     each item as told or inferred.
//  2. Extract: Story -> content records (scenes, risks, friction,
//     archetypes, facts) in the kb shapes. Anecdotes are never "real":
//     everything is provenance "estimated" (told) or "invented" (inferred),
//     sourced to the story itself, so the research agents know what to
//     go and confirm.
//  3. Reconcile: prefix ids, drop collisions with existing content, run
//     kb.Validate against the merged tree, write files and a report.
//
// The raw story and the normalised Story are kept under content/stories so
// the extraction can be re-run when the prompts improve without asking the
// founder to tell it again.
package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ramank775/founder-sim/internal/llm"
	"github.com/ramank775/founder-sim/kb"
)

// Story is the normalised form of what the founder told us.
type Story struct {
	Slug     string   `json:"slug"`
	Title    string   `json:"title"`
	Summary  string   `json:"summary"`
	Segment  string   `json:"segment,omitempty"` // b2b_saas | b2c | deep_tech | ""
	Origin   string   `json:"origin,omitempty"`  // own_pain | saw_and_liked | event_forced | ""
	Timeline []Beat   `json:"timeline"`
	People   []Person `json:"people"`
	Decision []Choice `json:"decisions"`
	Walls    []Wall   `json:"walls"`
	Numbers  []Number `json:"numbers"`
	Lessons  []Item   `json:"lessons"`
}

type Item struct {
	Text   string `json:"text"`
	Basis  string `json:"basis"` // told | inferred
	Reason string `json:"reason,omitempty"`
}

type Beat struct {
	When  string `json:"when"` // relative: "week 2", "after launch", or a date if given
	What  string `json:"what"`
	Basis string `json:"basis"`
}

type Person struct {
	Role     string `json:"role"` // father, friend, agency PM, first customer...
	Relation string `json:"relation"`
	Behavior string `json:"behavior"` // how they acted, as a pattern
	Basis    string `json:"basis"`
}

type Choice struct {
	Decision     string   `json:"decision"`
	Options      []string `json:"options"`
	Chosen       string   `json:"chosen"`
	WhatHappened string   `json:"what_happened"`
	Later        string   `json:"later,omitempty"` // delayed consequence
	Basis        string   `json:"basis"`
}

type Wall struct {
	Belief    string `json:"belief"`
	Reality   string `json:"reality,omitempty"`
	Imaginary *bool  `json:"imaginary,omitempty"` // nil = unknown
	CostDays  int    `json:"cost_days,omitempty"`
	Basis     string `json:"basis"`
}

type Number struct {
	What  string `json:"what"`
	Value string `json:"value"` // as told, e.g. "₹3 lakh", "6 weeks"
	Basis string `json:"basis"`
}

// Result of one ingestion.
type Result struct {
	Story   Story
	Bundle  kb.Bundle // records written (after reconciliation)
	Dropped []string  // ids dropped as collisions
	Files   []string
	Report  string
	Raw     [2]string // raw model outputs for the two stages, for debugging
}

type Pipeline struct {
	C          llm.Client
	ContentDir string
	Now        func() time.Time
}

func (p *Pipeline) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

var (
	slugRe = regexp.MustCompile(`[^a-z0-9]+`)
	idRe   = regexp.MustCompile(`[^a-z0-9_]+`)
)

// idSlug normalises a model-chosen record id to snake_case.
func idSlug(s string) string {
	s = strings.Trim(idRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "_"), "_")
	if len(s) > 48 {
		s = s[:48]
	}
	return s
}

func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Trim(slugRe.ReplaceAllString(s, "-"), "-")
	if len(s) > 40 {
		s = s[:40]
	}
	if s == "" {
		s = "story"
	}
	return s
}

const normalisePrompt = `You are the story editor for Founder Sim, a simulation of starting a company in India. A founder has told their story below, in whatever form they had: a voice transcript, chat messages, notes, a blog post. It may be messy, out of order, and full of asides.

Rebuild it into a clean, structured story. Fill in the connective tissue a game designer needs (what came before, what the decision points were, who was involved) but be honest about what the founder actually said versus what you inferred: every item carries "basis": "told" or "inferred". Never invent numbers; if a number is inferred, say so and give the reason.

Look especially for:
- decisions with alternatives the founder did not take, and what happened later because of the choice
- "walls": something the founder believed was a blocker. If the story shows the belief was wrong, set imaginary=true and give the reality; if unknown, omit imaginary
- people and how they behaved, as reusable patterns (not names)
- numbers: costs, durations, counts, as told
- the idea's origin: own_pain, saw_and_liked, event_forced
- the segment if it is clear: b2b_saas, b2c, deep_tech

Return one JSON object only:
{"title": string, "summary": 2-4 sentences, "segment": string|"", "origin": string|"",
 "timeline": [{"when": string, "what": string, "basis": "told"|"inferred"}],
 "people": [{"role": string, "relation": string, "behavior": string, "basis": string}],
 "decisions": [{"decision": string, "options": [string], "chosen": string, "what_happened": string, "later": string, "basis": string}],
 "walls": [{"belief": string, "reality": string, "imaginary": true|false, "cost_days": int, "basis": string}],
 "numbers": [{"what": string, "value": string, "basis": string}],
 "lessons": [{"text": string, "basis": string, "reason": string}]}

Do not include any personal names, phone numbers, emails or company names of private individuals; describe people by role.`

const extractPrompt = `You turn a structured founder story into Founder Sim content records. The engine owns state and dice; content describes situations, options and outcome ranges. Nothing you write is a script. The game never tells the player what to do, so records must describe what happens, never what the founder should do.

Produce records in these exact shapes (JSON, one object with five arrays):

{"scenes": [{"id": string, "chapter": "ideation"|"building"|"zero_to_one"|"compliance", "title": string,
   "situation": "the situation as the player would meet it, second person, present tense",
   "options": ["things a player might phrase"], "truth": "what is really going on, for the narrator only",
   "outcomes": ["outcome range the narrator may roll among"], "delayed_consequences": [string],
   "kb_topics": ["gst"|"incorporation"|"payments"|"build_market"|"zero_to_one"|"life"|"compliance"|"tax"|"registration"|"invoicing"]}],
 "risks": [{"id": string, "label": "Operating without X / Believing Y", "best_case": string, "worst_case": string,
   "trigger_prob_per_week": 0.02-0.3, "applies_when": ""|"unincorporated"|"incorporated"|"has_customers"|"has_gstin"|"agency_build",
   "imaginary": bool, "basis": "told"|"inferred"}],
 "friction": [{"id": string, "process": "company_name_approval"|"gst_registration"|"bank_account"|"gateway_kyc"|"agency"|"any_government_portal"|"dsc_din"|"pan_tan"|"udyam"|"trademark"|"app_store",
   "what_happens": "paraphrased pattern, no names", "delay_days": {"min": int, "max": int}, "extra_cost_inr": {"min": int, "max": int},
   "frequency_hint": "rare"|"occasional"|"common", "basis": string}],
 "archetypes": [{"id": string, "relation": string, "sketch": "how they behave, as a pattern the narrator can play"}],
 "facts": [{"id": string, "topic": string, "chapter": string, "claim": "a concrete, narrator-usable statement", "basis": string,
   "numbers": {"cost_inr_min": int, "cost_inr_max": int, "days_min": int, "days_max": int}, "tags": [string], "wall": "real"|"imaginary"|""}]}

Rules:
- ids: short snake_case, no prefix (the pipeline adds one). Unique within your output.
- Every wall in the story becomes a risk with "imaginary" set from the story, AND a scene. Every decision with a delayed consequence becomes a scene. Every recurring person becomes an archetype. Every process that went wrong becomes a friction template. Numbers the founder gave become facts with "numbers" filled.
- Prefer few, sharp records over many vague ones. Typically 2-6 scenes, 1-4 risks, 0-4 friction, 1-4 archetypes, 0-6 facts.
- Include "numbers" keys only when known. Omit "wall" unless the fact is about a blocker.
- No names, handles, or identifying details of private people or small companies.`

// Run executes the pipeline on raw text.
func (p *Pipeline) Run(ctx context.Context, slug, raw string) (*Result, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) < 40 {
		return nil, fmt.Errorf("the story is too short to work with (%d chars)", len(raw))
	}
	slug = Slugify(slug)
	res := &Result{}

	// Stage 1: normalise.
	out, err := p.jsonCall(ctx, normalisePrompt, "FOUNDER'S STORY:\n\n"+raw, 3000)
	if err != nil {
		return nil, fmt.Errorf("normalise: %w", err)
	}
	res.Raw[0] = out
	if err := json.Unmarshal([]byte(out), &res.Story); err != nil {
		return nil, fmt.Errorf("normalise: bad JSON: %w", err)
	}
	res.Story.Slug = slug
	if res.Story.Title == "" {
		res.Story.Title = slug
	}
	if len(res.Story.Timeline) == 0 && len(res.Story.Decision) == 0 {
		return nil, fmt.Errorf("normalise: the model found no timeline or decisions in the story")
	}

	// Stage 2: extract records.
	storyJSON, _ := json.MarshalIndent(res.Story, "", " ")
	out, err = p.jsonCall(ctx, extractPrompt, "STRUCTURED STORY:\n\n"+string(storyJSON), 4000)
	if err != nil {
		return nil, fmt.Errorf("extract: %w", err)
	}
	res.Raw[1] = out
	var ex extraction
	if err := json.Unmarshal([]byte(out), &ex); err != nil {
		return nil, fmt.Errorf("extract: bad JSON: %w", err)
	}

	// Stage 3: reconcile.
	existing, err := kb.LoadDir(p.ContentDir)
	if err != nil {
		return nil, fmt.Errorf("existing content: %w", err)
	}
	res.Bundle, res.Dropped = p.reconcile(slug, res.Story, ex, existing)
	merged := *existing
	merged.Facts = append(merged.Facts, res.Bundle.Facts...)
	merged.Risks = append(merged.Risks, res.Bundle.Risks...)
	merged.Friction = append(merged.Friction, res.Bundle.Friction...)
	merged.Archetypes = append(merged.Archetypes, res.Bundle.Archetypes...)
	merged.Scenes = append(merged.Scenes, res.Bundle.Scenes...)
	if errs := kb.Validate(&merged); len(errs) > 0 {
		return res, fmt.Errorf("extracted records failed validation: %v (and %d more)", errs[0], len(errs)-1)
	}

	// Write.
	files, err := p.write(slug, raw, res)
	if err != nil {
		return res, err
	}
	res.Files = files
	res.Report = p.report(slug, res)
	rp := filepath.Join(p.ContentDir, "reports", "story-"+slug+".md")
	if err := os.WriteFile(rp, []byte(res.Report), 0o644); err != nil {
		return res, err
	}
	res.Files = append(res.Files, rp)
	return res, nil
}

type extraction struct {
	Scenes []struct {
		kb.Scene
	} `json:"scenes"`
	Risks []struct {
		kb.Risk
		Basis string `json:"basis"`
	} `json:"risks"`
	Friction []struct {
		kb.Friction
		Basis string `json:"basis"`
	} `json:"friction"`
	Archetypes []kb.Archetype `json:"archetypes"`
	Facts      []struct {
		kb.Fact
		Basis string `json:"basis"`
	} `json:"facts"`
}

func provFor(basis string) string {
	if strings.EqualFold(strings.TrimSpace(basis), "told") {
		return kb.ProvEstimated
	}
	return kb.ProvInvented
}

func (p *Pipeline) reconcile(slug string, st Story, ex extraction, existing *kb.Bundle) (kb.Bundle, []string) {
	var b kb.Bundle
	var dropped []string
	asOf := p.now().Format("2006-01")
	src := []kb.Source{{Title: "founder story: " + slug, Retrieved: p.now().Format("2006-01-02")}}
	taken := map[string]bool{}
	for _, f := range existing.Facts {
		taken["fact:"+f.ID] = true
	}
	for _, r := range existing.Risks {
		taken["risk:"+r.ID] = true
	}
	for _, f := range existing.Friction {
		taken["friction:"+f.ID] = true
	}
	for _, a := range existing.Archetypes {
		taken["archetype:"+a.ID] = true
	}
	for _, s := range existing.Scenes {
		taken["scene:"+s.ID] = true
	}
	claim := func(kind, id string) bool {
		if id == "" || taken[kind+":"+id] {
			dropped = append(dropped, kind+":"+id)
			return false
		}
		taken[kind+":"+id] = true
		return true
	}
	var segs []string
	if st.Segment != "" {
		segs = []string{st.Segment}
	}
	note := "From a founder's story (" + slug + "); an anecdote, not a sourced fact. Research agents should confirm or replace."

	for _, s := range ex.Scenes {
		sc := s.Scene
		sc.ID = "scene." + slug + "." + idSlug(sc.ID)
		if !claim("scene", sc.ID) {
			continue
		}
		sc.Source = "story:" + slug
		sc.Segment = segs
		b.Scenes = append(b.Scenes, sc)
	}
	for _, r := range ex.Risks {
		rk := r.Risk
		rk.ID = slug + "_" + idSlug(rk.ID)
		if !claim("risk", rk.ID) {
			continue
		}
		rk.Provenance, rk.Sources, rk.AsOf, rk.Note, rk.Segment = provFor(r.Basis), src, asOf, note, segs
		if rk.TriggerProbPerWeek <= 0 || rk.TriggerProbPerWeek > 1 {
			rk.TriggerProbPerWeek = 0.1
		}
		b.Risks = append(b.Risks, rk)
	}
	for _, f := range ex.Friction {
		fr := f.Friction
		fr.ID = "friction." + slug + "." + idSlug(fr.ID)
		if !claim("friction", fr.ID) {
			continue
		}
		fr.Provenance, fr.Sources, fr.AsOf, fr.Note = provFor(f.Basis), src, asOf, note
		if fr.FrequencyHint == "" {
			fr.FrequencyHint = "occasional"
		}
		if fr.DelayDays.Max < fr.DelayDays.Min {
			fr.DelayDays.Max = fr.DelayDays.Min
		}
		if fr.ExtraCostINR.Max < fr.ExtraCostINR.Min {
			fr.ExtraCostINR.Max = fr.ExtraCostINR.Min
		}
		b.Friction = append(b.Friction, fr)
	}
	for _, a := range ex.Archetypes {
		a.ID = slug + "_" + idSlug(a.ID)
		if !claim("archetype", a.ID) {
			continue
		}
		a.Provenance = kb.ProvEstimated
		b.Archetypes = append(b.Archetypes, a)
	}
	for _, f := range ex.Facts {
		fc := f.Fact
		fc.ID = "story." + slug + "." + idSlug(fc.ID)
		if !claim("fact", fc.ID) {
			continue
		}
		if fc.Topic == "" {
			fc.Topic = "life"
		}
		fc.Provenance, fc.Sources, fc.AsOf, fc.Note, fc.Segment = provFor(f.Basis), src, asOf, note, segs
		fc.Confidence = "low"
		fc.UsedBy = nil
		b.Facts = append(b.Facts, fc)
	}
	sort.Strings(dropped)
	return b, dropped
}

func (p *Pipeline) write(slug, raw string, res *Result) ([]string, error) {
	seg := "common"
	if res.Story.Segment != "" {
		seg = res.Story.Segment
	}
	var files []string
	put := func(rel string, recs any) error {
		path := filepath.Join(p.ContentDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		var sb strings.Builder
		sb.WriteString("# generated by founder-sim-content ingest from content/stories/" + slug + ".raw.md; re-run to regenerate\n")
		if err := writeJSONL(&sb, recs); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
			return err
		}
		files = append(files, path)
		return nil
	}
	if len(res.Bundle.Scenes) > 0 {
		if err := put(filepath.Join(seg, "scenes.story-"+slug+".jsonl"), res.Bundle.Scenes); err != nil {
			return nil, err
		}
	}
	if len(res.Bundle.Risks) > 0 {
		if err := put(filepath.Join(seg, "risks.story-"+slug+".jsonl"), res.Bundle.Risks); err != nil {
			return nil, err
		}
	}
	if len(res.Bundle.Friction) > 0 {
		if err := put(filepath.Join("common", "friction.story-"+slug+".jsonl"), res.Bundle.Friction); err != nil {
			return nil, err
		}
	}
	if len(res.Bundle.Archetypes) > 0 {
		if err := put(filepath.Join("common", "archetypes.story-"+slug+".jsonl"), res.Bundle.Archetypes); err != nil {
			return nil, err
		}
	}
	if len(res.Bundle.Facts) > 0 {
		if err := put(filepath.Join(seg, "facts.story-"+slug+".jsonl"), res.Bundle.Facts); err != nil {
			return nil, err
		}
	}
	// Keep the raw and normalised story for re-runs.
	sd := filepath.Join(p.ContentDir, "stories")
	if err := os.MkdirAll(sd, 0o755); err != nil {
		return nil, err
	}
	rawPath := filepath.Join(sd, slug+".raw.md")
	if err := os.WriteFile(rawPath, []byte(raw+"\n"), 0o644); err != nil {
		return nil, err
	}
	stJSON, _ := json.MarshalIndent(res.Story, "", " ")
	stPath := filepath.Join(sd, slug+".json")
	if err := os.WriteFile(stPath, stJSON, 0o644); err != nil {
		return nil, err
	}
	return append(files, rawPath, stPath), nil
}

func writeJSONL(sb *strings.Builder, recs any) error {
	raw, err := json.Marshal(recs)
	if err != nil {
		return err
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return err
	}
	for _, it := range items {
		sb.Write(it)
		sb.WriteString("\n")
	}
	return nil
}

func (p *Pipeline) report(slug string, res *Result) string {
	var b strings.Builder
	st := res.Story
	fmt.Fprintf(&b, "# Story ingest: %s (%s)\n\n%s\n\n", st.Title, slug, st.Summary)
	fmt.Fprintf(&b, "Segment: %s · Origin: %s\n\n", or(st.Segment, "unknown"), or(st.Origin, "unknown"))
	fmt.Fprintf(&b, "## Written\n\n%d scenes, %d risks, %d friction, %d archetypes, %d facts. All provenance is `estimated` (told) or `invented` (inferred); none is `real`.\n\n",
		len(res.Bundle.Scenes), len(res.Bundle.Risks), len(res.Bundle.Friction), len(res.Bundle.Archetypes), len(res.Bundle.Facts))
	for _, f := range res.Files {
		fmt.Fprintf(&b, "- %s\n", f)
	}
	if len(res.Dropped) > 0 {
		fmt.Fprintf(&b, "\n## Dropped (id collisions with existing content)\n\n")
		for _, d := range res.Dropped {
			fmt.Fprintf(&b, "- %s\n", d)
		}
	}
	b.WriteString("\n## Walls\n\n")
	for _, w := range st.Walls {
		im := "unknown"
		if w.Imaginary != nil {
			if *w.Imaginary {
				im = "IMAGINARY"
			} else {
				im = "real"
			}
		}
		fmt.Fprintf(&b, "- [%s, %s] believed: %s", im, w.Basis, w.Belief)
		if w.Reality != "" {
			fmt.Fprintf(&b, " — reality: %s", w.Reality)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n## Inferred (not told) — check with the founder\n\n")
	n := 0
	for _, t := range st.Timeline {
		if t.Basis != "told" {
			fmt.Fprintf(&b, "- timeline: %s — %s\n", t.When, t.What)
			n++
		}
	}
	for _, d := range st.Decision {
		if d.Basis != "told" {
			fmt.Fprintf(&b, "- decision: %s\n", d.Decision)
			n++
		}
	}
	for _, l := range st.Lessons {
		if l.Basis != "told" {
			fmt.Fprintf(&b, "- lesson: %s (%s)\n", l.Text, l.Reason)
			n++
		}
	}
	if n == 0 {
		b.WriteString("- nothing inferred\n")
	}
	b.WriteString("\n## Research to-dos (claims worth sourcing so they can become `real`)\n\n")
	for _, f := range res.Bundle.Facts {
		fmt.Fprintf(&b, "- %s: %s\n", f.ID, f.Claim)
	}
	for _, w := range st.Walls {
		fmt.Fprintf(&b, "- confirm wall: %s\n", w.Belief)
	}
	for _, num := range st.Numbers {
		fmt.Fprintf(&b, "- number as told: %s = %s (%s)\n", num.What, num.Value, num.Basis)
	}
	return b.String()
}

func or(a, b string) string {
	if a == "" {
		return b
	}
	return a
}

// jsonCall asks the model for JSON with one repair retry.
func (p *Pipeline) jsonCall(ctx context.Context, system, user string, maxTokens int) (string, error) {
	msgs := []llm.Message{{Role: "system", Content: system}, {Role: "user", Content: user}}
	var last error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := p.C.Complete(ctx, llm.Request{Messages: msgs, JSON: true, MaxTokens: maxTokens, Temperature: 0.3})
		if err != nil {
			return "", err
		}
		js, err := llm.ExtractJSON(resp.Content)
		if err == nil && json.Valid([]byte(js)) {
			return js, nil
		}
		if err == nil {
			err = fmt.Errorf("invalid JSON")
		}
		last = err
		msgs = append(msgs, llm.Message{Role: "assistant", Content: resp.Content},
			llm.Message{Role: "user", Content: "That was not valid JSON (" + err.Error() + "). Return only the corrected JSON object."})
	}
	return "", last
}
