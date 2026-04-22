# WooAgent OS -- Product Requirements Document

**Status:** Draft v0.2
**Owner:** Elizabeth
**License:** Apache 2.0 (LICENSE committed; repo currently private until open-sourcing)
**Repo:** `github.com/elizaan36/wooagent-os` (private during prototype; will be relocated to a `wooagent-os` org at open-sourcing time — GitHub redirects preserve existing clones)
**Source layout:** monorepo with `daemon/` (Go) and `ui/` (React/Vite) as siblings. PRD §6 still describes the daemon and UI as independently-releasable surfaces; the monorepo is a convenience during the prototype phase, not a retreat from that separation.

---

## 1. Summary

WooAgent OS is an open-source, local-first agent operating system for WooCommerce store operators. It runs a fleet of purpose-built agent personas (marketing, pricing, inventory, accounting, reporting, sales support) that plan, execute, and report on store-operations work against a live WooCommerce store.

The system is **MCP-native**. Rather than maintaining a proprietary WooCommerce API integration layer, WooAgent OS connects to a store's existing MCP endpoint provided by the WordPress MCP Adapter. This means every WordPress plugin that registers abilities via the Abilities API (WooCommerce, Yoast SEO, ACF, Gravity Forms, WPForms, Ninja Forms, and 70+ others) is automatically available to the agent fleet without WooAgent OS writing a single adapter. Agents operate on the full surface of the store, not just the commerce data.

The system is model-agnostic. Operators point it at any frontier provider (Anthropic, Google, OpenAI, xAI) or any local runtime (Ollama, LM Studio, llama.cpp) and the agent fleet runs identically. All state lives on the operator's machine by default. No data leaves the host unless the operator explicitly wires in a hosted model or a remote store.

The interface is a kanban-style command center where every unit of agent work is a trackable issue. Operators see exactly what the agents are queueing, running, blocking on, or completing, and can intervene at any step.

## 2. Problem Statement

Running a WooCommerce store at any meaningful volume is a coordination problem across a dozen loosely-connected disciplines. A solo operator or small team is expected to do keyword research, write product copy, audit pricing against competitors, monitor stock, reconcile payouts, generate reports for tax season, and answer customer questions. Most of this work is repetitive, bounded, and well-suited to LLM automation.

WordPress 7.0 shipped the building blocks for AI integration: the Abilities API, WP AI Client SDK, and MCP Adapter. The ecosystem adopted fast. WooCommerce registers abilities under `woocommerce/*`. Yoast exposes SEO controls. ACF exposes field groups and content models. Over 70 plugins register abilities. Every WordPress site with the MCP Adapter installed is now an MCP server that AI agents can connect to.

But the ecosystem has no opinionated agent layer. The building blocks are there. What's missing is:

1. **A persistent agent fleet** that runs continuously against the store, not one-shot prompts.
2. **Multi-agent coordination** where specialized personas collaborate on shared state.
3. **A work-tracking system** where every agent action is an auditable, approvable issue.
4. **A propose/approve workflow** so agents never apply changes without operator review.
5. **An orchestration layer** that runs locally and keeps store data on the operator's infrastructure.

WordPress built the protocol layer. WooAgent OS is the operations layer on top of it.

## 3. Goals

1. **Local-first operation.** Install and run the entire stack on a single Linux, macOS, or Windows machine. No cloud account required for core functionality.
2. **MCP-native store integration.** Connect to any WordPress/WooCommerce site via its MCP endpoint. Automatically discover and use every registered ability from every installed plugin.
3. **Model-agnostic harness.** First-class support for Anthropic Claude, Google Gemini, OpenAI GPT, and any OpenAI-compatible local endpoint (Ollama, LM Studio).
4. **Purpose-built agent personas** for the six most common store-ops disciplines, shipped as defaults and extensible by the operator.
5. **Kanban-native UX.** Every agent action is a first-class issue with status, priority, owner (agent persona), and full execution log.
6. **Single-binary install.** `curl | sh` or `brew install`, then `wooagent init`, then the operator is running.
7. **Auditable by default.** Every tool call, model call, ability invocation, and state change is logged locally and replayable.
8. **Ecosystem-aware agents.** Agent personas load WordPress Agent Skills for domain knowledge and adapt to the specific abilities registered on each connected store.

## 4. Non-Goals (v1)

1. Multi-tenant SaaS hosting. This is an operator-run project; a hosted version may come later but is not v1.
2. Shopify, BigCommerce, Magento, or generic e-commerce support. WooCommerce on WordPress only in v1.
3. A marketing content CMS of its own. Agents write to Woo; Woo is the system of record.
4. An agent marketplace or agent-to-agent commerce. Agents collaborate internally; external exchange is out of scope.
5. Browser-automation agents. Agents work via MCP, REST API, and file system. Headless-browser work is a later extension.
6. Replacing the WordPress AI provider plugin ecosystem. WooAgent OS runs its own model adapters locally for agent inference. It does not duplicate or compete with `wp_ai_client_prompt()` or the WP provider plugins.

## 5. Target Users

1. **Solo WooCommerce operators** running $100K-$5M GMV stores who are drowning in ops work and want leverage.
2. **Small agencies** managing 5-50 Woo stores for clients, who need consistent ops automation across tenants.
3. **Developers and power users** building custom Woo extensions who want an AI-native control plane for their store's content, catalog, and operations.
4. **Open-source contributors** building new agent personas, skills, and model adapters on top of the harness.

## 6. High-Level Architecture

