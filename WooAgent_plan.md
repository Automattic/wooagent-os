# WooAgent OS — Prototype Plan (Apr 22 – May 22)

**Start:** Wednesday, April 22, 2026
**Deadline:** Friday, May 22, 2026 (internal hack/review day)
**Window:** 23 work days (weekends excluded)
**Team:** Elizabeth (@elizaan36, builder + designer) · Nevena (@nevenailic1013, designer in Claude.ai/design)

---

## Context

The WooAgent OS PRD and pitch describe a local-first, MCP-native agent OS for WooCommerce operators. The pitch names four design surfaces in priority order: onboarding, kanban, review/approval, ability browser. The pitch is signed by both Elizabeth and Nevena as designers pairing on the UX. For the May 22 hack day, the goal is a working prototype:

- **UI + real Go daemon + multiple personas**,
- Built **in-house** — Elizabeth does **all the coding**, Nevena designs in Claude.ai/design,
- Demoed to an **internal team audience**,
- Nevena's deliverables are **designs and flows** (annotated mockups, shared via Claude.ai/design links or exported frames); Elizabeth translates them into React using `@wordpress/components` / `@wordpress/ui`, with the **WordPress Design System MCP** (`@wordpress/design-system-mcp`) as the authoritative reference for component APIs and design tokens,
- **No shared code repo** between the two — Nevena works in Claude.ai/design, Elizabeth works in her local codebase,
- Covering **three design surfaces**: onboarding, kanban, review/approval.

Ability browser / agent roster is explicitly deferred. Three personas (not six) is the target.

---

## Scope decisions

### In scope

1. **Onboarding / first-run flow**
   - Install, connect to a test WooCommerce store via MCP, discover abilities, configure a model, deploy a small fleet.
   - Device-pair auth UX is designed; implementation falls back to WordPress Application Password if Companion Plugin work slips (documented fallback in the PRD).
2. **Kanban board** with real issue data
   - Backlog / Todo / In Progress / In Review / Done columns.
   - Issue cards showing persona avatar, title, status, priority.
   - Drag-drop between columns (at minimum between Todo → In Progress → In Review → Done).
3. **Review / approval interaction**
   - In Review column opens an issue detail with a before/after diff (rendered from MCP ability call parameters).
   - Approve button triggers the ability invocation; Reject stays in Review with a comment.
4. **Three personas running end-to-end**
   - **Marketing & SEO** (product-description rewrite — shows off a content-write diff).
   - **Pricing** (margin flag + proposed price change — shows off a structured-field diff).
   - **Sales Support** (order-status draft reply — shows off a customer-facing write).
   - These three cover the three most interesting diff shapes (prose, numbers, customer message).

### Out of scope for May 22

- Ability browser UI, agent roster UI, settings UI beyond a stub page.
- Remaining three personas (Inventory, Accounting, Reporting) — architecture supports them; they just aren't built.
- Webhook ingestion, local repo integration, read-only DB replica.
- OAuth 2.1 / host-specific auth.
- UI self-host / hosted build story — run the UI via `wooagent ui` locally for the demo.
- Multi-store, multi-operator, cost caps.

### Demo constraints

- **Test store:** a dev WooCommerce instance Elizabeth stands up on Day 1 (Wed Apr 22) of Phase 1 (WordPress.com sandbox, InstaWP, or local via `wp-env`). Real MCP Adapter installed. Seeded with ~20 sample products and ~10 sample orders.
- **Model:** Anthropic Claude (fastest to integrate, reliable tool use) + Ollama as a "look, it works locally too" fallback for the demo narrative.

---

## Track split

### Nevena's track — Design in Claude.ai/design, deliver mockups and flows

Nevena works entirely in Claude.ai/design and delivers **visual designs and flows** — no code. Each deliverable is a set of annotated mockups (one frame per screen state) plus a short flow description that Elizabeth can translate to React on her end. Designs target the WordPress design system (`@wordpress/components` / `@wordpress/ui`) so translation is mostly composition, not custom CSS. Nevena names components and tokens by their WordPress design-system names in her specs (e.g. "use a `Panel`," "set to `color-accent-primary`"); Elizabeth cross-checks those names against the WordPress Design System MCP (`wpds://components`, `wpds://design-tokens`) during implementation to catch anything that's been renamed, deprecated, or isn't actually stable.

