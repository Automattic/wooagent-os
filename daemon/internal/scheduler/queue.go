package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrRunNotFound is returned by Cancel when no run with the given id exists.
var ErrRunNotFound = errors.New("scheduler: run not found")

// ErrRunNotCancellable is returned by Cancel when the run is already in a
// terminal state (succeeded / skipped / failed / failed_permanent). The
// operator can only cancel rows that are still queued or running.
var ErrRunNotCancellable = errors.New("scheduler: run is already terminal")

// Queue wraps the runs table. All scheduler DB access goes through here so
// the worker and loop never write raw SQL.
type Queue struct {
	DB  *sql.DB
	Now func() time.Time
}

// EnqueueParams covers every shape of enqueue: tick, manual, bootstrap,
// retry. The caller decides which Trigger to set.
type EnqueueParams struct {
	Persona     string
	Trigger     Trigger
	ScheduledAt time.Time
	Attempt     int     // defaults to 1
	RetryOf     *string // set for retries
}

// Enqueue inserts a queued run row and returns the populated Run.
func (q *Queue) Enqueue(ctx context.Context, p EnqueueParams) (Run, error) {
	if p.Attempt == 0 {
		p.Attempt = 1
	}
	now := q.Now().UTC()
	r := Run{
		ID:          uuid.NewString(),
		Persona:     p.Persona,
		Trigger:     p.Trigger,
		Status:      StatusQueued,
		Attempt:     p.Attempt,
		RetryOf:     p.RetryOf,
		ScheduledAt: p.ScheduledAt.UTC(),
		CreatedAt:   now,
	}
	_, err := q.DB.ExecContext(ctx, `
		INSERT INTO runs(id, persona, trigger, status, attempt, retry_of, scheduled_at, created_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)
	`,
		r.ID, r.Persona, string(r.Trigger), string(r.Status), r.Attempt,
		nullString(r.RetryOf), r.ScheduledAt.Format(time.RFC3339), r.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return Run{}, fmt.Errorf("enqueue: %w", err)
	}
	return r, nil
}

// ClaimNext atomically transitions the oldest due queued row to running and
// returns it. Returns (nil, nil) if no row is due. Uses RowsAffected==1 to
// guarantee at-most-once delivery if two workers ever race.
func (q *Queue) ClaimNext(ctx context.Context) (*Run, error) {
	now := q.Now().UTC().Format(time.RFC3339)
	var id string
	err := q.DB.QueryRowContext(ctx, `
		SELECT id FROM runs
		WHERE status = 'queued' AND scheduled_at <= ?
		ORDER BY scheduled_at ASC
		LIMIT 1
	`, now).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim select: %w", err)
	}
	res, err := q.DB.ExecContext(ctx, `
		UPDATE runs SET status='running', claimed_at=?
		WHERE id=? AND status='queued'
	`, now, id)
	if err != nil {
		return nil, fmt.Errorf("claim update: %w", err)
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		// Someone else got there first.
		return nil, nil
	}
	return q.Get(ctx, id)
}

// Get loads a single Run by id.
func (q *Queue) Get(ctx context.Context, id string) (*Run, error) {
	row := q.DB.QueryRowContext(ctx, `
		SELECT id, persona, trigger, status, attempt, retry_of,
		       scheduled_at, claimed_at, completed_at, latency_ms,
		       issue_id, turn_id, skip_reason, failure_reason, failure_class, created_at
		FROM runs WHERE id = ?
	`, id)
	return ScanRunForRow(row)
}

// MarkTerminalParams captures everything the worker learned during the run.
type MarkTerminalParams struct {
	ID            string
	Status        Status
	CompletedAt   time.Time
	LatencyMS     int64
	IssueID       string
	TurnID        string
	SkipReason    string
	FailureReason string
	FailureClass  FailureClass
}

