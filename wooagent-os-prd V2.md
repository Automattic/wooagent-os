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
7. A WordPress.com account or Jetpack connection as a hard dependency. WooAgent OS runs end-to-end against any self-hosted WordPress/Woo store with the MCP Adapter installed; no SaaS tether is required or used by the core path. Stores that do have a Jetpack connection get access to an enhanced ability surface (see §11.7), but this is strictly an optional enhancement.

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

### 8.1 MCP Abilities (Remote, Discovered Universally, Trust Curated)

These are provided by the store. WooAgent OS discovers every ability registered via `wp_register_ability()` on the connected store, caches the schemas, and makes them visible in the Ability explorer. **Discovery is universal. Trust is curated** — which abilities are actually invocable by which agents is governed by the pre-signed manifest described in §8.4, not by the fact of discovery alone.

**Baseline (always present on any store running our Companion Plugin):**
The `wooagent-*` ability namespaces registered by the WooAgent OS Companion Plugin (§11.4) — products, orders, customers, device pairing, and (from v0.3) staged changes and guardrail policy. These are pre-signed by construction: we ship both sides of the contract.

**WooCommerce core abilities** (registered under `woocommerce/*`):
Products (list, get, create, update), orders (list, get, refund), customers (list, get), categories, tags, coupons, stock, variations. When the WooCommerce AI plugin is installed, its 10 local `woocommerce/*` abilities (`analyze-shipping-needs`, `create-bookable-product`, `manage-shipping-zones`, `search-products`, etc.) light up alongside the core set.

**Plugin-provided abilities** (discovered from any plugin using `wp_register_ability()`):
Yoast SEO (`yoast-seo/*`), ACF (`acf/*`), Jetpack Forms (`jetpack-forms/*`), Ninja Forms, WS Form, Gravity Forms via GravityMCP, WPForms, Jetpack, MainWP, WPCode, hCaptcha, miniOrange SSO/OAuth/LDAP, and any other plugin using the Abilities API.

**WordPress core abilities** (registered by WordPress 7.0 itself):
Posts, pages, media, users, settings, taxonomies, plus `core/get-site-info`, `core/get-environment-info`. Includes proposals in active development: `core/get-settings`, `core/update-settings`, `core/get-user`, `core/get-post`.

A freshly-discovered plugin namespace is not automatically trusted. It is surfaced to the operator through the Ability explorer with a clear `unapproved` badge, and the operator can promote it to `operator-approved` after inspecting the schema, or it can be picked up by the pre-signed manifest in a subsequent WooAgent OS release. See §8.4 for the full trust model.

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

### 8.4 Ability Trust Model — Pre-Signed Manifest and Policy Enforcement

LLMs produce tool calls, but LLMs are untrusted: prompt injection, hallucinated arguments, and compromised plugin updates ("rug pulls") can all result in a model deciding to invoke an ability that should not be invoked. WooAgent OS's stance is that **trust in a tool call cannot come from the model's self-report** — it has to be decided ahead of time, enforced deterministically at runtime, and cryptographically grounded where possible. This section describes how.

The design has two layers: a **pre-signed manifest** (the allowlist, decided ahead of time) and a **Policy Enforcement Point** (the runtime gate, enforced at every invocation).

#### 8.4.1 The pre-signed manifest

A curated JSON document shipped with every WooAgent OS release. It enumerates trusted plugin namespaces and the specific abilities within each namespace that the daemon is willing to invoke by default. Each manifest entry contains:

- **Fully-qualified ability name** (e.g., `yoast-seo/meta.update`, never bare `meta.update`). Prevents namespace impersonation.
- **Namespace owner and source-of-truth URL** (plugin slug on WordPress.org, or GitHub repo for first-party abilities). Human-readable provenance.
- **Expected schema hash** — a hash over the ability's `input_schema` + `output_schema` as registered. Detects silent contract changes.
- **Version constraint** — semver range or plugin version range the manifest entry is valid for.
- **Default persona mappings** — which personas get access (e.g., `yoast-seo/meta.update` maps to Marketing & SEO).
- **Default scope** — `read`, `propose`, or `apply`. Least-privilege by default; `apply` is opt-in per ability.

The manifest lives at `~/.wooagent/manifest.json` after install, is inspectable by the operator, and is extensible: operators can add entries for abilities they've reviewed themselves. Upstream additions (new plugins, updated schema hashes for plugin releases) flow through pull requests to `github.com/wooagent-os/manifest` and ship with daemon releases. The manifest file itself is signed by the WooAgent OS release key; the daemon refuses to load an unsigned or mismatched manifest.

**Three trust states apply to every discovered ability:**

- **`pre-signed`** — matches a manifest entry, schema hash verifies, version constraint holds. Invocable by mapped personas at the manifest's default scope.
- **`operator-approved`** — not in the manifest, but the operator has explicitly enabled it after reviewing the schema. Invocable only by the personas and at the scope the operator specified.
- **`unapproved`** — discovered but not trusted. Visible in the Ability explorer, dormant to every persona. Cannot be invoked.

