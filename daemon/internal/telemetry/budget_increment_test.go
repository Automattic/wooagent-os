package telemetry

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
	"github.com/wooagent-os/wooagent-os/daemon/internal/pep"
)

const turnEventsDDLTest = `
CREATE TABLE turn_events (
    turn_id              TEXT PRIMARY KEY,
    event_schema_version INTEGER NOT NULL,
    issue_id             TEXT,
    persona              TEXT,
    prompt_version       TEXT,
    skill_versions_json  TEXT NOT NULL DEFAULT '',
    started_at           TEXT NOT NULL,
    completed_at         TEXT,
    latency_ms           INTEGER NOT NULL DEFAULT 0,
    context_json         TEXT NOT NULL DEFAULT '',
    model_calls_json     TEXT NOT NULL DEFAULT '',
    skill_calls_json     TEXT NOT NULL DEFAULT '',
    proposal_text        TEXT,
    proposal_sha         TEXT,
    verdict_json         TEXT,
    created_at           TEXT NOT NULL
);`

const budgetUsageDDLTest = `
CREATE TABLE persona_budget_usage (
    persona     TEXT NOT NULL,
    usage_date  TEXT NOT NULL,
    cost_usd    REAL NOT NULL DEFAULT 0,
    call_count  INTEGER NOT NULL DEFAULT 0,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (persona, usage_date)
);`

func newRecorderTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(turnEventsDDLTest); err != nil {
		t.Fatalf("turn_events ddl: %v", err)
	}
	if _, err := db.Exec(budgetUsageDDLTest); err != nil {
		t.Fatalf("budget ddl: %v", err)
	}
	return db
}

func TestRecorder_IncrementsCostFromModelCalls(t *testing.T) {
	db := newRecorderTestDB(t)
	budget := pep.NewBudgetGate(db, pep.DefaultThresholds())
	rec := NewSQLiteRecorder(db, budget)

	turn := TurnEvent{
		TurnID:    "tn_1",
		Persona:   string(manifest.PersonaMarketing),
		StartedAt: time.Now().UTC(),
		ModelCalls: []ModelCall{
			{Provider: "anthropic", Model: "claude-opus-4-7", CostUSD: 2.50},
			{Provider: "anthropic", Model: "claude-opus-4-7", CostUSD: 1.25},
		},
	}
	if err := rec.Record(context.Background(), turn); err != nil {
		t.Fatalf("record: %v", err)
	}

	today := time.Now().Local().Format("2006-01-02")
	var cost float64
	if err := db.QueryRow(
		`SELECT cost_usd FROM persona_budget_usage WHERE persona='marketing' AND usage_date=?`, today,
	).Scan(&cost); err != nil {
		t.Fatalf("read usage: %v", err)
	}
	if cost != 3.75 {
		t.Errorf("cost = %v, want 3.75 (sum of model_calls)", cost)
	}
}

func TestRecorder_NoIncrementWhenCostIsZero(t *testing.T) {
	db := newRecorderTestDB(t)
	budget := pep.NewBudgetGate(db, pep.DefaultThresholds())
	rec := NewSQLiteRecorder(db, budget)

	turn := TurnEvent{
		TurnID:    "tn_2",
		Persona:   string(manifest.PersonaMarketing),
		StartedAt: time.Now().UTC(),
		ModelCalls: []ModelCall{
			{Provider: "anthropic", Model: "claude-opus-4-7"},
		},
	}
	_ = rec.Record(context.Background(), turn)

	today := time.Now().Local().Format("2006-01-02")
	var cost float64
	err := db.QueryRow(
		`SELECT cost_usd FROM persona_budget_usage WHERE persona='marketing' AND usage_date=?`, today,
	).Scan(&cost)
	if err == nil && cost != 0 {
		t.Errorf("no positive cost should mean no row or cost=0; got %v", cost)
	}
}

func TestRecorder_NoIncrementWhenPersonaEmpty(t *testing.T) {
	db := newRecorderTestDB(t)
	budget := pep.NewBudgetGate(db, pep.DefaultThresholds())
	rec := NewSQLiteRecorder(db, budget)

	turn := TurnEvent{
		TurnID:    "tn_3",
		Persona:   "",
		StartedAt: time.Now().UTC(),
		ModelCalls: []ModelCall{
			{CostUSD: 1.0},
		},
	}
	_ = rec.Record(context.Background(), turn)

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM persona_budget_usage`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("empty persona should not create a row, got count=%d", count)
	}
}
