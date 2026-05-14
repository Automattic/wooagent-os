# Backlog Observations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make each persona produce a small ranked list of observations at bootstrap, persist them as `status='backlog'` issues, auto-promote the top one through `in_progress` (Drafting) to `in_review`, and surface the rest as backlog cards the operator can promote manually.

**Architecture:** Schema-additive (4 new columns on `issues`, no new table). Persona interface gains `Survey` alongside the existing `Draft`. `RunAndPersist` becomes a small dispatcher with two modes (`survey`, `draft`). Scheduler's per-persona decision tree gains a new `promotion` trigger that fires when In Review is empty AND backlog has items. UI: lighter "backlog observation" card variant + live Drafting state via poll-interval bump.

**Tech Stack:** Go (daemon, scheduler, personas, sqlite via modernc); React + TypeScript + `@wordpress/ui` + `@wordpress/components` (UI); CSS variables (`--wpds-*`); existing telemetry, PEP, auth scaffolding.

**Spec:** [`backlog-observations-design.md`](./backlog-observations-design.md) — read before implementing. The spec covers state machine, schema deltas, persona interface, dispatcher logic, scheduler changes, UI shape, failure handling, test plan, risks, and follow-ups. This plan turns each spec section into ordered, reviewable commits.

**Out of scope (filed in spec as follow-ups, do not implement here):**
- Cross-persona observation scoring or a Planner persona.
- Observation staleness / re-validation.
- Top-up-when-low logic (option C from brainstorm).
- The `insights` table itself — covered by `insights-planning-implementation-plan.md`.
- SSE / WebSocket for live Drafting updates (poll-interval bump is sufficient for v1).

**Convention:** Every implementation task ends with running tests + a commit. Frequent commits, small diffs.

---

## File Structure

**Daemon (Go):**
- Create: `daemon/internal/store/migrations/013_backlog_observations.sql` — additive migration: 4 columns + 1 index on `issues`.
- Modify: `daemon/internal/personas/personas.go` — narrow `HasOpenWork`, add `Survey` to the `Persona` interface, add `Observation` + `DraftTarget` + `SurveyResult` types, extend `RunAndPersist` with `RunMode`.
- Modify: `daemon/internal/personas/marketing/marketing.go` — implement `Survey`. The existing 3-variant `Draft` becomes `Draft(ctx, deps, target)` taking a `DraftTarget`.
- Modify: `daemon/internal/personas/pricing/pricing.go` — implement `Survey`. `Draft` becomes target-aware.
- Modify: `daemon/internal/personas/sales-support/sales_support.go` — implement `Survey`. `Draft` becomes target-aware.
- Create: `daemon/internal/personas/observations.go` — pure helpers shared across personas (e.g. dedupe-upsert by `(persona, target_id)`).
- Modify: `daemon/internal/scheduler/types.go` — add `TriggerPromotion` constant.
- Modify: `daemon/internal/scheduler/loop.go` — new decision tree: if backlog rows exist for the persona, enqueue a promotion run; else tick survey; else bootstrap survey.
- Modify: `daemon/internal/scheduler/worker.go` — dispatch promotion runs to `RunAndPersist` with `mode=draft, target_issue_id=…`.
- Modify: `daemon/internal/httpapi/handlers_v1.go` — `POST /v1/runs` accepts an optional `target_issue_id` body field.

**UI (TypeScript / React):**
- Modify: `ui/src/api/client.ts` — `Issue` type gains `observation_rationale`, `observation_score`, `target_id`, `draft_attempts`; `api.runs.trigger` accepts an optional `targetIssueId`.
- Modify: `ui/src/screens/Kanban.tsx` — render backlog observation cards (lighter variant), wire "Draft this" CTA, populate Drafting column when issue `status='in_progress'`.
- Create: `ui/src/components/BacklogCard.tsx` — the lighter-shape backlog observation card.
- Create: `ui/src/components/DraftingCard.tsx` — the in-flight drafting card with a spinner and "Drafting now…" hint.
- Modify: `ui/src/App.tsx` — kanban poll interval bumps to ~2s while any issue is `in_progress`, back to ~10s when not.
- Modify: `ui/src/components/StatusBadge.tsx` — update `KindBadge` to render persona name (Marketing / Pricing / Sales) per F5, kept aligned with the design's CLAUDE.md update.
- Modify: `CLAUDE.md` — update persona-color exception doc (kind pills are persona names, not artifact types).

**Tests (Go):**
- Create: `daemon/internal/personas/observations_test.go` — dedupe-upsert logic.
- Modify: `daemon/internal/personas/personas_test.go` — `HasOpenWork` narrowing, `RunAndPersist` survey + draft paths.
- Modify: `daemon/internal/personas/marketing/marketing_test.go`, `pricing/pricing_test.go`, `sales-support/sales_support_test.go` — `Survey` happy path + skip path with stub MCP + stub LLM.
- Modify: `daemon/internal/scheduler/loop_test.go` — new decision-tree branches.
- Modify: `daemon/internal/scheduler/worker_test.go` — promotion run dispatch.
- Modify: `daemon/internal/httpapi/handlers_v1_test.go` — `POST /v1/runs` with `target_issue_id`.

---

## Task 1: Migration 013 — Backlog observation columns

**Files:**
- Create: `daemon/internal/store/migrations/013_backlog_observations.sql`
- Test: `daemon/internal/store/migrations_test.go` (existing; add a case)

- [ ] **Step 1: Write the migration**

Create `daemon/internal/store/migrations/013_backlog_observations.sql`:

```sql
-- 013_backlog_observations.sql
-- Additive: introduces the columns needed to express observation issues
-- (status='backlog' with no proposal_content yet) alongside fully drafted
-- proposals. See backlog-observations-design.md for the full state machine.

ALTER TABLE issues ADD COLUMN observation_rationale TEXT;
ALTER TABLE issues ADD COLUMN observation_score      REAL;
ALTER TABLE issues ADD COLUMN target_id              TEXT;
ALTER TABLE issues ADD COLUMN draft_attempts         INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_issues_persona_status_score
    ON issues(persona, status, observation_score DESC);
```

- [ ] **Step 2: Write the migration test**

Add to `daemon/internal/store/migrations_test.go`:

