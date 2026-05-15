package pep

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

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
