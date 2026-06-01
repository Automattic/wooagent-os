package lessons

import (
	"context"
	"strings"
	"testing"
)

func TestLoadForReturnsEmptyWhenNoRow(t *testing.T) {
	db := newDB(t)
	got, err := LoadFor(context.Background(), db, "marketing")
	if err != nil {
		t.Fatalf("LoadFor: %v", err)
	}
	if got != "" {
		t.Errorf("LoadFor(no row) = %q, want empty", got)
	}
}

func TestLoadForRespectsKillSwitch(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	upsertRow(ctx, db, Row{Persona: "marketing", LessonsText: "- be warm", GeneratedAt: "2026-06-01T00:00:00Z",
		SourceCount: 5, SourceOldest: "2026-05-20T00:00:00Z", SourceNewest: "2026-06-01T00:00:00Z"})
	t.Setenv("WOOAGENT_PERSONA_LESSONS_DISABLED", "marketing,pricing")
	got, _ := LoadFor(ctx, db, "marketing")
	if got != "" {
		t.Errorf("kill switch should suppress marketing; got %q", got)
	}
}

func TestLoadForFormatsHeader(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	upsertRow(ctx, db, Row{Persona: "marketing", LessonsText: "- be warm\n- be concrete", GeneratedAt: "2026-06-01T00:00:00Z",
		SourceCount: 5, SourceOldest: "2026-05-21T00:00:00Z", SourceNewest: "2026-06-01T00:00:00Z"})
	got, err := LoadFor(ctx, db, "marketing")
	if err != nil {
		t.Fatalf("LoadFor: %v", err)
	}
	if !strings.HasPrefix(got, "Lessons from recent operator dismissals (5 dismissals · last 11 days):") {
		t.Errorf("header missing/wrong:\n%s", got)
	}
	if !strings.Contains(got, "- be warm") {
		t.Errorf("body missing:\n%s", got)
	}
}

func TestDisabledForHelper(t *testing.T) {
	t.Setenv("WOOAGENT_PERSONA_LESSONS_DISABLED", "marketing , pricing")
	if !DisabledFor("marketing") {
		t.Error("marketing should be disabled (whitespace-tolerant)")
	}
	if DisabledFor("sales_support") {
		t.Error("sales_support should not be disabled")
	}
}
