# WooAgent OS — May 22 demo

## Summary

WooAgent OS is an operator-first interface for running AI agents against a real WooCommerce store. Three personas (Marketing, Pricing, Sales Support) produce proposals; the operator reviews and approves them on a kanban board.

Three design surfaces carry the experience end-to-end: onboarding, kanban, and review / approval. Target is a demo-ready build on 2026-05-22 with rough fidelity to the final design across all three surfaces.

## Problem Statement

Running a WooCommerce store is a second job inside the operator's actual job — writing product copy, watching competitor pricing, tracking stock, answering customer questions, reconciling payouts — most of it repetitive and most of it the kind of work AI is good at. The current answer, "open a chat window and write a long prompt," has no memory, no coordination, no audit trail, and no place on the store where the work actually lives.

**Qualitative evidence**
- HVM call transcripts (Q4 2025, Q1 2026) surface operational tax as a primary churn driver — Xero Shoes left Woo for Shopify specifically because "anyone types a natural-language question and gets the answer in 2 minutes," while on Woo they paid for Metorik plus specialists.
- Field feedback (Mike Monan): Stripe's dashboard advantage exists because "businesses have built their reporting practices and compliance workflows around it" — operators want systems, not chat windows.

**Quantitative evidence**
- $1M+ active WooCommerce stores grew +38% YoY (5,115 → 7,041) while $1M+ merchants on WooPayments fell -11.7% — platform is winning HVMs but tooling is losing them.
- 19% of churned HVMs go to Shopify, primarily for operational simplicity, not payments.

**Reference users consulted**
- Xero Shoes (churned HVM testimony)
- ScandiKitchen (~$3M TPV, retained via CS relationship)
- Straight To Hell Apparel ($4M GMV, discoverability pattern)
- Q1 2026 Tier 1 support ticket sample (PAYOPS-87, WOOPMNT-5386)

## Target audience

**Primary** — Solo operators and small teams running $250K–$1M+ GMV WooCommerce stores. Multi-channel (DTC + B2B + wholesale), mature operations, often agency-supported, high operational complexity. They are already using AI ad-hoc (ChatGPT, Claude) but without a system around it.

**Key traits**
- Checks in for ~15 minutes in the morning; not monitoring a dashboard all day.
- Final decision-maker on anything that writes to the store — rubber-stamping or blanket-rejecting are both failure modes.
- Comfortable with familiar patterns (kanban from Linear / Jira / Trello) but skeptical of AI agents they can't see or audit.
- Expects local-first (data stays on their machine) and model-agnostic (they bring their own API key or local model).

**Not for this phase**
- Enterprise multi-store / multi-operator teams — coordination and permissions are a v2 problem.
- Hosted-cloud users — local-first is a deliberate constraint and v1 is localhost-only.
- Non-Woo stores — the Abilities API bet is Woo-specific by design.
- Agency operators managing fleets of client stores — same coordination gap as enterprise.

## User Stories

- As an operator, I want to see what my agents did overnight **without hunting through a chat log**, so my morning check-in actually takes 15 minutes.
- As an operator, I want to approve a product rewrite with a clear before/after diff, so I'm not rubber-stamping or rejecting everything out of caution.
- As an operator, I want to connect a store and see a real agent-produced issue in **under 10 minutes**, so I know this is real before I invest in setup.
- As an operator, I want each agent to have a distinct mandate and identity, so I develop an intuition for who does what.
- As an operator, I want every model call, ability invocation, and change logged, so I can answer "what exactly did the agent do?" in one click.
- As an operator, I want to approve a price change as easily as a product description, so the interaction feels consistent across very different kinds of work.

## Proposed Approach

A kanban command center where every agent action is a card. Operators see what's queued, running, waiting, or done at a glance, and every write-to-store is gated by a human-readable diff. The product is local-first, model-agnostic, and talks to the store through the new WordPress Abilities API + MCP Adapter.

**Feature 1 — First-run onboarding**
Connect store → discover MCP abilities across installed plugins → configure a model provider → deploy the default agent fleet. Goal: first agent-produced issue visible under 10 minutes on a fresh machine.

