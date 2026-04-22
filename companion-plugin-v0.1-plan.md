# Companion Plugin v0.1 — scope pull-forward

**Owner:** Elizabeth
**Date:** 2026-04-21
**Status:** Decided. Scaffolding shipped; install + verify remaining.

---

## What changed and why

The PRD (§10.4, §16) scoped the Companion Plugin's ability surface to v0.3 — only the `wooagent/device-pair` flow shipped in v0.1. That plan assumed WooCommerce and the WordPress plugin ecosystem would register rich `woocommerce/*`, `yoast-seo/*`, `acf/*` abilities via the Abilities API, and that WooAgent OS could ride those natively through MCP auto-discovery.

**Reality on the test store (`woo-demo-store-99cc5c.mystagingwebsite.com`, probed 2026-04-21):**
- MCP Adapter is installed and healthy — handshake works, protocol `2025-06-18`.
- **But only 5 abilities are registered across the entire install**: `core/get-site-info`, `core/get-environment-info`, and three Jetpack Forms abilities.
- **WooCommerce has registered zero abilities.** No `woocommerce/*` namespace. No products, orders, customers, coupons via the Abilities API.
- WooCommerce REST API (`/wc/v3/*`) works fine and the store has 15 seeded apparel products.

Without `woocommerce/*` abilities, the Phase 1 personas (Marketing, Pricing, Sales Support) can't operate through MCP as the PRD envisions. We have two real options:

1. Lean on the WooCommerce REST API fallback (PRD §10.2). Bypass MCP for Phase 1. Works today, but cedes the PRD's strategic bet on MCP-native architecture.
2. **Pull Companion Plugin scope forward** (this plan). Register `wooagent/*` abilities in our own plugin — they flow through the existing MCP Adapter automatically, agents talk MCP as designed, and when WooCommerce eventually ships `woocommerce/*` abilities natively, ours deprecate gracefully.

Option 2 preserves the PRD's positioning ("MCP-native, every plugin's abilities auto-discovered") while bridging the ecosystem gap on today's stores.

---

## v0.1 ability surface (11 abilities)

### Fully implemented (7)

| Ability | Used by | Purpose |
|---|---|---|
| `wooagent-products/list` | Marketing, Pricing | Paginated product summaries with price, description lengths |
| `wooagent-products/get` | Marketing, Pricing | Full product incl. description, short description, SEO meta |
| `wooagent-products/update` | Marketing, Pricing (via propose-mode) | Update any subset of editable fields |
| `wooagent-orders/list` | Pricing, Sales Support | Recent orders filterable by status/date |
| `wooagent-orders/get` | Sales Support | Full order with line items and customer refs |
| `wooagent-orders/add-note` | Sales Support | Internal or customer-facing order notes |
| `wooagent-customers/get` | Sales Support | Single customer with lifetime spend |

### Stubbed (3)

| Ability | Status | Reason |
|---|---|---|
| `wooagent-device-pair/request` | stub, 501 | Multi-step token flow deserves its own pass |
| `wooagent-device-pair/confirm` | stub, 501 | Ships in v0.2 |
| `wooagent-device-pair/revoke` | stub, 501 | Ships in v0.2 |

Registration is real so the ability surface is stable from day one; callbacks return a clear "use Application Password for now" error.

### Explicitly deferred

- `wooagent/staged-changes/*` — staging lives in the daemon's `In Review` column. Plugin-side staging remains v0.3 per PRD §10.4.
- `wooagent/guardrail-policy/*` — v0.3.
- Inventory, accounting, coupons abilities — out of Phase 1 scope per `progress.md`.

---

## Tradeoffs

**Upside**
- Unblocks the three Phase 1 personas against real products/orders/customers through the MCP path the PRD specifies.
- Uses the Abilities API as the clean interoperability seam. No daemon-side changes needed when Woo eventually ships native abilities.
- Every ability invocation respects WP user capabilities — the PRD's permission model (§10.1, §14) is preserved.
- Zero new HTTP routes. Nothing bypasses MCP. Zero new auth paths.

**Downside**
- Pulls ~2-3 days of PHP into Phase 1. Phase 4 (testing buffer) is the absorber per `progress.md`.
- Weakens the "zero-install on Pressable / WP Engine / WordPress.com" pitch in the short term — the demo now requires installing a plugin on the test store. Long term (when Woo ships native abilities) this reverses.
- Device-pair flow remains stubbed, so Application Password is the Phase 1 auth path. That was already the fallback per PRD §10.1.

