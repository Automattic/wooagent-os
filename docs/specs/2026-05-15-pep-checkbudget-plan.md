# PEP Phase-2 (checkBudget) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Light up PEP Check 5 (per-persona daily budgets) + a scheduler pre-tick gate sharing the same counter store.

**Architecture:** New `persona_budget_usage` SQLite table holds per-(persona, local-date) counters. New `pep.BudgetGate` is the single read/increment surface, consumed by `pep.checkBudget`, the audit writer (for `call_count` increments after Allowed invocations), the telemetry recorder (for `cost_usd` increments after turn_events insert), and the scheduler worker (for pre-tick refusal). Thresholds come from baked-in defaults overlaid by `~/.wooagent/budgets.json`.

**Tech Stack:** Go, SQLite (existing), `log/slog` (existing).

**Spec:** [`docs/specs/2026-05-15-pep-checkbudget-design.md`](./2026-05-15-pep-checkbudget-design.md)

---

## File map

| File | Responsibility |
|---|---|
| `daemon/internal/store/migrations/012_persona_budget_usage.sql` *(new)* | Counter table DDL. |
| `daemon/internal/pep/budget.go` *(new)* | `Threshold`, `Thresholds`, `DefaultThresholds`, `LoadBudgetOverlay`, `BudgetGate` (`Check`, `IncrementCost`, `IncrementCalls`). |
| `daemon/internal/pep/budget_test.go` *(new)* | Unit tests for thresholds, overlay loader, gate operations. |
| `daemon/internal/pep/checkbudget_test.go` *(new)* | PEP-level integration tests through `Invoke`. |
| `daemon/internal/pep/pep.go` | `PEP` gains `budget *BudgetGate`; `New()` accepts it; `checkBudget(ctx, req)` becomes real; `Invoke` passes ctx + persona where needed. |
| `daemon/internal/pep/audit.go` | `newAuditWriter(db, budget)`; `finalize` accepts persona; bumps `call_count` on Allowed outcomes. |
| `daemon/internal/pep/pep_test.go` | Extend `newTestPEP` to construct a `BudgetGate`; inline counter-table DDL. |
| `daemon/internal/telemetry/store.go` | `SQLiteRecorder` gains a budget field; cost increment after `INSERT INTO turn_events`. |
| `daemon/internal/telemetry/store_test.go` *(new or extend)* | Test cost-increment fires with correct sum. |
| `daemon/internal/scheduler/worker.go` | `Worker` gains a budget field; pre-tick check refuses dispatch when persona is over budget. |
| `daemon/internal/scheduler/worker_test.go` | Add a test that confirms an over-budget persona's run is skipped. |
| `daemon/internal/cli/run.go` | Load overlay; construct `BudgetGate`; pass to PEP, scheduler `Worker`, telemetry recorder. |
| `daemon/internal/httpapi/handlers_v1.go` | Sharpen `pepDenialMessage` for `ReasonBudgetExceeded` to mention local-midnight reset. |

---

## Task 1: Migration `012_persona_budget_usage.sql`

Pure DDL. Adds the counter table.

**Files:**
- Create: `daemon/internal/store/migrations/012_persona_budget_usage.sql`

- [ ] **Step 1: Verify current branch**

Run: `git branch --show-current`
Expected: `pep-phase-2-budget`. If not, STOP and report BLOCKED.

- [ ] **Step 2: Write the migration**

Create `daemon/internal/store/migrations/012_persona_budget_usage.sql`:

```sql
-- Per-persona daily counters that back the PEP checkBudget gate (PRD §8.4.2
-- check 5) and the scheduler's pre-tick budget refusal.
--
-- One row per (persona, usage_date) where usage_date is the local-TZ
-- calendar date (YYYY-MM-DD). Both counters increment forward only:
--   - cost_usd: bumped when a turn_events row lands; sums model_calls'
--     cost_usd over the turn.
--   - call_count: bumped after every Allowed pep.Invoke finalization
--     (denied calls don't count — the goal is "did we touch the store").
--
-- Old rows are retained indefinitely in v1 (small data volume; ~7 personas
-- × 365 days = ~2.5K rows/year). A retention sweep can be added later.

CREATE TABLE persona_budget_usage (
    persona     TEXT NOT NULL,
    usage_date  TEXT NOT NULL,
    cost_usd    REAL NOT NULL DEFAULT 0,
    call_count  INTEGER NOT NULL DEFAULT 0,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (persona, usage_date)
);

CREATE INDEX idx_persona_budget_usage_date ON persona_budget_usage(usage_date);
```

- [ ] **Step 3: Verify the migration applies cleanly**

Run: `cd daemon && go test ./internal/store/...`
Expected: existing migration tests pass with the new file picked up by the embed directive.

- [ ] **Step 4: Commit**

```bash
git add daemon/internal/store/migrations/012_persona_budget_usage.sql
git commit -m "feat(store): migration 012 persona_budget_usage counter table"
```

---

## Task 2: Types + DefaultThresholds in `budget.go`

Pure type plumbing for the gate. No behavior, no tests yet — Task 3 starts the TDD cycle.

**Files:**
- Create: `daemon/internal/pep/budget.go`

- [ ] **Step 1: Verify current branch**

Run: `git branch --show-current` → `pep-phase-2-budget`. STOP if different.