Sequencing (compressed — designs need to run ahead of Elizabeth's build so she always has something to translate):
1. **Phase 1 — Apr 22–28 (front-loaded):** style tile + kanban card anatomy + full kanban board mockups + start of the review/approval screen (prose diff at minimum). Style tile and kanban card need to land in the first 1–2 days so Elizabeth can unblock.
2. **Phase 2 — Apr 29–May 5:** finish review/approval mockups (numeric + message diffs, approve/reject, run-log peek) + start onboarding flow mockups.
3. **Phase 3 — May 6–12:** finish onboarding flow (welcome → store URL → auth picker → pairing code → model provider → fleet deploy → done, plus error states) + run-log panel design + persona visual identities.
4. **Phase 4 — May 13–19 (testing phase):** no new surfaces. Ride along with Elizabeth's dogfooding — review real screens, flag fidelity drift, produce targeted redline/redesign sketches for the parts that are breaking under real use. **Stretch:** ability browser + agent roster sketches for v2.
5. **Phase 5 — May 20–22:** demo-asset polish only — slide-worthy screens, walkthrough assets, anything needed for the hack-day presentation itself.

### Elizabeth's track — Build everything

Elizabeth writes all code: Go daemon, React UI, MCP integration, model adapters, the works. She implements Nevena's designs in React as they're delivered, using `@wordpress/components` and `@wordpress/ui` primitives so the fidelity stays close to Nevena's intent without custom CSS work. She queries the **WordPress Design System MCP** (`@wordpress/design-system-mcp`) from her local Claude Code session to look up component props, verify a component is stable before using it, and confirm design-token values (colors, spacing, typography) — so the translation from Nevena's specs to shipped React has one authoritative reference, not guesswork.

1. **Phase 1 — Apr 22–28 (compressed — foundations + first persona end-to-end):** React app skeleton, Go daemon skeleton, test Woo store, API contract doc, ADK Go integration, MCP client with ability discovery, Marketing agent producing real issues, kanban board React from Nevena's Phase 1 designs. The hard compression phase — many small deliverables, long days, no polish.
2. **Phase 2 — Apr 29–May 5:** Pricing and Sales Support agents, propose-mode diff generation, approval backend (Approve actually invokes via MCP), review/approval screen React from Nevena's Phase 1/Phase 2 designs.
3. **Phase 3 — May 6–12:** onboarding backend (auth, provider config, fleet bootstrap), onboarding React from Nevena's Phase 2/Phase 3 designs, device-pair UX (or App Password fallback), starter-issue seeding, run-log panel.
4. **Phase 4 — May 13–19 (testing + iterating):** dedicated phase to run the app against the real test store, find bugs, tune agent prompts, fix UX friction, refine diff rendering, close fidelity gaps Nevena flags. No new surfaces, no new personas.
5. **Phase 5 — May 20–22:** final polish, demo script, rehearse, hack day.

### Handoff mechanics

- **No shared repo.** Nevena works in Claude.ai/design; Elizabeth works in her local codebase. Designs flow one direction: Nevena → Elizabeth.
- **Design delivery format:** Claude.ai/design share link + a short written spec (what states exist, what each button does, what triggers transitions). A Loom walkthrough is optional but helpful for complex flows like onboarding.
- **API contract doc** lands by Thu Apr 23 (Day 2 of Phase 1) — so Nevena's designs reflect real data shapes (issue fields, persona names, diff structure).
- **Cadence:** daily async checkpoint (2–3 sentences each on what shipped and what's blocked). Twice-weekly 30-min sync call. Nevena delivers new designs on Mondays and Thursdays to give Elizabeth predictable translation windows.
- **Design review loop:** when Elizabeth finishes a screen in React, she shares a screenshot or short screen recording. Nevena flags fidelity issues; Elizabeth fixes or defers. Bias toward "close enough" for Phase 4 testing scope.
- **Fidelity floor:** Elizabeth uses `@wordpress/components` / `@wordpress/ui` primitives wherever possible so translation is mostly about composition, spacing, and copy — not pixel-pushing.
- **Canonical design-system reference:** the **WordPress Design System MCP** (`@wordpress/design-system-mcp`, from [Automattic/wpds-mcp](https://github.com/Automattic/wpds-mcp)) is the single source of truth for stable component availability, props, and design tokens. Elizabeth queries it via Claude Code (`wpds://components`, `wpds://components/{name}`, `wpds://design-tokens`) during the build phase instead of guessing from TypeScript types or skimming Gutenberg docs. When Nevena references a component or token by name in her specs (e.g. "use a `Panel`," "color-accent-primary"), Elizabeth verifies via the MCP before composing. Treat the MCP's `wpds://components` list as the source-of-truth vocabulary both designers share.

---

## Phase-by-phase schedule

> Each phase spans 5 work days (Mon–Fri equivalent) except Phase 5 which is 3 days (May 20–22, Wed–Fri). Phase 1 straddles a weekend — it starts Wed Apr 22 and runs through Tue Apr 28.

### Phase 1 — Apr 22–28 (Wed → Tue, 5 work days): Foundations + first persona end-to-end (compressed)

Aggressive — this phase combines what was originally two phases of work into one. Goal: by Tue Apr 28, a real Marketing agent is producing real issues on a kanban board backed by a real daemon against a real test store. Everything is rough; that's fine.

**Both (Day 1 — Wed Apr 22)**
- Agree user flows for the 3 surfaces on paper (single session, 60 min).
- Agree design-delivery conventions: share-link format, spec template, iteration cadence.

**Elizabeth (Wed Apr 22 → Tue Apr 28)**
- **Wed–Thu (Apr 22–23):** React app skeleton (Vite + `@wordpress/components` + `@wordpress/ui`), WordPress Design System MCP (`@wordpress/design-system-mcp`) registered in Claude Code so component/token lookups work from day 1, Go daemon skeleton (single binary, REST API, SQLite), test WooCommerce store stood up + MCP Adapter installed, API contract doc v1.
- **Fri (Apr 24):** ADK Go spike — get *any* adk-go agent running with Anthropic + a stub tool.
- **Mon (Apr 27):** MCP client — connect to test store, discover abilities, cache schemas.
- **Mon–Tue (Apr 27–28):** Marketing agent persona — reads products, generates a rewrite proposal, lands in `In Review` via `GET /issues`, `POST /issues/:id/approve`, `POST /issues/:id/reject`. Build the kanban board React from Nevena's designs as they land.

**Nevena (Wed Apr 22 → Tue Apr 28)**
- **Wed–Thu (Apr 22–23):** lightweight style tile (colors, type, spacing — persona avatars can slip to Phase 2) so Elizabeth isn't blocked on visual tokens.
- **Fri–Mon (Apr 24, 27):** kanban card anatomy + board layout wireframes delivered by end of Mon so Elizabeth can start building Tuesday.
- **Tue (Apr 28):** full kanban board mockups (all 5 columns, card states, interaction spec) + first pass on the review/approval prose diff layout.

**Phase 1 exit criteria**
- Marketing agent produces a real issue against the test store.
- The issue appears on a kanban board at rough fidelity to Nevena's designs.
- Daemon + React app + test store + MCP are all working end-to-end.

### Phase 2 — Apr 29 – May 5 (Wed → Tue, 5 work days): Multi-persona + review

**Elizabeth**
- Ship **Pricing agent** (structured numeric diff) and **Sales Support agent** (customer-reply draft).
- Propose-mode for all three (no ability writes until approved).
- Wire the `Approve` button to actually invoke the staged ability via MCP.
- Review log: store every model call, every ability call, every state change.
- Build the **review/approval screen** React from Nevena's Phase 1/Phase 2 designs (prose diff first; numeric and message as they land).

**Nevena**
- Finish **review/approval screen** mockups covering all three diff shapes (prose, numeric, message), including approve/reject flow and run-log peek.
- Start **onboarding flow** mockups (welcome → store URL → auth picker → pairing code → model provider → fleet deploy → done).

**Phase 2 exit criteria**
- All three personas generate real issues.
- Operator can approve a Marketing rewrite and the product description actually updates on the test store.
- Review screen renders all three diff shapes at rough fidelity to Nevena's designs.

### Phase 3 — May 6–12 (Wed → Tue, 5 work days): Onboarding + integration

**Elizabeth**
- Implement onboarding backend (connection config, device-pair or App-Password auth, provider config, fleet bootstrap).
- Build the **onboarding flow** React from Nevena's Phase 2/Phase 3 designs (every screen, transitions, error states).
- Seed logic: on first run, Chief of Staff stub produces 2–3 starter issues per persona so the kanban has content for the demo.
- Build the run-log panel React once Nevena's design lands.

**Nevena**
- Finish **onboarding flow** mockups including error states (store unreachable, pairing timed out, model key invalid).
- Design the **run-log panel** (quick view of "what did the agent actually do").
- Finalize persona visual identities (avatars, color accents).

**Phase 3 exit criteria**
- End-to-end first-run walkthrough works on a clean machine: install → pair → configure → deploy → see issues on board → approve one → verify change on store.
- All three design surfaces implemented in React at rough fidelity to Nevena's designs.

### Phase 4 — May 13–19 (Wed → Tue, 5 work days): Testing + iterating (dedicated)

No new features. The whole phase is about running the prototype against the test store and fixing what's broken.

**Elizabeth**
- Daily: run through the full operator flow (install → pair → kanban → approve 1 issue of each type) and log every friction point, bug, and odd agent behavior.
- Fix the highest-impact bugs first; defer cosmetic issues to Phase 5.
- Tune agent prompts based on what the agents actually produce — the hardest iteration is usually getting Marketing to write on-brand copy and Pricing to propose sane numbers.
- Refine diff rendering based on real content (edge cases: very long product descriptions, zero-margin items, order IDs that don't exist).

**Nevena**
- Ride along: watch Elizabeth's daily walkthroughs or screenshots. Flag fidelity drift, clunky transitions, confusing microcopy.
- Produce targeted redesigns for whatever is breaking under real use (likely: an empty state, a loading indicator, an error message).
- **Stretch:** start sketches of deferred surfaces (ability browser, agent roster) for v2.

**Phase 4 exit criteria**
- Elizabeth has completed at least three full operator walkthroughs on her machine without crashes.
- The top 5 bugs or friction points from Phase 3 are resolved.
- Agent output quality has measurably improved (e.g., Pricing proposals no longer suggest negative prices; Marketing rewrites stay under a sensible length).

### Phase 5 — May 20–22 (Wed → Fri, 3 work days): Polish + demo

**Both**
- **Wed May 20:** feature freeze. Final polish, demo script, screenshot and video asset prep.
- **Thu May 21:** dress rehearsal — full walkthrough on the demo laptop, timing it, fixing any demo-breakers.
- **Fri May 22:** hack/review day.

---

## Demo readiness criteria (May 22 morning)

- [ ] Fresh-machine install works: `wooagent init` → `wooagent run` → `wooagent ui` opens the app.
- [ ] Onboarding completes in under 10 minutes on a fresh machine (matches the pitch's success metric).
- [ ] Three personas generate real issues against the test store within the first session.
- [ ] Operator can approve one issue of each type (prose, numeric, message) and the change lands on the store.
- [ ] Run-log panel shows a clean trace for any approved issue.
- [ ] No crashes on the demo laptop during a 10-minute walkthrough.

---

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| **Phase 1 is deliberately over-scoped.** | This is the biggest risk. If Phase 1 slips, Phase 4 (testing) is the absorber — not Phase 5 (demo). Hard stops: by end of Fri Apr 24, ADK Go must be working with a stub tool; by end of Mon Apr 27, MCP client must be talking to the test store. If either misses, cut the Marketing agent's scope to a hard-coded proposal (skip the LLM) so kanban can still demo a real issue flow. |
| **Scope is ambitious, especially with Elizabeth doing all the coding.** | Three personas is the hard floor — drop to two if Phase 2 slips. Drop Ollama fallback from the demo if time is tight; ship only Anthropic. Consider pre-configuring the app for the demo instead of running onboarding live (preserves the surface in designs but saves build time). |
| **MCP Adapter auth (device-pair) is new work in the PRD.** | Ship App Password auth as the v0 fallback. Nevena designs the device-pair UX; Elizabeth only implements it if Phase 3 has slack. |
| **Design-to-code translation fidelity loss.** | Elizabeth uses `@wordpress/components` / `@wordpress/ui` primitives everywhere so translation is mostly composition, not pixel-pushing. The WordPress Design System MCP (`@wordpress/design-system-mcp`) gives her authoritative props and tokens during build, so she isn't improvising API shapes from Nevena's screenshots. Nevena reviews screenshots twice weekly; fidelity fixes are scoped as "close enough for testing." Don't let Phase 5 (3 days only) become a pixel-polish vortex. |
| **Design-ahead-of-code lag.** | Nevena's Phase 1 is heavily front-loaded so Elizabeth is never blocked: lightweight style tile by end of Thu Apr 23, kanban card + layout by end of Mon Apr 27, full kanban mockups by Tue Apr 28. If Nevena slips on a later surface, Elizabeth defers that screen and uses a placeholder for the demo. |
| **ADK Go unfamiliarity for Elizabeth.** | Spike on Fri Apr 24 (Day 3 of Phase 1): get *any* ADK Go agent running with Anthropic + a stub tool before committing to MCP integration the following Monday. |
| **Test Woo store setup blocks everything else.** | Day 1 (Wed Apr 22) priority. InstaWP or WordPress.com sandbox are fastest. Fallback: local `wp-env` on Elizabeth's machine. |
| **Drag-drop interactions are a rabbit hole.** | First, check the WordPress Design System MCP (`wpds://components`) for a drag-drop or sortable primitive — the WP ecosystem has iterated on this for Gutenberg. If nothing suitable exists, use `@dnd-kit/core` wrapped in WP-styled components. Don't over-invest. |
| **Real LLM cost during dogfooding.** | Set a per-day dollar cap in the model adapter from day 1; cache ability discovery. Use Ollama as a cost-free fallback for persona development when Elizabeth doesn't need frontier-quality output. |

---

## Deferred — post-May 22

The PRD roadmap v0.1 items not shipping by May 22:
- Ability browser + agent roster UI
- Inventory / Accounting / Reporting personas
- OAuth 2.1 + host-specific auth paths
- Full Companion Plugin features (staged-changes, guardrails, issue-status sync)
- Hosted UI build at `ui.wooagent.dev`
- Multi-store, multi-operator
- Skill pack installer, cost caps, offline mode

These stay on the roadmap and become post-demo priorities.

---

## What would cut the timeline further (if we wanted to)

The compressed schedule already moves everything left to buy the Phase 4 testing block. If we wanted to cut further:

- **Drop one persona** (Sales Support first — smallest diff-shape value). Frees ~2 days of Phase 2, but reduces the "agent fleet" feel of the demo.
- **Pre-configure the app for the demo** (skip onboarding live). Saves Phase 3's onboarding React build, meaning Phase 3 becomes a second testing/polish phase. The onboarding surface is still *designed* (Nevena's work doesn't change) — it just isn't wired up for May 22.
- **Ship only Anthropic** (drop the Ollama "look, local too" narrative). Saves ~half a day of model-adapter work.

We do not recommend all three together; each is a separate lever available if a specific phase slips.

---

## Verification — what "done" looks like on May 22

End-to-end walkthrough on a clean laptop:

1. Run `curl -fsSL https://wooagent.dev/install.sh | sh` (or pre-built `.dmg`).
2. Run `wooagent init` — pair with the test store via the device-pair flow (or App Password fallback), configure Anthropic, deploy the three-persona fleet.
3. Run `wooagent run` + `wooagent ui` — kanban loads with 6–9 starter issues (2–3 per persona).
4. Click into a Marketing issue in `In Review` — see before/after product-description diff — approve — watch the product update on the store in a second browser tab.
5. Repeat for a Pricing issue and a Sales Support issue.
6. Open the run log for one approved issue — see the model calls and ability invocations listed cleanly.

If those six steps complete on the demo laptop, we're done.
