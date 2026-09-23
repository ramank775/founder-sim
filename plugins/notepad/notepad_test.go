package notepad

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ramank775/founder-sim/engine"
	"github.com/ramank775/founder-sim/plugin"
	"github.com/ramank775/founder-sim/plugin/conformance"
	"github.com/ramank775/founder-sim/plugin/httpadapter"
)

// The same suite, both transports. If these diverge, the adapter is wrong.
func TestConformanceInProcess(t *testing.T) { conformance.Run(t, New()) }

func TestConformanceOverHTTP(t *testing.T) {
	srv := httptest.NewServer(httpadapter.NewServer(New()))
	defer srv.Close()
	conformance.Run(t, httpadapter.NewClient(srv.URL))
}

func TestNotepadBehaviour(t *testing.T) {
	for name, p := range map[string]plugin.Plugin{
		"inproc": New(),
	} {
		t.Run(name, func(t *testing.T) { exercise(t, p) })
	}
	srv := httptest.NewServer(httpadapter.NewServer(New()))
	defer srv.Close()
	t.Run("http", func(t *testing.T) { exercise(t, httpadapter.NewClient(srv.URL)) })
}

func exercise(t *testing.T, p plugin.Plugin) {
	ctx := context.Background()
	run := engine.NewRun("r", 3, 0, engine.Player{Savings: 100})
	view := run.View()

	res, err := p.Invoke(ctx, plugin.InvokeRequest{Tool: "notepad", Input: "call the CA about GST", View: view})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.HTML, "call the CA about GST") {
		t.Fatalf("note not rendered: %s", res.HTML)
	}
	state := res.State

	res, err = p.Invoke(ctx, plugin.InvokeRequest{Tool: "notepad", Input: "remind 3: follow up with agency", View: view, State: state})
	if err != nil {
		t.Fatal(err)
	}
	if res.Proposal == nil || len(res.Proposal.Pending) != 1 || res.Proposal.Pending[0].InDays != 3 {
		t.Fatalf("reminder did not produce a pending proposal: %+v", res.Proposal)
	}
	state = res.State
	// Engine accepts it from a plugin source.
	ar := run.Apply(*res.Proposal, engine.Source{Kind: "plugin", Name: "notepad"})
	if len(ar.Rejected) != 0 || len(run.Pending) != 1 {
		t.Fatalf("engine rejected reminder: %+v", ar.Rejected)
	}

	res, err = p.Invoke(ctx, plugin.InvokeRequest{Tool: "notepad", Input: "done 1", View: view, State: state})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.HTML, "call the CA") {
		t.Fatal("done note still shown")
	}
	state = res.State

	// LLM tool sees only open notes.
	lr, err := p.CallLLMTool(ctx, plugin.LLMToolRequest{Tool: "notepad_read", Args: []byte(`{}`), View: view, State: state})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(lr.Result, "call the CA") || !strings.Contains(lr.Result, "follow up with agency") {
		t.Fatalf("llm tool result wrong: %q", lr.Result)
	}

	// Day hook reports open-note count as a namespaced metric.
	hr, err := p.OnHook(ctx, plugin.HookRequest{Kind: plugin.HookDayAdvanced, View: view, State: state})
	if err != nil {
		t.Fatal(err)
	}
	ar = run.Apply(*hr.Proposal, engine.Source{Kind: "plugin", Name: "notepad"})
	if len(ar.Rejected) != 0 || run.World.Metrics["notepad.open_notes"] != 1 {
		t.Fatalf("metric not applied: %+v %v", ar.Rejected, run.World.Metrics)
	}
}