**Drift detection.** On every connection and every periodic re-sync, the daemon recomputes the schema hash of each discovered ability and compares it to the manifest. A mismatch auto-demotes the ability from `pre-signed` to `unapproved` and alerts the operator: *"the `yoast-seo/meta.update` ability's schema changed since the last manifest update; review required."* This catches plugin updates that silently alter contracts, as well as a malicious plugin replacing a legitimate namespace.

#### 8.4.2 The Policy Enforcement Point (PEP)

The PEP is deterministic Go code sitting between the agent orchestrator and the MCP client. Every ability invocation from every persona flows through it. There is no orchestrator → MCP shortcut. The PEP is **not an LLM**, holds no model state, and cannot be prompt-injected. Its behavior is reproducible, auditable, and covered by unit tests — not by vibes.

On every invocation, the PEP checks, in order:

1. **Trust state.** Is the ability `pre-signed` or `operator-approved`? If not, deny with `ability_unapproved`.
2. **Persona scope.** Is this persona permitted to invoke this ability? (Manifest default, plus operator overrides.) Deny with `persona_forbidden` otherwise.
3. **Schema validation.** Do the arguments validate against the ability's `input_schema`? Deny with `invalid_arguments` otherwise. Structural, not semantic.
4. **Policy predicates.** Do the arguments satisfy any operator-configured policies? (e.g., *"never invoke `woocommerce/products-update` with `price < cost + 10%`,"* *"never invoke `orders-refund` for an order >$500 without explicit per-issue approval."*) Deny with `policy_violation` otherwise.
5. **Budgets.** Is this invocation within the persona's daily token/cost/call budgets? Deny with `budget_exceeded` otherwise.
6. **Scope sufficiency.** If this is a write and the ability's scope is `read` or `propose`, deny with `scope_insufficient` unless the call is explicitly staging a proposal (not applying it).

If all checks pass, the PEP mints a short-lived scoped capability token bound to this single invocation, routes the call through the MCP client, and records a full **chain-of-identity** record to the audit log:

```
plan_id → task_id → step_id → persona → model → prompt_hash → ability → arguments → outcome
```

Every row is append-only, tamper-evident, and queryable. See §15.

If any check fails, the orchestrator receives a typed `permission_denied` error with the reason code. The agent can retry with adjusted arguments, but cannot bypass the PEP.

#### 8.4.3 Invariants

- The agent never holds long-lived store credentials. The OS keychain holds the device-pair token (or Application Password); only the PEP accesses it.
- No ability can be invoked except through the PEP. There is no direct orchestrator → MCP client path.
- Manifest updates never take effect without operator review — the daemon shows a diff at upgrade time and waits for confirmation.
- Every ability call is traceable to an operator-approved plan, task, or explicit per-issue sign-off.

#### 8.4.4 What this buys us

- **Prompt injection becomes an inconvenience, not a breach.** A malicious prompt that convinces the model to call `woocommerce/products-update { id: 1, price: 0 }` is simply denied by the policy predicate.
- **Rug-pull resistance.** A plugin update that silently changes an ability's contract to do something different gets caught by schema-hash drift and surfaces for operator review before any agent can invoke the new version.
- **Auditable autonomy.** The operator can always answer "why did the agent do that?" because every invocation has a cryptographically-linked chain back to a human approval.

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

The sidebar groups destinations into three labelled clusters. Items prefixed *(v2)* are rendered as visible-but-paused placeholders so the operator sees the full breadth of the surface without expecting them to function.

- **INBOX**
  - *(v2)* **My issues.** Items assigned to the operator for review or originated by the operator. V2 — render-only in v1.
  - **Board.** The kanban board (the main screen). Carries an "in review" count badge.
- **FLEET**
  - **Agents.** Fleet roster: status, model, last-run, abilities/skills enabled. Replaces per-persona sidebar items — the agent list is the single entry point into a persona's profile and edit modal.
  - **Abilities.** Browser for all discovered MCP abilities and local skills. Shows which store plugin provides each ability, the schema, and which personas have access. Toggle on/off per agent.
  - **Runtimes.** Which model endpoints are configured and healthy.
- **SETTINGS**
  - **Stores.** Store connections (MCP endpoint, Companion Plugin pairing, Application Password fallback). See §11.
  - **Guardrails.** Per-persona policy predicates, reversibility thresholds, daily budgets. See §10.5 + §8.4.
  - **Secrets.** API keys and model-provider credentials. Shown masked; rotated, scoped, audited.

A connected-store footer at the bottom of the sidebar shows the current MCP endpoint hostname + environment label (e.g. "Pressable staging") with a health-state indicator dot.

### 9.4 Key Interactions

- Drag-drop issues between columns. Some transitions trigger actions (moving from In Review to Done applies the staged change via MCP).
- Create an issue manually and assign to any persona.
- Issue `@mention` routing (`@pricing audit holiday sale margins`) from anywhere in the UI or via CLI.
- One-click rerun of any past run with same or different model.
- **Ability explorer.** Click any discovered ability to see its schema, try a test invocation, and see which agents use it.

### 9.5 Batches

