// Package plugin defines the one contract for extending Founder Sim.
//
// A plugin can be compiled into the binary (implements Plugin directly) or
// run anywhere as an HTTP service (the httpadapter package wraps this same
// interface on both ends). The engine cannot tell the difference, and the
// conformance package proves it.
//
// Design rules that every method obeys:
//
//   - Every call is request/response by value. No pointers into engine
//     state, no callbacks, no shared memory. Assume a network in between.
//   - A plugin sees the PlayerView only. The hidden checklist, unresolved
//     rolls and missed things never cross this boundary.
//   - A plugin never changes core state. It returns an engine.Proposal and
//     the engine applies only what a plugin is allowed to do (todos, pending
//     events, NPCs, namespaced metrics). See engine.Apply.
//   - A plugin's per-run memory travels with each request as an opaque
//     State blob and comes back in the response. The engine stores it. This
//     is what makes remote plugins stateless and restarts painless.
package plugin

import (
	"context"
	"encoding/json"

	"github.com/ramank775/founder-sim/engine"
	"github.com/ramank775/founder-sim/kb"
)

// ProtocolVersion is bumped on any incompatible change to the wire types.
// A plugin declares the version it speaks in its Manifest; the engine
// refuses mismatches loudly rather than misreading silently.
const ProtocolVersion = "1"

// MaxStateBytes caps the per-run state blob a plugin may keep.
const MaxStateBytes = 64 * 1024

// Capabilities a plugin may declare. The engine only calls the methods a
// plugin has declared; undeclared ones are never invoked.
const (
	CapTools    = "tools"     // player-facing tools (notepad, alarm...)
	CapHooks    = "hooks"     // engine events
	CapContent  = "content"   // knowledge base packs
	CapLLMTools = "llm_tools" // tools the narrator model may call
)

// Manifest describes a plugin.
type Manifest struct {
	Name            string   `json:"name"` // lowercase, [a-z0-9_-], unique; also the metric namespace
	Version         string   `json:"version"`
	ProtocolVersion string   `json:"protocol_version"`
	Description     string   `json:"description"`
	Capabilities    []string `json:"capabilities"`
	// Hooks lists the event kinds the plugin wants, when CapHooks is set.
	Hooks []string `json:"hooks,omitempty"`
}

func (m Manifest) Has(cap string) bool {
	for _, c := range m.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// Hook event kinds.
const (
	HookRunStarted   = "run_started"
	HookDayAdvanced  = "day_advanced"  // once per resolved day, payload: DayPayload
	HookTurnResolved = "turn_resolved" // after a player action was applied, payload: TurnPayload
	HookEventFired   = "event_fired"   // a pending event or risk surfaced, payload: EventPayload
	HookRunExited    = "run_exited"
)

// HookRequest is sent for every subscribed event.
type HookRequest struct {
	Kind    string            `json:"kind"`
	View    engine.PlayerView `json:"view"`
	Payload json.RawMessage   `json:"payload,omitempty"`
	State   json.RawMessage   `json:"state,omitempty"` // plugin's own per-run state, opaque to the engine
}

type DayPayload struct {
	Day int `json:"day"`
}

type TurnPayload struct {
	Action    string `json:"action"`    // what the player typed
	Narration string `json:"narration"` // what they were told
	SlotCost  int    `json:"slot_cost"`
}

type EventPayload struct {
	Source      string `json:"source"`
	Description string `json:"description"`
}

// HookResponse may propose additions and update the plugin's state.
type HookResponse struct {
	Proposal *engine.Proposal `json:"proposal,omitempty"`
	State    json.RawMessage  `json:"state,omitempty"` // nil = unchanged
}

// ToolSpec is a player-facing tool. The engine renders a button/command for
// it; invoking it calls Invoke. Input is free text from the player in v1.
type ToolSpec struct {
	Name        string `json:"name"` // unique within the plugin
	Title       string `json:"title"`
	Description string `json:"description"`
	// Panel means the tool has a persistent panel rendered from the last
	// InvokeResponse.HTML; otherwise it is a one-shot command.
	Panel bool `json:"panel"`
}

// InvokeRequest is a player using a tool.
type InvokeRequest struct {
	Tool  string            `json:"tool"`
	Input string            `json:"input"` // free text; structured inputs can come later
	Args  map[string]string `json:"args,omitempty"`
	View  engine.PlayerView `json:"view"`
	State json.RawMessage   `json:"state,omitempty"`
}

// InvokeResponse is what the player sees, plus optional proposals.
type InvokeResponse struct {
	// Text is shown to the player. HTML, if set, is rendered instead
	// (sanitised by the engine: a small allowlist of tags).
	Text     string           `json:"text,omitempty"`
	HTML     string           `json:"html,omitempty"`
	Proposal *engine.Proposal `json:"proposal,omitempty"`
	State    json.RawMessage  `json:"state,omitempty"`
}

// LLMToolSpec is a tool the narrator model may call mid-turn. The schema is
// a JSON Schema object for the arguments, as in OpenAI-style tool calling.
type LLMToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
}

type LLMToolRequest struct {
	Tool  string            `json:"tool"`
	Args  json.RawMessage   `json:"args"`
	View  engine.PlayerView `json:"view"`
	State json.RawMessage   `json:"state,omitempty"`
}

type LLMToolResponse struct {
	// Result is handed back to the model verbatim. Keep it short.
	Result string          `json:"result"`
	State  json.RawMessage `json:"state,omitempty"`
}

// Plugin is the whole contract. Implement it directly to be built in; run it
// behind httpadapter.Server to be remote. Embed Base to get no-op defaults.
type Plugin interface {
	Manifest(ctx context.Context) (Manifest, error)

	// CapHooks
	OnHook(ctx context.Context, req HookRequest) (HookResponse, error)

	// CapTools
	Tools(ctx context.Context) ([]ToolSpec, error)
	Invoke(ctx context.Context, req InvokeRequest) (InvokeResponse, error)

	// CapContent: a content pack in the same shape as a compiled bundle
	// (manifest + records). Merged into the knowledge base at startup.
	Content(ctx context.Context) (kb.Bundle, error)

	// CapLLMTools
	LLMTools(ctx context.Context) ([]LLMToolSpec, error)
	CallLLMTool(ctx context.Context, req LLMToolRequest) (LLMToolResponse, error)
}

// Base provides no-op implementations so a plugin only writes what it uses.
type Base struct{}

func (Base) OnHook(context.Context, HookRequest) (HookResponse, error) { return HookResponse{}, nil }
func (Base) Tools(context.Context) ([]ToolSpec, error)                 { return nil, nil }
func (Base) Invoke(context.Context, InvokeRequest) (InvokeResponse, error) {
	return InvokeResponse{}, ErrNoSuchTool
}
func (Base) Content(context.Context) (kb.Bundle, error)      { return kb.Bundle{}, nil }
func (Base) LLMTools(context.Context) ([]LLMToolSpec, error) { return nil, nil }
func (Base) CallLLMTool(context.Context, LLMToolRequest) (LLMToolResponse, error) {
	return LLMToolResponse{}, ErrNoSuchTool
}
