# Marketing cold-draft batches: fill empty product copy

**Linear:** DSGWOO-XXXX (to be filed in the WooAgent OS project, assigned to Elizabeth)
**Status:** Design — ready for review
**Author:** Elizabeth Pizzuti
**Date:** 2026-05-18

## Problem

The Reporting persona currently emits a proposal it calls "Product health digest — N products need attention" (`daemon/internal/personas/reporting/reporting.go:73-128`). It scans the catalog for products with empty `short_description` and/or `long_description` and renders a markdown bullet list naming each one. The proposal has no variants. The operator's only options are "review each in WP-admin and add the missing fields, or dismiss this digest."

That output is the wrong shape on two axes:

1. **Persona.** The artifact under review is product *copy*, which is squarely Marketing's domain. Marketing already biases toward exactly these products via the `data_issues` priority signal (`marketing/marketing.go:359-381`). Reporting is currently shadow-running Marketing's scan and emitting a useless intermediate result.
2. **Value.** The agent is supposed to do the work, not hand the operator a checklist of work to do elsewhere. A digest that says "go fix these in WP-admin" violates the WooAgent product premise.

The Figma at [node 797-22195](https://www.figma.com/design/0kiSHVNytQ7QjfglyxvBxF/WooAgent?node-id=797-22195) shows the right shape: a Marketing batch review where the agent has drafted variants for each missing-copy product and the operator's job is reduced to approval. The UI for this — `BatchReview.tsx` — already exists and is currently used by the Pricing batch path.

## Goals

- When Marketing's scheduler tick finds ≥3 published products with empty description fields out of cooldown, Marketing emits a **batch proposal** of drafted descriptions instead of a single rewrite.
- Each child in the batch has 3 variant drafts; the operator selects one per product and approves; the approved variant writes only to the previously-empty fields via the existing `wooagent-products/update` ability.
- The Reporting `product_health_digest` Draft path is retired. The persona stays registered (returns `Skipped:true`) so the seven-persona sidebar model is preserved.
- The new flow renders through the existing `BatchReview` component with a small variant-shape extension; no new screen, no new route.

## Non-goals

- A separate Marketing-Cold-Draft persona. The decision to batch vs. single-rewrite lives entirely inside Marketing's `Draft()`.
- Plumbing an `intent` field through the scheduler. The trigger is implicit — Marketing decides per tick based on candidate availability.
- Batch-aggregate scoring for the KPI header tiles. Per-row variant scoring already works (DSGWOO-1326); aggregate computation lands later and is out of scope here. The KPI header tiles keep their existing placeholder values for this work.
- Operator UI to configure thresholds. `coldDraftMin=3` and `coldDraftMax=10` are constants; tuning happens via code change after observing real-world batch sizes.
- Filing the Reporting persona's next real skill. After retirement the persona's `Draft()` returns `Skipped:true` until separate work lands.

## Architecture (smart Draft, additive)

```
Marketing run (existing)                       Marketing run (new)
──────────────────────────                     ──────────────────────────

  Draft() called                                 Draft() called

  ┌─ scan: rank products by                      ┌─ scan: find products with
  │  data_issues + score_missing                 │  empty short OR long copy,
  │                                              │  out of cooldown
  │                                              │
  │                                              ├─ ≥3 candidates? → cold-draft path:
  │                                              │    take up to 10
  │                                              │    for each: skill call drafts
  │                                              │      missing fields, 3 variants
  │                                              │    pack as BatchSiblings:
  │                                              │      title = "Review & approve ·
  │                                              │        N product descriptions"
  │                                              │      intent = "fill missing copy"
  │                                              │
  └─ pick top product → 3 variants               └─ <3 candidates? → single-rewrite
     persist as single proposal                    path (unchanged from today)
```

The cold-draft branch and the single-rewrite branch are mutually exclusive within a tick. If both could fire (≥3 cold candidates and a healthy rewrite queue) the cold-draft path wins, on the principle that empty-copy products are higher priority than rewrites of already-decent copy.

### Key shape changes

**Go — `daemon/internal/personas/marketing/marketing.go`:**

- Add a `selectColdDraftCandidates()` helper that filters the priority list to products with `short_description == ""` OR `long_description == ""`, dropping any in cooldown. Returns up to `coldDraftMax`.
- `Draft()` calls `selectColdDraftCandidates` first; if the slice has ≥`coldDraftMin`, enters a new `draftColdBatch()` path that calls the skill per candidate and packs results via `BatchSiblings`. Otherwise falls through to existing `draftForProduct` single-rewrite loop.
- New constants: `coldDraftMin = 3`, `coldDraftMax = 10`.
- New skill invocation parameter: `mode = "cold_draft" | "rewrite"`. The skill prompt branches on `mode` to either draft missing-only fields (cold) or rewrite existing copy (rewrite).
- Dedup key: `product:{id}` for cold-draft children — same as Marketing's existing rewrite path (`marketing.go:299`, per the convention in `docs/specs/2026-05-18-agent-proposal-dedup-design.md`). Cold-draft and rewrite are mutually-exclusive product states (both fields empty → cold-draft eligible; any field filled → rewrite candidate), so they don't need distinct keys. Sharing the key also means a still-pending cold-draft proposal blocks a rewrite proposal from emitting for the same product, which is correct.
- The first-mover guard against re-batching the same products is **Marketing's existing picker filter**: `selectColdDraftCandidates` skips products that already have an open `in_review` issue for this persona (same filter Marketing's single-rewrite picker uses today — `marketing.go:295-296`). Once a cold-draft batch lands, its children are open issues and their products drop out of the next tick's candidate scan until they're approved or dismissed.

