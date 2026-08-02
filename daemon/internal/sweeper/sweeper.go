// Package sweeper implements the daemon's background retention jobs.
//
// Two independent sweeps, each with its own TTL, run on a shared ticker:
//
//   - SweepOnce — the 30-day purge for dismissed issues (DSGWOO-1277).
//     The Dismiss dialog promises operators that archived proposals are
//     permanently deleted 30 days after dismiss; this makes that true.
//   - SweepRunsOnce — retention for terminal runs rows and orphaned
//     turn_events (DSGWOO-1294). See runs.go.
//
// One Sweeper per daemon process. Start() fires both sweeps immediately
// (catching rows that crossed a TTL while the daemon was down) and then
// loops on a 24-hour ticker.
package sweeper

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"time"
)

// DefaultTTL is the dismiss-to-delete window operators see in the Dismiss
// dialog copy. Override via WOOAGENT_DISMISS_TTL_DAYS for demo / dogfood
// timelines.
const DefaultTTL = 30 * 24 * time.Hour

// DefaultEvery is the cadence between sweep cycles. One day is enough —
// the TTL is measured in days, not minutes.
const DefaultEvery = 24 * time.Hour

// Sweeper deletes dismissed issues (and their associated runs,
// turn_events, audit_invocations rows) once they cross the TTL window.
type Sweeper struct {
	DB    *sql.DB
	TTL   time.Duration
	Every time.Duration
	Now   func() time.Time
	Out   io.Writer

	// RunsTTL is the retention window for terminal runs rows
	// (DSGWOO-1294). Independent of TTL, which governs dismissed issues.
	// Defaults to DefaultRunsTTL. See SweepRunsOnce in runs.go.
	RunsTTL time.Duration
}

// SweepOnce runs a single sweep cycle. Returns the number of issues
// deleted. Safe to call repeatedly; if no rows match the cutoff the
// call is a cheap SELECT.
func (s *Sweeper) SweepOnce(ctx context.Context) (int64, error) {
	now := s.now()
	cutoff := now.Add(-s.ttl()).UTC().Format(time.RFC3339)

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		SELECT id FROM issues
		WHERE status = 'dismissed'
		  AND dismissed_at IS NOT NULL
		  AND dismissed_at < ?
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("select expired: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan id: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate ids: %w", err)
	}
	if len(ids) == 0 {
		_ = tx.Commit()
		return 0, nil
	}

	// IN-clause placeholders. Same idiom as personas.CooldownExclude.
	placeholders := strings.Repeat("?,", len(ids)-1) + "?"
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	// Cascade order: drop dependents first so FKs that are SET NULL
	// don't leave dangling references in turn_events / runs (both
	// FK to issues with ON DELETE SET NULL — see migrations 001 and
	// 011). audit_invocations has no FK at all; explicit delete is
	// the only path. Proposals live as columns on the issues row
	// itself (no separate table), so the final DELETE on issues
	// takes them with it.
	for _, q := range []string{
		`DELETE FROM audit_invocations WHERE issue_id IN (` + placeholders + `)`,
		`DELETE FROM runs              WHERE issue_id IN (` + placeholders + `)`,
		`DELETE FROM turn_events       WHERE issue_id IN (` + placeholders + `)`,
		`DELETE FROM issues            WHERE id       IN (` + placeholders + `)`,
	} {
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return 0, fmt.Errorf("delete cascade: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	n := int64(len(ids))
	if n > 0 && s.Out != nil {
		fmt.Fprintf(s.Out, "→ sweeper: deleted %d dismissed issue(s) older than %s\n", n, s.ttl())
	}
	return n, nil
}

// Start runs SweepOnce immediately, then on every s.Every tick until
// ctx is cancelled. Fire-and-forget: errors are logged to s.Out, never
// returned (a transient DB hiccup shouldn't take down the daemon).
func (s *Sweeper) Start(ctx context.Context) {
	s.cycle(ctx, "initial")
	t := time.NewTicker(s.every())
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.cycle(ctx, "cycle")
		}
	}
}

// cycle runs both sweeps back to back. The dismissed-issue sweep goes
// first so that any runs it orphans are eligible for the retention sweep
// in the same pass rather than waiting another 24 hours.
//
// Each sweep's error is logged independently: a failure in one must not
// skip the other, and neither should take down the daemon.
func (s *Sweeper) cycle(ctx context.Context, label string) {
	if _, err := s.SweepOnce(ctx); err != nil && s.Out != nil {
		fmt.Fprintf(s.Out, "→ sweeper: %s dismissed-issue sweep failed: %v\n", label, err)
	}
	if _, err := s.SweepRunsOnce(ctx); err != nil && s.Out != nil {
		fmt.Fprintf(s.Out, "→ sweeper: %s runs-retention sweep failed: %v\n", label, err)
	}
}

func (s *Sweeper) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Sweeper) ttl() time.Duration {
	if s.TTL > 0 {
		return s.TTL
	}
	return DefaultTTL
}

func (s *Sweeper) every() time.Duration {
	if s.Every > 0 {
		return s.Every
	}
	return DefaultEvery
}
