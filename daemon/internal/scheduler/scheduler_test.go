package scheduler

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

func TestScheduler_EnqueueManual(t *testing.T) {
	db := openTestDB(t)
	st := &store.Store{DB: db}
	s := &Scheduler{
		Store: st,
		Now:   func() time.Time { return time.Unix(1700000000, 0).UTC() },
		Out:   &bytes.Buffer{},
	}
	// Call Start so internals are wired; cancel immediately so the
	// background loops don't race the assertions.
	ctx, cancel := context.WithCancel(context.Background())
	_ = s.Start(ctx)
	cancel()
	// Give goroutines a moment to observe the cancel.
	time.Sleep(50 * time.Millisecond)

	run, err := s.EnqueueManual(context.Background(), "marketing")
	if err != nil {
		t.Fatalf("enqueue manual: %v", err)
	}
	if run.Trigger != TriggerManual {
		t.Errorf("trigger = %s, want manual", run.Trigger)
	}
	var n int
	_ = db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM runs WHERE persona='marketing' AND trigger='manual'`,
	).Scan(&n)
	if n != 1 {
		t.Errorf("expected 1 manual row, got %d", n)
	}
}

func TestScheduler_EnqueueManual_DisabledPersona_Rejects(t *testing.T) {
	db := openTestDB(t)
	_, _ = db.ExecContext(context.Background(),
		`UPDATE agents SET enabled=0 WHERE persona='marketing'`)
	st := &store.Store{DB: db}
	s := &Scheduler{Store: st, Now: time.Now, Out: &bytes.Buffer{}}
	ctx, cancel := context.WithCancel(context.Background())
	_ = s.Start(ctx)
	cancel()
	time.Sleep(50 * time.Millisecond)

	_, err := s.EnqueueManual(context.Background(), "marketing")
	if err == nil {
		t.Fatalf("expected error for disabled persona, got nil")
	}
}

func TestScheduler_EnqueueManual_DisabledPersona_ReturnsSentinel(t *testing.T) {
	db := openTestDB(t)
	_, _ = db.ExecContext(context.Background(),
		`UPDATE agents SET enabled=0 WHERE persona='marketing'`)
	st := &store.Store{DB: db}
	s := &Scheduler{Store: st, Now: time.Now} // Note: no Out — must not panic
	ctx, cancel := context.WithCancel(context.Background())
	_ = s.Start(ctx)
	cancel()
	time.Sleep(50 * time.Millisecond)

	_, err := s.EnqueueManual(context.Background(), "marketing")
	if !errors.Is(err, ErrPersonaUnavailable) {
		t.Fatalf("want ErrPersonaUnavailable, got %v", err)
	}
}

// TestScheduler_Start_SweepsOrphanedRunning seeds a `running` row from a
// hypothetical prior daemon process and verifies Start() marks it
// failed_permanent before the new worker takes over. Without this, the
// row would block hasActiveRun forever and the persona would silently
// stop receiving tick enqueues.
func TestScheduler_Start_SweepsOrphanedRunning(t *testing.T) {
	db := openTestDB(t)
	now := time.Unix(1700000000, 0).UTC()
	// Insert an orphan: claimed_at set, completed_at NULL, status='running'.
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO runs (id, persona, trigger, status, attempt, scheduled_at, claimed_at, created_at)
		VALUES ('orphan-1', 'marketing', 'manual', 'running', 1, ?, ?, ?)
	`, now.Add(-5*time.Minute).Format(time.RFC3339),
		now.Add(-5*time.Minute).Format(time.RFC3339),
		now.Add(-5*time.Minute).Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seed orphan: %v", err)
	}

	st := &store.Store{DB: db}
	s := &Scheduler{Store: st, Now: func() time.Time { return now }, Out: &bytes.Buffer{}}
	ctx, cancel := context.WithCancel(context.Background())
	_ = s.Start(ctx)
	cancel()
	time.Sleep(50 * time.Millisecond)

	var status, reason, class string
	err = db.QueryRowContext(context.Background(),
		`SELECT status, COALESCE(failure_reason,''), COALESCE(failure_class,'') FROM runs WHERE id='orphan-1'`,
	).Scan(&status, &reason, &class)
	if err != nil {
		t.Fatalf("read orphan: %v", err)
	}
	if status != "failed_permanent" {
		t.Errorf("orphan status = %q, want failed_permanent", status)
	}
	if reason == "" {
		t.Errorf("orphan failure_reason should be set")
	}
	if class != "permanent" {
		t.Errorf("orphan failure_class = %q, want permanent", class)
	}
}

// TestScheduler_Start_DoesNotTouchTerminalRows verifies the sweep only
// targets `running` rows — succeeded/skipped/failed/queued must be left
// alone.
func TestScheduler_Start_DoesNotTouchTerminalRows(t *testing.T) {
	db := openTestDB(t)
	now := time.Unix(1700000000, 0).UTC()
	for _, status := range []string{"succeeded", "skipped", "failed", "failed_permanent", "queued"} {
		_, err := db.ExecContext(context.Background(), `
			INSERT INTO runs (id, persona, trigger, status, attempt, scheduled_at, created_at)
			VALUES (?, 'marketing', 'manual', ?, 1, ?, ?)
		`, "pre-"+status, status,
			now.Add(-5*time.Minute).Format(time.RFC3339),
			now.Add(-5*time.Minute).Format(time.RFC3339))
		if err != nil {
			t.Fatalf("seed %s: %v", status, err)
		}
	}

	st := &store.Store{DB: db}
	s := &Scheduler{Store: st, Now: func() time.Time { return now }, Out: &bytes.Buffer{}}
	ctx, cancel := context.WithCancel(context.Background())
	_ = s.Start(ctx)
	cancel()
	time.Sleep(50 * time.Millisecond)

	for _, want := range []string{"succeeded", "skipped", "failed", "failed_permanent", "queued"} {
		var got string
		_ = db.QueryRowContext(context.Background(),
			`SELECT status FROM runs WHERE id=?`, "pre-"+want,
		).Scan(&got)
		if got != want {
			t.Errorf("row pre-%s: status = %q, want %q (sweep mutated a non-running row)", want, got, want)
		}
	}
}