**Feature 2 — Kanban board (primary surface)**
Five columns (Backlog → Todo → In Progress → In Review → Done). Cards are agent actions, not human tasks. Information hierarchy tuned for glance-reading: what's the agent doing now, what needs my attention, what just happened.

**Feature 3 — Review / approval**
The most critical interaction. Before/after diff across three shapes: prose (Marketing rewrites), numeric (Pricing changes), and message (Sales Support replies). Fast approve/reject, run-log peek on demand. Nothing writes to the store without passing through this screen.

**Feature 4 — Agent roster (full fleet visible from day one)**
The roster surfaces all six operator personas + Chief of Staff from first run, so the demo conveys the intended scope of the system. Three personas (Marketing, Pricing, Sales Support) are fully functional with tune / disable controls; three (Inventory, Accounting, Reporting) ship UI-visible but inert with a clear "coming soon" treatment that reads as deliberate phasing, not unfinished work. Chief of Staff is scaffolded — it produces the first-run store-profile issue only. Ability browser is stretch for this release.

## Out of scope

- Ability browser UI (agent roster **is** in scope — the full fleet of six operator personas + Chief of Staff renders from first run so the demo conveys the intended scale)
- **Functional** Inventory, Accounting, and Reporting personas — they ship UI-visible but inert with a "coming soon" state and demo-only seeded sample issues; no real MCP writes in this release
- OAuth 2.1 and host-specific auth paths
- Full Companion Plugin (staged-changes, guardrails, issue-status sync) — v0.1 ships CRUD only
- Hosted UI at `ui.wooagent.dev`
- Multi-store, multi-operator, agency-fleet workflows
- Skill-pack installer, cost caps, offline mode
- v2 GEPA central prompt-tuning pipeline (scaffolding only in v1)

## Discovery Plan

**Assumption:** Kanban as a mental model translates cleanly from human tasks to agent actions.
*Validation:* User-flow agreement session Wed Apr 22 with Nevena on interactive prototypes; Phase 4 full operator walkthroughs against real content.

**Assumption:** Operators will approve 70%+ of agent-proposed writes at steady state once they trust the diff surface.
*Validation:* Phase 4 testing against real products on the live test store; track approve/reject ratio by diff shape.

**Assumption:** A non-technical operator can complete first-run and see an agent-produced issue in under 10 minutes.
*Validation:* Fresh-machine install walkthroughs starting Phase 3; three full runs by end of Phase 4.

**Assumption:** A prose before/after diff is more trustworthy than a chat-style summary.
*Validation:* Phase 2 review/approval mockup reviews; compare prose-diff against an A/B chat-summary variant in crit.

**Assumption:** Three persona identities are enough differentiation without feeling like mascots.
*Validation:* Design crits during Phase 3 persona-identity work; gut-check with operators in Phase 4.

**Assumption:** Operators will accept local-first (localhost:7777 UI) instead of a hosted app.
*Validation:* Already validated by the PRD stance; revisit only if fresh-machine walkthroughs surface friction.

## Competitive overview

| Competitor | Current approach | Gap / opportunity |
|---|---|---|
| **Shopify Magic / Sidekick** | Conversational AI embedded in admin; single-shot prompts, no approval surface | No audit trail, no multi-agent coordination, no work-tracking — operators can't see what it's done or queue work for later |
| **Shopify (platform)** | Operational simplicity is the draw; 19% of churned WooPayments HVMs land here | Closed ecosystem, no local-first, no model-choice — WooAgent leans into openness as the counter |
| **Woo Assist** | Reactive chat assistant inside WooCommerce admin | One-shot prompts, no memory across sessions, no kanban surface, no propose/approve separation |
| **ChatGPT / Claude (direct chat)** | What many operators use today | No store connection, no persistent state, no audit, no agent specialization — the "system around the AI" gap |
| **Zapier / n8n with AI steps** | DIY agent plumbing via automations | High setup cost, no operator-review step, no Woo-native ability surface |