```
┌──────────────────────┐      ┌──────────────────────┐
│  Standalone React UI │      │        CLI           │
│  (Vite + @wordpress  │      │     wooagent         │
│   design system)     │      │                      │
│  Hosted or local     │      │                      │
└──────────┬───────────┘      └──────────┬───────────┘
           │ REST / gRPC                 │ REST / gRPC
           │ (token auth)                │ (local socket)
           ▼                             ▼
┌─────────────────────────────────────────────────────────────────────┐
│                   WooAgent OS Daemon (Go, headless-capable)         │
│                                                                     │
│                   ┌─────────────────────────────────┐               │
│                   │   REST / gRPC API               │               │
│                   │   (bind: localhost or any iface)│               │
│                   └────────────────┬────────────────┘               │
│                                    │                                │
│                   ┌─────────▼──────────┐                            │
│                   │  Orchestrator Core │                            │
│                   │  (issue queue,     │                            │
│                   │   agent runtime,   │                            │
│                   │   run logs, auth)  │                            │
│                   └─────────┬──────────┘                            │
│                             │                                       │
│      ┌──────────────────────┼──────────────────────┐                │
│      │                      │                      │                │
│  ┌───▼────────┐   ┌────────▼─────────┐   ┌────────▼─────────┐      │
│  │  Agent     │   │ Model Provider   │   │  MCP Client      │      │
│  │  Personas  │   │ Abstraction      │   │  + Ability Cache  │      │
│  │  (ADK)     │   │                  │   │                   │      │
│  └───┬────────┘   └────────┬─────────┘   └────────┬─────────┘      │
│      │                     │                      │                │
│      │  ┌──────────────────┼──────────────┐       │                │
│      │  │                  │              │       │                │
│      │  │  ┌───────┐ ┌────▼───┐ ┌────────▼──┐    │                │
│      │  │  │Claude │ │Gemini  │ │GPT / xAI  │    │                │
│      │  │  └───────┘ └────────┘ └───────────┘    │                │
│      │  │                                        │                │
│      │  │  ┌─────────────────────┐               │                │
│      │  │  │ Ollama / LM Studio  │               │                │
│      │  │  │ (local)             │               │                │
│      │  │  └─────────────────────┘               │                │
│      │  └────────────────────────────────────────│────────┐       │
│      │                                           │        │       │
│      │  ┌────────────────────────────────────────┘        │       │
│      │  │                                                 │       │
│      │  │  ┌──────────────────────────────────────────┐   │       │
│      │  │  │  Local Skills (non-MCP)                  │   │       │
│      │  │  │  pricing.margin.compute                  │   │       │
│      │  │  │  accounting.pnl.compute                  │   │       │
│      │  │  │  report.generate, email.draft            │   │       │
│      │  │  │  repo.file.read, repo.git.diff           │   │       │
│      │  │  │  issue.create, schedule.cron             │   │       │
│      │  │  └──────────────────────────────────────────┘   │       │
│      │  │                                                 │       │
│      │  │  ┌──────────────────────────────────────────┐   │       │
│      │  │  │  WordPress Agent Skills (knowledge)      │   │       │
│      │  │  │  wp-rest-api, wp-abilities-api            │   │       │
│      │  │  │  wp-plugin-development, wp-wpcli-and-ops  │   │       │
│      │  │  └──────────────────────────────────────────┘   │       │
│      │  │                                                 │       │
│      └──┴─────────────────────────────────────────────────┘       │
│                                                                    │
│             ┌──────────────────┐                                   │
│             │  Local SQLite    │                                   │
│             │  + JSON run logs │                                   │
│             └────────┬─────────┘                                   │
└──────────────────────┼─────────────────────────────────────────────┘
                       │
       ┌───────────────┼────────────────────────┐
       │               │                        │
┌──────▼──────┐  ┌─────▼──────────────┐  ┌──────▼──────────┐
│ WP/Woo Site │  │ WP/Woo Site        │  │ Local Woo Repo  │
│ MCP Endpoint│  │ REST API (fallback)│  │ (optional)      │
│             │  │ + Webhooks         │  │ plugins/themes  │
│ Abilities:  │  └────────────────────┘  └─────────────────┘
│ woocommerce/│
│ yoast-seo/  │
│ acf/        │
│ ninja-forms/│
│ gravitykit/ │
│ (auto-disc) │
└─────────────┘
```

### 6.1 Components

1. **Orchestrator Core.** Single daemon process. Owns the issue queue, agent lifecycle, run logs, local auth, and the API that the UI and CLI talk to.
2. **Agent Personas.** Built on top of the Agent Development Kit (ADK). Each persona is a packaged agent with its own system prompt, skill bindings, WordPress Agent Skills context, default model preferences, and guardrails.
3. **MCP Client + Ability Cache.** Connects to the store's MCP endpoint. On connection, discovers all registered abilities across every installed plugin. Caches the ability schema locally. Agents invoke abilities through this client. This is the primary interface to the store.
4. **Local Skills Library.** Computational and orchestration skills that don't map to WordPress abilities: margin calculations, report generation, competitor data fetching, email drafting, issue management, scheduling. These run entirely within WooAgent OS.
5. **WordPress Agent Skills.** Portable instruction bundles from the `WordPress/agent-skills` repo. Loaded as context into agent personas to give them deep WordPress and WooCommerce domain knowledge.
6. **Model Provider Abstraction.** Thin adapter layer that normalizes frontier and local model APIs to a single internal interface. Handles streaming, tool/function calling, retries, and cost accounting.
7. **Web UI.** Standalone React application, shipped and deployed independently from the daemon. Not served by the orchestrator. Connects to any running WooAgent OS daemon over its REST/gRPC API (operator enters the daemon URL and an auth token on first launch). Three deployment modes: (a) use the hosted build at `https://ui.wooagent.dev`; (b) run `wooagent ui` to serve the prebuilt bundle locally; (c) self-host the static assets on any web server. Kanban board, issue detail view, agent roster, ability browser, run log viewer, settings. The daemon can run fully headless without any UI.
8. **CLI.** `wooagent` command for install, configuration, store connection, agent management, manual runs, and export.
9. **Storage.** SQLite for structured state (issues, agents, runs, ability cache). Newline-delimited JSON for run logs. Optional Postgres for operators running at scale.

### 6.2 Why MCP-Native

The v0.1 draft of this PRD specified a custom WooCommerce REST API client. That approach is superseded.