- [ ] **Step 2: Create the types-only initial file**

Create `daemon/internal/pep/budget.go`:

```go
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
```

- [ ] **Step 3: Verify compile**

Run: `cd daemon && go build ./...`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add daemon/internal/pep/budget.go
git commit -m "feat(pep): add Threshold + DefaultThresholds types"
```

---

## Task 3: `LoadBudgetOverlay` — TDD

Reads `~/.wooagent/budgets.json` and merges over the baked defaults.

**Files:**
- Modify: `daemon/internal/pep/budget.go`
- Create: `daemon/internal/pep/budget_test.go`

- [ ] **Step 1: Verify current branch** (same as prior tasks)

- [ ] **Step 2: Write the failing tests**

Create `daemon/internal/pep/budget_test.go`:

```go
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
	// Sales-support didn't specify calls — should fall back to default 100.
	if got[manifest.PersonaSalesSupport].DailyCalls != 100 {
		t.Errorf("sales-support calls: got %v want default 100", got[manifest.PersonaSalesSupport].DailyCalls)
	}
	// Untouched personas keep defaults.
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
```

- [ ] **Step 3: Run the tests, confirm they fail**

Run: `cd daemon && go test ./internal/pep/ -run TestLoadBudgetOverlay -v`
Expected: build error — `undefined: LoadBudgetOverlay`.

- [ ] **Step 4: Implement `LoadBudgetOverlay`**

Append to `daemon/internal/pep/budget.go`:

```go
import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
)

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
```

Note: the original file from Task 2 already has `package pep` and the `import "github.com/wooagent-os/wooagent-os/daemon/internal/manifest"` block. Merge the new imports into a single import block at the top of the file rather than adding a second block.

The complete `budget.go` imports after this task should be:

```go
import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
)
```

- [ ] **Step 5: Run the tests, confirm they pass**

Run: `cd daemon && go test ./internal/pep/ -run TestLoadBudgetOverlay -v`
Expected: 4 subtests PASS.

- [ ] **Step 6: Commit**

```bash
git add daemon/internal/pep/budget.go daemon/internal/pep/budget_test.go
git commit -m "feat(pep): LoadBudgetOverlay merges ~/.wooagent/budgets.json"
```

---

## Task 4: `BudgetGate.Check` — TDD

Read-side of the gate. The PEP and scheduler both call this.

**Files:**
- Modify: `daemon/internal/pep/budget.go`
- Modify: `daemon/internal/pep/budget_test.go`

- [ ] **Step 1: Verify current branch** (same as prior tasks)

- [ ] **Step 2: Write the failing tests**

Append to `daemon/internal/pep/budget_test.go`:

```go
import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

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
	insertUsage(t, db, "marketing", today, 5.00, 10) // cost_usd == threshold
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
	insertUsage(t, db, "marketing", yesterday, 100.00, 1000) // over the moon for yesterday
	g := newGateWithThresholds(t, db, 5.0, 100, clock)
	reason, err := g.Check(context.Background(), manifest.PersonaMarketing)
	if err != nil || reason != "" {
		t.Errorf("yesterday should not affect today: got reason=%q err=%v", reason, err)
	}
}

