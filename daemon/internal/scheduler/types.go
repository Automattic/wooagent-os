// Package scheduler owns the SQLite-backed run queue, the periodic tick
// loop, and the single-worker run executor. The worker calls into
// personas.RunAndPersist — the scheduler has no business logic of its own.
package scheduler

import "time"

// Status is the lifecycle state of a single Run row.
type Status string

const (
	StatusQueued          Status = "queued"
	StatusRunning         Status = "running"
	StatusSucceeded       Status = "succeeded"
	StatusSkipped         Status = "skipped"
	StatusFailed          Status = "failed"
	StatusFailedPermanent Status = "failed_permanent"
)

// Trigger is how a Run came into existence.
type Trigger string

const (
	TriggerTick      Trigger = "tick"
	TriggerManual    Trigger = "manual"
	TriggerBootstrap Trigger = "bootstrap"
	TriggerRetry     Trigger = "retry"
)

// FailureClass narrows how a failure should be handled. Transient → retry
// with backoff; Permanent → fail fast with reason; Unknown is only used
// internally before classify resolves to one of the above.
type FailureClass string

const (
	FailureUnknown   FailureClass = ""
	FailureTransient FailureClass = "transient"
	FailurePermanent FailureClass = "permanent"
)

// Run is the wire + DB shape. Time fields are RFC3339 strings on the wire
// and time.Time in Go; nullable columns become pointer types.
type Run struct {
	ID            string       `json:"id"`
	Persona       string       `json:"persona"`
	Trigger       Trigger      `json:"trigger"`
	Status        Status       `json:"status"`
	Attempt       int          `json:"attempt"`
	RetryOf       *string      `json:"retry_of"`
	ScheduledAt   time.Time    `json:"scheduled_at"`
	ClaimedAt     *time.Time   `json:"claimed_at"`
	CompletedAt   *time.Time   `json:"completed_at"`
	LatencyMS     *int64       `json:"latency_ms"`
	IssueID       *string      `json:"issue_id"`
	TurnID        *string      `json:"turn_id"`
	SkipReason    *string      `json:"skip_reason"`
	FailureReason *string      `json:"failure_reason"`
	FailureClass  FailureClass `json:"failure_class"`
	CreatedAt     time.Time    `json:"created_at"`
}

// DefaultBackoff is the delay schedule applied to retries. Index 0 is the
// delay applied after the first failure (between attempt 1 and 2).
var DefaultBackoff = []time.Duration{5 * time.Minute, 30 * time.Minute, 2 * time.Hour}

// DefaultTickInterval is how often Loop wakes up to consider due personas.
const DefaultTickInterval = 60 * time.Second

// DefaultCadence is the per-persona default if a row hasn't customized.
// Mirrors the column default in the migration.
const DefaultCadence = 6 * time.Hour