**Go — `daemon/internal/personas/personas.go`:**

- No changes to the `Drafted` struct. `BatchSiblings`, `BatchTitle`, `BatchIntent` already exist (lines 185-191) and are exactly the API we use.

**Go — skill template (`prompts/skills/marketing-description-rewrite/v1.yaml`):**

- Add a `mode` parameter; when `mode == "cold_draft"`, the prompt sections shift:
  - Drop "rewrite the existing copy" framing; substitute "draft copy from scratch for the missing field(s)."
  - Include only the field(s) the variant should fill (`short`, `long`, or both) in the LLM's expected JSON schema.
  - The voice corpus / SEO rubric sections from DSGWOO-1326 stay the same.

**Go LLM output → Variant:**

- The LLM's variant JSON gains a structured body: `{"short": "...", "long": "..."}` instead of a single `body` string. Either field may be omitted when not being drafted.
- `parseVariants` validates the structured shape; if both fields are missing, the variant is rejected.
- The proposal's `target` map carries `target.drafting = ["short"] | ["long"] | ["short", "long"]` to indicate which field(s) the batch's children write to. (Per-child resolution: each child's variants only emit drafts for fields listed in that child's `drafting`.)

**TypeScript — `ui/src/api/client.ts`:**

- `Variant` body changes from `body: string` to a union:
  - For rewrites (existing): `body: string` (back-compat — old proposals still parse).
  - For cold-drafts (new): `body: { short?: string; long?: string }`.
- A `Variant.body_kind: "single" | "structured"` discriminator lets the UI branch cleanly. The accessor (`variantsFromProposal`) tags variants based on the proposal's `target.drafting` field.

**TypeScript — `ui/src/screens/BatchReview.tsx`:**

- `renderMarketingBody` branches on body_kind. For `structured`:
  - `Current` column splits into two cells stacked vertically: "Short description (live)" and "Long description (live)." Each shows existing copy or `— empty —`.
  - Each variant column mirrors the same two-cell layout. A field not being drafted by that variant (i.e., not in `target.drafting`) shows "no change" greyed.
- The KPI strip stays four tiles. Hint text on the brand-voice tile changes from "vs. your existing copy" to "vs. your voice model" when *any* child in the batch has empty existing copy (the condition fires for any cold-draft batch).
- Footer sticky-bar message: `Variant {ID} selected — will write product.short_description + long_description on approve`. When only one field is being filled, the bar narrows to that single field name.

### What this doesn't change

- `BatchReview.tsx:421` already renders `previousCopy || '— empty —'` — the existing empty-state placeholder is reused.
- Per-row "Best SEO / Voice" badges (BatchReview.tsx:372-387) already read from variant.seo / variant.voice; they keep working for cold-draft variants that have scores attached.
- `wooagent-products/update` (manifest at `default.json:87`) already supports writing arbitrary product fields. No manifest change, no new ability.
- The scheduler doesn't need to know about cold-draft mode. The persona makes the decision per tick.

### Cost / latency impact

- For a cold-draft batch of N children, Marketing makes N LLM calls (one per product) in series within a single Draft() invocation. Each call is the same shape as today's rewrite (~5–15s). Worst case (N=10): the tick takes ~50–150s instead of the typical ~5–15s.
- This is significantly longer than a single-rewrite tick but acceptable because the batch surfaces 10 products of work in one go, and the alternative (10 sequential single-rewrite ticks across many days) is slower for the operator.
- If 150s wall-clock turns out to be problematic, follow-up work parallelizes the per-product LLM calls within the same tick. Out of scope here.

