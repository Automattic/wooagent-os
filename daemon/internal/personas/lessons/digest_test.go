package lessons

import (
	"context"
	"strings"
	"testing"
)

type fakeDigester struct {
	calls int
	out   string
	err   error
}

func (f *fakeDigester) Digest(ctx context.Context, persona string, recs []SourceRecord) (string, error) {
	f.calls++
	return f.out, f.err
}

type errString string

func (e errString) Error() string { return string(e) }

func TestJob_SkipsBelowThreshold(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		db.ExecContext(ctx, `INSERT INTO issues (id,persona,status,dismissed_at) VALUES (?,?,?,?)`,
			string(rune('a'+i)), "marketing", "dismissed", "2026-05-2"+string(rune('0'+i))+"T00:00:00Z")
	}
	fd := &fakeDigester{out: "- lesson"}
	j := &Job{DB: db, Digester: fd, Threshold: 5, Personas: []string{"marketing"}}
	if err := j.runOnce(ctx); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	if fd.calls != 0 {
		t.Errorf("below threshold should not call digester; calls=%d", fd.calls)
	}
	if row, _ := loadRow(ctx, db, "marketing"); row != nil {
		t.Errorf("no row should be written below threshold")
	}
}

func TestJob_TriggersAtThresholdAndUpserts(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		db.ExecContext(ctx, `INSERT INTO issues (id,persona,status,dismiss_reason,dismiss_comment,dismissed_at,proposal_content) VALUES (?,?,?,?,?,?,?)`,
			string(rune('a'+i)), "marketing", "dismissed", "tone_off", "too cold", "2026-05-2"+string(rune('0'+i))+"T00:00:00Z", "Stoneware mug.")
	}
	fd := &fakeDigester{out: "- be warm\n- be concrete"}
	j := &Job{DB: db, Digester: fd, Threshold: 5, Personas: []string{"marketing"}}
	if err := j.runOnce(ctx); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	if fd.calls != 1 {
		t.Fatalf("expected 1 digest call, got %d", fd.calls)
	}
	row, _ := loadRow(ctx, db, "marketing")
	if row == nil || row.LessonsText != "- be warm\n- be concrete" {
		t.Fatalf("row not upserted: %+v", row)
	}
	if row.SourceCount != 5 || row.SourceNewest != "2026-05-24T00:00:00Z" {
		t.Errorf("watermark/count wrong: count=%d newest=%q", row.SourceCount, row.SourceNewest)
	}
	if row.SourceOldest != "2026-05-20T00:00:00Z" {
		t.Errorf("oldest wrong: %q", row.SourceOldest)
	}
}

func TestJob_UsesWatermark(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	upsertRow(ctx, db, Row{Persona: "marketing", LessonsText: "- prior", GeneratedAt: "x",
		SourceCount: 3, SourceOldest: "2026-05-20T00:00:00Z", SourceNewest: "2026-05-22T00:00:00Z"})
	for i, at := range []string{"2026-05-20T00:00:00Z", "2026-05-21T00:00:00Z", "2026-05-22T00:00:00Z", "2026-05-23T00:00:00Z", "2026-05-24T00:00:00Z"} {
		db.ExecContext(ctx, `INSERT INTO issues (id,persona,status,dismissed_at) VALUES (?,?,?,?)`,
			string(rune('a'+i)), "marketing", "dismissed", at)
	}
	fd := &fakeDigester{out: "- new"}
	j := &Job{DB: db, Digester: fd, Threshold: 5, Personas: []string{"marketing"}}
	j.runOnce(ctx)
	if fd.calls != 0 {
		t.Errorf("only 2 new past watermark (<5) → no digest; calls=%d", fd.calls)
	}
}

func TestJob_TruncatesOversizeOutput(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		db.ExecContext(ctx, `INSERT INTO issues (id,persona,status,dismissed_at) VALUES (?,?,?,?)`,
			string(rune('a'+i)), "marketing", "dismissed", "2026-05-2"+string(rune('0'+i))+"T00:00:00Z")
	}
	fd := &fakeDigester{out: strings.Repeat("word ", 400)}
	j := &Job{DB: db, Digester: fd, Threshold: 5, Personas: []string{"marketing"}}
	j.runOnce(ctx)
	row, _ := loadRow(ctx, db, "marketing")
	if row == nil || len(row.LessonsText) > maxLessonsChars {
		t.Errorf("lessons_text not truncated to <= %d: len=%d", maxLessonsChars, len(row.LessonsText))
	}
}

func TestJob_LLMErrorLeavesRowUnchanged(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	upsertRow(ctx, db, Row{Persona: "marketing", LessonsText: "- prior", GeneratedAt: "orig",
		SourceCount: 5, SourceOldest: "2026-05-10T00:00:00Z", SourceNewest: "2026-05-15T00:00:00Z"})
	for i := 0; i < 6; i++ {
		db.ExecContext(ctx, `INSERT INTO issues (id,persona,status,dismissed_at) VALUES (?,?,?,?)`,
			string(rune('a'+i)), "marketing", "dismissed", "2026-05-2"+string(rune('0'+i))+"T00:00:00Z")
	}
	fd := &fakeDigester{err: errString("boom")}
	j := &Job{DB: db, Digester: fd, Threshold: 5, Personas: []string{"marketing"}}
	j.runOnce(ctx)
	row, _ := loadRow(ctx, db, "marketing")
	if row.GeneratedAt != "orig" {
		t.Errorf("errored digest must not overwrite row; generated_at=%q", row.GeneratedAt)
	}
}