A **batch** is a lightweight parent grouping N sibling issues that share a generation prompt — e.g. one "rewrite metadata for these 7 products" run produces seven child issues, all tagged with the same `batch_id`. The operator reviews the batch as a set rather than seven separate kanban cards.

Batches and variants are **orthogonal** axes:

- A **variant** is one of N alternative bodies *for the same proposal* on a single issue (`proposal.target.variants[]`). Issues commonly carry 3 variants (A / B / C) with different voice or SEO trade-offs; the operator picks one before approving.
- A **batch** is N sibling issues sharing a parent. Each child can independently carry its own variants. A 7-product batch where each product has 3 voice variants is `7 × 3` = 21 candidate bodies; the operator picks one variant per child, then approves the batch.

The lightweight grouping is a deliberate scope decision: batches are a metadata table + an `issues.batch_id` foreign key. Counters (Pending / Approved / Rejected) are derived at read time from joined children; batches have no separate lifecycle and can never drift out of sync. This avoids a duplicate state machine on top of the per-issue one already in §9.1.

**Review surface.** Batches open at `/batches/:id` (not on the kanban itself; kanban cards with a `batch_id` route into the batch view rather than the single-issue detail view). The screen renders:

- A title + counter strip showing approved / rejected / pending children with a progress bar.
- Per-child accordion rows. Each header shows the child's product name, slug, best variant scores, and pending/approved/rejected pill. Expanded body lays out the current copy alongside the variant columns, with per-row variant pick + per-row approve/reject buttons (uses the single-issue endpoints).
- A sticky bottom bar with batch-level **Approve all** + **Reject all**. Approve-all carries the per-child variant picks (`{children: [{issue_id, variant_id}, ...]}`) and runs each child through PEP independently.

**Best-effort approve-all.** Batch approve-all is a multistatus operation: it loops sequentially over children, runs each through the full Policy Enforcement Point (§8.4.2), and always returns `200` with a per-child `{ok, status?, error?}` array — never rolls back successful children when a sibling denies. This keeps the per-child audit story granular (one audit row per `pep.Invoke` call, identical to single-issue approve) and matches operator intuition that one bad row shouldn't stall the rest.

**Creation in v1.** Batches are created by `POST /v1/batches` from seed scripts, the persona-marketing CLI, or future agent runs; v1 has no in-UI "compose a batch" affordance. An autonomous agent loop that produces N issues from a single prompt is a separate post-v1 concern.

### 9.6 Top bar

A persistent top bar runs above the main content column on all screens (collapses into the mobile hamburger bar below 768px). It carries:

- **Search.** Global search input. Stub in v1 — rendered to lock the layout for when search lands.
- **Ask agent.** Opens a right-anchored command-palette drawer (⌘K toggle, Esc to close). The drawer shows a context chip naming the current page ("Board · Today's marketing queue · 9 items"), a static "Suggested for this page" list with four queries, and a "Recent" list. V1 ships the affordance shell; the actual NL routing of asks into agent runs is v2.

## 10. Insights, Planning, and Background Execution

WooAgent OS is not a chat-first assistant that waits to be asked. The daemon is designed to run continuously against a connected store, surface what matters, propose what to do, and — once approved — execute in the background while the operator gets on with their day. This section describes the work loop that sits between raw MCP access and the Kanban surface.

The loop has four stages: **insight → plan → task → execution.** Each stage is independently useful (an operator can open a plan from scratch without an insight, or approve a bare task without a plan), and each stage persists as a first-class object in the daemon's store.

### 10.1 Insight generation and scoring

Agents — primarily Chief of Staff, supported by the persona fleet — generate **insights** from observed store state. Insights are short, structured findings: "15 products have no images," "orders from Germany spiked 40% week-over-week," "three coupons expire tomorrow." Insight generation runs on a schedule (daily by default, configurable per persona), not synchronously on UI load.

Every insight is scored across four dimensions:

| Dimension | Range | What it captures |
| --- | --- | --- |
| **Impact** | 0.0–1.0 | Estimated effect on revenue, conversion, or operator time if acted on. |
| **Confidence** | 0.0–1.0 | How certain the agent is that the finding is real and not noise. |
| **Urgency** | 0.0–1.0 | How time-sensitive the action is (expiring coupon = high; blog topic idea = low). |
| **Reversibility** | 0.0–1.0 | How easy it is to undo. Editing a draft = 1.0; issuing a refund = 0.2; deleting an order = 0.0. |

Scoring serves two purposes. First, it **ranks** insights on the Kanban Backlog so the operator sees the right things on top. Second, it **feeds policy**: the reversibility dimension is a direct input to the approval-gate rules in §10.5. A highly-reversible action can be auto-run inside an approved plan; a low-reversibility action always surfaces for review regardless of how it was initiated.

Insights are not a separate UI surface. They appear in the Kanban Backlog as issues tagged `insight`, with the four scores visible on the card. Clicking "Explore this" on an insight opens the planning surface described in §10.2 with the insight attached as context.

### 10.2 The plan as a durable artifact

When an operator asks an agent to plan something — either from an insight or from scratch — the agent writes a **plan**: a markdown document, separate from the chat, that evolves through conversation. Plans are owned by the daemon and persisted on disk, not ephemeral chat state.

