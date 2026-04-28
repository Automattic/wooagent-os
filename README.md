# WooAgent OS

**Open-source, local-first agent operating system for WooCommerce store operators.**

A fleet of opinionated AI agents — Marketing, Pricing, Inventory, Accounting, Reporting, Sales Support — runs continuously against your live WooCommerce store, coordinated by a Chief of Staff meta-agent. Each unit of agent work is a trackable issue on a kanban-style command center. Nothing gets written to the store without the operator seeing a diff and approving it.

See `wooagent-os-prd V2.md` for the full PRD and `WooAgent_Pitch.md` for the short version.

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
- Top-level `.md` — PRD, pitch, and related planning documents.

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

The daemon talks to any WooCommerce store running WP 6.9+ with the MCP Adapter installed. You also need the **WooAgent Companion** plugin on the store — it registers the `wooagent-*` ability surface the agent fleet uses. Rationale and debugging notes live in `companion-plugin-v0.1-plan.md`.

```bash
# Build the plugin zip
cd companion-plugin
zip -r ../dist/wooagent-companion-0.1.0.zip . -x "*.DS_Store"
```

Upload the zip via **wp-admin → Plugins → Add New → Upload Plugin**. Authenticate the daemon with a WordPress Application Password (**Users → Profile → Application Passwords**). A native device-pair flow ships in a later plugin version.

## Status

Phase 1 close to wrapping. Live as of 2026-04-27:

- **Go daemon** — CLI surface, SQLite store + migrations, REST API with bearer auth, agents/issues/runs schemas, embedded prompt + skill registry, telemetry scaffolding.
- **MCP client** — Streamable HTTP + JSON-RPC 2.0 client with session-id tracking, Basic Auth, and pass-through to the WP MCP Adapter's three-meta-tool pattern (`discover-abilities`, `get-ability-info`, `execute-ability`).
- **Companion plugin v0.1** — `wooagent-products/*`, `wooagent-orders/*`, `wooagent-customers/*` CRUD abilities, verified end-to-end against a real WooCommerce store over MCP.
- **Pre-signed ability manifest** — shipped default with 25 real entries covering the Companion Plugin baseline, the WooCommerce AI plugin's local abilities, Jetpack Forms, and WP core — every entry with a SHA-256 schema hash computed from live schemas, so drift detection is live from day one. Operator overlay support at `~/.wooagent/manifest.json`. A `manifest-compute` dev tool refreshes the seed as plugin schemas change.
- **Agent runtime, end-to-end** — ADK Go agent loop verified against a live WooCommerce store: model → ADK orchestrator → MCP client → `mcp-adapter-execute-ability` → real product data → grounded text. Driven by local `gemma-4-e4b-it` on LM Studio; Anthropic and other providers slot in via config.
- **Marketing persona, propose → review → approve → write** — first production persona ships an end-to-end loop. The agent reads products via `wooagent-products/{list,get}`, generates a rewrite proposal, surfaces it as an `in_review` issue, and on operator approval writes the new copy back to the store via `wooagent-products/update`. The previous copy is snapshotted before write so the change is reversible from the Done column.
- **Daemon UI** (`/ui/`) — sidebar layout (App / Agents / Connected store), 4-column kanban backed by live `/v1/issues`, IssueDetail with KPI row + variant card + brand-voice/Yoast right rail + sticky Approve/Reject action bar wired to the daemon. Built on the WordPress Design System (`@wordpress/ui` + `@wordpress/components`, see `CLAUDE.md`). Mobile-responsive: sidebar collapses to an off-canvas drawer at <768px, kanban scrolls horizontally with snap, IssueDetail stacks at <1024px.
- **Marketing-prototype** (`/marketing-prototype/`) — standalone WPDS-based demo deployed to GHES Pages at https://***REMOVED***/pages/Automattic/wooagent-os/. Same component primitives as the daemon UI; mock data lets teammates click through ContentReview / CampaignPlanner / EmailReview without spinning up the daemon.

Up next:

- **Apr 28** — first end-to-end demo recording on the daemon UI.
- **Apr 29–30** — design refinement pass with Nevena. The build is rough; visual fidelity is the priority before more screens land on top.
- **Phase 2 (Apr 29 – May 5)** — Pricing agent (numeric diff) and Sales Support agent (customer-reply draft) so all three Phase-1 personas are writing through Approve. Review/run-log panel covering every model call, ability call, and state change. Numeric-diff and message-diff variants of IssueDetail.
- **Phase 3 (May 6–12)** — onboarding flow (welcome → store URL → auth → pairing → model provider → fleet deploy). Companion Plugin v0.2 with native device-pair (replaces the App Password path).
- **Backstage** — Policy Enforcement Point middleware (the deterministic gate between orchestrator and MCP client), daemon schema for multi-variant proposals, embedding the built UI in the daemon binary as static assets.

## License

Apache 2.0. See `LICENSE`.
