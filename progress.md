# WooAgent OS — Build progress

Continuously-updated log of what's shipped vs. what's scheduled. The source of truth for scope and dates is `WooAgent_plan.md`; this file tracks actuals.

**Legend:** ✅ done · 🚧 in progress · ⏳ scheduled · ❌ cut/deferred

---

## At a glance

| Phase | Window | Status | Headline |
|-------|--------|--------|----------|
| Pre-Phase 1 | Tue Apr 21 | ✅ done | Foundation scaffold: Go daemon + React UI + API contract |
| Phase 1 | Wed Apr 22 – Tue Apr 28 | 🚧 in progress | Foundations + first persona end-to-end (Marketing) |
| Phase 2 | Wed Apr 29 – Tue May 5 | ⏳ | Multi-persona + review/approval |
| Phase 3 | Wed May 6 – Tue May 12 | ⏳ | Onboarding + integration |
| Phase 4 | Wed May 13 – Tue May 19 | ⏳ | Testing + iterating (dedicated) |
| Phase 5 | Wed May 20 – Fri May 22 | ⏳ | Polish + hack-day demo |

---

## Pre-Phase 1 — Tue Apr 21 (foundation scaffold)

Pulled the Wed–Thu foundation tasks forward a day so Phase 1's co-work block with Nevena stays free for user-flow agreement, design-delivery conventions, and test-store standup. Plan file: `.claude/plans/effervescent-whistling-glacier.md`.

### Shipped

**Repo + tooling**
- ✅ Monorepo layout under `/Users/elizabethpizzuti/claude/wooagent/` with `daemon/`, `ui/`, `docs/`. Existing `.md` docs stay at repo root.
- ✅ `README.md` (dev quickstart) and `.gitignore` (Go + Node + OS).
- ✅ Installed Go 1.26.2 via `brew install go` (reversible: `brew uninstall go`).

**Go daemon (`daemon/`)**
- ✅ `go.mod` (`github.com/wooagent-os/wooagent-os/daemon`) pinning cobra, chi/v5, chi/cors, google/uuid, modernc.org/sqlite (pure-Go, no CGO), yaml.v3.
- ✅ Full CLI surface from PRD §13.3 wired up as cobra subcommands. **Working:** `init`, `run`, `version`, `auth token create`. **Stubs (return "not implemented yet"):** `ui`, `store [add|list|pair|unpair|abilities]`, `agent [list|deploy|disable]`, `model provider [add|test]`, `issue [create|list]`, `logs`, `abilities refresh`, `export`.
- ✅ SQLite store via `modernc.org/sqlite` with embedded migrations. WAL mode, FK enforcement, busy-timeout tuned.
- ✅ `migrations/001_init.sql` creates `agents`, `issues`, `runs`, `turn_events`, `blobs`, `auth_tokens`, `schema_migrations`. The `turn_events` + `blobs` tables match the v2 GEPA schema spec up front (changing that schema after operators accumulate data is painful — this is the "nail it now" decision from `wooagent-os-gepa-v2-plan.md`).
- ✅ Bearer-token auth: SHA-256 hashed storage, tokens minted as `wo_pat_<hex>`, `last_used_at` tracked. Keychain storage scheduled for Phase 3.
- ✅ HTTP server: chi router, CORS (permissive — bearer is the real boundary), RequestID/Logger/Recoverer/Timeout middleware, graceful shutdown on SIGINT/SIGTERM.
- ✅ `/v1` endpoints: `GET /v1/health` (unauth), `GET /v1/agents`, `GET|POST /v1/issues`, `GET /v1/issues/{id}`, `POST /v1/issues/{id}/approve|reject` (501 Phase 2), `GET /v1/abilities` (empty until MCP client lands).
- ✅ `internal/registry` — `LoadPrompts` + `LoadSkills` read versioned YAML from `prompts/<persona>/v*.yaml` and `skills/<name>/v*.yaml`. GEPA optimization target from v1.
- ✅ `internal/telemetry` — `TurnEvent` struct mirrors the SQLite schema + the v2 GEPA ingest format. `SQLiteRecorder` ready for Phase 2 agents to start writing immediately.
- ✅ `wooagent init` seeds three default personas (marketing, pricing, sales_support) into the `agents` table so issue FKs resolve.

