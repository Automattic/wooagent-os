# WooAgent + WC AI plugin integration strategy

**Date:** 2026-05-18
**Status:** Design draft — pending review
**Related:**
- [DSGWOO-1279](https://linear.app/a8c/issue/DSGWOO-1279) — Manifest hygiene: WC 10.9 canonical naming alignment (Phase 0 resolves Issues 2 + 3; Issue 1 pre-handled via placeholder entries)
- [DSGWOO-1322](https://linear.app/a8c/issue/DSGWOO-1322) — Pricing: subscription product support (still ships as baseline filter fix)
- [DSGWOO-1323](https://linear.app/a8c/issue/DSGWOO-1323) — Pricing: grouped products via child fan-out (still ships as baseline)
- [DSGWOO-1324](https://linear.app/a8c/issue/DSGWOO-1324) — Pricing: variable-product design + ship (partial shrink: Marketing covered by `batch-update-product-content`; Pricing variation pricing still real work — see Phase 0 findings below)
- [DSGWOO-1325](https://linear.app/a8c/issue/DSGWOO-1325) — Inventory: design product-type-aware semantics
- PRD §11.4 (Companion Plugin = baseline ability provider) and §11.7 (WPCOM-enhanced mode = optional additive tier)
- Manifest snapshot at `daemon/internal/manifest/default.json` (refreshed 2026-05-18 via `scripts/refresh-manifest.sh`)
**Branch:** not started

## Why

The WC AI plugin (`woocommerce-ai/v1`) shipped a major update that adds a complete **batch-operation pipeline** with a natively reversible primitive (`plan` → `confirm` with `undo`), a behavioral-filter target picker (`find-products`), and bulk content-update tools. Live ability count on the staging store jumped from 25 (Apr 23 snapshot) to 37, with 15 net-new abilities — 8 of them on `woocommerce/*`.

These new abilities don't form a coherent new persona on their own — they're a *toolbox* spanning multiple existing persona jobs plus a few capabilities that anchor what was always going to be the Reporting persona. The strategic question is how to:

1. **Keep the baseline promise.** PRD §4 non-goal 7 — no WPCOM/WC-AI hard dependency. Every Woo store works on day one.
2. **Reward stores that connect to WPCOM.** PRD §11.7 — stores with WC AI installed get a measurably more capable WooAgent. This is the Automattic-commercial-alignment lever; it has to be real, not aspirational.
3. **Not muddle the operator's mental model.** Personas are roles ("Marketing handles descriptions"). They shouldn't become plugins ("now you have a Woo AI agent in addition to a Marketing agent that also does descriptions").

This spec captures the three-layer model that resolves all three.

## Scope

**In scope**

- Strategy for distinguishing baseline vs. enhanced abilities at runtime.
- Persona-by-persona impact when WC AI is detected.
- Recommended shape for the new Reporting persona (a v0.2 persona unblocked by this update).
- Discovery mechanism — how the daemon learns which abilities are present without re-pairing.
- PEP / manifest hygiene implications (drift detection, refresh cadence).
- UX: how the operator finds out, opts in, and isn't surprised when behavior shifts.
- A sequenced implementation roadmap that defers no longer-needed work.

**Out of scope**

- Implementation specifics (a follow-up plan doc handles each numbered phase).
- The WPCOM-proxied 50-tool progressive registry (PRD §11.7 already documents this; it's adjacent to but not part of the locally-registered WC AI abilities this spec covers).
- Companion Plugin deprecation timing — Companion abilities remain the baseline and are NOT deprecated when WC AI is present (we use the WC AI ones additively; Companion stays for the no-WPCOM path).
- Multi-store scenarios (each connected store has its own ability set; in v1 there is only one store anyway).
- Whether WooAgent itself should *recommend* installing WC AI (the integration shows up only after the operator has installed it on their store; we don't promote it from inside WooAgent in this design).

## The three-layer model

Each layer is independent — operators on the baseline path get layer 1 only, operators with WC AI installed get all three.

| Layer | What it is | Operator-visible? | Requires |
|---|---|---|---|
| **1. Baseline** | Companion Plugin abilities. All current personas. Today's behavior. | Default; no action. | Just the Companion Plugin (installed at pair time, no WPCOM required). |
| **2. Enhanced existing personas** | Existing personas auto-adopt better primitives at run time when they're available. | Mostly invisible. One-time notice surfaces the upgrade. | WC AI plugin installed + WPCOM connection. |
| **3. New agent: Reporting** | A net-new persona that requires WC AI to function. Surfaces in "Add agent" with an affinity badge. | Explicit opt-in. | WC AI plugin installed + WPCOM connection. |

### Layer 1 — Baseline (unchanged)

What ships today. `wooagent-products/list`, `wooagent-products/get`, `wooagent-products/update`, `wooagent-orders/{list,get,add-note}`, `wooagent-customers/get`. Marketing, Pricing, Sales Support all work against any Woo store, zero extra dependencies.

The Companion Plugin abilities are not deprecated when WC AI is present. They remain the baseline path. WC AI abilities are *additive* — when present, certain code paths prefer them; when absent, the existing Companion-backed paths run.

### Layer 2 — Enhanced existing personas

When the daemon detects WC AI abilities on the connected store, existing personas branch to the better primitives at run time.

| Persona | Current behavior | Enhanced behavior (WC AI present) |
|---|---|---|
| **Pricing** (bulk batches) | N independent `wooagent-products/update` writes via Companion. Our `batches` table wraps N child issues. No native undo — dismiss after approve requires operator compensating action. | `woocommerce/plan-batch-operation` → operator review (renders WC AI's preview) → `woocommerce/confirm-batch-operation`. Dismiss-after-approve dispatches `woocommerce/undo-batch-operation` (drift-aware: items modified since the original run are skipped by default). |
| **Pricing** (target picker) | `pickFirstProduct` lists 100 and skips cooldown — picks first eligible by position. | `woocommerce/find-products` with behavioral filters (`slow_mover`, `margin`, `data_issues`) — model targets the *interesting* products. The cooldown set still applies (subtract from `find-products` results). |
| **Marketing** (single description rewrite) | `wooagent-products/get` → LLM rewrite → `wooagent-products/update`. | No change. Single-product descriptions are already the right shape. |
| **Marketing** (batch descriptions, future) | Not built. | `woocommerce/batch-update-product-content` against a selector (e.g., "all uncategorized products" or "products in the Tshirts category"). Same plan/confirm/undo pipeline as Pricing. |
| **Marketing** (target picker for bulk runs) | n/a (single-product today). | `woocommerce/find-products` with `data_issues` filter — surfaces products missing descriptions, missing categories, etc. |
| **Sales Support** | Order-centric. Uses `wooagent-orders/*`. | No change. WC AI's `woocommerce/get-order` covers the same ground as Companion's; no reason to swap. |

### Layer 3 — New agent: Reporting

The WC AI abilities that don't fit any existing persona job cluster cleanly into what was always going to be the Reporting persona (currently deferred per `progress.md` "Explicitly deferred").

**Reporting persona seed abilities** (all from WC AI):
- `woocommerce/analyze-shipping-needs` — shipping zone / method audit.
- `woocommerce/manage-shipping-zones` — propose shipping configuration changes.
- `woocommerce/find-products` with behavioral filters — "products that need attention" digests (slow movers, margin outliers, data-quality gaps).
- `woocommerce/get-batch-operation-status` + historical querying — "what ran last week and how did it go" summaries.
- `woocommerce/create-product-category` + `woocommerce/create-product-tag` — taxonomy housekeeping proposals (when Reporting detects uncategorized clusters).

**Why this is "Reporting" and not "Woo AI agent":**

- Job-to-be-done framing. The operator's mental model is "the Reporting agent tells me what to look at this week." That maps to `find-products` + behavioral filters + summarization.
- Persona name is durable. If we name it after the plugin and the plugin name changes (or another vendor ships a similar surface), our persona breaks. Naming by job is invariant.
- It leaves room for future Reporting expansion that doesn't depend on WC AI (e.g., a baseline Reporting tier later that uses `wooagent-orders/list` for revenue summaries).

**Why it requires WC AI in v1:**

- Without `find-products` or `analyze-shipping-needs`, Reporting can't actually do useful work — only the trivial "count orders" path, which no operator wants weekly. Better to gate the agent on the precondition that lets it earn its keep.
- The gate is opt-in via the "Add agent" flow (Section "UX" below) — operators without WC AI never see Reporting suggested in a way that creates expectations the daemon can't meet.

**Persona-color and avatar:** Reporting already has tokens reserved in CLAUDE.md (`--wa-persona-rp-bg` / `--wa-persona-rp-ink`, violet) and a placeholder avatar slot. No new design tokens needed.

## Discovery mechanism

The daemon needs to know which enhanced abilities are present *without re-pairing* the store. Today's `stores.last_discovered_at` only runs at pair time.

**Changes:**

1. **Re-discover on a cadence.** Daily scheduled run via the existing `cmd/manifest-compute` codepath (now an internal package, not just a CLI tool). Updates `stores.last_discovered_at` + writes ability rows to a new `store_abilities` table.

2. **New `store_abilities` table.**
   ```sql
   CREATE TABLE store_abilities (
       store_id     TEXT NOT NULL,
       ability      TEXT NOT NULL,   -- e.g. "woocommerce/find-products"
       schema_hash  TEXT NOT NULL,   -- sha256 from manifest-compute
       discovered_at TEXT NOT NULL,
       PRIMARY KEY (store_id, ability),
       FOREIGN KEY (store_id) REFERENCES stores(id)
   );
   ```
   This is the runtime source of truth for "is this ability available right now." The pre-signed manifest (`daemon/internal/manifest/default.json`) is the *trust* source — `store_abilities` is the *presence* source. PEP joins them at invocation time.

3. **Persona-level capability check.**
   ```go
   if deps.Abilities.Has("woocommerce/find-products") {
       // enhanced target picker
   } else {
       // existing pickFirstProduct path
   }
   ```
   `Abilities` is a new field on `personas.Deps`, populated from `store_abilities` at run-claim time. Capability checks live in the persona, not in the loop — keeps the loop oblivious to ability tiers.

4. **Drift handling.** The existing manifest schema-hash drift logic (per PRD §8.4.2: a hash mismatch auto-demotes a pre-signed ability to `unapproved`) extends naturally: if `find-products`' schema changes after a plugin update, the matching enhanced path is disabled for that store until the manifest catches up. Personas fall back to baseline transparently.

## UX

### Operator-surprise mitigation (layer 2)

The layer-2 upgrade is invisible by default — that's the point. But Pricing batches behaving differently one day (using WC AI's preview UI + native undo) without explanation is the kind of surprise that erodes trust.

**Recommendation: a one-time per-upgrade notice in the Agents page.** When the daemon first detects a WC AI ability that activates a layer-2 upgrade, surface a single dismissable notice on the affected persona's row:

> *"Pricing now uses Woo AI's batch pipeline — operations include a preview before confirmation and a one-click undo. [Learn more]"*

Notice state stored in a new `agents.upgrades_seen` JSON column (set of upgrade IDs). After dismissal it never reappears for that upgrade. New upgrades each get their own one-time notice.

### "Add agent" flow (layer 3)

The Agents page today lists the 7 personas in fixed order. The "Add agent" affordance is new UX work.

**Shape:**

- Sidebar Agents page gains a "+ Add agent" button at the bottom of the list.
- Clicking it opens a modal listing **available personas** — the deferred ones (Inventory, Accounting, Reporting, Chief of Staff) plus any future plugins-as-personas if we go that route later.
- Each card surfaces:
  - Persona icon + name.
  - One-sentence job description.
  - **Requirements line** — "Companion Plugin only" (always met) or "Requires: Woo AI plugin connected" (gated).
  - **Affinity badge** when the precondition is met and the persona is recommended:
    - "Ready to add — Woo AI detected" (positive)
    - "Install Woo AI to enable" (with a link to the plugin install instructions; we do NOT install it for them)
- Personas without their build complete yet show "Coming v0.2" and a disabled button.

This shape works for any future "plugin enables a persona" pattern, not just WC AI. The integration point is generic — the persona registry declares its preconditions, the UI renders accordingly.

**Why not "Add the Woo AI agent" as the user initially proposed:**

The 18 `woocommerce/*` abilities span four distinct jobs (targeting, CRUD, bulk pipeline, reporting). A single "Woo AI" agent that does all of them either:
- duplicates capabilities of existing personas (Marketing already does descriptions; what would "Woo AI doing descriptions" be?), or
- becomes an incoherent grab-bag persona ("Woo AI does… a bit of everything").

Splitting into "enhance existing" (layer 2, no new agent) + "unlock a job-shaped new agent" (layer 3, Reporting) keeps each ability with the persona whose job it serves.

## PEP / trust / manifest implications

**Pre-signed manifest expansion.** The 15 net-new abilities from the WC AI plugin (8 of them `woocommerce/*`, 5 `jetpack-forms/*`, 2 `akismet/*`) need entries in `daemon/internal/manifest/default.json` before any layer-2 or layer-3 code can use them. Each entry needs:
- Canonical schema hash (from `cmd/manifest-compute`).
- Scope classification (`read` / `propose` / `apply`).
- Reversibility score — **`woocommerce/plan-batch-operation` and `woocommerce/batch-update-product-content` get reversibility = 1.0 because of the `undo-batch-operation` primitive.** This is the first time a non-read `propose` action gets a reversibility of 1.0; PEP's scoring should already handle it (no code change), but flag for verification.
- Persona mappings (which personas can invoke).

**Manifest refresh cadence.** Today's manifest snapshot is 3 weeks old and missed all 15 of these. A monthly `manifest-compute` regen against the staging store, committed as a chore PR, prevents future drift. Either:
- Add a Makefile target / npm script + a CI nag if the snapshot is >30 days old, or
- A GitHub Action that runs `manifest-compute` + opens a PR on schema-hash diffs.

**Trust model unchanged.** Operators still approve abilities via the existing PEP framework. WC AI abilities are pre-signed in the manifest just like Companion's; their availability is per-store (gated by presence), not per-operator.

## Open questions / decisions to make

1. **Does layer 2 ship before layer 3?** Strongly recommend yes — layer 2 is a free upgrade for operators who already have WC AI; layer 3 (the Reporting persona) is net-new product work and benefits from layer 2's pipeline already being proven in Pricing.

2. **Does WC AI's `update-product` displace Companion's `wooagent-products/update`?** Open. Two options:
   - **(a) Yes, when present** — fewer code paths, but introduces a per-store behavior split for *every* Pricing/Marketing approve action, not just batches. Higher operator-surprise surface.
   - **(b) No — Companion stays for single-product writes; WC AI only used for batches and target picking** — keeps single-product writes uniform across stores. Smaller blast radius if WC AI's `update-product` ever regresses.
   - Recommend **(b)** for v0.2; revisit after the batch pipeline migration is proven.

3. **Reporting persona's first proposal types.** Sketch: `shipping_zones_audit` (output of `analyze-shipping-needs`), `product_health_digest` (output of `find-products` with `data_issues`), `weekly_taxonomy_review`. Needs a brainstorm; this spec doesn't lock it down.

4. **Do we surface a "Woo AI plugin recommended" message anywhere in the baseline UI?** Today: no — only the "Add agent" modal mentions it, and only after the operator self-navigates there. Could add a one-line nudge in onboarding, but that risks the same "WooAgent is upselling Automattic products" perception we've been careful to avoid. **Recommend: don't promote from within WooAgent. Operators discover WC AI via their normal WP-admin browsing.**

5. **What happens to DSGWOO-1322 / 1323 / 1324?**
   - **DSGWOO-1322 (subscription support)** — still relevant for baseline (Companion path). Trivial one-line fix; ship as-is.
   - **DSGWOO-1323 (grouped via child fan-out)** — still relevant for baseline. Small change.
   - **DSGWOO-1324 (variable-product design)** — *scope may shrink dramatically.* When WC AI is present, `woocommerce/find-products` likely surfaces variations directly (verify schema), and `batch-update-product-content` writes to a variation selector. The Companion-side build of `variations-list` + custom percent-change proposal may not be needed in the WC AI path. Need to verify against live schemas before re-scoping the issue.

## Implementation sequencing

A suggested order. Each phase produces something operators can use.

**Phase 0 — Manifest refresh + verification (1-2 days). ✅ DONE 2026-05-18.**
- ✅ Re-run `cmd/manifest-compute` against staging, commit refreshed `daemon/internal/manifest/default.json`. (Landed in PR #42 — 25 → 36 entries, refreshed schema hashes.)
- ✅ Verify `woocommerce/find-products` and `woocommerce/get-product` schemas for variation coverage. **Finding: scope of DSGWOO-1324 partially shrinks for Marketing (`batch-update-product-content` covers variable-product descriptions at the parent), does NOT shrink for Pricing (no `update-variation` or per-variation pricing ability exists in the WC AI plugin as of 2026-05-18).** See "Findings" subsection below.
- ✅ Document monthly manifest-refresh cadence — `scripts/refresh-manifest.sh` (surgical merge: preserves pre-signed placeholder entries) + section in CLAUDE.md. GH Action deferred — too much YAML for too little value at v0.1.
- ⚠️ "No persona behavior changes yet" **slipped.** Baseline-tier picker improvements landed in the same PR #42 (Pricing → `orderby=total_sales asc`, Marketing → `orderby=date_modified asc` + `data_issues` post-filter). Behavior change uses the new Companion Plugin v0.2 `orderby` arg, not WC AI's `find-products`, so it's not Phase 2. But it IS persona behavior that Phase 0 said we'd defer. Noted for the next phase planning.
- ✅ Cross-link to [DSGWOO-1279](https://linear.app/a8c/issue/DSGWOO-1279) added. Phase 0 also resolved Issues 2 + 3 from that ticket and pre-added Issue 1's WC 10.9 canonical names with placeholder schema hashes (PEP whitelists them now; drift detection fires when real schemas arrive). Guard test `TestPreSignedWC109CanonicalEntriesPresent` prevents accidental clobber on future refreshes.

### Findings from Phase 0 schema verification

- **`woocommerce/find-products`** input filter accepts `product_type` (array, can target `variable`); output `counts` includes `variations` and `total_variations` (server-side tracks them). Preview only returns `{product_id, name}` — no variation IDs in the preview.
- **`woocommerce/get-product`** output is parent-only. No `variations` array, no per-variation pricing, no variation IDs. Does NOT cover variations.
- **`woocommerce/update-product`** input takes single `product_id` + parent property fields. No `variation_id`, no per-variation update. Does NOT cover variations.
- **`woocommerce/batch-update-product-content`** `field` enum restricted to `description` / `short_description`. Writes to parent. **Covers Marketing's variable-product needs at the parent level**; does NOT cover Pricing.
- **No `update-variation`, `update-variation-price`, or `variations-list` ability exists in the WC AI plugin's current ability surface.**

**Consequence for [DSGWOO-1324](https://linear.app/a8c/issue/DSGWOO-1324):** scope does NOT shrink as the original spec hoped. Pricing variable-product support is still real work — either build a Companion Plugin variation extension (`wooagent-products/variations-list` + `wooagent-products/update-variation`) or wait for WC AI to add variation support upstream.

**Phase 1 — Discovery infrastructure (3-5 days est, ~0.5 day actual). ✅ DONE 2026-05-18.**
- ✅ Per-store ability cache table — **pre-existed.** Migration `008_abilities.sql` (May 7-ish, alongside the trust-labeling work) already created the `abilities` table keyed by `(store_id, name)` with `schema_hash`, `trust_state` (new / trusted / schema_changed), `revoked_at`, CASCADE on store delete. The spec's "store_abilities" name was a misnomer for the existing table.
- ✅ Discovery service — **pre-existed.** `daemon/internal/abilities/abilities.Runner` already runs `DiscoverAbilities` + `GetAbilityInfo` per store via MCP, upserts rows, manages trust transitions.
- ✅ Periodic re-discovery — **pre-existed.** `Runner.SchedulePeriodic` runs every 6h (more aggressive than the 24h the spec sketched, which is fine). Invoked from `cli/run.go:156-157` at daemon startup.
- ✅ `personas.Deps.Abilities.Has(name)` API — **new in this phase.** Added the `Abilities` interface + `NilAbilities` sentinel in `daemon/internal/personas/personas.go`; concrete `abilities.Checker` implementation that mirrors PEP's `checkTrustState` exactly (revoked → false; manifest pre-sign → true; trust_state=trusted → true; everything else → false). Wired at daemon startup so every persona's `Deps` carries a checker against the live abilities table + bundled manifest.
- ✅ Tests for the new API — 9 cases covering the cross-product of (abilities-row state × manifest-presence × revoke).
- ✅ No persona behavior changes — personas don't yet call `Has`. Phase 2 is where Pricing first branches on it.

**Takeaway:** the trust-labeling work shipped earlier in May had already built ~80% of "Phase 1" before the integration spec was written. The remaining 20% was the persona-facing API surface. Phase 2 is unblocked.

### Sequencing change — 2026-05-18, driven by Friday launch posture

The original sequencing (Phase 2 → 3 → 4 → 5 → 6) was written assuming the WC AI plugin would be available to operators *before* WooAgent launched. As of 2026-05-18 that's not the case: the plugin hasn't shipped publicly yet, so layer-2 enhancements that quietly improve existing personas when WC AI is present deliver **zero operator-visible value** for the Friday May 22 launch — and they add blast radius to personas operators will be exercising heavily during the launch window.

**Launch-week order (replaces Phases 2–6 below for the immediate window):**

1. ✅ **Phase 0** — manifest refresh + verification (done).
2. ✅ **Phase 1** — discovery infrastructure + `Deps.Abilities.Has` API (done).
3. ✅ **Phase 5a — Reporting persona scaffold (LAUNCH).** One proposal type (`product_health_digest`). Initially shipped (PR #46) gated on the WC AI plugin's `woocommerce/find-products` — that was a launch-speed shortcut, not the right architectural answer. **Rewritten 2026-05-18 (`feat/reporting-baseline-no-wc-ai-dep`)** to use the Companion Plugin's `wooagent-products/list` directly with Go-side data-issue detection (missing long description, missing short description — both signals already in the list summary). Reporting is now a **baseline persona** (PRD §11.4): works on any Woo store with no optional-plugin dependency, no LLM, no writes. Persona registers via `init()` but is **NOT** seeded into the agents table — stays dormant until the operator opts in via the next phase.

   *Why the rewrite:* the `data_issues` check is field-presence on data the Companion Plugin already returns. Gating Reporting on WC AI created an operator-confusion risk ("I added Reporting but it's grayed out until I install a different plugin") and put a launch-week persona awkwardly into §11.7 territory when it belonged in §11.4. The `Deps.Abilities.Has` API from Phase 1 stays in place for genuinely-plugin-gated personas in the future (a Shipping persona that needs `analyze-shipping-needs`, etc.) and for Phases 2/3 below — Reporting just doesn't use it.

4. ⏳ **Phase 7 — "Add Agent" UI (LAUNCH).** Promoted from a Phase 5 sub-bullet to its own deliverable because it's the operator-visible artifact of the "operators curate their fleet" promise. Modal off the Agents page; lists Inventory / Accounting / Reporting / Chief as addable; one-click insert of the agents row with `enabled=1` + operator-chosen cadence. Reporting appears universally available (no plugin gate). The "requires plugin" treatment stays in the UI framework for future plugin-gated personas (Shipping audit, etc.).

**Deferred to post-launch (still real work, just not blocking 2026-05-22):**

5. **Phase 2 — Pricing target-picker upgrade.** `find-products` with `slow_mover` filter behind `Abilities.Has`. 2-3 days. *Why deferred:* zero value until operators have WC AI; adds blast radius to the persona that's done most of the launch-week dogfooding.
6. **Phase 3 — Pricing batch pipeline migration.** `plan/confirm/undo-batch-operation` replacing the in-house batch path. 1-2 weeks. **This is now the strategic core of the WC AI integration** — drift-aware undo is the one piece genuinely hard to replicate in our Companion Plugin (the integration spec at large is more about this than about Reporting now that Reporting is baseline).
7. **Phase 4 — DSGWOO-1322 / 1323 / 1324 execution.** Baseline filter fixes (1322 + 1323) + variable-product split (1324a closed-as-covered for Marketing, 1324b Pricing per-variation pricing). *Why deferred:* 1322/1323 are small and could fit in if there's slack, but neither is launch-blocking; 1324b needs its own design pass anyway.
8. **Phase 5b — Reporting persona expansion.** Additional proposal types (`slow_movers_alert` using `total_sales` data already in the list summary; `missing_categories` once `categories` is added to the list summary; `weekly_summary` LLM-narrated digest). 2-3 weeks. *Why deferred:* baseline scaffold proves the persona end-to-end at launch; expansion is incremental, not blocking. Note that most expansion stays on the baseline Companion path — `analyze-shipping-needs` (WC AI) would unlock a *Shipping* persona, not a Reporting expansion.
9. **Phase 6 — Marketing batch updates.** `batch-update-product-content` for "fix all uncategorized products" / per-category description rewrites. 1 week. *Why deferred:* zero value until operators have WC AI; reuses Phase 3's pipeline infrastructure.

### Narrowed integration scope (2026-05-18 revision)

Stripped of the "Reporting needs it" justification, the WC AI integration narrows to a tight set of post-launch wins:

- **Phase 3 (batch pipeline) is the strategic core.** Native drift-aware undo for Pricing + Marketing batches. Genuinely hard to replicate. ~1-2 weeks of integration vs. 2-3 weeks of building our own.
- **Phase 2 (find-products target picker) is a tactical layer-2 win.** Smaller, replicable, but free if we adopt it.
- **`analyze-shipping-needs` + `manage-shipping-zones`** would seed a future Shipping persona, not feed into Reporting.

Everything else — baseline data-issue detection, per-persona pickers, taxonomy proposals — can ship on the Companion Plugin without an optional dependency. The "your fleet grows when you install plugins" positioning still holds (operators can add personas like Reporting via the Add Agent UI; future plugin-gated personas like Shipping audit demonstrate the gate semantics), but Reporting itself is no longer the proof-point.

### Original Phases 2-6 (kept for post-launch reference)

**Phase 2 — Pricing target-picker upgrade (2-3 days).**
- Pricing's `pickFirstProduct` branches on `Abilities.Has("woocommerce/find-products")`.
- Behavioral filter selection — start with `slow_mover` for the test, expand once the pattern is proven.
- Cooldown still applies (subtract from `find-products` results).
- Operator-visible — flag with a one-time notice.
- **First validation that the three-layer model holds end-to-end.**

**Phase 3 — Pricing batch pipeline migration (1-2 weeks).**
- Replace per-bucket N-writes with `plan-batch-operation` → `confirm-batch-operation`.
- Approve flow renders WC AI's preview (new UI work — the existing batch preview UI is replaced).
- Dismiss-after-approve dispatches `undo-batch-operation`.
- `batches` table gains an `operation_uuid` column; existing `BatchSiblings` plumbing stays for the baseline path.
- Operator notice: "Pricing now uses Woo AI batches…"

**Phase 4 — DSGWOO-1322 / 1323 / 1324 re-scoping and execution.**
- 1322 + 1323 ship as previously scoped (baseline filter fixes).
- 1324 partial re-scope based on Phase 0 schema verification: Marketing variable-product needs are covered by `batch-update-product-content` (writes parent description); Pricing variable-product needs are NOT covered and require either a Companion Plugin extension or upstream WC AI work. 1324 splits into (a) closed-as-covered for Marketing, (b) re-titled "Pricing: per-variation pricing support" for the remaining real work.

**Phase 5b — Reporting persona full design (post-launch; own design pass; 2-4 weeks).**
- Separate spec at `docs/specs/YYYY-MM-DD-reporting-persona-design.md`.
- First proposal types beyond `product_health_digest`: `shipping_zones_audit`, `weekly_taxonomy_review`, `bestseller_summary`.
- LLM-narrated digest copy (the scaffold renders raw lists).

**Phase 6 — Marketing batch updates (1 week).**
- `batch-update-product-content` for "fix all uncategorized products" / "rewrite all Tshirts category descriptions" workflows.
- Reuses Phase 3's pipeline infrastructure.

## Linear issues / follow-ups to file

After this spec is reviewed:

1. **DSGWOO-NNNN — Adopt WC AI plugin's batch-operation pipeline (architectural).** High. Links to this spec. Tracks Phases 1–3 above.
2. **DSGWOO-NNNN — Manifest refresh chore + monthly cadence.** Medium. Phase 0.
3. **DSGWOO-NNNN — Pricing: adopt `find-products` as target picker.** High, quick win. Phase 2.
4. **DSGWOO-NNNN — Reporting persona: design spec.** High, design-only ticket that produces the follow-up spec.
5. **DSGWOO-1324 (existing) — re-scope after schema verification in Phase 0.**

## Decisions needed before implementation

- [ ] Sequencing confirmed (Phases 0 → 2 → 3 → 5 → 6, with 1322/1323 in parallel).
- [ ] Decision on layer-2 question 2 above (Companion vs. WC AI for single-product writes).
- [ ] Reporting persona's first proposal type set sketched in a brainstorm.
- [ ] UX sign-off on the "Add agent" modal shape (Nevena).
- [ ] Operator-notice copy reviewed (avoid "automattic-marketing" tone).
