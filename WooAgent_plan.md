# WooAgent OS — Prototype Plan (Apr 22 – May 22)

**Start:** Wednesday, April 22, 2026 (Pre-Phase 1 foundation work landed Tue Apr 21)
**Deadline:** Friday, May 22, 2026 (internal hack/review day)
**Window:** 23 work days (weekends excluded)
**Team:** Elizabeth (@elizaan36, builder + designer) · Nevena (@nevenailic1013, designer in Claude.ai/design)

> Live status: `progress.md` is the continuously-updated actuals log; this file is the canonical scope + dates.

---

## Context

The WooAgent OS PRD and pitch describe a local-first, MCP-native agent OS for WooCommerce operators. The pitch names four design surfaces in priority order: onboarding, kanban, review/approval, ability browser. The pitch is signed by both Elizabeth and Nevena as designers pairing on the UX. For the May 22 hack day, the goal is a working prototype:

- **UI + real Go daemon + multiple personas**,
- Built **in-house** — Elizabeth does **all the coding**, Nevena designs in Claude.ai/design,
- Demoed to an **internal team audience**,
- Nevena's deliverables are **designs and flows** (annotated mockups, shared via Claude.ai/design links or exported frames); Elizabeth translates them into React using `@wordpress/components` / `@wordpress/ui`, with the **WordPress Design System MCP** (`@wordpress/design-system-mcp`) as the authoritative reference for component APIs and design tokens,
- **No shared code repo** between the two — Nevena works in Claude.ai/design, Elizabeth works in her local codebase,
- Covering **three design surfaces**: onboarding, kanban, review/approval.

Ability browser is explicitly deferred. The **agent roster is visible from day one and shows all six operator personas plus Chief of Staff** so the demo conveys the full fleet — functional readiness is phased: three personas run end-to-end, the other three are UI-visible but inert.

---

## Scope decisions

### In scope

1. **Onboarding / first-run flow**
   - Install, connect to a test WooCommerce store via MCP, discover abilities, configure a model, deploy a small fleet.
   - Device-pair auth UX is designed; implementation uses WordPress Application Password for Phase 1 (Companion Plugin v0.1 shipped device-pair abilities as 501 stubs — native pairing flow ships in Companion Plugin v0.2, likely post-May 22).
2. **Kanban board** with real issue data
   - Backlog / Todo / In Progress / In Review / Done columns.
   - Issue cards showing persona avatar, title, status, priority.
   - Drag-drop between columns (at minimum between Todo → In Progress → In Review → Done).
3. **Review / approval interaction**
   - In Review column opens an issue detail with a before/after diff (rendered from MCP ability call parameters).
   - Approve button triggers the ability invocation; Reject stays in Review with a comment.
4. **Full fleet in the UI; three personas running end-to-end**
   - **Agent roster shows all six operator personas + Chief of Staff** from first run, so the demo conveys the intended scope of the system. This includes the onboarding "default fleet deployed" summary, the agent-roster surface, and the owner dropdown on issue cards.
   - **Fully functional for May 22:**
     - **Marketing & SEO** (product-description rewrite — shows off a content-write diff).
     - **Pricing** (margin flag + proposed price change — shows off a structured-field diff).
     - **Sales Support** (order-status draft reply — shows off a customer-facing write).
     - These three cover the three most interesting diff shapes (prose, numbers, customer message).
   - **UI-visible but inert for May 22:**
     - **Inventory Manager**, **Accounting**, **Reporting** — appear in the roster with a "coming soon" badge. Optional demo-only seeded sample issues may appear on the board to illustrate the shape of their work (e.g., a low-stock flag card), but Approve does not invoke an MCP write — it shows a "functionality in a later release" state instead.
     - **Chief of Staff** — scaffolded. Produces the first-run store-profile issue only; autonomous coordination deferred.

### Out of scope for May 22

- Ability browser UI, settings UI beyond a stub page. *(Note: agent roster is now **in scope** at minimum fidelity — needed to render the full fleet per item 4 above.)*
- **Functional** Inventory, Accounting, and Reporting agents — they ship UI-visible but inert; no real MCP writes, no real diff generation. Full functionality in a later release.
- Webhook ingestion, local repo integration, read-only DB replica.
- OAuth 2.1 / host-specific auth.
- UI self-host / hosted build story — run the UI via `wooagent ui` locally for the demo.
- Multi-store, multi-operator, cost caps.

### Demo constraints

- **Test store:** Pressable staging site at `woo-demo-store-99cc5c.mystagingwebsite.com` (WP 6.9.4, WC 10.7.0, PHP 8.4). MCP Adapter pre-installed by Pressable. 15 apparel products seeded. **WooAgent Companion v0.1 installed + verified end-to-end via MCP (Tue Apr 21).** Ready for Phase 1.
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

### Pre-Phase 1 — Tue Apr 21 (completed, pulled forward)

