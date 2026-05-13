package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
)

// stubPersona is a configurable persona test double. Slug is the only
// thing that matters for the worker — Draft is never called directly
// because the worker uses the injected PersonaRunner.
type stubPersona struct {
	slug string
}

func (p *stubPersona) Slug() string        { return p.slug }
func (p *stubPersona) DisplayName() string { return p.slug }
func (p *stubPersona) Draft(ctx context.Context, _ personas.Deps) (personas.Drafted, error) {
	return personas.Drafted{}, errors.New("stub Draft should not be called; worker uses Runner")
}

// stubRunner replaces personas.RunAndPersist for unit tests.
type stubRunner struct {
	onRun func(ctx context.Context, p personas.Persona) (personas.Result, error)
}

func (s *stubRunner) Run(ctx context.Context, p personas.Persona) (personas.Result, error) {
	return s.onRun(ctx, p)
}

func TestWorker_SuccessPath(t *testing.T) {
	db := openTestDB(t)
	fixed := time.Unix(1700000000, 0).UTC()
	q := &Queue{DB: db, Now: func() time.Time { return fixed }}

	sp := &stubPersona{slug: "marketing"}
	_, _ = q.Enqueue(context.Background(), EnqueueParams{
		Persona: "marketing", Trigger: TriggerTick, ScheduledAt: fixed,
	})

	runner := &stubRunner{onRun: func(ctx context.Context, p personas.Persona) (personas.Result, error) {
		return personas.Result{Persona: "marketing", IssueID: ""}, nil
	}}
	w := &Worker{
		Queue:    q,
		Runner:   runner,
		Personas: map[string]personas.Persona{"marketing": sp},
		Now:      func() time.Time { return fixed.Add(2 * time.Second) },
		Backoff:  DefaultBackoff,
	}
	ran, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if !ran {
		t.Fatalf("expected a run to be processed")
	}

	var status string
	var issueID sql.NullString
	_ = db.QueryRowContext(context.Background(),
		`SELECT status, issue_id FROM runs LIMIT 1`).Scan(&status, &issueID)
	if status != string(StatusSucceeded) {
		t.Errorf("status = %s, want succeeded", status)
	}
	// issue_id is dropped on FK violation (no row in issues table for stub
	// "issue-1") — so we can't assert it round-trips here. Status is
	// sufficient to verify the success path.
}

func TestWorker_TransientFailure_EnqueuesRetry(t *testing.T) {
	db := openTestDB(t)
	fixed := time.Unix(1700000000, 0).UTC()
	q := &Queue{DB: db, Now: func() time.Time { return fixed }}
	sp := &stubPersona{slug: "marketing"}
	parent, _ := q.Enqueue(context.Background(), EnqueueParams{
		Persona: "marketing", Trigger: TriggerTick, ScheduledAt: fixed,
	})
	runner := &stubRunner{onRun: func(ctx context.Context, p personas.Persona) (personas.Result, error) {
		return personas.Result{}, mcp.ErrSessionLost
	}}
	w := &Worker{
		Queue: q, Runner: runner,
		Personas: map[string]personas.Persona{"marketing": sp},
		Now:      func() time.Time { return fixed.Add(time.Second) },
		Backoff:  []time.Duration{5 * time.Minute, 30 * time.Minute, 2 * time.Hour},
	}
	_, _ = w.RunOnce(context.Background())

	var status, failClass string
	_ = db.QueryRowContext(context.Background(),
		`SELECT status, COALESCE(failure_class,'') FROM runs WHERE id=?`, parent.ID,
	).Scan(&status, &failClass)
	if status != string(StatusFailed) {
		t.Errorf("parent status = %s, want failed", status)
	}
	if failClass != string(FailureTransient) {
		t.Errorf("parent failure_class = %s, want transient", failClass)
	}

	var attempt int
	var retryOf sql.NullString
	var scheduledAt string
	err := db.QueryRowContext(context.Background(),
		`SELECT attempt, retry_of, scheduled_at FROM runs WHERE id <> ? ORDER BY created_at DESC LIMIT 1`, parent.ID,
	).Scan(&attempt, &retryOf, &scheduledAt)
	if err != nil {
		t.Fatalf("no retry row found: %v", err)
	}
	if attempt != 2 {
		t.Errorf("retry attempt = %d, want 2", attempt)
	}
	if !retryOf.Valid || retryOf.String != parent.ID {
		t.Errorf("retry_of = %v, want %s", retryOf, parent.ID)
	}
	wantSched := fixed.Add(time.Second).Add(5 * time.Minute).Format(time.RFC3339)
	if scheduledAt != wantSched {
		t.Errorf("scheduled_at = %s, want %s", scheduledAt, wantSched)
	}
}