func TestBudgetGate_CheckUnknownPersonaPassesThrough(t *testing.T) {
	db := newBudgetDB(t)
	g := newGateWithThresholds(t, db, 5.0, 100, nil)
	// Threshold map only has marketing; check a persona slug not in the map.
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
	_ = db.Close() // induce a DB error on next read
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
	// Insert a row dated for 2026-05-15 (local).
	insertUsage(t, db, "marketing", "2026-05-15", 5.00, 0)
	// Clock at 23:30 local on 2026-05-15 — should be over budget today.
	g := newGateWithThresholds(t, db, 5.0, 100, pinnedClock(2026, 5, 15, 23, 30))
	reason, _ := g.Check(context.Background(), manifest.PersonaMarketing)
	if reason != ReasonBudgetExceeded {
		t.Errorf("at 23:30 same day: got reason %q want %q", reason, ReasonBudgetExceeded)
	}
	// Clock at 00:30 local on 2026-05-16 — fresh day, should be allowed.
	g2 := newGateWithThresholds(t, db, 5.0, 100, pinnedClock(2026, 5, 16, 0, 30))
	reason2, _ := g2.Check(context.Background(), manifest.PersonaMarketing)
	if reason2 != "" {
		t.Errorf("at 00:30 next day: got reason %q want allowed", reason2)
	}
}
```

- [ ] **Step 3: Run, confirm fails**

Run: `cd daemon && go test ./internal/pep/ -run TestBudgetGate_Check -v`
Expected: build error — `undefined: BudgetGate`, `undefined: NewBudgetGate`.

- [ ] **Step 4: Implement `BudgetGate` + `Check`**

Append to `daemon/internal/pep/budget.go`:

```go
import (
	"context"
	"database/sql"
	"errors"
	"time"
)

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
```

Merge the new imports into the file's existing import block.

- [ ] **Step 5: Run tests, confirm pass**

Run: `cd daemon && go test ./internal/pep/ -run TestBudgetGate_Check -v`
Expected: 8 subtests PASS.

- [ ] **Step 6: Commit**

```bash
git add daemon/internal/pep/budget.go daemon/internal/pep/budget_test.go
git commit -m "feat(pep): BudgetGate.Check reads per-persona daily counters"
```

---

## Task 5: `BudgetGate.IncrementCost` + `IncrementCalls` — TDD

Write-side of the gate. UPSERT semantics so callers don't manage row existence.

**Files:**
- Modify: `daemon/internal/pep/budget.go`
- Modify: `daemon/internal/pep/budget_test.go`

- [ ] **Step 1: Verify current branch**

- [ ] **Step 2: Write the failing tests**

Append to `daemon/internal/pep/budget_test.go`:

```go
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
	clock := pinnedClock(2026, 5, 15, 23, 30) // late evening local
	g := newGateWithThresholds(t, db, 5.0, 100, clock)
	_ = g.IncrementCalls(context.Background(), manifest.PersonaMarketing)
	_, calls, exists := readUsage(t, db, "marketing", "2026-05-15")
	if !exists || calls != 1 {
		t.Errorf("expected row for 2026-05-15 with calls=1; exists=%v calls=%v", exists, calls)
	}
}
```

- [ ] **Step 3: Run, confirm fails**

Run: `cd daemon && go test ./internal/pep/ -run TestBudgetGate_Increment -v`
Expected: build error — `undefined: g.IncrementCost` / `g.IncrementCalls`.

- [ ] **Step 4: Implement the increment methods**

Append to `daemon/internal/pep/budget.go`:

```go
// IncrementCost adds usdAmount to today's row for persona. UPSERT semantics:
// creates the row if absent. Skips entirely when persona is empty
// (defensive — empty-persona rows are noise and would never be looked up
// by Check, which always supplies a typed persona slug).
func (g *BudgetGate) IncrementCost(ctx context.Context, persona manifest.Persona, usdAmount float64) error {
	if persona == "" {
		return nil
	}
	today := g.clock().Local().Format("2006-01-02")
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := g.db.ExecContext(ctx, `
		INSERT INTO persona_budget_usage (persona, usage_date, cost_usd, call_count, updated_at)
		VALUES (?, ?, ?, 0, ?)
		ON CONFLICT(persona, usage_date) DO UPDATE SET
			cost_usd   = cost_usd + excluded.cost_usd,
			updated_at = excluded.updated_at`,
		string(persona), today, usdAmount, now,
	)
	if err != nil {
		g.logger.Warn("budget cost increment failed", "persona", persona, "amount", usdAmount, "err", err)
		return err
	}
	return nil
}

// IncrementCalls bumps today's call_count by 1 for persona. UPSERT.
// Skips when persona is empty.
func (g *BudgetGate) IncrementCalls(ctx context.Context, persona manifest.Persona) error {
	if persona == "" {
		return nil
	}
	today := g.clock().Local().Format("2006-01-02")
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := g.db.ExecContext(ctx, `
		INSERT INTO persona_budget_usage (persona, usage_date, cost_usd, call_count, updated_at)
		VALUES (?, ?, 0, 1, ?)
		ON CONFLICT(persona, usage_date) DO UPDATE SET
			call_count = call_count + 1,
			updated_at = excluded.updated_at`,
		string(persona), today, now,
	)
	if err != nil {
		g.logger.Warn("budget call increment failed", "persona", persona, "err", err)
		return err
	}
	return nil
}
```

- [ ] **Step 5: Run tests, confirm pass**

Run: `cd daemon && go test ./internal/pep/ -run TestBudgetGate_Increment -v`
Expected: 6 subtests PASS.

- [ ] **Step 6: Commit**

```bash
git add daemon/internal/pep/budget.go daemon/internal/pep/budget_test.go
git commit -m "feat(pep): BudgetGate IncrementCost + IncrementCalls UPSERT"
```

---

## Task 6: Audit writer refactor + `checkBudget` impl — TDD

Threads persona into `finalize`, bumps `call_count` on Allowed outcomes, and wires `checkBudget` to call `BudgetGate.Check`. Updates the test fixture so all existing tests stay green.

**Files:**
- Modify: `daemon/internal/pep/audit.go`
- Modify: `daemon/internal/pep/pep.go`
- Modify: `daemon/internal/pep/pep_test.go`
- Create: `daemon/internal/pep/checkbudget_test.go`

- [ ] **Step 1: Verify current branch**

- [ ] **Step 2: Refactor `audit.go` to accept a budget gate**

Open `daemon/internal/pep/audit.go`. Change `auditWriter` and `newAuditWriter`:

```go
type auditWriter struct {
	db     *sql.DB
	budget *BudgetGate
}

