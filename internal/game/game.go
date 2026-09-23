// Package game orchestrates one player action end to end: catch the clock
// up, ask the narrator, validate and apply, fan out to plugins, persist.
// The web layer calls this; nothing here knows about HTTP.
package game

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ramank775/founder-sim/engine"
	"github.com/ramank775/founder-sim/internal/llm"
	"github.com/ramank775/founder-sim/internal/store"
	"github.com/ramank775/founder-sim/kb"
	"github.com/ramank775/founder-sim/plugin"
)

type Service struct {
	Store   store.Store
	KB      *kb.Base
	Plugins *plugin.Registry
	// NewClient builds a model client for a player. Lets main swap in the
	// fake for development.
	NewClient func(cfg llm.Config) llm.Client
	Now       func() int64 // unix ms; injectable for tests
}

func nowMs() int64 { return time.Now().UnixMilli() }

func (s *Service) now() int64 {
	if s.Now != nil {
		return s.Now()
	}
	return nowMs()
}

func randomSeed() int64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return int64(binary.LittleEndian.Uint64(b[:]) >> 1)
}

// StartInput is the session-start questionnaire.
type StartInput struct {
	JobDescription string
	Idea           string
	Age            int
	MaritalStatus  string
	Kids           string
	Savings        int64
	Obligations    []engine.Obligation
	Technical      bool
	TimeScaleMs    int64    // 0 = default (1 day = 1 hour)
	Plugins        []string // enabled plugin names, opt-in
}

// Start creates a run: classify the idea, generate the hidden checklist,
// pre-tick what this player learned in earlier runs.
func (s *Service) Start(ctx context.Context, p *store.Player, cfg llm.Config, in StartInput) (*store.RunRecord, error) {
	if strings.TrimSpace(in.Idea) == "" {
		return nil, errors.New("describe the idea first")
	}
	n := &llm.Narrator{C: s.NewClient(cfg), KB: s.KB}
	seg, err := n.Classify(ctx, in.Idea, in.JobDescription)
	if err != nil && seg == "" {
		return nil, fmt.Errorf("could not classify the idea: %w", err)
	}
	player := engine.Player{
		JobDescription: in.JobDescription, Idea: in.Idea, Age: in.Age, MaritalStatus: in.MaritalStatus,
		Kids: in.Kids, Savings: in.Savings, MonthlyObligations: in.Obligations, Technical: in.Technical, Segment: seg,
	}
	run := engine.NewRun(store.NewRunID(), randomSeed(), s.now(), player)
	if in.TimeScaleMs > 0 {
		run.TimeScaleMs = in.TimeScaleMs
	}
	run.Risks = s.KB.RisksFor(seg)
	// Cross-run learning, faked: items met before start discovered (the
	// player knows they exist), not ticked (they still have to do them).
	for i := range run.Risks {
		for _, l := range p.Learned {
			if l == run.Risks[i].ID {
				run.Risks[i].Discovered = true
			}
		}
	}
	for _, name := range in.Plugins {
		if _, _, ok := s.Plugins.Get(name); ok {
			run.Plugins = append(run.Plugins, name)
		}
	}
	run.Log = append(run.Log, engine.LogEntry{Day: 0, Kind: "system",
		Text: "Day 0. You still have a job. Nobody knows about the idea yet except you. What do you do?"})

	rec := &store.RunRecord{PlayerID: p.ID, Run: run, PluginState: map[string]map[string][]byte{}}
	s.fanOutHook(ctx, rec, plugin.HookRunStarted, nil)
	if err := s.Store.PutRun(ctx, rec); err != nil {
		return nil, err
	}
	return rec, nil
}

// TurnResult is what the web layer renders after an action.
type TurnResult struct {
	Narration string
	Result    engine.Result
	Reports   []engine.DayReport
	Raw       string // raw model output, for the dev panel
}