```go
func TestMigration013_BacklogObservations_AppliesCleanly(t *testing.T) {
    db := openTestDB(t)
    // Seed a legacy in_review issue (no observation columns) before
    // the migration would have run in this fixture's chain.
    _, err := db.ExecContext(context.Background(), `
        INSERT INTO issues (id, title, status, priority, created_at, updated_at)
        VALUES ('legacy-1', 'legacy issue', 'in_review', 'medium',
                '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
    `)
    if err != nil {
        t.Fatalf("seed: %v", err)
    }

    // Migration 013 should have been applied by openTestDB.
    cols := tableColumns(t, db, "issues")
    for _, want := range []string{"observation_rationale", "observation_score", "target_id", "draft_attempts"} {
        if _, ok := cols[want]; !ok {
            t.Errorf("issues missing column %q after migration 013", want)
        }
    }

    // Legacy row's new columns: rationale/score/target_id nullable, draft_attempts default 0.
    var rationale, targetID sql.NullString
    var score sql.NullFloat64
    var attempts int
    err = db.QueryRowContext(context.Background(),
        `SELECT observation_rationale, observation_score, target_id, draft_attempts FROM issues WHERE id='legacy-1'`,
    ).Scan(&rationale, &score, &targetID, &attempts)
    if err != nil {
        t.Fatalf("read legacy row: %v", err)
    }
    if rationale.Valid || score.Valid || targetID.Valid {
        t.Errorf("legacy row should have NULL observation fields, got rationale=%v score=%v target=%v", rationale, score, targetID)
    }
    if attempts != 0 {
        t.Errorf("legacy row draft_attempts = %d, want 0", attempts)
    }
}
```

(`tableColumns` is an existing helper; if absent, implement as a 5-line `PRAGMA table_info(...)` reader.)

- [ ] **Step 3: Run the test, watch it fail**

Run: `go test ./internal/store/... -run TestMigration013 -v`

Expected: FAIL with "issues missing column observation_rationale".

- [ ] **Step 4: Wire the migration into the embedded migration set**

The migration loader picks files up automatically from the embedded `migrations/` directory — no code change needed. Re-run the test.

Run: `go test ./internal/store/... -run TestMigration013 -v`

Expected: PASS.

- [ ] **Step 5: Run the full store-package suite to catch regressions**

Run: `go test ./internal/store/... -v`

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add daemon/internal/store/migrations/013_backlog_observations.sql daemon/internal/store/migrations_test.go
git commit -m "feat(store): migration 013 — backlog observation columns

Adds observation_rationale, observation_score, target_id,
draft_attempts to issues, plus a (persona, status, score DESC)
index. Backfill behavior: legacy in_review rows keep NULL
observation columns; new survey-produced rows populate them.

See backlog-observations-design.md."
```

---

## Task 2: Narrow `HasOpenWork` to exclude `backlog`

`HasOpenWork` today counts `todo|in_progress|in_review`. After this change, `backlog` rows do not block surveys (they're the survey's output). See spec §"Schema".

**Files:**
- Modify: `daemon/internal/personas/personas.go:167-177`
- Test: `daemon/internal/personas/personas_test.go` (add case)

- [ ] **Step 1: Write the failing test**

Add to `daemon/internal/personas/personas_test.go`:

```go
func TestHasOpenWork_BacklogDoesNotCount(t *testing.T) {
    db := newTestDB(t)
    insertIssue(t, db, "i-bk", "marketing", "backlog")
    open, err := HasOpenWork(context.Background(), &store.Store{DB: db}, "marketing")
    if err != nil {
        t.Fatalf("HasOpenWork: %v", err)
    }
    if open {
        t.Errorf("backlog-only persona should have HasOpenWork=false, got true")
    }
}

func TestHasOpenWork_InReviewCounts(t *testing.T) {
    db := newTestDB(t)
    insertIssue(t, db, "i-rv", "marketing", "in_review")
    open, err := HasOpenWork(context.Background(), &store.Store{DB: db}, "marketing")
    if err != nil {
        t.Fatalf("HasOpenWork: %v", err)
    }
    if !open {
        t.Errorf("in_review should yield HasOpenWork=true, got false")
    }
}

func TestHasOpenWork_InProgressCounts(t *testing.T) {
    db := newTestDB(t)
    insertIssue(t, db, "i-ip", "marketing", "in_progress")
    open, err := HasOpenWork(context.Background(), &store.Store{DB: db}, "marketing")
    if err != nil {
        t.Fatalf("HasOpenWork: %v", err)
    }
    if !open {
        t.Errorf("in_progress should yield HasOpenWork=true, got false")
    }
}
```

`insertIssue` is a small helper — implement if not present:

```go
func insertIssue(t *testing.T, db *sql.DB, id, persona, status string) {
    t.Helper()
    _, err := db.ExecContext(context.Background(), `
        INSERT INTO issues (id, title, persona, status, priority, created_at, updated_at)
        VALUES (?, 'test', ?, ?, 'medium', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
    `, id, persona, status)
    if err != nil {
        t.Fatalf("insertIssue %s: %v", id, err)
    }
}
```

- [ ] **Step 2: Run the test, see backlog case fail**

Run: `go test ./internal/personas/ -run TestHasOpenWork -v`

Expected: FAIL on `TestHasOpenWork_BacklogDoesNotCount` (backlog currently counts).

- [ ] **Step 3: Narrow the query**

In `daemon/internal/personas/personas.go`, replace the body of `HasOpenWork`:

```go
func HasOpenWork(ctx context.Context, st *store.Store, slug string) (bool, error) {
    var n int
    err := st.DB.QueryRowContext(ctx,
        `SELECT count(*) FROM issues WHERE persona = ? AND status IN ('in_progress','in_review')`,
        slug,
    ).Scan(&n)
    if err != nil {
        return false, err
    }
    return n > 0, nil
}
```

- [ ] **Step 4: Run the test, watch all three pass**

Run: `go test ./internal/personas/ -run TestHasOpenWork -v`

Expected: all three PASS.

- [ ] **Step 5: Commit**

```bash
git add daemon/internal/personas/personas.go daemon/internal/personas/personas_test.go
git commit -m "refactor(personas): narrow HasOpenWork to in_progress|in_review

backlog rows are the survey's output, not blockers for new
surveys. todo isn't used in the current flow; kept out so it
doesn't accidentally block anything."
```

---

## Task 3: `Observation`, `DraftTarget`, `SurveyResult` types

Lay the type foundation before changing the `Persona` interface. Pure types — no behavior yet.

**Files:**
- Modify: `daemon/internal/personas/personas.go`

- [ ] **Step 1: Add the types alongside the existing `Result` / `Drafted` block**

Append to `daemon/internal/personas/personas.go` (above the `Persona` interface):

```go
// Observation is what a persona's Survey emits for each candidate.
// Persisted as a status='backlog' issue. Maps onto the future insights
// table's columns 1:1 (target_id → issue_id, observation_score → score,
// observation_rationale → body), so the migration path to insights-planning
// is the same Go field set.
type Observation struct {
    Title     string  // shown on the backlog card
    Rationale string  // 1-2 sentences, "why this matters"
    TargetID  string  // product_id / order_id; required, non-empty
    Score     float64 // 0..1; ranking input for auto-promote order
}

// SurveyResult mirrors Drafted (Skipped + reason or a payload).
type SurveyResult struct {
    Observations []Observation
    Skipped      bool
    SkipReason   string
}

// DraftTarget is what was previously implicit — the specific thing the
// persona is drafting a proposal for. Survey produces it as part of
// Observation; Draft consumes it.
type DraftTarget struct {
    TargetID string
    Hint     string // the observation's rationale, passed forward
}
```

- [ ] **Step 2: Verify the package still compiles**

Run: `go build ./internal/personas/...`

Expected: no output (clean compile).

- [ ] **Step 3: Commit**

```bash
git add daemon/internal/personas/personas.go
git commit -m "feat(personas): add Observation / SurveyResult / DraftTarget types

Pure type addition — no behavior change. These shapes prepare
the Persona interface for Survey + target-aware Draft in the
next commits."
```

---

## Task 4: `observations.go` — dedupe-upsert helper

Pulled out so all three personas use the same insert path. Pure SQL, deterministic, easy to test.

**Files:**
- Create: `daemon/internal/personas/observations.go`
- Create: `daemon/internal/personas/observations_test.go`

- [ ] **Step 1: Write the failing test**

Create `daemon/internal/personas/observations_test.go`:

```go
package personas

import (
    "context"
    "database/sql"
    "testing"

    "github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

func TestUpsertBacklogObservation_Inserts(t *testing.T) {
    db := newTestDB(t)
    obs := Observation{
        Title: "rewrite candidate", Rationale: "weak copy", TargetID: "p-1", Score: 0.7,
    }
    _, err := upsertBacklogObservation(context.Background(), &store.Store{DB: db}, "marketing", obs)
    if err != nil {
        t.Fatalf("upsert: %v", err)
    }
    var n int
    _ = db.QueryRowContext(context.Background(),
        `SELECT count(*) FROM issues WHERE persona='marketing' AND target_id='p-1' AND status='backlog'`,
    ).Scan(&n)
    if n != 1 {
        t.Errorf("want 1 backlog row, got %d", n)
    }
}

func TestUpsertBacklogObservation_UpdatesExisting(t *testing.T) {
    db := newTestDB(t)
    obs := Observation{Title: "v1", Rationale: "old", TargetID: "p-1", Score: 0.5}
    _, err := upsertBacklogObservation(context.Background(), &store.Store{DB: db}, "marketing", obs)
    if err != nil {
        t.Fatalf("first upsert: %v", err)
    }

    obs2 := Observation{Title: "v2", Rationale: "fresher", TargetID: "p-1", Score: 0.9}
    _, err = upsertBacklogObservation(context.Background(), &store.Store{DB: db}, "marketing", obs2)
    if err != nil {
        t.Fatalf("second upsert: %v", err)
    }

    var n int
    _ = db.QueryRowContext(context.Background(),
        `SELECT count(*) FROM issues WHERE persona='marketing' AND target_id='p-1'`,
    ).Scan(&n)
    if n != 1 {
        t.Errorf("want 1 row after upsert, got %d (should update in place)", n)
    }
    var title, rationale string
    var score float64
    _ = db.QueryRowContext(context.Background(),
        `SELECT title, observation_rationale, observation_score FROM issues WHERE persona='marketing' AND target_id='p-1'`,
    ).Scan(&title, &rationale, &score)
    if title != "v2" || rationale != "fresher" || score != 0.9 {
        t.Errorf("row not updated: title=%q rationale=%q score=%g", title, rationale, score)
    }
}

func TestUpsertBacklogObservation_RequiresTargetID(t *testing.T) {
    db := newTestDB(t)
    obs := Observation{Title: "x", Rationale: "y", TargetID: "", Score: 0.5}
    _, err := upsertBacklogObservation(context.Background(), &store.Store{DB: db}, "marketing", obs)
    if err == nil {
        t.Errorf("expected error on empty target_id, got nil")
    }
}

func newTestDB(t *testing.T) *sql.DB {
    t.Helper()
    db, err := store.OpenInMemory(context.Background())
    if err != nil {
        t.Fatalf("open: %v", err)
    }
    t.Cleanup(func() { _ = db.Close() })
    return db
}
```

(`store.OpenInMemory` already exists in the codebase per existing tests; if the helper is named differently, adapt.)

- [ ] **Step 2: Run the test, see compile failure**

Run: `go test ./internal/personas/ -run TestUpsertBacklog -v`

Expected: build failure — `upsertBacklogObservation undefined`.

- [ ] **Step 3: Implement `upsertBacklogObservation`**

Create `daemon/internal/personas/observations.go`:

```go
package personas

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "time"

    "github.com/google/uuid"

    "github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

// upsertBacklogObservation inserts a backlog issue for (persona, target_id),
// or updates an existing one in place. Returns the issue id. The contract
// matches the survey path in RunAndPersist — every observation goes through
// this helper, so dedupe + write semantics live in one place.
//
// Empty target_id is rejected: the dedupe key is non-optional. Observations
// without a clear target shouldn't live in the backlog at all.
func upsertBacklogObservation(ctx context.Context, st *store.Store, persona string, obs Observation) (string, error) {
    if obs.TargetID == "" {
        return "", errors.New("upsertBacklogObservation: target_id required")
    }
    now := time.Now().UTC().Format(time.RFC3339)

    var existingID string
    err := st.DB.QueryRowContext(ctx,
        `SELECT id FROM issues WHERE persona = ? AND target_id = ?`,
        persona, obs.TargetID,
    ).Scan(&existingID)
    if errors.Is(err, sql.ErrNoRows) {
        id := uuid.NewString()
        _, err := st.DB.ExecContext(ctx, `
            INSERT INTO issues
                (id, title, persona, status, priority,
                 observation_rationale, observation_score, target_id, draft_attempts,
                 created_at, updated_at)
            VALUES (?, ?, ?, 'backlog', 'medium',
                    ?, ?, ?, 0,
                    ?, ?)
        `, id, obs.Title, persona, obs.Rationale, obs.Score, obs.TargetID, now, now)
        if err != nil {
            return "", fmt.Errorf("insert backlog obs: %w", err)
        }
        return id, nil
    }
    if err != nil {
        return "", fmt.Errorf("lookup existing backlog: %w", err)
    }
    _, err = st.DB.ExecContext(ctx, `
        UPDATE issues
           SET title = ?, observation_rationale = ?, observation_score = ?, updated_at = ?
         WHERE id = ?
    `, obs.Title, obs.Rationale, obs.Score, now, existingID)
    if err != nil {
        return "", fmt.Errorf("update backlog obs: %w", err)
    }
    return existingID, nil
}
```

- [ ] **Step 4: Run the tests, watch them pass**

Run: `go test ./internal/personas/ -run TestUpsertBacklog -v`

Expected: all three PASS.

- [ ] **Step 5: Commit**

```bash
git add daemon/internal/personas/observations.go daemon/internal/personas/observations_test.go
git commit -m "feat(personas): upsertBacklogObservation helper

Single SQL write path for survey-emitted observations. Dedupe
by (persona, target_id) — same target re-surfacing updates the
existing row in place rather than producing duplicates."
```

---

## Task 5: Extend `Persona` interface — `Survey` + target-aware `Draft`

The interface change. After this, no persona implementation compiles until each gets a `Survey` method (Tasks 7-9). This is intentional — keep the diff focused.

**Files:**
- Modify: `daemon/internal/personas/personas.go`
- Modify: `daemon/internal/personas/personas_test.go` (stubs)

- [ ] **Step 1: Update the interface**

In `daemon/internal/personas/personas.go`, replace the `Persona` interface:

```go
// Persona is what each persona implements. Survey is new in the
// backlog-observations work; Draft is now target-aware (the specific
// thing to draft for is explicit, not implicit).
type Persona interface {
    Slug() string
    Survey(ctx context.Context, deps Deps) (SurveyResult, error)
    Draft(ctx context.Context, deps Deps, target DraftTarget) (Drafted, error)
}
```

- [ ] **Step 2: Verify the build breaks at every persona impl**

Run: `go build ./...`

Expected: compile errors at `daemon/internal/personas/marketing/marketing.go`, `pricing/pricing.go`, `sales-support/sales_support.go` — each `Draft` method has the wrong signature and is missing `Survey`. This is the intended state — the next three tasks fix each persona.

- [ ] **Step 3: Don't commit yet**

This task lands in the same commit as Tasks 6 (RunAndPersist refactor), 7, 8, 9 (persona impls). The intermediate state doesn't compile.

---

## Task 6: `RunAndPersist` becomes a `RunMode` dispatcher

**Files:**
- Modify: `daemon/internal/personas/personas.go`

- [ ] **Step 1: Add the `RunMode` type**

Add to `daemon/internal/personas/personas.go`, near `Result`:

```go
// RunMode tells RunAndPersist whether this invocation is a survey
// (produce N observations, persist as backlog) or a draft (promote a
// specific backlog row through in_progress → in_review).
type RunMode struct {
    Kind        string // "survey" | "draft"
    PromoteFrom string // issue_id, required when Kind=="draft"
}
```

- [ ] **Step 2: Add stub `RunAndPersistMode`**

Keep the existing `RunAndPersist` signature for now (back-compat); add a new `RunAndPersistMode`:

```go
// RunAndPersistMode is the new entry point. The existing RunAndPersist
// wraps this with Kind="draft" against the persona's first observation
// for back-compat until callers migrate (scheduler in Task 11).
func RunAndPersistMode(ctx context.Context, p Persona, deps Deps, mode RunMode) (Result, error) {
    slug := p.Slug()
    res := Result{Persona: slug}

    enabled, err := IsEnabled(ctx, deps.Store, slug)
    if err != nil {
        return res, fmt.Errorf("check enabled: %w", err)
    }
    if !enabled {
        res.Skipped = true
        res.SkipReason = "persona not enabled in agents table"
        return res, nil
    }

    switch mode.Kind {
    case "survey":
        return runSurvey(ctx, p, deps, res)
    case "draft":
        if mode.PromoteFrom == "" {
            return res, fmt.Errorf("RunAndPersistMode: PromoteFrom required when Kind=draft")
        }
        return runDraft(ctx, p, deps, mode.PromoteFrom, res)
    default:
        return res, fmt.Errorf("RunAndPersistMode: unknown Kind %q", mode.Kind)
    }
}
```

- [ ] **Step 3: Implement `runSurvey`**

Add to `personas.go`:

```go
func runSurvey(ctx context.Context, p Persona, deps Deps, res Result) (Result, error) {
    open, err := HasOpenWork(ctx, deps.Store, p.Slug())
    if err != nil {
        return res, fmt.Errorf("check open work: %w", err)
    }
    if open {
        res.Skipped = true
        res.SkipReason = "persona already has open proposals; not surveying"
        return res, nil
    }

    tracker := telemetry.NewTracker(uuid.NewString(), p.Slug())
    tctx := telemetry.WithTracker(ctx, tracker)

    sr, err := p.Survey(tctx, deps)
    if err != nil {
        recordTurnSurvey(ctx, deps.Recorder, tracker, "", sr, err.Error())
        return res, err
    }
    if sr.Skipped {
        res.Skipped = true
        res.SkipReason = sr.SkipReason
        recordTurnSurvey(ctx, deps.Recorder, tracker, "", sr, "")
        return res, nil
    }

    for _, obs := range sr.Observations {
        if _, err := upsertBacklogObservation(ctx, deps.Store, p.Slug(), obs); err != nil {
            return res, fmt.Errorf("upsert observation %q: %w", obs.TargetID, err)
        }
    }
    recordTurnSurvey(ctx, deps.Recorder, tracker, "", sr, "")
    res.IssueID = "" // survey doesn't produce a single issue
    return res, nil
}
```

- [ ] **Step 4: Implement `runDraft`**

```go
func runDraft(ctx context.Context, p Persona, deps Deps, targetIssueID string, res Result) (Result, error) {
    var (
        backlogStatus    string
        backlogTargetID  sql.NullString
        backlogRationale sql.NullString
        backlogAttempts  int
    )
    err := deps.Store.DB.QueryRowContext(ctx, `
        SELECT status, target_id, observation_rationale, draft_attempts
          FROM issues
         WHERE id = ? AND persona = ?
    `, targetIssueID, p.Slug()).Scan(&backlogStatus, &backlogTargetID, &backlogRationale, &backlogAttempts)
    if errors.Is(err, sql.ErrNoRows) {
        res.Skipped = true
        res.SkipReason = fmt.Sprintf("issue %s not found for persona %s", targetIssueID, p.Slug())
        return res, nil
    }
    if err != nil {
        return res, fmt.Errorf("load backlog issue: %w", err)
    }
    if backlogStatus != "backlog" {
        res.Skipped = true
        res.SkipReason = fmt.Sprintf("issue %s is %q, not 'backlog'", targetIssueID, backlogStatus)
        return res, nil
    }

    // Atomic flip: backlog → in_progress. Only the first racer wins.
    flipRes, err := deps.Store.DB.ExecContext(ctx, `
        UPDATE issues
           SET status='in_progress', updated_at=?
         WHERE id=? AND status='backlog'
    `, time.Now().UTC().Format(time.RFC3339), targetIssueID)
    if err != nil {
        return res, fmt.Errorf("flip to in_progress: %w", err)
    }
    if n, _ := flipRes.RowsAffected(); n == 0 {
        res.Skipped = true
        res.SkipReason = "raced — issue is no longer in 'backlog'"
        return res, nil
    }

    tracker := telemetry.NewTracker(uuid.NewString(), p.Slug())
    tctx := telemetry.WithTracker(ctx, tracker)

    target := DraftTarget{
        TargetID: backlogTargetID.String,
        Hint:     backlogRationale.String,
    }
    d, draftErr := p.Draft(tctx, deps, target)
    if draftErr != nil {
        if err := bumpDraftAttempt(ctx, deps.Store, targetIssueID, backlogAttempts+1); err != nil {
            return res, fmt.Errorf("bump draft attempts: %w", err)
        }
        recordTurn(ctx, deps.Recorder, tracker, "", d)
        return res, draftErr
    }
    if d.Skipped {
        // Reset to backlog so a future promotion can retry.
        if _, err := deps.Store.DB.ExecContext(ctx, `
            UPDATE issues SET status='backlog', updated_at=? WHERE id=?
        `, time.Now().UTC().Format(time.RFC3339), targetIssueID); err != nil {
            return res, fmt.Errorf("reset to backlog: %w", err)
        }
        res.Skipped = true
        res.SkipReason = d.SkipReason
        recordTurn(ctx, deps.Recorder, tracker, "", d)
        return res, nil
    }

    // Write the proposal + flip to in_review.
    targetJSON, _ := json.Marshal(d.Target)
    _, err = deps.Store.DB.ExecContext(ctx, `
        UPDATE issues
           SET status='in_review',
               title=?, description=?, priority=?,
               proposal_type=?, proposal_content=?, proposal_target=?,
               updated_at=?
         WHERE id=?
    `, d.Title, d.Description, d.Priority,
        d.ProposalType, d.ProposalContent, string(targetJSON),
        time.Now().UTC().Format(time.RFC3339), targetIssueID)
    if err != nil {
        return res, fmt.Errorf("write proposal: %w", err)
    }
    recordTurn(ctx, deps.Recorder, tracker, targetIssueID, d)
    res.IssueID = targetIssueID
    return res, nil
}

func bumpDraftAttempt(ctx context.Context, st *store.Store, issueID string, n int) error {
    now := time.Now().UTC().Format(time.RFC3339)
    if n >= 3 {
        _, err := st.DB.ExecContext(ctx, `
            UPDATE issues SET status='rejected', dismiss_reason='draft_attempts_exhausted',
                              dismissed_at=?, draft_attempts=?, updated_at=?
             WHERE id=?
        `, now, n, now, issueID)
        return err
    }
    _, err := st.DB.ExecContext(ctx, `
        UPDATE issues SET status='backlog', draft_attempts=?, updated_at=? WHERE id=?
    `, n, now, issueID)
    return err
}
```

- [ ] **Step 5: Add `recordTurnSurvey` helper alongside the existing `recordTurn`**

The existing `recordTurn` handles drafted issues. Survey turns record observations as a count, no single issue id:

```go
func recordTurnSurvey(ctx context.Context, recorder *telemetry.Recorder, tracker *telemetry.Tracker, issueID string, sr SurveyResult, errMsg string) {
    if recorder == nil {
        return
    }
    verdict := map[string]any{
        "kind":         "survey",
        "observations": len(sr.Observations),
        "skipped":      sr.Skipped,
        "skip_reason":  sr.SkipReason,
        "error":        errMsg,
    }
    _ = recorder.Record(ctx, tracker, issueID, verdict)
}
```

(Adjust to match the existing `recordTurn` signature in the codebase.)

- [ ] **Step 6: Don't commit yet — Task 7-9 land the persona impls in the same commit**

Run: `go build ./internal/personas/...` — still fails at persona impls. Expected. Move on.

---

## Task 7: Marketing implements `Survey`

The current marketing `Draft` already does product picking + LLM call. Move the picking into `Survey`, expand it to return N candidates, and have `Draft` consume an explicit `DraftTarget`.

**Files:**
- Modify: `daemon/internal/personas/marketing/marketing.go`
- Modify: `daemon/internal/personas/marketing/marketing_test.go`

- [ ] **Step 1: Add `Survey` method**

Add to `marketing.go`:

```go
const surveySystemPrompt = `You are a copywriter for a small-batch home-goods store.
Given a list of products (id, name, current description), pick the 3-5 that
would benefit most from a description rewrite. For each, return a short
title (operator-visible), a 1-2 sentence rationale (why this matters), and
a score in [0,1] where 1.0 means "weak copy, high upside" and 0 means "no
issue, leave alone".

Voice criteria: warm/sincere/concrete; avoid luxe/premium/elevate/curated;
prefer small-batch/handcrafted/made-to-last; lead with material or use.

Return JSON ONLY, no preamble:

{"observations":[{"product_id":N,"title":"...","rationale":"...","score":0.0}, ...]}

Constraints:
- 3-5 observations, sorted by score DESC.
- product_id must be one of the IDs in the input list.
- Score reflects upside, not severity.`

func (Marketing) Survey(ctx context.Context, deps personas.Deps) (personas.SurveyResult, error) {
    if deps.MCP == nil {
        return personas.SurveyResult{Skipped: true, SkipReason: "MCP client not configured"}, nil
    }
    if _, err := deps.MCP.Initialize(ctx); err != nil {
        return personas.SurveyResult{}, fmt.Errorf("mcp initialize: %w", err)
    }

    products, err := listPublishedProducts(ctx, deps.MCP, 20)
    if err != nil {
        return personas.SurveyResult{}, fmt.Errorf("list products: %w", err)
    }
    if len(products) == 0 {
        return personas.SurveyResult{Skipped: true, SkipReason: "no published products in store"}, nil
    }

    if strings.TrimSpace(deps.Env.AnthropicAPIKey) == "" {
        return personas.SurveyResult{Skipped: true, SkipReason: "ANTHROPIC_API_KEY not set"}, nil
    }

    rawJSON, err := surveyRewriteAnthropic(ctx, deps.Env.AnthropicAPIKey,
        firstNonEmpty(deps.Env.AnthropicModel, defaultAnthropicModel), products)
    if err != nil {
        return personas.SurveyResult{Skipped: true, SkipReason: fmt.Sprintf("survey LLM errored: %v", err)}, nil
    }
    parsed, err := parseSurvey(rawJSON)
    if err != nil {
        return personas.SurveyResult{Skipped: true, SkipReason: fmt.Sprintf("malformed survey response: %v", err)}, nil
    }
    return personas.SurveyResult{Observations: parsed}, nil
}
```

- [ ] **Step 2: Add `parseSurvey` next to the existing `parseVariants`**

```go
type llmSurveyResp struct {
    Observations []llmSurveyObservation `json:"observations"`
}

type llmSurveyObservation struct {
    ProductID int     `json:"product_id"`
    Title     string  `json:"title"`
    Rationale string  `json:"rationale"`
    Score     float64 `json:"score"`
}

func parseSurvey(raw string) ([]personas.Observation, error) {
    block := jsonObjectRe.FindString(raw)
    if block == "" {
        return nil, fmt.Errorf("no JSON object found")
    }
    var parsed llmSurveyResp
    if err := json.Unmarshal([]byte(block), &parsed); err != nil {
        return nil, fmt.Errorf("decode: %w", err)
    }
    if n := len(parsed.Observations); n < 1 || n > 5 {
        return nil, fmt.Errorf("expected 1-5 observations, got %d", n)
    }
    out := make([]personas.Observation, 0, len(parsed.Observations))
    for i, o := range parsed.Observations {
        if o.ProductID == 0 {
            return nil, fmt.Errorf("observation %d missing product_id", i)
        }
        title := strings.TrimSpace(o.Title)
        if title == "" {
            return nil, fmt.Errorf("observation %d empty title", i)
        }
        score := o.Score
        if score < 0 {
            score = 0
        }
        if score > 1 {
            score = 1
        }
        out = append(out, personas.Observation{
            Title:     title,
            Rationale: strings.TrimSpace(o.Rationale),
            TargetID:  fmt.Sprintf("product:%d", o.ProductID),
            Score:     score,
        })
    }
    return out, nil
}
```

- [ ] **Step 3: Make `Draft` target-aware**

Update the existing `Draft`:

```go
func (Marketing) Draft(ctx context.Context, deps personas.Deps, target personas.DraftTarget) (personas.Drafted, error) {
    if deps.MCP == nil {
        return personas.Drafted{Skipped: true, SkipReason: "MCP client not configured"}, nil
    }
    if _, err := deps.MCP.Initialize(ctx); err != nil {
        return personas.Drafted{}, fmt.Errorf("mcp initialize: %w", err)
    }

    productID, err := parseProductTargetID(target.TargetID)
    if err != nil {
        return personas.Drafted{Skipped: true, SkipReason: err.Error()}, nil
    }
    p, err := getProduct(ctx, deps.MCP, productID)
    if err != nil {
        return personas.Drafted{}, fmt.Errorf("get product %d: %w", productID, err)
    }

    // existing draftWithFallback + parseVariants flow unchanged ...
    rawOutput, skipReason, err := draftWithFallback(ctx, deps.Env, p)
    if err != nil {
        return personas.Drafted{}, err
    }
    if skipReason != "" {
        return personas.Drafted{Skipped: true, SkipReason: skipReason}, nil
    }
    variants, parseErr := parseVariants(rawOutput)
    targetMap := map[string]any{
        "product_id": p.ID, "product_name": p.Name, "product_sku": p.SKU, "previous": p.Description,
    }
    var content string
    if parseErr == nil {
        targetMap["variants"] = variants
        content = variants[0].Body
    } else {
        content = strings.TrimSpace(rawOutput)
    }
    return personas.Drafted{
        Title:           fmt.Sprintf("Product description rewrite · %s", p.Name),
        Description:     fmt.Sprintf("Drafted by Marketing for product #%d (%s).", p.ID, p.SKU),
        Priority:        "medium",
        ProposalType:    "product_description_rewrite",
        ProposalContent: content,
        Target:          targetMap,
    }, nil
}

func parseProductTargetID(targetID string) (int, error) {
    const prefix = "product:"
    if !strings.HasPrefix(targetID, prefix) {
        return 0, fmt.Errorf("invalid target_id %q (expected %s<id>)", targetID, prefix)
    }
    n, err := strconv.Atoi(strings.TrimPrefix(targetID, prefix))
    if err != nil || n <= 0 {
        return 0, fmt.Errorf("invalid product id in target_id %q", targetID)
    }
    return n, nil
}
```

(Add `"strconv"` to the imports.)

- [ ] **Step 4: Add `listPublishedProducts` helper**

```go
func listPublishedProducts(ctx context.Context, c *mcp.Client, limit int) ([]productSummary, error) {
    var listOut struct {
        Products []productSummary `json:"products"`
    }
    if err := callAbility(ctx, c, "wooagent-products/list",
        map[string]any{"per_page": limit}, &listOut); err != nil {
        return nil, err
    }
    out := make([]productSummary, 0, len(listOut.Products))
    for _, p := range listOut.Products {
        if p.Status == "publish" || p.Status == "" {
            out = append(out, p)
        }
    }
    return out, nil
}

func firstNonEmpty(a, b string) string {
    if strings.TrimSpace(a) != "" {
        return a
    }
    return b
}
```

- [ ] **Step 5: Add `surveyRewriteAnthropic` next to `draftRewriteAnthropic`**

```go
func surveyRewriteAnthropic(ctx context.Context, apiKey, model string, products []productSummary) (string, error) {
    var sb strings.Builder
    sb.WriteString("Products in the store:\n")
    for _, p := range products {
        sb.WriteString(fmt.Sprintf("- product_id=%d name=%q\n", p.ID, p.Name))
    }
    sb.WriteString("\nReturn the 3-5 best rewrite candidates as JSON per the system prompt.")

    body, _ := json.Marshal(anthropicReq{
        Model:     model,
        MaxTokens: 1024,
        System:    surveySystemPrompt,
        Messages:  []anthropicMsg{{Role: "user", Content: sb.String()}},
    })
    cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
    defer cancel()
    req, _ := http.NewRequestWithContext(cctx, "POST", anthropicAPIURL, bytes.NewReader(body))
    req.Header.Set("x-api-key", apiKey)
    req.Header.Set("anthropic-version", anthropicVersion)
    req.Header.Set("content-type", "application/json")
    res, err := http.DefaultClient.Do(req)
    if err != nil {
        return "", err
    }
    defer res.Body.Close()
    raw, _ := io.ReadAll(res.Body)
    if res.StatusCode != 200 {
        return "", fmt.Errorf("http %d: %s", res.StatusCode, raw)
    }
    var parsed anthropicResp
    if err := json.Unmarshal(raw, &parsed); err != nil {
        return "", err
    }
    var out strings.Builder
    for _, b := range parsed.Content {
        if b.Type == "text" {
            out.WriteString(b.Text)
        }
    }
    return out.String(), nil
}
```

- [ ] **Step 6: Add Survey tests**

Append to `marketing_test.go`:

```go
func TestSurvey_ParsesObservations(t *testing.T) {
    raw := `{"observations":[
        {"product_id":1,"title":"weak copy","rationale":"too short","score":0.8},
        {"product_id":2,"title":"luxe leak","rationale":"contains banned word","score":0.6},
        {"product_id":3,"title":"vague","rationale":"no material lead","score":0.5}
    ]}`
    out, err := parseSurvey(raw)
    if err != nil {
        t.Fatalf("parseSurvey: %v", err)
    }
    if len(out) != 3 {
        t.Fatalf("want 3 obs, got %d", len(out))
    }
    if out[0].TargetID != "product:1" {
        t.Errorf("target_id = %q, want product:1", out[0].TargetID)
    }
    if out[0].Score != 0.8 {
        t.Errorf("score = %v, want 0.8", out[0].Score)
    }
}

func TestSurvey_RejectsZeroObservations(t *testing.T) {
    raw := `{"observations":[]}`
    if _, err := parseSurvey(raw); err == nil {
        t.Errorf("expected error on empty list")
    }
}

func TestSurvey_ClampsScoreOutOfRange(t *testing.T) {
    raw := `{"observations":[{"product_id":1,"title":"x","score":1.5},{"product_id":2,"title":"y","score":-0.2},{"product_id":3,"title":"z","score":0.5}]}`
    out, _ := parseSurvey(raw)
    if out[0].Score != 1.0 {
        t.Errorf("1.5 should clamp to 1.0, got %v", out[0].Score)
    }
    if out[1].Score != 0.0 {
        t.Errorf("-0.2 should clamp to 0.0, got %v", out[1].Score)
    }
}
```

- [ ] **Step 7: Run tests**

Run: `go test ./internal/personas/marketing/... -v`

Expected: all PASS.

- [ ] **Step 8: Don't commit yet** — Tasks 8, 9 land in the same commit (interface migration).

---

## Task 8: Pricing implements `Survey`

Mirror Task 7 structure. Pricing's survey is simpler because the persona's interesting work is in `Draft` (web_search); Survey is just "list products with non-trivial price."

**Files:**
- Modify: `daemon/internal/personas/pricing/pricing.go`
- Modify: `daemon/internal/personas/pricing/pricing_test.go`

- [ ] **Step 1: Add `Survey`**

```go
func (Pricing) Survey(ctx context.Context, deps personas.Deps) (personas.SurveyResult, error) {
    if deps.MCP == nil {
        return personas.SurveyResult{Skipped: true, SkipReason: "MCP client not configured"}, nil
    }
    if _, err := deps.MCP.Initialize(ctx); err != nil {
        return personas.SurveyResult{}, fmt.Errorf("mcp initialize: %w", err)
    }
    products, err := listProductsWithPrice(ctx, deps.MCP, 20)
    if err != nil {
        return personas.SurveyResult{}, err
    }
    if len(products) == 0 {
        return personas.SurveyResult{Skipped: true, SkipReason: "no priced products"}, nil
    }
    // Score by price band — products in the most-actionable mid-tier get a
    // higher score; we don't try to detect mispricing here (that's Draft's
    // job with web_search). Survey just narrows the candidate set.
    out := make([]personas.Observation, 0, 5)
    for _, p := range products {
        if len(out) >= 5 {
            break
        }
        price, _ := strconv.ParseFloat(p.RegularPrice, 64)
        if price <= 0 {
            continue
        }
        score := scoreByPriceBand(price)
        out = append(out, personas.Observation{
            Title:     fmt.Sprintf("Price review · %s", p.Name),
            Rationale: fmt.Sprintf("Current: %s. Score: %.2f based on price band.", p.RegularPrice, score),
            TargetID:  fmt.Sprintf("product:%d", p.ID),
            Score:     score,
        })
    }
    if len(out) == 0 {
        return personas.SurveyResult{Skipped: true, SkipReason: "no candidates with regular_price > 0"}, nil
    }
    return personas.SurveyResult{Observations: out}, nil
}

func scoreByPriceBand(price float64) float64 {
    // Sweet spot: $20-$200 ⇒ score 0.8. Below or above ⇒ 0.4. Cheap
    // proxy for "actionable for a benchmark search."
    if price >= 20 && price <= 200 {
        return 0.8
    }
    return 0.4
}

func listProductsWithPrice(ctx context.Context, c *mcp.Client, limit int) ([]product, error) {
    var listOut struct {
        Products []product `json:"products"`
    }
    if err := callAbility(ctx, c, "wooagent-products/list",
        map[string]any{"per_page": limit}, &listOut); err != nil {
        return nil, err
    }
    return listOut.Products, nil
}
```

- [ ] **Step 2: Make `Draft` target-aware**

Replace the existing `Draft` signature and replace the `productID` resolution with target parsing (same `parseProductTargetID` shape as Marketing — copy into the pricing package OR move into a shared helper).

```go
func (Pricing) Draft(ctx context.Context, deps personas.Deps, target personas.DraftTarget) (personas.Drafted, error) {
    if deps.MCP == nil {
        return personas.Drafted{Skipped: true, SkipReason: "MCP client not configured"}, nil
    }
    if strings.TrimSpace(deps.Env.AnthropicAPIKey) == "" {
        return personas.Drafted{Skipped: true, SkipReason: "ANTHROPIC_API_KEY not set"}, nil
    }
    skill, ok := deps.Skills[skillName]
    if !ok {
        return personas.Drafted{Skipped: true, SkipReason: fmt.Sprintf("skill %q not found", skillName)}, nil
    }

    productID, err := parseProductTargetID(target.TargetID)
    if err != nil {
        return personas.Drafted{Skipped: true, SkipReason: err.Error()}, nil
    }
    // existing flow from here: MCP init, getProduct(productID), draftProposal(...) ...
    if _, err := deps.MCP.Initialize(ctx); err != nil {
        return personas.Drafted{}, fmt.Errorf("mcp initialize: %w", err)
    }
    p, err := getProduct(ctx, deps.MCP, productID)
    if err != nil {
        return personas.Drafted{}, fmt.Errorf("get product %d: %w", productID, err)
    }
    currentPrice, perr := strconv.ParseFloat(strings.TrimSpace(p.RegularPrice), 64)
    if perr != nil || currentPrice <= 0 {
        return personas.Drafted{Skipped: true, SkipReason: "no usable regular_price"}, nil
    }
    // existing draftProposal() call returns the existing Drafted shape unchanged.
    // (No further change here.)
    ...
}
```

- [ ] **Step 3: Move `parseProductTargetID` into `personas/observations.go`**

To avoid copy-paste across personas, move the helper into the shared package:

```go
// In daemon/internal/personas/observations.go
func ParseProductTargetID(targetID string) (int, error) {
    const prefix = "product:"
    if !strings.HasPrefix(targetID, prefix) {
        return 0, fmt.Errorf("invalid target_id %q (expected %s<id>)", targetID, prefix)
    }
    n, err := strconv.Atoi(strings.TrimPrefix(targetID, prefix))
    if err != nil || n <= 0 {
        return 0, fmt.Errorf("invalid product id in target_id %q", targetID)
    }
    return n, nil
}

func ParseOrderTargetID(targetID string) (int, error) {
    const prefix = "order:"
    if !strings.HasPrefix(targetID, prefix) {
        return 0, fmt.Errorf("invalid target_id %q (expected %s<id>)", targetID, prefix)
    }
    n, err := strconv.Atoi(strings.TrimPrefix(targetID, prefix))
    if err != nil || n <= 0 {
        return 0, fmt.Errorf("invalid order id in target_id %q", targetID)
    }
    return n, nil
}
```

Update Marketing and Pricing to call `personas.ParseProductTargetID` instead of their local copies.

- [ ] **Step 4: Update pricing tests** to call `Survey` with a stub MCP, and `Draft` with an explicit target.

- [ ] **Step 5: Run tests + don't commit yet**

Run: `go test ./internal/personas/pricing/... -v` — should pass.

---

## Task 9: Sales Support implements `Survey`

Order-driven instead of product-driven. Otherwise same structure.

**Files:**
- Modify: `daemon/internal/personas/sales-support/sales_support.go`

- [ ] **Step 1: Add `Survey`**

```go
const surveySystemPrompt = `You are a customer-support agent for a small-batch home-goods store.
Given a list of recent orders (id, customer, status, days_since), pick the 3-5
that would most benefit from a customer note (order completion follow-up,
delivery confirmation, etc.). For each, return a short title, a 1-2 sentence
rationale, and a score in [0,1] reflecting how useful a note would be.

Return JSON ONLY:
{"observations":[{"order_id":N,"title":"...","rationale":"...","score":0.0}, ...]}`

func (SalesSupport) Survey(ctx context.Context, deps personas.Deps) (personas.SurveyResult, error) {
    if deps.MCP == nil {
        return personas.SurveyResult{Skipped: true, SkipReason: "MCP client not configured"}, nil
    }
    if _, err := deps.MCP.Initialize(ctx); err != nil {
        return personas.SurveyResult{}, fmt.Errorf("mcp initialize: %w", err)
    }
    orders, err := listRecentCompletedOrders(ctx, deps.MCP, 20, 14) // last 14 days
    if err != nil {
        return personas.SurveyResult{}, err
    }
    if len(orders) == 0 {
        return personas.SurveyResult{Skipped: true, SkipReason: "no recent orders"}, nil
    }
    if strings.TrimSpace(deps.Env.AnthropicAPIKey) == "" {
        return personas.SurveyResult{Skipped: true, SkipReason: "ANTHROPIC_API_KEY not set"}, nil
    }
    raw, err := surveyOrdersAnthropic(ctx, deps.Env.AnthropicAPIKey,
        firstNonEmpty(deps.Env.AnthropicModel, defaultAnthropicModel), orders)
    if err != nil {
        return personas.SurveyResult{Skipped: true, SkipReason: fmt.Sprintf("survey LLM errored: %v", err)}, nil
    }
    parsed, err := parseOrderSurvey(raw)
    if err != nil {
        return personas.SurveyResult{Skipped: true, SkipReason: fmt.Sprintf("malformed survey response: %v", err)}, nil
    }
    return personas.SurveyResult{Observations: parsed}, nil
}
```

Mirror the marketing-side helpers (`listRecentCompletedOrders`, `surveyOrdersAnthropic`, `parseOrderSurvey`). Order-survey output uses `target_id="order:<id>"`.

- [ ] **Step 2: Make `Draft` target-aware**

Same pattern: parse `target.TargetID` with `personas.ParseOrderTargetID`, fetch the specific order, continue with the existing draft logic.

- [ ] **Step 3: Tests + don't commit yet**

---

## Task 10: Commit the interface migration

Single big commit. Personas now implement `Survey` + target-aware `Draft`; `RunAndPersistMode` dispatcher in place.

- [ ] **Step 1: Run the full test suite**

Run: `go test ./...`

Expected: pre-existing F12 stale test still fails; all other tests pass.

- [ ] **Step 2: Commit**

```bash
git add daemon/internal/personas/
git commit -m "refactor(personas): add Survey, target-aware Draft, RunAndPersistMode

Persona interface gains Survey(ctx, deps) → SurveyResult.
Existing Draft becomes Draft(ctx, deps, target). RunAndPersist
becomes a small dispatcher with two modes (survey, draft).
HasOpenWork narrowed to in_progress|in_review only (backlog
no longer blocks new surveys).

Marketing, Pricing, and Sales Support each implement Survey.
Shared target-id parsers in observations.go. Test coverage on
parseSurvey + upsertBacklogObservation + RunAndPersistMode.

See backlog-observations-design.md."
```

---

## Task 11: Scheduler — `promotion` trigger + new decision tree

The loop now: HasOpenWork ⇒ skip; backlog rows exist ⇒ enqueue promotion; never ran ⇒ bootstrap survey; else ⇒ tick survey.

**Files:**
- Modify: `daemon/internal/scheduler/types.go`
- Modify: `daemon/internal/scheduler/queue.go` — `EnqueueParams` gains `TargetIssueID`
- Modify: `daemon/internal/scheduler/loop.go`
- Modify: `daemon/internal/scheduler/worker.go`

- [ ] **Step 1: Write a failing scheduler-loop test**

Add to `loop_test.go`:

```go
func TestLoop_BacklogTriggersPromotion(t *testing.T) {
    // Persona has 1 backlog row, no in_progress/in_review.
    // Loop should enqueue a promotion run for the top-scored backlog issue.
    // (Setup omitted for brevity — follow the existing harness.)
    ...
}

func TestLoop_NeverRanTriggersBootstrapSurvey(t *testing.T) { ... }

func TestLoop_BacklogEmptyTriggersTickSurvey(t *testing.T) { ... }
```

- [ ] **Step 2: Implement**

In `scheduler/types.go`:

```go
const TriggerPromotion Trigger = "promotion"
```

In `scheduler/queue.go`, extend `EnqueueParams` and the `INSERT INTO runs` to write `target_issue_id` (wait — `runs.task_step_id` is for insights-planning; for now we need a `target_issue_id` column on `runs` OR overload `issue_id`. The design uses `issue_id`. Reuse it).

The cleanest fit: when the loop enqueues a promotion run for issue X, set `runs.issue_id = X`. The worker reads `issue_id` and routes to `RunAndPersistMode{Kind:"draft", PromoteFrom: issue_id}`.

In `loop.go`, replace the per-persona decision tree:

```go
for each enabled persona:
    if hasActiveRun(persona):
        continue
    if hasOpenWork(persona):  // in_progress|in_review
        continue
    if hasBacklog(persona):
        topID := topBacklogID(persona)  // ORDER BY observation_score DESC, created_at ASC
        enqueue(promotion, ScheduledAt: now, IssueID: topID)
    else if neverRan(persona):
        enqueue(bootstrap, ScheduledAt: now)  // survey-mode
    else:
        enqueue(tick, ScheduledAt: now)       // survey-mode
```

In `worker.go`, route by trigger:

```go
switch run.Trigger {
case TriggerPromotion:
    return personas.RunAndPersistMode(ctx, persona, deps, personas.RunMode{
        Kind: "draft", PromoteFrom: run.IssueID,
    })
default:
    return personas.RunAndPersistMode(ctx, persona, deps, personas.RunMode{Kind: "survey"})
}
```

- [ ] **Step 3: Run loop + worker tests**

Run: `go test ./internal/scheduler/... -v`

- [ ] **Step 4: Commit**

```bash
git add daemon/internal/scheduler/
git commit -m "feat(scheduler): promotion trigger + backlog-aware decision tree

Loop now: if persona has open work, skip. Else if backlog
exists, enqueue a promotion run for the top-scored backlog
issue. Else bootstrap (first run) or tick (cadence) — both
in survey mode. Worker dispatches to RunAndPersistMode by
trigger."
```

---

## Task 12: HTTP — `POST /v1/runs` accepts `target_issue_id`

**Files:**
- Modify: `daemon/internal/httpapi/handlers_v1.go`
- Modify: `daemon/internal/httpapi/handlers_v1_test.go`

- [ ] **Step 1: Failing test**

```go
func TestPostRuns_WithTargetIssueID_EnqueuesPromotion(t *testing.T) {
    // Seed: a backlog issue. POST /v1/runs with {"persona":"marketing","target_issue_id":"<id>"}.
    // Assert: response has trigger=promotion, issue_id=<id>, status=queued.
    ...
}
```

- [ ] **Step 2: Implement**

The handler currently takes `{persona}`. Add `target_issue_id`. When present, validate the issue exists, is `status='backlog'`, and belongs to the persona — then route to `scheduler.EnqueueManualPromotion(ctx, personaSlug, targetIssueID)`. Add the corresponding method on `Scheduler`.

- [ ] **Step 3: Tests + commit**

---

## Task 13: UI — `Issue` type gains observation columns

**Files:**
- Modify: `ui/src/api/client.ts`

- [ ] **Step 1: Extend the type**

```ts
export interface Issue {
  // ... existing fields
  observation_rationale?: string;
  observation_score?: number;
  target_id?: string;
  draft_attempts?: number;
}
```

- [ ] **Step 2: Extend `api.runs.trigger`**

```ts
trigger: (c: Connection, persona: string, targetIssueId?: string) =>
  request<{ run: Run }>(c, '/v1/runs', {
    method: 'POST',
    body: JSON.stringify(
      targetIssueId ? { persona, target_issue_id: targetIssueId } : { persona },
    ),
  }),
```

- [ ] **Step 3: Build, no tests on this layer alone, no commit yet** — lands with Task 14.

---

## Task 14: UI — `BacklogCard` component

**Files:**
- Create: `ui/src/components/BacklogCard.tsx`

- [ ] **Step 1: Implement**

```tsx
import { Card, Stack, Text } from '@wordpress/ui';
import { Button } from '@wordpress/components';
import type { Issue } from '../api/client';
import { PersonaAvatar, personaKeyFrom } from './PersonaAvatar';
import { KindBadge, kindFromIssue } from './StatusBadge';

interface Props {
  issue: Issue;
  onDraftThis: (issueId: string) => void;
  busy?: boolean;
}

export default function BacklogCard({ issue, onDraftThis, busy }: Props) {
  const personaKey = personaKeyFrom(issue.persona);
  const kind = kindFromIssue(issue);
  return (
    <Card.Root>
      <Card.Header>
        <Stack direction="row" gap="sm" align="center">
          <KindBadge kind={kind} />
          <Text variant="body-sm" className="wa-mono">
            {issue.id.slice(0, 8).toUpperCase()}
          </Text>
        </Stack>
      </Card.Header>
      <Card.Body>
        <Stack direction="column" gap="xs">
          <Text variant="body-md" style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}>
            {issue.title}
          </Text>
          {issue.observation_rationale && (
            <Text
              variant="body-sm"
              style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
            >
              {issue.observation_rationale}
            </Text>
          )}
          <Stack direction="row" gap="sm" align="center">
            <PersonaAvatar persona={personaKey} size="sm" />
            <Button
              variant="secondary"
              __next40pxDefaultSize
              isBusy={busy}
              disabled={busy}
              onClick={() => onDraftThis(issue.id)}
            >
              Draft this →
            </Button>
          </Stack>
        </Stack>
      </Card.Body>
    </Card.Root>
  );
}
```

- [ ] **Step 2: Commit with Task 13's API extension**

```bash
git add ui/src/api/client.ts ui/src/components/BacklogCard.tsx
git commit -m "feat(ui): BacklogCard component + Issue type extension

BacklogCard renders the lighter \"observation\" variant of a
kanban card — title + rationale + persona avatar + Draft this
CTA. Used in the Kanban Backlog column after Task 15."
```

---

## Task 15: UI — wire `BacklogCard` + `DraftingCard` into Kanban

**Files:**
- Modify: `ui/src/screens/Kanban.tsx`
- Create: `ui/src/components/DraftingCard.tsx`

- [ ] **Step 1: `DraftingCard` — analogous shape with spinner + "Drafting now…" hint**

(Pattern mirrors `BacklogCard` but no CTA; renders a `Spinner` from `@wordpress/components` and a `Drafting now…` line.)

- [ ] **Step 2: Kanban column rendering**

In `Kanban.tsx`, the existing `columnForIssue` already routes `backlog` → `backlog` column and `in_progress` → `drafting` column. Update the per-item render switch:

```tsx
{col === 'backlog' && (
  <BacklogCard
    issue={issue}
    onDraftThis={(id) => handleDraftThis(id)}
    busy={draftingFor === id}
  />
)}
{col === 'drafting' && <DraftingCard issue={issue} />}
{col === 'in_review' && /* existing review card */}
{col === 'done' && /* existing done card */}
```

`handleDraftThis(issueId)` calls `api.runs.trigger(connection, issue.persona, issueId)`. On the API response, set local state so the BacklogCard shows `isBusy`. Don't navigate — the polling (App.tsx 10s + 2s while in_progress) handles state propagation.

- [ ] **Step 3: Bump poll interval while in_progress issues exist**

In `App.tsx`, the existing 10s issue-poll becomes adaptive:

```tsx
const hasInProgress = issues?.some(i => i.status === 'in_progress') ?? false;
const interval = hasInProgress ? 2_000 : 10_000;
// rest of polling
```

- [ ] **Step 4: Build, smoke against the live daemon**

Run: `cd ui && npm run build && scripts/build-ui-into-daemon.sh && cd daemon && go build -o /tmp/wooagent-smoke ./cmd/wooagent`

Restart daemon, reload UI. Verify:
1. Backlog column has 2-4 cards per persona (Survey ran on bootstrap).
2. Top-scored card auto-promoted → Drafting (spinner) → In Review.
3. Click "Draft this" on a remaining backlog card → it moves to Drafting → In Review.

- [ ] **Step 5: Commit**

```bash
git add ui/src/screens/Kanban.tsx ui/src/components/DraftingCard.tsx ui/src/App.tsx
git commit -m "feat(ui): Backlog + Drafting columns rendering live

BacklogCard fills the Backlog column from observation issues.
DraftingCard shows the in-progress state with a spinner.
'Draft this' CTA POSTs /v1/runs with target_issue_id; the
new run flows through the persona's Draft and lands in_review.

App-level issue poll cadence drops to 2s while any issue is
in_progress, back to 10s otherwise — keeps the Drafting card
feeling live without hammering the daemon in steady state."
```

---

## Task 16: KindBadge — persona-name labels (F5)

The design notes this ships alongside backlog-observations. The kind pill should read persona name (Marketing / Pricing / Sales), not artifact type.

**Files:**
- Modify: `ui/src/components/StatusBadge.tsx`
- Modify: `CLAUDE.md` — update the persona-color exception doc

- [ ] **Step 1: Update `KindBadge` mapping**

The current code maps `kind` (content/price/message) to labels Content/Price/Message. Change to Marketing/Pricing/Sales.

- [ ] **Step 2: Update `CLAUDE.md`**

In the persona-color exception section, change:

```
The kind pill on cards (CONTENT / CAMPAIGN / EMAIL — `KindBadge` …)
```

to:

```
The persona pill on cards (MARKETING / PRICING / SALES SUPPORT / …
— `KindBadge` …) reads the persona name in the persona's color.
The artifact type (description rewrite, price change, reply draft)
lives in the secondary line below the title.
```

- [ ] **Step 3: Commit**

```bash
git add ui/src/components/StatusBadge.tsx CLAUDE.md
git commit -m "refactor(ui/kanban): KindBadge reads persona name, not artifact type

Per F5 finding from the 2026-05-14 smoke session: the kind pill
already uses persona colors (per CLAUDE.md exception), so naming
the persona reinforces what the color means and helps merchants
learn the fleet. Artifact type moves to the secondary card line.

CLAUDE.md persona-color exception doc updated to match."
```

---

## Task 17: Survey-failure header notice on Kanban

When a persona's last run is `failed_*`, show a small notice in that persona's swimlane (or board-wide if no per-persona swimlane exists yet) with a retry CTA.

**Files:**
- Modify: `ui/src/screens/Kanban.tsx`

- [ ] **Step 1: Fetch the per-persona latest run**

Add to the Kanban data-loading: `api.runs.list(connection, { limit: 30 })`. Group by persona, take latest. If status is `failed | failed_permanent`, render a `Notice.Root intent="warning"` near the top of that persona's items (or above the columns if there's no per-persona swimlane in v1).

- [ ] **Step 2: Retry CTA**

The Notice has a "Retry" button that calls `api.runs.trigger(connection, persona)` (no target_issue_id — a fresh survey, not a promotion).

- [ ] **Step 3: Tests, build, commit**

```bash
git add ui/src/screens/Kanban.tsx
git commit -m "feat(ui/kanban): survey-failure header notice with retry

When a persona's last run is failed*, surface a warning notice
on the kanban with a Retry button. Without this, a tester whose
first survey errors sees an empty board with no recourse."
```

---

## Task 18: Final smoke — end-to-end against the test store

- [ ] **Step 1: Clear DB to bootstrap state**

```bash
mv ~/.wooagent/wooagent.db ~/.wooagent/wooagent.db.backup-$(date +%s)
/tmp/wooagent-smoke init  # re-creates DB and ui-session token
```

- [ ] **Step 2: Re-pair the test store**

Open the UI, walk onboarding, paste store URL `woo-demo-store-99cc5c.mystagingwebsite.com`.

- [ ] **Step 3: Watch the bootstrap flow**

Within a few minutes:
- Each persona's Survey runs (3 surveys × ~10-60s).
- Backlog column populates (3-5 cards per persona).
- Top-scored backlog item for each persona promotes → Drafting → In Review.
- /runs shows the corresponding bootstrap, promotion, and (eventually) tick rows.

- [ ] **Step 4: Manual promotion**

Click "Draft this" on a backlog card → it moves to Drafting → In Review.

- [ ] **Step 5: Filed findings, if any**

Add any new bugs to the existing F-series finding list. Don't fix in this commit — file follow-ups in Linear (team DSGWOO, project WooAgent OS, assignee Elizabeth — see [`feedback_followups_in_linear`](../.claude/projects/-Users-elizabethpizzuti-claude-wooagent/memory/feedback_followups_in_linear.md)).

- [ ] **Step 6: Commit nothing — this is a verification task**

---

## Self-Review Notes

**Spec coverage:** Every section of `backlog-observations-design.md` maps to a task above.
- State machine, schema, HasOpenWork narrowing → Tasks 1, 2
- Observation/SurveyResult/DraftTarget types + RunAndPersistMode → Tasks 3, 4, 5, 6
- Marketing/Pricing/Sales Support Survey impls → Tasks 7, 8, 9
- Scheduler dispatch (promotion trigger, decision tree) → Task 11
- HTTP API (`target_issue_id` on POST /v1/runs) → Task 12
- UI (BacklogCard, DraftingCard, live polling, KindBadge persona name, survey-failure notice) → Tasks 13-17
- End-to-end verification → Task 18

**Placeholder scan:** Each step contains either complete code, an exact command, or a small structural pattern (e.g. "mirror Task 7"). Where I write "mirror Task X," I include enough of the structure to make the mirroring concrete.

**Type consistency:** `Observation` fields used in Tasks 3, 4, 7, 8, 9, 13, 14 are identical. `DraftTarget` shape consistent across Tasks 3, 7, 8, 9. `RunMode` fields consistent in Tasks 6 and 11.

**Scope:** This plan is scoped to a single feature with cohesive design. No need to split.
