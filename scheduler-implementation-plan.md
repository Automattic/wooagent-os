# Scheduler — Implementation Plan

Replace startup persona seeding with a SQLite-backed scheduler that owns periodic runs, retries with backoff, and operator-visible skip / failure reasons — then wire the run-log panel that consumes it. Codex internal-review item #5.

## Goal

Today, `runPersonas` in `daemon/internal/cli/run.go` fans the three registered personas out sequentially on boot and then never runs again. `HasOpenWork` silently dedups on restart. The `runs` table from `001_init.sql` exists but has zero callers. The result is a kanban that goes stale the moment the first wave of proposals lands, and operators have no view into what the daemon decided not to do.

The new model: one in-process scheduler, one worker, SQLite as the queue. Every persona attempt lands as a row in `runs` with a status, a reason, a retry chain, and a turn-event trace. Boot-time seeding becomes a special case (a one-shot bootstrap for fresh installs). Operators see every tick decision in a fleet-wide activity view and can drill into the trace for any single run.

## Scope for V1

In:
- SQLite-backed `runs` table populated for every persona attempt (queued, running, succeeded, skipped, failed, failed_permanent).
- Periodic tick loop with a per-persona `cadence_seconds`. Default 6h, configurable per row in `agents`.
- Manual triggers via `POST /v1/runs`.
- Bootstrap pass on fresh installs (zero historical runs → enqueue one bootstrap run per enabled persona).
- Classified retries: transient errors retry with 5m / 30m / 2h backoff up to `max_attempts`; permanent errors fail fast with a clear reason.
- Operator-visible skip rows when the scheduler considered a persona and chose not to run (e.g., `HasOpenWork`).
- `GET /v1/runs`, `GET /v1/runs/:id`, `POST /v1/runs`. Existing `GET /v1/agents` extended with `cadence_seconds`, `max_attempts`, `last_run_at`, `next_run_at`.
- Fleet-wide `/runs` route in `ui/` with `DataViews`-driven filters and a per-run detail page that surfaces the trace. Per-issue drill-down inside the existing IssueDetail.

Out (filed as Linear follow-ups after this lands):
- Concurrent workers. Today's single worker is correct because MCP holds one session-id; multi-store / multi-session unblocks parallelism.
- `POST /v1/runs/:id/cancel` for a stuck `running` row.
- Retention / pruning of old `runs` rows.
- Webhook-driven runs (depends on Companion Plugin v0.2 webhook surface).
- Per-run aggregate cost budget (PEP enforces per-call).

## What we already have

- **Personas registry** (`daemon/internal/personas/`). `Persona`, `Deps`, `Drafted`, `RunAndPersist`. `Drafted.Skipped` is a first-class outcome with a reason. `HasOpenWork` exists as a silent guard.
- **Turn-event telemetry** (`daemon/internal/telemetry/`). `TurnEvent` + `SQLiteRecorder` already write a row per persona attempt with model calls and skill calls. The scheduler just needs to bind each run to its turn_id.
- **Periodic scheduler precedent**. `abilities.SchedulePeriodic` (`daemon/internal/abilities/abilities.go`) is the pattern to mirror — same package shape, same context cancellation discipline.
- **Race-hardened claim pattern**. The batch-approve path in `httpapi` already uses `UPDATE … WHERE status=? AND id=?` + `RowsAffected==1` for at-most-once claims. Reuse verbatim.
- **Dead `runs` table.** Existing `001_init.sql:33` schema has four columns and zero writers; safe to drop and rebuild.

## What we're building

### Package `daemon/internal/scheduler/`

```go
// Scheduler owns the loop, worker, and dequeue/claim transaction.
// Holds no business logic — it calls personas.RunAndPersist.
type Scheduler struct {
    DB        *sql.DB
    Personas  []personas.Persona
    Deps      personas.Deps
    Clock     func() time.Time      // injectable for tests
    TickEvery time.Duration         // default 60s
    Backoff   []time.Duration       // default {5m, 30m, 2h}
}

func (s *Scheduler) Start(ctx context.Context) error
func (s *Scheduler) EnqueueManual(ctx context.Context, persona string) (Run, error)
func (s *Scheduler) tickOnce(now time.Time) error    // testable seam
```

