package sweeper

import (
	"context"
	"fmt"
	"time"
)

// DefaultRunsTTL is how long a terminal run row is kept before the
// retention sweep reclaims it. Override via WOOAGENT_RUNS_TTL_DAYS.
const DefaultRunsTTL = 30 * 24 * time.Hour

// SweepRunsOnce enforces the retention policy on the runs table
// (DSGWOO-1294). A daemon left running for months accumulates runs rows
// indefinitely — three personas on a 6-hour cadence is ~12 rows/day, the
// large majority of them skipped runs that never produced an issue — and
// /v1/runs scans the whole table (its cursor pagination slices
// presentation, not storage).
//
// Policy: delete terminal runs older than the TTL, but PRESERVE any run
// still linked to an existing issue. That split keeps the two sweeps
// honest about their jobs:
//
//   - SweepOnce owns issue-lifecycle deletion. When a dismissed issue is
//     purged it already takes that issue's runs with it.
//   - SweepRunsOnce owns volume control. The growth is dominated by runs
//     with a NULL issue_id, so skipping issue-linked rows costs almost
//     nothing and guarantees the run-log link from a live or archived
//     issue never breaks.
//
// Returns the number of runs rows deleted. Orphaned turn_events are swept
// in the same transaction; their count is logged but not returned.
func (s *Sweeper) SweepRunsOnce(ctx context.Context) (int64, error) {
	now := s.now()
	cutoff := now.Add(-s.runsTTL()).UTC().Format(time.RFC3339)

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Terminal is expressed as NOT IN ('queued','running') rather than by
	// enumerating the terminal statuses. It mirrors the guard in
	// Queue.Cancel and stays correct if a new terminal status is added —
	// an in-flight run must never be deleted regardless of age.
	//
	// retry_of is a self-FK with ON DELETE SET NULL, so deleting an old
	// parent just nulls a surviving retry's back-link. No cascade needed.
	runsRes, err := tx.ExecContext(ctx, `
		DELETE FROM runs
		 WHERE status NOT IN ('queued', 'running')
		   AND completed_at IS NOT NULL
		   AND completed_at < ?
		   AND (issue_id IS NULL OR issue_id NOT IN (SELECT id FROM issues))
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete expired runs: %w", err)
	}
	runsDeleted, _ := runsRes.RowsAffected()

	// Orphaned turn_events. runs.turn_id → turn_events is ON DELETE SET
	// NULL, i.e. the FK points the wrong way to clean up after us, so the
	// orphans need an explicit pass.
	//
	// Both guards matter. The issue_id guard protects the audit trail of a
	// live issue — turn_events is the GEPA telemetry substrate. The
	// created_at guard protects rows that are legitimately unreferenced
	// while fresh: an Ask Agent turn has no run and no issue, and would
	// otherwise be deleted seconds after it was written.
	teRes, err := tx.ExecContext(ctx, `
		DELETE FROM turn_events
		 WHERE created_at < ?
		   AND turn_id NOT IN (SELECT turn_id FROM runs WHERE turn_id IS NOT NULL)
		   AND (issue_id IS NULL OR issue_id NOT IN (SELECT id FROM issues))
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete orphaned turn_events: %w", err)
	}
	turnsDeleted, _ := teRes.RowsAffected()

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	if (runsDeleted > 0 || turnsDeleted > 0) && s.Out != nil {
		fmt.Fprintf(s.Out, "→ sweeper: deleted %d run(s) and %d orphaned turn event(s) older than %s\n",
			runsDeleted, turnsDeleted, s.runsTTL())
	}
	return runsDeleted, nil
}

func (s *Sweeper) runsTTL() time.Duration {
	if s.RunsTTL > 0 {
		return s.RunsTTL
	}
	return DefaultRunsTTL
}