**React UI (`ui/`)**
- ✅ Vite 5 + React 18 + TypeScript 5 project. `tsc -b` and `vite build` both clean.
- ✅ Dependencies: `@wordpress/components`, `@wordpress/i18n`, `@wordpress/icons`, `react-router-dom`.
- ✅ First-launch connection form — probes `/v1/health` (unauth) then `/v1/agents` (auth) before persisting daemon URL + token to `localStorage`.
- ✅ Kanban shell: 5 `@wordpress/components` `Panel`/`PanelBody` columns (Backlog, Todo, In Progress, In Review, Done) fetching `/v1/issues`.
- ✅ Stub routes for `/issues/:id` and `/settings`.
- ✅ Component selection verified against the WordPress Design System MCP (`@wordpress/design-system-mcp`) per the architectural-stance memory.

**API contract + docs**
- ✅ `docs/api-contract-v1.md` — every `/v1/*` endpoint with JSON examples, `Issue`/`Persona`/`Run`/`AbilityRef`/`DiffProposal` type definitions (with prose/numeric/message diff variants matching the three Phase 1 personas), error-code table, status machine, versioning policy. This is the handoff artifact for Nevena.

### Verified end-to-end

- `go build ./...` clean
- `wooagent init` on empty state dir → creates `~/.wooagent/`, migrates SQLite, seeds 3 personas, mints initial token
- `wooagent run` → serves http://localhost:7777; `/v1/health` unauth returns 200; unauth `/v1/issues` returns 401; auth `/v1/issues` + `/v1/agents` return 200
- `POST /v1/issues` creates an issue and it shows up in list
- CORS preflight from `localhost:5173` → 200 with `Access-Control-Allow-Origin` / `-Allow-Headers: Authorization`
- `npm run dev` in `ui/` serves Vite at `localhost:5173`, React refresh active, main.tsx loads
- `npm run build` produces a clean static bundle

---

## Phase 1 — Wed Apr 22 – Tue Apr 28 (foundations + first persona end-to-end)

### Scheduled

- ✅ **Tue Apr 21** (Elizabeth): test WooCommerce store provisioned on Pressable staging (`woo-demo-store-99cc5c.mystagingwebsite.com`). MCP Adapter pre-installed and verified via handshake. 15 apparel products seeded. **Discovered:** WooCommerce has not registered `woocommerce/*` abilities on this install; only 5 core/Jetpack abilities exposed. Triggered Companion Plugin scope pull-forward — see `companion-plugin-v0.1-plan.md`.
- ✅ **Tue Apr 21** (Elizabeth): **Companion Plugin v0.1 shipped + verified end-to-end** (`companion-plugin/`). 7 CRUD abilities fully implemented, 3 device-pair abilities stubbed. Installed on test store and **live product data returned through MCP `mcp-adapter-execute-ability`** — Marketing persona's critical path is unblocked. Uncovered three non-obvious WP Abilities API rules (single-slash names, dual init hooks, separate `meta.mcp.public` opt-in); all documented in `companion-plugin-v0.1-plan.md` "Debugging lessons" section.
- ⏳ **Wed Apr 22** (both): user-flow agreement on the 3 surfaces (60 min session); design-delivery conventions (share-link format, spec template, cadence).
- ⏳ **Wed Apr 22** (Elizabeth): install Companion Plugin on test store; verify the 7 CRUD abilities discover via `/wp-json/wp-abilities/v1/abilities` and invoke through MCP `mcp-adapter-execute-ability`; API contract v1 delivered to Nevena.
- ⏳ **Wed–Thu** (Nevena): lightweight style tile (colors, type, spacing).
- ⏳ **Fri Apr 24** (Elizabeth): **ADK Go spike** — get any `github.com/google/adk-go` agent running with Anthropic + a stub tool. Hard stop: working by EOD.
- ⏳ **Fri–Mon Apr 24, 27** (Nevena): kanban card anatomy + board-layout wireframes delivered by EOD Mon.
- ⏳ **Mon Apr 27** (Elizabeth): MCP client — connect to test store, discover the `wooagent-*` ability surface, cache schemas. Hard stop: talking to the store.
- ⏳ **Mon–Tue Apr 27–28** (Elizabeth): **Marketing agent persona** — reads products via `wooagent-products/list` + `wooagent-products/get`, generates a rewrite proposal, lands in `In Review` via the existing HTTP endpoints; kanban board React translated from Nevena's designs as they land.
- ⏳ **Tue Apr 28** (Nevena): full kanban board mockups (all 5 columns, card states, interaction spec) + first pass on review/approval prose diff layout.