// MarkTerminal writes the terminal status for a running row. Also bumps
// agents.last_run_at so the Loop's due-check doesn't have to scan runs.
func (q *Queue) MarkTerminal(ctx context.Context, p MarkTerminalParams) error {
	tx, err := q.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		UPDATE runs
		SET status=?, completed_at=?, latency_ms=?,
		    issue_id=?, turn_id=?, skip_reason=?,
		    failure_reason=?, failure_class=?
		WHERE id=?
	`,
		string(p.Status), p.CompletedAt.UTC().Format(time.RFC3339), p.LatencyMS,
		nullStringFromValue(p.IssueID), nullStringFromValue(p.TurnID), nullStringFromValue(p.SkipReason),
		nullStringFromValue(p.FailureReason), nullStringFromValue(string(p.FailureClass)),
		p.ID,
	)
	if err != nil {
		return fmt.Errorf("mark terminal: %w", err)
	}

	// Bump agents.last_run_at — denormalized so the loop doesn't scan runs.
	_, err = tx.ExecContext(ctx, `
		UPDATE agents SET last_run_at = ?
		WHERE persona = (SELECT persona FROM runs WHERE id = ?)
	`, p.CompletedAt.UTC().Format(time.RFC3339), p.ID)
	if err != nil {
		return fmt.Errorf("bump last_run_at: %w", err)
	}
	return tx.Commit()
}

// Cancel transitions a queued or running row to failed_permanent with a
// fixed failure_reason. Used by POST /v1/runs/:id/cancel to unstick rows
// the worker abandoned mid-flight (or to abort a queued row before it
// claims). Returns ErrRunNotFound / ErrRunNotCancellable on the obvious
// failure cases so handlers can map to the right HTTP status.
//
// Does not bump agents.last_run_at — mirroring the orphan sweep on
// startup. last_run_at is the timestamp of the last *legitimate* run;
// leaving it alone means the persona becomes eligible for the next tick
// (or a manual Run-now) immediately, which is what the operator wants
// when cancelling a stuck row.
//
// Race note: the worker's MarkTerminal is unconditional and will overwrite
// this status if a hung run finishes after the cancel lands. For the
// "stuck row" use case this is acceptable — by definition we cancel rows
// the worker is no longer making progress on.
func (q *Queue) Cancel(ctx context.Context, id string) (*Run, error) {
	now := q.Now().UTC().Format(time.RFC3339)
	res, err := q.DB.ExecContext(ctx, `
		UPDATE runs
		   SET status = 'failed_permanent',
		       completed_at = ?,
		       failure_reason = 'cancelled by operator',
		       failure_class = 'permanent'
		 WHERE id = ? AND status IN ('queued', 'running')
	`, now, id)
	if err != nil {
		return nil, fmt.Errorf("cancel: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 1 {
		return q.Get(ctx, id)
	}
	// Zero rows updated — either the row doesn't exist or it's already
	// terminal. One SELECT to distinguish.
	existing, err := q.Get(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRunNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("cancel lookup: %w", err)
	}
	return existing, ErrRunNotCancellable
}

// --- helpers ---

func nullString(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func nullStringFromValue(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ScanRunForRow is exported so HTTP handlers can scan rows from *sql.Rows
// without duplicating the column list. Works with both *sql.Row and *sql.Rows.
func ScanRunForRow(row interface{ Scan(...any) error }) (*Run, error) {
	var r Run
	var status, trigger string
	var retryOf, claimedAt, completedAt, issueID, turnID, skipReason, failureReason, failureClass sql.NullString
	var scheduledAt, createdAt string
	var latencyMS sql.NullInt64
	if err := row.Scan(
		&r.ID, &r.Persona, &trigger, &status, &r.Attempt, &retryOf,
		&scheduledAt, &claimedAt, &completedAt, &latencyMS,
		&issueID, &turnID, &skipReason, &failureReason, &failureClass, &createdAt,
	); err != nil {
		return nil, fmt.Errorf("scan run: %w", err)
	}
	r.Trigger = Trigger(trigger)
	r.Status = Status(status)
	if retryOf.Valid {
		r.RetryOf = &retryOf.String
	}
	t, err := time.Parse(time.RFC3339, scheduledAt)
	if err != nil {
		return nil, fmt.Errorf("scan scheduled_at: %w", err)
	}
	r.ScheduledAt = t
	if claimedAt.Valid {
		ct, _ := time.Parse(time.RFC3339, claimedAt.String)
		r.ClaimedAt = &ct
	}
	if completedAt.Valid {
		ct, _ := time.Parse(time.RFC3339, completedAt.String)
		r.CompletedAt = &ct
	}
	if latencyMS.Valid {
		v := latencyMS.Int64
		r.LatencyMS = &v
	}
	if issueID.Valid {
		r.IssueID = &issueID.String
	}
	if turnID.Valid {
		r.TurnID = &turnID.String
	}
	if skipReason.Valid {
		r.SkipReason = &skipReason.String
	}
	if failureReason.Valid {
		r.FailureReason = &failureReason.String
	}
	if failureClass.Valid {
		r.FailureClass = FailureClass(failureClass.String)
	}
	crt, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("scan created_at: %w", err)
	}
	r.CreatedAt = crt
	return &r, nil
}
