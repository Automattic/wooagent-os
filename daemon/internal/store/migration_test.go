package store

import (
	"context"
	"path/filepath"
	"testing"
)

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