`Start` is non-blocking: it spawns two goroutines and returns. Cancellation via `ctx`.

- **Loop.** Ticks every `TickEvery`. For each enabled persona: if `agents.last_run_at + cadence_seconds <= now` and no run is already `queued`/`running`, decide whether to enqueue. If `HasOpenWork(persona)` is true, write a `skipped` row instead. Bootstrap pass on first start (zero historical runs → enqueue with `trigger='bootstrap'`).
- **Worker.** Loops on the queue. Claims one row at a time with `UPDATE runs SET status='running', claimed_at=? WHERE id=? AND status='queued'` + `RowsAffected==1`. Calls a thin wrapper around `personas.RunAndPersist`. Writes the terminal status. Inserts the retry row if applicable. **After every terminal write (succeeded / skipped / failed / failed_permanent), updates `agents.last_run_at = completed_at`** so the loop's due-check stays accurate without scanning `runs`.

### Failure classification

```go
type FailureClass int

const (
    FailureUnknown FailureClass = iota
    FailureTransient   // retry with backoff
    FailurePermanent   // no retry; surface to operator
)

// classify maps an error from personas.RunAndPersist to a class + reason.
func classify(err error) (FailureClass, string)
```

Transient: `context.DeadlineExceeded`, `context.Canceled`, `mcp.ErrSessionLost`, `mcp.ErrTransport`, `*llm.RateLimitError`, unknown errors (conservative default).

Permanent: `*pep.DeniedError`, `manifest.ErrAbilityNotInManifest`, `secrets.ErrMissingProviderKey`.

The sentinel errors don't all exist yet — introducing them at source is its own commit (step 2 below).

### Migration `010_runs_scheduler.sql`

```sql
DROP TABLE IF EXISTS runs;

CREATE TABLE runs (
    id              TEXT PRIMARY KEY,
    persona         TEXT NOT NULL,
    trigger         TEXT NOT NULL,                 -- tick | manual | bootstrap | retry
    status          TEXT NOT NULL,                 -- queued | running | succeeded | skipped | failed | failed_permanent
    attempt         INTEGER NOT NULL DEFAULT 1,
    retry_of        TEXT,                          -- runs.id of the previous attempt
    scheduled_at    TEXT NOT NULL,
    claimed_at      TEXT,
    completed_at    TEXT,
    latency_ms      INTEGER,
    issue_id        TEXT,
    turn_id         TEXT,
    skip_reason     TEXT,
    failure_reason  TEXT,
    failure_class   TEXT,                          -- transient | permanent
    created_at      TEXT NOT NULL,
    FOREIGN KEY (persona)  REFERENCES agents(persona)     ON DELETE CASCADE,
    FOREIGN KEY (issue_id) REFERENCES issues(id)          ON DELETE SET NULL,
    FOREIGN KEY (turn_id)  REFERENCES turn_events(turn_id) ON DELETE SET NULL,
    FOREIGN KEY (retry_of) REFERENCES runs(id)            ON DELETE SET NULL
);

CREATE INDEX idx_runs_persona_status   ON runs(persona, status);
CREATE INDEX idx_runs_scheduled        ON runs(status, scheduled_at) WHERE status = 'queued';
CREATE INDEX idx_runs_issue            ON runs(issue_id);

ALTER TABLE agents ADD COLUMN cadence_seconds INTEGER NOT NULL DEFAULT 21600;  -- 6h
ALTER TABLE agents ADD COLUMN max_attempts    INTEGER NOT NULL DEFAULT 3;
ALTER TABLE agents ADD COLUMN last_run_at     TEXT;
```

### State machine

```
queued ──claim──▶ running ──┬─▶ succeeded
                            ├─▶ skipped
                            ├─▶ failed   ──(attempt<max, transient)──▶ NEW queued (retry_of=parent, scheduled_at=now+backoff)
                            └─▶ failed_permanent
```

