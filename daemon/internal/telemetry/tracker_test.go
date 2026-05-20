package telemetry

import (
	"errors"
	"strings"
	"testing"
)

func TestTracker_RecordModelCall_NoBudgetAccumulates(t *testing.T) {
	tr := NewTracker("turn-1", "marketing")
	for i := 0; i < 5; i++ {
		if err := tr.RecordModelCall(ModelCall{
			Provider: "anthropic",
			Model:    "claude-sonnet-4-6",
			CostUSD:  0.10,
		}); err != nil {
			t.Fatalf("call %d returned err with no budget set: %v", i, err)
		}
	}
	if got := tr.TotalCostUSD(); got < 0.499 || got > 0.501 {
		t.Errorf("TotalCostUSD = %v, want ~0.50", got)
	}
}

func TestTracker_RecordModelCall_BudgetTrips(t *testing.T) {
	tr := NewTracker("turn-1", "marketing")
	tr.SetBudgetUSD(0.25)

	// First two calls fit: 0.10 + 0.10 = 0.20 < 0.25.
	for i := 0; i < 2; i++ {
		if err := tr.RecordModelCall(ModelCall{CostUSD: 0.10}); err != nil {
			t.Fatalf("call %d should fit under cap: %v", i, err)
		}
	}
	// Third call pushes total to 0.30 > 0.25 — must trip.
	err := tr.RecordModelCall(ModelCall{CostUSD: 0.10})
	if err == nil {
		t.Fatal("expected ErrRunBudgetExceeded on call that exceeds cap")
	}
	if !errors.Is(err, ErrRunBudgetExceeded) {
		t.Errorf("err = %v, want errors.Is(_, ErrRunBudgetExceeded)", err)
	}
	if !strings.Contains(err.Error(), "$0.30") {
		t.Errorf("err should carry running total $0.30, got %v", err)
	}
	if !strings.Contains(err.Error(), "$0.25") {
		t.Errorf("err should carry cap $0.25, got %v", err)
	}
}

func TestTracker_NilSafe(t *testing.T) {
	// All tracker methods must be nil-safe so persona helpers can call
	// them unconditionally without an extra context check.
	var tr *Tracker
	tr.SetBudgetUSD(1.00)
	if err := tr.RecordModelCall(ModelCall{CostUSD: 5.00}); err != nil {
		t.Errorf("nil tracker should swallow record + err return, got %v", err)
	}
	if got := tr.TotalCostUSD(); got != 0 {
		t.Errorf("nil tracker TotalCostUSD = %v, want 0", got)
	}
}

// Sentinel: ensure the cost accumulator is stored on the tracker (not
// recomputed from t.model) so a future refactor doesn't accidentally
// double-count when ModelCall is appended before the cost check.
func TestTracker_TotalCostMatchesRecorded(t *testing.T) {
	tr := NewTracker("turn-1", "marketing")
	_ = tr.RecordModelCall(ModelCall{CostUSD: 0.05})
	_ = tr.RecordModelCall(ModelCall{CostUSD: 0.025})
	if got := tr.TotalCostUSD(); got < 0.0749 || got > 0.0751 {
		t.Errorf("TotalCostUSD = %v, want ~0.075", got)
	}
}