// Turn runs one free-text action.
func (s *Service) Turn(ctx context.Context, p *store.Player, cfg llm.Config, rec *store.RunRecord, action string) (*TurnResult, error) {
	run := rec.Run
	if run.Status != "active" {
		return nil, errors.New("this run has ended")
	}
	action = strings.TrimSpace(action)
	if action == "" {
		return nil, errors.New("say what you do")
	}
	reports := s.catchUp(ctx, rec)
	if run.SlotsLeft <= 0 {
		_ = s.Store.PutRun(ctx, rec)
		return nil, fmt.Errorf("the day is over; the next one starts in %s", s.untilNextDay(run))
	}

	// What the founder missed while away, for the narrator.
	var since []engine.LogEntry
	for _, rep := range reports {
		for _, ev := range rep.Fired {
			since = append(since, engine.LogEntry{Day: rep.Day, Text: ev.Description})
		}
		for _, ri := range rep.Risks {
			since = append(since, engine.LogEntry{Day: rep.Day, Text: ri.Label + " surfaced"})
		}
	}

	extra := s.pluginContext(ctx, rec)
	n := &llm.Narrator{C: s.NewClient(cfg), KB: s.KB}
	prop, raw, err := n.Turn(ctx, llm.TurnInput{Run: run, Action: action, SinceLast: since, Extra: extra, Chapter: chapterFor(run)})
	if err != nil {
		return nil, err
	}
	res := run.Apply(prop, engine.Source{Kind: "llm"})
	if len(res.Rejected) > 0 {
		log.Printf("run %s: rejected from model: %+v", run.ID, res.Rejected)
	}
	s.learn(ctx, p, run)

	payload, _ := json.Marshal(plugin.TurnPayload{Action: action, Narration: res.Narration, SlotCost: res.SlotCost})
	s.fanOutHook(ctx, rec, plugin.HookTurnResolved, payload)

	if err := s.Store.PutRun(ctx, rec); err != nil {
		return nil, err
	}
	return &TurnResult{Narration: res.Narration, Result: res, Reports: reports, Raw: raw}, nil
}

// Refresh catches the clock up without taking an action (page load).
func (s *Service) Refresh(ctx context.Context, rec *store.RunRecord) ([]engine.DayReport, error) {
	reports := s.catchUp(ctx, rec)
	if len(reports) > 0 {
		if err := s.Store.PutRun(ctx, rec); err != nil {
			return nil, err
		}
	}
	return reports, nil
}

func (s *Service) catchUp(ctx context.Context, rec *store.RunRecord) []engine.DayReport {
	reports := rec.Run.CatchUp(s.now())
	for _, rep := range reports {
		payload, _ := json.Marshal(plugin.DayPayload{Day: rep.Day})
		s.fanOutHook(ctx, rec, plugin.HookDayAdvanced, payload)
		for _, ev := range rep.Fired {
			pl, _ := json.Marshal(plugin.EventPayload{Source: ev.Source, Description: ev.Description})
			s.fanOutHook(ctx, rec, plugin.HookEventFired, pl)
		}
	}
	return reports
}

func (s *Service) untilNextDay(run *engine.Run) time.Duration {
	anchorAt := run.AnchorAtMs
	if anchorAt == 0 {
		anchorAt = run.StartedAt
	}
	nextAt := anchorAt + int64(run.CurrentDay-run.AnchorDay+1)*run.TimeScaleMs
	d := time.Duration(nextAt-s.now()) * time.Millisecond
	if d < 0 {
		d = 0
	}
	return d.Round(time.Minute)
}

// learn records discovered checklist items on the player profile.
func (s *Service) learn(ctx context.Context, p *store.Player, run *engine.Run) {
	changed := false
	for _, ri := range run.Risks {
		if !ri.Discovered {
			continue
		}
		known := false
		for _, l := range p.Learned {
			if l == ri.ID {
				known = true
			}
		}
		if !known {
			p.Learned = append(p.Learned, ri.ID)
			changed = true
		}
	}
	if changed {
		if err := s.Store.PutPlayer(ctx, p); err != nil {
			log.Printf("learn: %v", err)
		}
	}
}

func chapterFor(run *engine.Run) string {
	switch {
	case run.World.Users > 0:
		return "zero_to_one"
	case run.World.Build.Phase > 0:
		return "building"
	default:
		return "ideation"
	}
}

// ---------- plugins ----------

func (s *Service) pluginState(rec *store.RunRecord, name string) json.RawMessage {
	if m, ok := rec.PluginState[name]; ok {
		return m[""]
	}
	return nil
}

func (s *Service) setPluginState(rec *store.RunRecord, name string, st json.RawMessage) {
	if st == nil {
		return
	}
	if len(st) > plugin.MaxStateBytes {
		log.Printf("plugin %s: state too large (%d bytes), dropped", name, len(st))
		return
	}
	if rec.PluginState == nil {
		rec.PluginState = map[string]map[string][]byte{}
	}
	if rec.PluginState[name] == nil {
		rec.PluginState[name] = map[string][]byte{}
	}
	rec.PluginState[name][""] = st
}

