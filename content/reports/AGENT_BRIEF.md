# Research agent brief

You are one of the research agents described in `docs/STORY_AND_RESEARCH.md` §B (read §B1–B7 first, then `content/README.md`). Your job: produce sourced, dated, provenance-tagged JSONL records for the Founder Sim knowledge base, for the topic assigned to you, plus a short REPORT.md.

Today is 2026-09-23. Use `"retrieved": "2026-09-23"` and `"as_of": "2026-09"` unless the source is clearly older, in which case put the source's date in `as_of`.

## Where to write

Repo root: `/home/claude/founder-sim`. Write ONLY these files (create them; never edit files you did not create):

- `content/common/facts.<topic>.jsonl` — facts that apply to every segment
- `content/<segment>/facts.<topic>.jsonl` — segment-specific facts (b2b_saas, b2c, deep_tech)
- `content/common/risks.<topic>.jsonl` — new hidden-checklist items your research justifies
- `content/common/friction.<topic>.jsonl` — friction event templates (the friction miner writes most of these; other agents add only what they stumble on)
- `content/reports/<agent>.md` — your REPORT.md

One JSON object per line, valid JSON, no trailing commas, UTF-8. `#` comment lines are allowed.

## Record shapes (exact field names; extra fields are ignored, wrong types fail validation)

Fact:
```
{"id":"<topic>.<snake_case_slug>","segment":["b2b_saas"],   // omit "segment" for all segments
 "topic":"<topic>","chapter":"compliance",                    // ideation|building|zero_to_one|compliance
 "claim":"One or two plain sentences a narrator can use. Concrete: rupees, days, form names.",
 "numbers":{"cost_inr_min":0,"cost_inr_max":0,"days_min":0,"days_max":0},   // include only the keys you know
 "provenance":"real",                                         // real|estimated|invented
 "sources":[{"url":"https://…","title":"…","retrieved":"2026-09-23"}],
 "as_of":"2026-09","confidence":"high",                       // high|medium|low
 "wall":"imaginary",                                          // real|imaginary, ONLY for payments/blocker facts; omit otherwise
 "tags":["…"],"note":"caveats for humans; never shown to the model"}
```

Risk (hidden checklist template):
```
{"id":"<snake_case>","label":"Operating without X","best_case":"…","worst_case":"…",
 "trigger_prob_per_week":0.1,"applies_when":"has_customers",  // ""|unincorporated|incorporated|has_customers|has_gstin|agency_build
 "imaginary":false,"provenance":"real","sources":[…],"as_of":"2026-09","note":"…"}
```

Friction:
```
{"id":"friction.<snake_case>","process":"<process>",          // company_name_approval|gst_registration|bank_account|gateway_kyc|agency|any_government_portal|dsc_din|pan_tan|udyam|trademark|app_store
 "what_happens":"Paraphrased pattern. No names, handles, companies of individuals, or quotes.",
 "delay_days":{"min":3,"max":14},"extra_cost_inr":{"min":0,"max":2000},
 "frequency_hint":"common",                                    // rare|occasional|common
 "provenance":"estimated","sources":[…],"as_of":"2026-09","note":"…"}
```

## Rules (validation enforces the first three)

1. `provenance: "real"` REQUIRES at least one source with a real URL you actually fetched or saw in search results. If you could not open the page, downgrade to `estimated` and say so in `note`.
2. Every `id` is unique. Prefix with your topic. Check `content/common/*.jsonl` and `content/b2b_saas/*.jsonl` for existing ids and do not reuse them. If your research corrects or improves an existing record, write a NEW record with a new id and list the superseded id in your REPORT under "Supersedes".
3. Segments are only `b2b_saas`, `b2c`, `deep_tech`. Chapters only `ideation|building|zero_to_one|compliance`.
4. Paraphrase; never copy sentences from sources. Never store personal data: no names, handles, emails, phone numbers, or identifying details from complaints or forum posts. Store the *pattern*.
5. Invented is fine; unmarked is not. Numbers you inferred are `estimated`. Anything you made up to fill a gap is `invented`, with a `note` saying what real data would replace it.
6. Prefer official sources (mca.gov.in, gst.gov.in, incometaxindia.gov.in, startupindia.gov.in, rbi.org.in, udyamregistration.gov.in, payment-gateway docs) for numbers and rules; practitioner blogs (CA/compliance firms) for timelines and common mistakes, cross-checked; Reddit/forums/founder blogs for human friction. Record every source you used.
7. Claims must be usable by a narrator: concrete and specific. "GST registration usually takes 3–7 working days once the ARN is generated, longer if the officer raises a query" beats "GST registration takes time".
8. Aim for 15–40 fact records for your topic, plus risks/friction where justified. Quality over count. Do not pad.
9. Work in the repo directory. When done, run `cd /home/claude/founder-sim && go run ./cmd/founder-sim-content validate ./content`. If errors are in YOUR files, fix them. If errors are in another agent's files, ignore them and note it. Other agents write concurrently; do not touch their files.
10. Time budget: be efficient. Don't fetch more than ~25 pages. If a site refuses to load, move on and note it.

## REPORT.md structure

```
# <Agent name> — report (2026-09-23)
## Files written
## What was found (3–8 bullets, the important facts and numbers)
## What was guessed or estimated (and what would replace it)
## Supersedes (existing record ids your records improve on, if any)
## Sources that failed to load
## Open questions for the game designer
```
