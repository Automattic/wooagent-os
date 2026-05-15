package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/pep"
	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

// ErrPersonaUnavailable is returned by EnqueueManual when the persona is
// either disabled in the agents table or absent entirely. HTTP handlers
// map this to 409.
var ErrPersonaUnavailable = errors.New("scheduler: persona disabled or unknown")

// Scheduler bundles the Loop and Worker. One per daemon process.
type Scheduler struct {
	Store       *store.Store
	Personas    []personas.Persona
	Deps        personas.Deps
	Now         func() time.Time
	TickEvery   time.Duration
	Backoff     []time.Duration
	MaxAttempts int
	Out         io.Writer
	// Budget is passed through to the Worker so it can skip over-budget
	// personas before invoking the LLM. Nil disables the pre-tick gate.
	Budget *pep.BudgetGate

	queue  *Queue
	loop   *Loop
	worker *Worker
}

// Start spawns the Loop and Worker goroutines. Returns immediately; cancel
// via ctx.
func (s *Scheduler) Start(ctx context.Context) error {
	if s.Now == nil {
		s.Now = time.Now
	}
	if s.TickEvery == 0 {
		s.TickEvery = DefaultTickInterval
	}
	if len(s.Backoff) == 0 {
		s.Backoff = DefaultBackoff
	}
	if s.MaxAttempts == 0 {
		s.MaxAttempts = 3
	}
	if s.Out == nil {
		s.Out = io.Discard
	}
	s.queue = &Queue{DB: s.Store.DB, Now: s.Now}

	// Sweep orphaned `running` rows from a prior daemon process before
	// starting the worker. A run is marked `running` the moment a worker
	// claims it; if the daemon dies (Ctrl+C, crash, OS restart) before
	// MarkTerminal lands, the row stays `running` forever — and the
	// Loop's hasActiveRun count then blocks all future tick enqueues for
	// that persona. Mark them failed_permanent so the operator sees what
	// happened and the scheduler unblocks. Queued rows don't need this;
	// the new worker claims them naturally.
	if swept, err := s.sweepOrphanedRunning(ctx); err != nil {
		fmt.Fprintf(s.Out, "→ scheduler: orphan sweep failed: %v\n", err)
	} else if swept > 0 {
		fmt.Fprintf(s.Out, "→ scheduler: marked %d orphaned 'running' run(s) as failed_permanent (prior daemon process interrupted)\n", swept)
	}

	pmap := map[string]personas.Persona{}
	for _, p := range s.Personas {
		pmap[p.Slug()] = p
	}

	s.worker = &Worker{
		Queue:       s.queue,
		Runner:      personaRunnerAdapter{deps: s.Deps},
		Personas:    pmap,
		Now:         s.Now,
		Backoff:     s.Backoff,
		MaxAttempts: s.MaxAttempts,
		Budget:      s.Budget,
	}
	s.loop = &Loop{
		DB:        s.Store.DB,
		Queue:     s.queue,
		Now:       s.Now,
		TickEvery: s.TickEvery,
		HasOpenWorkFn: func(ctx context.Context, persona string) (bool, error) {
			return personas.HasOpenWork(ctx, s.Store, persona)
		},
	}

	go func() {
		if err := s.loop.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(s.Out, "→ scheduler: loop stopped: %v\n", err)
		}
	}()
	go func() {
		if err := s.worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(s.Out, "→ scheduler: worker stopped: %v\n", err)
		}
	}()
	fmt.Fprintln(s.Out, "→ scheduler: started")
	return nil
}

// EnqueueManual is the API/CLI entry point for manual runs.
func (s *Scheduler) EnqueueManual(ctx context.Context, personaSlug string) (Run, error) {
	if s.queue == nil {
		return Run{}, fmt.Errorf("scheduler: not started")
	}
	enabled, err := personaEnabled(ctx, s.Store.DB, personaSlug)
	if err != nil {
		return Run{}, err
	}
	if !enabled {
		return Run{}, fmt.Errorf("%w: %q", ErrPersonaUnavailable, personaSlug)
	}
	return s.queue.Enqueue(ctx, EnqueueParams{
		Persona: personaSlug, Trigger: TriggerManual, ScheduledAt: s.Now(),
	})
}

func personaEnabled(ctx context.Context, db *sql.DB, slug string) (bool, error) {
	var enabled int
	err := db.QueryRowContext(ctx, `SELECT enabled FROM agents WHERE persona=?`, slug).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return enabled == 1, nil
}

// sweepOrphanedRunning marks every `running` row as `failed_permanent`
// with a descriptive failure_reason. Called once at scheduler startup —
// any row left in `running` belongs to a prior daemon process that was
// killed before the worker could record a terminal status. Returns the
// number of rows updated for observability.
func (s *Scheduler) sweepOrphanedRunning(ctx context.Context) (int64, error) {
	now := s.Now().UTC().Format(time.RFC3339)
	res, err := s.Store.DB.ExecContext(ctx, `
		UPDATE runs
		   SET status = 'failed_permanent',
		       completed_at = ?,
		       failure_reason = 'daemon process exited before run completed',
		       failure_class = 'permanent'
		 WHERE status = 'running'
	`, now)
	if err != nil {
		return 0, fmt.Errorf("sweep orphans: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// personaRunnerAdapter wraps personas.RunAndPersist behind PersonaRunner.
type personaRunnerAdapter struct {
	deps personas.Deps
}

func (a personaRunnerAdapter) Run(ctx context.Context, p personas.Persona) (personas.Result, error) {
	return personas.RunAndPersist(ctx, p, a.deps)
}
