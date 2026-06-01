package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigration_PersonaLessonsTable(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	if _, err := st.DB.ExecContext(ctx,
		`INSERT INTO persona_lessons (persona, lessons_text, generated_at, source_count, source_oldest_dismissed_at, source_newest_dismissed_at)
		 VALUES ('marketing','- be warm','2026-06-01T00:00:00Z',5,'2026-05-20T00:00:00Z','2026-06-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert persona_lessons: %v", err)
	}
	var txt string
	if err := st.DB.QueryRowContext(ctx, `SELECT lessons_text FROM persona_lessons WHERE persona='marketing'`).Scan(&txt); err != nil {
		t.Fatalf("select: %v", err)
	}
	if txt != "- be warm" {
		t.Errorf("lessons_text = %q, want %q", txt, "- be warm")
	}
}

func TestMigration_DaemonMetaTable(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	if _, err := st.DB.ExecContext(ctx,
		`INSERT INTO daemon_meta (key, value, updated_at) VALUES ('install_id','abc','2026-06-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert daemon_meta: %v", err)
	}
	var v string
	if err := st.DB.QueryRowContext(ctx, `SELECT value FROM daemon_meta WHERE key='install_id'`).Scan(&v); err != nil {
		t.Fatalf("select: %v", err)
	}
	if v != "abc" {
		t.Errorf("value = %q, want abc", v)
	}
}

func TestMigration011AppliesFresh(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	// Sanity: new columns + table are reachable.
	if _, err := st.DB.ExecContext(ctx,
		`INSERT INTO agents(persona, name, enabled, created_at, updated_at, cadence_seconds, max_attempts) VALUES('test','Test',1,'2026-01-01T00:00:00Z','2026-01-01T00:00:00Z',60,3)`,
	); err != nil {
		t.Fatalf("insert agent w/ new columns: %v", err)
	}
	if _, err := st.DB.ExecContext(ctx,
		`INSERT INTO runs(id, persona, trigger, status, scheduled_at, created_at) VALUES('r1','test','tick','queued','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`,
	); err != nil {
		t.Fatalf("insert run: %v", err)
	}
}
