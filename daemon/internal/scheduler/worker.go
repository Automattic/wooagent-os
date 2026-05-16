package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
	"github.com/wooagent-os/wooagent-os/daemon/internal/pep"
	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
)

// PersonaRunner is the interface the worker uses to actually run a persona.
// Production wires it to personas.RunAndPersist via personaRunnerAdapter
// (see scheduler.go in Task 7). Tests inject a stub.
type PersonaRunner interface {
	Run(ctx context.Context, p personas.Persona, opts RunOpts) (personas.Result, error)
}

// RunOpts carries per-run knobs the worker derives from the Run row before
// dispatching to the runner. Kept narrow on purpose — anything that lives
// inside personas.Deps stays out of here.
type RunOpts struct {
	// EmitCount is how many proposals this run should produce. 1 by
	// default; the worker sets 2 for TriggerBootstrap so the operator's
	// queue isn't empty right after onboarding. The runner translates
	// this into personas.Deps.MaxEmits.
	EmitCount int
}

// bootstrapEmitCount is the number of proposals each persona produces on
// its very first run after onboarding (TriggerBootstrap). 2 gives the
// operator something to scan + compare on day one without overwhelming
// the queue.
const bootstrapEmitCount = 2

// Worker is the single-goroutine claim-and-run loop. One Worker per
// Scheduler — MCP session-id forces serial.
type Worker struct {
	Queue       *Queue
	Runner      PersonaRunner
	Personas    map[string]personas.Persona // persona slug → registered impl
	Now         func() time.Time
	Backoff     []time.Duration
	MaxAttempts int // default 3 if zero
	// Budget is the PEP budget gate. When non-nil, RunOnce checks the gate
	// before dispatching a claimed run; over-budget runs are marked Skipped
	// immediately without invoking the LLM.
	Budget *pep.BudgetGate
}

// RunOnce claims the next due run and processes it. Returns ran=true when a
// row was processed. Used in tests and by Run() in production.
func (w *Worker) RunOnce(ctx context.Context) (ran bool, err error) {
	if w.MaxAttempts == 0 {
		w.MaxAttempts = 3
	}
	r, err := w.Queue.ClaimNext(ctx)
	if err != nil {
		return false, err
	}
	if r == nil {
		return false, nil
	}
	persona, ok := w.Personas[r.Persona]
	if !ok {
		_ = w.markPermanent(ctx, r, fmt.Sprintf("no implementation registered for persona %q", r.Persona))
		return true, nil
	}
	// Pre-tick budget check. The PEP gate is the backstop; this stops the
	// LLM from being called at all for an over-budget persona. Match the
	// existing skip path in executeAndRecord — terminal status=Skipped,
	// reason describes the budget block.
	if w.Budget != nil {
		reason, checkErr := w.Budget.Check(ctx, manifest.Persona(r.Persona))
		if checkErr != nil || reason != "" {
			slog.Info("scheduler skipped tick over budget",
				"persona", r.Persona,
				"reason", string(reason),
				"err", checkErr,
			)
			end := w.Now()
			_ = w.Queue.MarkTerminal(ctx, MarkTerminalParams{
				ID:          r.ID,
				Status:      StatusSkipped,
				CompletedAt: end,
				LatencyMS:   0,
				SkipReason:  "over daily budget — counters reset at local midnight",
			})
			return true, nil
		}
	}
	return true, w.executeAndRecord(ctx, r, persona)
}

// Run blocks until ctx is done. Polls the queue every 250ms when idle.
func (w *Worker) Run(ctx context.Context) error {
	idleSleep := 250 * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		ran, err := w.RunOnce(ctx)
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(idleSleep):
			}
			continue
		}
		if !ran {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(idleSleep):
			}
		}
	}
}

func (w *Worker) executeAndRecord(ctx context.Context, r *Run, persona personas.Persona) error {
	start := w.Now()
	opts := RunOpts{EmitCount: 1}
	if r.Trigger == TriggerBootstrap {
		opts.EmitCount = bootstrapEmitCount
	}
	res, runErr := w.Runner.Run(ctx, persona, opts)
	end := w.Now()

	// Skip path — succeeded with a reason but no issue.
	if runErr == nil && res.Skipped {
		return w.Queue.MarkTerminal(ctx, MarkTerminalParams{
			ID:          r.ID,
			Status:      StatusSkipped,
			CompletedAt: end,
			LatencyMS:   end.Sub(start).Milliseconds(),
			SkipReason:  res.SkipReason,
		})
	}
	// Success path.
	if runErr == nil {
		// A multi-emit run can come back successful overall but with a
		// best-effort SkipReason describing a partial: e.g. "emit 2/2
		// skipped: no eligible orders". Surface that on the run row so the
		// operator sees why the second proposal didn't land. Status stays
		// `succeeded` — at least one proposal made it.
		return w.Queue.MarkTerminal(ctx, MarkTerminalParams{
			ID:          r.ID,
			Status:      StatusSucceeded,
			CompletedAt: end,
			LatencyMS:   end.Sub(start).Milliseconds(),
			IssueID:     res.IssueID,
			SkipReason:  res.SkipReason,
			// TurnID flows through telemetry; left empty here because
			// personas.RunAndPersist owns the turn_event write. The link is
			// joined by issue_id in the API layer for V1.
		})
	}

	// Failure path.
	cls, reason := classify(runErr)
	isPermanent := cls == FailurePermanent || r.Attempt >= w.MaxAttempts

	status := StatusFailed
	if isPermanent {
		status = StatusFailedPermanent
	}
	if err := w.Queue.MarkTerminal(ctx, MarkTerminalParams{
		ID:            r.ID,
		Status:        status,
		CompletedAt:   end,
		LatencyMS:     end.Sub(start).Milliseconds(),
		FailureReason: reason,
		FailureClass:  cls,
	}); err != nil {
		return err
	}
	if isPermanent {
		return nil
	}
	// Enqueue the retry.
	var delay time.Duration
	if len(w.Backoff) > 0 {
		backoffIdx := r.Attempt - 1
		if backoffIdx >= len(w.Backoff) {
			backoffIdx = len(w.Backoff) - 1
		}
		delay = w.Backoff[backoffIdx]
	}
	parentID := r.ID
	_, err := w.Queue.Enqueue(ctx, EnqueueParams{
		Persona:     r.Persona,
		Trigger:     TriggerRetry,
		ScheduledAt: end.Add(delay),
		Attempt:     r.Attempt + 1,
		RetryOf:     &parentID,
	})
	return err
}

func (w *Worker) markPermanent(ctx context.Context, r *Run, reason string) error {
	return w.Queue.MarkTerminal(ctx, MarkTerminalParams{
		ID:            r.ID,
		Status:        StatusFailedPermanent,
		CompletedAt:   w.Now(),
		LatencyMS:     0,
		FailureReason: reason,
		FailureClass:  FailurePermanent,
	})
}
