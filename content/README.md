# Content

Licensed CC BY-SA 4.0 (`content/LICENSE`), separately from the AGPL code; see `LICENSING.md` at the repo root.

Everything the narrator is grounded on lives here, as data. The engine never embeds it; it loads this directory (development) or a compiled bundle (production) at startup. Change a fact, a scene or a probability, restart the server or rebuild the bundle, done. No Go involved.

The story bible and research plan that this tree implements are in [docs/STORY_AND_RESEARCH.md](../docs/STORY_AND_RESEARCH.md).

## Layout

```
manifest.json            name, version, as_of
common/                  applies to every segment
b2b_saas/  b2c/  deep_tech/   segment-specific
  facts.jsonl            grounding statements (research plan §B4)
  risks.jsonl            hidden checklist templates
  friction.jsonl         "person behind the counter" event templates
  archetypes.jsonl       NPC sketches
  scenes.jsonl           scenario cards from the story bible
```

One JSON object per line. Lines starting with `#` or `//` are ignored. A record's own `segment` array overrides the directory it sits in.

## Rules

Every fact, risk and friction record carries `provenance`: `real`, `estimated` or `invented`. `real` requires at least one entry in `sources`. Invented is fine; unmarked is not. Date everything with `as_of`; government fees go stale. Paraphrase complaints, never copy them, and never store names or handles. `note` is for humans (caveats, what to replace); it is never shown to the model.

## Record shapes

Fact:

```json
{"id":"gst.mandatory_threshold","segment":["b2b_saas"],"topic":"gst","chapter":"compliance",
 "claim":"...","numbers":{"cost_inr_min":null,"days_min":null},"provenance":"real",
 "sources":[{"url":"...","title":"...","retrieved":"2026-09-23"}],"as_of":"2026-09",
 "confidence":"high","wall":null,"used_by":["scene.customer_asks_gst_invoice"],"tags":["invoice"]}
```

`topic` is the retrieval key (gst, invoicing, tax, incorporation, registration, compliance, payments, build_market, zero_to_one, life). `wall` is `real` or `imaginary` for facts that feed the imaginary-wall mechanic.

Risk (hidden checklist item):

```json
{"id":"gst_invoice_request","label":"...","best_case":"...","worst_case":"...",
 "trigger_prob_per_week":0.15,"applies_when":"has_customers","imaginary":false,
 "provenance":"real","sources":[...],"as_of":"2026-09"}
```

`applies_when` gates the weekly roll: empty (always), `unincorporated`, `incorporated`, `has_customers`, `has_gstin`, `agency_build`.

Friction:

```json
{"id":"friction.name_mismatch_rejection","process":"company_name_approval","what_happens":"...",
 "delay_days":{"min":3,"max":14},"extra_cost_inr":{"min":0,"max":2000},"frequency_hint":"common",
 "provenance":"estimated","as_of":"2026-09"}
```

Scene (a situation, never a script):

```json
{"id":"scene.friend_wants_in","chapter":"ideation","title":"...","situation":"...",
 "options":["..."],"truth":"...","outcomes":["..."],"delayed_consequences":["..."],
 "kb_topics":["life"],"source":"raman:klixa"}
```

## Publishing

```sh
go run ./cmd/founder-sim-content validate ./content
go run ./cmd/founder-sim-content stats ./content
go run ./cmd/founder-sim-content build ./content -o content.bundle.json
founder-sim serve -content content.bundle.json
```

The bundle carries a hash; the server logs it at startup so you know which content a run was played against. The bundle's `index` field is reserved for a precomputed retrieval index (embeddings) so the engine never computes one at runtime; `FactsFor` uses keyword scoring until that exists.

## Story pipeline

Tell a story in any form — a voice transcript, a WhatsApp dump, bullet notes — save it as text, and run:

```sh
export FOUNDER_SIM_LLM_BASE_URL=https://api.openai.com/v1 FOUNDER_SIM_LLM_MODEL=gpt-4o FOUNDER_SIM_LLM_API_KEY=sk-…
go run ./cmd/founder-sim-content ingest ./content -story ~/klixa.txt -slug klixa
```

The pipeline normalises the story (timeline, people, decisions, walls, numbers, lessons), marking every item **told** or **inferred**; extracts scenes, risks, friction templates, archetypes and facts in the shapes above; prefixes ids with the story slug and drops any that collide with existing content; validates the merged tree; and writes `<segment>/<kind>.story-<slug>.jsonl` plus `reports/story-<slug>.md`. Nothing from a story is ever `real`: told → `estimated`, inferred → `invented`, all sourced to `founder story: <slug>`. The report lists research to-dos so an agent can go and confirm them. The raw and normalised story are kept in `stories/` so the extraction can be re-run when the prompts improve.

## Research agents

Part B of the story and research doc describes seven agent runs whose output is JSONL in exactly these shapes, one file per topic per segment, plus a `REPORT.md` each. Drop their files here, run `validate`, and replace the `estimated`/`invented` records they supersede.