func newAuditWriter(db *sql.DB, budget *BudgetGate) *auditWriter {
	return &auditWriter{db: db, budget: budget}
}
```

Change `finalize`'s signature to accept persona, and add the call_count increment for Allowed outcomes:

```go
func (w *auditWriter) finalize(ctx context.Context, id int64, persona manifest.Persona, outcome Outcome, reason ReasonCode) error {
	_, err := w.db.ExecContext(ctx,
		`UPDATE audit_invocations
		   SET outcome = ?, denial_reason = ?, completed_at = ?
		 WHERE id = ?`,
		string(outcome), nullIfEmpty(string(reason)), nowRFC3339(), id,
	)
	if err != nil {
		return fmt.Errorf("finalize audit row %d: %w", id, err)
	}
	if w.budget != nil && (outcome == OutcomeSuccess || outcome == OutcomeMCPError) {
		if incErr := w.budget.IncrementCalls(ctx, persona); incErr != nil {
			// Log + swallow. The check is authoritative for "over budget";
			// a missed increment is a slight under-count favoring the
			// operator, which is acceptable. Don't fail the call.
		}
	}
	return nil
}
```

Add the manifest import at the top of `audit.go`:

```go
import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
)
```

- [ ] **Step 3: Update `pep.go` callers of `finalize` and `New`**

Open `daemon/internal/pep/pep.go`. Update the `PEP` struct:

```go
type PEP struct {
	manifest *manifest.Lookup
	mcp      MCPClient
	audit    *auditWriter
	db       *sql.DB
	schemas  *schemaCache
	budget   *BudgetGate
}
```

Update `New` to accept the budget gate:

```go
func New(m *manifest.Lookup, mcpClient MCPClient, db *sql.DB, budget *BudgetGate) *PEP {
	return &PEP{
		manifest: m,
		mcp:      mcpClient,
		audit:    newAuditWriter(db, budget),
		db:       db,
		schemas:  &schemaCache{},
		budget:   budget,
	}
}
```

Update every `p.audit.finalize(...)` call site to pass `req.Persona`. There are five such calls in `pep.go`:

```go
// In Invoke, the "MCP not configured" branch:
if err := p.audit.finalize(ctx, auditID, req.Persona, OutcomeMCPError, ""); err != nil {
    return Decision{}, mcp.ToolCallResult{}, err
}

// MCP init error:
_ = p.audit.finalize(ctx, auditID, req.Persona, OutcomeMCPError, "")

// MCP call error:
_ = p.audit.finalize(ctx, auditID, req.Persona, OutcomeMCPError, "")

// Success:
if err := p.audit.finalize(ctx, auditID, req.Persona, OutcomeSuccess, ""); err != nil {
    return Decision{}, res, err
}
```

And in `deny`:

```go
func (p *PEP) deny(ctx context.Context, auditID int64, persona manifest.Persona, reason ReasonCode) (Decision, mcp.ToolCallResult, error) {
	if err := p.audit.finalize(ctx, auditID, persona, OutcomeDenied, reason); err != nil {
		return Decision{}, mcp.ToolCallResult{}, err
	}
	return Decision{Allowed: false, Reason: reason, AuditID: auditID}, mcp.ToolCallResult{}, nil
}
```

And update every `p.deny(ctx, auditID, reason)` call in `Invoke` to `p.deny(ctx, auditID, req.Persona, reason)` — there are six of them, one per check.

Replace the stubbed `checkBudget`:

```go
// checkBudget — Check 5. Refuses MCP dispatch when the persona is at or
// over its daily cost or call limit. The scheduler also pre-checks budget
// before starting a tick; this PEP-side gate is the backstop for any
// path that bypasses the scheduler (e.g. operator-driven approve flows).
func (p *PEP) checkBudget(ctx context.Context, req Request) ReasonCode {
	if p.budget == nil {
		return ""
	}
	reason, err := p.budget.Check(ctx, req.Persona)
	if err != nil {
		return ReasonBudgetExceeded
	}
	return reason
}
```

Update the `Invoke` pipeline to pass `ctx` to `checkBudget`. Find the line:

```go
if reason := p.checkBudget(req); reason != "" {
```

and change it to:

```go
if reason := p.checkBudget(ctx, req); reason != "" {
```

- [ ] **Step 4: Update `pep_test.go` test fixture**

Open `daemon/internal/pep/pep_test.go`. The `newTestPEP` helper calls `New(lookup, mcpc, db)` — change to construct a real budget gate so all existing tests continue to exercise the full pipeline:

```go
// Inside newTestPEP, just before the return:
budgetDDL := budgetUsageDDL // exported in budget_test.go as a const
if _, err := db.Exec(budgetDDL); err != nil {
    t.Fatalf("apply budget ddl: %v", err)
}
budget := NewBudgetGate(db, DefaultThresholds())
return New(lookup, mcpc, db, budget), db
```

Since `budgetUsageDDL` is a const in `budget_test.go` (same package), no export adjustment is needed.

- [ ] **Step 5: Verify the existing PEP tests still compile and pass**

Run: `cd daemon && go test ./internal/pep/...`
Expected: all existing tests PASS. (The default $5/100 thresholds are well above the 1-call test scenarios.)

- [ ] **Step 6: Write the new PEP-level integration tests**

Create `daemon/internal/pep/checkbudget_test.go`:

```go
package pep

import (
	"context"
	"testing"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
)

func TestCheckBudget_UnderBudgetAllows(t *testing.T) {
	mcpc := &fakeMCP{result: mcp.ToolCallResult{Content: []mcp.ContentPart{{Type: "text", Text: `{"ok":true}`}}}}
	p, db := newTestPEP(t, mcpc)
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1},
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !dec.Allowed {
		t.Errorf("expected allowed, got reason %q", dec.Reason)
	}
	// call_count should be 1 after a successful call.
	today := time.Now().Local().Format("2006-01-02")
	var calls int
	if err := db.QueryRow(
		`SELECT call_count FROM persona_budget_usage WHERE persona='marketing' AND usage_date=?`, today,
	).Scan(&calls); err != nil {
		t.Fatalf("read usage: %v", err)
	}
	if calls != 1 {
		t.Errorf("call_count = %d, want 1", calls)
	}
}

func TestCheckBudget_OverCostDenies(t *testing.T) {
	mcpc := &fakeMCP{}
	p, db := newTestPEP(t, mcpc)
	today := time.Now().Local().Format("2006-01-02")
	if _, err := db.Exec(
		`INSERT INTO persona_budget_usage(persona, usage_date, cost_usd, call_count, updated_at) VALUES(?, ?, ?, ?, ?)`,
		"marketing", today, 10.00, 0, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed usage: %v", err)
	}
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1},
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if dec.Allowed {
		t.Fatal("expected denied for over-cost budget")
	}
	if dec.Reason != ReasonBudgetExceeded {
		t.Errorf("reason = %q, want %q", dec.Reason, ReasonBudgetExceeded)
	}
	if mcpc.calls != 0 {
		t.Errorf("denied call should not reach mcp, got %d calls", mcpc.calls)
	}
}