## Apply path

The approve handler for a batch child reads the proposal's selected variant and the `target.drafting` field, then issues a single `wooagent-products/update` call with a payload containing only the previously-empty fields:

```json
{
  "product_id": 3894,
  "short_description": "<variant.body.short>",
  "long_description": "<variant.body.long>"
}
```

If `target.drafting == ["short"]`, the payload omits `long_description` and the existing long copy is left untouched.

**Reversibility (Undo):** The apply handler snapshots the product's previous values (empty strings for the empty fields) before writing. Undo issues a second `wooagent-products/update` with the snapshotted values, restoring the literal previous state — empty stays empty.

## Retirement of Reporting `product_health_digest`

- Delete the body of `Draft()` in `reporting/reporting.go:73-128` along with the helpers `findProductsWithDataIssues`, `buildDataIssuesDigest`, and `renderDigest`. The persona's `Draft()` becomes a stub that returns `Drafted{Skipped: true, SkipReason: "no skills registered yet"}`.
- Keep the persona registration in `cli/run.go` so Reporting still appears in the sidebar and is wired into the scheduler. The seven-persona model is preserved.
- Existing in-flight `product_health_digest` proposals on operator stores are left alone. Operators dismiss them naturally. No banner, no auto-dismiss, no migration.

## Testing

**Go:**

- `marketing_test.go` gains:
  - `TestDraft_ColdDraftBatch_AboveThreshold` — given a catalog with 4 empty-copy products, `Draft()` returns a `Drafted` with `BatchSiblings` length 4 and the expected title.
  - `TestDraft_ColdDraftBatch_BelowThreshold_FallsThrough` — given 2 empty-copy products, `Draft()` returns a single rewrite (or `Skipped` if no rewrite candidates).
  - `TestDraft_ColdDraftBatch_PartialEmptyFields` — given a product with short_description="" and long_description="filled", the variant body has `short` populated and `long` omitted.
  - `TestDraft_ColdDraftBatch_RespectsCooldown` — products in `marketing:cold_draft:*` cooldown are filtered out.
- `handlers_batches_test.go` gains a cold-draft fixture variant for the existing approve test, asserting the `wooagent-products/update` payload contains only the previously-empty fields.

**UI:**

- BatchReview snapshot test gains a fixture for a cold-draft batch with mixed partial cases (some short-only, some long-only, some both-empty). Verifies:
  - Current column shows two cells per row with correct empty-state rendering.
  - Variant columns show "no change" greyed cells for fields not being drafted.
  - Footer sticky bar text matches the field(s) being filled.

## Migration / rollout

- No schema migration. Proposal `target` is a free-form JSON map; adding `target.drafting` is additive.
- No feature flag needed. The trigger threshold (3 candidates) is conservative enough that the first cold-draft batch a store sees will be obvious in context.
- Reporting digest retirement is a code deletion plus persona stub. No data migration; existing proposals stay valid records.

## Follow-ups (file in DSGWOO after merge)

- **Build the undo system end-to-end.** Discovered during implementation: the UI's `onUndo` handler just navigates home (`IssueDetail.tsx:267`); there is no `/v1/issues/:id/undo` endpoint, no snapshot store, no reverse-dispatch. The "Reversible · always" label in the action bar is aspirational. The cold-draft path captures `target.previous_short` and `target.previous_long` at draft time so the snapshot data is already in place — when undo is built, the cold-draft case will work without a backfill. Scope of the follow-up: HTTP endpoint, reverse-dispatch map keyed on proposal_type, audit row for the undo action, and a UI wire-up that actually calls the endpoint.
- **Dedup gate on the batch-emit path in `RunAndPersist`.** The existing dedup spec deliberately left the batch branch ungated ("a few-line follow-up when a persona needs it"). Cold-draft batches are the first non-Pricing batch use case where this matters — if Marketing's picker filter ever lets a product slip through (race condition, partial dismissal, etc.), `RunAndPersist` should refuse a duplicate child by `(persona, dedup_key)` the same way it does for non-batch issues.
- **Batch-aggregate scoring for the KPI header strip.** Today the four tiles in BatchReview are hardcoded placeholders. Compute average SEO and brand-voice scores across the batch's selected variants and render real values.
- **Parallel per-product LLM calls within a cold-draft tick.** If wall-clock latency becomes a friction point in practice, parallelize the N LLM calls.
- **Reporting's next skill.** After this work lands, Reporting is a dormant persona. Sales summary / top movers / KPI digest work that uses real reporting data is the next logical addition.
