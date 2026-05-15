package pep

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
)

// Threshold is one persona's daily limits. A value of zero means "no limit
// in this dimension" — useful if an operator wants to gate on calls only,
// not cost (or vice versa).
type Threshold struct {
	DailyCostUSD float64
	DailyCalls   int
}

// Thresholds maps persona slug to per-persona limits. Construct via
// DefaultThresholds() or LoadBudgetOverlay().
type Thresholds = map[manifest.Persona]Threshold

// overlayEntry is the JSON shape per persona in budgets.json. Pointer fields
// distinguish "not set" (fall back to default) from "explicit zero" (which
// is also a valid override meaning "no limit").
type overlayEntry struct {
	DailyCostUSD *float64 `json:"daily_cost_usd,omitempty"`
	DailyCalls   *int     `json:"daily_calls,omitempty"`
}

// LoadBudgetOverlay merges values from path on top of base. Missing fields
// fall back to base. Missing personas use base entirely. Returns base
// unchanged if path doesn't exist. Unknown persona slugs in the file are
// logged at Warn and skipped (so a typo in budgets.json doesn't refuse
// startup). Malformed JSON returns an error; the caller decides whether
// to log + fall back or refuse.
func LoadBudgetOverlay(path string, base Thresholds, logger *slog.Logger) (Thresholds, error) {
	out := make(Thresholds, len(base))
	for k, v := range base {
		out[k] = v
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, fmt.Errorf("read budgets overlay: %w", err)
	}
	var overlay map[string]overlayEntry
	if err := json.Unmarshal(data, &overlay); err != nil {
		return out, fmt.Errorf("parse budgets overlay: %w", err)
	}
	for slug, entry := range overlay {
		persona := manifest.Persona(slug)
		current, known := out[persona]
		if !known {
			if logger != nil {
				logger.Warn("budgets.json: unknown persona slug, skipping", "slug", slug)
			}
			continue
		}
		if entry.DailyCostUSD != nil {
			current.DailyCostUSD = *entry.DailyCostUSD
		}
		if entry.DailyCalls != nil {
			current.DailyCalls = *entry.DailyCalls
		}
		out[persona] = current
	}
	return out, nil
}

// DefaultThresholds returns a fresh map of the baked-in default per-persona
// budgets. Conservative — high enough that normal operation doesn't trip
// them, low enough to catch a runaway loop within hours. Operators override
// via ~/.wooagent/budgets.json (see LoadBudgetOverlay).
func DefaultThresholds() Thresholds {
	return Thresholds{
		manifest.PersonaMarketing:    {DailyCostUSD: 5.00, DailyCalls: 100},
		manifest.PersonaPricing:      {DailyCostUSD: 5.00, DailyCalls: 100},
		manifest.PersonaInventory:    {DailyCostUSD: 5.00, DailyCalls: 100},
		manifest.PersonaAccounting:   {DailyCostUSD: 5.00, DailyCalls: 100},
		manifest.PersonaReporting:    {DailyCostUSD: 5.00, DailyCalls: 100},
		manifest.PersonaSalesSupport: {DailyCostUSD: 5.00, DailyCalls: 100},
		manifest.PersonaChiefOfStaff: {DailyCostUSD: 5.00, DailyCalls: 100},
	}
}

// BudgetGate is the per-persona daily budget enforcer. Both pep.checkBudget
// and the scheduler's pre-tick check consume the same gate, so spend and
// call accounting stay consistent.
//
// Goroutine-safe: holds no mutable in-memory state. All state lives in
// SQLite (persona_budget_usage), which serializes writers; concurrent
// callers may briefly overshoot a threshold (both pass a check at 99/100
// then both increment to 101) — acceptable for a budget meant to catch
// runaway loops, not enforce a hard cap.
type BudgetGate struct {
	db         *sql.DB
	thresholds Thresholds
	clock      func() time.Time
	logger     *slog.Logger
}

// NewBudgetGate constructs a gate. Defaults clock to time.Now and logger to
// slog.Default(); tests overwrite these directly.
func NewBudgetGate(db *sql.DB, thresholds Thresholds) *BudgetGate {
	return &BudgetGate{
		db:         db,
		thresholds: thresholds,
		clock:      time.Now,
		logger:     slog.Default(),
	}
}

// Check reports whether the persona is at or over its daily cost or call
// limit RIGHT NOW. Returns the typed reason code when blocked, "" when
// within budget. On DB error, returns (ReasonBudgetExceeded, err) —
// conservative deny matches the rest of the PEP's posture.
func (g *BudgetGate) Check(ctx context.Context, persona manifest.Persona) (ReasonCode, error) {
	today := g.clock().Local().Format("2006-01-02")
	var costUSD float64
	var callCount int
	err := g.db.QueryRowContext(ctx,
		`SELECT cost_usd, call_count FROM persona_budget_usage WHERE persona = ? AND usage_date = ?`,
		string(persona), today,
	).Scan(&costUSD, &callCount)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		g.logger.Warn("budget check db error", "persona", persona, "err", err)
		return ReasonBudgetExceeded, err
	}
	threshold, ok := g.thresholds[persona]
	if !ok {
		g.logger.Warn("budget check: persona has no threshold entry, passing through", "persona", persona)
		return "", nil
	}
	if costUSD >= threshold.DailyCostUSD || callCount >= threshold.DailyCalls {
		return ReasonBudgetExceeded, nil
	}
	return "", nil
}