### Exit criteria

- [ ] Marketing agent produces a real issue against the test store.
- [ ] The issue appears on a kanban board at rough fidelity to Nevena's designs.
- [ ] Daemon + React app + test store + MCP all working end-to-end.

### Risks being watched

- Phase 1 is deliberately over-scoped (two old phases collapsed). If it slips, Phase 4 testing is the absorber — not Phase 5 demo.
- ADK Go unfamiliarity — hence the Fri spike before committing to MCP integration on Mon.
- Test-store standup blocking everything else — Day 1 priority.

---

## Phase 2 — Wed Apr 29 – Tue May 5 (multi-persona + review)

### Planned

- ⏳ **Elizabeth:** Pricing agent (structured numeric diff); Sales Support agent (customer-reply draft); propose-mode across all three; `Approve` button actually invokes the staged MCP ability; run log stores every model call + ability call + state change; review/approval screen React.
- ⏳ **Nevena:** review/approval mockups covering all three diff shapes (prose, numeric, message) + approve/reject + run-log peek; start onboarding flow mockups.

### Exit criteria

- [ ] All three personas generate real issues.
- [ ] Operator can approve a Marketing rewrite and the product description updates on the test store.
- [ ] Review screen renders all three diff shapes at rough fidelity to Nevena's designs.

---

## Phase 3 — Wed May 6 – Tue May 12 (onboarding + integration)

### Planned

- ⏳ **Elizabeth:** onboarding backend (connection config, device-pair or App-Password auth, provider config, fleet bootstrap); onboarding flow React from Nevena's designs; starter-issue seeding (2–3 per persona for demo content); run-log panel React.
- ⏳ **Nevena:** finish onboarding flow including error states (store unreachable, pairing timed out, model key invalid); run-log panel design; finalize persona visual identities.

### Exit criteria

- [ ] End-to-end first-run on a clean machine: install → pair → configure → deploy → see issues on board → approve one → verify change on store.
- [ ] All three design surfaces implemented at rough fidelity to Nevena's designs.

---

## Phase 4 — Wed May 13 – Tue May 19 (testing + iterating)

### Planned

- ⏳ No new features. Daily: full operator walkthrough (install → pair → kanban → approve one issue of each type); log every friction point and bug; fix highest-impact first; tune agent prompts against real outputs; refine diff rendering for edge cases (very long descriptions, zero-margin items, nonexistent order IDs).
- ⏳ **Nevena:** ride along with Elizabeth's walkthroughs, flag fidelity drift, produce targeted redesigns for empty/loading/error states that are breaking under real use. **Stretch:** sketches of deferred surfaces (ability browser, agent roster) for v2.

### Exit criteria

