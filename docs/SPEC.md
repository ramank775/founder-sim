# Founder Sim — Build Spec (v1)

A text-based, decision-driven simulation where the player lives through the early pain of starting a company in India. Not a game about winning. A game about the hundreds of small, boring, unforced decisions that actually decide whether a startup lives.

This is a fun side project. Optimise for "playable and true-feeling", not scale or polish.

---

## 1. Design principles (non-negotiable)

These drive every implementation choice. When in doubt, re-read these.

1. **The game never tells you what to do.** No "incorporate now?" prompts, no "time to launch" cards. The player must think of actions themselves, the way a real founder must.
2. **No grading, no right answers.** Each choice shows pros and cons. The player lives with consequences. There is no score.
3. **Non-deterministic outcomes.** The same decision plays out differently for different players (and different runs). Nobody should be able to memorise a correct path.
4. **Time and attention are currencies, not just money.** Doing it yourself saves cash and burns weeks. Every day has limited slots.
5. **The engine remembers everything; the player might not.** Ignored calls, unticked compliance items, skipped decisions all persist in hidden state and resurface later as events.
6. **Some walls are imaginary.** The game may let the player believe a blocker is real (e.g. "you need a company to get a payment gateway") when it isn't. Finding out costs time.
7. **Skipping is a default, not a shortcut.** Players can fast-forward decisions, but the engine picks a default for them and the consequences still land.
8. **Only the founder ends the run.** No game-over screen. The run ends when the player chooses to exit: sell, shut down, or walk away.
9. **Every run leaves you better off.** Knowledge and checklist items learned carry into the next run.

---

## 2. Architecture

### Core split: hard state + agent narration

- **Structured state is the source of truth.** Money, time, obligations, cap table, NPC relationships, hidden risk checklist, pending events. Stored as typed data.
- **The LLM narrates and interprets, it never invents state.** It turns state + a chosen action into prose and proposes state changes as structured output. The engine validates and applies them. The agent cannot give the player money that the state doesn't allow.
- **Randomness is owned by the engine**, via a seeded RNG per run, not by the LLM. The LLM may return probabilities for outcomes; the engine rolls against them.

### Bring your own key (BYOK)

- The player pastes their own API key. The key is stored only in the browser, never sent to any server of ours.
- Build a small provider adapter so the model is swappable. Start with one provider (Anthropic). Leave room for others.
- Optional later: use TypeSafe AI's Jev for bounded judgment calls (classify segment, pick outcome from allowed set, score risk) since it returns typed, calibrated decisions against a schema. v1 does not need it.

### Stack (suggested, adjust if faster)

- Web app first. No native apps in v1 (no Apple developer account, and web gets it playable fastest).
- TypeScript, React + Vite, simple UI.
- Persistence: browser storage (IndexedDB or localStorage) for v1. No backend required unless something forces it.
- LLM calls made directly from the browser with the player's key.

---

## 3. Game loop

### Session start

1. Player describes their **current situation**: job, day-to-day work.
2. Player describes their **idea**.
3. Player gives basic **profile**: age, married or not, kids or expecting, bank balance / savings, monthly obligations (EMIs, rent, bills).
4. Player declares whether they are **technical or non-technical** relative to their idea. This comes after the idea, not before.
5. Engine **classifies the idea into a segment** (internal, not shown).
6. Engine generates the **hidden risk & compliance checklist** for this idea and segment.
7. Player picks a **time scale** (see §5).

Profile is not easy/hard mode. A 24-year-old has little savings and little to lose. A 34-year-old has savings but EMIs that never pause. Both are real constraints, just different ones.

### Daily loop

- Each in-game day, the player sees a handful of things that came up (up to 5–10 to-do items generated from prior choices and events) and a free-text input.
- The player decides what to do with the day. They can type anything; the agent parses intent.
- Each action consumes a slice of the day. The player cannot do everything.
- NPCs interrupt on their own schedule. Some weeks are smooth; some are hell. Pressure should cluster, not spread evenly.
- Player can ignore anything. Ignoring is free today and may cost later.

