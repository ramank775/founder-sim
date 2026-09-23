# Founder Sim — Story Bible & Research Plan

Companion to the build spec. Two parts:

- **Part A — Story bible:** the scenarios, beats and hidden mechanics that make up the play, written as scenario cards the engine and content packs can be built from.
- **Part B — Research plan:** what data the scenarios need, where to get it, the output format, and how to run the research agents.

Content rule for everything here: **the engine owns state and dice; content describes situations, options, and outcome ranges.** Nothing below is a script. Every card is a situation the player may or may not walk into.

---

# PART A — STORY BIBLE

## A1. The shape of a run

A run is not a plot. It's a life with a startup inside it.

```
Still employed, idea forming
  → tells people (or doesn't)
  → decides who builds it (self / agency / cofounder)
  → building, in phases, day by day
  → maybe launches, maybe validates first
  → hunts for the first real user
  → paperwork arrives when reality forces it (or earlier, if the player thinks of it)
  → ... until the founder chooses to exit
```

Running underneath everything, at all times:

- The **clock** (real-time by default; the world moves while you're away).
- **Monthly obligations** (EMIs, rent, bills) draining regardless of progress.
- The **hidden risk & compliance checklist**, quietly accumulating exposure.
- **NPCs** interrupting on their own schedule.
- **Motivation**, which drops from tedium more than from difficulty (see A6).

## A2. Player setup cards

### Card: Where are you right now?
- **Ask:** current job and what a normal day looks like.
- **Feeds:** available hours per day, network (who they plausibly know), notice-period friction if they quit, starting savings realism.

### Card: What's the idea?
- **Ask:** free text.
- **Engine does:** classify into segment (B2C / B2B SaaS / deep tech), generate hidden risk & compliance checklist, pick which scenario cards are eligible.

### Card: Where did it come from?
Idea origin changes the starting conditions.

| Origin | Starts you with | Hidden cost |
|---|---|---|
| Your own pain | narrow, real problem; strong conviction | may be the only user; validation skipped because "I know this" |
| Saw something and liked it | fast start, clear picture of the product | shallow understanding of why it worked there |
| An event forces it | a real deadline and a real first audience | built for one occasion; what happens after the event? |

*Real source:* One9x (own pain: people kept asking Raman to deploy things for them; he built it without validating). Klixa (saw something similar, a cousin's function was the deadline).

### Card: Can you build it?
- Asked **after** the idea, so it lands on top of the excitement.
- Technical → burns **time**. Non-technical with a technical idea → **skill mismatch**, burns **money**.

### Card: Your life
- Age, married or not, kids or expecting, savings, monthly obligations.
- Not difficulty. Two different games:
  - **~24, single:** almost no savings; quitting costs little; family asks about marriage and "real jobs".
  - **~34, family:** savings and a network; EMIs and school fees that don't care about your startup; every risk weighs more.

### Card: Time scale
- Real-time default (1 day = 1 hour, clock runs while away), faster options available.

## A3. Chapter 0 — The idea, while still employed

### Scene: The first weekend
- **Situation:** You have a free weekend and an idea.
- **Options (player phrases it; engine recognises intent):** build a prototype · message people who might want it · research whether it already exists · do nothing yet.
- **Truth:** most founders build. The game does not punish this. It just starts the clock on the question they skipped.
- **Delayed consequence (if validation skipped):** weeks later, the product works and nobody uses it; or it turns out someone already built it.

### Scene: Telling people
- **Cost:** free.
- **Reaction roll per person** (drawn from the characters the player described):
  - dismissive ("so it's just Uber for X?")
  - over-enthusiastic, useless ("brilliant, do it!")
  - the one hard question you can't answer
  - quietly useful (knows someone)
  - it turns into an argument
- **Key truth:** you can't tell which feedback mattered until much later. Encouragement feels good and teaches nothing.

### Scene: The friend who wants in
- **Trigger:** an enthusiastic friend offers to join.
- **Decision:** cofounder or not; if yes, what equity; vesting or not (almost nobody writes vesting with a friend).
- **Common default:** 50-50 because anything else feels insulting.
- **Delayed consequence roll:** friend keeps day job and contributes little · friend is genuinely great · friend leaves after N months holding equity.
- **Fixing it later:** claw back equity (lose the friend; social pressure arrives randomly — family asks why you did that to him) or compromise (live with the cap table).

### Scene: The imaginary wall
- **Situation:** the player hits something non-technical that looks like a hard blocker.
- **Canonical example (Klixa):** "I need an incorporated company to get a payment gateway." In reality a PAN-based setup would have worked.
- **Mechanic:** the game presents the belief as plausible and does not correct it. If the player investigates (asks someone, researches, calls the provider), they discover the truth, at a time cost. If not, the project may stall on a wall that never existed.
- **Why it matters:** this is how a lot of founder projects actually die. Not from failure; from a belief.

## A4. Chapter 1 — Building

### Path: Non-technical → agencies
- **Scene: Getting quotes.** Player contacts several agencies/freelancers. Every one is polite and agrees to everything.
- **Quote spread:** cheap / middle / expensive. The player cannot tell which is lying. Outcomes are rolled, not priced in.
- **Outcome roll per agency:** delivers roughly as promised · late (commitment issues) · overcharges mid-way · build fails · ghosts.
- **Scene: It's late and money is running out.** Options: pay more (sunk cost with the same people) · start over (lose months and cash) · slowly learn enough to check the work yourself.
- **The slow path:** should feel slow and frustrating. It becomes daily choices: test the build yourself today? follow up with them? read up on something? just wait? The agency comes back with random questions and requirement changes.
- **Bugs:** delivered builds come with bugs the founder can ask to be fixed (each ask costs time and sometimes money).

### Path: Technical → build it yourself
- Burns time, not money. Feels productive. Nothing else moves while you build: no customers, no revenue, and the EMI still hits.
- **Truth:** building is the most comfortable way to burn runway.
- **Pattern to model (from Raman):** technical founders often want to build, not run a company. Non-technical tasks drain motivation fast.

### Hidden factors (both paths)
- Example: the website is slow.
- **Technical founder:** can notice the cause.
- **Non-technical founder:** only sees the symptom (fewer signups) and has to trust the agency, who says it's fine.
- Other candidates for research: security holes, broken mobile layout, missing analytics, email going to spam, app store rejection.

### Phases and launch timing
- Build happens in phases. Shipping half-done is always the founder's call.
- **Before the build finishes:** option to put up a landing page and collect beta signups. Validating risks sitting on 12 signups not knowing if that's good. Skipping risks finding out much later.

### Metrics
- Not given. The founder chooses what to track, or pays someone who knows.
- Wrong-metric traps: tracking signups and missing that nobody returns; tracking revenue and missing that it's one customer.

## A5. Chapter 2 — Zero to one

### Scene: The first user who isn't a friend
- **Routes (Raman tried all of them with One9x):** post it somewhere and hope · message strangers one by one · paid ads without knowing what you're doing · intros through people you know.
- **Network honesty:** the game asks, when needed, "who would you ask for an intro?" and the player names someone from their real life. First users come from the network the player actually described. If they said they know nobody in tech, the game holds them to it.
- **Outcome rolls:** silence · a polite "interesting" · a real user who churns · a real user who stays · a user who wants something you can't provide yet (invoice, contract, integration).

## A6. Cross-cutting systems (story side)

### Incorporation & compliance
- **Never prompted.** The player must think of it.
- Hidden checklist per idea, generated at setup. Each item: best case, worst case, probability per week of surfacing.
- **Items surface as events, not warnings.** Canonical: a customer asks for a GST invoice; you have no GSTIN; the deal stalls; recoverable, but it costs weeks.
- **DIY vs CA:** doing it yourself asks the player for the actual documents needed (this is the knowledge test inside the game). A CA costs money and still has delays.

### The person behind the counter
- Government processes are simulated as people, not fee tables.
- Friction events: form rejected over a name mismatch · a document nobody mentioned · the portal is down · the officer is on leave · it just goes through.
- Same filing: sails through for one player, bounces three times for another.
- **Bribe option:** exists as satire of a broken process. Always costly and uncertain, never the optimal strategy, can backfire.

### NPCs
- Cast built lazily from the player's own descriptions (family, friends, colleagues, people they know).
- Nag, interrupt, sometimes help, sometimes fight. Each interaction is a roll. The same uncle: money one visit, a week of silence the next.
- Family pressure recurs, a little sharper each time.
- Pressure clusters: smooth weeks and hell weeks. A quiet week is the reward.
- Ignoring is always allowed; it costs later, sometimes.

### Missed things
- Unanswered calls, unfollowed leads, unticked items, unwritten notes. The engine logs them all.
- They resurface later as **realizations** ("that call in week 3 was the investor's assistant").

### Waiting
- Deliberate dead time after filings, agency handoffs, first outreach. Long enough that players feel like quitting, because that's where real projects die.

### Motivation (hidden variable)
- Rises with shipping and real user contact. Bleeds from tedium (paperwork, waiting, follow-ups), not from difficulty.
- Low motivation doesn't end the run; it makes the "park it" option look increasingly reasonable. The player still decides.

### Skipped decisions
- Fast-forward is allowed. The engine picks a default, records it, and the consequence arrives later like any other.

### Ending
- Only the founder ends it: sell · shut down · take a job · walk away. End summary: how it ended, runway burned, key moments, what you learned.
- Learned items carry into the next run.

## A7. Real-life source log

Beats taken from Raman's experience, to keep grounded and to reference in content:

- **One9x:** own itch (repeated deploy requests), built without validation, tried every zero-to-one route.
- **Klixa:** inspired by something similar, cousin's function as deadline, stalled at payment gateway on the false belief that incorporation was required.
- **General pattern:** hitting a non-technical wall → park it or lose interest. Tech founders want to build, not to run a startup.

---

# PART B — RESEARCH PLAN

## B1. Goal

Build a versioned, dated knowledge base (KB) that the scenario cards draw from, per segment. v1 target: **B2B SaaS** first (smallest, matches the hand-written KB in v1), then B2C, then deep tech.

## B2. What the KB must contain

Grouped by the scenario that consumes it.

### Incorporation & registration
- Entity types available to an early founder (sole proprietorship, partnership, LLP, OPC, private limited): when each makes sense, cost range, typical timeline, documents required.
- DIY vs CA/online-filing-service: cost range, time, common mistakes.
- Name approval process and common rejection reasons.
- Post-incorporation must-dos (bank account, PAN/TAN for entity, etc.).

### Tax & compliance
- GST: when registration becomes mandatory vs voluntary, what's needed, timeline, what breaks without it (B2B invoicing).
- Invoicing rules for individuals vs entities.
- Recurring compliance obligations by entity type and their penalties for missing them.
- Startup recognition schemes (e.g. DPIIT): eligibility, benefits, effort.

### Payments (imaginary wall data)
- Which payment gateways onboard individuals / sole proprietors with PAN only, what they need, typical onboarding time and rejection reasons.
- What genuinely requires an entity.
- **Tag each fact as "wall is real" or "wall is imaginary"** — this directly feeds the imaginary-wall mechanic.

### Building
- Agency/freelancer market in India: quote ranges for an MVP by type (web app, mobile app, SaaS dashboard), typical timeline slippage, common failure patterns, payment-milestone norms.
- Common hidden technical issues in agency-built MVPs.
- App store / hosting / domain costs and friction.

### Zero to one
- Channels early Indian founders use to find first users per segment, and what founders report about each.
- Typical early conversion numbers where credible sources exist (flag as rough).

### Human friction (complaints mining)
- Real complaints about registration, GST, bank account opening, gateway onboarding, agencies: what went wrong, how long it took, what was asked for unexpectedly.
- Output as **friction event templates**, not quotes (see B4). No personal identifiers ever stored.

### Life & money
- Rough realistic ranges for early-career vs mid-career savings, EMI patterns, notice periods, and family-pressure themes. Treat as flavor; mark as invented where not sourced.

## B3. Sources

- **Official:** MCA, GST portal, Income Tax, DPIIT/Startup India, RBI (for payment aggregator rules), payment gateway docs and onboarding pages.
- **Practitioner:** CA and compliance-service blogs and guides (useful for timelines and common mistakes; cross-check with official sources).
- **Founder experience:** Reddit (Indian startup and business subreddits), forums, founder blogs, Twitter/X threads. Note Twitter/X API access is expensive; forums and Reddit often have fuller write-ups anyway.
- The agent picks sources itself within these classes, and records every source it used.

## B4. Output format

Every fact is one record. JSON Lines, one file per topic per segment.

```json
{
  "id": "gst.mandatory_threshold",
  "segment": ["b2b_saas", "b2c", "deep_tech"],
  "topic": "gst",
  "claim": "Plain-language statement of the fact.",
  "numbers": { "cost_inr_min": null, "cost_inr_max": null, "days_min": null, "days_max": null },
  "provenance": "real",              // real | invented | estimated
  "sources": [{ "url": "...", "title": "...", "retrieved": "2026-09-23" }],
  "as_of": "2026-09",
  "confidence": "high",              // high | medium | low
  "wall": null,                      // "real" | "imaginary" | null
  "used_by": ["scene.customer_asks_gst_invoice"]
}
```

Friction event templates (from complaint mining):

```json
{
  "id": "friction.name_mismatch_rejection",
  "process": "company_name_approval",
  "what_happens": "Application rejected because a name differs slightly across documents.",
  "delay_days": { "min": 3, "max": 14 },
  "extra_cost_inr": { "min": 0, "max": 2000 },
  "frequency_hint": "common",       // rare | occasional | common
  "provenance": "real",
  "sources": [{ "url": "...", "retrieved": "2026-09-23" }],
  "as_of": "2026-09"
}
```

Rules:
- **Paraphrase, don't copy.** Store the pattern of the complaint, not the text.
- **No personal data.** No names, handles, or identifying details from complaints.
- **Invented is fine; unmarked is not.** Anything without a source is `invented` or `estimated`.
- **Date everything.** Numbers go stale, especially government fees.

## B5. Research agent runs

Run as separate, parallelisable jobs, each producing JSONL into `kb/<segment>/<topic>.jsonl` plus a short `REPORT.md` of what was found, what was guessed, and open questions.

1. **Entity & incorporation agent** — B2 incorporation section.
2. **Tax & compliance agent** — GST, invoicing, recurring compliance, startup recognition.
3. **Payments agent** — gateway onboarding for individuals vs entities; mark every wall real or imaginary.
4. **Build market agent** — agency quotes, slippage, failure patterns, hidden issues.
5. **Zero-to-one agent** — first-user channels and founder reports per segment.
6. **Friction miner** — complaints across all of the above → friction event templates.
7. **Reviewer agent** — cross-checks records against official sources, flags contradictions and stale data, downgrades confidence where needed. Runs last.

Order for v1: 1, 2, 3 first (they feed the compliance checklist and the imaginary-wall mechanic, which are the most distinctive parts of the game), then 6, then 4 and 5, then 7.

## B6. Mapping KB → scenes

| Scene | KB topics |
|---|---|
| Imaginary wall (payments) | payments (wall flags) |
| Customer asks for GST invoice | gst, invoicing |
| DIY vs CA incorporation | incorporation, friction |
| Person behind the counter | friction templates |
| Agency quotes and outcomes | build market |
| Hidden technical issues | build market |
| First user hunt | zero-to-one |
| Hidden risk checklist | incorporation, gst, compliance, payments |

## B7. Definition of done for the first research pass

- B2B SaaS KB covers incorporation, GST/invoicing, payments, and at least 20 friction templates.
- Every record has provenance, as_of, and at least one source if marked real.
- A REPORT.md per agent listing gaps and invented items to replace later.
- Enough data to generate a believable hidden checklist for a B2B SaaS idea end to end.
