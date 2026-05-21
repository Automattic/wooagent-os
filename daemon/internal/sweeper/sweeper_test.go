package sweeper

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

// openTestDB opens a fresh SQLite DB with the full migration set
// applied and seeds an agent so issue FKs pass. Mirrors the helper in
// scheduler/queue_test.go.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_, err = st.DB.ExecContext(context.Background(),
		`INSERT INTO agents(persona, name, enabled, created_at, updated_at)
         VALUES('marketing', 'Marketing', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	return st.DB
}

// seedDismissed inserts an issue with status='dismissed' and a
// dismissed_at offset relative to refNow. Use negativeAgo > 0 to
// place the dismiss in the past.
func seedDismissed(t *testing.T, db *sql.DB, id string, refNow time.Time, dismissedAgo time.Duration) {
	t.Helper()
	createdAt := refNow.Add(-dismissedAgo - time.Hour).UTC().Format(time.RFC3339)
	dismissedAt := refNow.Add(-dismissedAgo).UTC().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT INTO issues (id, title, persona, status, priority, created_at, updated_at, dismiss_reason, dismissed_at)
		 VALUES (?, ?, 'marketing', 'dismissed', 'medium', ?, ?, 'tone_off', ?)`,
		id, "fixture-"+id, createdAt, dismissedAt, dismissedAt,
	)
	if err != nil {
		t.Fatalf("seed dismissed: %v", err)
	}
}

func seedDependents(t *testing.T, db *sql.DB, issueID string, refNow time.Time) {
	t.Helper()
	ts := refNow.UTC().Format(time.RFC3339)
	if _, err := db.Exec(
		`INSERT INTO runs (id, persona, trigger, status, attempt, scheduled_at, created_at, issue_id)
		 VALUES (?, 'marketing', 'tick', 'succeeded', 1, ?, ?, ?)`,
		"run-"+issueID, ts, ts, issueID,
	); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO turn_events (turn_id, event_schema_version, issue_id, persona, started_at, created_at)
		 VALUES (?, 1, ?, 'marketing', ?, ?)`,
		"turn-"+issueID, issueID, ts, ts,
	); err != nil {
		t.Fatalf("seed turn_event: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO audit_invocations
		 (plan_id, task_id, step_id, issue_id, persona, model, prompt_hash, ability, args_hash, intent, outcome, created_at)
		 VALUES ('', '', '', ?, 'marketing', '', '', 'fake.read', '', 'read', 'success', ?)`,
		issueID, ts,
	); err != nil {
		t.Fatalf("seed audit: %v", err)
	}
}

func countWhere(t *testing.T, db *sql.DB, table, where string, args ...any) int {
	t.Helper()
	var n int
	q := fmt.Sprintf(`SELECT count(*) FROM %s WHERE %s`, table, where)
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestSweepOnce_DeletesExpired(t *testing.T) {
	db := openTestDB(t)
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)

	// Past TTL: should be deleted.
	seedDismissed(t, db, "old", now, 31*24*time.Hour)
	seedDependents(t, db, "old", now)
	// Within TTL: should survive.
	seedDismissed(t, db, "fresh", now, 29*24*time.Hour)
	seedDependents(t, db, "fresh", now)
	// status='dismissed' but dismissed_at NULL — defensive case from the
	// Linear issue. Should never happen (the handler always stamps), but
	// the sweeper must leave it alone.
	if _, err := db.Exec(
		`INSERT INTO issues (id, title, persona, status, priority, created_at, updated_at, dismiss_reason)
		 VALUES ('orphan', 'fixture-orphan', 'marketing', 'dismissed', 'medium', ?, ?, 'tone_off')`,
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed orphan: %v", err)
	}

	var buf bytes.Buffer
	sw := &Sweeper{DB: db, Now: func() time.Time { return now }, Out: &buf}
	deleted, err := sw.SweepOnce(context.Background())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	// old: gone, with all dependents.
	if n := countWhere(t, db, "issues", "id = ?", "old"); n != 0 {
		t.Errorf("expected old issue gone, found %d", n)
	}
	if n := countWhere(t, db, "runs", "issue_id = ?", "old"); n != 0 {
		t.Errorf("expected runs for old gone, found %d", n)
	}
	if n := countWhere(t, db, "turn_events", "issue_id = ?", "old"); n != 0 {
		t.Errorf("expected turn_events for old gone, found %d", n)
	}
	if n := countWhere(t, db, "audit_invocations", "issue_id = ?", "old"); n != 0 {
		t.Errorf("expected audit rows for old gone, found %d", n)
	}

	// fresh: still here.
	if n := countWhere(t, db, "issues", "id = ?", "fresh"); n != 1 {
		t.Errorf("expected fresh issue intact, found %d", n)
	}
	if n := countWhere(t, db, "runs", "issue_id = ?", "fresh"); n != 1 {
		t.Errorf("expected fresh run intact, found %d", n)
	}

	// orphan (NULL dismissed_at): still here.
	if n := countWhere(t, db, "issues", "id = ?", "orphan"); n != 1 {
		t.Errorf("expected orphan issue intact, found %d", n)
	}

	if !strings.Contains(buf.String(), "deleted 1 dismissed issue") {
		t.Errorf("expected log line, got: %q", buf.String())
	}
}

