package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

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
	s.queue = &Queue{DB: s.Store.DB, Now: s.Now}

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
		return Run{}, fmt.Errorf("persona %q is disabled or unknown", personaSlug)
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

// personaRunnerAdapter wraps personas.RunAndPersist behind PersonaRunner.
type personaRunnerAdapter struct {
	deps personas.Deps
}

func (a personaRunnerAdapter) Run(ctx context.Context, p personas.Persona) (personas.Result, error) {
	return personas.RunAndPersist(ctx, p, a.deps)
}