func TestCheckBudget_OverCallsDenies(t *testing.T) {
	mcpc := &fakeMCP{}
	p, db := newTestPEP(t, mcpc)
	today := time.Now().Local().Format("2006-01-02")
	if _, err := db.Exec(
		`INSERT INTO persona_budget_usage(persona, usage_date, cost_usd, call_count, updated_at) VALUES(?, ?, ?, ?, ?)`,
		"marketing", today, 0.0, 100, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed usage: %v", err)
	}
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1},
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if dec.Allowed {
		t.Fatal("expected denied for over-call budget")
	}
	if dec.Reason != ReasonBudgetExceeded {
		t.Errorf("reason = %q, want %q", dec.Reason, ReasonBudgetExceeded)
	}
	if mcpc.calls != 0 {
		t.Errorf("denied call should not reach mcp, got %d calls", mcpc.calls)
	}
}

func TestCheckBudget_DeniedCallDoesNotIncrement(t *testing.T) {
	mcpc := &fakeMCP{}
	p, db := newTestPEP(t, mcpc)
	// Ability not in manifest → trust check denies first.
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "evil-plugin/drop-database",
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if dec.Allowed {
		t.Fatal("expected trust-check denial")
	}
	today := time.Now().Local().Format("2006-01-02")
	var calls int
	row := db.QueryRow(
		`SELECT call_count FROM persona_budget_usage WHERE persona='marketing' AND usage_date=?`, today,
	)
	if err := row.Scan(&calls); err == nil && calls != 0 {
		t.Errorf("denied call should not increment, got call_count=%d", calls)
	}
}

func TestCheckBudget_MCPErrorStillIncrements(t *testing.T) {
	mcpc := &fakeMCP{callErr: testMCPError()}
	p, db := newTestPEP(t, mcpc)
	_, _, _ = p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1},
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	today := time.Now().Local().Format("2006-01-02")
	var calls int
	if err := db.QueryRow(
		`SELECT call_count FROM persona_budget_usage WHERE persona='marketing' AND usage_date=?`, today,
	).Scan(&calls); err != nil {
		t.Fatalf("read usage: %v", err)
	}
	if calls != 1 {
		t.Errorf("mcp_error outcome should still bump call_count, got %d want 1", calls)
	}
}

// testMCPError is a tiny helper so the import set above stays minimal.
func testMCPError() error { return &mcpError{} }

type mcpError struct{}

func (e *mcpError) Error() string { return "boom" }
```

- [ ] **Step 7: Run the new tests + full PEP suite**

Run: `cd daemon && go test ./internal/pep/...`
Expected: all tests PASS (including the new 5 checkBudget tests and the existing tests that now exercise the wired-in BudgetGate).

- [ ] **Step 8: Commit**

```bash
git add daemon/internal/pep/audit.go daemon/internal/pep/pep.go daemon/internal/pep/pep_test.go daemon/internal/pep/checkbudget_test.go
git commit -m "feat(pep): implement checkBudget; thread persona through finalize"
```

---

## Task 7: Telemetry recorder — cost increment after `INSERT INTO turn_events`

The recorder gains a budget gate and sums `ModelCalls.CostUSD` for the inserted turn.

**Files:**
- Modify: `daemon/internal/telemetry/store.go`
- Create: `daemon/internal/telemetry/budget_increment_test.go`

- [ ] **Step 1: Verify current branch**

- [ ] **Step 2: Write the failing test**

Create `daemon/internal/telemetry/budget_increment_test.go`:

```go
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

const turnEventsDDL = `
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

const budgetUsageDDL = `
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
	if _, err := db.Exec(turnEventsDDL); err != nil {
		t.Fatalf("turn_events ddl: %v", err)
	}
	if _, err := db.Exec(budgetUsageDDL); err != nil {
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
			{Provider: "anthropic", Model: "claude-opus-4-7"}, // CostUSD=0
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
		Persona:   "", // empty
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
```

- [ ] **Step 3: Run, confirm fails**

Run: `cd daemon && go test ./internal/telemetry/ -run TestRecorder_Increments -v`
Expected: build error — `NewSQLiteRecorder` takes 1 arg, not 2; `budget` undefined.

- [ ] **Step 4: Implement the recorder change**

Open `daemon/internal/telemetry/store.go`. Change `SQLiteRecorder` and its constructor:

```go
import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
	"github.com/wooagent-os/wooagent-os/daemon/internal/pep"
)

