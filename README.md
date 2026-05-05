# WooAgent OS

**Open-source, local-first agent operating system for WooCommerce store operators.**

A fleet of opinionated AI agents — Marketing, Pricing, Inventory, Accounting, Reporting, Sales Support — runs continuously against your live WooCommerce store, coordinated by a Chief of Staff meta-agent. Each unit of agent work is a trackable issue on a kanban-style command center. Nothing gets written to the store without the operator seeing a diff and approving it.

## What makes it different

- **MCP-native.** WooAgent OS connects to any WordPress/WooCommerce site via the WordPress MCP Adapter. Every plugin that registers abilities through the WP Abilities API — WooCommerce, Yoast SEO, ACF, Gravity Forms, 70+ others — is automatically available to the agent fleet. No bespoke integrations.
- **Local-first, no WordPress.com required.** The whole stack runs on a single operator machine against any self-hosted Woo store. Store data, run logs, model calls, and agent state stay on the host. A Jetpack connection is optional and unlocks additional WPCOM-routed abilities (an enhanced tier), but is never a prerequisite for core functionality.
- **Model-agnostic.** Point the daemon at any frontier provider (Anthropic, Google, OpenAI, xAI) or any local runtime (LM Studio, Ollama, llama.cpp). The agent fleet runs identically.
- **Pre-signed, policy-enforced abilities.** The agent doesn't get to invent what it can call. WooAgent OS ships a curated manifest of trusted plugin abilities with canonical schema hashes and per-persona scope. A deterministic non-LLM middleware verifies every invocation before it reaches the store. Prompt-injected or hallucinated tool calls are denied, not executed. Plugin updates that silently change a schema auto-demote the affected ability to unapproved until the operator reviews.
- **Propose / approve.** On top of trust enforcement, agents never apply changes directly by default. Every write lands as an issue with a diff the operator reviews. A run log captures every model call, every ability call, and every state change.

## Architecture

- **Go daemon** (`daemon/`) — headless single binary. Owns the agent fleet, MCP client, issue queue, auth, local storage (SQLite), and the REST API the UI and CLI consume. Cross-platform, no external runtime dependencies.
- **React UI** (`ui/`) — standalone Vite + `@wordpress/components` app. Connects to any daemon over HTTP. Ships on its own release cadence; runs as a hosted build, a local `wooagent ui` command, or self-hosted static assets.
- **Companion WordPress plugin** (`companion-plugin/`) — installed on the WooCommerce store. Registers the `wooagent-*` ability surface the agent fleet consumes over MCP. Becomes thinner as WooCommerce's own native ability surface fills in.

## Repo layout

- `daemon/` — Go daemon. Entry point `cmd/wooagent`; internals under `internal/`.
- `ui/` — Standalone React UI (Vite + `@wordpress/components`).
- `companion-plugin/` — WordPress plugin source.
- `docs/` — API contracts and shared design notes (`api-contract-v1.md`).
- `dist/` — Build artifacts (plugin zips, UI bundles). Gitignored.

## Quickstart (dev)

Two terminals.

```bash
# Terminal 1 — daemon
cd daemon
go run ./cmd/wooagent init           # creates ~/.wooagent and mints an initial auth token
go run ./cmd/wooagent run            # serves http://localhost:7777

# Terminal 2 — UI
cd ui
npm install
npm run dev                          # serves http://localhost:5173
```

Open `http://localhost:5173`, paste the daemon URL and token from Terminal 1, and you're connected.

## Connecting a WooCommerce store

The daemon talks to any WooCommerce store running WP 6.9+ with the MCP Adapter installed. You also need the **WooAgent Companion** plugin on the store — it registers the `wooagent-*` ability surface the agent fleet uses.

```bash
# Build the plugin zip
cd companion-plugin
zip -r ../dist/wooagent-companion-0.1.0.zip . -x "*.DS_Store"
```

Upload the zip via **wp-admin → Plugins → Add New → Upload Plugin**. Authenticate the daemon with a WordPress Application Password (**Users → Profile → Application Passwords**). A native device-pair flow ships in a later plugin version.

## Status

Phase 2 complete (Mon May 4). All three Phase-1 personas — **Marketing, Pricing, Sales Support** — now propose changes that land as `in_review` issues, render in their respective diff layouts, and write through to the connected WooCommerce store on operator approval.

