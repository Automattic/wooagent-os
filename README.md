# WooAgent OS

**Open-source, local-first agent operating system for WooCommerce store operators.**

A fleet of opinionated AI agents — Marketing, Pricing, Inventory, Accounting, Reporting, Sales Support — runs continuously against your live WooCommerce store, coordinated by a Chief of Staff meta-agent. Each unit of agent work is a trackable issue on a review-queue command center. Nothing gets written to the store without the operator seeing a diff and approving it.

## What makes it different

- **MCP-native.** WooAgent OS connects to any WordPress/WooCommerce site via the WordPress MCP Adapter. Every plugin that registers abilities through the WP Abilities API — WooCommerce, Yoast SEO, ACF, Gravity Forms, 70+ others — is automatically available to the agent fleet. No bespoke integrations.
- **Local-first, no WordPress.com required.** The whole stack runs on a single operator machine against any self-hosted Woo store. Store data, run logs, model calls, and agent state stay on the host. A Jetpack connection is optional and unlocks additional WPCOM-routed abilities (an enhanced tier), but is never a prerequisite for core functionality.
- **Model-agnostic.** Point the daemon at any frontier provider (Anthropic, Google, OpenAI, xAI) or any local runtime (LM Studio, Ollama, llama.cpp). The agent fleet runs identically.
- **Pre-signed, policy-enforced abilities.** The agent doesn't get to invent what it can call. WooAgent OS ships a curated manifest of trusted plugin abilities with canonical schema hashes and per-persona scope. A deterministic non-LLM middleware verifies every invocation before it reaches the store. Prompt-injected or hallucinated tool calls are denied, not executed. Plugin updates that silently change a schema auto-demote the affected ability to unapproved until the operator reviews.
- **Propose / approve.** On top of trust enforcement, agents never apply changes directly by default. Every write lands as an issue with a diff the operator reviews. A run log captures every model call, every ability call, and every state change.

## Data handling

WooAgent OS runs on your machine and talks to your store directly. The daemon, the SQLite store, the run log, and the Companion Plugin all stay on your host. The only data that leaves is the prompt the agent sends to whichever LLM provider you've configured — so what travels depends on which agent is running and which provider you've pointed it at.

What each persona sends in a prompt:

- **Marketing** — current product copy (title, description, attributes). No customer data.
- **Pricing** — product attributes plus the agent's `web_search` queries to retailer sites. No customer data.
- **Inventory** — SKU, current stock, sales velocity. No customer data.
- **Sales Support** — order context for one recent order: order number, status, total, line items, and the customer's **first name only**. The customer's surname, email, and shipping/billing address are dropped before the prompt is constructed and never reach the LLM provider. The full record (with PII intact) stays on the daemon and is what you see in the proposal preview and what gets dispatched when you approve. If your store handles orders where even the first name + line items combination is sensitive, point Sales Support at a local model (Ollama, LM Studio, llama.cpp) — same propose-approve loop, zero egress.
- **Chief of Staff (v0.1)** — only metadata about other agents' open issues (persona, status, summary). Does not see store data.

Reporting and Accounting ship in v0.2 and will get their own data-handling line when they land.

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

## Install