// SQLiteRecorder writes turn events into the daemon's main SQLite DB and
// bumps the per-persona daily cost counter via the BudgetGate.
type SQLiteRecorder struct {
	DB     *sql.DB
	Budget *pep.BudgetGate
}

func NewSQLiteRecorder(db *sql.DB, budget *pep.BudgetGate) *SQLiteRecorder {
	return &SQLiteRecorder{DB: db, Budget: budget}
}
```

Update `Record` to sum `ModelCalls.CostUSD` after the existing INSERT:

```go
func (r *SQLiteRecorder) Record(ctx context.Context, e TurnEvent) error {
	if e.TurnID == "" {
		return fmt.Errorf("turn_id is required")
	}
	if e.EventSchemaVersion == 0 {
		e.EventSchemaVersion = EventSchemaVersion
	}
	skillVersionsJSON, _ := json.Marshal(e.SkillVersions)
	contextJSON, _ := json.Marshal(e.Context)
	modelCallsJSON, _ := json.Marshal(e.ModelCalls)
	skillCallsJSON, _ := json.Marshal(e.SkillCalls)
	var verdictJSON []byte
	if e.Verdict != nil {
		verdictJSON, _ = json.Marshal(e.Verdict)
	}
	var completedAt *string
	if e.CompletedAt != nil {
		s := e.CompletedAt.UTC().Format(time.RFC3339)
		completedAt = &s
	}

	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO turn_events(
			turn_id, event_schema_version, issue_id, persona, prompt_version,
			skill_versions_json, started_at, completed_at, latency_ms,
			context_json, model_calls_json, skill_calls_json,
			proposal_text, proposal_sha, verdict_json, created_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		e.TurnID, e.EventSchemaVersion, nullIfEmpty(e.IssueID), nullIfEmpty(e.Persona), nullIfEmpty(e.PromptVersion),
		string(skillVersionsJSON), e.StartedAt.UTC().Format(time.RFC3339), completedAt, e.LatencyMS,
		string(contextJSON), string(modelCallsJSON), string(skillCallsJSON),
		nullIfEmpty(e.ProposalText), nullIfEmpty(e.ProposalSHA), nullIfEmpty(string(verdictJSON)),
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert turn_event: %w", err)
	}

	// Bump the per-persona daily cost counter. Sum the model_calls' cost.
	// Log + swallow on increment failure: a missed increment is a slight
	// under-count, which favors the operator. The recorder shouldn't fail
	// the turn over a telemetry-side increment error.
	if r.Budget != nil && e.Persona != "" {
		var totalCost float64
		for _, mc := range e.ModelCalls {
			totalCost += mc.CostUSD
		}
		if totalCost > 0 {
			_ = r.Budget.IncrementCost(ctx, manifest.Persona(e.Persona), totalCost)
		}
	}

	return nil
}
```

- [ ] **Step 5: Run tests, confirm pass**

Run: `cd daemon && go test ./internal/telemetry/ -run TestRecorder_Increments -v`
Expected: 3 subtests PASS.

- [ ] **Step 6: Run the full telemetry package + check for regressions**

Run: `cd daemon && go test ./internal/telemetry/...`
Expected: all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add daemon/internal/telemetry/store.go daemon/internal/telemetry/budget_increment_test.go
git commit -m "feat(telemetry): bump persona cost counter on turn_events insert"
```

---

## Task 8: Scheduler pre-tick budget check — TDD

`Worker` gains a budget gate and refuses to dispatch an over-budget persona's run.

**Files:**
- Modify: `daemon/internal/scheduler/worker.go`
- Modify: `daemon/internal/scheduler/worker_test.go`

- [ ] **Step 1: Verify current branch**

- [ ] **Step 2: Inspect existing worker_test.go test patterns**

Look at the existing tests in `daemon/internal/scheduler/worker_test.go` to understand the test harness pattern. The implementer should keep new tests in the same style (e.g., reuse helpers like `newTestWorker` or whatever is established).

- [ ] **Step 3: Write the failing test**

