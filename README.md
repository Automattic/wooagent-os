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

**Your responsibilities.** Using WooAgent OS means you agree to comply with your LLM provider's terms of service and acceptable-use policies — the daemon doesn't intermediate that for you. You're responsible for your store's privacy and data-protection obligations (GDPR, CCPA, and any other laws that apply to your customers), including any disclosures you owe customers about automated processing of order data. If you handle data that can't leave your jurisdiction at all, the local-model path (Ollama / LM Studio / llama.cpp) keeps everything on-host.

## What WooAgent OS can change on your store

WooAgent OS can write to your live WooCommerce store. Today, that means: product descriptions and titles (Marketing), regular prices (Pricing), and customer-facing or internal order notes (Sales Support). Future personas extend this surface — Inventory adjusts stock levels and Accounting reconciles entries when they ship.

Every write goes through a propose-approve loop. Agents never apply changes directly. You see a diff in the review queue, and the daemon only dispatches the call when you click **Approve**.

**You are responsible** for reviewing each proposal before approving, for maintaining store backups, and for verifying writes against your store's data after they land. WooAgent OS is provided "AS IS" — see [`LICENSE`](./LICENSE) §7 (Disclaimer of Warranty) and §8 (Limitation of Liability) for the full text.

## Architecture

- **Go daemon** (`daemon/`) — headless single binary. Owns the agent fleet, MCP client, issue queue, auth, local storage (SQLite), and the REST API the UI and CLI consume. Cross-platform, no external runtime dependencies.
- **React UI** (`ui/`) — standalone Vite + `@wordpress/components` app. Connects to any daemon over HTTP. Ships on its own release cadence; runs as a hosted build, a local `wooagent ui` command, or self-hosted static assets.
- **Companion WordPress plugin** (`companion-plugin/`) — installed on the WooCommerce store. Registers the `wooagent-*` ability surface the agent fleet consumes over MCP. Becomes thinner as WooCommerce's own native ability surface fills in.

## Repo layout

- `daemon/` — Go daemon. Entry point `cmd/wooagent`; internals under `internal/`.
- `ui/` — Standalone React UI (Vite + `@wordpress/components`).
- `companion-plugin/` — WordPress plugin source.
- `scripts/` — Build, release, and maintenance scripts.
- `dist/` — Build artifacts (plugin zips, UI bundles). Gitignored.

## Install

If you just want to run WooAgent OS — not develop on it — grab the latest release with one line:

```bash
curl -fsSL https://raw.githubusercontent.com/elizaan36/wooagent-os/trunk/install.sh | bash
```

The installer detects your platform (macOS / Linux on amd64 or arm64), downloads the matching binary from the latest GitHub Release, verifies its SHA-256, and drops it in `~/.wooagent/bin/wooagent`. After install:

```bash
wooagent init      # creates ~/.wooagent and mints an initial auth token
wooagent run       # serves http://localhost:7777
```

Open <http://localhost:7777> in a browser — the React UI is baked into the binary and served from the same daemon. No second process, no separate Vite dev server.

Pin a specific version with `WOOAGENT_VERSION=v0.1.0`. Use `WOOAGENT_INSTALL_DIR=/usr/local/bin` to install into a system location instead. Windows isn't supported by the script — download the `.zip` from the [Releases page](https://github.com/elizaan36/wooagent-os/releases) directly, or use WSL.

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

WooAgent OS is in active pre-1.0 development. Today, three personas — **Marketing**, **Pricing**, and **Sales Support** — propose changes against a live WooCommerce store and write through on operator approval. Inventory, Reporting, Accounting, and Chief of Staff are in progress and will ship as they're ready.

Expect breaking changes between v0.x releases.

## Reporting security issues

Please do **not** report security vulnerabilities through public GitHub issues. See [`SECURITY.md`](./SECURITY.md) for the private disclosure process.

## License and trademarks

Source code is licensed under the Apache License, Version 2.0. See [`LICENSE`](./LICENSE) for the full text and [`NOTICE`](./NOTICE) for third-party attribution required under §4(d).

The Apache 2.0 license does not grant any rights to Automattic trademarks. See [`TRADEMARKS.md`](./TRADEMARKS.md) for permitted use of the WooAgent, WooCommerce, Woo, WordPress, and Jetpack names.

## Export control

This project may be subject to U.S. and other applicable export-control laws and regulations, including the U.S. Export Administration Regulations (EAR). By downloading, using, or distributing this software, you agree to comply with all such laws and regulations. You may not export, re-export, or transfer this software (directly or indirectly) to any country, person, or entity prohibited from receiving it under U.S. export-control rules — including, without limitation, parties listed on the U.S. Treasury Department's List of Specially Designated Nationals or the U.S. Commerce Department's Entity List.
