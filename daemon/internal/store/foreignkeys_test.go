package store

import (
	"context"
	"path/filepath"
	"testing"
)

// TestForeignKeys_CascadeFiresPoolWide is the regression test for the FK-cascade bug: deleting
// a store must cascade-delete its abilities rows. This only holds if
// foreign_keys is ON for every pooled connection — when it was set via a
// one-shot Exec it stuck on a single connection and CASCADE silently no-opped,
// orphaning ability rows that then produced spurious schema_drift denials.
func TestForeignKeys_CascadeFiresPoolWide(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	// Multiple connections in the pool so the DELETE and the follow-up read
	// can land on different ones — exactly the scenario the per-connection
	// pragma bug hid behind.
	st.DB.SetMaxOpenConns(4)

	// Every connection must report foreign_keys = ON.
	for i := 0; i < 8; i++ {
		var fk int
		if err := st.DB.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil {
			t.Fatalf("read pragma: %v", err)
		}
		if fk != 1 {
			t.Fatalf("foreign_keys = %d on a pooled connection; want 1", fk)
		}
	}

	if _, err := st.DB.ExecContext(ctx,
		`INSERT INTO stores(id, url, mcp_endpoint, status, created_at, updated_at)
		 VALUES('store_fk', 'https://x.local', 'https://x.local/mcp', 'paired', '2026-06-07T00:00:00Z', '2026-06-07T00:00:00Z')`); err != nil {
		t.Fatalf("insert store: %v", err)
	}
	if _, err := st.DB.ExecContext(ctx,
		`INSERT INTO abilities(id, store_id, name, schema_hash, trust_state, last_seen_at, created_at, updated_at)
		 VALUES('ab_fk', 'store_fk', 'wooagent-products/update', 'sha256:abc', 'new', '2026-06-07T00:00:00Z', '2026-06-07T00:00:00Z', '2026-06-07T00:00:00Z')`); err != nil {
		t.Fatalf("insert ability: %v", err)
	}

	if _, err := st.DB.ExecContext(ctx, `DELETE FROM stores WHERE id = 'store_fk'`); err != nil {
		t.Fatalf("delete store: %v", err)
	}

	var n int
	if err := st.DB.QueryRowContext(ctx, `SELECT count(*) FROM abilities WHERE store_id = 'store_fk'`).Scan(&n); err != nil {
		t.Fatalf("count abilities: %v", err)
	}
	if n != 0 {
		t.Fatalf("ability rows after store delete = %d; want 0 (ON DELETE CASCADE did not fire)", n)
	}
}
