# WooAgent OS

Open-source, local-first agent operating system for WooCommerce store operators. See `wooagent-os-prd V2.md` for the full PRD and `WooAgent_plan.md` for the Phase 1 build plan.

## Repo layout

- `daemon/` — Go daemon. Headless by default. Exposes a REST API that the UI and CLI consume.
- `ui/` — Standalone React UI (Vite + `@wordpress/components`). Connects to any daemon over HTTP.
- `docs/` — API contracts and design notes shared across daemon + UI.
- Top-level `.md` files — PRD, pitch, plans, persona research.

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

## Status

Phase 1 scaffolding. The daemon CLI and HTTP endpoints are mostly stubs — real behaviour (ADK Go agents, MCP client, personas) lands through Phase 1–3 per `WooAgent_plan.md`.