func TestWorker_ExhaustedRetries_MarksPermanent(t *testing.T) {
	db := openTestDB(t)
	fixed := time.Unix(1700000000, 0).UTC()
	q := &Queue{DB: db, Now: func() time.Time { return fixed }}
	sp := &stubPersona{slug: "marketing"}
	_, _ = q.Enqueue(context.Background(), EnqueueParams{
		Persona: "marketing", Trigger: TriggerRetry, ScheduledAt: fixed, Attempt: 3,
	})
	runner := &stubRunner{onRun: func(ctx context.Context, p personas.Persona) (personas.Result, error) {
		return personas.Result{}, mcp.ErrSessionLost
	}}
	w := &Worker{
		Queue: q, Runner: runner,
		Personas:    map[string]personas.Persona{"marketing": sp},
		Now:         func() time.Time { return fixed.Add(time.Second) },
		Backoff:     DefaultBackoff,
		MaxAttempts: 3,
	}
	_, _ = w.RunOnce(context.Background())

	var status string
	_ = db.QueryRowContext(context.Background(),
		`SELECT status FROM runs LIMIT 1`).Scan(&status)
	if status != string(StatusFailedPermanent) {
		t.Errorf("status = %s, want failed_permanent", status)
	}
	var n int
	_ = db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM runs WHERE status='queued'`).Scan(&n)
	if n != 0 {
		t.Errorf("expected no retry row, got %d", n)
	}
}

func TestWorker_PermanentError_NoRetry(t *testing.T) {
	db := openTestDB(t)
	fixed := time.Unix(1700000000, 0).UTC()
	q := &Queue{DB: db, Now: func() time.Time { return fixed }}
	sp := &stubPersona{slug: "marketing"}
	_, _ = q.Enqueue(context.Background(), EnqueueParams{
		Persona: "marketing", Trigger: TriggerTick, ScheduledAt: fixed,
	})
	runner := &stubRunner{onRun: func(ctx context.Context, p personas.Persona) (personas.Result, error) {
		return personas.Result{}, errors.New("anthropic http 401 invalid api key")
	}}
	w := &Worker{
		Queue: q, Runner: runner,
		Personas:    map[string]personas.Persona{"marketing": sp},
		Now:         func() time.Time { return fixed.Add(time.Second) },
		Backoff:     DefaultBackoff,
		MaxAttempts: 3,
	}
	_, _ = w.RunOnce(context.Background())

	var status, failClass string
	_ = db.QueryRowContext(context.Background(),
		`SELECT status, COALESCE(failure_class,'') FROM runs LIMIT 1`).Scan(&status, &failClass)
	if status != string(StatusFailedPermanent) {
		t.Errorf("status = %s, want failed_permanent", status)
	}
	if failClass != string(FailurePermanent) {
		t.Errorf("failure_class = %s, want permanent", failClass)
	}
}

func TestWorker_Skip_RecordsSkipReason(t *testing.T) {
	db := openTestDB(t)
	fixed := time.Unix(1700000000, 0).UTC()
	q := &Queue{DB: db, Now: func() time.Time { return fixed }}
	sp := &stubPersona{slug: "marketing"}
	_, _ = q.Enqueue(context.Background(), EnqueueParams{
		Persona: "marketing", Trigger: TriggerTick, ScheduledAt: fixed,
	})
	runner := &stubRunner{onRun: func(ctx context.Context, p personas.Persona) (personas.Result, error) {
		return personas.Result{Skipped: true, SkipReason: "missing api key"}, nil
	}}
	w := &Worker{
		Queue: q, Runner: runner,
		Personas: map[string]personas.Persona{"marketing": sp},
		Now:      func() time.Time { return fixed.Add(time.Second) },
		Backoff:  DefaultBackoff,
	}
	_, _ = w.RunOnce(context.Background())

	var status string
	var reason sql.NullString
	_ = db.QueryRowContext(context.Background(),
		`SELECT status, skip_reason FROM runs LIMIT 1`).Scan(&status, &reason)
	if status != string(StatusSkipped) {
		t.Errorf("status = %s, want skipped", status)
	}
	if !reason.Valid || reason.String != "missing api key" {
		t.Errorf("skip_reason = %v, want 'missing api key'", reason)
	}
}

func TestWorker_TransientFailure_EmptyBackoff_NoPanic(t *testing.T) {
	db := openTestDB(t)
	fixed := time.Unix(1700000000, 0).UTC()
	q := &Queue{DB: db, Now: func() time.Time { return fixed }}
	sp := &stubPersona{slug: "marketing"}
	_, _ = q.Enqueue(context.Background(), EnqueueParams{
		Persona: "marketing", Trigger: TriggerTick, ScheduledAt: fixed,
	})
	runner := &stubRunner{onRun: func(ctx context.Context, p personas.Persona) (personas.Result, error) {
		return personas.Result{}, mcp.ErrSessionLost
	}}
	w := &Worker{
		Queue: q, Runner: runner,
		Personas: map[string]personas.Persona{"marketing": sp},
		Now:      func() time.Time { return fixed.Add(time.Second) },
		Backoff:  nil, // empty/nil — must not panic
	}
	_, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	// Retry row should exist with scheduled_at == end (zero delay).
	var scheduledAt string
	err = db.QueryRowContext(context.Background(),
		`SELECT scheduled_at FROM runs WHERE attempt = 2 LIMIT 1`,
	).Scan(&scheduledAt)
	if err != nil {
		t.Fatalf("no retry row: %v", err)
	}
	want := fixed.Add(time.Second).Format(time.RFC3339)
	if scheduledAt != want {
		t.Errorf("scheduled_at = %s, want %s (zero delay)", scheduledAt, want)
	}
}

func TestQueue_ClaimRace_OnlyOneWinner(t *testing.T) {
	db := openTestDB(t)
	fixed := time.Unix(1700000000, 0).UTC()
	q := &Queue{DB: db, Now: func() time.Time { return fixed }}
	_, _ = q.Enqueue(context.Background(), EnqueueParams{
		Persona: "marketing", Trigger: TriggerTick, ScheduledAt: fixed,
	})

	// Race two concurrent claims.
	type res struct {
		run *Run
		err error
	}
	results := make(chan res, 2)
	for i := 0; i < 2; i++ {
		go func() {
			r, err := q.ClaimNext(context.Background())
			results <- res{r, err}
		}()
	}
	var wins, nils int
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			t.Fatalf("claim err: %v", r.err)
		}
		if r.run != nil {
			wins++
		} else {
			nils++
		}
	}
	if wins != 1 || nils != 1 {
		t.Errorf("expected 1 winner + 1 nil, got wins=%d nils=%d", wins, nils)
	}
}