`failed` is terminal for its row; retries are new rows linked via `retry_of`. The UI collapses a chain to one row with `attempt N of M`.

### Backoff

| Attempt fails | Delay before retry |
|---------------|--------------------|
| 1             | 5 min              |
| 2             | 30 min             |
| 3             | 2 h                |
| 4+            | `failed_permanent`, no retry |

### Skip-row policy

When the scheduler considers a persona at tick time and chooses not to run, write a `skipped` row **only when cadence would have triggered a run** (i.e., `last_run_at + cadence <= now`). This bounds row count by cadence rather than tick interval and gives operators meaningful "would have run, but…" visibility. Below-cadence ticks write nothing.

### HTTP API

```
GET  /v1/runs                 List with filters: persona, status, issue_id. Cursor pagination, default limit 50.
GET  /v1/runs/:id             Single run + turn_event + retry_chain.
POST /v1/runs                 Body: { persona }. Enqueues with trigger='manual', scheduled_at=now. 409 if disabled/unknown.
GET  /v1/agents               (extended) cadence_seconds, max_attempts, last_run_at, next_run_at.
```

`Run` wire shape:

```ts
type Run = {
  id: string;
  persona: string;
  trigger: 'tick' | 'manual' | 'bootstrap' | 'retry';
  status: 'queued' | 'running' | 'succeeded' | 'skipped' | 'failed' | 'failed_permanent';
  attempt: number;
  retryOf: string | null;
  scheduledAt: string;        // RFC3339
  claimedAt: string | null;
  completedAt: string | null;
  latencyMs: number | null;
  issueId: string | null;
  turnId: string | null;
  skipReason: string | null;
  failureReason: string | null;
  failureClass: 'transient' | 'permanent' | null;
};
```

### UI

