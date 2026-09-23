# Founder Sim — working notes for Claude Code

Read this first. Then `docs/SPEC.md` (what the game is), `docs/STORY_AND_RESEARCH.md` (story bible, research plan), `docs/DECISIONS.md` (what was decided and why, keep it updated), `docs/PLUGIN_PROTOCOL.md`, `content/README.md`.

## What this is

A text-based, decision-driven simulation of the first months of starting a company in India. Go server, htmx/vanilla UI, content as JSONL outside the binary, LLM as narrator only. Side project; optimise for "playable and true-feeling", not scale.

## Non-negotiable design rules (from the spec)

1. The game never tells the player what to do. No prompts to incorporate, no "you should". The narrator prompt enforces this; UI copy must too.
2. No grading, no score, no right answers. Consequences only.
3. Outcomes are non-deterministic and owned by the engine's seeded RNG. The LLM proposes weighted outcomes; `engine.Apply` rolls.
4. Time and attention are currencies. 4 slots a day; the clock runs while the player is away (faked on return, capped at 60 days).
5. The engine remembers what the player ignored; it resurfaces later as events.
6. Some walls are imaginary (`risk.imaginary`); the narrator may let the belief stand until the player tests it.
7. Only the founder ends a run. No game over.
8. Content is data under `content/`; the engine never embeds it.

## Architecture in one paragraph

`engine/` is the single source of truth and the only write path: `Run.Apply(Proposal, Source)` validates typed deltas against who sent them (LLM vs plugin), rolls outcomes, and applies. `internal/llm` builds the narrator prompt from run state + `kb` content and parses strict JSON proposals (one repair retry). `internal/game` runs one action end to end: catch up the clock, ask the narrator, apply, fan out hooks to plugins, persist. `plugin/` is one Go interface with two transports (in-process, HTTP) and a conformance suite; plugins see the player's view only and may add (todos, pending events, NPCs, namespaced metrics) but never mutate core state. `bridge/` is a generic reverse tunnel so the server can reach services on a player's machine (`bridge://llm`). `kb/` loads/validates `content/`. `internal/store` is a file store behind a `Store` interface (SQLite is the intended drop-in: `modernc.org/sqlite`, no cgo). `internal/web` is the UI.

Three binaries: `cmd/founder-sim` (server), `cmd/founder-sim-bridge` (player's machine), `cmd/founder-sim-content` (validate/build/stats/ingest).

## Conventions

- Go 1.24, stdlib only so far. Adding a dependency is a decision; note it in DECISIONS.md.
- `gofmt`, `go vet ./...`, `go test ./...` must pass before a commit. Tests are behavioural (whole HTTP loop with the fake model, bridge with two players, plugin conformance over both transports). Add a test for every engine rule.
- Every content record has `provenance` (`real|estimated|invented`); `real` needs sources. `go run ./cmd/founder-sim-content validate ./content` must pass.
- Commit messages: what and why, present tense. Licence: AGPL-3.0-only code, CC BY-SA 4.0 content (`LICENSING.md`).
- Never commit `data/`, `.env`, `*.bundle.json`, `content/stories/*.raw.md` (see `.gitignore`).
- Run locally: `FOUNDER_SIM_FAKE_LLM=1 go run ./cmd/founder-sim serve -dev` then http://localhost:8080 (magic link shows on the page in dev mode). Real model: set base URL under Model (any OpenAI-compatible endpoint, or `bridge://llm` via the bridge). `FOUNDER_SIM_LLM_TIMEOUT=10m` for slow local models.

## Known gaps / next work (in order)

1. **Game screen.** `internal/web/templates/run.html` is a web form, not a game. Rebuild the run view as a single persistent screen: the founder's phone. A message feed where narration, family, agencies and customers arrive as messages; the action box is the chat input; status bar shows day, slots as four blocks that burn, and a live countdown to the next day; pending events that fire while the page is open slide in as notifications (poll `/api/run/{id}` every few seconds, or SSE); narration reveals at reading pace; money is a bank "app" that costs a slot to open unless pinned; the notepad is another app; plugins' tool panels are apps. Mobile-first, works as a centred phone on desktop. Add a JSON API beside the htmx routes (`GET /api/run/{id}`, `POST /api/run/{id}/act|balance|pin|todo|tool|exit`) and build the screen with vanilla JS + CSS; vendor htmx (`npm pack htmx.org` → `internal/web/static/htmx.min.js`, serve via `embed`) so nothing loads from a CDN. Keep `run.html` working until the new screen is at parity, then delete it. Not three.js.
2. **SQLite store** behind `store.Store`, with a shared store test suite the file store also passes.
3. **Research agents 2–7** per `content/reports/AGENT_BRIEF.md` (only incorporation has run). Use Sonnet for gathering, Opus for the final reviewer.
4. **Two-call turn** (parse/propose, then narrate after the roll) if narration is observed to contradict applied state; keep raw model output visible in dev mode to judge.
5. Chapter 0 content depth (NPC reactions, cofounder/equity branch), then Chapter 1 agency path, then incorporation events, then Chapter 2.
6. Bridge: shorter per-turn timeout for tunnelled models; store last N raw model responses per player for debugging.
7. Replace the regex HTML sanitiser before enabling untrusted remote plugins with panels.

## Sub-agents

Use them. Sonnet for implementation and research gathering, Opus for design judgement and review, Haiku for mechanical fan-out. Give each a file-scoped brief and require `go test ./...` green before it reports back.
