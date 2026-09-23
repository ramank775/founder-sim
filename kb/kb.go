// Package kb loads the knowledge base: the facts, hidden-checklist items,
// friction templates, NPC archetypes and scenario cards that ground the
// narrator. The engine is easy; this is the moat.
//
// Content lives OUTSIDE the binary, in a directory of JSON Lines files
// (see content/README.md), so story and research can change without
// rebuilding or redeploying the engine. The founder-sim-content tool
// validates a directory and compiles it into one dated Bundle; the server
// loads either a directory (development) or a bundle (production).
//
// Every record says whether it is real, invented or estimated, and when it
// was true. Invented is fine; unmarked is not.
package kb

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ramank775/founder-sim/engine"
)

// Source is where a record came from.
type Source struct {
	URL       string `json:"url,omitempty"`
	Title     string `json:"title,omitempty"`
	Retrieved string `json:"retrieved,omitempty"` // YYYY-MM-DD
}

// Provenance values.
const (
	ProvReal      = "real"
	ProvInvented  = "invented"
	ProvEstimated = "estimated"
)

// Numbers carries the quantitative part of a fact, all optional.
type Numbers struct {
	CostINRMin *int64 `json:"cost_inr_min,omitempty"`
	CostINRMax *int64 `json:"cost_inr_max,omitempty"`
	DaysMin    *int   `json:"days_min,omitempty"`
	DaysMax    *int   `json:"days_max,omitempty"`
}

// Fact is one grounding statement (research plan §B4).
type Fact struct {
	ID         string   `json:"id"`
	Segment    []string `json:"segment,omitempty"` // empty = all
	Topic      string   `json:"topic"`             // gst, incorporation, payments, build_market, zero_to_one, life...
	Chapter    string   `json:"chapter,omitempty"` // ideation | building | zero_to_one | compliance
	Claim      string   `json:"claim"`
	Numbers    Numbers  `json:"numbers,omitempty"`
	Provenance string   `json:"provenance"`
	Sources    []Source `json:"sources,omitempty"`
	AsOf       string   `json:"as_of,omitempty"`
	Confidence string   `json:"confidence,omitempty"` // high | medium | low
	Wall       string   `json:"wall,omitempty"`       // real | imaginary | ""
	UsedBy     []string `json:"used_by,omitempty"`    // scene ids
	Tags       []string `json:"tags,omitempty"`
	Note       string   `json:"note,omitempty"` // provenance caveats for humans, never shown to the model
}

// Risk is a hidden-checklist template. The engine instantiates per run.
type Risk struct {
	ID                 string   `json:"id"`
	Segment            []string `json:"segment,omitempty"`
	Label              string   `json:"label"`
	BestCase           string   `json:"best_case"`
	WorstCase          string   `json:"worst_case"`
	TriggerProbPerWeek float64  `json:"trigger_prob_per_week"`
	Imaginary          bool     `json:"imaginary,omitempty"`
	AppliesWhen        string   `json:"applies_when,omitempty"`
	Provenance         string   `json:"provenance"`
	Sources            []Source `json:"sources,omitempty"`
	AsOf               string   `json:"as_of,omitempty"`
	Note               string   `json:"note,omitempty"`
}

// Friction is a "person behind the counter" event template (§B4).
type Friction struct {
	ID            string   `json:"id"`
	Process       string   `json:"process"` // company_name_approval, gst_registration, bank_account, gateway_kyc, agency...
	WhatHappens   string   `json:"what_happens"`
	DelayDays     Range    `json:"delay_days"`
	ExtraCostINR  Range    `json:"extra_cost_inr"`
	FrequencyHint string   `json:"frequency_hint"` // rare | occasional | common
	Provenance    string   `json:"provenance"`
	Sources       []Source `json:"sources,omitempty"`
	AsOf          string   `json:"as_of,omitempty"`
	Note          string   `json:"note,omitempty"`
}

type Range struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// Archetype seeds engine-generated characters.
type Archetype struct {
	ID         string `json:"id"`
	Relation   string `json:"relation"`
	Sketch     string `json:"sketch"`
	Provenance string `json:"provenance"`
}

