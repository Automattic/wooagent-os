package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// HasOpenWorkFunc tells the Loop whether a persona already has open issues.
// Production wires it to personas.HasOpenWork.
type HasOpenWorkFunc func(ctx context.Context, persona string) (bool, error)

// Loop is the periodic tick driver. One Loop per Scheduler.
type Loop struct {
	DB            *sql.DB
	Queue         *Queue
	Now           func() time.Time
	TickEvery     time.Duration
	HasOpenWorkFn HasOpenWorkFunc
}

// Run blocks until ctx is done. Calls tickOnce every TickEvery.
func (l *Loop) Run(ctx context.Context) error {
	if l.TickEvery == 0 {
		l.TickEvery = DefaultTickInterval
	}
	ticker := time.NewTicker(l.TickEvery)
	defer ticker.Stop()
	// Tick immediately so a fresh daemon doesn't wait a full interval to
	// bootstrap.
	_ = l.tickOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			_ = l.tickOnce(ctx)
		}
	}
}

// agentSchedule mirrors the columns we read for each enabled agent.
type agentSchedule struct {
	Persona        string
	CadenceSeconds int
	LastRunAt      sql.NullString
}

// tickOnce makes one scheduling decision per enabled persona.
func (l *Loop) tickOnce(ctx context.Context) error {
	now := l.Now().UTC()
	rows, err := l.DB.QueryContext(ctx, `
		SELECT persona, cadence_seconds, last_run_at
		FROM agents WHERE enabled = 1
	`)
	if err != nil {
		return fmt.Errorf("select agents: %w", err)
	}
	defer rows.Close()

	var schedules []agentSchedule
	for rows.Next() {
		var a agentSchedule
		if err := rows.Scan(&a.Persona, &a.CadenceSeconds, &a.LastRunAt); err != nil {
			return fmt.Errorf("scan agent: %w", err)
		}
		schedules = append(schedules, a)
	}

	for _, a := range schedules {
		// One persona's failure must not block the rest.
		_ = l.considerPersona(ctx, a, now)
	}
	return nil
}

func (l *Loop) considerPersona(ctx context.Context, a agentSchedule, now time.Time) error {
	// Bootstrap: never run before → enqueue a bootstrap run.
	if !a.LastRunAt.Valid {
		if has, _ := l.hasActiveRun(ctx, a.Persona); has {
			return nil
		}
		_, err := l.Queue.Enqueue(ctx, EnqueueParams{
			Persona: a.Persona, Trigger: TriggerBootstrap, ScheduledAt: now,
		})
		return err
	}

	last, err := time.Parse(time.RFC3339, a.LastRunAt.String)
	if err != nil {
		return fmt.Errorf("parse last_run_at: %w", err)
	}
	nextDue := last.Add(time.Duration(a.CadenceSeconds) * time.Second)
	if nextDue.After(now) {
		return nil // not yet due
	}
	// Don't enqueue if a row is already queued/running for this persona.
	if has, _ := l.hasActiveRun(ctx, a.Persona); has {
		return nil
	}
	// If the persona already has open issues, write a skip row instead of
	// a tick — operator sees the reasoning.
	if l.HasOpenWorkFn != nil {
		openWork, err := l.HasOpenWorkFn(ctx, a.Persona)
		if err != nil {
			return fmt.Errorf("has open work: %w", err)
		}
		if openWork {
			return l.writeSkipRow(ctx, a.Persona, now, "persona already has open proposals; not enqueuing")
		}
	}
	_, err = l.Queue.Enqueue(ctx, EnqueueParams{
		Persona: a.Persona, Trigger: TriggerTick, ScheduledAt: now,
	})
	return err
}

func (l *Loop) hasActiveRun(ctx context.Context, persona string) (bool, error) {
	var n int
	err := l.DB.QueryRowContext(ctx, `
		SELECT count(*) FROM runs
		WHERE persona=? AND status IN ('queued','running')
	`, persona).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// writeSkipRow inserts a row and immediately marks it skipped so the
// operator sees the would-have-run-but reasoning.
func (l *Loop) writeSkipRow(ctx context.Context, persona string, now time.Time, reason string) error {
	r, err := l.Queue.Enqueue(ctx, EnqueueParams{
		Persona: persona, Trigger: TriggerTick, ScheduledAt: now,
	})
	if err != nil {
		return err
	}
	return l.Queue.MarkTerminal(ctx, MarkTerminalParams{
		ID:          r.ID,
		Status:      StatusSkipped,
		CompletedAt: now,
		LatencyMS:   0,
		SkipReason:  reason,
	})
}
