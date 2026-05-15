package telemetry

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

func TestLoadTurnEvent_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	rec := NewSQLiteRecorder(st.DB, nil)
	e := TurnEvent{
		TurnID:    "t1",
		Persona:   "marketing",
		StartedAt: time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC),
		ModelCalls: []ModelCall{{Provider: "anthropic", Model: "claude", InputTokens: 100}},
	}
	if err := rec.Record(context.Background(), e); err != nil {
		t.Fatalf("record: %v", err)
	}
	got, err := LoadTurnEvent(context.Background(), st.DB, "t1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Persona != "marketing" || len(got.ModelCalls) != 1 || got.ModelCalls[0].InputTokens != 100 {
		t.Errorf("round trip lost data: %+v", got)
	}
}
