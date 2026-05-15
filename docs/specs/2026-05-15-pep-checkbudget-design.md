# PEP Phase-2: checkBudget — Design

**Date:** 2026-05-15
**Status:** Design (pre-implementation plan)
**Scope:** PEP Check 5 (budget) plus a scheduler-level pre-tick gate sharing the same counter store. `checkPolicy` remains stubbed.

## Motivation

WooAgent is launching to the public, not to a demo audience. Of the four originally-stubbed PEP checks, schema validation and scope sufficiency landed in the [previous round](./2026-05-15-pep-schema-scope-design.md). This spec covers Check 5 — daily budgets per persona.

The threat model is autonomous agents burning model spend in a loop. The PEP-only gate is necessary but not sufficient because the LLM tokens are spent *before* the gate fires (the LLM produces the MCP call, then the PEP checks it). To meaningfully cap spend, we need a pre-tick check in the scheduler too. Both gates share one counter store, one threshold table, one set of types.

## Goals

1. Block MCP dispatch via `pep.checkBudget` when a persona is at or over its daily cost or call limit.
2. Block persona-tick scheduling when the same conditions hold — stops the LLM from being called at all for over-budget personas.
3. Track per-persona daily cost (USD) and call counts in a dedicated SQLite table.
4. Allow operators to override conservative baked-in defaults via `~/.wooagent/budgets.json`.
5. Reset counters at the operator's local midnight (calendar day).
6. Surface denials via the existing audit log + Info-level scheduler skip logs — UI is a follow-up.

## Non-goals

- Policy predicate evaluation (`checkPolicy`).
- Any new UI: no "paused" badge, no per-persona usage indicator, no threshold editor screen.
- Per-ability or per-store sub-budgets.
- Mid-day budget edit reload (changes require daemon restart, matching the manifest-overlay pattern).
- Token-to-USD fallback computation (we rely on `ModelCall.CostUSD` being populated by the LLM adapter; zero-cost calls count as zero).
- Retention/pruning of old counter rows (low cardinality; deferred).
- Reconciliation tool for drift between counters and `turn_events`.

## Design decisions

### 1. Dual gate, shared counter store

The PEP-only gate is the canonical Check 5 per PRD §8.4.2 — recorded as a typed denial in `audit_invocations`. The scheduler pre-tick gate is the "real" enforcement: it refuses to start a persona run when the daily budget is exhausted, stopping the LLM spend at the source. Both read the same `persona_budget_usage` table; both use the same threshold table; both reach the same conclusion deterministically.

This avoids the architectural mismatch where PEP-only enforcement would let the LLM spend tokens, then deny the resulting action — letting the operator burn budget without producing any output.

### 2. Two dimensions: cost (USD) + call count

- `cost_usd` covers LLM burn rate (the threat model).
- `call_count` covers the orthogonal "even if free, are we hammering the store" case (e.g. a future free-tier model with no cost signal).

Tokens are not enforced directly — they're bookkeeping behind the cost calculation, which the LLM adapter already does.

### 3. Calendar day in the operator's local TZ

The daemon runs on the operator's machine; `time.Now().Local()` gives us the operator's TZ for free. Counters reset at local midnight. Display is intuitive: "your budget resets at midnight."

### 4. Defaults baked in + JSON overlay

Per-persona defaults live in `daemon/internal/pep/budget.go`. Operators override via `~/.wooagent/budgets.json` (partial overrides allowed). Matches the existing manifest-overlay pattern. No UI in v1.

### 5. New `persona_budget_usage` table

Materialized counter table with composite primary key `(persona, usage_date)`. Direct lookup at PEP/scheduler check time; UPSERT increments. Sub-ms reads. Old rows retained indefinitely (low volume).

### 6. Silent gating

When over budget: PEP denies (HTTP 429 — existing mapping), scheduler skips ticks with Info log, audit log records denial. No UI signal in v1. Lack of new proposals + audit log are the observability. Surfacing the state in UI is a follow-up Linear ticket.

## Data model

### `persona_budget_usage` table (new migration `012_persona_budget_usage.sql`)

```sql
CREATE TABLE persona_budget_usage (
    persona     TEXT NOT NULL,         -- e.g. "marketing"
    usage_date  TEXT NOT NULL,         -- YYYY-MM-DD, operator's local TZ
    cost_usd    REAL NOT NULL DEFAULT 0,
    call_count  INTEGER NOT NULL DEFAULT 0,
    updated_at  TEXT NOT NULL,         -- ISO-8601 UTC
    PRIMARY KEY (persona, usage_date)
);

CREATE INDEX idx_persona_budget_usage_date ON persona_budget_usage(usage_date);
```

