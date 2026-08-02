package sweeper

import (
	"bytes"
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

// seedRun inserts a runs row with completed_at placed completedAgo before
// refNow. Pass issueID="" for the NULL-issue case (a skipped run that never
// produced a proposal — the bulk of real volume).
func seedRun(t *testing.T, db *sql.DB, id, status, issueID string, refNow time.Time, completedAgo time.Duration) {
	t.Helper()
	ts := refNow.Add(-completedAgo).UTC().Format(time.RFC3339)
	var issue any
	if issueID != "" {
		issue = issueID
	}
	// queued/running rows legitimately have no completed_at.
	var completed any
	if status != "queued" && status != "running" {
		completed = ts
	}
	if _, err := db.Exec(
		`INSERT INTO runs (id, persona, trigger, status, attempt, scheduled_at, created_at, completed_at, issue_id)
		 VALUES (?, 'marketing', 'tick', ?, 1, ?, ?, ?, ?)`,
		id, status, ts, ts, completed, issue,
	); err != nil {
		t.Fatalf("seed run %s: %v", id, err)
	}
}

// seedIssue inserts a live (non-dismissed) issue so runs can reference it.
func seedIssue(t *testing.T, db *sql.DB, id string, refNow time.Time) {
	t.Helper()
	ts := refNow.UTC().Format(time.RFC3339)
	if _, err := db.Exec(
		`INSERT INTO issues (id, title, persona, status, priority, created_at, updated_at)
		 VALUES (?, ?, 'marketing', 'in_review', 'medium', ?, ?)`,
		id, "fixture-"+id, ts, ts,
	); err != nil {
		t.Fatalf("seed issue %s: %v", id, err)
	}
}

func seedTurnEvent(t *testing.T, db *sql.DB, turnID, issueID string, refNow time.Time, createdAgo time.Duration) {
	t.Helper()
	ts := refNow.Add(-createdAgo).UTC().Format(time.RFC3339)
	var issue any
	if issueID != "" {
		issue = issueID
	}
	if _, err := db.Exec(
		`INSERT INTO turn_events (turn_id, event_schema_version, issue_id, persona, started_at, created_at)
		 VALUES (?, 1, ?, 'marketing', ?, ?)`,
		turnID, issue, ts, ts,
	); err != nil {
		t.Fatalf("seed turn_event %s: %v", turnID, err)
	}
}

func TestSweepRunsOnce_DeletesOldUnlinkedRuns(t *testing.T) {
	db := openTestDB(t)
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)

	// The dominant real-world case: an old skipped run that never produced
	// an issue. Should go.
	seedRun(t, db, "old-skipped", "skipped", "", now, 40*24*time.Hour)
	// Old and terminal but still linked to a live issue — must survive so
	// the issue's run-log link doesn't break.
	seedIssue(t, db, "live-issue", now)
	seedRun(t, db, "old-linked", "succeeded", "live-issue", now, 40*24*time.Hour)
	// Inside the window.
	seedRun(t, db, "recent", "succeeded", "", now, 5*24*time.Hour)

	var buf bytes.Buffer
	sw := &Sweeper{DB: db, Now: func() time.Time { return now }, Out: &buf}
	deleted, err := sw.SweepRunsOnce(context.Background())
	if err != nil {
		t.Fatalf("sweep runs: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}
	if n := countWhere(t, db, "runs", "id = ?", "old-skipped"); n != 0 {
		t.Errorf("expected old unlinked run gone, found %d", n)
	}
	if n := countWhere(t, db, "runs", "id = ?", "old-linked"); n != 1 {
		t.Errorf("expected issue-linked run preserved, found %d", n)
	}
	if n := countWhere(t, db, "runs", "id = ?", "recent"); n != 1 {
		t.Errorf("expected recent run preserved, found %d", n)
	}
	if !strings.Contains(buf.String(), "deleted 1 run(s)") {
		t.Errorf("expected log line, got: %q", buf.String())
	}
}

// In-flight rows must never be deleted, no matter how old. A run stuck in
// 'running' for months is a bug to surface via /v1/runs, not something to
// silently reclaim — DSGWOO-1293 added an explicit Cancel path for it.
func TestSweepRunsOnce_NeverDeletesInFlight(t *testing.T) {
	db := openTestDB(t)
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)

	for _, status := range []string{"queued", "running"} {
		seedRun(t, db, "stuck-"+status, status, "", now, 200*24*time.Hour)
	}

	sw := &Sweeper{DB: db, Now: func() time.Time { return now }, Out: &bytes.Buffer{}}
	deleted, err := sw.SweepRunsOnce(context.Background())
	if err != nil {
		t.Fatalf("sweep runs: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
	for _, status := range []string{"queued", "running"} {
		if n := countWhere(t, db, "runs", "id = ?", "stuck-"+status); n != 1 {
			t.Errorf("%s run should survive regardless of age", status)
		}
	}
}

// A run whose issue was deleted out from under it (FK sets issue_id NULL)
// is reclaimable once it ages out.
func TestSweepRunsOnce_DanglingIssueRefIsReclaimed(t *testing.T) {
	db := openTestDB(t)
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)

	seedIssue(t, db, "doomed", now)
	seedRun(t, db, "orphan-run", "succeeded", "doomed", now, 40*24*time.Hour)
	// Point issue_id at an id that no longer exists, bypassing the FK's
	// SET NULL so we exercise the NOT IN (SELECT id FROM issues) branch.
	if _, err := db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM issues WHERE id='doomed'`); err != nil {
		t.Fatalf("delete issue: %v", err)
	}

	sw := &Sweeper{DB: db, Now: func() time.Time { return now }, Out: &bytes.Buffer{}}
	deleted, err := sw.SweepRunsOnce(context.Background())
	if err != nil {
		t.Fatalf("sweep runs: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1 (dangling issue ref should be reclaimable)", deleted)
	}
}

func TestSweepRunsOnce_TurnEvents(t *testing.T) {
	db := openTestDB(t)
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)

	// Old, referenced by nothing → reclaim.
	seedTurnEvent(t, db, "turn-orphan", "", now, 40*24*time.Hour)
	// Old but tied to a live issue → keep. turn_events is the audit trail.
	seedIssue(t, db, "live-issue", now)
	seedTurnEvent(t, db, "turn-on-issue", "live-issue", now, 40*24*time.Hour)
	// Fresh and unreferenced → keep. This is the Ask Agent shape: no run,
	// no issue. Without the created_at guard it would vanish immediately.
	seedTurnEvent(t, db, "turn-fresh-ask", "", now, time.Minute)
	// Old, but a surviving recent run points at it → keep.
	seedTurnEvent(t, db, "turn-referenced", "", now, 40*24*time.Hour)
	if _, err := db.Exec(
		`INSERT INTO runs (id, persona, trigger, status, attempt, scheduled_at, created_at, completed_at, turn_id)
		 VALUES ('recent-run', 'marketing', 'tick', 'succeeded', 1, ?, ?, ?, 'turn-referenced')`,
		now.Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed referencing run: %v", err)
	}

	sw := &Sweeper{DB: db, Now: func() time.Time { return now }, Out: &bytes.Buffer{}}
	if _, err := sw.SweepRunsOnce(context.Background()); err != nil {
		t.Fatalf("sweep runs: %v", err)
	}

	for _, tc := range []struct {
		turnID string
		want   int
		why    string
	}{
		{"turn-orphan", 0, "old and unreferenced should be reclaimed"},
		{"turn-on-issue", 1, "tied to a live issue, audit trail must survive"},
		{"turn-fresh-ask", 1, "fresh unreferenced (Ask Agent) must survive"},
		{"turn-referenced", 1, "referenced by a surviving run must survive"},
	} {
		if n := countWhere(t, db, "turn_events", "turn_id = ?", tc.turnID); n != tc.want {
			t.Errorf("%s: count = %d, want %d (%s)", tc.turnID, n, tc.want, tc.why)
		}
	}
}

func TestSweepRunsOnce_NothingExpired_IsSilent(t *testing.T) {
	db := openTestDB(t)
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	seedRun(t, db, "recent", "succeeded", "", now, time.Hour)

	var buf bytes.Buffer
	sw := &Sweeper{DB: db, Now: func() time.Time { return now }, Out: &buf}
	deleted, err := sw.SweepRunsOnce(context.Background())
	if err != nil {
		t.Fatalf("sweep runs: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no log output for a zero-delete cycle, got: %q", buf.String())
	}
}

func TestSweepRunsOnce_RespectsRunsTTLOverride(t *testing.T) {
	db := openTestDB(t)
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	// Two days old — inside the 30-day default, outside a 1-day override.
	seedRun(t, db, "two-days", "skipped", "", now, 48*time.Hour)

	sw := &Sweeper{DB: db, Now: func() time.Time { return now }, Out: &bytes.Buffer{}}
	if deleted, err := sw.SweepRunsOnce(context.Background()); err != nil || deleted != 0 {
		t.Fatalf("default TTL: deleted=%d err=%v, want 0/nil", deleted, err)
	}

	sw.RunsTTL = 24 * time.Hour
	deleted, err := sw.SweepRunsOnce(context.Background())
	if err != nil {
		t.Fatalf("sweep runs: %v", err)
	}
	if deleted != 1 {
		t.Errorf("with RunsTTL=1d: deleted = %d, want 1", deleted)
	}
}

// The two sweeps must not interfere: Start's cycle runs the dismissed-issue
// purge first, then retention. A dismissed issue past its TTL should take
// its runs with it, and the retention pass must not error on the aftermath.
func TestCycle_BothSweepsRun(t *testing.T) {
	db := openTestDB(t)
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)

	seedDismissed(t, db, "old-dismissed", now, 40*24*time.Hour)
	seedDependents(t, db, "old-dismissed", now)
	seedRun(t, db, "old-unlinked", "skipped", "", now, 40*24*time.Hour)

	var buf bytes.Buffer
	sw := &Sweeper{DB: db, Now: func() time.Time { return now }, Out: &buf}
	sw.cycle(context.Background(), "test")

	if n := countWhere(t, db, "issues", "id = ?", "old-dismissed"); n != 0 {
		t.Errorf("dismissed issue should be purged, found %d", n)
	}
	if n := countWhere(t, db, "runs", "id = ?", "old-unlinked"); n != 0 {
		t.Errorf("old unlinked run should be reclaimed, found %d", n)
	}
	out := buf.String()
	if strings.Contains(out, "failed") {
		t.Errorf("neither sweep should fail, got: %q", out)
	}
}