Foundation scaffold + test-store standup + Companion Plugin v0.1 all landed the day before Phase 1 formally begins. PR #1 merged to trunk. Shipped:

- **Foundation scaffold** — Go daemon (`daemon/`) with CLI, SQLite migrations, REST API, auth tokens. React UI (`ui/`) with Vite + `@wordpress/components`, connection form, kanban shell, stub routes. API contract v0.
- **Test store** — Pressable staging site provisioned, MCP Adapter verified, 15 apparel products seeded, Application Password auth working for the daemon. Replaces the "InstaWP / WordPress.com sandbox / `wp-env`" Day-1 task originally scheduled for Wed Apr 22.
- **WooAgent Companion Plugin v0.1** (`companion-plugin/`) — 7 CRUD abilities fully implemented (`wooagent-products/{list,get,update}`, `wooagent-orders/{list,get,add-note}`, `wooagent-customers/get`), 3 `wooagent-device-pair/*` stubs for v0.2. Pulled forward from PRD v0.3 scope to compensate for WooCommerce not yet registering a native `woocommerce/*` ability namespace on WC 10.7.0. Full MCP invocation verified end-to-end against real product data.

**Impact on Phase 1 timeline**
- Elizabeth's Wed–Thu block loses the "test store stand-up" item (~1 day) and lands with a working `wooagent-*` ability surface Monday's MCP-client work can target directly.
- The Companion-Plugin scope pull-forward is sunk cost; Phase 4 testing buffer is intact.
- **No downstream phase dates change.** Marketing persona still targets Mon–Tue Apr 27–28; demo still May 22.

Decisions + debugging lessons captured in `companion-plugin-v0.1-plan.md` and `progress.md`.

### Phase 1 — Apr 22–28 (Wed → Tue, 5 work days): Foundations + first persona end-to-end (compressed)

Aggressive — this phase combines what was originally two phases of work into one. Goal: by Tue Apr 28, a real Marketing agent is producing real issues on a kanban board backed by a real daemon against a real test store. Everything is rough; that's fine.

**Both (Day 1 — Wed Apr 22)**
- Agree user flows for the 3 surfaces on paper (single session, 60 min).
- Agree design-delivery conventions: share-link format, spec template, iteration cadence.

**Elizabeth (Wed Apr 22 → Tue Apr 28)**
- **Wed–Thu (Apr 22–23):** Deliver API contract v1 to Nevena. (React app skeleton, Go daemon skeleton, WP Design System MCP registration, test store + Companion Plugin all done in Pre-Phase 1 — see above.)
- **Fri (Apr 24):** ADK Go spike — get *any* adk-go agent running with Anthropic + a stub tool. Hard stop: working by EOD.
- **Mon (Apr 27):** MCP client in daemon — connect to test store, discover the `wooagent-*` ability surface, cache schemas. Starts from a known-good endpoint (Companion Plugin already serving data).
- **Mon–Tue (Apr 27–28):** Marketing agent persona — reads products via `wooagent-products/list` + `wooagent-products/get`, generates a rewrite proposal, lands in `In Review` via `GET /issues`, `POST /issues/:id/approve`, `POST /issues/:id/reject`. Build the kanban board React from Nevena's designs as they land.

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
- Seed logic: on first run, Chief of Staff stub produces 2–3 starter issues for each of the three functional personas (Marketing, Pricing, Sales Support) and 1–2 demo-only sample issues for each of the inert personas (Inventory, Accounting, Reporting) so the kanban has content for the demo and the full fleet feels alive. Inert-persona cards carry a "preview — coming soon" label on the detail view so the approve action is obviously disabled.
- Build the run-log panel React once Nevena's design lands.

**Nevena**
- Finish **onboarding flow** mockups including error states (store unreachable, pairing timed out, model key invalid).
- Design the **run-log panel** (quick view of "what did the agent actually do").
- Finalize persona visual identities (avatars, color accents) for all six operator personas + Chief of Staff, including a "coming soon" treatment for the three inert personas that reads as deliberate rather than unfinished.

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
- **Stretch:** start sketches of deferred surfaces (ability browser) for v2, plus concepts for the "coming soon" inert-persona detail view if Phase 3 shipped only a rough version.

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
- [ ] **Full fleet visible in the agent roster:** all six operator personas + Chief of Staff present, with clear functional-vs-inert states.
- [ ] Three personas (Marketing, Pricing, Sales Support) generate real issues against the test store within the first session.
- [ ] The three inert personas (Inventory, Accounting, Reporting) appear with demo sample issues and an obviously-disabled approve action labeled "coming soon."
- [ ] Operator can approve one issue of each functional type (prose, numeric, message) and the change lands on the store.
- [ ] Run-log panel shows a clean trace for any approved issue.
- [ ] No crashes on the demo laptop during a 10-minute walkthrough.

