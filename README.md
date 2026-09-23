# Founder Sim

A text-based, decision-driven simulation of the first months of starting a company in India. Not a game about winning. A game about the hundreds of small, boring, unforced decisions that decide whether a startup lives.

The game never tells you what to do. Nothing is graded. Time keeps running while you're away. The engine remembers everything you ignored.

**Status: engine skeleton.** The loop works end to end (login → profile → idea → daily actions → events → exit) with a fake model. Content is a hand-seeded starter set for B2B SaaS. See [docs/SPEC.md](docs/SPEC.md) for the design, [docs/STORY_AND_RESEARCH.md](docs/STORY_AND_RESEARCH.md) for the story bible and research plan, and [docs/DECISIONS.md](docs/DECISIONS.md) for what was decided and why.

Three binaries, deliberately separate:

| binary | runs where | job |
|---|---|---|
| `founder-sim` | your server | the game: engine, UI, accounts, plugins |
| `founder-sim-bridge` | a player's machine | generic reverse tunnel so the server can reach services only that machine can: a local model, a plugin in development |
| `founder-sim-content` | wherever content is edited | validates and compiles the `content/` tree into one dated bundle; ingests founder stories into game data; where an embedding index will be built before publishing |

## Run it

```sh
go build -o founder-sim ./cmd/founder-sim
FOUNDER_SIM_FAKE_LLM=1 ./founder-sim serve -dev
```

Open http://localhost:8080, enter any email; in dev mode the magic link is shown on the page (and printed to the log). Pick a 10-second day to see the clock move.

To play with a real model, drop `FOUNDER_SIM_FAKE_LLM`, set `FOUNDER_SIM_MASTER_KEY` (any 16+ chars; it encrypts player API keys at rest), and configure the model under **Model**: any OpenAI-compatible chat endpoint works — OpenAI, OpenRouter, a LiteLLM proxy, vLLM. Your key, your inference; the server never pays for tokens.

```sh
FOUNDER_SIM_MASTER_KEY='something-long-and-private' ./founder-sim serve -addr :8080 -data ./data -content ./content
```

### A model on your own machine

The hosted server can't reach Ollama on your laptop, and you shouldn't open a port for it. Run the bridge instead: it dials out, long-polls the server for requests, forwards them to local services you name, and posts results back. Nothing listens on your machine.

```sh
go build -o founder-sim-bridge ./cmd/founder-sim-bridge
founder-sim-bridge -server https://your-server -token <from Settings ▸ Bridge> \
  -service llm=http://localhost:11434/v1
```

Then set the model's base URL to `bridge://llm`. The bridge is generic: `-service myplugin=http://localhost:9001` exposes a plugin you're developing the same way. Every player has their own token; many players can run bridges against one server.

## How it's built

```
engine/      hard state, seeded RNG, clock, delta validation — the only write path
content/     THE DATA: facts, checklist items, friction templates, archetypes, scenario cards (JSONL, provenance-tagged) — edited and published independently of the engine
kb/          loader, validator and queries over that content
bridge/      generic reverse tunnel: hub (server side), client (player side), wire protocol
plugin/      the plugin contract: one Go interface, HTTP adapter, conformance suite
plugins/     built-in plugins (notepad is the reference)
internal/llm   OpenAI-compatible client, turn prompt, strict proposal parsing, fake model
internal/game  one action end to end: catch up the clock, narrate, validate, fan out to plugins, persist
internal/store file store (SQLite is a drop-in behind the same interface)
internal/auth  magic links, sessions, AES-GCM for keys at rest
internal/web   htmx UI
cmd/founder-sim          the server
cmd/founder-sim-bridge   the tunnel a player runs
cmd/founder-sim-content  content validate / build
cmd/notepad-plugin       the notepad as a standalone HTTP plugin
```

The core split: **structured state is the source of truth; the model narrates and proposes.** Every turn the model returns JSON — narration, a slot cost, weighted outcomes, typed deltas, new to-dos, future events. `engine.Apply` validates each piece against who sent it and rolls outcomes with the run's seeded RNG. The model cannot mint money, cannot end a run, cannot see a roll before it happens. Plugins go through the same gate with fewer rights.

Randomness is a pure function of (run seed, draw index), so a run is replayable from its JSON.

## Plugins

Anyone can extend the game: a notepad, an alarm, a mentor NPC, a content pack of real MCA fees. A plugin is either compiled in (implement `plugin.Plugin`) or a separate HTTP service in any language speaking the protocol in [docs/PLUGIN_PROTOCOL.md](docs/PLUGIN_PROTOCOL.md). The engine can't tell the difference; `plugin/conformance` proves it by running the same suite through both transports.

What a plugin may do: add player-facing tools with their own panel and per-run state; subscribe to engine hooks (day advanced, turn resolved, event fired, run started/exited); contribute knowledge-base packs; expose tools the narrator model can read. What it may not do: see the hidden checklist or unresolved rolls, or change core state (money, time, entity, cap table, build, users). It can propose to-dos, future events, NPCs and namespaced metrics; the engine rejects anything else and logs it.

Register remote plugins with `FOUNDER_SIM_PLUGINS=http://host:9001,http://other:9002`. Plugin names must be unique. Players opt into plugins per run.

## Develop

```sh
go test ./...          # engine invariants, content validity, plugin conformance, bridge, whole HTTP loop
go run ./cmd/notepad-plugin -addr :9001   # the notepad, remotely
go run ./cmd/founder-sim-content validate ./content
go run ./cmd/founder-sim-content ingest ./content -story story.txt -slug name   # story → game data (needs a model)
```

## License

Code: **AGPL-3.0-only** (`LICENSE`), with an additional permission so plugins and bridged services that talk to the engine over its protocols can use any licence. Content under `content/`: **CC BY-SA 4.0** (`content/LICENSE`). Details and reasoning in [LICENSING.md](LICENSING.md).