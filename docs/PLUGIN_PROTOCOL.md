# Plugin protocol v1

One contract, two transports. `plugin.Plugin` is the Go interface; this document is the same interface over HTTP+JSON so a plugin can be written in any language. Wire types are the JSON encoding of the Go types in `plugin/plugin.go` and `engine/delta.go`; those files are the source of truth and this page is derived from them.

## Licence

Programs that interact with Founder Sim only through this protocol are not derivative works of it and may be under any licence; see the additional permission in [LICENSING.md](../LICENSING.md). Plugins compiled into the binary are AGPL like the rest of the code.

## Principles

1. **Request/response by value.** No callbacks, no shared memory. Assume a network in between, even for built-ins.
2. **Player's view only.** A plugin receives `engine.PlayerView`, never the hidden checklist, unresolved pending events or missed things. A plugin cannot become a cheat overlay.
3. **Propose, never mutate.** A plugin returns an `engine.Proposal`. The engine applies only what plugins may do: `todos`, `pending` events, `npcs`, and `metric_set` deltas whose key is namespaced `<plugin-name>.<metric>`. Everything else — money, users, time slots, entity, cap table, build, risk ticks, exit, narration — is rejected and logged.
4. **State travels with the request.** The engine stores an opaque JSON blob per (run, plugin), ≤ 64 KiB, and sends it on every call as `state`. Return a new `state` to replace it; omit it to keep it. Remote plugins are therefore stateless and restart-safe.
5. **Every call has a 10-second timeout.** A slow plugin is skipped for that call, not retried.
6. **Protocol version is declared and checked.** Manifest `protocol_version` must equal `"1"`; mismatches are refused at registration.

## Routes

| Method | Path | Body | Response |
|---|---|---|---|
| GET | `/v1/manifest` | — | `Manifest` |
| POST | `/v1/hook` | `HookRequest` | `HookResponse` |
| GET | `/v1/tools` | — | `[]ToolSpec` |
| POST | `/v1/invoke` | `InvokeRequest` | `InvokeResponse` |
| GET | `/v1/content` | — | `kb.Bundle` |
| GET | `/v1/llm-tools` | — | `[]LLMToolSpec` |
| POST | `/v1/llm-tool` | `LLMToolRequest` | `LLMToolResponse` |

Errors: JSON `{"error": "..."}` with a 4xx/5xx. An unknown tool name is `404` with an error containing `no such tool`. Bodies are capped at 1 MiB.

The engine only calls routes for capabilities the manifest declares.

## Manifest

```json
{
  "name": "notepad",
  "version": "0.1.0",
  "protocol_version": "1",
  "description": "A notepad with reminders.",
  "capabilities": ["tools", "hooks", "llm_tools"],
  "hooks": ["day_advanced"]
}
```

`name` is `[a-z0-9][a-z0-9_-]{0,31}`, unique across registered plugins, and is the namespace for metrics.

## Hooks (`capabilities: ["hooks"]`)

Kinds: `run_started`, `day_advanced` (payload `{"day": n}`), `turn_resolved` (payload `{"action", "narration", "slot_cost"}`), `event_fired` (payload `{"source", "description"}`), `run_exited`.

```json
// request
{"kind": "day_advanced", "view": PlayerView, "payload": {"day": 4}, "state": {...}}
// response
{"proposal": Proposal, "state": {...}}
```

An alarm plugin is a `turn_resolved`/`invoke` that proposes a `pending` event `{"in_days": 3, "description": "Alarm: ...", "probability": 1}`; the engine fires it when day comes, not before.

## Player-facing tools (`capabilities: ["tools"]`)

`ToolSpec`: `{"name", "title", "description", "panel": bool}`. A `panel` tool keeps a persistent panel rendered from the last `html`.

```json
// request
{"tool": "notepad", "input": "remind 3: chase the agency", "args": {}, "view": PlayerView, "state": {...}}
// response
{"text": "Reminder set for day 7.", "html": "<div class=\"notepad\">...</div>", "proposal": Proposal, "state": {...}}
```

`html` is sanitised to a small tag allowlist (`b i em strong p ul ol li br span div code pre h3 h4 small hr`, `class` attribute only).

## Content packs (`capabilities: ["content"]`)

Return a `kb.Bundle` (the same shape `founder-sim-content build` produces: `manifest` plus `facts`, `risks`, `friction`, `archetypes`, `scenes`; see content/README.md). Records follow the content rules: `provenance` is `real|estimated|invented`, and `real` needs `sources`. The engine validates the pack with `kb.Validate` and merges it at startup.

## LLM-visible tools (`capabilities: ["llm_tools"]`)

`LLMToolSpec`: `{"name", "description", "schema": <JSON Schema object>}`. v1 limitation: the engine pre-runs argument-less tools (no `required` properties) before each turn and hands the result to the narrator as context. Mid-turn tool calling by the model is a later milestone.

## Proposal (what a plugin may return)

```json
{
  "todos": ["Call the CA"],
  "pending": [{"in_days": 2, "description": "…", "probability": 0.7}],
  "npcs": [{"name": "Ravi", "relation": "friend", "description": "…"}],
  "deltas": [{"op": "metric_set", "key": "notepad.open_notes", "amount": 3}]
}
```

Anything else in the proposal (`narration`, `slot_cost`, `outcomes`, core-state ops) is rejected with a reason you can see in the server log. The conformance suite fails a plugin whose proposals include any of it.

## Conformance

Go plugins: call `conformance.Run(t, yourPlugin)` from a test. Other languages: run your service and point the suite at it —

```go
conformance.Run(t, httpadapter.NewClient("http://localhost:9001"))
```

The built-in notepad passes the suite both in-process and over HTTP; that is the guarantee that the two transports behave identically.
