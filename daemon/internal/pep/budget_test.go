package pep

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
)

// nullLogger discards everything; used in tests to keep test output clean.
var nullLogger = slog.New(slog.NewTextHandler(os.NewFile(0, os.DevNull), nil))

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestLoadBudgetOverlay_FileMissingReturnsBase(t *testing.T) {
	base := DefaultThresholds()
	got, err := LoadBudgetOverlay(filepath.Join(t.TempDir(), "does-not-exist.json"), base, nullLogger)
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if got[manifest.PersonaMarketing].DailyCostUSD != 5.00 {
		t.Errorf("expected base default 5.00, got %v", got[manifest.PersonaMarketing].DailyCostUSD)
	}
}

func TestLoadBudgetOverlay_PartialOverlayMergesPerField(t *testing.T) {
	base := DefaultThresholds()
	path := writeFile(t, t.TempDir(), "budgets.json", `{
		"marketing":    {"daily_cost_usd": 10.0, "daily_calls": 200},
		"sales-support": {"daily_cost_usd": 2.0}
	}`)
	got, err := LoadBudgetOverlay(path, base, nullLogger)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got[manifest.PersonaMarketing].DailyCostUSD != 10.0 {
		t.Errorf("marketing cost: got %v want 10.0", got[manifest.PersonaMarketing].DailyCostUSD)
	}
	if got[manifest.PersonaMarketing].DailyCalls != 200 {
		t.Errorf("marketing calls: got %v want 200", got[manifest.PersonaMarketing].DailyCalls)
	}
	if got[manifest.PersonaSalesSupport].DailyCostUSD != 2.0 {
		t.Errorf("sales-support cost: got %v want 2.0", got[manifest.PersonaSalesSupport].DailyCostUSD)
	}
	if got[manifest.PersonaSalesSupport].DailyCalls != 100 {
		t.Errorf("sales-support calls: got %v want default 100", got[manifest.PersonaSalesSupport].DailyCalls)
	}
	if got[manifest.PersonaPricing].DailyCostUSD != 5.00 {
		t.Errorf("pricing cost: got %v want 5.00", got[manifest.PersonaPricing].DailyCostUSD)
	}
}

func TestLoadBudgetOverlay_UnknownPersonaIsSkipped(t *testing.T) {
	base := DefaultThresholds()
	path := writeFile(t, t.TempDir(), "budgets.json", `{
		"marketting": {"daily_cost_usd": 1.0},
		"marketing":  {"daily_cost_usd": 7.0}
	}`)
	got, err := LoadBudgetOverlay(path, base, nullLogger)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got[manifest.PersonaMarketing].DailyCostUSD != 7.0 {
		t.Errorf("marketing cost: got %v want 7.0 (typo entry should not affect this)", got[manifest.PersonaMarketing].DailyCostUSD)
	}
}

func TestLoadBudgetOverlay_MalformedJSONReturnsError(t *testing.T) {
	base := DefaultThresholds()
	path := writeFile(t, t.TempDir(), "budgets.json", `{not valid json`)
	_, err := LoadBudgetOverlay(path, base, nullLogger)
	if err == nil {
		t.Fatal("expected parse error for malformed JSON")
	}
}

// budgetUsageDDL is the inlined DDL for the persona_budget_usage table —
// mirrors migration 012 so the budget tests don't depend on the full
// migration runner.
const budgetUsageDDL = `
CREATE TABLE persona_budget_usage (
    persona     TEXT NOT NULL,
    usage_date  TEXT NOT NULL,
    cost_usd    REAL NOT NULL DEFAULT 0,
    call_count  INTEGER NOT NULL DEFAULT 0,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (persona, usage_date)
);`

func newBudgetDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(budgetUsageDDL); err != nil {
		t.Fatalf("apply budget ddl: %v", err)
	}
	return db
}

// pinnedClock returns a clock fixed at a specific local time. The TZ comes
// from time.Local at call time, matching production behavior.
func pinnedClock(year int, month time.Month, day, hour, minute int) func() time.Time {
	return func() time.Time {
		return time.Date(year, month, day, hour, minute, 0, 0, time.Local)
	}
}

