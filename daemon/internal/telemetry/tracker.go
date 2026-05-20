package telemetry

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ErrRunBudgetExceeded is returned by Tracker.RecordModelCall when the
// accumulated cost for the current run would exceed the configured
// per-run cap. The scheduler classifies any wrapping of this sentinel as
// a permanent failure (no retries) and surfaces the human-readable
// reason via runs.failure_reason. Use errors.Is to identify it.
var ErrRunBudgetExceeded = errors.New("run budget exceeded")

// Tracker accumulates the model + skill calls a persona makes during one
// agent turn, then materializes them into a single TurnEvent at the end of
// the turn. Tracker is the v1 GEPA scaffold the central pipeline ingests
// (see DSGWOO-1236 + wooagent-os-gepa-v2-plan.md §"v1 scaffolding").
//
// Lifecycle:
//
//  1. RunAndPersist calls NewTracker at the start of a turn.
//  2. Tracker is attached to the context via WithTracker so persona helpers
//     (callAbility, draftRewrite*) can append events without signature
//     changes.
//  3. Helpers call RecordSkillCall / RecordModelCall after each network call.
//  4. RunAndPersist calls Finalize at end of turn to get the TurnEvent and
//     hands it to the Recorder.
type Tracker struct {
	turnID    string
	persona   string
	startedAt time.Time
	mu        sync.Mutex
	model     []ModelCall
	skill     []SkillCall
	// budgetUSD is the per-run cost cap. Zero (the default) disables
	// enforcement — tests and ad-hoc personas that don't care about budget
	// keep working without setting it. Configured per persona via the
	// agents.run_budget_cents column and propagated through Deps.
	budgetUSD    float64
	totalCostUSD float64
}

// NewTracker starts a fresh turn. turnID is the row's primary key; pass
// uuid.NewString() (or your caller's uuid) so the turn's verdict can be
// joined later. persona is the slug (marketing, pricing, sales_support).
func NewTracker(turnID, persona string) *Tracker {
	if turnID == "" {
		turnID = uuid.NewString()
	}
	return &Tracker{
		turnID:    turnID,
		persona:   persona,
		startedAt: time.Now().UTC(),
	}
}

// TurnID returns the row id assigned to this turn. RunAndPersist uses it to
// stamp the new issue's `latest_turn_id` so verdict updates know which row
// to attach to.
func (t *Tracker) TurnID() string { return t.turnID }

// SetBudgetUSD configures the per-run cost cap. Zero disables enforcement.
// Call once before the persona's first model call; later changes apply
// only to subsequent calls.
func (t *Tracker) SetBudgetUSD(usd float64) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.budgetUSD = usd
	t.mu.Unlock()
}

// TotalCostUSD returns the sum of CostUSD across all model calls
// recorded so far. Mainly useful in tests and as a read-side hook for
// observability — the budget check is performed inline in RecordModelCall.
func (t *Tracker) TotalCostUSD() float64 {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.totalCostUSD
}

// RecordModelCall appends one LLM API call's metadata to this turn. Safe
// to call from any goroutine. Returns ErrRunBudgetExceeded (wrapped with
// the running total + configured cap) when the call pushes the run over
// its per-run budget. Callers must propagate the error so the scheduler
// can classify it as a permanent failure.
func (t *Tracker) RecordModelCall(c ModelCall) error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.model = append(t.model, c)
	t.totalCostUSD += c.CostUSD
	if t.budgetUSD > 0 && t.totalCostUSD > t.budgetUSD {
		return fmt.Errorf("%w: total $%.4f exceeds cap $%.2f",
			ErrRunBudgetExceeded, t.totalCostUSD, t.budgetUSD)
	}
	return nil
}

// RecordSkillCall appends one MCP ability invocation's metadata to this
// turn. Safe to call from any goroutine.
func (t *Tracker) RecordSkillCall(c SkillCall) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.skill = append(t.skill, c)
	t.mu.Unlock()
}

// Finalize stamps completion + latency and returns the assembled TurnEvent
// ready for SQLiteRecorder.Record. Caller fills in IssueID + ProposalText
// after insertIssue runs. Safe to call exactly once per turn.
func (t *Tracker) Finalize() TurnEvent {
	if t == nil {
		return TurnEvent{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	completed := time.Now().UTC()
	return TurnEvent{
		TurnID:             t.turnID,
		EventSchemaVersion: EventSchemaVersion,
		Persona:            t.persona,
		StartedAt:          t.startedAt,
		CompletedAt:        &completed,
		LatencyMS:          completed.Sub(t.startedAt).Milliseconds(),
		ModelCalls:         append([]ModelCall(nil), t.model...),
		SkillCalls:         append([]SkillCall(nil), t.skill...),
	}
}

// trackerCtxKey is unexported so callers can't pull it from other packages
// by guessing the key.
type trackerCtxKey struct{}

// WithTracker attaches a Tracker to ctx. Helper functions in the persona
// packages (callAbility, draftRewriteAnthropic, etc.) read it back out via
// TrackerFromContext — keeps their signatures unchanged.
func WithTracker(ctx context.Context, t *Tracker) context.Context {
	if t == nil {
		return ctx
	}
	return context.WithValue(ctx, trackerCtxKey{}, t)
}

// TrackerFromContext returns the Tracker attached to ctx, or nil if none.
// Helpers should call RecordX methods unconditionally — they're nil-safe.
func TrackerFromContext(ctx context.Context) *Tracker {
	t, _ := ctx.Value(trackerCtxKey{}).(*Tracker)
	return t
}

// RecordSkillCallFromError is a convenience for the common shape of an MCP
// ability call: measure the latency, derive status from whether err is nil,
// and truncate the error string to a bounded payload size. Callers can
// pass any context — if no Tracker is attached, the call is a no-op.
func RecordSkillCallFromError(
	ctx context.Context,
	ability string,
	took time.Duration,
	err error,
) {
	t := TrackerFromContext(ctx)
	if t == nil {
		return
	}
	status := "ok"
	var errCode string
	if err != nil {
		status = "error"
		errCode = err.Error()
		if len(errCode) > 200 {
			errCode = errCode[:200]
		}
	}
	t.RecordSkillCall(SkillCall{
		Name:      ability,
		Status:    status,
		LatencyMS: took.Milliseconds(),
		ErrorCode: errCode,
	})
}
