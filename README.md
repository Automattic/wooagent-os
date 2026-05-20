# WooAgent OS

**Open-source, local-first agent operating system for WooCommerce store operators.**

A fleet of AI agents — Marketing, Pricing, Sales Support today; Inventory, Reporting, Accounting, Chief of Staff in progress — runs against your live WooCommerce store. Every proposed change lands as an issue the operator reviews and approves. Nothing writes to the store automatically.

## What's different

- **MCP-native.** Connects to any WordPress/WooCommerce site via the MCP Adapter. Every plugin that registers abilities through the WP Abilities API is available to the agent fleet — no bespoke integrations.
- **Local-first.** Daemon, SQLite store, run log, and Companion Plugin stay on your machine. The only data that leaves is the prompt you've configured your LLM provider to receive.
- **Model-agnostic.** Point the daemon at any frontier provider (Anthropic, Google, OpenAI, xAI) or local runtime (Ollama, LM Studio, llama.cpp). The fleet runs identically.
- **Propose-approve trust gate.** A pre-signed manifest of trusted abilities plus a deterministic non-LLM middleware verifies every store-mutating call. Prompt-injected or hallucinated tool calls are denied, not executed. Every write requires explicit operator approval.

## Data handling

What each shipping persona sends to the LLM:

- **Marketing** — current product copy (title, description, attributes). No customer data.
- **Pricing** — product attributes plus the agent's `web_search` queries to retailer sites. No customer data.
- **Sales Support** — order context for one recent order: order number, status, total, line items, and the customer's **first name only**. Surname, email, and shipping/billing address are dropped at the prompt boundary and never reach the LLM provider. For zero-egress, point Sales Support at a local model.

Inventory, Reporting, Accounting, and Chief of Staff are in progress and will get their own data-handling lines when they ship.

**Your responsibilities.** You agree to your LLM provider's terms and acceptable-use policies. You own GDPR/CCPA and any other privacy obligations to your customers, including any disclosures about automated processing of order data. You're responsible for reviewing each proposal before approval and for maintaining store backups. WooAgent OS is provided "AS IS" — see [`LICENSE`](./LICENSE) §7–§8.

## Install

If you just want to run WooAgent OS:

```bash
curl -fsSL https://raw.githubusercontent.com/elizaan36/wooagent-os/trunk/install.sh | bash
```

The installer detects your platform (macOS / Linux on amd64 or arm64), downloads the matching binary from the latest GitHub Release, verifies its SHA-256, and drops it in `~/.wooagent/bin/wooagent`.

```bash
wooagent init      # creates ~/.wooagent and mints an initial auth token
wooagent run       # serves http://localhost:7777
```

The React UI is baked into the binary — open <http://localhost:7777> in a browser.

Pin a version with `WOOAGENT_VERSION=v0.1.0`. Windows: download the `.zip` from the [Releases page](https://github.com/elizaan36/wooagent-os/releases) directly or use WSL.

## Connecting a WooCommerce store

The daemon talks to any WooCommerce store with the WordPress MCP Adapter installed. You also need the **WooAgent Companion** plugin on the store — it registers the `wooagent-*` ability surface the fleet uses.

Grab `wooagent-companion.zip` from the same GitHub Release as the daemon, or build from source:

```bash
bash scripts/build-companion-plugin-zip.sh   # writes dist/wooagent-companion.zip
```

Upload via **wp-admin → Plugins → Add New → Upload Plugin**. Walk the daemon's first-run UI to pair: type your store URL, click **Open wp-admin → Pair device**, click Approve. The pair handshake mints a device token the daemon stores in your OS keychain.

## Dev quickstart

Two terminals; the daemon's CORS allows cross-origin from `:5173`.

```bash
# Terminal 1 — daemon
cd daemon
go run ./cmd/wooagent init           # mints an initial auth token
go run ./cmd/wooagent run            # serves http://localhost:7777

# Terminal 2 — Vite dev server
cd ui && npm install && npm run dev  # serves http://localhost:5173 with hot reload
```

Open <http://localhost:5173> and paste the daemon URL + token from Terminal 1.

To test against the embedded UI (matches the release-binary build):

```bash
cd ui && npm install && npm run build
bash scripts/build-ui-into-daemon.sh
cd daemon && go run ./cmd/wooagent run
```

## Releases

Binaries are built by [GoReleaser](https://goreleaser.com/) via `.github/workflows/release.yml` from tagged commits. Every GitHub Release publishes a `SHA256SUMS` file alongside the platform archives. See [`CONTRIBUTING.md`](./CONTRIBUTING.md#cutting-a-release) for the maintainer procedure.

## Status

WooAgent OS is in active pre-1.0 development. Expect breaking changes between v0.x releases.

## Security, license, and legal

- **Reporting vulnerabilities** — see [`SECURITY.md`](./SECURITY.md). Do not open public issues.
- **License** — Apache 2.0. See [`LICENSE`](./LICENSE) and [`NOTICE`](./NOTICE) (third-party attribution required under §4(d)).
- **Trademarks** — see [`TRADEMARKS.md`](./TRADEMARKS.md). The Apache 2.0 license does not grant rights to Automattic trademarks.
- **Contributing** — see [`CONTRIBUTING.md`](./CONTRIBUTING.md) and [`CODE_OF_CONDUCT.md`](./CODE_OF_CONDUCT.md).
- **Export control** — Subject to U.S. and other export-control laws. Do not export, re-export, or transfer to embargoed countries or restricted parties (U.S. Treasury SDN List, U.S. Commerce Entity List).
