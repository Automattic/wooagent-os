package pep

import (
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
