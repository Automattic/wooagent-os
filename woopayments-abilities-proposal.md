# WooPayments Abilities — Proposal from WooAgent OS

**From:** Elizabeth / WooAgent OS
**To:** WooPayments developer working on agent abilities
**Purpose:** Map the WooPayments ability surface WooAgent OS would like to see registered via `wp_register_ability()`, so an autonomous agent fleet (Accounting, Sales Support, Reporting, Pricing, Chief of Staff) can operate against WooPayments-backed stores.

## Context

WooAgent OS is a local-first agent runtime that connects to WooCommerce stores via MCP. It runs a fleet of role-specific personas — Marketing, Pricing, Inventory, Accounting, Reporting, Sales Support, Chief of Staff — each with domain-scoped ability bindings. Every ability invocation routes through a deterministic Policy Enforcement Point that verifies trust state, schema match, scope, and reversibility before the call reaches the store.

Practical implication for ability design: **the agent reasons about which ability to call, not how to call it**. Abilities should be semantically clean, schema-rich, idempotent where possible, and return enough context that the agent doesn't need three follow-up calls to understand the result.

## Conventions

- **Namespace:** `woopayments/*`. Matches the `plugin-slug/*` pattern WooCommerce AI and Yoast already use.
- **Action naming:** plural-resource-verb — `payouts-list`, `disputes-get`, `refunds-create`. Mirrors the progressive MCP registry WooCommerce AI ships.
- **Idempotency:** write operations should accept a client-supplied `idempotency_key` and treat duplicate keys as a no-op returning the original result. Prevents duplicate refunds / retries when the agent retries a step.
- **Pagination:** read-list operations support `per_page` (max 100) and either `page` or `cursor`. Cursor is preferred for stable iteration.
- **Date filtering:** all analytical list operations support `after` and `before` (ISO 8601 or Unix timestamp). Most accounting workflows are date-bounded.
- **Amounts:** always return as integer minor units (e.g., cents) + a separate `currency` field. Avoid float amounts.
- **Structured errors:** distinguish `validation_error`, `auth_error`, `rate_limited`, `upstream_error` (Stripe/acquirer), and `not_found`. The PEP and the agent loop both branch on error class.
- **Reversibility hint:** include a `reversibility` annotation in the ability's metadata (0.0–1.0) — feeds WooAgent OS's approval-gate logic (see §10.5 of our PRD). Read ops are 1.0; a refund might be 0.2; accepting a dispute is 0.0.

## Tier 1 — Must have for v1

These map to the highest-value agent workflows: payout reconciliation, dispute response, customer support refunds, and revenue reporting.

### Payouts & reconciliation (Accounting persona)

| Ability | Kind | Rev. | Notes |
| --- | --- | :---: | --- |
| `woopayments/payouts-list` | read | 1.0 | Filters: date range, status (paid/pending/failed), destination account. Returns payout id, arrival date, amount, currency, status, transaction count. Essential for matching bank deposits to WC revenue. |
| `woopayments/payouts-get` | read | 1.0 | Single payout. Include a summary object: `gross_volume`, `refunds`, `disputes`, `fees`, `net_deposit` — saves the agent an arithmetic round-trip. |
| `woopayments/payouts-transactions-list` | read | 1.0 | Transactions inside a payout, paginated. Each row: transaction id, order id, type (charge/refund/adjustment), gross, fee, net. This is the ability Accounting uses to reconcile a single bank deposit line-by-line. |
| `woopayments/balance-get` | read | 1.0 | Current available + pending + on-hold balances, per currency. |

### Disputes & chargebacks (Sales Support + Accounting)

| Ability | Kind | Rev. | Notes |
| --- | --- | :---: | --- |
| `woopayments/disputes-list` | read | 1.0 | Filters: status (`warning_needs_response`, `warning_under_review`, `warning_closed`, `needs_response`, `under_review`, `charge_refunded`, `won`, `lost`), date range, amount range. |
| `woopayments/disputes-get` | read | 1.0 | Full dispute: reason, evidence_due_by, amount, currency, linked order, linked charge, current evidence state, stripe dispute id. Include the merchant-facing reason text and the network-specific reason code separately — they're different semantic fields. |
| `woopayments/disputes-submit-evidence` | write | 0.2 | Submit evidence fields (customer communication, receipt, shipping docs, etc.). Idempotency-keyed. Should accept a `draft: true` flag that stages evidence without submitting — lets the agent prepare, operator reviews, then a second call with `draft: false` submits. |
| `woopayments/disputes-accept` | write | 0.0 | Accept the dispute (relinquish funds). **Never** auto-invocable; our PEP will require per-issue operator approval regardless of persona config. |

### Transactions (Reporting, Sales Support)

