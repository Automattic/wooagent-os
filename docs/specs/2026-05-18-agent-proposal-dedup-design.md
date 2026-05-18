# Proposal Dedup — Design

**Date:** 2026-05-18
**Status:** Design (pre-implementation plan)
**Scope:** A system-wide guard in `RunAndPersist` that blocks a persona from inserting a new issue when an identical-shape `in_review` issue is already open for that persona. Adds an opt-in `DedupKey` to `personas.Drafted`, a nullable `dedup_key` column on `issues`, and adopts the field across the four shipping personas.

## Motivation

On 2026-05-18 the Reporting persona emitted two identical "Week of May 18 · Sales digest" proposals 12 minutes apart. Both rows landed in `in_review` and surfaced as two visually-identical cards in Needs Review.

The existing dedup pattern (`personas.CooldownPolicy` + `RecentlyTouchedTargets`) keys on an integer field inside `proposal_target` — `product_id` for Marketing/Pricing, `order_id` for Sales Support. Digest-style personas don't fit this contract: Reporting summarizes *catalog state right now*, with no per-target id to dedup against. Its `Cooldown()` returns zero-value, and the persona doc-comment says the cadence-based gating on `agents.cadence_seconds` is "the only 'don't propose too often' lever this persona uses." That lever clearly didn't hold here — most likely two scheduler ticks raced through `RunAndPersist` (or the operator clicked Run Now twice) and `RunAndPersist`'s only existing pile-up guard, `CountOpenWork` against `OpenProposalSkipThreshold`, is count-based, not content-based.

The fix is a narrow system-wide invariant: **the same persona cannot have two open proposals with the same logical identity at the same time.** Per-persona Cooldown stays where it is — it handles the longer-window approved/dismissed history, which this rule deliberately does not.

## Goals

1. Prevent a persona from inserting a new issue when another `in_review` issue with the same `(persona, dedup_key)` already exists.
2. Make the dedup identity explicit on the persona side — each `Drafted` declares its own `DedupKey`. No JSON-canonicalization of `Target`, no hashing-by-convention.
3. Surface dup skips through the same telemetry + run-row machinery already used for Cooldown skips (run row written, turn event recorded, no issue inserted, `Result.Skipped = true` on first emit).
4. Cover Reporting today and Marketing / Pricing / Sales Support as a belt-on-top-of-Cooldown.

## Non-goals

- **Per-persona approved/dismissed windows for digest-style personas.** That's a Cooldown extension, deferred. If Reporting needs a 1-day window after dismiss, we revisit by extending `CooldownPolicy` to accept string target keys — separate spec.
- **Batches.** No `dedup_key` column on `batches`, no dedup gate on the batch-emit path. No shipping persona emits a digest-style batch today; adding this when a persona needs it is a few-line follow-up.
- **Cross-persona dedup.** Two personas proposing on the same product is not a duplicate — it's deliberate (e.g., Marketing copy + Pricing price for the same SKU).
- **Backfill of historical `issues` rows.** Existing rows stay `dedup_key = NULL`. NULL is treated as "no opinion / don't dedup", so the existing fleet behavior is unchanged.
- **UI surfacing of the dup skip.** The skip writes a run row with a reason like every other skip; that's enough for the operator to find via the runs view. No new badge or banner.

## Design decisions

### 1. `DedupKey` is an explicit field on `Drafted`, not derived

Each persona declares its own identity. Adding a `DedupKey string` field to `personas.Drafted`:

- **Stable across runs** even when volatile counts change. Reporting's `Target` carries `total_count` and `preview_size`, which change as the catalog evolves. Hashing the whole target map would treat "5 products need attention" and "6 products need attention" as different proposals — the opposite of what the operator wants.
- **Persona authors choose the granularity.** Reporting's current data-issues digest uses `"digest:data_issues"`. A future weekly sales digest would use `"digest:sales_weekly:2026-W20"` — ISO-week-stamped so the next week's digest isn't blocked by this week's still-open one. Marketing uses `"product:42"`.
- **Empty string opts out.** A persona that genuinely wants overlapping proposals (none today) sets `DedupKey: ""` and the guard is a no-op. Same back-compat as today.

The Persona interface doc-comment in `personas.go` gains a third "established pattern" alongside Cooldown and the within-run loop: set `Drafted.DedupKey` to a stable logical identity.