> **Internal testers (pre-public):** grab the binary directly from the [a8c Releases page](https://***REMOVED***/Automattic/wooagent-os/releases) — download the archive for your platform, verify against `SHA256SUMS`, extract to `~/.wooagent/bin/`, and run `wooagent init && wooagent run`. The one-line installer below assumes a public mirror with a tagged release; until that's in place, the curl URL will 404.

If you just want to run WooAgent OS — not develop on it — grab the latest release with one line:

```bash
curl -fsSL https://raw.githubusercontent.com/automattic/wooagent-os/trunk/install.sh | bash
```

The installer detects your platform (macOS / Linux on amd64 or arm64), downloads the matching binary from the latest GitHub Release, verifies its SHA-256, and drops it in `~/.wooagent/bin/wooagent`. After install:

```bash
wooagent init      # creates ~/.wooagent and mints an initial auth token
wooagent run       # serves http://localhost:7777
```

Open <http://localhost:7777> in a browser — the React UI is baked into the binary and served from the same daemon. No second process, no separate Vite dev server.

Pin a specific version with `WOOAGENT_VERSION=v0.1.0`. Use `WOOAGENT_INSTALL_DIR=/usr/local/bin` to install into a system location instead. Windows isn't supported by the script — download the `.zip` from the [Releases page](https://github.com/automattic/wooagent-os/releases) directly, or use WSL.

## Quickstart (dev)

The release binary bundles the UI; for source-tree development you have two options.

**Option A — Vite dev server** (recommended for UI iteration). Two terminals; the daemon's CORS allows cross-origin from `:5173`.

```bash
# Terminal 1 — daemon
cd daemon
go run ./cmd/wooagent init           # creates ~/.wooagent and mints an initial auth token
go run ./cmd/wooagent run            # serves http://localhost:7777 (with the placeholder UI)

# Terminal 2 — Vite dev server
cd ui
npm install
npm run dev                          # serves http://localhost:5173 with hot reload
```

Open <http://localhost:5173>, paste the daemon URL + token from Terminal 1.

**Option B — embedded UI** (matches the release-binary behavior; useful for testing the install flow).

```bash
cd ui && npm install && npm run build
bash scripts/build-ui-into-daemon.sh
cd daemon && go run ./cmd/wooagent run    # http://localhost:7777 now serves the bundled UI
```

## Connecting a WooCommerce store

The daemon talks to any WooCommerce store running WP 6.9+ with the MCP Adapter installed. You also need the **WooAgent Companion** plugin on the store — it registers the `wooagent-*` ability surface the agent fleet uses.

Grab the plugin zip from the same GitHub Release as the daemon (`wooagent-companion.zip`) — the release pipeline packages both off the same tag. To build it from source:

```bash
bash scripts/build-companion-plugin-zip.sh   # writes dist/wooagent-companion.zip
```

Upload the zip via **wp-admin → Plugins → Add New → Upload Plugin**. Then walk the daemon's first-run UI to pair: type your store URL, click **Open wp-admin → Pair device**, click Approve. The Companion Plugin's pair handshake mints a device token the daemon stores in your OS keychain.

## Cutting a release

Releases are automated. Tag a commit and push the tag:

```bash
git tag v0.1.0
git push origin v0.1.0
```

`.github/workflows/release.yml` runs [GoReleaser](https://goreleaser.com/) which cross-compiles the `wooagent` binary for darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, and windows/amd64, packages the Companion Plugin zip, generates a `SHA256SUMS` file, and publishes everything as a GitHub Release. Tags like `v0.1.0-rc.1` or `v0.1.0-alpha.2` are auto-flagged as prereleases.

Validate the build locally before tagging:

```bash
goreleaser release --snapshot --clean
```

## Status

Phase 2 complete (Mon May 4). All three Phase-1 personas — **Marketing, Pricing, Sales Support** — now propose changes that land as `in_review` issues, render in their respective diff layouts, and write through to the connected WooCommerce store on operator approval.

- **Go daemon** — CLI surface, SQLite store + migrations, REST API with bearer auth, agents/issues/runs schemas, embedded prompt + skill registry, telemetry scaffolding.
- **MCP client** — Streamable HTTP + JSON-RPC 2.0 client with session-id tracking, Basic Auth, and pass-through to the WP MCP Adapter's three-meta-tool pattern (`discover-abilities`, `get-ability-info`, `execute-ability`).
- **Companion plugin v0.1** — `wooagent-products/*`, `wooagent-orders/*`, `wooagent-customers/*` CRUD abilities, verified end-to-end against a real WooCommerce store over MCP.
- **Pre-signed ability manifest** — shipped default with 25 real entries covering the Companion Plugin baseline, the WooCommerce AI plugin's local abilities, Jetpack Forms, and WP core — every entry with a SHA-256 schema hash computed from live schemas, so drift detection is live from day one. Operator overlay support at `~/.wooagent/manifest.json`. A `manifest-compute` dev tool refreshes the seed as plugin schemas change.
- **Policy Enforcement Point — V1** (`daemon/internal/pep/`) — the deterministic non-LLM gate from PRD §8.4.2. Every store-mutating MCP call now routes through `pep.Invoke`; the rule "no orchestrator → MCP shortcut" is enforced by removing the direct MCP handle from the HTTP server entirely. V1 enforces two of the six §8.4.2 checks (trust state, persona scope) and writes a chain-of-identity audit row per invocation (`audit_invocations` table, append-only) regardless of outcome. Denials return typed reason codes (`ability_unapproved`, `persona_forbidden`) mapped to HTTP 403/422/429 by the approve handler. Schema validation, policy predicates, budgets, and capability tokens are stubbed pass-through behind the same `Decision`/`Request`/`Intent` surface so post-V1 phases are additive.
- **Persona registry** (`daemon/internal/personas/`) — shared `Persona` interface with side-effect registration on import. The daemon spawns enabled personas sequentially on boot via `RunAndPersist`, which checks `agents.enabled`, guards against duplicate seeding, and writes the issue. No more per-persona CLI; the existing `cmd/persona-{slug}/` binaries became thin debug wrappers around the same code path.
- **Marketing persona, propose → review → approve → write** — reads products via `wooagent-products/{list,get}`, generates a rewrite proposal via local LM Studio (`gemma-4-e4b`), surfaces it as an `in_review` issue, and on operator approval writes the new copy back via `wooagent-products/update`. The previous copy is snapshotted before write so the change is reversible from the Done column.
- **Pricing persona, propose → review → approve → write** — Claude Haiku 4.5 with the Anthropic native `web_search` tool. The `pricing-benchmark` skill (`daemon/internal/registry/skills/pricing-benchmark/v1.yaml`, embedded into the binary at build time via `//go:embed`) names the preferred mid-tier retailers (J.Crew, Madewell, Aritzia, Everlane, Quince, COS for apparel; Parachute, Anthropologie, West Elm, Crate & Barrel, Coyuchi for home goods) plus the search method (`site:` queries first, cap at 6). Emits `product_price_change` proposals with structured numeric output (current → proposed, observed low/median/high, percent change, sources with retailer attribution) and a `no_proposal` contract that refuses to invent comparables when grounding is thin. Approve dispatches `wooagent-products/update` with a 2dp-canonical `regular_price` string.
- **Sales Support persona, propose → review → approve → write** — Claude Haiku 4.5, brand-voice system prompt for warm small-batch home-goods customer support. Picks the most recent order in `processing` or `completed` status, drafts a customer-facing note (or returns `no_proposal` for shapes without comparable retail comps). Approve dispatches `wooagent-orders/add-note` with the `is_customer_note` flag — operator chooses customer-facing email vs internal wp-admin note via `target.note_type`.
- **Daemon UI** (`/ui/`) — sidebar layout (App / Agents / Connected store), review queue backed by live `/v1/issues`. **`IssueDetail` branches by `proposal.type`:** prose (Marketing) keeps the variant-card layout; **numeric** (Pricing) renders current → proposed price block, observed-range bar with low/median/high markers, rationale paragraph, and a sources list with persona-blue retailer pills (J.Crew / Madewell / etc.); **message** (Sales Support) renders order-context card (customer + line items) + email-shaped preview with a `To: <customer>` chrome line for customer-facing notes. Sticky `ActionBar` adapts: "Approve & apply price" / "Approve & send" / "Approve & save note" depending on type. **The queue dedupes batch children** — fetches `/v1/batches` alongside issues and renders one synthetic row per batch with a state-aware meta line (`5 of 7 rewrites pending`). Built on the WordPress Design System (`@wordpress/ui` + `@wordpress/components`, see `CLAUDE.md`). Mobile-responsive: sidebar collapses to an off-canvas drawer at <768px, the queue scrolls horizontally with snap, IssueDetail stacks at <1024px.
- **Agent runtime, end-to-end** — verified against a live WooCommerce store across all three personas. Pricing persona produced an Indigo-Dyed Throw Pillow proposal grounded at Crate & Barrel ($59.95), Anthropologie ($68), Parachute ($72), proposing $48 → $56 (+16.67%) with cited URLs; approved through the UI, `regular_price` updated on the staging store.

Up next:

- **Phase 3 (May 5–12)** — design refinement on the rough build (Tue May 5), then full onboarding flow (welcome → store URL → auth picker → pairing code → model provider → fleet deploy → done) plus error states. TurnEvent recorder wiring so every model call + ability invocation + state transition writes a `turn_events` row, and the run-log panel that consumes it. Companion Plugin v0.2 with native device-pair (replaces the App Password path).

## License

Apache 2.0. See `LICENSE`.