New top-level `/runs` route, reachable from a new left-nav "Activity" group placed below the agent list (operator-debug surface, not promoted to the same tier as Kanban — match Nevena's design when it lands).

- **`/runs`.** `Page` header + `DataViews` table. Columns: persona (with `PersonaAvatar`), trigger, status badge (`succeeded`→stable, `skipped`→informational, `failed`→medium, `failed_permanent`→high, `queued`/`running`→low), started-at relative, latency, issue link. Filters: persona multi-select, status multi-select. Sort: `scheduledAt` DESC. Poll every 5s while foregrounded. Header shows `N runs queued · M running` from a cheap aggregate query.
- **`/runs/:id`.** Three stacked sections. (1) Header card: persona, trigger, status badge, scheduled/claimed/completed times, latency, `failure_reason` / `skip_reason`, link to `/issues/:id` if set. (2) Trace: two `CollapsibleCard`s — model calls (provider, model, tokens, cost), skill calls (name, version, status, latency). (3) Retry chain when applicable.
- **`IssueDetail`.** New collapsed "Runs" `CollapsibleCard` between diff and verdict. Lists every run with `issue_id` matching, newest first.
- **`/agents`.** Add a per-row "Run now" `Button` → `POST /v1/runs` → redirect to `/runs/:id`.

Edge cases the UI must handle clearly:
- `failed_permanent` due to missing API key → link to Settings → Providers with the Configure CTA highlighted.
- `skipped` because `HasOpenWork` → "already has N open proposals" with a link to the kanban filtered to that persona.

## Implementation order

Eight commits. Each ends with a working daemon and a green test suite.

1. **Migration + Go types.** `010_runs_scheduler.sql`, `daemon/internal/scheduler/types.go` (`Run`, `Status`, `Trigger`, `FailureClass`). No callers yet. Tests: migration applies cleanly fresh and against a populated v0.1 DB; the `runs` drop leaves no FK orphans elsewhere.
2. **Sentinel errors at source.** Introduce `mcp.ErrSessionLost`, `mcp.ErrTransport`, `llm.RateLimitError`, `pep.DeniedError`, `secrets.ErrMissingProviderKey`, `manifest.ErrAbilityNotInManifest`. Replace the `fmt.Errorf` returns at the relevant call sites. Pure rewrite, no behavior change. Tests assert each sentinel fires under the right condition.
3. **`classify`.** Pure function, 100% branch coverage. Table-driven test: one row per sentinel, plus the conservative default and the context-canceled case.
4. **Worker.** Single-goroutine claim-and-run loop. Uses an in-memory persona stub (already supported by `personas.Register`) to drive every (outcome × retry remaining × failure class) cell. Race-hardened claim test: two goroutines call `claim(run_id)`; exactly one wins.
5. **Loop.** 60s ticker with a fake clock. Tests `tickOnce(t)` against every selection branch — due / not due, `HasOpenWork` skip row, bootstrap on first start.
6. **Wire into `cli/run.go`.** Replace `go runPersonas(...)`. Rename `--skip-personas` → `--skip-scheduler` (keep old flag as deprecated alias for one release). Smoke test against the live staging store: daemon boots, scheduler ticks, `POST /v1/runs` produces a `succeeded` run end-to-end.
7. **HTTP handlers.** `GET /v1/runs`, `GET /v1/runs/:id`, `POST /v1/runs`, agents extension. Existing httptest harness. Asserts wire shapes, cursor round-trip, 409 paths, `trigger='manual'` insertion.
8. **UI.** `/runs` + `/runs/:id` + IssueDetail integration + `/agents` "Run now" button. WPDS-only per `DESIGN.md`. Visual review checklist: badge intents, persona avatars, polling cadence, manual-trigger CTA placement. Smoke test against staging store: trigger a Marketing run, watch the state transitions live.

## Test plan

| Layer | Test shape |
|-------|-----------|
| Migration | `010_runs_scheduler.sql` applies fresh and against a populated v0.1 DB. New indexes are used by the queue query (`EXPLAIN QUERY PLAN`). |
| `classify` | Table-driven. One row per sentinel + default + cancellation. |
| Worker | State machine matrix: every (input outcome × retry remaining × failure class) cell. |
| Worker claim race | Two goroutines, one winner, `ErrAlreadyClaimed` for the loser. Same pattern as the batch-approve claim. |
| Loop | Fake clock; `tickOnce` enqueues the right rows at the right `scheduled_at`. Covers `HasOpenWork` skip-row path and bootstrap path. |
| API | httptest. Filter combinations, cursor round-trip, manual trigger inserts row, 409 paths, agents response gains the new fields. |
| UI smoke | Staging store. Trigger a Marketing run; see queued → running → succeeded in `/runs`; click into the trace; verify the issue link and the kanban link from a `skipped` row. |

## Risks and mitigations

- **Sentinel error rollout (step 2) is a wide-but-shallow rewrite.** Mitigation: keep step 2 strictly behavior-preserving — its only job is to swap `fmt.Errorf` for typed errors. Land it before any scheduler code so `classify` has something to match on.
- **Skip-row noise.** Cadence-bounded skip rows (only at would-have-run ticks) keep the table small. If real-world data shows growth issues, retention is the planned follow-up.
- **`HasOpenWork` semantics changing from silent-no-op to visible-skip is a small behavior change.** Today an operator sees a stale kanban with no explanation. New behavior: same stale kanban, but operators can now see "would have run, but already has 2 open proposals." Net positive; no regression.
- **Single-worker bottleneck.** A long-running persona blocks all others until done. Acceptable for V1 (MCP session-id constraint forces it anyway); flagged for the parallel-workers follow-up.

## Follow-ups (filed in Linear, WooAgent OS project, team WOOAI)

- Concurrent workers when MCP can hold multiple sessions.
- `POST /v1/runs/:id/cancel`.
- Retention policy for `runs` rows.
- Webhook-driven runs (after Companion Plugin v0.2).
- Per-run aggregate cost budget.
- UI design pass on the run-log panel with Nevena once a first implementation is browsable.