WordPress 7.0 shipped the Abilities API and MCP Adapter. WooCommerce registers product, order, and customer abilities under `woocommerce/*`. Over 70 plugins now register abilities. The MCP Adapter exposes all of them through a single MCP endpoint. Major WordPress hosts (Pressable, WP Engine, WordPress.com, InstaWP) pre-install or support MCP out of the box.

By connecting as an MCP client, WooAgent OS gets:

- **Every WooCommerce ability** (products, orders, customers, coupons, stock) without writing Woo-specific API code.
- **Every plugin's abilities** automatically. If the store has Yoast SEO, the Marketing Agent can use `yoast-seo/*` abilities. If it has ACF, agents can inspect and create custom field groups. If it has Gravity Forms via GravityMCP, agents can build forms. None of this requires WooAgent OS to know about those plugins in advance.
- **Forward compatibility.** As plugins ship new abilities, agents gain access immediately.
- **A standardized permission model.** The MCP Adapter respects WordPress's existing user roles and capability system. WooAgent OS authenticates once; the store controls what it can do.

The REST API remains as a fallback for stores not yet on WordPress 7.0 or without the MCP Adapter, and for bulk data operations where MCP's request-response model is too chatty.

## 7. Agent Personas (v1 default fleet)

Each persona is opinionated and shipped configured. Operators can disable, clone, fork, or create new personas. Every persona is built on ADK and loads relevant WordPress Agent Skills as context.

**UI visibility vs. functional readiness.** All six operator personas plus the Chief of Staff meta-agent ship visible in the UI from v0.1 — they appear in the agent roster, as selectable owners on issue cards, and in the onboarding "default fleet deployed" summary. Functional readiness is phased: at the May 22 demo, Marketing, Pricing, and Sales Support run the full propose → review → approve → write loop against the live store, while Inventory, Accounting, and Reporting are present in the roster with a clearly-labeled inert state ("coming soon" badge; demo-only seeded sample issues may appear on the board to illustrate the shape of their work but do not invoke MCP writes). Chief of Staff is scaffolded — it exists in the roster and generates the first-run store-profile issue, but autonomous coordination remains deferred to v0.2. The intent is for the operator to see the full breadth of the fleet from day one, and for the phased functional rollout to happen underneath a stable UI surface. Personas flagged as non-functional for v1:
- **Inventory Manager** (§7.3) — UI-visible, inert for v1.
- **Accounting** (§7.4) — UI-visible, inert for v1.
- **Reporting** (§7.5) — UI-visible, inert for v1.
- **Chief of Staff** (§7.7) — scaffolded only; full arbitration behavior per the v0.2 roadmap.

### 7.1 Marketing & SEO Agent

**Mandate:** Grow organic traffic and conversion.

**WordPress Agent Skills loaded:** `wp-rest-api`, `wp-abilities-api`

**MCP abilities used:** `woocommerce/products/*`, `woocommerce/categories/*`, `yoast-seo/*` (if available), `acf/*` (if available), WordPress core post/page abilities

**Typical issues it creates or picks up:**
- Rewrite product descriptions for clarity, keyword density, and brand voice.
- Generate meta titles and descriptions for products and categories (via Yoast abilities if present, or direct post meta updates).
- Audit and fix missing alt text on product images.
- Propose and draft blog posts tied to seasonal demand.
- Generate JSON-LD schema markup. If Yoast is present with Schema.org abilities, use those. Otherwise generate standalone markup.
- Identify cannibalizing keywords across category pages.
- Draft email newsletter content from recent products and bestsellers.
- If the store has an `llms.txt` endpoint (Yoast, Rank Math, SEOPress), audit it for completeness.

### 7.2 Pricing Agent

**Mandate:** Protect margin and stay competitive.

**MCP abilities used:** `woocommerce/products/*`, `woocommerce/orders/*`, `woocommerce/coupons/*`

**Local skills used:** `pricing.competitor.fetch`, `pricing.margin.compute`, `pricing.elasticity.estimate`

**Typical issues:**
- Pull competitor pricing (via configured data source) and flag items more than N% above market.
- Compute per-SKU margin after fees, shipping, and COGS.
- Propose sale prices ahead of holidays with projected margin impact.
- Identify zombie SKUs (no sales in 90+ days) for clearance pricing.
- Validate price consistency across currencies and tax jurisdictions.
- Audit coupon usage patterns and flag abuse or expiring promotions.

### 7.3 Inventory Manager Agent

**Mandate:** Keep stock healthy.

**MCP abilities used:** `woocommerce/products/*` (stock fields), `woocommerce/orders/*` (velocity data)

**Typical issues:**
- Flag SKUs below reorder point.
- Compute reorder quantities from 30/60/90-day velocity.
- Draft POs to suppliers (email draft, not auto-send).
- Reconcile received inventory against POs.
- Identify slow-mover and fast-mover shifts week over week.
- Warn on oversell risk for variations with shared stock.

### 7.4 Accounting Agent

**Mandate:** Keep the books clean and tax-ready.

**MCP abilities used:** `woocommerce/orders/*`, `woocommerce/refunds/*`

**Local skills used:** `accounting.payout.reconcile`, `accounting.tax.summarize`, `accounting.pnl.compute`, `accounting.export.*`

**Typical issues:**
- Reconcile payouts (Stripe, PayPal, etc.) against Woo orders.
- Categorize refunds, chargebacks, and adjustments.
- Track sales tax collected by jurisdiction.
- Compute monthly P&L from Woo data plus configured expense feeds.
- Export to formats used by common accounting systems (CSV, IIF, QBO-friendly layouts).
- Flag orders missing invoices or with mismatched totals.

### 7.5 Reporting Agent

**Mandate:** Make the numbers legible.

**MCP abilities used:** `woocommerce/orders/*`, `woocommerce/customers/*`, `woocommerce/products/*`

**Local skills used:** `report.generate`, `report.dashboard.update`, `email.digest`

**Typical issues:**
- Produce weekly, monthly, quarterly sales reports.
- Build cohort and retention reports from order history.
- Maintain a live KPI dashboard (GMV, AOV, repeat rate, gross margin).
- Email scheduled digests.
- Generate narrative executive summaries from raw numbers.

