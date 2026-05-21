package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	// Seed one agent so the FK passes.
	_, err = st.DB.ExecContext(context.Background(),
		`INSERT INTO agents(persona, name, enabled, created_at, updated_at)
         VALUES('marketing', 'Marketing', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	return st.DB
}

func TestEnqueueAndClaim(t *testing.T) {
	db := openTestDB(t)
	q := &Queue{DB: db, Now: func() time.Time { return time.Unix(1700000000, 0).UTC() }}

	run, err := q.Enqueue(context.Background(), EnqueueParams{
		Persona: "marketing", Trigger: TriggerTick, ScheduledAt: q.Now(),
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if run.Status != StatusQueued || run.Attempt != 1 {
		t.Fatalf("unexpected enqueue result: %+v", run)
	}

	claimed, err := q.ClaimNext(context.Background())
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed == nil || claimed.ID != run.ID || claimed.Status != StatusRunning {
		t.Fatalf("claim returned wrong row: %+v", claimed)
	}

	// Second claim should find no work.
	second, err := q.ClaimNext(context.Background())
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if second != nil {
		t.Fatalf("expected nil on empty queue, got %+v", second)
	}
}

func TestMarkTerminalBumpsLastRunAt(t *testing.T) {
	db := openTestDB(t)
	fixed := time.Unix(1700000000, 0).UTC()
	q := &Queue{DB: db, Now: func() time.Time { return fixed }}

	r, _ := q.Enqueue(context.Background(), EnqueueParams{
		Persona: "marketing", Trigger: TriggerTick, ScheduledAt: fixed,
	})
	_, _ = q.ClaimNext(context.Background())

	completed := fixed.Add(2 * time.Second)
	err := q.MarkTerminal(context.Background(), MarkTerminalParams{
		ID:          r.ID,
		Status:      StatusSucceeded,
		CompletedAt: completed,
		LatencyMS:   2000,
		// IssueID and TurnID intentionally omitted: FK references require
		// rows in issues/turn_events which we don't seed in the unit test.
	})
	if err != nil {
		t.Fatalf("mark terminal: %v", err)
	}

	var lastRunAt sql.NullString
	_ = db.QueryRowContext(context.Background(),
		`SELECT last_run_at FROM agents WHERE persona='marketing'`).Scan(&lastRunAt)
	if !lastRunAt.Valid || lastRunAt.String != completed.Format(time.RFC3339) {
		t.Errorf("agents.last_run_at = %v, want %s", lastRunAt, completed.Format(time.RFC3339))
	}
}

func TestCancel_RunningRow(t *testing.T) {
	db := openTestDB(t)
	fixed := time.Unix(1700000000, 0).UTC()
	q := &Queue{DB: db, Now: func() time.Time { return fixed }}

	r, _ := q.Enqueue(context.Background(), EnqueueParams{
		Persona: "marketing", Trigger: TriggerTick, ScheduledAt: fixed,
	})
	if _, err := q.ClaimNext(context.Background()); err != nil {
		t.Fatalf("claim: %v", err)
	}

	got, err := q.Cancel(context.Background(), r.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got.Status != StatusFailedPermanent {
		t.Errorf("status = %s, want failed_permanent", got.Status)
	}
	if got.FailureClass != FailurePermanent {
		t.Errorf("failure_class = %s, want permanent", got.FailureClass)
	}
	if got.FailureReason == nil || *got.FailureReason != "cancelled by operator" {
		t.Errorf("failure_reason = %v, want \"cancelled by operator\"", got.FailureReason)
	}
	if got.CompletedAt == nil {
		t.Error("completed_at should be set after cancel")
	}

	// last_run_at must NOT be bumped — operator wants the persona to be
	// eligible for the next tick after cancelling a stuck row.
	var lastRunAt sql.NullString
	_ = db.QueryRowContext(context.Background(),
		`SELECT last_run_at FROM agents WHERE persona='marketing'`).Scan(&lastRunAt)
	if lastRunAt.Valid {
		t.Errorf("last_run_at should not be set; got %s", lastRunAt.String)
	}
}

func TestCancel_QueuedRow(t *testing.T) {
	db := openTestDB(t)
	fixed := time.Unix(1700000000, 0).UTC()
	q := &Queue{DB: db, Now: func() time.Time { return fixed }}

	r, _ := q.Enqueue(context.Background(), EnqueueParams{
		Persona: "marketing", Trigger: TriggerTick, ScheduledAt: fixed,
	})
	got, err := q.Cancel(context.Background(), r.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got.Status != StatusFailedPermanent {
		t.Errorf("status = %s, want failed_permanent", got.Status)
	}
}

func TestCancel_AlreadyTerminal(t *testing.T) {
	db := openTestDB(t)
	fixed := time.Unix(1700000000, 0).UTC()
	q := &Queue{DB: db, Now: func() time.Time { return fixed }}

	r, _ := q.Enqueue(context.Background(), EnqueueParams{
		Persona: "marketing", Trigger: TriggerTick, ScheduledAt: fixed,
	})
	_, _ = q.ClaimNext(context.Background())
	if err := q.MarkTerminal(context.Background(), MarkTerminalParams{
		ID: r.ID, Status: StatusSucceeded, CompletedAt: fixed, LatencyMS: 100,
	}); err != nil {
		t.Fatalf("mark terminal: %v", err)
	}

	_, err := q.Cancel(context.Background(), r.ID)
	if !errors.Is(err, ErrRunNotCancellable) {
		t.Errorf("err = %v, want ErrRunNotCancellable", err)
	}
}

func TestCancel_NotFound(t *testing.T) {
	db := openTestDB(t)
	q := &Queue{DB: db, Now: time.Now}

	_, err := q.Cancel(context.Background(), "does-not-exist")
	if !errors.Is(err, ErrRunNotFound) {
		t.Errorf("err = %v, want ErrRunNotFound", err)
	}
}