### Money and metrics visibility

- Balance is **not shown by default**. The player can check it (costs a bit of time) or pin it.
- The player chooses which metrics to track, or hires someone who knows. Tracking the wrong things means flying blind.

### Notepad

- The game provides an optional notepad / to-do list. Using it is the player's choice.
- The engine independently tracks everything the player missed, regardless of the notepad.

### Run end

- Only when the player chooses to exit. Record how it ended, final money, time spent, what was learned.
- Show a short end-of-run summary (how you exited, runway burned, key moments).

---

## 4. Chapters for v1

Build 2–3 chapters first. The early stages are the point; funding is less important.

### Chapter 0 — Ideation (starts while still employed)

- Idea origin shapes the start: your own pain (narrow but real), something you saw and liked (fast but shallow), an upcoming event forcing your hand.
- Decisions: build a prototype first vs talk to people first. Most founders build. Don't punish it; just start the clock.
- Telling friends and family: free, but reactions are random (helpful, dismissive, over-enthusiastic, the one hard question).
- An excited friend may ask to join → cofounder and equity decision. Getting it wrong costs nothing on day one and a lot later, including the social cost of fixing it (lose a friend vs compromise), with family pressure arriving randomly.

### Chapter 1 — Building

- **Non-technical path:** get quotes from multiple agencies. They're all polite and say yes. Outcomes roll: build failure, missed commitments, overcharging, or it works. Then: pay more, start over, or slowly learn enough to check the work yourself (frustrating, slow, and should feel it).
- **Technical path:** build it yourself. Burns time instead of money.
- Build happens in phases. Agencies deliver bugs the founder can ask them to fix.
- **Hidden factors** (e.g. slow website): a technical founder sees the cause; a non-technical founder only sees the symptom (fewer signups) and has to trust the agency's word.
- Shipping half-done is the founder's call.

### Chapter 2 — Zero to one

- Launch timing is the founder's call, including before the build is done (beta signup page to validate) or skipping validation.
- Getting the first real user who isn't a friend: posting and hoping, messaging strangers, ads, intros through people you know.
- First users come from the network the player actually described. If they said they know nobody in tech, hold them to it.

### Incorporation & compliance (cross-cutting, not a chapter prompt)

- Never prompted. The player must decide it's time.
- The hidden risk checklist accumulates exposure while they operate without an entity.
- Unticked items surface as events, not warnings. Example: a customer asks for a GST invoice and you have no GSTIN; the deal stalls.
- Each checklist item has a best case and worst case; the engine rolls.
- Government friction is modelled as the **person behind the counter**, not just a fee table: rejected forms over a name mismatch, a surprise document requirement, variable speed. Same filing sails through for one player and bounces three times for another.
- There may be a choice to bribe. Treat it as satire of a broken process: costly and uncertain, never the optimal strategy.
- Imaginary walls live here too (e.g. payment gateway without incorporation is possible with just a PAN; the game may let the player believe otherwise).

---

## 5. Time

- Default real-time mode: **1 in-game day = 1 real hour**, and the clock runs while the player is away. Waiting is deliberate; it should be long enough that people feel like quitting, because that's where real projects die.
- Player can choose a faster time scale. Keep real-time as the default.
- **v1: fake it.** Compute elapsed in-game time from wall-clock timestamps when the player returns, and resolve pending events in a batch. No background workers or notifications yet.

---

## 6. NPCs

- Family, friends, cofounders, agencies, clerks, customers.
- The player populates the cast from their real life, **lazily**: the game asks only when it needs someone ("Who's the one person you'd ask for an intro?").
- NPCs nag, interrupt, sometimes help, sometimes fight. Each interaction is a roll. The same uncle can give you money one visit and a week of silence the next.
- Family pressure recurs (the same conversation at every visit, slightly sharper each time).

---

## 7. Content & research

The engine is easy. Content is the moat.

