package lessons

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	_, err = db.ExecContext(context.Background(), `
		CREATE TABLE issues (
			id TEXT PRIMARY KEY, persona TEXT, status TEXT,
			dismiss_reason TEXT, dismiss_comment TEXT, dismissed_at TEXT,
			proposal_content TEXT
		);
		CREATE TABLE persona_lessons (
			persona TEXT PRIMARY KEY, lessons_text TEXT NOT NULL, generated_at TEXT NOT NULL,
			source_count INTEGER NOT NULL, source_oldest_dismissed_at TEXT NOT NULL,
			source_newest_dismissed_at TEXT NOT NULL
		);`)
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

func TestCountNewDismissals_RespectsWatermark(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	for _, r := range []struct{ id, at string }{
		{"a", "2026-05-10T00:00:00Z"}, {"b", "2026-05-20T00:00:00Z"}, {"c", "2026-05-30T00:00:00Z"},
	} {
		db.ExecContext(ctx, `INSERT INTO issues (id,persona,status,dismissed_at) VALUES (?,?,?,?)`, r.id, "marketing", "dismissed", r.at)
	}
	n, err := countNewDismissals(ctx, db, "marketing", "2026-05-15T00:00:00Z")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Errorf("count = %d, want 2", n)
	}
	if n, _ = countNewDismissals(ctx, db, "marketing", ""); n != 3 {
		t.Errorf("count(empty watermark) = %d, want 3", n)
	}
}

func TestFetchRecentDismissals_OrdersAndCaps(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	for i, at := range []string{"2026-05-10T00:00:00Z", "2026-05-20T00:00:00Z", "2026-05-30T00:00:00Z"} {
		db.ExecContext(ctx, `INSERT INTO issues (id,persona,status,dismiss_reason,dismiss_comment,dismissed_at,proposal_content) VALUES (?,?,?,?,?,?,?)`,
			string(rune('a'+i)), "marketing", "dismissed", "tone_off", "too cold", at, "Stoneware mug.")
	}
	recs, err := fetchRecentDismissals(ctx, db, "marketing", 2)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("len = %d, want 2 (capped)", len(recs))
	}
	if recs[0].DismissedAt != "2026-05-30T00:00:00Z" {
		t.Errorf("newest first: got %q", recs[0].DismissedAt)
	}
	if recs[0].Reason != "tone_off" || recs[0].Variant != "Stoneware mug." {
		t.Errorf("record fields not populated: %+v", recs[0])
	}
}

func TestQueriesIncludeBatchRejected(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	// one dismissed (single-issue) + one rejected (batch) — both carry dismiss fields.
	db.ExecContext(ctx, `INSERT INTO issues (id,persona,status,dismiss_reason,dismiss_comment,dismissed_at,proposal_content) VALUES (?,?,?,?,?,?,?)`,
		"d1", "marketing", "dismissed", "tone_off", "cold", "2026-05-20T00:00:00Z", "Mug A.")
	db.ExecContext(ctx, `INSERT INTO issues (id,persona,status,dismiss_reason,dismiss_comment,dismissed_at,proposal_content) VALUES (?,?,?,?,?,?,?)`,
		"r1", "marketing", "rejected", "wrong_focus", "off", "2026-05-21T00:00:00Z", "Mug B.")
	n, err := countNewDismissals(ctx, db, "marketing", "")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Errorf("count = %d, want 2 (dismissed + rejected)", n)
	}
	recs, err := fetchRecentDismissals(ctx, db, "marketing", 10)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(recs) != 2 {
		t.Errorf("fetch len = %d, want 2 (dismissed + rejected)", len(recs))
	}
}

func TestUpsertAndLoadRow(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	row := Row{Persona: "marketing", LessonsText: "- be warm", GeneratedAt: "2026-06-01T00:00:00Z",
		SourceCount: 5, SourceOldest: "2026-05-20T00:00:00Z", SourceNewest: "2026-06-01T00:00:00Z"}
	if err := upsertRow(ctx, db, row); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	row.LessonsText = "- be warmer"
	if err := upsertRow(ctx, db, row); err != nil {
		t.Fatalf("upsert2: %v", err)
	}
	got, err := loadRow(ctx, db, "marketing")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got == nil || got.LessonsText != "- be warmer" {
		t.Errorf("loadRow = %+v, want replaced text", got)
	}
	none, err := loadRow(ctx, db, "pricing")
	if err != nil || none != nil {
		t.Errorf("loadRow(missing) = %+v, %v; want nil,nil", none, err)
	}
}
