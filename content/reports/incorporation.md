# Entity & Incorporation agent — report (2026-09-23)

## Files written
- `content/common/facts.incorporation.jsonl` (24 records)
- `content/common/risks.incorporation.jsonl` (5 records)
- `content/common/friction.incorporation.jsonl` (4 records)
- `content/reports/incorporation.md` (this file)

No segment-specific facts were written: incorporation mechanics (entity choice, SPICe+, post-incorporation deadlines, closure) do not vary by b2b_saas/b2c/deep_tech, so everything went into `common`.

## What was found (key facts and numbers)
- **Fee waiver + stamp duty gap**: MCA charges zero registration fee on SPICe+ Part B up to ₹15 lakh authorised capital, but state stamp duty on the MoA/AoA is separate and swings wildly — roughly ₹135–270 in Haryana, ~₹1,700 in Delhi, ~₹4,100 in Maharashtra, and ~₹6,020 in Karnataka at ₹10 lakh capital (Karnataka's AoA duty rose ~10x in a 2024 revision). This is the kind of gotcha a founder picking a "cheap" state doesn't expect.
- **Name reservation mechanics**: SPICe+ Part A costs ₹1,000, allows two proposed names, reserves an approved name for 20 days (extendable to 60), and a "resubmission required" flag gives roughly a 15-day window to fix and resubmit on the same fee — a hard rejection means paying again. Rejections cluster into three CRC buckets under Companies (Incorporation) Rules 2014: name-similarity (rule 8), "undesirable" names incl. unconsented trademarks (rule 8A), and restricted words needing Central Government approval (rule 8B).
- **AGILE-PRO-S bundle has conditions, not automatics**: GSTIN only issues if the registered office matches the correspondence address given; EPFO/ESIC are automatic for every company; profession tax auto-applies only in Karnataka/Maharashtra/West Bengal; Shops & Establishment only for Mumbai/Delhi offices; bank account choice is constrained by which partner bank serves that pincode. Founders elsewhere still do GST/bank KYC by hand later.
- **Post-incorporation deadlines have real teeth**: INC-20A (180 days) — ₹50,000 on the company plus ₹1,000/day per officer capped at ₹1 lakh, enforced even for a one-day delay; share certificates (2 months, s.56(4)) — up to ₹50,000/₹25,000 penalty; DIR-3 KYC (30 Sept annually) — flat ₹5,000 reactivation fee and DIN deactivation from 1 October.
- **Closure is not free**: fast-track strike-off (STK-2) costs a flat ₹5,000 government fee, ₹8,000–25,000 in professional fees for a compliant company (₹25,000–75,000+ if filings are behind), and takes 3–6 months end to end because of the ROC public-notice period. Eligibility itself requires nil assets/liabilities, all annual filings caught up, GST cancelled, and DIR-3 KYC valid — you can't skip paperwork on the way out either.
- **Entity comparison**: sole proprietorship (no registration, ~₹1,000–5,000 in supporting registrations) and unregistered partnership (~₹1,000–5,000, 1–3 weeks) are cheap to start but an unregistered partnership loses standing to sue under Partnership Act s.69; LLP needs 2 designated partners and costs an estimated ₹5,000–12,000 over 7–15 days; OPC needs one shareholder + a mandatory nominee, costs ₹10,000–25,000 over 5–8 days, and its old mandatory-conversion-to-Pvt-Ltd threshold was removed in 2021.
- **DIY vs CA vs platform**: raw MCA filing avoids ~₹3,000–25,000 in CA fees but exposes the founder to name/document mistakes; assisted total for a simple 2-director Pvt Ltd runs ₹12,000–30,000; online platforms' headline prices (₹1,999–7,999 seen in listings) typically exclude government fees, stamp duty and DSC and upsell them back in.

## What was guessed or estimated (and what would replace it)
- Exact MCA fee table above ₹15 lakh authorised capital and the precise "zero fee" statutory citation — no practitioner page reproduced the actual Companies (Registration Offices and Fees) Rules, 2014 fee schedule; the official MCA fee rules PDF would replace this.
- Sole proprietorship / partnership total cost ranges are aggregated estimates across several practitioner blogs, not a single authoritative figure.
- LLP total cost range and OPC-vs-Pvt-Ltd cost comparisons are estimated by combining platform headline prices with what those prices typically exclude; a real quote comparison across 3–5 CA firms and platforms would sharpen this.
- `incorporation.common_mistakes_by_path` is explicitly `invented` — a synthesis of recurring practitioner-blog and forum themes (boilerplate MoA objects, no vesting, padded authorised capital, DIY name-search failures) rather than a measured mistake-rate; real founder complaint mining (the friction-miner agent's job) would ground this properly.
- `incorporation.statutory_registers_requirement` is framed generally from the Companies Act sections (88, 170, 189) rather than fetched from a specific current practitioner explainer this pass.
- LLP annual-filing due dates (Form 11 ~30 May, Form 8 ~30 Oct) and the "additional fee accumulates" consequence are stated from general LLP Act knowledge; the exact current additional-fee-per-day schedule was not independently confirmed this pass.
- Two `friction.*` records (`din_video_kyc_holdup`, `agile_pro_gst_pan_name_mismatch`) are marked `invented` — plausible patterns inferred from adjacent, sourced friction (DSC video-KYC issues, DIR-3 KYC name-mismatch rejections) but not tied to a specific incorporation complaint found this pass. The dedicated friction-miner agent should verify or replace these against real complaint threads.

## Supersedes
None. This agent's records are new ids (`incorporation.*`) that complement, rather than replace, the existing hand-written records already in `content/common/facts.jsonl` and `content/common/risks.jsonl` (e.g. `pvt_ltd_spice`, `name_rejection`, `inc_20a`, `first_auditor`, `annual_compliance_cost`, `inc20a_missed`). Two new facts (`incorporation.inc20a_penalty_detail`, and the ADT-1/first-auditor angle folded into `incorporation.pvt_ltd_profile`/`incorporation.dsc_cost_validity` context) add exact penalty figures and mechanics that the existing brief-level records don't state; they were kept as separate records per the brief's instruction not to edit files this agent didn't create.

## Sources that failed to load
- `https://khannaandassociates.com/blog/mca-filing-fees-2026/` and `https://khannaandassociates.com/blog/fast-track-strike-off-process-india-2026/` — blocked by robots.txt.
- `https://vakilsearch.com/article/company-registration-diy-vs-platform/` and `https://www.patronaccounting.com/llp-incorporation` (second fetch attempt) — returned 403.
- `https://www.mca.gov.in/bin/dms/getdocument?...` (SPICe+ instruction kit PDF) — returned 403 on direct fetch; substituted with a practitioner explainer (cspratik) that appears to closely paraphrase the same official instruction kit for Part A fee/validity details.
- `https://khannaandassociates.com/blog/free-director-identification-numberdin/` — blocked by robots.txt; did not confirm the exact free-DIN-at-incorporation limit (3 vs 5 directors), so `incorporation.din_at_incorporation` describes the mechanism without citing a specific numeric cap.

## Note on an existing (pre-existing) id collision
While checking for id uniqueness I found that `content/common/facts.jsonl` and `content/common/risks.jsonl` (both seed files that predate this run, not written by this agent) each contain a record with the same id `first_auditor`. The validator does not currently flag this. I did not touch either file per the "never edit files you did not create" rule; flagging here for the reviewer agent to dedupe.

## Open questions for the game designer
- Should the "imaginary wall" mechanic also cover the belief that closing a dead company is free/instant? `incorporation.stk2_strike_off_cost_timeline` and the `incorporation_dormant_company_never_closed` risk seem like good material for a late-game "founder gives up" scene, but no `wall` tag was applied since the brief scopes `wall` to payments/blocker facts specifically — confirm whether closure-cost surprise should also be flagged as a wall.
- State stamp duty numbers are volatile and state-specific (Karnataka's 10x jump in 2024 shows how fast they move); worth flagging for the reviewer agent to re-verify against each state's stamp act rather than aggregator blogs before shipping to players as "current."
- `incorporation.common_mistakes_by_path` is fully invented pattern-matching; recommend either the friction-miner or reviewer agent replace it with something forum-sourced, or downgrade its confidence further in the final bundle.
