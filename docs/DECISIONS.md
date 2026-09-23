# Decisions

Running log of choices made while building. The spec (SPEC.md) says what the game is; this says what we picked when the spec left it open, and what is a deliberate v1 shortcut versus a real decision.

## 2026-09-23 — engine skeleton

**Stack: Go single binary, server-authoritative.** The spec suggested React + Vite with browser-only persistence. Changed to a Go backend because keys and clock live server-side now, and Raman wants one thing to host. UI is Go-rendered HTML + htmx. No JS build.

**Keys are stored server-side, encrypted (AES-256-GCM, master key from env).** This departs from the spec's "key never touches our server". BYOK still holds in spirit: players supply the key, the server never pays for inference. Per-user key derivation can replace the single master key later without changing the stored shape.

**LLM interface is OpenAI-compatible chat completions**, so LiteLLM, Ollama, vLLM, OpenRouter and OpenAI all work by changing a base URL. An Anthropic-native adapter can sit behind the same `llm.Client` if needed. JSON mode is probed per endpoint; without it the parser tolerates prose wrapping and fences.

**Bridge is a separate binary and is generic.** `founder-sim-bridge` runs on a player's machine, dials out to the server and long-polls (stdlib HTTP, no WebSocket dependency, NAT-friendly). It exposes named local services (`-service llm=...`, `-service myplugin=...`); the server addresses them as `bridge://<service>` and gets an `http.RoundTripper`, so any component that takes an HTTP client can reach a player's machine. Per-player tokens, many players per server. The LLM is the first user; plugins in development are the intended second.

**Content is data, outside the binary.** `content/` holds JSONL in the research plan's §B4 shapes plus scenario cards from the story bible. The engine loads a directory or a compiled bundle at startup. `founder-sim-content` validates and builds the bundle; that step is where an embedding index will be computed and stored in the bundle so the engine never does it at runtime. Story and research ship on their own cadence.

**One LLM call per turn for now.** The spec worried narration might contradict applied state after the roll. We ship one call, keep raw model output visible in dev mode, and split into parse-then-narrate only if drift is observed.

**Time cost of an action: the model proposes, the engine clamps** to 1..slots-left. A KB-defined per-action-type range is a later refinement.

**Plugins: one Go interface, two transports.** Built-ins compile in; remote plugins speak the same interface over HTTP+JSON. The conformance suite runs both against the notepad. Plugins see the player's view only, and may add (todos, events, NPCs, namespaced metrics) but never mutate core state. Per-run state is an opaque blob carried on each call.

**Plugins are per-run opt-in.** A notepad that exists changes how the game feels; the spec makes using one a choice.

**Persistence: file store (one JSON per record) in v1.** Deliberate shortcut, not a decision — the Go module proxy was unreachable from the sandbox where this was built. `store.Store` is the seam; a SQLite implementation (`modernc.org/sqlite`, no cgo) is the intended next step and should reuse `store_test` once written.

**Auth: email magic link, printed to the server log.** No SMTP until there is a second player. `auth.Sender` is the seam.

**Randomness: hash of (seed, draw index).** Replayable from JSON without storing generator state. Weekly risk probabilities are converted to daily as `1-(1-p)^(1/7)`.

**Catch-up cap: 60 days per return.** A run abandoned for a year resolves 60 days and re-anchors rather than grinding 8,000 days.

**Cross-run learning, faked:** discovered checklist items are stored on the player; next run they start `discovered` (the founder knows they exist) but not `ticked` (still has to do them).

**Best/worst case on a triggered risk is a 50/50 roll.** Placeholder; should depend on how long the item has been unticked and on disposition of the NPC involved.

**Seed content:** 14 facts, 12 checklist items, 8 friction templates, 10 archetypes, 14 scenes, each provenance-tagged (`real` / `estimated` / `invented`). Fee ranges and probabilities are mostly estimates and say so; the research agents in STORY_AND_RESEARCH.md §B5 replace them.

**Story pipeline (`founder-sim-content ingest`).** Raw story in any form → normalised story with told/inferred marking → records in the content shapes → id prefixing and collision drop → validate → write files and a report with research to-dos. Anecdotes never become `real`; the research agents confirm them. Raw and normalised stories are kept for re-runs.

**Research agents.** Brief in `content/reports/AGENT_BRIEF.md`; §B5's seven runs as subagents (sonnet for gathering, opus for the reviewer). Only the incorporation run has been executed so far; the rest are on hold at Raman's request.

**UI: not three.js.** A text sim feels like a game through a single persistent screen, a visible clock, interruptions that arrive, paced narration and a diegetic frame (desk, phone, notebook), not 3D. Plan: add a JSON API beside the htmx pages and build the run screen as one "desk" view; 3D stays optional on top of that API.

## Open

- Should plugins' LLM tools be able to inject text into the narrator's context (a "mentor" plugin)? Powerful; blurs "the game never tells you what to do".
- Plugin discovery: env var of URLs now; a manifest directory or registry later.
- Lower per-turn timeout for tunnelled models; store last N raw model responses per player for debugging.
- HTML sanitiser is an allowlist regex; replace before enabling untrusted remote plugins with panels.
- htmx is loaded from unpkg; vendor it for offline/self-hosted installs.