## Success and Measurement

**Business objective:** Reduce the operational tax that pushes HVMs toward Shopify, and establish an opinionated AI-operations layer on top of the WordPress Abilities API before a competitor owns the category.

**Success metrics**
- **Time-to-first-issue:** under 10 minutes from install to first agent-produced issue on a fresh machine *(baseline: no comparable product exists today)*.
- **Approval rate:** operators approve 70%+ of agent-proposed writes at steady state.
- **30-day retention:** operators running at least one agent daily after a month.
- **Ability coverage:** agents draw on abilities from 3+ plugins in the first session *(current ecosystem: only 5 core/Jetpack abilities registered on WC 10.7.0; Companion Plugin adds 7 Woo-specific abilities in v0.1).*

**Definition of done (2026-05-22 morning)**
- Fresh-machine install: `wooagent init` → `wooagent run` → `wooagent ui` opens clean.
- Onboarding completes in under 10 minutes on a fresh machine.
- All three personas generate real issues against the test store within the first session.
- Operator can approve one issue of each diff type (prose, numeric, message) and the change lands on the store.
- Run-log panel shows a clean trace for any approved issue.
- No crashes during a 10-minute walkthrough.
- All three design surfaces implemented at rough fidelity to Nevena's designs.

*Data collection: the daemon writes every model call, ability invocation, and state change to local SQLite via the v2 GEPA `turn_events` schema — scaffolding is in place so post-launch central ingest requires no schema change.*

## Risks and Open Questions

**Value risk — Will operators trust the diff enough to approve?**
This is the central product bet. *Address:* Phase 2 review/approval mockup pass is the crunch point; Phase 4 is a dedicated fidelity + trust testing week.

**Usability risk — Does the kanban metaphor hold when cards are agent actions, not human tasks?**
Cards don't move themselves in human kanban; here they do. *Address:* User-flow agreement session Wed Apr 22; Phase 4 walkthroughs watch specifically for "I lost track of where this card came from" moments.

**Usability risk — 10-minute onboarding on a fresh machine is aggressive.**
Includes store connection, MCP discovery, model config, and fleet deploy. *Address:* Phase 3 builds error-state designs early (store unreachable, pairing timed out, invalid model key) rather than leaving them for Phase 4.

**Feasibility risk — ADK Go is unfamiliar territory.**
The agent runtime is the pure-Go dependency we're least sure of. *Address:* Fri Apr 24 spike with a hard stop — a working agent + Anthropic + stub tool by EOD, or we re-scope.

**Feasibility risk — WooCommerce has not registered a `woocommerce/*` ability namespace on WC 10.7.0.**
Discovered during the Apr 21 ecosystem probe. *Address already underway:* Companion Plugin v0.1 ships 7 `wooagent-*` CRUD abilities; they deprecate gracefully when WC ships native ones.

**Business viability risk — How does open-source, local-first fit Woo's commercial strategy?**
We're building the "opinionated layer on top" of infrastructure Automattic already shipped (Abilities API + MCP Adapter), but the GTM story for a local-first, model-agnostic agent OS inside Woo needs alignment. *Address:* Flag for Woo leadership review before Phase 5 demo.

**Open question — Which diff shape is hardest to make trustworthy?**
Prose (subjective), numeric (stakes), or message (tone)? *Will know by:* end of Phase 2 review-mockup pass.

**Open question — Do persona identities need visual/personality treatment at launch, or can they stay as names + mandates?**
*Will know by:* Phase 3 persona-identity design work — if it delays, names-only is the fallback and Phase 4 can add identity if time allows.

**Open question — Share-link / spec-handoff convention between Nevena and Elizabeth.**
Resolves Wed Apr 22 in the design-delivery conventions session.

---

**Attachments to drag in** (`April 22 - wooagent-mockups/_diagrams/`): `d1_overview.png`, `d2_firstrun.png`, `d3_daily.png`, `d4_approval.png`, `d5_tuning.png`. Interactive HTML prototypes live alongside (`01-onboarding.html` through `05-user-flow.html`).