---

## Timeline impact

### Phase 1 (Wed Apr 22 – Tue Apr 28)

Unchanged dates. One insertion and one shift:

- **Insertion: Wed Apr 22 (Elizabeth)** — install the Companion Plugin on the test store. Verify the 7 CRUD abilities appear via `GET /wp-json/wp-abilities/v1/abilities` and are invokable through MCP `tools/call mcp-adapter-execute-ability`. Fix any registration-time schema bugs.
- **Shift: Mon Apr 27 (Elizabeth)** — MCP client in the daemon now targets `wooagent/*` abilities directly instead of `woocommerce/*`. Functionally identical code path (both go through the Adapter's execute-tool), but the ability names differ.

**Phase 1 exit criteria unchanged.** The Marketing persona still needs to produce a real issue against the test store, the issue appears on a kanban board at rough fidelity to Nevena's designs, and daemon + React + test store + MCP work end-to-end.

### Downstream phases

- **Phase 2 (Wed Apr 29 – Tue May 5)** — no change to scope.
- **Phase 3 (Wed May 6 – Tue May 12)** — onboarding flow now includes a plugin-install step for stores that don't have the Companion Plugin yet. One extra screen in the wizard.
- **Phase 4 + 5** — no change.

### v0.2 follow-up (post-May 22)

The device-pair token flow ships as its own slice: generate code in daemon, store pending code in plugin with 10-minute TTL, mint scoped token on operator approval in wp-admin, daemon polls for approval. This is the work PRD §10.4 originally scoped for v0.1.

---

## What Nevena needs to know

1. **No design-scope change.** The three Phase 1 surfaces (kanban, review/approval, onboarding) are unaffected. Diff shapes, card anatomy, and column layout are unchanged.
2. **Onboarding flow (Phase 3) adds one conditional step.** If the operator's store doesn't have the Companion Plugin installed, show a "Install the WooAgent Companion plugin" panel with the plugin ZIP download link + short install instructions. This is a screen we'd have had anyway once v0.2 device-pair shipped — we just need it earlier.
3. **No additional mockups needed for Apr 22 – Apr 28.** The board + card designs Nevena is already working on still land as planned.

---

## Install + verify checklist

- [x] Plugin uploaded to `woo-demo-store-99cc5c.mystagingwebsite.com` and activated (2026-04-21).
- [x] All 10 abilities register cleanly — confirmed via `wp_get_ability()` runtime check + `/wp-json/wp-abilities/v1/abilities` returns them.
- [x] MCP `tools/call mcp-adapter-execute-ability` → `wooagent-products/list` + `wooagent-products/get` return live product data end-to-end.
- [x] Stub abilities return error via MCP (as designed for v0.1).
- [ ] Clean up temporary `/wooagent-companion/v1/selftest` and `/source` debug endpoints before committing the feature branch — they're admin-gated but shouldn't ship in v0.1.0.
- [ ] Update PRD §16 roadmap to reflect the v0.1 Companion Plugin scope change.

## Debugging lessons captured (Apr 21 diagnostic session)

Three non-obvious WP Abilities API (core 6.9.0) rules that cost us five redeploys to uncover — worth documenting for the daemon's MCP client and future ability authors:

1. **Ability names must match `/^[a-z0-9-]+\/[a-z0-9-]+$/` — exactly one slash.** The core docblock on `wp_register_ability()` says "forward slashes" (plural); the actual validator in `WP_Abilities_Registry::register()` only accepts one. We had to flatten `wooagent/products/list` → `wooagent-products/list`.
2. **Two separate init hooks.** Categories register on `wp_abilities_api_categories_init`; abilities register on `wp_abilities_api_init`. Both must be declared with `wp_` prefix. Not just `abilities_api_init` (that hook does not exist).
3. **MCP Adapter has a second opt-in beyond the Abilities API.** Setting `show_in_rest => true` exposes an ability via REST, but invoking through the MCP Adapter's `mcp-adapter-execute-ability` tool also requires `meta.mcp.public => true`. Without it, MCP returns `"not exposed via MCP (mcp.public!=true)"`.

All three rules are enforced with `_doing_it_wrong()` notices that are suppressed outside `WP_DEBUG`. Debug with an `error_handler` wrapper or a `/selftest`-style endpoint that calls `wp_get_ability()` after registration — visible failures beat silent nulls.