- `cost_usd` is `REAL` (not integer cents) — model providers report fractional USD.
- `usage_date` is `TEXT` (YYYY-MM-DD) so `usage_date = ?` is a string equality, no parsing.
- Index on `usage_date` for future retention sweeps.

### Threshold types (in `daemon/internal/pep/budget.go`)

```go
type Threshold struct {
    DailyCostUSD float64
    DailyCalls   int
}

// Default: 7 personas × {$5, 100 calls}. Conservative — high enough that
// normal use doesn't trip; low enough to catch a runaway loop within hours.
var defaultThresholds = map[manifest.Persona]Threshold{
    manifest.PersonaMarketing:    {DailyCostUSD: 5.00, DailyCalls: 100},
    manifest.PersonaPricing:      {DailyCostUSD: 5.00, DailyCalls: 100},
    manifest.PersonaInventory:    {DailyCostUSD: 5.00, DailyCalls: 100},
    manifest.PersonaAccounting:   {DailyCostUSD: 5.00, DailyCalls: 100},
    manifest.PersonaReporting:    {DailyCostUSD: 5.00, DailyCalls: 100},
    manifest.PersonaSalesSupport: {DailyCostUSD: 5.00, DailyCalls: 100},
    manifest.PersonaChiefOfStaff: {DailyCostUSD: 5.00, DailyCalls: 100},
}
```

### Overlay file shape (`~/.wooagent/budgets.json`)

```json
{
  "marketing":     {"daily_cost_usd": 10.0, "daily_calls": 200},
  "sales-support": {"daily_cost_usd": 2.0}
}
```

- Missing fields fall back to the baked default.
- Missing personas use defaults entirely.
- Unknown persona slugs → Warn log, entry skipped.
- File missing → defaults silently. Malformed JSON → Error log + defaults.

## Implementation

### Files

| File | Change |
|---|---|
| `daemon/internal/store/migrations/012_persona_budget_usage.sql` *(new)* | Counter table migration. |
| `daemon/internal/pep/budget.go` *(new)* | `Threshold`, `BudgetGate`, `LoadBudgetOverlay`, `DefaultThresholds`, `NewBudgetGate`. |
| `daemon/internal/pep/budget_test.go` *(new)* | Unit tests for thresholds + counter operations + TZ math. |
| `daemon/internal/pep/checkbudget_test.go` *(new)* | PEP-level integration tests. |
| `daemon/internal/pep/pep.go` | Add `budget *BudgetGate` to PEP struct; implement `checkBudget(ctx, req)`; pass ctx in Invoke; thread `req.Persona` through `audit.finalize`. |
| `daemon/internal/pep/audit.go` | `newAuditWriter(db, budget)`; `finalize` accepts persona, increments call_count on Allowed outcomes. |
| `daemon/internal/telemetry/store.go` | `SQLiteRecorder` gains a `budget` field; after `INSERT INTO turn_events`, sum `ModelCalls.CostUSD` and call `budget.IncrementCost`. |
| `daemon/internal/scheduler/worker.go` | Pre-tick budget check before persona run dispatch; skip + Info log when exhausted. |
| `daemon/internal/cli/run.go` | Load overlay; construct `BudgetGate`; pass to PEP, scheduler worker, telemetry recorder. |
| `daemon/internal/httpapi/handlers_v1.go` | Sharpen `pepDenialMessage` for `ReasonBudgetExceeded` to include the local-midnight reset hint. |

### `BudgetGate` interface

```go
type BudgetGate struct {
    db         *sql.DB
    thresholds map[manifest.Persona]Threshold
    clock      func() time.Time // injectable for tests; defaults to time.Now
}

func NewBudgetGate(db *sql.DB, thresholds map[manifest.Persona]Threshold) *BudgetGate

// Check reports whether the persona is at or over its daily limit RIGHT NOW.
// Returns the typed reason code when blocked, or "" when within budget.
func (g *BudgetGate) Check(ctx context.Context, persona manifest.Persona) (ReasonCode, error)

// IncrementCost adds the dollar amount to today's row for persona.
func (g *BudgetGate) IncrementCost(ctx context.Context, persona manifest.Persona, usdAmount float64) error

// IncrementCalls bumps today's call_count by 1 for persona.
func (g *BudgetGate) IncrementCalls(ctx context.Context, persona manifest.Persona) error
```