func insertUsage(t *testing.T, db *sql.DB, persona, date string, cost float64, calls int) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO persona_budget_usage(persona, usage_date, cost_usd, call_count, updated_at) VALUES(?, ?, ?, ?, ?)`,
		persona, date, cost, calls, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("seed usage row: %v", err)
	}
}

func newGateWithThresholds(t *testing.T, db *sql.DB, costCap float64, callCap int, clock func() time.Time) *BudgetGate {
	t.Helper()
	g := NewBudgetGate(db, Thresholds{
		manifest.PersonaMarketing: {DailyCostUSD: costCap, DailyCalls: callCap},
	})
	if clock != nil {
		g.clock = clock
	}
	g.logger = nullLogger
	return g
}

func TestBudgetGate_CheckNoRowAllows(t *testing.T) {
	db := newBudgetDB(t)
	g := newGateWithThresholds(t, db, 5.0, 100, nil)
	reason, err := g.Check(context.Background(), manifest.PersonaMarketing)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if reason != "" {
		t.Errorf("expected allowed, got reason %q", reason)
	}
}

func TestBudgetGate_CheckUnderCostAllows(t *testing.T) {
	db := newBudgetDB(t)
	clock := pinnedClock(2026, 5, 15, 12, 0)
	today := clock().Local().Format("2006-01-02")
	insertUsage(t, db, "marketing", today, 1.00, 10)
	g := newGateWithThresholds(t, db, 5.0, 100, clock)
	reason, err := g.Check(context.Background(), manifest.PersonaMarketing)
	if err != nil || reason != "" {
		t.Errorf("under budget: got reason=%q err=%v", reason, err)
	}
}

func TestBudgetGate_CheckAtCostThresholdDenies(t *testing.T) {
	db := newBudgetDB(t)
	clock := pinnedClock(2026, 5, 15, 12, 0)
	today := clock().Local().Format("2006-01-02")
	insertUsage(t, db, "marketing", today, 5.00, 10)
	g := newGateWithThresholds(t, db, 5.0, 100, clock)
	reason, err := g.Check(context.Background(), manifest.PersonaMarketing)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if reason != ReasonBudgetExceeded {
		t.Errorf("at threshold: got reason %q want %q", reason, ReasonBudgetExceeded)
	}
}

func TestBudgetGate_CheckAtCallThresholdDenies(t *testing.T) {
	db := newBudgetDB(t)
	clock := pinnedClock(2026, 5, 15, 12, 0)
	today := clock().Local().Format("2006-01-02")
	insertUsage(t, db, "marketing", today, 1.00, 100)
	g := newGateWithThresholds(t, db, 5.0, 100, clock)
	reason, err := g.Check(context.Background(), manifest.PersonaMarketing)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if reason != ReasonBudgetExceeded {
		t.Errorf("at call threshold: got reason %q want %q", reason, ReasonBudgetExceeded)
	}
}

func TestBudgetGate_CheckYesterdayDoesntBleed(t *testing.T) {
	db := newBudgetDB(t)
	clock := pinnedClock(2026, 5, 15, 12, 0)
	yesterday := time.Date(2026, 5, 14, 12, 0, 0, 0, time.Local).Format("2006-01-02")
	insertUsage(t, db, "marketing", yesterday, 100.00, 1000)
	g := newGateWithThresholds(t, db, 5.0, 100, clock)
	reason, err := g.Check(context.Background(), manifest.PersonaMarketing)
	if err != nil || reason != "" {
		t.Errorf("yesterday should not affect today: got reason=%q err=%v", reason, err)
	}
}

func TestBudgetGate_CheckUnknownPersonaPassesThrough(t *testing.T) {
	db := newBudgetDB(t)
	g := newGateWithThresholds(t, db, 5.0, 100, nil)
	reason, err := g.Check(context.Background(), manifest.Persona("ghost"))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if reason != "" {
		t.Errorf("unknown persona should pass through, got reason %q", reason)
	}
}

func TestBudgetGate_CheckDBErrorReturnsConservativeDeny(t *testing.T) {
	db := newBudgetDB(t)
	g := newGateWithThresholds(t, db, 5.0, 100, nil)
	_ = db.Close()
	reason, err := g.Check(context.Background(), manifest.PersonaMarketing)
	if err == nil {
		t.Fatal("expected db error")
	}
	if reason != ReasonBudgetExceeded {
		t.Errorf("conservative deny: got reason %q want %q", reason, ReasonBudgetExceeded)
	}
}

func TestBudgetGate_CheckRespectsLocalTZ(t *testing.T) {
	db := newBudgetDB(t)
	insertUsage(t, db, "marketing", "2026-05-15", 5.00, 0)
	g := newGateWithThresholds(t, db, 5.0, 100, pinnedClock(2026, 5, 15, 23, 30))
	reason, _ := g.Check(context.Background(), manifest.PersonaMarketing)
	if reason != ReasonBudgetExceeded {
		t.Errorf("at 23:30 same day: got reason %q want %q", reason, ReasonBudgetExceeded)
	}
	g2 := newGateWithThresholds(t, db, 5.0, 100, pinnedClock(2026, 5, 16, 0, 30))
	reason2, _ := g2.Check(context.Background(), manifest.PersonaMarketing)
	if reason2 != "" {
		t.Errorf("at 00:30 next day: got reason %q want allowed", reason2)
	}
}

func readUsage(t *testing.T, db *sql.DB, persona, date string) (cost float64, calls int, exists bool) {
	t.Helper()
	err := db.QueryRow(
		`SELECT cost_usd, call_count FROM persona_budget_usage WHERE persona = ? AND usage_date = ?`,
		persona, date,
	).Scan(&cost, &calls)
	if err == sql.ErrNoRows {
		return 0, 0, false
	}
	if err != nil {
		t.Fatalf("read usage: %v", err)
	}
	return cost, calls, true
}

func TestBudgetGate_IncrementCostCreatesRow(t *testing.T) {
	db := newBudgetDB(t)
	clock := pinnedClock(2026, 5, 15, 12, 0)
	today := clock().Local().Format("2006-01-02")
	g := newGateWithThresholds(t, db, 5.0, 100, clock)
	if err := g.IncrementCost(context.Background(), manifest.PersonaMarketing, 2.50); err != nil {
		t.Fatalf("increment: %v", err)
	}
	cost, calls, exists := readUsage(t, db, "marketing", today)
	if !exists {
		t.Fatal("expected row to be created")
	}
	if cost != 2.50 {
		t.Errorf("cost = %v, want 2.50", cost)
	}
	if calls != 0 {
		t.Errorf("calls = %v, want 0 (only cost incremented)", calls)
	}
}

func TestBudgetGate_IncrementCostAccumulates(t *testing.T) {
	db := newBudgetDB(t)
	clock := pinnedClock(2026, 5, 15, 12, 0)
	today := clock().Local().Format("2006-01-02")
	g := newGateWithThresholds(t, db, 5.0, 100, clock)
	_ = g.IncrementCost(context.Background(), manifest.PersonaMarketing, 1.25)
	_ = g.IncrementCost(context.Background(), manifest.PersonaMarketing, 2.50)
	cost, _, _ := readUsage(t, db, "marketing", today)
	if cost != 3.75 {
		t.Errorf("accumulated cost = %v, want 3.75", cost)
	}
}

func TestBudgetGate_IncrementCallsCreatesRow(t *testing.T) {
	db := newBudgetDB(t)
	clock := pinnedClock(2026, 5, 15, 12, 0)
	today := clock().Local().Format("2006-01-02")
	g := newGateWithThresholds(t, db, 5.0, 100, clock)
	if err := g.IncrementCalls(context.Background(), manifest.PersonaMarketing); err != nil {
		t.Fatalf("increment: %v", err)
	}
	cost, calls, exists := readUsage(t, db, "marketing", today)
	if !exists {
		t.Fatal("expected row")
	}
	if calls != 1 {
		t.Errorf("calls = %v, want 1", calls)
	}
	if cost != 0 {
		t.Errorf("cost = %v, want 0 (only calls incremented)", cost)
	}
}

func TestBudgetGate_IncrementCallsAccumulates(t *testing.T) {
	db := newBudgetDB(t)
	clock := pinnedClock(2026, 5, 15, 12, 0)
	today := clock().Local().Format("2006-01-02")
	g := newGateWithThresholds(t, db, 5.0, 100, clock)
	for i := 0; i < 3; i++ {
		_ = g.IncrementCalls(context.Background(), manifest.PersonaMarketing)
	}
	_, calls, _ := readUsage(t, db, "marketing", today)
	if calls != 3 {
		t.Errorf("calls = %v, want 3", calls)
	}
}

func TestBudgetGate_IncrementSkipsEmptyPersona(t *testing.T) {
	db := newBudgetDB(t)
	clock := pinnedClock(2026, 5, 15, 12, 0)
	today := clock().Local().Format("2006-01-02")
	g := newGateWithThresholds(t, db, 5.0, 100, clock)
	if err := g.IncrementCost(context.Background(), manifest.Persona(""), 1.0); err != nil {
		t.Fatalf("increment empty: %v", err)
	}
	if err := g.IncrementCalls(context.Background(), manifest.Persona("")); err != nil {
		t.Fatalf("increment empty: %v", err)
	}
	_, _, exists := readUsage(t, db, "", today)
	if exists {
		t.Error("expected no row to be created for empty persona")
	}
}

func TestBudgetGate_IncrementUsesLocalTZForDate(t *testing.T) {
	db := newBudgetDB(t)
	clock := pinnedClock(2026, 5, 15, 23, 30)
	g := newGateWithThresholds(t, db, 5.0, 100, clock)
	_ = g.IncrementCalls(context.Background(), manifest.PersonaMarketing)
	_, calls, exists := readUsage(t, db, "marketing", "2026-05-15")
	if !exists || calls != 1 {
		t.Errorf("expected row for 2026-05-15 with calls=1; exists=%v calls=%v", exists, calls)
	}
}
