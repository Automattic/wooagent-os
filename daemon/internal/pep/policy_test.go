package pep

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
)

// agentsDDL — minimal shape mirroring the merged migrations 001 + 011 +
// 014 + 016. Kept inline in tests so the pep package doesn't pull in
// store/migrations.
const agentsDDL = `
CREATE TABLE agents (
    persona            TEXT PRIMARY KEY,
    name               TEXT,
    model_preference   TEXT,
    enabled            INTEGER NOT NULL DEFAULT 0,
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL,
    cadence_seconds    INTEGER NOT NULL DEFAULT 21600,
    max_attempts       INTEGER NOT NULL DEFAULT 3,
    last_run_at        TEXT,
    run_budget_cents   INTEGER NOT NULL DEFAULT 1000,
    apply_hours_start  TEXT,
    apply_hours_end    TEXT
);`

func newAgentsDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(agentsDDL); err != nil {
		t.Fatalf("ddl: %v", err)
	}
	return db
}

func TestLoadAgentSettings_BothNullReturnsEmpty(t *testing.T) {
	db := newAgentsDB(t)
	if _, err := db.Exec(
		`INSERT INTO agents(persona, enabled, created_at, updated_at) VALUES('marketing', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
	); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := loadAgentSettings(context.Background(), db, manifest.PersonaMarketing)
	if err != nil {
		t.Fatalf("loadAgentSettings: %v", err)
	}
	if got.ApplyHoursStart != "" || got.ApplyHoursEnd != "" {
		t.Errorf("got %+v, want zero AgentSettings", got)
	}
}

func TestLoadAgentSettings_BothSetReturnsValues(t *testing.T) {
	db := newAgentsDB(t)
	if _, err := db.Exec(
		`INSERT INTO agents(persona, enabled, created_at, updated_at, apply_hours_start, apply_hours_end) VALUES('marketing', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', '09:00', '17:00')`,
	); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := loadAgentSettings(context.Background(), db, manifest.PersonaMarketing)
	if err != nil {
		t.Fatalf("loadAgentSettings: %v", err)
	}
	if got.ApplyHoursStart != "09:00" || got.ApplyHoursEnd != "17:00" {
		t.Errorf("got %+v, want {09:00, 17:00}", got)
	}
}

func TestLoadAgentSettings_MissingPersonaReturnsEmpty(t *testing.T) {
	db := newAgentsDB(t)
	got, err := loadAgentSettings(context.Background(), db, manifest.PersonaMarketing)
	if err != nil {
		t.Fatalf("loadAgentSettings: %v", err)
	}
	if got.ApplyHoursStart != "" || got.ApplyHoursEnd != "" {
		t.Errorf("got %+v, want zero AgentSettings", got)
	}
}