- **Go daemon** — CLI surface, SQLite store + migrations, REST API with bearer auth, agents/issues/runs schemas, embedded prompt + skill registry, telemetry scaffolding.
- **MCP client** — Streamable HTTP + JSON-RPC 2.0 client with session-id tracking, Basic Auth, and pass-through to the WP MCP Adapter's three-meta-tool pattern (`discover-abilities`, `get-ability-info`, `execute-ability`).
- **Companion plugin v0.1** — `wooagent-products/*`, `wooagent-orders/*`, `wooagent-customers/*` CRUD abilities, verified end-to-end against a real WooCommerce store over MCP.
- **Pre-signed ability manifest** — shipped default with 25 real entries covering the Companion Plugin baseline, the WooCommerce AI plugin's local abilities, Jetpack Forms, and WP core — every entry with a SHA-256 schema hash computed from live schemas, so drift detection is live from day one. Operator overlay support at `~/.wooagent/manifest.json`. A `manifest-compute` dev tool refreshes the seed as plugin schemas change.
- **Policy Enforcement Point — V1** (`daemon/internal/pep/`) — the deterministic non-LLM gate from PRD §8.4.2. Every store-mutating MCP call now routes through `pep.Invoke`; the rule "no orchestrator → MCP shortcut" is enforced by removing the direct MCP handle from the HTTP server entirely. V1 enforces two of the six §8.4.2 checks (trust state, persona scope) and writes a chain-of-identity audit row per invocation (`audit_invocations` table, append-only) regardless of outcome. Denials return typed reason codes (`ability_unapproved`, `persona_forbidden`) mapped to HTTP 403/422/429 by the approve handler. Schema validation, policy predicates, budgets, and capability tokens are stubbed pass-through behind the same `Decision`/`Request`/`Intent` surface so post-V1 phases are additive.
- **Persona registry** (`daemon/internal/personas/`) — shared `Persona` interface with side-effect registration on import. The daemon spawns enabled personas sequentially on boot via `RunAndPersist`, which checks `agents.enabled`, guards against duplicate seeding, and writes the issue. No more per-persona CLI; the existing `cmd/persona-{slug}/` binaries became thin debug wrappers around the same code path.
- **Marketing persona, propose → review → approve → write** — reads products via `wooagent-products/{list,get}`, generates a rewrite proposal via local LM Studio (`gemma-4-e4b`), surfaces it as an `in_review` issue, and on operator approval writes the new copy back via `wooagent-products/update`. The previous copy is snapshotted before write so the change is reversible from the Done column.
- **Pricing persona, propose → review → approve → write** — Claude Haiku 4.5 with the Anthropic native `web_search` tool. The `pricing-benchmark` skill (`daemon/skills/pricing-benchmark/v1.yaml`) names the preferred mid-tier retailers (J.Crew, Madewell, Aritzia, Everlane, Quince, COS for apparel; Parachute, Anthropologie, West Elm, Crate & Barrel, Coyuchi for home goods) plus the search method (`site:` queries first, cap at 6). Emits `product_price_change` proposals with structured numeric output (current → proposed, observed low/median/high, percent change, sources with retailer attribution) and a `no_proposal` contract that refuses to invent comparables when grounding is thin. Approve dispatches `wooagent-products/update` with a 2dp-canonical `regular_price` string.
- **Sales Support persona, propose → review → approve → write** — Claude Haiku 4.5, brand-voice system prompt for warm small-batch home-goods customer support. Picks the most recent order in `processing` or `completed` status, drafts a customer-facing note (or returns `no_proposal` for shapes without comparable retail comps). Approve dispatches `wooagent-orders/add-note` with the `is_customer_note` flag — operator chooses customer-facing email vs internal wp-admin note via `target.note_type`.
- **Daemon UI** (`/ui/`) — sidebar layout (App / Agents / Connected store), 4-column kanban backed by live `/v1/issues`. **`IssueDetail` branches by `proposal.type`:** prose (Marketing) keeps the variant-card layout; **numeric** (Pricing) renders current → proposed price block, observed-range bar with low/median/high markers, rationale paragraph, and a sources list with persona-blue retailer pills (J.Crew / Madewell / etc.); **message** (Sales Support) renders order-context card (customer + line items) + email-shaped preview with a `To: <customer>` chrome line for customer-facing notes. Sticky `ActionBar` adapts: "Approve & apply price" / "Approve & send" / "Approve & save note" depending on type. **Kanban dedupes batch children** — fetches `/v1/batches` alongside issues and renders one synthetic card per batch with a state-aware meta line (`5 of 7 rewrites pending`). Built on the WordPress Design System (`@wordpress/ui` + `@wordpress/components`, see `CLAUDE.md`). Mobile-responsive: sidebar collapses to an off-canvas drawer at <768px, kanban scrolls horizontally with snap, IssueDetail stacks at <1024px.
- **Agent runtime, end-to-end** — verified against a live WooCommerce store across all three personas. Pricing persona produced an Indigo-Dyed Throw Pillow proposal grounded at Crate & Barrel ($59.95), Anthropologie ($68), Parachute ($72), proposing $48 → $56 (+16.67%) with cited URLs; approved through the UI, `regular_price` updated on the staging store.
- **Marketing-prototype** (`/marketing-prototype/`) — standalone WPDS-based demo deployed to GHES Pages at https://github.a8c.com/pages/Automattic/wooagent-os/. Same component primitives as the daemon UI; mock data lets teammates click through ContentReview / CampaignPlanner / EmailReview without spinning up the daemon.

Up next:

- **Phase 3 (May 5–12)** — design refinement on the rough build (Tue May 5), then full onboarding flow (welcome → store URL → auth picker → pairing code → model provider → fleet deploy → done) plus error states. TurnEvent recorder wiring so every model call + ability invocation + state transition writes a `turn_events` row, and the run-log panel that consumes it. Companion Plugin v0.2 with native device-pair (replaces the App Password path).

## License

Apache 2.0. See `LICENSE`.