- [ ] At least three full operator walkthroughs with no crashes.
- [ ] Top 5 bugs/friction points from Phase 3 resolved.
- [ ] Agent output quality measurably improved (no negative-price proposals; marketing rewrites stay at sensible length).

---

## Phase 5 — Wed May 20 – Fri May 22 (polish + demo)

### Planned

- ⏳ **Wed May 20:** feature freeze. Final polish, demo script, screenshot/video assets.
- ⏳ **Thu May 21:** dress rehearsal on the demo laptop, fix any demo-breakers.
- ⏳ **Fri May 22:** hack/review day.

### Demo-ready criteria (May 22 morning)

- [ ] Fresh-machine install works: `wooagent init` → `wooagent run` → `wooagent ui` opens the app.
- [ ] Onboarding completes in under 10 minutes on a fresh machine.
- [ ] Three personas generate real issues against the test store within the first session.
- [ ] Operator can approve one issue of each type (prose, numeric, message) and the change lands on the store.
- [ ] Run-log panel shows a clean trace for any approved issue.
- [ ] No crashes during a 10-minute walkthrough.

---

## Explicitly deferred (post-May 22)

Straight from `WooAgent_plan.md`:
- Ability browser + agent roster UI
- Inventory / Accounting / Reporting personas (architecture supports them; just not built)
- OAuth 2.1 + host-specific auth paths
- Full Companion Plugin (staged-changes, guardrails, issue-status sync)
- Hosted UI build at `ui.wooagent.dev`
- Multi-store, multi-operator
- Skill pack installer, cost caps, offline mode
- v2 GEPA central pipeline — v1 scaffolding is already in the daemon (prompt registry, `TurnEvent` schema); the central ingest service + GEPA adapter + review UI are their own track post-launch.

---

## Notes and decisions captured

- **Go install (Apr 21):** Go 1.26.2 via Homebrew. Wasn't on the machine before. `brew uninstall go` to reverse.
- **Pure-Go SQLite driver choice (Apr 21):** `modernc.org/sqlite` chosen over `github.com/mattn/go-sqlite3` to preserve the single-binary cross-platform promise from PRD §6.1 / §11. No CGO required.
- **Agent registry format (Apr 21):** personas seeded into DB via `wooagent init`, not hard-coded in handler fixtures. `GET /v1/agents` reads from DB; operators can eventually disable personas without them re-appearing.
- **Day-1 scope (Apr 21):** user chose to pull Wed–Thu foundation scope forward to Tue, minus test-store standup which stays Wed. Buys a day of slack ahead of the aggressive Phase 1.
- **Repo + git (Apr 21):** `github.com/elizaan36/wooagent-os`, private. Monorepo with `daemon/` + `ui/` siblings; the PRD's plan to ship UI independently (its own release cadence) still stands — the UI splits out at open-sourcing or first independent release, whichever comes first. Apache 2.0 `LICENSE` committed now so open-sourcing doesn't require license-archeology later.
- **Collaboration mode (Apr 21):** Claude authorized to commit + push freely on feature branches; confirms before pushing to `main`; never force-pushes, skips hooks, or runs destructive git ops without explicit confirmation.
- **Companion Plugin scope pull-forward (Apr 21):** PRD §10.4 originally scoped the Companion Plugin's non-pairing abilities for v0.3. Test-store probe revealed WooCommerce has not registered `woocommerce/*` abilities on the current ecosystem — only 5 core/Jetpack abilities surface. Rather than fall back to WC REST (which sidesteps the PRD's MCP-native bet), we're registering `wooagent/*` CRUD abilities in our own plugin now. They flow through the existing MCP Adapter's execute-tool — no daemon changes needed. When WC eventually ships native abilities, ours deprecate gracefully. Full plan in `companion-plugin-v0.1-plan.md`. Tradeoff: weakens the "zero-install on Pressable/WPE/WordPress.com" pitch in the short term; pulls ~2-3 days of PHP into Phase 1, absorbed by Phase 4 buffer.
