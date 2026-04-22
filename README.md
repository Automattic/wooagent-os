# WooAgent OS

Open-source, local-first agent operating system for WooCommerce store operators. See `wooagent-os-prd V2.md` for the full PRD and `WooAgent_plan.md` for the Phase 1 build plan.

## Repo layout

- `daemon/` — Go daemon. Headless by default. Exposes a REST API that the UI and CLI consume.
- `ui/` — Standalone React UI (Vite + `@wordpress/components`). Connects to any daemon over HTTP.
- `companion-plugin/` — WordPress plugin that registers the `wooagent-*` ability surface on a WooCommerce store. Installed on the store side. Paired with the daemon.
- `docs/` — API contracts and design notes shared across daemon + UI.
- `dist/` — Build artifacts (plugin zips, etc.). Gitignored.
- Top-level `.md` files — PRD, pitch, plans, persona research, and `companion-plugin-v0.1-plan.md` scope note.

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

The daemon talks to any WooCommerce store running WP 6.9+ and the MCP Adapter. For Phase 1 you also need the **WooAgent Companion** plugin installed on the store — it registers the `wooagent-*` ability surface that the agent fleet consumes (PRD §10.4 and `companion-plugin-v0.1-plan.md` explain why we pulled this forward from v0.3).

```bash
# Build the plugin zip
cd companion-plugin
zip -r ../dist/wooagent-companion-0.1.0.zip . -x "*.DS_Store"
```

Upload the resulting zip via wp-admin → Plugins → Add New → Upload Plugin. Then authenticate the daemon with a WordPress Application Password (Users → Profile → Application Passwords). Native device-pair flow ships in Companion Plugin v0.2.

## Status

Pre-Phase 1 scaffolding + **Companion Plugin v0.1 verified end-to-end against a live Pressable staging store** (Apr 21). Daemon CLI and HTTP endpoints are still mostly stubs — real agent behaviour (ADK Go agents, MCP client, first persona) lands through Phase 1 per `WooAgent_plan.md` and `progress.md`.