### Segments (start with 2–3)

- B2C
- B2B SaaS
- Technical / deep tech

Early-stage pain overlaps heavily across segments; divergence mostly comes later (distribution, pricing).

### Knowledge base per segment

- Real challenges, costs, timelines, compliance items, document lists.
- Sources: government sites (MCA fees, stamp duty, GST), and complaints people post about real processes (forums, Reddit, Twitter/X) for the human friction.
- Built by a research agent that decides its own sources.
- Every data point is tagged **real** or **invented**, with a source and date where real. Inventing gaps is fine; just mark them so they can be replaced.
- Version and date the knowledge base so the game can say "as of 2026" instead of silently going stale.

**v1:** hand-seed a small JSON knowledge base for the chapters above. The research agent is a later milestone.

---

## 8. Persistence across runs

- A player profile persists across runs: what they've learned, checklist items they've encountered.
- Repeat players get fast-forwarded through what they already know and slowed down on what they haven't seen.
- **v1: fake it.** Store a simple "learned items" list locally and pre-tick those items on the next run.

---

## 9. Data model sketch (starting point, refine freely)

```ts
type Run = {
  id: string;
  seed: number;
  startedAt: number;          // wall-clock
  timeScale: number;          // real ms per in-game day
  currentDay: number;
  status: "active" | "exited";
  exit?: { type: "sold" | "shutdown" | "walked_away"; day: number; money: number; note?: string };
};

type Player = {
  jobDescription: string;
  idea: string;
  age: number;
  maritalStatus: string;
  kids: string;               // none / some / expecting
  savings: number;
  monthlyObligations: { label: string; amount: number }[];
  technical: boolean;
  segment: "b2c" | "b2b_saas" | "deep_tech";
};

type World = {
  money: number;
  pinnedMetrics: string[];
  entity: { incorporated: boolean; type?: string; gstin?: boolean };
  capTable: { holder: string; percent: number; vesting?: string }[];
  build: { phase: number; hiddenIssues: string[]; knownBugs: string[]; agency?: string };
  users: number;
};

type RiskItem = {
  id: string;
  label: string;
  ticked: boolean;
  bestCase: string;
  worstCase: string;
  triggerProbabilityPerWeek: number;
  imaginary?: boolean;        // wall that isn't actually real
};

type NPC = {
  id: string;
  name: string;
  relation: string;           // father, friend, cofounder, agency, clerk...
  playerDescription: string;  // what the player told us
  disposition: number;        // drifts over time
};

type PendingEvent = {
  id: string;
  dueDay: number;
  source: string;             // risk item, ignored call, NPC, filing...
  description: string;
};

type MissedThing = { day: number; what: string; resurfaced: boolean };
```

LLM turn contract: send current state + player's free-text action + relevant knowledge base entries. Receive structured JSON: narration text, proposed state deltas, new to-do items, new pending events with probabilities. Engine validates, rolls, applies.

---

## 10. Explicitly out of scope for v1

- Native mobile apps.
- Background workers, push notifications.
- Accounts, server-side persistence, multiplayer.
- Automated research agent / scraping pipeline.
- Funding chapter.
- Real-world artifacts: uploading an actual logo, picking a real domain, submitting a real GitHub repo for technical players. Planned for a later version, and optional when it arrives.

---

## 11. Milestones

1. **Engine skeleton:** state model, seeded RNG, day loop, save/load, BYOK key entry, one LLM call per turn with structured output and validation.
2. **Session start flow:** idea → profile → technical/non-technical → segment classification → risk checklist generation.
3. **Chapter 0** playable end to end with NPC reactions and the cofounder/equity branch.
4. **Chapter 1** with the agency and self-build paths, hidden issues, and phased builds.
5. **Incorporation & compliance** events wired into the hidden checklist.
6. **Chapter 2** zero-to-one.
7. Notepad, balance checking/pinning, metric choice, time-scale choice, run exit and summary, cross-run learned items.

Build in that order. Each milestone should leave the game playable.