// Scene is a scenario card from the story bible: a situation the player
// may walk into, never a script. The narrator gets the eligible ones.
type Scene struct {
	ID           string   `json:"id"`
	Chapter      string   `json:"chapter"`
	Title        string   `json:"title"`
	Situation    string   `json:"situation"`
	Options      []string `json:"options,omitempty"`  // things a player might phrase; never shown to them
	Truth        string   `json:"truth,omitempty"`    // what's really going on
	Outcomes     []string `json:"outcomes,omitempty"` // outcome range the narrator may roll among
	Consequences []string `json:"delayed_consequences,omitempty"`
	Topics       []string `json:"kb_topics,omitempty"`
	Segment      []string `json:"segment,omitempty"`
	Source       string   `json:"source,omitempty"` // "raman:klixa" etc. — where the beat came from
}

// Manifest names and dates a content set.
type Manifest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	AsOf    string `json:"as_of"`
}

// Bundle is a whole content set, as compiled by founder-sim-content.
type Bundle struct {
	Manifest   Manifest    `json:"manifest"`
	Facts      []Fact      `json:"facts"`
	Risks      []Risk      `json:"risks"`
	Friction   []Friction  `json:"friction"`
	Archetypes []Archetype `json:"archetypes"`
	Scenes     []Scene     `json:"scenes"`
	// Hash is over the sorted record ids and bodies; the server logs it so
	// you know which content a run was played against.
	Hash string `json:"hash"`
	// Index is reserved for a precomputed retrieval index (embeddings).
	// Empty in v1; FactsFor uses keyword scoring.
	Index json.RawMessage `json:"index,omitempty"`
}

// Base is what the engine queries.
type Base struct{ B Bundle }

// ---------- loading ----------

// Load reads a content directory or a compiled bundle (.json).
func Load(path string) (*Base, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		b, err := LoadDir(path)
		if err != nil {
			return nil, err
		}
		return &Base{B: *b}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var b Bundle
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("bundle %s: %w", path, err)
	}
	if errs := Validate(&b); len(errs) > 0 {
		return nil, fmt.Errorf("bundle %s: %d validation errors, first: %v", path, len(errs), errs[0])
	}
	return &Base{B: b}, nil
}

// LoadDir reads a content directory:
//
//	manifest.json
//	<segment>/<kind>.jsonl            e.g. common/facts.jsonl
//	<segment>/<kind>.<topic>.jsonl    e.g. common/facts.gst.jsonl (research agent output)
//
// where <kind> is facts, risks, friction, archetypes or scenes, and
// <segment> is b2b_saas, b2c, deep_tech, or "common" (all segments).
// A record's own "segment" field, if set, wins over the directory.
func LoadDir(dir string) (*Bundle, error) {
	b := &Bundle{}
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("content: %w (a content directory needs manifest.json)", err)
	}
	if err := json.Unmarshal(raw, &b.Manifest); err != nil {
		return nil, fmt.Errorf("manifest.json: %w", err)
	}
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) != 2 {
			return fmt.Errorf("%s: content files live one directory deep (<segment>/<kind>.jsonl)", rel)
		}
		seg, kind := parts[0], strings.TrimSuffix(parts[1], ".jsonl")
		if i := strings.Index(kind, "."); i >= 0 {
			kind = kind[:i] // facts.gst.jsonl -> facts
		}
		var segs []string
		if seg != "common" {
			segs = []string{seg}
		}
		return readJSONL(path, kind, segs, b)
	})
	if err != nil {
		return nil, err
	}
	b.Hash = hashBundle(b)
	if errs := Validate(b); len(errs) > 0 {
		return nil, fmt.Errorf("content %s: %d validation errors, first: %v", dir, len(errs), errs[0])
	}
	return b, nil
}

func readJSONL(path, kind string, segs []string, b *Bundle) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" || strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "#") {
			continue
		}
		var err error
		switch kind {
		case "facts":
			var r Fact
			if err = json.Unmarshal([]byte(raw), &r); err == nil {
				if len(r.Segment) == 0 {
					r.Segment = segs
				}
				b.Facts = append(b.Facts, r)
			}
		case "risks":
			var r Risk
			if err = json.Unmarshal([]byte(raw), &r); err == nil {
				if len(r.Segment) == 0 {
					r.Segment = segs
				}
				b.Risks = append(b.Risks, r)
			}
		case "friction":
			var r Friction
			if err = json.Unmarshal([]byte(raw), &r); err == nil {
				b.Friction = append(b.Friction, r)
			}
		case "archetypes":
			var r Archetype
			if err = json.Unmarshal([]byte(raw), &r); err == nil {
				b.Archetypes = append(b.Archetypes, r)
			}
		case "scenes":
			var r Scene
			if err = json.Unmarshal([]byte(raw), &r); err == nil {
				if len(r.Segment) == 0 {
					r.Segment = segs
				}
				b.Scenes = append(b.Scenes, r)
			}
		default:
			return fmt.Errorf("%s: unknown record kind %q (want facts, risks, friction, archetypes, scenes)", path, kind)
		}
		if err != nil {
			return fmt.Errorf("%s:%d: %w", path, line, err)
		}
	}
	return sc.Err()
}