func TestSweepOnce_NoExpired_NoOp(t *testing.T) {
	db := openTestDB(t)
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	seedDismissed(t, db, "fresh", now, 5*24*time.Hour)

	var buf bytes.Buffer
	sw := &Sweeper{DB: db, Now: func() time.Time { return now }, Out: &buf}
	deleted, err := sw.SweepOnce(context.Background())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
	if n := countWhere(t, db, "issues", "id = ?", "fresh"); n != 1 {
		t.Errorf("fresh issue should still exist")
	}
	if buf.Len() != 0 {
		t.Errorf("expected no log output for zero-delete cycle, got: %q", buf.String())
	}
}

// TestSweepOnce_RestoredIssue_Survives covers the Linear issue's
// edge-case note: an operator clicks Restore in the Archive side
// panel, which flips status back to in_review and clears dismiss_*.
// The sweeper's WHERE clause naturally excludes it.
func TestSweepOnce_RestoredIssue_Survives(t *testing.T) {
	db := openTestDB(t)
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	seedDismissed(t, db, "restored", now, 60*24*time.Hour)
	// Simulate Restore.
	if _, err := db.Exec(
		`UPDATE issues SET status='in_review', dismiss_reason=NULL, dismiss_comment=NULL, dismissed_at=NULL WHERE id='restored'`,
	); err != nil {
		t.Fatalf("restore: %v", err)
	}

	sw := &Sweeper{DB: db, Now: func() time.Time { return now }, Out: &bytes.Buffer{}}
	deleted, err := sw.SweepOnce(context.Background())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 (restored issue should survive)", deleted)
	}
	if n := countWhere(t, db, "issues", "id = ?", "restored"); n != 1 {
		t.Errorf("restored issue should still exist")
	}
}

// TestSweepOnce_LeavesNonDismissed makes sure rows in other terminal
// statuses (done, rejected) are never touched by the sweeper, even if
// their updated_at is years old.
func TestSweepOnce_LeavesNonDismissed(t *testing.T) {
	db := openTestDB(t)
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	old := now.Add(-365 * 24 * time.Hour).UTC().Format(time.RFC3339)
	for _, status := range []string{"done", "rejected", "in_review", "backlog"} {
		_, err := db.Exec(
			`INSERT INTO issues (id, title, persona, status, priority, created_at, updated_at)
			 VALUES (?, ?, 'marketing', ?, 'medium', ?, ?)`,
			status, "fixture-"+status, status, old, old,
		)
		if err != nil {
			t.Fatalf("seed %s: %v", status, err)
		}
	}

	sw := &Sweeper{DB: db, Now: func() time.Time { return now }, Out: &bytes.Buffer{}}
	deleted, err := sw.SweepOnce(context.Background())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
	for _, status := range []string{"done", "rejected", "in_review", "backlog"} {
		if n := countWhere(t, db, "issues", "id = ?", status); n != 1 {
			t.Errorf("%s issue should still exist", status)
		}
	}
}