### 2. `dedup_key TEXT` column on `issues`, additive migration

Migration `013_issues_dedup_key.sql`:

```sql
ALTER TABLE issues ADD COLUMN dedup_key TEXT;

-- Partial index: only open rows with a dedup_key participate in the
-- lookup. Cheap insurance; can stay even if v0.1 traffic is small.
CREATE INDEX idx_issues_dedup_open
  ON issues(persona, dedup_key)
  WHERE status = 'in_review' AND dedup_key IS NOT NULL;
```

Schema is purely additive. No backfill — existing rows get `NULL`, which the guard reads as "no opinion." No data migration risk.

### 3. Insert-time guard in `RunAndPersist`

The check goes inside the emit loop in `personas.RunAndPersist`, immediately before the `insertIssue(...)` call in the non-batch branch. The batch branch (`len(d.BatchSiblings) > 0` → `insertBatch`) is deliberately untouched — see Non-goals and Out of scope.

```go
if d.DedupKey != "" {
    existingID, err := findOpenIssueWithDedupKey(ctx, deps.Store, slug, d.DedupKey)
    if err != nil {
        // Fail open: log and proceed to insert. The guard is a safety
        // net, not a correctness invariant — refusing to emit on a
        // transient query error would be worse than the occasional dup.
        // The DB-side unique index (deferred follow-up) is where we'd
        // promote this to a hard guarantee.
    }
    if existingID != "" {
        recordTurn(ctx, deps.Recorder, tracker, "", d)
        reason := fmt.Sprintf("duplicate of %s (still in review)", existingID)
        if i == 0 {
            res.Skipped = true
            res.SkipReason = reason
            return res, nil
        }
        if res.SkipReason == "" {
            res.SkipReason = fmt.Sprintf("emit %d/%d skipped: %s", i+1, emitTarget, reason)
        }
        break
    }
}
```

`findOpenIssueWithDedupKey` lives in `daemon/internal/personas/personas.go` next to `RecentlyTouchedTargets`. It is a single-row lookup:

```sql
SELECT id FROM issues
 WHERE persona = ?
   AND dedup_key = ?
   AND status = 'in_review'
 LIMIT 1;
```

The exit shape mirrors the existing Cooldown/Skipped pattern — `recordTurn` writes a telemetry row, no issue is inserted, the run row stores the dup reason. The operator can see the skip in the runs view.

**`insertIssue` writes `dedup_key`.** The column is populated from `Drafted.DedupKey`; empty string maps to SQL `NULL` so the partial index stays clean.

**Race window.** The check and the insert are not in a single transaction. Two near-simultaneous emits could both see "no existing" and both insert. v0.1 accepts this — `RunAndPersist` is called serially per persona by the scheduler worker, and the operator's "Run Now" button is the only known way to drive concurrent runs of the same persona. If duplicates surface in practice we tighten to a transaction or add a unique partial index. Calling out as a known limitation rather than premature hardening.

### 4. Per-persona DedupKey adoption

All four shipping personas set `Drafted.DedupKey`:

| Persona       | DedupKey                       | Notes |
|---------------|--------------------------------|-------|
| Reporting     | `"digest:data_issues"`         | Stable today (only one digest kind). Future digest kinds slot in beside this. |
| Marketing     | `"product:<id>"`               | Belt over the existing product_id Cooldown. |
| Pricing       | `"product:<id>"`               | Same. |
| Sales Support | `"order:<id>"`                 | Belt over the existing order_id Cooldown. |

Marketing / Pricing / Sales Support already dedup at the *picker* level via `RecentlyTouchedTargets` (which includes `in_review`), so the new guard rarely fires for them in practice — it's the safety net against the same race that bit Reporting.

### 5. Telemetry and runs

- The dup skip path calls `recordTurn(ctx, deps.Recorder, tracker, "", d)` so the GEPA pipeline still gets a turn row for the not-emitted proposal.
- The run row's `skip_reason` carries the dup reason exactly like other Skipped outcomes.
- No new metric / counter / column added.

## Concrete changes

Files touched:

- `daemon/internal/store/migrations/013_issues_dedup_key.sql` — new migration (additive column + partial index).
- `daemon/internal/store/store.go` — `InsertIssue` (or wherever issues are inserted) takes / writes `dedup_key`. New `FindOpenIssueWithDedupKey(ctx, persona, key string) (string, error)` storage helper.
- `daemon/internal/personas/personas.go`
  - Add `DedupKey string` to `Drafted`.
  - Update Persona interface doc-comment to list DedupKey as the 3rd established pattern.
  - Add `findOpenIssueWithDedupKey` (thin wrapper, or inline call to store helper).
  - Insert-time guard inside the `RunAndPersist` emit loop.
  - Update `insertIssue` to pass `DedupKey` through to the store.
- `daemon/internal/personas/reporting/reporting.go` — return `DedupKey: "digest:data_issues"` in `Draft`. Doc-comment on `Cooldown()` updated to point at DedupKey as the in-review guard (it stays zero-value).
- `daemon/internal/personas/marketing/marketing.go` — set `DedupKey: fmt.Sprintf("product:%d", productID)` in the Drafted.
- `daemon/internal/personas/pricing/pricing.go` — same shape.
- `daemon/internal/personas/sales-support/sales_support.go` — `DedupKey: fmt.Sprintf("order:%d", orderID)`.

## Tests

Unit-level (in `personas/personas_test.go`):

- `TestFindOpenIssueWithDedupKey_HitsInReview` — seed one `in_review` issue with `dedup_key = "X"`, lookup returns its id.
- `TestFindOpenIssueWithDedupKey_IgnoresNonInReview` — seed `done` and `dismissed` rows with same key; lookup returns empty.
- `TestFindOpenIssueWithDedupKey_IgnoresOtherPersona` — same key under a different persona; lookup returns empty.
- `TestFindOpenIssueWithDedupKey_NullStoredKey` — a `dedup_key IS NULL` row never matches a non-empty query key.
- `TestInsertIssue_EmptyDedupKeyStoresNull` — storage helper writes SQL `NULL`, not the empty string, so the partial index stays clean.

Integration (in `personas/personas_test.go` using the existing `fakePersona` harness):

- `TestRunAndPersist_DedupKey_BlocksWhileInReview` — `RunAndPersist` twice with identical Drafted (`DedupKey` set); first inserts, second returns `Result.Skipped = true` with `SkipReason` containing the first id.
- `TestRunAndPersist_DedupKey_EmptyKeyIsNoOp` — same scenario, `DedupKey: ""`; both inserts succeed (back-compat).
- `TestRunAndPersist_DedupKey_DoneRowDoesNotBlock` — first insert moved to `status='done'`; second emit with same key succeeds.

Per-persona (each persona package):

- Reporting: `TestReporting_Draft_SetsDedupKey` — Drafted carries `"digest:data_issues"`.
- Marketing / Pricing: same — Drafted carries `"product:<productID>"`.
- Sales Support: Drafted carries `"order:<orderID>"`.

## Risks and open questions

1. **Race window between check and insert.** Two concurrent `RunAndPersist` calls for the same persona could both pass the check and both insert. Mitigation deferred to follow-up; if duplicates surface, switch to `INSERT … WHERE NOT EXISTS …` or a unique partial index `(persona, dedup_key) WHERE status = 'in_review'`. Accepted as known limitation given current single-worker scheduler.
2. **Unique partial index instead of guard?** SQLite supports partial unique indexes. We could enforce dedup as a constraint and treat the resulting insert error as the skip signal. Considered and rejected for v0.1: the guard's explicit "skip with reason" is easier to thread through telemetry and easier to read in logs than catching a constraint violation. Revisit if the race-window mitigation forces us to lean on the DB.
3. **What if the in-review issue is stale?** A Reporting digest from a week ago that nobody touched would block today's emit. v0.1 accepts this — the operator can dismiss the stale card and the next tick succeeds. A future "auto-stale" pass on `in_review` issues is a separate question.
4. **Approved/dismissed windows for Reporting.** Out of scope here. The right home is extending `CooldownPolicy` to support string keys; that's a separate spec the day Reporting (or another digest) needs it.

## Out of scope (follow-ups, in priority order)

1. Batches: `dedup_key` column on `batches`, gate on the batch-emit path. Needed only when a persona emits a digest-style batch.
2. Cooldown extension for string target keys (post-approve/dismiss windows for digests).
3. Transactional check+insert (or unique partial index) if dup races surface in practice.
4. UI: a "duplicate skipped" badge in the runs view if operators ask for it.