`Check` flow:

1. `g.clock().Local().Format("2006-01-02")` → today's local-TZ date.
2. `SELECT cost_usd, call_count FROM persona_budget_usage WHERE persona = ? AND usage_date = ?`.
3. `ErrNoRows` → counters are zero → return `""`.
4. Other DB error → return `ReasonBudgetExceeded` (conservative deny; logged at Warn).
5. Look up threshold for persona. Missing (unknown persona slug) → return `""` (don't block on misconfig; Warn log).
6. If `cost_usd >= threshold.DailyCostUSD` OR `call_count >= threshold.DailyCalls` → return `ReasonBudgetExceeded`. Else `""`.

`>=` not `>` because increments happen *after* the check — at threshold `100`, the 100th call goes through, the 101st is denied.

Increment uses UPSERT:

```sql
INSERT INTO persona_budget_usage (persona, usage_date, cost_usd, call_count, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(persona, usage_date) DO UPDATE SET
    cost_usd   = cost_usd + excluded.cost_usd,
    call_count = call_count + excluded.call_count,
    updated_at = excluded.updated_at;
```

### Where increments fire

| Caller | Trigger | Counter | Source of value |
|---|---|---|---|
| `auditWriter.finalize` | Outcome is `success` or `mcp_error`, AND `persona != ""` | `call_count += 1` | n/a |
| `telemetry.SQLiteRecorder.Record` | After `INSERT INTO turn_events` succeeds, AND `e.Persona != ""` | `cost_usd += Σ(model_calls.CostUSD)` | sum the `ModelCall.CostUSD` slice in `e.ModelCalls` |

Denied PEP calls (outcome `denied`) do not bump `call_count` — the call never touched the store. The audit row records the denial regardless. Empty-persona rows would just be noise in the counter table (and the audit log's `persona NOT NULL` constraint means we'd be writing `("", today)` rows that no `Check` call would ever look up), so both increment paths skip when persona is empty.

### Invoke pipeline

Pipeline order stays as PRD §8.4.2: trust → persona → schema → policy → budget → scope-sufficiency. `checkBudget` gains a `ctx context.Context` parameter (matching `checkSchema` from the prior round) so it can do the DB read with cancellation.

```go
func (p *PEP) checkBudget(ctx context.Context, req Request) ReasonCode {
    if p.budget == nil {
        return ""
    }
    reason, err := p.budget.Check(ctx, req.Persona)
    if err != nil {
        return ReasonBudgetExceeded
    }
    return reason
}
```

The `p.budget == nil` guard is defensive — tests can construct a minimal `&PEP{}` without wiring the gate.

### Scheduler pre-tick gate

In `daemon/internal/scheduler/worker.go`, before dispatching a persona tick:

```go
if reason, err := s.budget.Check(ctx, persona); err != nil || reason != "" {
    s.log.Info("scheduler skipped tick over budget", "persona", persona, "reason", reason)
    continue
}
```

The exact insertion point follows the existing per-persona dispatch loop. No state change beyond the skip + log.

## Error handling

| Failure | Behavior | HTTP status |
|---|---|---|
| Daily threshold reached | `ReasonBudgetExceeded` denial; audit row + 429 | 429 (existing mapping) |
| `BudgetGate.Check` DB error | Conservative deny (`ReasonBudgetExceeded`); Warn log | 429 |
| `IncrementCost` DB error | Log Warn, swallow. The check is authoritative; a missed increment is a slight under-count. | n/a |
| `IncrementCalls` DB error | Same as above. | n/a |
| Overlay JSON malformed | Error log at startup, fall back to defaults. Daemon continues. | n/a |
| Overlay has an unrecognized persona slug | Warn log at load time, that entry skipped. Other entries still applied. (Typo case: operator wrote `"marketting"` in budgets.json.) | n/a |
| `Check` called with a persona not in the threshold map | `""` (don't block on misconfig); Warn log. (Caller-side bug: a Request reached `checkBudget` with an unrecognized Persona value.) | n/a |
| Increment called with empty Persona | Skip the increment, no log. (Defensive — the upstream caller should have set Persona; we don't want to pollute the counter table with empty-persona rows.) | n/a |

`pepDenialMessage` update:

```go
case pep.ReasonBudgetExceeded:
    return "persona is over its daily budget — counters reset at local midnight"
```

### Concurrency

`BudgetGate` is goroutine-safe: no mutable in-memory state, all writes through SQLite (which serializes writers). Concurrent ticks of the same persona may briefly overshoot (both pass the check at 99/100 calls, both increment to 101) — acceptable for a budget meant to catch runaway loops, not enforce a hard cap. SQLite's atomic per-statement semantics keep the math correct (no lost increments).

## Testing

### `budget_test.go` (BudgetGate unit tests)

Backed by in-memory SQLite, no PEP fixture. Tests cover the gate's contract:

| Case | Setup | Expected |
|---|---|---|
| Check, no row | Empty table | `""`, no error |
| Check, under cost | cost_usd=1.00, threshold=5.00 | `""` |
| Check, at cost threshold | cost_usd=5.00, threshold=5.00 | `ReasonBudgetExceeded` |
| Check, over cost | cost_usd=6.00, threshold=5.00 | `ReasonBudgetExceeded` |
| Check, at call threshold | call_count=100, threshold=100 | `ReasonBudgetExceeded` |
| Check, under both | cost=1.00, calls=10, thresholds=5/100 | `""` |
| Check, unknown persona | persona slug not in threshold map | `""` (Warn log) |
| Check, DB error (closed DB) | Close DB then call | `ReasonBudgetExceeded` |
| IncrementCost, no prior row | Empty table | Row created with cost_usd=X |
| IncrementCost, prior row | Existing row today | cost_usd accumulates |
| IncrementCost, different day | Existing row for yesterday | Today row created separately |
| IncrementCalls, no prior row | Empty table | Row created with call_count=1 |
| IncrementCalls, prior row | Existing row today | call_count += 1 |
| Date respects local TZ at 23:30 | clock=2026-05-15 23:30 local | usage_date="2026-05-15" |
| Date respects local TZ at 00:30 | clock=2026-05-16 00:30 local | usage_date="2026-05-16" |

Clock injected via `BudgetGate.clock func() time.Time`.

### `checkbudget_test.go` (PEP integration)

Extends the existing `newTestPEP` fixture to wire a real `BudgetGate`.

| Case | Setup | Expected |
|---|---|---|
| Under budget, allow | No usage row | `Allowed=true`, MCP called, call_count=1 after |
| Cost over budget, deny | Pre-insert today's row with cost_usd > threshold | `Reason=ReasonBudgetExceeded`, MCP not called |
| Calls over budget, deny | Pre-insert today's row with call_count >= threshold | `Reason=ReasonBudgetExceeded`, MCP not called |
| Yesterday's usage doesn't bleed | Pre-insert yesterday's row near threshold | Today's call passes |
| Successful Invoke increments | Empty fixture, Invoke succeeds | `call_count=1` after |
| Denied Invoke (trust check) does NOT increment | Ability not in manifest | counter unchanged |
| MCP-error Invoke counts as a call | fakeMCP with callErr | `call_count=1` (attempt happened) |

### `recorder_budget_test.go` (telemetry-side cost increment)

Record a TurnEvent with `ModelCalls: [{CostUSD: 2.50}, {CostUSD: 1.25}]` and verify `persona_budget_usage.cost_usd = 3.75` for today's row.

### Scheduler test (in worker_test.go or new file)

Pre-insert a row that exhausts the persona's threshold; run one scheduling tick; verify no run was started and the skip was logged.

### Existing tests

The existing `pep_test.go` tests pass `Source: SourceOperator` (from the prior round) and use `newTestPEP`. They'll continue to pass once `newTestPEP` wires the `BudgetGate` — the call_count increment path will be exercised in `TestInvoke_AllowedSuccess` automatically.

## Out of scope

- `checkPolicy` — separate brainstorm round (operator policy admin surface).
- UI surfacing (paused badge, usage indicator, threshold editor) — follow-up Linear ticket.
- Per-ability or per-store sub-budgets.
- Mid-day overlay reload.
- Token-to-USD fallback computation.
- Retention/pruning sweep.
- Reconciliation tool for counter drift.
- No coordination with the existing per-persona cooldown ([[project_agent_dedup_pattern]]) — independent gates.

## Open questions

None at design time. All six decision points were resolved in the brainstorming round:
1. Gate placement: PEP + scheduler pre-check sharing a counter store.
2. Dimensions: cost (USD) + call count.
3. Window: calendar day in operator's local TZ.
4. Threshold config: baked defaults + JSON overlay; no UI.
5. Counter store: new `persona_budget_usage` table.
6. Over-budget behavior: silent gating + audit + Info log; no UI in v1.
