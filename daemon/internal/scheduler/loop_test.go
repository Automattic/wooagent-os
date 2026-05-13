package scheduler

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestLoop_DueAgent_EnqueuesTickRow(t *testing.T) {
	db := openTestDB(t)
	fixed := time.Unix(1700000000, 0).UTC()
	// Make the seeded marketing agent overdue: last_run_at = now - 7h, cadence = 6h.
	_, _ = db.ExecContext(context.Background(),
		`UPDATE agents SET cadence_seconds=21600, last_run_at=? WHERE persona='marketing'`,
		fixed.Add(-7*time.Hour).Format(time.RFC3339),
	)
	q := &Queue{DB: db, Now: func() time.Time { return fixed }}
	l := &Loop{
		DB:    db,
		Queue: q,
		Now:   func() time.Time { return fixed },
		HasOpenWorkFn: func(ctx context.Context, persona string) (bool, error) {
			return false, nil
		},
	}
	if err := l.tickOnce(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	var n int
	_ = db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM runs WHERE persona='marketing' AND status='queued' AND trigger='tick'`,
	).Scan(&n)
	if n != 1 {
		t.Errorf("expected 1 queued tick row, got %d", n)
	}
}

func TestLoop_NotDueAgent_NoRow(t *testing.T) {
	db := openTestDB(t)
	fixed := time.Unix(1700000000, 0).UTC()
	// Disable the sales-support agent that migrations always seed so this test
	// can assert strictly zero rows without the bootstrap path firing for it.
	_, _ = db.ExecContext(context.Background(),
		`UPDATE agents SET enabled=0 WHERE persona='sales-support'`)
	_, _ = db.ExecContext(context.Background(),
		`UPDATE agents SET cadence_seconds=21600, last_run_at=? WHERE persona='marketing'`,
		fixed.Add(-1*time.Hour).Format(time.RFC3339),
	)
	q := &Queue{DB: db, Now: func() time.Time { return fixed }}
	l := &Loop{DB: db, Queue: q, Now: func() time.Time { return fixed }}
	if err := l.tickOnce(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	var n int
	_ = db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM runs`).Scan(&n)
	if n != 0 {
		t.Errorf("expected zero rows, got %d", n)
	}
}

func TestLoop_BootstrapOnFirstStart_EnqueuesBootstrap(t *testing.T) {
	db := openTestDB(t)
	fixed := time.Unix(1700000000, 0).UTC()
	// last_run_at is NULL (default state for a freshly migrated DB).
	q := &Queue{DB: db, Now: func() time.Time { return fixed }}
	l := &Loop{DB: db, Queue: q, Now: func() time.Time { return fixed }}
	if err := l.tickOnce(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	var trig string
	_ = db.QueryRowContext(context.Background(),
		`SELECT trigger FROM runs WHERE persona='marketing' LIMIT 1`,
	).Scan(&trig)
	if trig != string(TriggerBootstrap) {
		t.Errorf("first run trigger = %s, want bootstrap", trig)
	}
}

func TestLoop_HasOpenWork_WritesSkipRow(t *testing.T) {
	db := openTestDB(t)
	fixed := time.Unix(1700000000, 0).UTC()
	_, _ = db.ExecContext(context.Background(),
		`UPDATE agents SET cadence_seconds=21600, last_run_at=? WHERE persona='marketing'`,
		fixed.Add(-7*time.Hour).Format(time.RFC3339),
	)
	q := &Queue{DB: db, Now: func() time.Time { return fixed }}
	l := &Loop{
		DB: db, Queue: q, Now: func() time.Time { return fixed },
		HasOpenWorkFn: func(ctx context.Context, persona string) (bool, error) {
			return true, nil
		},
	}
	if err := l.tickOnce(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	var status string
	var reason sql.NullString
	_ = db.QueryRowContext(context.Background(),
		`SELECT status, skip_reason FROM runs WHERE persona='marketing' LIMIT 1`).Scan(&status, &reason)
	if status != string(StatusSkipped) {
		t.Errorf("status = %s, want skipped", status)
	}
	if !reason.Valid || !strings.Contains(reason.String, "open") {
		t.Errorf("skip_reason = %v, want contains 'open'", reason)
	}
}

func TestLoop_DueButQueueAlreadyHasRun_NoDupe(t *testing.T) {
	db := openTestDB(t)
	fixed := time.Unix(1700000000, 0).UTC()
	_, _ = db.ExecContext(context.Background(),
		`UPDATE agents SET cadence_seconds=21600, last_run_at=? WHERE persona='marketing'`,
		fixed.Add(-7*time.Hour).Format(time.RFC3339),
	)
	q := &Queue{DB: db, Now: func() time.Time { return fixed }}
	_, _ = q.Enqueue(context.Background(), EnqueueParams{
		Persona: "marketing", Trigger: TriggerManual, ScheduledAt: fixed,
	})
	l := &Loop{
		DB: db, Queue: q, Now: func() time.Time { return fixed },
		HasOpenWorkFn: func(context.Context, string) (bool, error) { return false, nil },
	}
	if err := l.tickOnce(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	var n int
	_ = db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM runs WHERE persona='marketing'`,
	).Scan(&n)
	if n != 1 {
		t.Errorf("expected exactly 1 row (no dup), got %d", n)
	}
}