| Ability | Kind | Rev. | Notes |
| --- | --- | :---: | --- |
| `woopayments/transactions-list` | read | 1.0 | Filters: date range, status, customer id, order id, amount range, payment method type. |
| `woopayments/transactions-get` | read | 1.0 | Include: raw Stripe metadata, risk assessment, 3DS outcome, payment method details, linked dispute (if any), linked refunds. |
| `woopayments/transactions-search` | read | 1.0 | Free-text search over customer email, card last 4, order number. Common Sales Support path. |

### Refunds (Sales Support)

Note overlap with `woocommerce/order-refunds-*`. Recommend WooPayments abilities only where they offer what Woo-native refunds don't — instant refunds, partial refunds to different methods, refund fee handling.

| Ability | Kind | Rev. | Notes |
| --- | --- | :---: | --- |
| `woopayments/refunds-create` | write | 0.2 | Full or partial. Inputs: charge id, amount (minor units), reason (structured enum + optional freetext), idempotency_key. Return: refund id, arrival estimate, fee refunded. |
| `woopayments/refunds-list` | read | 1.0 | Filters: date range, charge id, status. |
| `woopayments/refunds-get` | read | 1.0 | Single refund with full trail. |

### Fees (Accounting, Pricing)

| Ability | Kind | Rev. | Notes |
| --- | --- | :---: | --- |
| `woopayments/fees-breakdown` | read | 1.0 | Per-transaction fee components: processing fee, platform fee, currency conversion fee, dispute fee (when applicable). Pricing uses this to compute true margin. |
| `woopayments/processing-fee-summary` | read | 1.0 | Aggregated over a date range. Returns total fees, effective rate, fee-vs-gross ratio, breakdown by payment method type. The number every operator wants on a monthly report. |

## Tier 2 — Strong nice-to-have

### Customers & saved payment methods (Sales Support)

- `woopayments/customer-get` — WooPayments customer record linked to WC customer. Include lifetime gross, refund rate, dispute rate — quick risk signal for the agent handling a ticket.
- `woopayments/customer-payment-methods-list` — saved cards/bank accounts, last 4, brand, expiry. Used when a subscription payment fails and the agent wants to suggest alternatives.

### Subscription-adjacent (when WC Subscriptions is active)

- `woopayments/subscriptions-failed-payments-list` — recent failures that need dunning. Filter: `retry_exhausted: true` to isolate unsalvageable. Critical for MRR retention workflows.
- `woopayments/subscriptions-retry-payment` — trigger an explicit retry. Write, reversibility 0.5 (retry can't hurt but may annoy the customer with a second 3DS prompt).

### Risk (Chief of Staff)

- `woopayments/risk-flagged-list` — transactions currently flagged. Include the risk score and the specific rules that fired. Chief of Staff surfaces these as daily insights.
- `woopayments/risk-rules-list` — current fraud rules (read). Merchants often forget what rules they have.

## Tier 3 — Future

Useful but not blocking v1. Flagging so they're considered in schema design even if not shipped first.

- `woopayments/risk-rules-update` — write, reversibility 0.5. Scary (can block legitimate orders); requires explicit operator approval per-rule.
- `woopayments/payout-schedule-update` — write, reversibility 0.7. Changing payout cadence has downstream cash-flow implications.
- `woopayments/currencies-enabled-list` + `/exchange-rates-get` — only relevant for international merchants; deferred until v1.1.
- `woopayments/woopay-enabled-get`, `woopayments/checkout-methods-list` — Marketing persona uses these to understand checkout UX. Read-only. Low priority.
- `woopayments/capital-offer-get` — if WooPayments Capital surfaces offers via API, a read ability lets the agent flag them. Non-urgent.

## Cross-cutting asks

1. **Schema hashes in ability metadata.** WooAgent OS computes a schema hash over each ability's `input_schema` + `output_schema` for drift detection (see §8.4.1 of our PRD). If the plugin publishes a stable hash in the ability's metadata, we can verify against it. Not a hard requirement — we can compute our own — but nice to have.
2. **Ability versions.** Include a `version` field in each ability registration that bumps when the schema changes. Lets our pre-signed manifest pin to known-good versions and gracefully demote when a plugin update changes the contract.
3. **Test fixtures.** A `woopayments/_selftest` read-only ability that returns known-shaped sample data would make it trivial to validate wire-up end-to-end without touching real transactions. Similar to what our Companion Plugin does with `/selftest`.
4. **Rate-limit hints.** Some of these (especially `transactions-list`, `disputes-list`) can be large. If the ability can return a `rate_limit_remaining` in its output envelope, our orchestrator can back off gracefully rather than discovering limits through 429s.

## Not asking for (explicit non-asks)

- Abilities that wrap many operations in one call (e.g., "reconcile entire month"). The agent composes these from primitives; a bundled ability is harder to reason about and harder to audit.
- Abilities that accept freeform LLM-generated queries (e.g., "find transactions matching this description"). Text matching should be a local skill in WooAgent OS, not a remote ability, because ambiguity is best resolved client-side.
- Abilities that trigger email or customer-visible communication as a side effect. Those belong to WooCommerce's communication surface, not the payments provider.