Why a separate document:

- **Chat scrolls; plans don't.** A plan embedded in conversation history gets buried. A plan as a standalone artifact stays readable, diffable, and shareable.
- **Any UI can render it.** Because the plan is a markdown file on the daemon, the React app, the CLI (`wooagent plan show`), and any future surface can all display the same artifact.
- **It survives restart.** Plan drafts persist by default. An operator can close the laptop, come back tomorrow, and the plan is where they left it.

The agent evolves the plan through two internal abilities:

- `wooagent/plan.start` — opens a new plan with initial content, linked to an issue.
- `wooagent/plan.update` — replaces the plan body.

Plans are freeform markdown. No required template. The agent writes whatever structure fits the problem. The operator can edit the plan directly (contenteditable in the React UI, `$EDITOR` from the CLI) or steer it through conversation with the agent.

### 10.3 Plan-to-task conversion

When the operator is happy with a plan, they **approve** it. Approval does two things:

1. A structured task is extracted from the plan: `title`, `goal`, ordered `steps[]`, estimated cost/time, persona assignment. Extraction uses an LLM call against the same provider the agent is running on.
2. The task is persisted to the daemon's task store (SQLite, not WordPress options) and appears in the Kanban `Todo` column.

The plan itself stays attached to the task as its source document. Operators (and agents) can re-open the plan later to see the thinking behind a completed task. Re-extraction is cheap and idempotent, so editing the plan after the fact and re-approving is supported.

### 10.4 Step semantics: goals, not tool calls

Steps inside a task are written as **goals**, not as specific ability invocations. "Find all products without images and list their IDs" is a step. "Call `woocommerce/products.list` with `meta_query={_thumbnail_id: NOT EXISTS}`" is not.

Each step runs as a fresh orchestrator session with access to the assigned persona's abilities. The orchestrator reasons about which tools to use, retries on failure, tries alternatives when a tool is unavailable, and passes context forward to the next step. This is the pattern the ADK-Go agent loop already implements — steps are just goal-scoped sessions chained together.

This matters because stores differ. The same goal ("tag every product that hasn't sold in 90 days") may require `woocommerce/*` abilities on one store and a mix of `woocommerce/*` + custom plugin abilities on another. Goals survive that variation; tool-call recipes don't.

### 10.5 Approval gates and provenance

Every task starts in one of two approval states, determined by its **provenance**:

| Provenance | Initial state | Rationale |
| --- | --- | --- |
| Operator approved a plan, plan was extracted into this task | `approved`, ready to run | The operator already reviewed and signed off on the plan. Re-asking would be noise. |
| Agent spawned this task on its own (e.g., Chief of Staff chained follow-up work) | `pending_approval` | The operator has not seen this. Human-in-the-loop required before anything runs. |

Within an approved task, individual steps may still surface for review based on **reversibility**. A step whose primary ability has `reversibility < 0.5` (storewide price change, refund issuance, bulk deletion) transitions the task to Kanban's **In Review** column and waits for per-step approval, even though the task as a whole is approved. This keeps the "approve once" ergonomics of a plan without opening the door to irreversible damage from a misread step.

Operators can override the reversibility threshold per persona in settings. The default threshold for v1 is 0.5.

**Batch approve-all is a multistatus shape**, distinct from per-step gates above. When the operator approves a batch (§9.5), each child runs through PEP independently and gets its own audit row; the call always returns success with a per-child `{ok, error?}` array, never rolling back successful children on a sibling denial. This preserves the granular per-step audit story while letting the operator dispatch a whole batch in one click.

### 10.6 Background execution

Once a task is approved, the daemon runs it in the background. Step by step. Without the operator on the page. The operator can close the UI, close the browser, shut the laptop — the daemon continues.

This is where WooAgent OS's architecture pays off relative to browser-bound agents. Prototypes elsewhere in the ecosystem achieve background execution by dispatching steps to WPCOM async jobs over Jetpack Connection, because the browser is the only always-available runtime in that stack. WooAgent OS already has an always-available runtime: the Go daemon. There is no round-trip to WPCOM, no per-step async job, no cold-start. A step's orchestrator session runs in-process and calls MCP abilities directly against the connected store.

Execution produces:

- **Step results** — structured output attached to each step, visible in the issue detail view.
- **Run logs** — every model call, every ability invocation, every retry, timestamped and replayable (§9.2).
- **Change deltas** — every write ability emits a structured `_delta` record (before/after) attached to the run log. Deltas are the substrate for v2 undo (see §17 Roadmap); v1 uses them for diffs in the Kanban `In Review` and `Done` columns.

Tasks that fail or need attention transition back to `In Review` with the partial state preserved. The operator can modify the plan, re-approve, and resume — the daemon picks up from the first incomplete step.

### 10.7 Deferred to later versions

The following are explicitly out of scope for v1 and tracked in §17 Roadmap:

- **Smart Undo ability.** A generic `wooagent/undo` tool that reads `_delta` records from a task's run log and reverses them. v1 captures the deltas; v2 ships the undo.
- **Inline task progress components.** Rendering live task progress inside a chat message or plan document, rather than only in the Kanban issue view.
- **Mid-execution interaction.** Pausing a running task to ask the operator a clarifying question, then resuming. v1 runs approved tasks to completion or to failure; mid-run prompts are a v2 design question.
- **Auto-approval policy learning.** Letting the operator teach the system "always auto-approve price changes under $5" rather than setting a per-persona threshold. v1 exposes thresholds; v2 could learn them from approval history.

## 11. Store Integration Model

### 11.1 MCP Endpoint (Primary Mode)

The operator connects a live WordPress/WooCommerce store by its MCP endpoint URL. Authentication uses one of two mechanisms, in priority order:

1. **Device pairing via the Companion Plugin (default, recommended).** On first connection, the daemon displays a short pairing code. The operator opens **wp-admin → WooAgent → Pair device**, enters the code, and approves. The Companion Plugin mints a scoped token bound to a WooAgent device identity (not a WP user), which the daemon stores in the OS keychain. Tokens are independently revocable per-device from wp-admin. See §11.4. This is the primary path and the default in `wooagent init`.

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

### 11.2 REST API Fallback

For stores not yet on WordPress 7.0 or without the MCP Adapter, WooAgent OS can connect via WooCommerce REST API key pair (consumer key + secret). This mode has reduced capability: agents can only access WooCommerce data (products, orders, customers), not the broader WordPress ecosystem. The UI displays a clear notice that MCP mode is recommended.

### 11.3 Webhooks

The orchestrator optionally subscribes to WooCommerce webhooks for real-time event ingestion (new orders, stock changes, customer registration). Requires an HTTPS ingress (via ngrok, Cloudflare Tunnel, or operator-provided tunnel). Webhook events are converted to issues and routed to the appropriate persona.

### 11.4 WooAgent OS Companion Plugin (Baseline Ability Provider)

The Companion Plugin is the **baseline ability surface** that guarantees WooAgent OS works out of the box on any WooCommerce store, without requiring the operator to install or configure any other plugin. It is pre-signed by construction (we ship both sides of the contract) and is the anchor that makes the no-WPCOM / no-third-party-dependency stance in §4 concrete. It ships in two tiers.

**Minimal Companion Plugin (v0.1, the baseline):**

- **`wooagent-products/*`** — `list`, `get`, `update`. A minimum-viable product-catalog surface that every WooAgent OS install can rely on, independent of whether the WooCommerce AI plugin (or any other plugin) is present.
- **`wooagent-orders/*`** — `list`, `get`, `add-note`. Order inspection and annotation.
- **`wooagent-customers/*`** — `get`. Customer lookup.
- **`wooagent-device-pair/*`** — `request`, `confirm`, `revoke`. Implements the device-pairing flow: exposes a **WooAgent → Pair device** screen in wp-admin, validates operator-entered pairing codes against pending requests from a WooAgent daemon, mints scoped tokens bound to a WooAgent device identity, and provides per-device revocation. This replaces hand-managed Application Passwords as the default auth path and gives operators a Claude-Code-style pairing experience instead of basic-auth credential copy-paste.

**Full Companion Plugin (v0.3):**

- `wooagent-staged-changes/*` — allows the store admin to see and approve changes staged by agents directly in wp-admin.
- `wooagent-issue-status/*` — syncs issue status between WooAgent OS and a wp-admin dashboard widget.
- `wooagent-guardrail-policy/*` — lets the store admin define per-ability guardrails (policy predicates the PEP in §8.4.2 will enforce).

**Positioning.** The Minimal plugin is the default path in `wooagent init`, the UX is designed around it, and every persona in §7 has at least one invocable ability from it. Operators can still fall back to WordPress Application Password auth (§11.1) and skip the Companion Plugin entirely — WooAgent OS will connect and discover whatever other abilities the store has — but in that configuration, the available ability surface is entirely dependent on what third-party plugins the store has installed, and may be empty. Installing the Minimal plugin is the only way to guarantee a working baseline. The Full plugin is additive — once the Minimal plugin is installed, enabling the Full features is an in-plugin toggle, not a separate install.

**Trust.** Because WooAgent OS ships the Companion Plugin, the manifest entries for `wooagent-*/*` abilities are signed by the WooAgent OS release key with matching schema hashes. They will always be `pre-signed` on any release version the operator is running. The drift-detection behavior in §8.4.1 applies equally here: if the Companion Plugin installed on the store doesn't match the daemon's expected schema, the abilities demote to `unapproved` pending operator review, same as for any third-party plugin.

### 11.5 Local Repo (Optional Mode)

If the operator has the Woo codebase available locally (for custom plugins, themes, migrations), they can register the repo path. Agents with `repo.*` skills can read and propose changes. Writes are always via git branches with the agent as author; commits happen only after operator approval, and pushes are never automatic. Repo-mode agents load `wp-plugin-development`, `wp-phpstan`, and `wp-wpcli-and-ops` Agent Skills as context.

### 11.6 Read-Only Database Replica (Advanced)

For stores where agents need analytical queries beyond what MCP or the REST API supports cheaply, the operator can configure a read-only MySQL connection to a Woo replica. Skills can run typed, sandboxed SQL with LIMIT enforcement and query cost budgets.