func hashBundle(b *Bundle) string {
	h := sha256.New()
	enc := json.NewEncoder(h)
	_ = enc.Encode(b.Manifest)
	_ = enc.Encode(b.Facts)
	_ = enc.Encode(b.Risks)
	_ = enc.Encode(b.Friction)
	_ = enc.Encode(b.Archetypes)
	_ = enc.Encode(b.Scenes)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// Compile finalises a bundle read from a directory (hash, sorted order).
func Compile(b *Bundle) *Bundle {
	sort.Slice(b.Facts, func(i, j int) bool { return b.Facts[i].ID < b.Facts[j].ID })
	sort.Slice(b.Risks, func(i, j int) bool { return b.Risks[i].ID < b.Risks[j].ID })
	sort.Slice(b.Friction, func(i, j int) bool { return b.Friction[i].ID < b.Friction[j].ID })
	sort.Slice(b.Archetypes, func(i, j int) bool { return b.Archetypes[i].ID < b.Archetypes[j].ID })
	sort.Slice(b.Scenes, func(i, j int) bool { return b.Scenes[i].ID < b.Scenes[j].ID })
	b.Hash = hashBundle(b)
	return b
}

// ---------- validation ----------

var validProv = map[string]bool{ProvReal: true, ProvInvented: true, ProvEstimated: true}
var validGates = map[string]bool{"": true, "always": true, "unincorporated": true, "incorporated": true, "has_customers": true, "has_gstin": true, "agency_build": true}
var validSegments = map[string]bool{"b2b_saas": true, "b2c": true, "deep_tech": true}
var validChapters = map[string]bool{"": true, "ideation": true, "building": true, "zero_to_one": true, "compliance": true}

// Validate returns every problem in a bundle. Empty means publishable.
func Validate(b *Bundle) []error {
	var errs []error
	add := func(f string, a ...any) { errs = append(errs, fmt.Errorf(f, a...)) }
	if b.Manifest.Name == "" || b.Manifest.Version == "" || b.Manifest.AsOf == "" {
		add("manifest needs name, version and as_of")
	}
	ids := map[string]bool{}
	dup := func(kind, id string) {
		if id == "" {
			add("%s record with empty id", kind)
			return
		}
		if ids[kind+":"+id] {
			add("duplicate %s id %q", kind, id)
		}
		ids[kind+":"+id] = true
	}
	prov := func(kind, id, p string, srcs []Source) {
		if !validProv[p] {
			add("%s %s: provenance must be real|invented|estimated, got %q", kind, id, p)
		}
		if p == ProvReal && len(srcs) == 0 {
			add("%s %s: claims real but has no sources", kind, id)
		}
	}
	segs := func(kind, id string, ss []string) {
		for _, s := range ss {
			if !validSegments[s] {
				add("%s %s: unknown segment %q", kind, id, s)
			}
		}
	}
	for _, f := range b.Facts {
		dup("fact", f.ID)
		prov("fact", f.ID, f.Provenance, f.Sources)
		segs("fact", f.ID, f.Segment)
		if f.Claim == "" || f.Topic == "" {
			add("fact %s: needs topic and claim", f.ID)
		}
		if !validChapters[f.Chapter] {
			add("fact %s: unknown chapter %q", f.ID, f.Chapter)
		}
		if f.Wall != "" && f.Wall != "real" && f.Wall != "imaginary" {
			add("fact %s: wall must be real|imaginary", f.ID)
		}
	}
	for _, r := range b.Risks {
		dup("risk", r.ID)
		prov("risk", r.ID, r.Provenance, r.Sources)
		segs("risk", r.ID, r.Segment)
		if r.Label == "" || r.BestCase == "" || r.WorstCase == "" {
			add("risk %s: needs label, best_case, worst_case", r.ID)
		}
		if r.TriggerProbPerWeek <= 0 || r.TriggerProbPerWeek > 1 {
			add("risk %s: trigger_prob_per_week must be in (0,1]", r.ID)
		}
		if !validGates[r.AppliesWhen] {
			add("risk %s: unknown applies_when %q", r.ID, r.AppliesWhen)
		}
	}
	for _, f := range b.Friction {
		dup("friction", f.ID)
		prov("friction", f.ID, f.Provenance, f.Sources)
		if f.Process == "" || f.WhatHappens == "" {
			add("friction %s: needs process and what_happens", f.ID)
		}
		if f.DelayDays.Max < f.DelayDays.Min || f.ExtraCostINR.Max < f.ExtraCostINR.Min {
			add("friction %s: ranges must have min <= max", f.ID)
		}
	}
	for _, a := range b.Archetypes {
		dup("archetype", a.ID)
		if a.Relation == "" || a.Sketch == "" {
			add("archetype %s: needs relation and sketch", a.ID)
		}
	}
	sceneIDs := map[string]bool{}
	for _, s := range b.Scenes {
		dup("scene", s.ID)
		sceneIDs[s.ID] = true
		segs("scene", s.ID, s.Segment)
		if s.Situation == "" || s.Title == "" || !validChapters[s.Chapter] || s.Chapter == "" {
			add("scene %s: needs title, situation and a valid chapter", s.ID)
		}
	}
	for _, f := range b.Facts {
		for _, u := range f.UsedBy {
			if !sceneIDs[u] {
				add("fact %s: used_by references unknown scene %q", f.ID, u)
			}
		}
	}
	return errs
}

// Merge adds another bundle's records (e.g. a plugin's content pack).
func (b *Base) Merge(o Bundle) {
	b.B.Facts = append(b.B.Facts, o.Facts...)
	b.B.Risks = append(b.B.Risks, o.Risks...)
	b.B.Friction = append(b.B.Friction, o.Friction...)
	b.B.Archetypes = append(b.B.Archetypes, o.Archetypes...)
	b.B.Scenes = append(b.B.Scenes, o.Scenes...)
}

// ---------- queries ----------

func inSegment(segs []string, seg engine.Segment) bool {
	if len(segs) == 0 {
		return true
	}
	for _, s := range segs {
		if s == string(seg) {
			return true
		}
	}
	return false
}

// RisksFor instantiates the hidden checklist for a segment.
func (b *Base) RisksFor(seg engine.Segment) []engine.RiskItem {
	var out []engine.RiskItem
	for _, r := range b.B.Risks {
		if !inSegment(r.Segment, seg) {
			continue
		}
		out = append(out, engine.RiskItem{
			ID: r.ID, Label: r.Label, BestCase: r.BestCase, WorstCase: r.WorstCase,
			TriggerProbPerWeek: r.TriggerProbPerWeek, Imaginary: r.Imaginary, AppliesWhen: r.AppliesWhen,
		})
	}
	return out
}

// FactsFor ranks facts for a segment/chapter/keywords. Keyword scoring in
// v1; a bundle Index (embeddings) can replace this without changing callers.
func (b *Base) FactsFor(seg engine.Segment, chapter string, tags []string, max int) []Fact {
	var out []Fact
	for _, f := range b.B.Facts {
		if inSegment(f.Segment, seg) {
			out = append(out, f)
		}
	}
	score := func(f Fact) int {
		s := 0
		if len(f.Segment) > 0 {
			s += 2 // segment-specific beats generic
		}
		if f.Chapter == chapter {
			s += 2
		}
		for _, t := range tags {
			if strings.EqualFold(t, f.Topic) {
				s += 3
			}
			for _, ft := range f.Tags {
				if strings.EqualFold(t, ft) {
					s++
				}
			}
		}
		return s
	}
	sort.SliceStable(out, func(i, j int) bool { return score(out[i]) > score(out[j]) })
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

// ScenesFor returns eligible scenario cards for a chapter.
func (b *Base) ScenesFor(seg engine.Segment, chapter string, max int) []Scene {
	var out []Scene
	for _, s := range b.B.Scenes {
		if s.Chapter == chapter && inSegment(s.Segment, seg) {
			out = append(out, s)
		}
	}
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

// FrictionFor returns event templates for a government/agency process.
func (b *Base) FrictionFor(process string) []Friction {
	var out []Friction
	for _, f := range b.B.Friction {
		if strings.EqualFold(f.Process, process) {
			out = append(out, f)
		}
	}
	return out
}

// Archetypes returns all NPC archetypes.
func (b *Base) Archetypes() []Archetype { return b.B.Archetypes }

// ErrEmpty is returned by callers that need at least some content.
var ErrEmpty = errors.New("knowledge base is empty")
