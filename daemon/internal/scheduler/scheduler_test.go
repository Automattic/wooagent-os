package scheduler

import (
	"bytes"
	"context"
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