### 11.7 WPCOM-Enhanced Mode (Optional)

Stores that are already connected to WordPress.com via Jetpack get access to an **enhanced ability surface** on top of everything in §11.1–§11.6. This is strictly optional, strictly additive, and never a prerequisite for core functionality. A store without a Jetpack connection is a first-class citizen.

**What lights up when `jetpack_connected: true`:**

- **The WooCommerce AI plugin's progressive MCP registry** — the 50-tool surface (14 providers: products, orders, customers, coupons, product categories, product tags, order notes, order refunds, shipping zones, payment gateways, settings, analytics, tax rates, system status) served via the plugin's `remote_mcp_url` at `public-api.wordpress.com/wpcom/v2/woocommerce-ai-mcp/v1`. Roughly 5× the coverage of the plugin's local Abilities API surface, particularly for Pricing (coupons, taxes), Accounting (payment-gateways, settings, system-status), and Reporting (analytics-revenue, analytics-orders).
- **WPCOM-native aggregations** — abilities injected by the WPCOM proxy layer that are not registered on the site at all (e.g., cross-store aggregation, WooCommerce Analytics with extended history, third-party service MCPs bridged via WPCOM).
- **Future WPCOM-side AI features** registered as abilities — whatever the ecosystem ships gets picked up without a WooAgent OS change.

**How the operator opts in:**

1. In `wooagent init` or Settings, the operator toggles **"Enable WPCOM-enhanced mode."**
2. The daemon detects `jetpack_connected: true` via the WooCommerce AI plugin's `/settings` endpoint (or a direct Jetpack status check) and enables the toggle.
3. WPCOM-enhanced abilities appear in the Ability explorer alongside local abilities, tagged `wpcom-enhanced`.

**Trust model unchanged.** WPCOM-enhanced abilities flow through the same pre-signed manifest and PEP described in §8.4. A WPCOM-routed ability must match a manifest entry or be explicitly operator-approved before any persona can invoke it. Connection to WPCOM does **not** imply trust, and the PEP applies every check (schema validation, policy predicates, budgets, scope) regardless of whether the ability is local or WPCOM-routed.

**Data flow and sovereignty.** When the daemon invokes a WPCOM-enhanced ability, the request goes: daemon → `public-api.wordpress.com/wpcom/v2/woocommerce-ai-mcp/v1` → Jetpack → store. The response returns on the same path. The daemon's own state (issue database, audit log, plans, secrets) never leaves the operator's host. The only data WPCOM sees is the ability arguments and responses — exactly the same as it would see if the operator drove the plugin through its standard UI. There is no daemon-to-WPCOM persistent channel, no telemetry, no account keyed to WooAgent OS.

**Why this matters strategically.** Enhanced mode creates a genuine reason for stores to stay (or become) Jetpack-connected: their WooAgent OS deployment is measurably more capable when they are. That aligns WooAgent OS with Automattic's commercial interests without compromising the local-first guarantee for operators who can't or won't connect.

## 12. Technical Stack