### 7.6 Sales Support Agent

**Mandate:** Reduce operator time on customer communication.

**MCP abilities used:** `woocommerce/orders/*`, `woocommerce/customers/*`, `woocommerce/products/*`, form plugin abilities (Ninja Forms, Gravity Forms, WPForms) if present

**Typical issues:**
- Draft responses to pre-sale product questions based on product data and FAQs.
- Handle order-status inquiries by looking up the order and drafting a reply.
- Process refund and return requests up to a configurable auto-approve threshold.
- Escalate anything outside policy to the human operator.
- Maintain a knowledge base of canned responses, updated from past decisions.
- If the store has a form plugin with abilities, inspect form submissions for support requests.

### 7.7 Meta-Agent: Chief of Staff

**Mandate:** Coordinate the fleet.

**MCP abilities used:** All (read-only discovery and arbitration).

**Typical issues:**
- Break down operator goals ("prep for Black Friday") into sub-issues routed to the right personas.
- Resolve conflicts when two personas want to act on the same SKU.
- Enforce guardrails (e.g., Pricing Agent may not drop below cost without approval).
- On first connect to a new store, inventory all discovered MCP abilities and recommend which personas to enable.
- Summarize daily activity for the operator.

## 8. Skills Architecture

WooAgent OS agents have access to two distinct categories of capabilities: **MCP abilities** (remote, on the WordPress site) and **local skills** (computational, running inside WooAgent OS).

### 8.1 MCP Abilities (Remote, Auto-Discovered)

These are provided by the store. WooAgent OS discovers them dynamically on connection and caches the schemas. Agents invoke them through the MCP client.

**WooCommerce core abilities** (registered under `woocommerce/*`):
Products (list, get, create, update), orders (list, get, refund), customers (list, get), categories, tags, coupons, stock, variations. WooCommerce also publishes a demo plugin showing third-party developers how to register custom abilities.

**Plugin-provided abilities** (auto-discovered, zero config):
Yoast SEO (`yoast-seo/*`), ACF (`acf/*`), Ninja Forms (32 abilities across 7 capability areas), WS Form (full MCP server), Gravity Forms via GravityMCP, WPForms, Jetpack, MainWP, WPCode, hCaptcha, miniOrange SSO/OAuth/LDAP, and any other plugin using `wp_register_ability()`.

**WordPress core abilities** (registered by WordPress 7.0 itself):
Posts, pages, media, users, settings, taxonomies. Includes proposals in active development: `core/get-settings`, `core/update-settings`, `core/get-user`, `core/get-post`.

The key design principle: **WooAgent OS does not maintain a list of supported plugins.** It connects to MCP, discovers what's there, and makes it available. If an operator installs a new plugin tomorrow that registers abilities, agents can use them immediately.

### 8.2 Local Skills (Computational, Built-In)

These run inside WooAgent OS. They handle logic that doesn't exist as a WordPress ability because it's computational, analytical, or orchestration work.

**Pricing skills:**
`pricing.competitor.fetch` (configurable data source), `pricing.margin.compute` (COGS + fees + shipping), `pricing.elasticity.estimate`, `pricing.rule.evaluate`

**Accounting skills:**
`accounting.payout.reconcile` (Stripe/PayPal CSV ingestion), `accounting.tax.summarize`, `accounting.pnl.compute`, `accounting.export.csv`, `accounting.export.iif`, `accounting.export.qbo`

**Reporting skills:**
`report.generate` (templated markdown/PDF reports), `report.dashboard.update`, `report.cohort.compute`, `report.kpi.snapshot`

**Communication skills:**
`email.draft`, `email.send` (send requires per-issue operator approval by default), `newsletter.draft`

**Repo skills (when local repo is configured):**
`repo.file.read`, `repo.file.write`, `repo.search`, `repo.plugin.scaffold`, `repo.migration.draft`, `repo.git.diff`, `repo.git.commit` (commit only, never push)

**System skills:**
`issue.create`, `issue.update`, `issue.block`, `issue.link`, `schedule.cron`, `schedule.once`, `ability.discover` (re-scan store MCP endpoint)

Every skill and ability invocation is logged with input, output, duration, cost (for model-backed skills), and caller identity (which agent, on which issue, in which run).

### 8.3 WordPress Agent Skills (Knowledge Context)

The `WordPress/agent-skills` repo contains 14 portable instruction bundles that teach AI agents how to work with WordPress correctly. WooAgent OS personas load relevant skills as system-prompt context:

- **All personas:** `wp-abilities-api` (how to discover and invoke abilities correctly), `wp-rest-api` (fallback API patterns)
- **Marketing & SEO:** `wp-block-development` (for content structure), `wp-block-themes` (for template awareness)
- **Repo-mode agents:** `wp-plugin-development`, `wp-wpcli-and-ops`, `wp-phpstan`
- **Chief of Staff:** `wp-project-triage`

Agent Skills are installed via `npx skills add` or bundled with the WooAgent OS distribution. They are Markdown files, not code. They update independently of WooAgent OS releases.

## 9. Kanban UX

The primary interface is a kanban board modeled on modern issue trackers. Every unit of agent work is an issue. The UI is a standalone React app that connects to any WooAgent OS daemon over its REST/gRPC API. By default the daemon listens on `localhost:7777`; the UI's daemon URL is configurable so a single UI instance can point at a local daemon, a teammate's machine, or a remote agency-managed daemon. Operators can use the hosted UI at `https://ui.wooagent.dev`, run `wooagent ui` to serve a local copy, or self-host the static bundle.

### 9.1 Columns

- **Backlog.** Ideas and low-urgency work queued but not yet ready. Chief of Staff triages from here.
- **Todo.** Ready to be picked up by an agent. Either the assigned agent will start on its next tick, or the operator can start it manually.
- **In Progress.** An agent is actively running. Shows a live spinner, current step, current ability/skill call.
- **In Review.** The agent has produced a result that needs operator approval before it's applied (e.g., price change, email send, published blog post). Shows a diff of the proposed ability call with before/after state.
- **Done.** Completed. Shows the full diff of what changed, with rollback available.

