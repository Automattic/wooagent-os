# WooAgent OS — Build progress

Continuously-updated log of what's shipped vs. what's scheduled. The source of truth for scope and dates is `WooAgent_plan.md`; this file tracks actuals.

**Legend:** ✅ done · 🚧 in progress · ⏳ scheduled · ❌ cut/deferred

---

## At a glance

| Phase | Window | Status | Headline |
|-------|--------|--------|----------|
| Pre-Phase 1 | Tue Apr 21 | ✅ done | Foundation scaffold: Go daemon + React UI + API contract |
| Phase 1 Day -1 | Tue Apr 21 (eve) | ✅ done | Test store + Companion Plugin v0.1 (PR #1) — live MCP data flowing |
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

### Pulled forward into Tue Apr 21 (evening session)

- ✅ **Test WooCommerce store provisioned** on Pressable staging (`woo-demo-store-99cc5c.mystagingwebsite.com`). MCP Adapter pre-installed. 15 apparel products seeded. API endpoint: `/wp-json/mcp/mcp-adapter-default-server` (Streamable HTTP, session-bound). Auth via WordPress Application Password for Phase 1.
- ✅ **Ecosystem probe** — discovered WooCommerce has **not** registered a `woocommerce/*` namespace yet on WC 10.7.0 / WP 6.9.4. The Abilities API itself is in WP core; only 5 abilities exposed (core site/env info + Jetpack Forms). Triggered the Companion Plugin scope pull-forward decision.
- ✅ **Companion Plugin v0.1 shipped + verified end-to-end** (`companion-plugin/`, merged via PR #1). 7 CRUD abilities fully implemented (`wooagent-products/{list,get,update}`, `wooagent-orders/{list,get,add-note}`, `wooagent-customers/get`), 3 `wooagent-device-pair/*` abilities registered as 501 stubs for v0.2. Live MCP `tools/call mcp-adapter-execute-ability` returns real product data. wp-admin menu entry points operators at Application Password auth until device-pair lands.

### Verified end-to-end (Tue Apr 21 evening)

- MCP handshake against the test store's adapter succeeds; session-bound `Mcp-Session-Id` header captured.
- `/wp-json/wp-abilities/v1/abilities` returns all 10 `wooagent-*` abilities alongside the preexisting core/Jetpack/MCP-Adapter abilities (19 total).
- MCP `tools/call mcp-adapter-execute-ability` → `wooagent-products/list` returns the 15 seeded products with summaries (name, slug, price, description/short-description lengths).
- MCP `tools/call mcp-adapter-execute-ability` → `wooagent-products/get` for id=821 returns the full product — real name, short description, long description.
- Stub abilities return `isError: true` via MCP as designed (friendly 501 message is collapsed by the adapter's error wrapper; passthrough polish tracked in `companion-plugin-v0.1-plan.md`).

### Pulled forward into Wed Apr 22 (morning)

- ✅ **ADK Go spike — green on first run.** Throwaway `daemon/cmd/spike-adk/` binary: one `llmagent` with one `functiontool` (`get_product_stub`), driven by `google.golang.org/adk` v1.1.0's `runner` against a local LM Studio server (`google/gemma-4-e4b` at `localhost:1234/v1`). Gemma correctly invoked the stub with `product_id=821`, consumed the structured reply, and produced a one-sentence marketing line using only the returned fields. Exit 0.

### Findings from the spike

- **PRD §11.1 and §11.10 import path fixed.** Module is `google.golang.org/adk` (v1.1.0 as of Apr 10, 2026), not `github.com/google/adk-go`. The GitHub URL is the repo, not the Go module path. Corrected inline on Apr 22.
- **No native Anthropic in ADK.** Only `gemini` + `apigee` in `google.golang.org/adk/model`. The `model.LLM` interface is typed on `google.golang.org/genai` content — any custom provider has to translate genai ↔ provider format. For now we use third-party adapters:
  - Anthropic: `github.com/Alcova-AI/adk-anthropic-go` v0.1.15 (Apr 7, 2026) — wired into go.mod, not used in this spike run but ready.
  - OpenAI-compatible (LM Studio / Ollama / vLLM): `github.com/huytd/adk-openai-go` — uses `/v1/chat/completions`, which is what LM Studio serves. `amammay/adk-go-openai` is newer but uses the `/v1/responses` endpoint that LM Studio doesn't implement, so rejected.
- **Third-party-adapter maintenance risk.** Both adapters are small, low-star, single-maintainer repos. **Acceptable for v1 prototype.** Before Phase 5 demo, either vendor them into `daemon/internal/models/` or follow the upstream `google/adk-go-community` effort (PR #242, pending as of Apr 4).
- **Cost posture.** Local Gemma on LM Studio works well enough for agent-loop smoke tests and early Marketing persona iteration. Flip to Claude Haiku 4.5 when diff quality matters (review/approval testing in Phase 2–4).
- **`model.LLM` interface hasn't broken since v0.3.0.** huytd's adapter pins v0.3.0 but compiles and runs against our v1.1.0 unchanged — reduces concern about ADK churn.

### Thu Apr 23 (morning session)

Originally scheduled only for "MCP client in daemon." Pulled in a lot more — PRD v0.2 design pass, WooCommerce AI plugin inspection, WooPayments abilities proposal for an external dev, and a pulled-forward pre-signed manifest. Three commits on `origin/trunk`.

- ✅ **MCP client in daemon + first real ability round-trip.** Commit `9542b3a`. New `daemon/internal/mcp/` implements Streamable HTTP + JSON-RPC: handshake, `Mcp-Session-Id` tracking, `tools/call`, HTTP Basic Auth. New `cmd/mcp-probe/` verifies the MCP transport in isolation (no ADK). New `cmd/spike-mcp/` replaces `spike-adk`'s stub with a real `list_products` that routes through `mcp-adapter-execute-ability` → `wooagent-products/list`. Verified end-to-end against the Phase 1 test store with `gemma-4-e4b-it` on LM Studio: model chose the tool zero-shot, 15-product catalog returned with real names (Cashmere Scarf, Wool Cardigan, Merino Turtleneck, Wool Crewneck, Ribbed Crewneck), final text grounded in tool output. Full daemon chain proven: model → ADK orchestrator → MCP client → WP MCP Adapter → `wp_register_ability` → WooCommerce → back to grounded text.
- ✅ **Pre-signed ability manifest seeded with 25 real entries.** Commit `1596ea1`. New `daemon/internal/manifest/` package: types (`Entry`/`Manifest`/`Scope`/`Persona`/`TrustState`), canonical JSON schema hashing (sorted keys, stable number format, RFC-conformant string escaping via `encoding/json`), embedded default via `//go:embed`, operator overlay support via `Load()`. Shipped default covers 10 Companion Plugin abilities (pre-signed by construction), 10 WooCommerce AI plugin local abilities, 3 Jetpack Forms abilities, 2 WP core abilities — every entry with a real SHA-256 hash computed from the live store's schemas, so drift detection is wired end-to-end. New `cmd/manifest-compute/` is the developer tool for seeding/refreshing when plugin schemas change. 6 tests enforce design invariants, not spot-checks: baseline coverage, read-scope implies reversibility 1.0, device-pair has no persona mappings (daemon-bootstrapping only), canonical hashing, drift detection on field addition.
- ✅ **PRD v0.2 — three significant refinements.** Commit `ab8459d`.
  - **§10 Insights, Planning, and Background Execution (new).** Codifies the insight → plan → task → execution loop. Four-dimension insight scoring (impact × confidence × urgency × reversibility) as both a ranking function and a policy input (reversibility feeds the approval gate). Plan as a durable markdown artifact owned by the daemon, not ephemeral chat state. Plan-to-task extraction into SQLite. Steps expressed as goals, not tool calls — composes naturally with ADK's per-step orchestrator sessions. Approval gates with provenance + reversibility overrides. In-process background execution. Inspired by the Big Ocean Command Centre demo and the AIP2 collaborative-planning experiment, but re-grounded in WooAgent OS's always-on daemon (the WPCOM-side prototype's async-jobs+Jetpack workaround isn't needed when the runtime is persistent).
  - **§8.4 Ability Trust Model — pre-signed manifest + Policy Enforcement Point (new).** Two layers. Manifest = allowlist (§8.4.1). PEP = deterministic non-LLM Go middleware between orchestrator and MCP client that verifies trust state, schema match, persona scope, argument validity, operator policy predicates, budget, and scope sufficiency, in that order, on every invocation (§8.4.2). Short-lived per-invocation capability tokens, chain-of-identity logging (`plan → task → step → persona → model → prompt → ability → outcome`), schema-hash drift auto-demotes a pre-signed ability to `unapproved`. §15 picks up the principle as security item #9.
  - **§11.4 Companion Plugin repositioned as Baseline Ability Provider + §11.7 WPCOM-Enhanced Mode (new) + §4.7 no-WPCOM-as-hard-dependency non-goal.** Companion Plugin now the self-sufficiency anchor, not an optional extra. §11.7 describes the optional enhanced tier for Jetpack-connected stores: the WooCommerce AI plugin's 50-tool progressive MCP registry and any WPCOM-native aggregations become available. Strictly additive, strictly opt-in. Explicitly the Automattic-commercial-alignment layer: stores that connect are measurably more capable, which gives leadership a reason to support the project without compromising the local-first guarantee. §20 gained a Dolly / WordPress Agent comparison bullet clarifying the chat-vs-kanban and hosted-vs-local-first axes.
- ✅ **WooPayments abilities proposal** for the dev starting on WP-registered payment abilities. 16 tier-1 abilities across four workflows (payouts & reconciliation, disputes & chargebacks, transactions, refunds, fees), plus tier-2 (customers, failed-subscription retries, risk) and tier-3 (risk rules, payout schedule, multi-currency). Naming conventions, idempotency-key requirement on writes, reversibility-as-first-class-metadata, two-phase `draft: true` mode for `disputes-submit-evidence`, and explicit non-asks (no bundled multi-op abilities, no freeform-query abilities, no side-effect comms). Saved as `woopayments-abilities-proposal.md` in the project root, shareable as-is.

### Findings from the Thu Apr 23 session

- **WooCommerce AI plugin has a two-surface architecture.** Local Abilities API exposes 10 `woocommerce/*` abilities that are deliberately agent-oriented/bundled (`analyze-shipping-needs`, `create-bookable-product`, `manage-shipping-zones`, `search-products`), not raw CRUD. A separate **progressive MCP registry** of 14 providers × 50 tools (products, orders, customers, coupons, categories, tags, order-notes, refunds, shipping-zones, payment-gateways, settings, analytics, taxes, system-status) is gated by an `mcp_progressive_enabled` toggle and designed to flow through a WPCOM proxy at `public-api.wordpress.com/wpcom/v2/woocommerce-ai-mcp/v1`. Local-only coverage has real gaps for Pricing (no coupons / taxes), Accounting (no settings / system-status), and Reporting (no analytics-revenue / analytics-orders). That gap directly motivates §11.7 and the WPCOM-enhanced opt-in.
- **MCP Adapter uses a three-meta-tool pattern, not one-tool-per-ability.** The default MCP server exposes only `mcp-adapter-discover-abilities`, `mcp-adapter-get-ability-info`, `mcp-adapter-execute-ability`. Agents discover the ability surface dynamically through these meta-tools instead of flooding the model context with 50+ tool schemas up-front. Matches the progressive-MCP pattern from the Big Ocean demo. Implication for the daemon: the ADK tool layer can either expose 1 generic `execute_ability` tool (simplest, what `spike-mcp` does) or N semantic tools that wrap `execute-ability` under the hood (better model ergonomics, Phase 1–2 direction).
- **MCP Streamable HTTP transport quirks captured.** (1) Server returns `Mcp-Session-Id` header on the initialize response; subsequent requests without it fail with `-32600 Invalid Request: Missing Mcp-Session-Id header`. (2) The protocol requires a `notifications/initialized` message between initialize and `tools/list` — skipping it makes tools/list return 0 tools with no error. (3) The WP Abilities API annotations use the `*Hint` names on this adapter (`readOnlyHint`, `destructiveHint`); the bare `readonly`/`destructive` fields come back `null`. All three captured in `daemon/internal/mcp/client.go` and `daemon/cmd/manifest-compute/main.go` so the next author doesn't rediscover.
- **Dolly / WordPress Agent positioning clarified.** Dolly is a general-purpose WPCOM-hosted chat agent that adopts a site's personality via Telegram/Slack/WhatsApp. WooAgent OS is a local-first kanban-native ops runtime with a fleet of role-specific personas. Adjacent problems, different product shapes — the two can coexist on the same store. New §20 bullet on this.

### Mon Apr 27 (evening session — UI on the WordPress Design System)

Goal for the evening was narrow: bridge the daemon-served UI to look like the marketing-prototype kanban + content-review screenshots. Scope expanded once the user named WPDS as the right foundation — both frontends got migrated in one branch instead of just reskinning the daemon UI. Branch: `feat/wpds-bridge-ui` (committed locally, not pushed to origin).

- ✅ **`CLAUDE.md` at repo root.** Codifies the WPDS-first stance for future sessions: canonical packages (`@wordpress/ui` primary, `@wordpress/components` for gaps, `@wordpress/icons`, `@wordpress/element`), `--wpds-*` design tokens only, `Stack`/flexbox for layout (no Tailwind, no Inter / Roboto / system-font defaults), the seven persona-color CSS variables (`--wa-persona-{mk,pr,in,ac,rp,ss,cs}-{bg,ink}`) as the only documented non-WPDS color exception (used on agent avatar squares + the kind pill), skills to invoke for UI work (`wpds`, `frontend-design`, `wordpress-mockups`). Reference site + private design-systems P2 + Storybook URLs catalogued.
- ✅ **`/ui/` (daemon UI) migrated to WPDS and reskinned.** Top navbar replaced by a left sidebar (`LeftNav.tsx`) with WooAgent OS branding, App / Agents groups, persona-color avatars, "Marketing N in review" badge wired to live `/v1/issues` data, connected-store footer derived from the current `connection.daemonUrl`. Kanban (`Kanban.tsx`) became a 4-column `Card`-based board (`backlog` / `drafting` / `in_review` / `done`) — daemon's 5 statuses collapse cleanly: `todo`+`in_progress` → drafting, `rejected` drops off the board. `IssueDetail.tsx` rebuilt from scratch to match the content-review screenshot: breadcrumb (Board · ID · status badge · kind pill) → persona eyebrow with MK avatar + model name → `<h1>` + subhead → 4 KPI `Card`s (Scope / Brand voice / SEO / Est. impact) → two-column body (Current description card with `proposal.target.previous`; single-variant card; right rail with sample Brand voice check / Yoast SEO / Reversible-always notice) → sticky bottom `ActionBar` (Approve & apply / Reject / Cancel). Approve / Reject still call `/v1/issues/:id/approve|reject`; on success the parent `App.tsx` refetches issues so the kanban reflects the new state. New components: `LeftNav`, `StatusBadge`/`KindBadge` (kind pill in mk-pink), `Kpi`, `SidebarRail`, `ActionBar`, `Placeholder`. Two new placeholder routes (`/activity`, `/abilities`) and six `/agents/*` routes so sidebar links don't 404.
- ✅ **`/marketing-prototype/` migrated to WPDS for parity.** Tailwind, PostCSS, autoprefixer, `tailwind.config.js`, `postcss.config.js` all removed. All seven components and five screens (Kanban, ContentReview, CampaignPlanner, EmailReview, Placeholder, plus SettingsDrawer + ToastStack + HostedBanner + LeftNav + StatusBadge) ported to `Stack` / `Card` / `Text` / `Badge` from `@wordpress/ui` with `--wpds-*` tokens. The 471-line ContentReview keeps its variant-cards / SEO bar / brand-voice highlights, just driven by tokens; sticky action bar uses a tiny `wa-action-bar` class because WPDS has no equivalent layout primitive. Mock `SEED_TASKS` data and types untouched. Real Woo API integration through `lib/woo.ts` (the dev-proxy flow) preserved.
- ✅ **GHES Pages redeploy.** Built with `PUBLIC_BASE=/pages/Automattic/wooagent-os/`, force-pushed to `gh-pages` on `github.a8c.com:Automattic/wooagent-os`. Live at https://github.a8c.com/pages/Automattic/wooagent-os/.

### Findings from the Mon Apr 27 session

- **`@wordpress/theme`'s `ThemeProvider` is currently locked behind `privateApis`.** The README example (`import { ThemeProvider } from '@wordpress/theme'`) is aspirational on 0.11.x — the type is exported but the value isn't. Static `import '@wordpress/theme/design-tokens.css'` defines every `--wpds-*` token on `:root` and is sufficient for our needs (no per-subtree theming or dark-mode override yet). Re-evaluate when ThemeProvider stabilizes.
- **Mapping screenshot pink to WPDS native intents would have lost agent identity.** WPDS `Badge` ships `high/medium/low/stable/informational/draft/none` — no pink. Rather than convert CONTENT/CAMPAIGN/EMAIL pills to `informational` (blue) we kept persona color as a small documented exception (`--wa-persona-mk-bg` / `--wa-persona-mk-ink`, etc.). Sidebar avatar squares use the same vars. CLAUDE.md is explicit that this exception does not extend to status badges, KPI cards, action buttons, or notices — those follow WPDS intents.
- **Persona colors as CSS variables make the design system extension legible.** Defining them once in `globals.css` / `app.css` and referencing them from JSX inline styles (vs. baking literal hex into components) keeps the CLAUDE.md rule enforceable: any non-`--wpds-*` color in a `style={}` block is a tell.
- **Daemon's single-proposal data shape is the hidden constraint behind a multi-variant UI.** IssueDetail's variants list is laid out for N cards but renders 1 in phase 1. The right rail's Brand voice / Yoast scoring is sample copy — when daemon scoring lands, the props replace the arrays in `SidebarRail.tsx`. Documented in the file as a phase-2 wiring contract so the next author doesn't reinvent the layout.
- **Both apps are now `tsc -b && vite build` clean.** Audit grep confirms zero `tailwind` / `@apply` / `Inter` / `Source Serif` / `JetBrains` in either `src/` tree. Stray hex colors are limited to `#ffffff` (white text on colored backgrounds) and one `#555` in the unchanged `ConnectionForm.tsx`.

### Remaining Wed Apr 22 – Tue Apr 28

- ✅ **Wed Apr 22** (pulled forward from Fri Apr 24): **ADK Go spike landed.** Agent loop runs end-to-end against local LM Studio (Gemma 4 e4b) — model calls stub tool, gets structured response, composes final text. Anthropic adapter wired into `go.mod` so provider-flip is a line change. PRD §11.1/§11.10 corrected inline.
- ⏳ **Wed Apr 22** (both): user-flow agreement on the 3 surfaces (60 min session); design-delivery conventions (share-link format, spec template, cadence).
- ⏳ **Wed Apr 22** (Elizabeth): formal delivery of API contract v1 to Nevena (`docs/api-contract-v1.md` already written in Pre-Phase 1).
- ⏳ **Wed–Thu** (Nevena): lightweight style tile (colors, type, spacing).
- ✅ **Thu Apr 23** (Elizabeth — pulled forward from Mon Apr 27): **MCP client in daemon landed.** `daemon/internal/mcp/` + `cmd/mcp-probe/` + `cmd/spike-mcp/` proven end-to-end against the live test store with gemma-4-e4b-it. Same morning: pre-signed manifest seeded with 25 real entries (pulled forward from later), three PRD §10/§8.4/§11.7 refinements, WooPayments abilities proposal for an external dev. Commits `9542b3a`, `1596ea1`, `ab8459d`.
- ⏳ **Fri Apr 24** (Elizabeth — pulled forward from Mon–Tue): **Begin Marketing agent persona.** Reads products, generates a rewrite proposal, lands in `In Review` via existing HTTP endpoints. If the MCP client slips into Friday, Marketing starts Monday on the original slot.
- ⏳ **Fri–Mon Apr 24, 27** (Nevena): kanban card anatomy + board-layout wireframes delivered by EOD Mon.
- ⏳ **Mon–Tue Apr 27–28** (Elizabeth): finish Marketing agent; translate kanban React from Nevena's designs as they land. Reclaimed-time options if Marketing is done early: kanban polish, run-log panel headstart, or pull Pricing agent skeleton forward from Phase 2.
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

### Mon May 4 (Figma UI alignment for v1 + batch-issues end-to-end)

Two threads in one day. First: Nevena landed the v1 visual direction in Figma over the weekend covering kanban, agent roster, issue detail / approval flow, and a "Review & approve · N meta rewrites" batch screen. `/ui/` got reskinned to match in three task passes. Second: the batch screen surfaced a daemon gap (no parent / group / batch concept on issues, every issue is independent), so the daemon grew the concept from scratch and the UI follow-on shipped against it. Branch: `feat/wpds-bridge-ui` (uncommitted on local at end of session). Plan at `daemon-batch-issues-plan.md`.

- ✅ **/ui/ — kanban + sidebar + top bar reskin.** Dark `LeftNav` with INBOX / FLEET / SETTINGS groupings (drops the per-persona items into a future Agents page); the unread badge moved from "My issues" to "Board" (in-review count) since My-issues is V2-deferred. Persona-pink "W" tile in the brand header. New `TopBar` (search input + Ask agent button) and `AskAgentDrawer` command-palette stub (⌘K toggle, Esc to close, suggestions list static for v1). Kanban columns lost their card chrome — header + thin colored accent line + cards. Card design refreshed: persona kind pill + mono ID, bold title, meta row (variants count + state-tinted timestamp + small persona-color avatar). New `PersonaAvatar` reusable component. Documented the WPDS sidebar exception in code: WPDS doesn't ship an inverse-surface token, so the dark sidebar borrows `--wpds-color-bg-interactive-neutral-strong` (the dark color used for primary buttons) — semantic stretch, but stays inside the design system. The alternative — a `--wa-sidebar-*` color exception — was rejected to avoid expanding the persona-color carve-out.
- ✅ **/ui/ — agent roster.** New `Agents.tsx` table-style roster fetching `/v1/agents`. Persona-color avatar tiles (V1 fallback for Figma's cartoon-character avatars; image asset pipeline lands later), name + slug, mandate (hardcoded per persona — daemon doesn't surface mandates), model dropdown (visual only), running/idle status (derived per-persona stub), enabled toggle (local React state only — daemon has no PATCH `/v1/agents` yet), kebab menu. Edit modal as visual-only — system-prompt textarea with character count, behavior chips, Save / Cancel / Delete (all just close). New `EditPersonaModal` component.
- ✅ **/ui/ — issue detail / approval flow.** `IssueDetail` lost its right rail (Figma collapsed to single column; the "Reversible · always" reminder moved into the action bar). Variant card redesigned: score chips became inline text labels with colored values (`SEO 91`, `Voice 96%`), radio mark on the right end of the header, Agent-pick pill inline. `Regenerate` tertiary button on the Proposed header (visual only — daemon has no regenerate endpoint). `ActionBar` rewritten as a discriminated `state: 'review' | 'done'` component — review state adds the Reversible-always green pill; done state shows ✓ + "Variant X written to WooCommerce / {scope} / Just now · snapshot saved · reversible from the Done column" with Undo + View-in-WooCommerce buttons. Post-approval no longer auto-navigates back to the board; the page stays mounted and the bar transitions. `SidebarRail.tsx` deleted. Connected-store dot in the sidebar footer switched from `--wpds-color-fg-content-success` (`#002900` — invisible against the dark sidebar) to `--wpds-color-stroke-surface-success` (`#8ac894` — readable). Deferred batch-products screen pending daemon work below.
- ✅ **Daemon — batch-issues concept end-to-end.** Lightweight grouping (no separate batch lifecycle): new `batches` metadata table + `issues.batch_id` FK; counts (Pending/Approved/Rejected) derived at read time via `LEFT JOIN ... GROUP BY` so they can never drift out of sync with children. Migration `004_add_batches.sql` applies cleanly against fresh + populated DBs. `approveOne` extracted from `handleApproveIssue` (a 137-line monolith that mixed request decode, DB load, PEP, dispatch, and response writing) into a pure helper + thin HTTP wrapper so the batch loop can reuse it. **Race-hardening landed in the same pass:** claim flip via `UPDATE ... WHERE status='in_review'` + `RowsAffected==1` gate before invoking PEP, with rollback on PEP/MCP failure — plugs a previously theoretical race that batches make real. New endpoints: `POST /v1/batches`, `GET /v1/batches`, `GET /v1/batches/:id`, `POST /v1/batches/:id/approve-all` (best-effort, sequential, always 200 with per-child `{ok, error?}` results — keeps audit granular per child), `POST /v1/batches/:id/reject-all` (bulk update with per-child `wrong_status` reporting). PEP `Request` gains `BatchID` as forward-compat (joining audit_invocations to issues.batch_id works retroactively, no audit table change). Seed script `seed-demo-batch-meta-rewrites.sh` creates a 7-product meta-rewrite batch in one POST. 10 new tests cover the round-trip, derived counts on empty + populated batches, mixed-failure approve-all (verifies the claim-rollback path), wrong-status reject-all, single-issue regression, race-loss short-circuit, `?batch_id=` filter, 404, confused-deputy `not_in_batch`, and route-registration sanity. All green.
- ✅ **/ui/ — batch review screen (Figma 210:28015).** New `BatchReview.tsx` matches Figma end-to-end: breadcrumb (`BATCH·xxxxxx` + intent pill), persona eyebrow, title + subtitle, KPI row (some hardcoded — daemon doesn't surface batch-level metrics yet), counter strip (Approved / Rejected / Pending + progress bar, all derived counts from the daemon), per-child accordion (header with `1/N · avatar · name · best-SEO/Voice scores · status pill · chevron`; expanded body shows Current + 3 variant columns side-by-side; auto-collapses to single column under 1024px), per-row footer (Reject + `Approve Variant {X}` — wires to single-issue endpoint), sticky bottom bar (Reject all + Approve & apply to store — wires to batch endpoint with the per-child variant picks). Kanban now routes cards with `batch_id` to `/batches/:id` and labels their meta line `batch XXXXXX` instead of variant count.

### Verified end-to-end (Mon May 4)

- `cd daemon && go test ./...` — all green; pep + manifest tests untouched, 10 new httpapi tests pass.
- `cd ui && npx tsc -b` — clean.
- Daemon restart against live env vars + `bash daemon/scripts/seed-demo-batch-meta-rewrites.sh` against the staging store created batch `bddb2d07` with 7 in_review children. `GET /v1/batches` derived counts read correctly (`{total: 7, pending: 7, approved: 0, rejected: 0}`). `Issue.batch_id` flows to the wire. UI at `/batches/bddb2d07-…` renders all 7 children, variant pickers work, per-row Approve and top-level Approve all both wire to the right endpoints.

### Findings from the Mon May 4 session

- **WPDS doesn't ship a true inverse / dark-surface token.** The closest match is `--wpds-color-bg-interactive-neutral-strong` (dark color used for primary-button backgrounds). The new sidebar borrows it; mild semantic stretch (button bg used as surface bg) but stays inside the design system. A clean fix at the WPDS level would add `--wpds-color-bg-surface-inverse` (and a paired foreground) so the borrow can swap out.
- **`--wpds-color-fg-content-success` is `#002900` — designed for green text on light surfaces, effectively invisible on the dark sidebar.** The connected-store indicator dot uses `--wpds-color-stroke-surface-success` (`#8ac894`) instead. Of the WPDS green tokens (`bg-surface-success`, `bg-surface-success-weak`, `fg-content-success`, `fg-content-success-weak`, `stroke-surface-success`, `stroke-surface-success-strong`) only `stroke-surface-success` and `bg-surface-success` read on a dark surface; the rest are tuned for light backgrounds. A token-by-token color audit before any future dark surface lands.
- **Cartoon-character avatars vs. persona-color tiles.** Figma's agent roster shows illustrated avatars (DiceBear / notion-avatar style). V1 falls back to the persona-color tiles already established in CLAUDE.md's documented exception (avatar squares + kind pill). Image asset pipeline lands later; the persona-color tiles stay as the fallback.
- **Variants and batches are orthogonal axes.** Variants = N alternative bodies for one issue (`proposal.target.variants[]`). Batches = N issues sharing a parent (`issues.batch_id`). Either axis composes with the other. The daemon resists overloading the proposal model to carry batch semantics; the UI uses different routes (`/issues/:id` vs. `/batches/:id`) to match. This was the load-bearing scope decision in the design phase.
- **Best-effort batch approve-all keeps audit granular.** Each child runs through `pep.Invoke` independently and gets its own audit row with full chain-of-identity (now including `BatchID`). The endpoint always returns 200 with per-child `{ok, error?}` so the UI can render mixed outcomes. The alternative — all-or-nothing transactional semantics — would have rolled back successful children on a single PEP denial, which is operationally awkward and forfeits the clean per-child audit story. Sequential, not parallel: SQLite's writer lock + audit row attribution makes goroutines a net loss for N=7.
- **Race-hardening was free with the refactor.** Extracting `approveOne` for the batch loop made it natural to fix the SELECT-then-UPDATE race that single-issue approve had latently. Cheapest fix: claim with `UPDATE ... WHERE status='in_review'` + `RowsAffected==1`, rollback to in_review on PEP/MCP failure. The kanban briefly shows the card under Drafting during the in-flight window — a feature, not a bug (visualises that work is in progress).
- **`go run` daemon caches stale routes.** The running daemon process from earlier in the session was built before the batch routes landed; it returned 404 on POST `/v1/batches` until restarted. SQLite at `~/.wooagent/wooagent.db` and the bearer token in env both persisted across restart, so no state loss — but worth noting that the dev loop is `kill && go run` for any handler change.

### Deferred / known V1 caveats

- `Issue` payload doesn't yet carry a variant count; `Kanban.tsx` shows `3 variants` as a placeholder for content cards. Wire to `proposal.target.variants.length` when the daemon's list endpoint surfaces it (or an explicit `variant_count` field).
- `Agents.tsx` toggle, model dropdown, and edit-modal Save/Delete are all local React state. Daemon needs `PATCH /v1/agents/:persona` to make them persist.
- Connected-store footer shows the daemon hostname; daemon doesn't yet surface a separate WooCommerce store identifier or environment label.
- `Regenerate` (issue detail), `Undo` (done state), and `View in WooCommerce` (done state) are visual-only; daemon needs corresponding endpoints / a store URL.
- "My issues" sidebar item is rendered as a paused/V2 placeholder; non-functional in v1.
- Batch creation UI doesn't exist — V1 creates batches via `POST /v1/batches` from seed scripts only. An autonomous agent loop that produces batches from a single prompt is its own follow-up.
- KPI tiles on the batch screen ("Brand voice match", "SEO score", "Est. impact") are hardcoded. Wire to a daemon batch-aggregates endpoint when one exists.

### Planned (still to land before May 5)

- ⏳ **Elizabeth:** Pricing agent (structured numeric diff); Sales Support agent (customer-reply draft); propose-mode across all three; run log stores every model call + ability call + state change.
- ⏳ **Nevena:** review/approval mockups covering all three diff shapes (prose, numeric, message); start onboarding flow mockups.

### Exit criteria

- [ ] All three personas generate real issues.
- [x] Operator can approve a Marketing rewrite and the product description updates on the test store.
- [x] Review screen renders the prose/content shape (single-product + batch) at rough fidelity to Nevena's designs. Numeric (Pricing) and message (Sales Support) shapes pending the persona work above.

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
- **Companion Plugin scope pull-forward (Apr 21):** PRD §10.4 originally scoped the Companion Plugin's non-pairing abilities for v0.3. Test-store probe revealed WooCommerce has not registered `woocommerce/*` abilities on the current ecosystem — only 5 core/Jetpack abilities surface. Rather than fall back to WC REST (which sidesteps the PRD's MCP-native bet), we're registering `wooagent-*` CRUD abilities in our own plugin now. They flow through the existing MCP Adapter's execute-tool — no daemon changes needed. When WC eventually ships native abilities, ours deprecate gracefully. Full plan in `companion-plugin-v0.1-plan.md`. Merged via PR #1. Tradeoff: weakens the "zero-install on Pressable/WPE/WordPress.com" pitch in the short term; pulls ~2-3 days of PHP into Phase 1, absorbed by Phase 4 buffer.
- **WP Abilities API gotchas uncovered (Apr 21):** five debug-redeploy cycles were needed before registration worked. Three rules aren't obvious from the WP docblocks: (1) ability names must match `/^[a-z0-9-]+\/[a-z0-9-]+$/` — **one slash only**, despite the docblock saying "forward slashes" plural; (2) categories register on `wp_abilities_api_categories_init`, abilities on `wp_abilities_api_init` (both `wp_`-prefixed, fire in that order); (3) MCP Adapter has a second opt-in beyond `show_in_rest` — abilities need `meta.mcp.public = true` to be invokable via `mcp-adapter-execute-ability`. All three are enforced with silent `_doing_it_wrong()` notices outside `WP_DEBUG`. Captured in `companion-plugin-v0.1-plan.md` "Debugging lessons" section so the daemon's MCP client team (and any future ability authors) don't repeat the rediscovery.
- **No-WPCOM-as-hard-dependency locked in (Apr 23):** codified as PRD §4 non-goal 7 and reflected in §11.4 (Companion Plugin = baseline ability provider) and §11.7 (WPCOM-enhanced mode = optional additive tier). Trigger: inspecting the WooCommerce AI plugin revealed it routes its 50-tool progressive MCP registry through a WPCOM proxy because the browser is their only always-available runtime. WooAgent OS's Go daemon *is* that runtime, so coupling to WPCOM would be a self-inflicted architectural trap. Framing for Automattic leadership: stores that *do* connect to WPCOM get a measurably more capable WooAgent OS deployment (§11.7), so the commercial incentive to connect is preserved — local-first is the guarantee, enhanced is the reward.
- **Ability trust model locked in (Apr 23):** PRD §8.4. Trust decisions cannot come from the LLM's self-report (prompt injection, hallucinated calls, rug-pulls). Two-layer design: (1) a pre-signed manifest shipped with the daemon binary, listing fully-qualified ability names + schema hashes + version constraints + persona mappings + default scope; (2) a deterministic non-LLM Go middleware (Policy Enforcement Point) between orchestrator and MCP client that verifies trust state, schema match, persona scope, argument validity, operator policy predicates, budgets, and scope sufficiency on every invocation. Short-lived per-invocation capability tokens, chain-of-identity logging, drift-detection auto-demotion. Framework borrowed from the dev.to article on preventing tool misuse + follow-up material on PEPs, OAuth 2.0 workload identity, and RFC 8693 delegation chains. §15 picks it up as security item #9. Manifest seed landed same day with 25 real entries.
- **MCP three-meta-tool pattern adopted (Apr 23):** the WP MCP Adapter exposes `discover-abilities`, `get-ability-info`, `execute-ability` rather than one MCP tool per registered ability. Avoids flooding the model context with dozens of tool schemas, matches the "progressive MCP" pattern from the Big Ocean demo. Phase 1 spike uses a single generic `execute_ability` ADK tool; Phase 1–2 direction is to expose N semantic ADK tools per persona that wrap `execute-ability` under the hood — better model ergonomics without giving up the meta-tool transport.