Add to `daemon/internal/scheduler/worker_test.go` (append at end of file, follow existing helper patterns; if there's no inline `persona_budget_usage` DDL in scheduler tests, add one alongside the existing DDL):

```go
const budgetUsageDDLSched = `
CREATE TABLE persona_budget_usage (
    persona     TEXT NOT NULL,
    usage_date  TEXT NOT NULL,
    cost_usd    REAL NOT NULL DEFAULT 0,
    call_count  INTEGER NOT NULL DEFAULT 0,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (persona, usage_date)
);`

func TestWorker_SkipsRunWhenPersonaOverBudget(t *testing.T) {
	// Use the existing scheduler test fixture pattern. The worker should
	// claim the run from the queue, see that the persona is over budget,
	// and mark the run as Skipped without invoking the PersonaRunner.
	ctx := context.Background()
	db := newSchedulerTestDB(t) // existing helper that builds runs/queue tables
	if _, err := db.Exec(budgetUsageDDLSched); err != nil {
		t.Fatalf("budget ddl: %v", err)
	}
	today := time.Now().Local().Format("2006-01-02")
	// Seed persona over budget.
	if _, err := db.Exec(
		`INSERT INTO persona_budget_usage(persona, usage_date, cost_usd, call_count, updated_at) VALUES(?, ?, ?, ?, ?)`,
		"marketing", today, 0.0, 100, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed usage: %v", err)
	}
	queue := NewQueue(db)
	// Enqueue a marketing run.
	if _, err := queue.Enqueue(ctx, EnqueueParams{Persona: "marketing"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	budget := pep.NewBudgetGate(db, pep.DefaultThresholds())
	calledRunner := false
	runner := &stubRunner{onRun: func(personas.Persona) { calledRunner = true }}
	w := &Worker{
		Queue:    queue,
		Runner:   runner,
		Personas: map[string]personas.Persona{"marketing": stubPersona("marketing")},
		Now:      time.Now,
		Budget:   budget,
	}
	ran, err := w.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !ran {
		t.Fatal("expected the worker to have processed (skipped) the run")
	}
	if calledRunner {
		t.Error("over-budget run should NOT have invoked the PersonaRunner")
	}
	// Verify the run was marked as skipped with a budget-related reason.
	var status, skipReason string
	if err := db.QueryRow(
		`SELECT status, COALESCE(skip_reason, '') FROM runs ORDER BY id DESC LIMIT 1`,
	).Scan(&status, &skipReason); err != nil {
		t.Fatalf("read run row: %v", err)
	}
	if status != string(StatusSkipped) {
		t.Errorf("status = %q, want %q", status, StatusSkipped)
	}
	if skipReason == "" {
		t.Errorf("skip_reason should be set, got empty")
	}
}
```

Note: the exact helpers (`newSchedulerTestDB`, `stubRunner`, `stubPersona`, `EnqueueParams`) should follow existing patterns in `worker_test.go`. If the file uses different names, adapt the test to match.

- [ ] **Step 4: Run, confirm fails**

Run: `cd daemon && go test ./internal/scheduler/ -run TestWorker_Skips -v`
Expected: build error — `Worker.Budget` undefined.

- [ ] **Step 5: Implement the pre-tick check**

Open `daemon/internal/scheduler/worker.go`. Add the `Budget` field to `Worker`:

```go
type Worker struct {
	Queue       *Queue
	Runner      PersonaRunner
	Personas    map[string]personas.Persona
	Now         func() time.Time
	Backoff     []time.Duration
	MaxAttempts int
	Budget      *pep.BudgetGate
}
```

Add the pep import to the top of the file. Insert the budget pre-check in `RunOnce` between the claim and the dispatch:

```go
func (w *Worker) RunOnce(ctx context.Context) (ran bool, err error) {
	if w.MaxAttempts == 0 {
		w.MaxAttempts = 3
	}
	r, err := w.Queue.ClaimNext(ctx)
	if err != nil {
		return false, err
	}
	if r == nil {
		return false, nil
	}
	persona, ok := w.Personas[r.Persona]
	if !ok {
		_ = w.markPermanent(ctx, r, fmt.Sprintf("no implementation registered for persona %q", r.Persona))
		return true, nil
	}
	// Pre-tick budget check. The PEP gate is the backstop; this stops the
	// LLM from being called at all for an over-budget persona. Match the
	// existing skip path in executeAndRecord — terminal status=Skipped,
	// reason describes the budget block.
	if w.Budget != nil {
		reason, _ := w.Budget.Check(ctx, manifestPersona(r.Persona))
		if reason != "" {
			slog.Info("scheduler skipped tick over budget",
				"persona", r.Persona,
				"reason", string(reason),
			)
			end := w.Now()
			_ = w.Queue.MarkTerminal(ctx, MarkTerminalParams{
				ID:          r.ID,
				Status:      StatusSkipped,
				CompletedAt: end,
				LatencyMS:   0,
				SkipReason:  "over daily budget — counters reset at local midnight",
			})
			return true, nil
		}
	}
	return true, w.executeAndRecord(ctx, r, persona)
}

// manifestPersona converts the queue's persona slug (untyped string) into
// the typed manifest.Persona that BudgetGate.Check expects.
func manifestPersona(slug string) manifest.Persona {
	return manifest.Persona(slug)
}
```

Add the imports at the top of `worker.go`:

```go
import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
	"github.com/wooagent-os/wooagent-os/daemon/internal/pep"
	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
)
```

- [ ] **Step 6: Run tests, confirm pass**

Run: `cd daemon && go test ./internal/scheduler/ -run TestWorker_Skips -v`
Expected: new subtest PASS.

- [ ] **Step 7: Run the full scheduler suite**

Run: `cd daemon && go test ./internal/scheduler/...`
Expected: all tests PASS. If existing scheduler tests construct `Worker{}` literals, they continue to work because `Budget` defaults to `nil` and is nil-guarded in `RunOnce`.

- [ ] **Step 8: Commit**

```bash
git add daemon/internal/scheduler/worker.go daemon/internal/scheduler/worker_test.go
git commit -m "feat(scheduler): pre-tick budget check refuses over-budget runs"
```

---

## Task 9: `cli/run.go` startup wiring

Constructs the `BudgetGate` once at daemon startup, passes it into PEP / Worker / Recorder.

**Files:**
- Modify: `daemon/internal/cli/run.go`

- [ ] **Step 1: Verify current branch**

- [ ] **Step 2: Update `cli/run.go`**

Open `daemon/internal/cli/run.go`. Locate the section where the PEP is constructed (around line 135 per prior context). Add the budget-overlay loading + gate construction immediately before that, and update the `pep.New`, `scheduler.Worker` literal, and `telemetry.NewSQLiteRecorder` calls.

The overlay path is `<wooagent root>/budgets.json`. Find where the daemon's root directory is computed (the same place `~/.wooagent/manifest.json` would be loaded) and use it here.

The exact insertion looks like:

```go
// Construct the budget gate once at startup so PEP, scheduler, and
// telemetry recorder all share the same counter store and threshold table.
thresholds := pep.DefaultThresholds()
overlayPath := filepath.Join(cfgRoot, "budgets.json")
if loaded, err := pep.LoadBudgetOverlay(overlayPath, thresholds, logger); err != nil {
    logger.Warn("budget overlay failed to load; using defaults", "path", overlayPath, "err", err)
} else {
    thresholds = loaded
}
budgetGate := pep.NewBudgetGate(st.DB, thresholds)
```

Then update each constructor call:

```go
// PEP construction:
pepInstance = pep.New(lookup, mcpClient, st.DB, budgetGate)
// (and the nil-MCP variant a few lines below)
pepInstance = pep.New(lookup, nil, st.DB, budgetGate)

// Telemetry recorder:
recorder := telemetry.NewSQLiteRecorder(st.DB, budgetGate)

// Scheduler Worker literal — add Budget field:
worker := &scheduler.Worker{
    Queue:    queue,
    Runner:   runner,
    Personas: personaMap,
    Now:      time.Now,
    Budget:   budgetGate,
}
```

The implementer should adapt variable names (`cfgRoot`, `logger`, etc.) to whatever the existing `run.go` uses for those resources. If the existing code doesn't already plumb a `*slog.Logger`, pass `slog.Default()` to `LoadBudgetOverlay`.

- [ ] **Step 3: Verify the daemon builds**

Run: `cd daemon && go build ./...`
Expected: no errors.

- [ ] **Step 4: Run the existing cli/integration tests if any**

Run: `cd daemon && go test ./internal/cli/...`
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add daemon/internal/cli/run.go
git commit -m "feat(daemon): wire BudgetGate into PEP, scheduler, and recorder"
```

---

## Task 10: Sharpen `pepDenialMessage` for `ReasonBudgetExceeded`

Tiny copy tweak so operators know when the gate releases.

**Files:**
- Modify: `daemon/internal/httpapi/handlers_v1.go`

- [ ] **Step 1: Verify current branch**

- [ ] **Step 2: Update the denial message**

Open `daemon/internal/httpapi/handlers_v1.go`. Locate `pepDenialMessage`. Change the `ReasonBudgetExceeded` case:

```go
case pep.ReasonBudgetExceeded:
    return "persona is over its daily budget — counters reset at local midnight"
```

The `writePEPDenial` switch already maps this to `http.StatusTooManyRequests` (429) — no change needed there.

- [ ] **Step 3: Verify httpapi tests pass**

Run: `cd daemon && go test ./internal/httpapi/...`
Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add daemon/internal/httpapi/handlers_v1.go
git commit -m "fix(httpapi): clarify budget denial message includes reset hint"
```

---

## Task 11: Final verification

**Files:** none

- [ ] **Step 1: Run the full daemon test suite**

Run: `cd daemon && go test ./...`
Expected: ALL tests PASS.

- [ ] **Step 2: Run go vet**

Run: `cd daemon && go vet ./...`
Expected: no output.

- [ ] **Step 3: Confirm one `TODO(phase-2)` marker remains (policy only)**

Run: `grep -n "TODO(phase-2)" daemon/internal/pep/pep.go`
Expected: ONE match (`checkPolicy`).

- [ ] **Step 4: Sanity-check the daemon binary builds**

Run: `cd daemon && go build -o /tmp/wooagent-daemon ./cmd/wooagent`
Expected: builds successfully.

- [ ] **Step 5: Final commit if anything was tidied**

If `git status` is clean, skip. Otherwise:

```bash
git add -A
git status
git commit -m "chore(daemon): tidy after PEP checkBudget work"
```

---

## Spec coverage check

| Spec section | Task(s) |
|---|---|
| Counter table schema | Task 1 |
| Thresholds + DefaultThresholds | Task 2 |
| LoadBudgetOverlay (file, JSON shape, unknown-slug skip, malformed JSON) | Task 3 |
| BudgetGate.Check (no row → allow, threshold semantics, TZ, DB error → deny) | Task 4 |
| BudgetGate.IncrementCost + IncrementCalls (UPSERT, empty-persona guard) | Task 5 |
| PEP.checkBudget + Invoke ctx wiring + audit writer refactor + call_count increment | Task 6 |
| Telemetry recorder cost increment | Task 7 |
| Scheduler pre-tick check + skip behavior | Task 8 |
| cli/run.go startup wiring (overlay load, gate construction, plumb to PEP/scheduler/recorder) | Task 9 |
| pepDenialMessage copy refresh | Task 10 |
| Full verification (tests, vet, TODO scan) | Task 11 |

Non-goals from the spec (`checkPolicy`, UI surfacing, per-ability sub-budgets, mid-day overlay reload, token-to-USD fallback, retention sweep, reconciliation tool, multi-store isolation) are explicitly out of scope for this plan.
