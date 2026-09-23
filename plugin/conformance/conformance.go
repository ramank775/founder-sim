// Package conformance is the test suite every plugin must pass, whether it
// is built in or remote. Plugin authors call Run from their own tests;
// the engine runs it against each built-in twice: directly and through
// httpadapter, to prove the two transports are indistinguishable.
package conformance

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ramank775/founder-sim/engine"
	"github.com/ramank775/founder-sim/kb"
	"github.com/ramank775/founder-sim/plugin"
)

// Run executes the suite against p.
func Run(t *testing.T, p plugin.Plugin) {
	t.Helper()
	ctx := context.Background()

	m, err := p.Manifest(ctx)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	if err := plugin.ValidateManifest(m); err != nil {
		t.Fatalf("manifest invalid: %v", err)
	}

	view := engine.NewRun("conf", 7, 0, engine.Player{Idea: "test idea"}).View()

	t.Run("tools", func(t *testing.T) {
		tools, err := p.Tools(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if m.Has(plugin.CapTools) && len(tools) == 0 {
			t.Fatal("declares tools capability but lists none")
		}
		if !m.Has(plugin.CapTools) && len(tools) > 0 {
			t.Fatal("lists tools without declaring the capability")
		}
		seen := map[string]bool{}
		for _, tl := range tools {
			if tl.Name == "" || tl.Title == "" {
				t.Fatalf("tool missing name/title: %+v", tl)
			}
			if seen[tl.Name] {
				t.Fatalf("duplicate tool %q", tl.Name)
			}
			seen[tl.Name] = true
		}
		// Unknown tool must return ErrNoSuchTool, on both transports.
		_, err = p.Invoke(ctx, plugin.InvokeRequest{Tool: "__definitely_not_a_tool__", View: view})
		if !errors.Is(err, plugin.ErrNoSuchTool) {
			t.Fatalf("unknown tool: want ErrNoSuchTool, got %v", err)
		}
		// Every declared tool must accept an empty invocation and respect
		// the state contract (state round-trips as opaque JSON).
		for _, tl := range tools {
			res, err := p.Invoke(ctx, plugin.InvokeRequest{Tool: tl.Name, View: view})
			if err != nil {
				t.Fatalf("Invoke(%s, empty): %v", tl.Name, err)
			}
			if len(res.State) > plugin.MaxStateBytes {
				t.Fatalf("tool %s returned %d bytes of state (max %d)", tl.Name, len(res.State), plugin.MaxStateBytes)
			}
			if len(res.State) > 0 && !json.Valid(res.State) {
				t.Fatalf("tool %s returned invalid JSON state", tl.Name)
			}
			// Feed the state back; must not error.
			if _, err := p.Invoke(ctx, plugin.InvokeRequest{Tool: tl.Name, View: view, State: res.State}); err != nil {
				t.Fatalf("Invoke(%s) with returned state: %v", tl.Name, err)
			}
			if res.Proposal != nil {
				assertPluginProposalIsLegal(t, *res.Proposal, m.Name)
			}
		}
	})

	t.Run("hooks", func(t *testing.T) {
		kinds := []string{plugin.HookRunStarted, plugin.HookDayAdvanced, plugin.HookTurnResolved, plugin.HookEventFired, plugin.HookRunExited}
		for _, k := range kinds {
			res, err := p.OnHook(ctx, plugin.HookRequest{Kind: k, View: view})
			if err != nil {
				t.Fatalf("OnHook(%s): %v", k, err)
			}
			if !m.Has(plugin.CapHooks) && res.Proposal != nil {
				t.Fatalf("proposes on %s without hooks capability", k)
			}
			if res.Proposal != nil {
				assertPluginProposalIsLegal(t, *res.Proposal, m.Name)
			}
		}
	})

	t.Run("content", func(t *testing.T) {
		pack, err := p.Content(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if m.Has(plugin.CapContent) {
			if pack.Manifest.Name == "" {
				t.Fatal("declares content but pack has no manifest name")
			}
			if errs := kb.Validate(&pack); len(errs) > 0 {
				t.Fatalf("content pack invalid: %v", errs[0])
			}
		}
	})

	t.Run("llm_tools", func(t *testing.T) {
		tools, err := p.LLMTools(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if m.Has(plugin.CapLLMTools) && len(tools) == 0 {
			t.Fatal("declares llm_tools but lists none")
		}
		for _, tl := range tools {
			if tl.Name == "" || tl.Description == "" || !json.Valid(tl.Schema) {
				t.Fatalf("llm tool malformed: %+v", tl)
			}
			res, err := p.CallLLMTool(ctx, plugin.LLMToolRequest{Tool: tl.Name, Args: json.RawMessage(`{}`), View: view})
			if err != nil {
				t.Fatalf("CallLLMTool(%s): %v", tl.Name, err)
			}
			if len(res.Result) > 8000 {
				t.Fatalf("llm tool %s result too long (%d chars)", tl.Name, len(res.Result))
			}
		}
		_, err = p.CallLLMTool(ctx, plugin.LLMToolRequest{Tool: "__nope__", Args: json.RawMessage(`{}`), View: view})
		if !errors.Is(err, plugin.ErrNoSuchTool) {
			t.Fatalf("unknown llm tool: want ErrNoSuchTool, got %v", err)
		}
	})
}

// assertPluginProposalIsLegal applies the proposal to a scratch run and
// fails if anything a plugin is not allowed to do was attempted. This is
// the contract in code: plugins may add, never mutate core state.
func assertPluginProposalIsLegal(t *testing.T, p engine.Proposal, name string) {
	t.Helper()
	r := engine.NewRun("scratch", 1, 0, engine.Player{Savings: 1000})
	res := r.Apply(p, engine.Source{Kind: "plugin", Name: name})
	if len(res.Rejected) > 0 {
		t.Fatalf("plugin %s proposed something it may not do: %+v", name, res.Rejected)
	}
}
