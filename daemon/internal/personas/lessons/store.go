// Package lessons derives a per-persona "lessons learned" block from operator
// dismissals and serves it to the draft path. See DSGWOO-1354.
package lessons

import (
	"context"
	"database/sql"
	"fmt"
)

// Row mirrors a persona_lessons row.
type Row struct {
	Persona      string
	LessonsText  string
	GeneratedAt  string
	SourceCount  int
	SourceOldest string
	SourceNewest string
}

// SourceRecord is one dismissal fed into the digest LLM call.
type SourceRecord struct {
	Reason      string
	Comment     string
	Variant     string // the proposal body we showed and the operator rejected
	DismissedAt string
}

// countNewDismissals counts dismissed proposals for persona with dismissed_at
// strictly greater than watermark (RFC3339). An empty watermark counts all.
func countNewDismissals(ctx context.Context, db *sql.DB, persona, watermark string) (int, error) {
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM issues
		 WHERE persona = ? AND status IN ('dismissed','rejected')
		   AND dismissed_at IS NOT NULL AND dismissed_at > ?`,
		persona, watermark).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count new dismissals: %w", err)
	}
	return n, nil
}

// fetchRecentDismissals returns up to limit most-recent dismissals for persona,
// newest first.
func fetchRecentDismissals(ctx context.Context, db *sql.DB, persona string, limit int) ([]SourceRecord, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT COALESCE(dismiss_reason,''), COALESCE(dismiss_comment,''),
		        COALESCE(proposal_content,''), COALESCE(dismissed_at,'')
		 FROM issues
		 WHERE persona = ? AND status IN ('dismissed','rejected') AND dismissed_at IS NOT NULL
		 ORDER BY dismissed_at DESC
		 LIMIT ?`, persona, limit)
	if err != nil {
		return nil, fmt.Errorf("fetch recent dismissals: %w", err)
	}
	defer rows.Close()
	var out []SourceRecord
	for rows.Next() {
		var r SourceRecord
		if err := rows.Scan(&r.Reason, &r.Comment, &r.Variant, &r.DismissedAt); err != nil {
			return nil, fmt.Errorf("scan dismissal: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// loadRow returns the persona_lessons row for persona, or (nil, nil) if absent.
func loadRow(ctx context.Context, db *sql.DB, persona string) (*Row, error) {
	var r Row
	err := db.QueryRowContext(ctx,
		`SELECT persona, lessons_text, generated_at, source_count,
		        source_oldest_dismissed_at, source_newest_dismissed_at
		 FROM persona_lessons WHERE persona = ?`, persona).
		Scan(&r.Persona, &r.LessonsText, &r.GeneratedAt, &r.SourceCount, &r.SourceOldest, &r.SourceNewest)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load lessons row: %w", err)
	}
	return &r, nil
}

// upsertRow inserts or replaces the persona_lessons row.
func upsertRow(ctx context.Context, db *sql.DB, r Row) error {
	_, err := db.ExecContext(ctx,
		`INSERT OR REPLACE INTO persona_lessons
		   (persona, lessons_text, generated_at, source_count, source_oldest_dismissed_at, source_newest_dismissed_at)
		 VALUES (?,?,?,?,?,?)`,
		r.Persona, r.LessonsText, r.GeneratedAt, r.SourceCount, r.SourceOldest, r.SourceNewest)
	if err != nil {
		return fmt.Errorf("upsert lessons row: %w", err)
	}
	return nil
}