// fanOutHook sends a hook to every plugin enabled for the run that
// subscribed to it, and applies whatever they propose under plugin rights.
func (s *Service) fanOutHook(ctx context.Context, rec *store.RunRecord, kind string, payload json.RawMessage) {
	view := rec.Run.View()
	for _, name := range rec.Run.Plugins {
		p, m, ok := s.Plugins.Get(name)
		if !ok || !m.Has(plugin.CapHooks) || !contains(m.Hooks, kind) {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, plugin.CallTimeout)
		res, err := p.OnHook(cctx, plugin.HookRequest{Kind: kind, View: view, Payload: payload, State: s.pluginState(rec, name)})
		cancel()
		if err != nil {
			log.Printf("plugin %s hook %s: %v", name, kind, err)
			continue
		}
		s.setPluginState(rec, name, res.State)
		if res.Proposal != nil {
			r := rec.Run.Apply(*res.Proposal, engine.Source{Kind: "plugin", Name: name})
			if len(r.Rejected) > 0 {
				log.Printf("plugin %s hook %s: rejected %+v", name, kind, r.Rejected)
			}
		}
	}
}

// pluginContext pre-runs argument-less LLM tools so the narrator sees
// their output. v1 shortcut: real tool-calling mid-turn comes later.
func (s *Service) pluginContext(ctx context.Context, rec *store.RunRecord) []string {
	var out []string
	view := rec.Run.View()
	for _, name := range rec.Run.Plugins {
		p, m, ok := s.Plugins.Get(name)
		if !ok || !m.Has(plugin.CapLLMTools) {
			continue
		}
		tools, err := p.LLMTools(ctx)
		if err != nil {
			continue
		}
		for _, t := range tools {
			if !argless(t.Schema) {
				continue
			}
			cctx, cancel := context.WithTimeout(ctx, plugin.CallTimeout)
			res, err := p.CallLLMTool(cctx, plugin.LLMToolRequest{Tool: t.Name, Args: json.RawMessage(`{}`), View: view, State: s.pluginState(rec, name)})
			cancel()
			if err != nil {
				continue
			}
			s.setPluginState(rec, name, res.State)
			out = append(out, fmt.Sprintf("PLUGIN %s/%s (%s):\n%s", name, t.Name, t.Description, res.Result))
		}
	}
	return out
}

func argless(schema json.RawMessage) bool {
	var s struct {
		Required   []string       `json:"required"`
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(schema, &s); err != nil {
		return false
	}
	return len(s.Required) == 0
}

// InvokeTool is the player using a plugin's tool.
func (s *Service) InvokeTool(ctx context.Context, rec *store.RunRecord, name, tool, input string) (*plugin.InvokeResponse, error) {
	if !contains(rec.Run.Plugins, name) {
		return nil, errors.New("plugin not enabled for this run")
	}
	p, m, ok := s.Plugins.Get(name)
	if !ok || !m.Has(plugin.CapTools) {
		return nil, errors.New("no such plugin tool")
	}
	cctx, cancel := context.WithTimeout(ctx, plugin.CallTimeout)
	defer cancel()
	res, err := p.Invoke(cctx, plugin.InvokeRequest{Tool: tool, Input: input, View: rec.Run.View(), State: s.pluginState(rec, name)})
	if err != nil {
		return nil, err
	}
	s.setPluginState(rec, name, res.State)
	if res.Proposal != nil {
		r := rec.Run.Apply(*res.Proposal, engine.Source{Kind: "plugin", Name: name})
		if len(r.Rejected) > 0 {
			log.Printf("plugin %s tool %s: rejected %+v", name, tool, r.Rejected)
		}
	}
	if err := s.Store.PutRun(ctx, rec); err != nil {
		return nil, err
	}
	return &res, nil
}

// Exit ends the run on the founder's word.
func (s *Service) Exit(ctx context.Context, rec *store.RunRecord, kind, note string) error {
	if err := rec.Run.PlayerExit(kind, note); err != nil {
		return err
	}
	s.fanOutHook(ctx, rec, plugin.HookRunExited, nil)
	return s.Store.PutRun(ctx, rec)
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