### 9.2 Issue Detail View

- Title and description (agent- or operator-written).
- Assigned persona (avatar, name, model it's using).
- Priority (Urgent, High, Medium, Low, None).
- Status and column.
- Linked SKUs, orders, customers, files.
- **Abilities used** (which MCP abilities and local skills were invoked).
- Full run log: every model call, every ability invocation, every thought, timestamped.
- Operator comments and overrides.
- Related issues (blocks, blocked by, duplicate of).

### 9.3 Left Nav

- **Inbox.** New webhooks, new customer inquiries, newly-flagged items.
- **My Issues.** Items assigned to the operator for review.
- **Issues.** Board view, the main screen.
- **Agents.** Fleet roster: status, model, last-run, abilities/skills enabled.
- **Abilities.** Browser for all discovered MCP abilities and local skills. Shows which store plugin provides each ability, the schema, and which personas have access. Toggle on/off per agent.
- **Runtimes.** Which model endpoints are configured and healthy.
- **Settings.** Store connections, model providers, guardrails, secrets.

### 9.4 Key Interactions

- Drag-drop issues between columns. Some transitions trigger actions (moving from In Review to Done applies the staged change via MCP).
- Create an issue manually and assign to any persona.
- Issue `@mention` routing (`@pricing audit holiday sale margins`) from anywhere in the UI or via CLI.
- One-click rerun of any past run with same or different model.
- **Ability explorer.** Click any discovered ability to see its schema, try a test invocation, and see which agents use it.

## 10. Store Integration Model

### 10.1 MCP Endpoint (Primary Mode)

The operator connects a live WordPress/WooCommerce store by its MCP endpoint URL. Authentication uses one of two mechanisms, in priority order:

1. **Device pairing via the Companion Plugin (default, recommended).** On first connection, the daemon displays a short pairing code. The operator opens **wp-admin → WooAgent → Pair device**, enters the code, and approves. The Companion Plugin mints a scoped token bound to a WooAgent device identity (not a WP user), which the daemon stores in the OS keychain. Tokens are independently revocable per-device from wp-admin. See §10.4. This is the primary path and the default in `wooagent init`.

2. **Application Password (fallback, always works).** For stores that have not yet installed the Companion Plugin — or for operators who prefer not to — the operator generates a WordPress Application Password (native since WP 5.6), and pastes it into `wooagent init`. The daemon then authenticates as that WP user; the user's role and capabilities determine which abilities are invokable.

Hosting-specific OAuth flows (WordPress.com, WP Engine, Pressable) and native MCP Adapter OAuth 2.1 are tracked and adopted as they become broadly available, but neither is required for v0.1 connectivity.

On first connection, WooAgent OS:
1. Discovers all registered abilities via MCP.
2. Caches the ability schemas locally.
3. Maps abilities to agent personas based on namespace (`woocommerce/*` to Pricing, Inventory, Accounting, Reporting, Sales Support; `yoast-seo/*` to Marketing; etc.).
4. Chief of Staff generates a "store profile" issue summarizing what's available and recommending persona configuration.

All write operations go through MCP with a configurable staging mode: staged changes land in `In Review` for operator approval before the ability is invoked with write parameters.

**Hosting-specific notes:**
- WordPress.com sites have MCP built in on all paid plans.
- Pressable pre-installs the MCP Adapter.
- WP Engine includes MCP in their AI Toolkit.
- InstaWP includes a built-in MCP server on every site.
- Self-hosted sites install the MCP Adapter plugin (free, official WordPress project, 800+ GitHub stars).

### 10.2 REST API Fallback

For stores not yet on WordPress 7.0 or without the MCP Adapter, WooAgent OS can connect via WooCommerce REST API key pair (consumer key + secret). This mode has reduced capability: agents can only access WooCommerce data (products, orders, customers), not the broader WordPress ecosystem. The UI displays a clear notice that MCP mode is recommended.

### 10.3 Webhooks

The orchestrator optionally subscribes to WooCommerce webhooks for real-time event ingestion (new orders, stock changes, customer registration). Requires an HTTPS ingress (via ngrok, Cloudflare Tunnel, or operator-provided tunnel). Webhook events are converted to issues and routed to the appropriate persona.

### 10.4 WooAgent OS Companion Plugin (Optional)

A lightweight WordPress plugin that registers WooAgent-specific abilities on the store side. Shipped in two tiers:

**Minimal Companion Plugin (v0.1, required for device pairing):**
- `wooagent/device-pair` -- implements the device-pairing flow: exposes a **WooAgent → Pair device** screen in wp-admin, validates operator-entered pairing codes against pending requests from a WooAgent daemon, mints scoped tokens bound to a WooAgent device identity, and provides per-device revocation. This replaces hand-managed Application Passwords as the default auth path and gives operators a Claude-Code-style pairing experience instead of basic-auth credential copy-paste.

**Full Companion Plugin (v0.3):**
- `wooagent/staged-changes` -- allows the store admin to see and approve changes staged by agents directly in wp-admin.
- `wooagent/issue-status` -- syncs issue status between WooAgent OS and a wp-admin dashboard widget.
- `wooagent/guardrail-policy` -- lets the store admin define per-ability guardrails that WooAgent OS respects.

The Minimal plugin is strongly recommended (it is the default path in `wooagent init` and the one the UX is designed around), but not required: operators can always fall back to WordPress Application Password auth (§10.1) and skip the plugin entirely. The Full plugin is additive — once the Minimal plugin is installed, enabling the Full features is an in-plugin toggle, not a separate install.

### 10.5 Local Repo (Optional Mode)

If the operator has the Woo codebase available locally (for custom plugins, themes, migrations), they can register the repo path. Agents with `repo.*` skills can read and propose changes. Writes are always via git branches with the agent as author; commits happen only after operator approval, and pushes are never automatic. Repo-mode agents load `wp-plugin-development`, `wp-phpstan`, and `wp-wpcli-and-ops` Agent Skills as context.

### 10.6 Read-Only Database Replica (Advanced)

For stores where agents need analytical queries beyond what MCP or the REST API supports cheaply, the operator can configure a read-only MySQL connection to a Woo replica. Skills can run typed, sandboxed SQL with LIMIT enforcement and query cost budgets.

## 11. Technical Stack

1. **Agent runtime:** Agent Development Kit for Go (module path `google.golang.org/adk`; source at [github.com/google/adk-go](https://github.com/google/adk-go)) as the agent and skill framework. The orchestrator, CLI, MCP client, and agent runtime all run in a single Go process.
2. **Orchestrator / CLI:** Go. Single static binary per platform. Runs as a headless daemon by default — no UI is served unless the operator explicitly opts in. An optional `wooagent ui` subcommand serves a bundled copy of the standalone React UI for operators who want a one-command local experience. No sidecar processes or external language runtimes required at install time.
3. **MCP client:** Go implementation of the MCP client protocol. Handles connection lifecycle, ability discovery, caching, and invocation. The MCP client is the primary interface to the store.
4. **Web UI:** Standalone React SPA (built with Vite, static assets). Will ship independently from the daemon on its own release cadence. (Source currently lives in `ui/` inside the `github.com/elizaan36/wooagent-os` monorepo during the prototype phase; splits into its own repo at open-sourcing / first independent release, whichever comes first.) Uses the WordPress design system — `@wordpress/components`, `@wordpress/ui`, `@wordpress/icons`, and `@wordpress/i18n` — so the UI feels native to WordPress operators and inherits accessibility, theming, and internationalization. The **canonical reference** for which components are stable, their props, and the design-token vocabulary (colors, spacing, typography, elevation) is the WordPress Design System MCP server (`@wordpress/design-system-mcp`, published from [Automattic/wpds-mcp](https://github.com/Automattic/wpds-mcp)). Implementers query it via the MCP resources `wpds://components`, `wpds://components/{name}`, `wpds://design-tokens`, and `wpds://pages/{slug}` rather than guessing from TypeScript types or scraping docs. The UI communicates with any WooAgent OS daemon over its REST/gRPC API; operators configure the daemon URL and paste an auth token on first launch. The daemon exposes CORS headers so the UI can be served from any origin (hosted build, localhost, intranet). The daemon runs fully headless by default; serving the UI locally is a separate opt-in command (`wooagent ui`).
5. **Storage:** SQLite (default), Postgres (optional for scale).
6. **Job queue:** Embedded SQLite-backed queue in default mode. River (Postgres) when Postgres is configured.
7. **Model adapters:** Single abstraction layer that wraps Anthropic, Google (Gemini), OpenAI, xAI, and any OpenAI-compatible endpoint (which covers Ollama, LM Studio, llama.cpp server, vLLM, and most self-hosted inference servers).
8. **Observability:** Built-in run-log viewer. OpenTelemetry export optional.
9. **Auth (local):** Device-pair model for the local web UI. Single operator by default; add teammates explicitly.

### 11.10 Resolved architecture decisions

- **Single-language Go runtime (resolved, supersedes earlier drafts).** Earlier drafts proposed a Go orchestrator managing a Python ADK sidecar. With ADK Go (module `google.golang.org/adk`) now at v1.1.0 (Apr 2026), the agent runtime is implemented natively in Go. This preserves the single-binary install promise, eliminates cross-language IPC, and removes Python as an install-time dependency on the operator's machine. **Caveat discovered during the Apr 22 spike:** ADK Go's `model/` package ships only Gemini and Apigee natively. Other providers (Anthropic, OpenAI/OpenAI-compatible) plug in via third-party implementations of the `model.LLM` interface — currently `github.com/Alcova-AI/adk-anthropic-go` and `github.com/huytd/adk-openai-go`. Acceptable for v1; before Phase 5, either vendor these into `daemon/internal/models/` or follow the upstream `google/adk-go-community` effort (PR #242). The `model.LLM` interface is typed on `google.golang.org/genai` content, so any custom adapter is fundamentally a genai ↔ provider translator — same cost whether we write our own or rely on the third-party shims.

## 12. Model Provider Support

### 12.1 Frontier providers (v1)

- **Anthropic:** Claude Opus, Sonnet, Haiku. Native tool use.
- **Google:** Gemini Pro and Flash families. Native tool use.
- **OpenAI:** GPT-5 and successor families. Native tool use.
- **xAI:** Grok family. Native tool use.

### 12.2 Local runtimes (v1)

- **Ollama:** Discovered automatically on `localhost:11434`. All pulled models listed in the UI. Tool use via the model's native or emulated function-calling.
- **LM Studio:** Discovered on `localhost:1234` (configurable). OpenAI-compatible endpoint.
- **llama.cpp server / vLLM / any OpenAI-compatible endpoint:** Configured manually with base URL, optional API key, model name.

### 12.3 Model routing

- Each persona declares a preferred model family and a fallback chain.
- Operator can override globally (e.g., "use only local models") or per persona.
- Cost accounting per run, per agent, per model, visible in the UI.
- Offline mode: when enabled, any attempt to reach a non-local endpoint fails loudly rather than silently falling back.

## 13. Installation and First-Run

### 13.1 Install

```bash
# macOS
brew install wooagent-os/tap/wooagent

# Linux
curl -fsSL https://wooagent.dev/install.sh | sh

# Windows
winget install wooagent.os
```

### 13.2 First Run

```bash
$ wooagent init
✓ Created ~/.wooagent (state dir)
✓ Initialized SQLite store
✓ ADK Go runtime ready

? Connect a WooCommerce store now? (Y/n) Y
? Connection mode: [MCP (recommended) / REST API (legacy)]

# MCP mode
? MCP endpoint URL: https://mystore.com/wp-json/mcp/v1
? Authentication: [Device pairing (recommended) / Application Password]

# Device pairing path (requires Companion Plugin installed on the store)
✓ Pairing code: WOOA-7K3P-9X2M  (valid for 10 minutes)
→ Open https://mystore.com/wp-admin/admin.php?page=wooagent-pair
→ Enter the code above and approve the pairing
✓ Paired as device "mystore-laptop" (scoped token stored in OS keychain)
✓ Connected to mystore.com via MCP
✓ Discovered 147 abilities across 8 plugins:
    woocommerce/*     42 abilities (products, orders, customers, coupons, stock)
    yoast-seo/*       18 abilities (meta, schema, readability, redirects)
    acf/*             12 abilities (field groups, CPTs, taxonomies)
    ninja-forms/*     32 abilities (forms, submissions, actions)
    wordpress/core/*  28 abilities (posts, pages, media, users, settings)
    jetpack/*          9 abilities (forms, stats)
    hcaptcha/*         3 abilities
    wpcode/*           3 abilities

? Configure a model provider now? (Y/n) Y
? Provider: [Anthropic / Google / OpenAI / Ollama / LM Studio / Custom]
...
✓ Default fleet deployed: marketing, pricing, inventory, accounting, reporting, sales-support
✓ Chief of Staff online -- generating store profile...

$ wooagent run
→ Daemon running on http://localhost:7777 (headless)
→ Auth token: wo_pat_9f3a... (paste this into any WooAgent UI)
→ Point a UI at this daemon:
    • hosted:  https://ui.wooagent.dev
    • local:   run `wooagent ui` in another terminal
→ Chief of Staff created 4 initial issues in Backlog
```

### 13.3 Common CLI Commands

```bash
wooagent init                           # scaffold local state
wooagent store add <url>                # add another store (MCP or REST)
wooagent store list
wooagent store pair <url>               # pair with a store via device-pairing flow
wooagent store unpair <url>             # revoke the local token (and prompt wp-admin revoke)
wooagent store abilities [store]        # list all discovered abilities
wooagent agent list
wooagent agent deploy <persona>         # add a new persona from template
wooagent agent disable <n>
wooagent model provider add <n>
wooagent model provider test <n>
wooagent issue create --agent pricing --title "..."
wooagent issue list --status in-progress
wooagent run                            # start the daemon (headless)
wooagent run --bind 0.0.0.0:7777        # bind to any interface for remote UI access
wooagent ui                             # serve the bundled standalone UI locally
wooagent ui --daemon <url>              # point the local UI at a remote daemon
wooagent auth token create              # mint an auth token for a UI or teammate
wooagent logs --follow
wooagent abilities refresh              # re-discover abilities from store
wooagent export --format json --since 2026-01-01
```

## 14. Security, Privacy, and Data

1. **Local by default.** No outbound network traffic except to the model provider(s) and the connected store's MCP endpoint.
2. **Secrets.** Store credentials, model API keys, and other secrets live in the OS keychain (Keychain on macOS, Credential Manager on Windows, Secret Service on Linux), not in plain-text config.
3. **PII in model calls.** Configurable redaction policies. Customer emails, names, and shipping addresses are masked by default before being sent to remote models. Operator can opt in to sending PII per-persona. When using local models (Ollama, LM Studio), PII redaction is disabled by default since data stays on-machine.
4. **MCP permission model.** WooAgent OS authenticates to the store under one of two identities: (a) a scoped device token issued by the Companion Plugin's `wooagent/device-pair` ability — bound to a device, not a human user, and independently revocable from wp-admin (recommended); or (b) a WordPress Application Password tied to a WP user (fallback). In both cases the store's capability system determines which abilities are invokable, and WooAgent OS never escalates beyond the permissions the store grants. Device-pair tokens can be rotated or revoked per-device without touching WP user accounts.
5. **Action guardrails.** Every ability invocation has a default mode: `read`, `propose`, or `apply`. Agents default to `propose` for any write. The operator decides which ability/persona pairs graduate to `apply`.
6. **Rate limits and cost caps.** Per-persona daily token and dollar caps. Hard stop at cap; operator is notified.
7. **Audit log.** Every model call, ability invocation, and state mutation is appended to a tamper-evident local log.
8. **Offline mode.** A single setting that forbids any outbound call to a non-local endpoint. Useful for sensitive stores or testing. In offline mode, only local models and repo-mode skills are available.

## 15. Extensibility

1. **Custom personas.** Operators can fork any default persona, edit its system prompt, ability bindings, Agent Skills context, and model preferences, and redeploy.
2. **Custom local skills.** Operators ship skills as standalone executables that speak the WooAgent skill protocol (gRPC over stdio, following the go-plugin pattern used by Terraform providers and similar Go ecosystems). Skills can be written in any language — Go, Python, TypeScript, Rust — as long as they implement the protocol. The orchestrator discovers skill binaries in `~/.wooagent/skills/` and in any repo-local `./wooagent.skills/` directory. A Go skill SDK built on ADK Go is shipped as the default path; reference Python and TypeScript SDKs follow in v1.
3. **Custom WordPress abilities.** Operators can register custom abilities on their store using `wp_register_ability()`. These are automatically discovered by WooAgent OS on the next MCP refresh. No WooAgent OS code change needed.
4. **Skill packs.** Community-published bundles (e.g., "WooSubscriptions pack", "German VAT pack") installable via `wooagent skills install <n>`. Skill packs can include both local skills and recommended WordPress plugin configurations.
5. **Webhook handlers.** Operators can register custom logic that turns inbound Woo webhooks into issues routed to specific personas.
6. **UI themes.** The board view supports theming via CSS variables.
7. **Agent Skills contributions.** Operators can write custom WordPress Agent Skills in Markdown and load them into their personas, or contribute them upstream to `WordPress/agent-skills`.

## 16. Roadmap

### v0.1 (Alpha)
- Single-store, single-operator.
- MCP client with ability discovery and caching.
- **Full fleet visible in UI: all six operator personas + Chief of Staff appear in the roster from first run.** Marketing, Pricing, and Sales Support are fully functional end-to-end (read → propose → approve → write). Inventory, Accounting, and Reporting ship UI-visible but inert (coming-soon state; optional demo-seeded sample issues illustrate their work shape without invoking MCP writes). Chief of Staff is scaffolded — generates the first-run store-profile issue; autonomous coordination lands in v0.2. Read-only mode (MCP reads only) across the fleet.
- Anthropic, OpenAI, Ollama providers.
- Kanban UI, CLI, SQLite storage.
- Ability browser in the UI.
- REST API fallback connector.
- **Minimal Companion Plugin (`wooagent/device-pair`)** — ships alongside the daemon so device pairing is the default first-run experience. Application Password fallback continues to work for stores that can't install the plugin.

### v0.2 (Beta)
- Write operations with operator approval (`propose` mode via MCP).
- Google Gemini and LM Studio providers.
- Webhook ingestion.
- Chief of Staff meta-agent with store profiling.
- Run-log viewer with replay.
- WordPress Agent Skills integration.

### v0.3
- **Full Companion Plugin** — adds `wooagent/staged-changes`, `wooagent/issue-status`, and `wooagent/guardrail-policy` on top of the Minimal pairing plugin shipped in v0.1.
- Local Woo repo integration (`repo.*` skills).
- Cost caps and offline mode.
- Skill pack installer.
- Multi-store support.

### v1.0
- `apply` mode with per-ability guardrail policy.
- Read-only DB replica support.
- Cohort and KPI dashboards.
- Accounting exports (CSV, IIF, QBO).
- Stable skill ABI; third-party skill packs supported.
- Full xAI Grok provider.

### Post-v1
- Browser-automation skills (headless agents for tasks without abilities or API).
- Agent fleet profiles (preset configurations by vertical: fashion, food, B2B, digital goods).
- Multi-operator / team mode with role-based access.
- Optional hosted control plane for agencies managing many stores.
- Two-way MCP: WooAgent OS registers itself as an MCP server so other AI tools can interact with the agent fleet.

## 17. Success Metrics

1. **Time-to-first-issue:** Operator completes install, connects via MCP, and sees the first agent-produced issue in under 10 minutes on a fresh machine.
2. **Ability coverage:** On a typical WooCommerce store with 5+ plugins, agents successfully use abilities from 3+ plugins within the first session.
3. **Retention:** Operators running at least one agent daily after 30 days.
4. **Action quality:** Operator approval rate of agent-proposed writes above 70% at steady state.
5. **Catalog:** 20+ community skill packs within 12 months of v1.
6. **Stars and forks** as proxy for community velocity, but secondary to active operators.

## 18. Open Questions

1. **Default local model.** Which small model best handles tool-use (MCP ability invocation) at acceptable quality on consumer hardware? Needs to reliably generate well-formed ability calls.
2. **Chief of Staff autonomy.** Fully autonomous arbitration of cross-agent conflicts vs. always-ask-the-operator? Current lean is propose-mode for conflict resolution with the operator as tiebreaker.
3. **Companion plugin scope.** How much functionality belongs in the WordPress companion plugin vs. WooAgent OS itself? The companion plugin should stay thin, but operators may want richer wp-admin integration.
4. **Ability schema evolution.** When a plugin updates and changes its ability schemas, how does WooAgent OS handle the drift? Current thinking: re-discover on each `wooagent run` startup, diff against cache, and alert on breaking changes.
5. **Pricing agent data sources.** Bundle a default competitor-data fetcher or leave that as a skill pack? The MCP ecosystem doesn't solve this since competitor data doesn't live on the operator's WordPress site.

## 19. Competitive Positioning

WooAgent OS sits in a unique position relative to the WordPress AI ecosystem:

- **vs. the WordPress AI Plugin:** The AI Plugin is a reference implementation for one-shot AI features (generate a title, summarize a post). WooAgent OS is persistent multi-agent orchestration with work tracking.
- **vs. Claude Desktop / Cursor / coding agents + MCP:** These are general-purpose agents that can connect to a Woo MCP endpoint. WooAgent OS ships opinionated store-ops personas with domain knowledge, a propose/approve workflow, and a purpose-built kanban UX. You don't have to prompt-engineer a pricing strategy from scratch every session.
- **vs. AI Engine / plugin-level AI:** AI Engine and similar plugins add AI features within WordPress. WooAgent OS runs externally on the operator's machine and manages a fleet of agents that coordinate across all store concerns.
- **vs. MainWP / agency tools:** MainWP manages multiple WordPress sites. WooAgent OS could sit alongside MainWP, using MainWP's abilities (MainWP registers abilities) to coordinate across a multi-site agency portfolio.

The positioning is: WordPress built the building blocks. WooAgent OS is the operations layer.

## 20. Glossary

- **Ability:** A capability registered by a WordPress plugin via `wp_register_ability()` and exposed through MCP. Examples: `woocommerce/products/list`, `yoast-seo/meta/update`.
- **Agent / Persona:** A configured ADK agent with a mandate, skill set, ability bindings, Agent Skills context, and model preferences.
- **Agent Skill:** A portable Markdown instruction bundle from `WordPress/agent-skills` that teaches agents WordPress domain knowledge.
- **Local Skill:** A typed, versioned, permissioned computational primitive that runs inside WooAgent OS (not via MCP).
- **MCP (Model Context Protocol):** The protocol WordPress uses to expose abilities to external AI agents. WooAgent OS connects as an MCP client.
- **MCP Adapter:** The official WordPress plugin that converts registered abilities into an MCP endpoint. Required on the store (or provided by the host).
- **Issue:** The unit of trackable work. Lives in a column on the board.
- **Run:** One execution of an agent on an issue. Contains model calls, ability invocations, skill calls, and state changes.
- **Harness:** The orchestrator plus the agent runtime plus the model abstraction plus the MCP client.
- **Propose / Apply:** The two write modes. `Propose` stages a change for review; `Apply` invokes the ability with write parameters directly.
- **Chief of Staff:** The meta-agent that triages, routes, profiles stores, and summarizes.
- **Companion Plugin:** Optional lightweight WordPress plugin that registers WooAgent-specific abilities on the store side.