---

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| **Phase 1 is deliberately over-scoped.** | This is the biggest risk. If Phase 1 slips, Phase 4 (testing) is the absorber — not Phase 5 (demo). Hard stops: by end of Fri Apr 24, ADK Go must be working with a stub tool; by end of Mon Apr 27, MCP client must be talking to the test store. If either misses, cut the Marketing agent's scope to a hard-coded proposal (skip the LLM) so kanban can still demo a real issue flow. |
| **Scope is ambitious, especially with Elizabeth doing all the coding.** | Three personas is the hard floor — drop to two if Phase 2 slips. Drop Ollama fallback from the demo if time is tight; ship only Anthropic. Consider pre-configuring the app for the demo instead of running onboarding live (preserves the surface in designs but saves build time). |
| **MCP Adapter auth (device-pair) is new work in the PRD.** | App Password auth is the Phase 1 path. Companion Plugin v0.1 already registers `wooagent-device-pair/*` abilities as 501 stubs; full token flow ships in Companion Plugin v0.2, likely post-May 22. Nevena designs the device-pair UX; Elizabeth only wires the frontend if Phase 3 has slack. |
| **WP Abilities API has silent-failure gotchas.** | Uncovered during the Apr 21 Companion Plugin build: single-slash name regex, `wp_`-prefixed dual init hooks (`wp_abilities_api_categories_init` fires before `wp_abilities_api_init`), and a separate `meta.mcp.public` opt-in for MCP exposure beyond `show_in_rest`. All are silent `_doing_it_wrong()` notices outside `WP_DEBUG`. Documented in `companion-plugin-v0.1-plan.md` "Debugging lessons." The daemon's MCP client should treat ability discovery as the source of truth and never assume a name space exists until it appears in the adapter's response. |
| **Design-to-code translation fidelity loss.** | Elizabeth uses `@wordpress/components` / `@wordpress/ui` primitives everywhere so translation is mostly composition, not pixel-pushing. The WordPress Design System MCP (`@wordpress/design-system-mcp`) gives her authoritative props and tokens during build, so she isn't improvising API shapes from Nevena's screenshots. Nevena reviews screenshots twice weekly; fidelity fixes are scoped as "close enough for testing." Don't let Phase 5 (3 days only) become a pixel-polish vortex. |
| **Design-ahead-of-code lag.** | Nevena's Phase 1 is heavily front-loaded so Elizabeth is never blocked: lightweight style tile by end of Thu Apr 23, kanban card + layout by end of Mon Apr 27, full kanban mockups by Tue Apr 28. If Nevena slips on a later surface, Elizabeth defers that screen and uses a placeholder for the demo. |
| **ADK Go unfamiliarity for Elizabeth.** | Spike on Fri Apr 24 (Day 3 of Phase 1): get *any* ADK Go agent running with Anthropic + a stub tool before committing to MCP integration the following Monday. |
| **Test Woo store setup blocks everything else.** | **Resolved Tue Apr 21** — Pressable staging site live, MCP Adapter + Companion Plugin v0.1 verified end-to-end. |
| **Drag-drop interactions are a rabbit hole.** | First, check the WordPress Design System MCP (`wpds://components`) for a drag-drop or sortable primitive — the WP ecosystem has iterated on this for Gutenberg. If nothing suitable exists, use `@dnd-kit/core` wrapped in WP-styled components. Don't over-invest. |
| **Real LLM cost during dogfooding.** | Set a per-day dollar cap in the model adapter from day 1; cache ability discovery. Use Ollama as a cost-free fallback for persona development when Elizabeth doesn't need frontier-quality output. |

---

## Deferred — post-May 22

The PRD roadmap v0.1 items not shipping by May 22:
- Ability browser UI (agent roster **is** shipping — see in-scope item 4).
- **Functional** Inventory / Accounting / Reporting personas — they ship UI-visible but inert; real propose/approve behavior and MCP writes are a post-May-22 release.
- OAuth 2.1 + host-specific auth paths
- **Companion Plugin v0.2:** native device-pair token flow (stubs are in place in v0.1). Staged-changes, guardrails, and issue-status sync remain scoped to PRD v0.3.
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
2. Run `wooagent init` — pair with the test store via the device-pair flow (or App Password fallback), configure Anthropic, deploy the full fleet (six operator personas + Chief of Staff visible in the roster; three of them functional).
3. Run `wooagent run` + `wooagent ui` — kanban loads with ~9–12 starter issues: 2–3 real issues from the three functional personas plus 1–2 demo-only sample issues from each inert persona so the full fleet is represented on the board.
4. Click into a Marketing issue in `In Review` — see before/after product-description diff — approve — watch the product update on the store in a second browser tab.
5. Repeat for a Pricing issue and a Sales Support issue.
6. Open the run log for one approved issue — see the model calls and ability invocations listed cleanly.

If those six steps complete on the demo laptop, we're done.