1. **Agent runtime:** Agent Development Kit for Go (module path `google.golang.org/adk`; source at [github.com/google/adk-go](https://github.com/google/adk-go)) as the agent and skill framework. The orchestrator, CLI, MCP client, and agent runtime all run in a single Go process.
2. **Orchestrator / CLI:** Go. Single static binary per platform. Runs as a headless daemon by default — no UI is served unless the operator explicitly opts in. An optional `wooagent ui` subcommand serves a bundled copy of the standalone React UI for operators who want a one-command local experience. No sidecar processes or external language runtimes required at install time.
3. **MCP client:** Go implementation of the MCP client protocol. Handles connection lifecycle, ability discovery, caching, and invocation. The MCP client is the primary interface to the store.
4. **Web UI:** Standalone React SPA (built with Vite, static assets). Will ship independently from the daemon on its own release cadence. (Source currently lives in `ui/` inside the `github.com/elizaan36/wooagent-os` monorepo during the prototype phase; splits into its own repo at open-sourcing / first independent release, whichever comes first.) Uses the WordPress design system — `@wordpress/components`, `@wordpress/ui`, `@wordpress/icons`, and `@wordpress/i18n` — so the UI feels native to WordPress operators and inherits accessibility, theming, and internationalization. The **canonical reference** for which components are stable, their props, and the design-token vocabulary (colors, spacing, typography, elevation) is the WordPress Design System MCP server (`@wordpress/design-system-mcp`, published from [Automattic/wpds-mcp](https://github.com/Automattic/wpds-mcp)). Implementers query it via the MCP resources `wpds://components`, `wpds://components/{name}`, `wpds://design-tokens`, and `wpds://pages/{slug}` rather than guessing from TypeScript types or scraping docs. The UI communicates with any WooAgent OS daemon over its REST/gRPC API; operators configure the daemon URL and paste an auth token on first launch. The daemon exposes CORS headers so the UI can be served from any origin (hosted build, localhost, intranet). The daemon runs fully headless by default; serving the UI locally is a separate opt-in command (`wooagent ui`).
5. **Storage:** SQLite (default), Postgres (optional for scale).
6. **Job queue:** Embedded SQLite-backed queue in default mode. River (Postgres) when Postgres is configured.
7. **Model adapters:** Single abstraction layer that wraps Anthropic, Google (Gemini), OpenAI, xAI, and any OpenAI-compatible endpoint (which covers Ollama, LM Studio, llama.cpp server, vLLM, and most self-hosted inference servers).
8. **Observability:** Built-in run-log viewer. OpenTelemetry export optional.
9. **Auth (local):** Device-pair model for the local web UI. Single operator by default; add teammates explicitly.

### 12.10 Resolved architecture decisions

- **Single-language Go runtime (resolved, supersedes earlier drafts).** Earlier drafts proposed a Go orchestrator managing a Python ADK sidecar. With ADK Go (module `google.golang.org/adk`) now at v1.1.0 (Apr 2026), the agent runtime is implemented natively in Go. This preserves the single-binary install promise, eliminates cross-language IPC, and removes Python as an install-time dependency on the operator's machine. **Caveat discovered during the Apr 22 spike:** ADK Go's `model/` package ships only Gemini and Apigee natively. Other providers (Anthropic, OpenAI/OpenAI-compatible) plug in via third-party implementations of the `model.LLM` interface — currently `github.com/Alcova-AI/adk-anthropic-go` and `github.com/huytd/adk-openai-go`. Acceptable for v1; before Phase 5, either vendor these into `daemon/internal/models/` or follow the upstream `google/adk-go-community` effort (PR #242). The `model.LLM` interface is typed on `google.golang.org/genai` content, so any custom adapter is fundamentally a genai ↔ provider translator — same cost whether we write our own or rely on the third-party shims.

## 13. Model Provider Support

### 13.1 Frontier providers (v1)

- **Anthropic:** Claude Opus, Sonnet, Haiku. Native tool use.
- **Google:** Gemini Pro and Flash families. Native tool use.
- **OpenAI:** GPT-5 and successor families. Native tool use.
- **xAI:** Grok family. Native tool use.

### 13.2 Local runtimes (v1)

- **Ollama:** Discovered automatically on `localhost:11434`. All pulled models listed in the UI. Tool use via the model's native or emulated function-calling.
- **LM Studio:** Discovered on `localhost:1234` (configurable). OpenAI-compatible endpoint.
- **llama.cpp server / vLLM / any OpenAI-compatible endpoint:** Configured manually with base URL, optional API key, model name.

### 13.3 Model routing

- Each persona declares a preferred model family and a fallback chain.
- Operator can override globally (e.g., "use only local models") or per persona.
- Cost accounting per run, per agent, per model, visible in the UI.
- Offline mode: when enabled, any attempt to reach a non-local endpoint fails loudly rather than silently falling back.

## 14. Installation and First-Run

### 14.1 Install

```bash
# macOS
brew install wooagent-os/tap/wooagent

# Linux
curl -fsSL https://wooagent.dev/install.sh | sh

# Windows
winget install wooagent.os
```

### 14.2 First Run

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

### 14.3 Common CLI Commands

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

## 15. Security, Privacy, and Data

1. **Local by default.** No outbound network traffic except to the model provider(s) and the connected store's MCP endpoint.
2. **Secrets.** Store credentials, model API keys, and other secrets live in the OS keychain (Keychain on macOS, Credential Manager on Windows, Secret Service on Linux), not in plain-text config.
3. **PII in model calls.** Configurable redaction policies. Customer emails, names, and shipping addresses are masked by default before being sent to remote models. Operator can opt in to sending PII per-persona. When using local models (Ollama, LM Studio), PII redaction is disabled by default since data stays on-machine.
4. **MCP permission model.** WooAgent OS authenticates to the store under one of two identities: (a) a scoped device token issued by the Companion Plugin's `wooagent/device-pair` ability — bound to a device, not a human user, and independently revocable from wp-admin (recommended); or (b) a WordPress Application Password tied to a WP user (fallback). In both cases the store's capability system determines which abilities are invokable, and WooAgent OS never escalates beyond the permissions the store grants. Device-pair tokens can be rotated or revoked per-device without touching WP user accounts.
5. **Action guardrails.** Every ability invocation has a default mode: `read`, `propose`, or `apply`. Agents default to `propose` for any write. The operator decides which ability/persona pairs graduate to `apply`.
6. **Rate limits and cost caps.** Per-persona daily token and dollar caps. Hard stop at cap; operator is notified.
7. **Audit log.** Every model call, ability invocation, and state mutation is appended to a tamper-evident local log.
8. **Offline mode.** A single setting that forbids any outbound call to a non-local endpoint. Useful for sensitive stores or testing. In offline mode, only local models and repo-mode skills are available.
9. **Pre-signed ability manifest + Policy Enforcement Point.** The daemon never auto-trusts an ability just because it was discovered via MCP. Trust is conferred by a manifest shipped (and signed) with the daemon release, or by explicit per-ability operator approval. Every ability invocation routes through a deterministic Policy Enforcement Point (non-LLM Go middleware) that verifies trust state, schema match, persona scope, argument validity, operator-configured policy predicates, and budget. Schema drift between the manifest and the discovered ability auto-demotes the ability to `unapproved` and alerts the operator — catching plugin rug-pulls and silent contract changes. Every approved invocation is logged with full chain-of-identity (plan → task → step → persona → model → prompt → ability → outcome). See §8.4 for the full model.

## 16. Extensibility

1. **Custom personas.** Operators can fork any default persona, edit its system prompt, ability bindings, Agent Skills context, and model preferences, and redeploy.
2. **Custom local skills.** Operators ship skills as standalone executables that speak the WooAgent skill protocol (gRPC over stdio, following the go-plugin pattern used by Terraform providers and similar Go ecosystems). Skills can be written in any language — Go, Python, TypeScript, Rust — as long as they implement the protocol. The orchestrator discovers skill binaries in `~/.wooagent/skills/` and in any repo-local `./wooagent.skills/` directory. A Go skill SDK built on ADK Go is shipped as the default path; reference Python and TypeScript SDKs follow in v1.
3. **Custom WordPress abilities.** Operators can register custom abilities on their store using `wp_register_ability()`. These are automatically discovered by WooAgent OS on the next MCP refresh. No WooAgent OS code change needed.
4. **Skill packs.** Community-published bundles (e.g., "WooSubscriptions pack", "German VAT pack") installable via `wooagent skills install <n>`. Skill packs can include both local skills and recommended WordPress plugin configurations.
5. **Webhook handlers.** Operators can register custom logic that turns inbound Woo webhooks into issues routed to specific personas.
6. **UI themes.** The board view supports theming via CSS variables.
7. **Agent Skills contributions.** Operators can write custom WordPress Agent Skills in Markdown and load them into their personas, or contribute them upstream to `WordPress/agent-skills`.

## 17. Roadmap

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

## 18. Success Metrics

1. **Time-to-first-issue:** Operator completes install, connects via MCP, and sees the first agent-produced issue in under 10 minutes on a fresh machine.
2. **Ability coverage:** On a typical WooCommerce store with 5+ plugins, agents successfully use abilities from 3+ plugins within the first session.
3. **Retention:** Operators running at least one agent daily after 30 days.
4. **Action quality:** Operator approval rate of agent-proposed writes above 70% at steady state.
5. **Catalog:** 20+ community skill packs within 12 months of v1.
6. **Stars and forks** as proxy for community velocity, but secondary to active operators.

## 19. Open Questions

1. **Default local model.** Which small model best handles tool-use (MCP ability invocation) at acceptable quality on consumer hardware? Needs to reliably generate well-formed ability calls.
2. **Chief of Staff autonomy.** Fully autonomous arbitration of cross-agent conflicts vs. always-ask-the-operator? Current lean is propose-mode for conflict resolution with the operator as tiebreaker.
3. **Companion plugin scope.** How much functionality belongs in the WordPress companion plugin vs. WooAgent OS itself? The companion plugin should stay thin, but operators may want richer wp-admin integration.
4. **Ability schema evolution.** When a plugin updates and changes its ability schemas, how does WooAgent OS handle the drift? Current thinking: re-discover on each `wooagent run` startup, diff against cache, and alert on breaking changes.
5. **Pricing agent data sources.** Bundle a default competitor-data fetcher or leave that as a skill pack? The MCP ecosystem doesn't solve this since competitor data doesn't live on the operator's WordPress site.

## 20. Competitive Positioning

WooAgent OS sits in a unique position relative to the WordPress AI ecosystem:

- **vs. the WordPress AI Plugin:** The AI Plugin is a reference implementation for one-shot AI features (generate a title, summarize a post). WooAgent OS is persistent multi-agent orchestration with work tracking.
- **vs. Dolly / WordPress Agent:** WordPress Agent (formerly Dolly) is a general-purpose, WPCOM-hosted chat agent that adopts the personality of a single site and reaches the user through messaging channels (Telegram, Slack, WhatsApp, Email). WooAgent OS is a local-first, kanban-native ops runtime purpose-built for WooCommerce, running a fleet of specialized personas whose work is structured as trackable issues with propose/approve gates and full audit logs. Adjacent problems, different product shapes — the two can coexist on the same store.
- **vs. Claude Desktop / Cursor / coding agents + MCP:** These are general-purpose agents that can connect to a Woo MCP endpoint. WooAgent OS ships opinionated store-ops personas with domain knowledge, a propose/approve workflow, and a purpose-built kanban UX. You don't have to prompt-engineer a pricing strategy from scratch every session.
- **vs. AI Engine / plugin-level AI:** AI Engine and similar plugins add AI features within WordPress. WooAgent OS runs externally on the operator's machine and manages a fleet of agents that coordinate across all store concerns.
- **vs. MainWP / agency tools:** MainWP manages multiple WordPress sites. WooAgent OS could sit alongside MainWP, using MainWP's abilities (MainWP registers abilities) to coordinate across a multi-site agency portfolio.

The positioning is: WordPress built the building blocks. WooAgent OS is the operations layer.

## 21. Glossary

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
