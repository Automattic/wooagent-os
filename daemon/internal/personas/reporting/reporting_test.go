package reporting

import (
	"context"
	"testing"

	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
)

func TestReporting_SlugAndDisplayName(t *testing.T) {
	r := Reporting{}
	if r.Slug() != "reporting" {
		t.Errorf("Slug() = %q, want \"reporting\"", r.Slug())
	}
	if r.DisplayName() != "Reporting" {
		t.Errorf("DisplayName() = %q, want \"Reporting\"", r.DisplayName())
	}
}

// Cooldown is zero-value because Reporting doesn't dedup by target.
// Confirm that's preserved — a non-empty TargetKey would cause
// RecentlyTouchedTargets to be called (and the persona doesn't call it),
// but the contract is that the policy is meaningfully empty.
func TestReporting_Cooldown_IsZeroValue(t *testing.T) {
	c := Reporting{}.Cooldown()
	if c.TargetKey != "" {
		t.Errorf("Cooldown.TargetKey = %q, want empty (Reporting doesn't dedup by target)", c.TargetKey)
	}
	if c.Approved != 0 || c.Dismissed != 0 {
		t.Errorf("Cooldown should be zero-value; got Approved=%v Dismissed=%v", c.Approved, c.Dismissed)
	}
}

// Draft is a dormant stub until a real reporting skill lands.
// Confirm it returns Skipped:true with a non-empty SkipReason under all
// conditions — even with a nil MCP client (deps is zero-value here).
func TestDraft_ReturnsSkipped(t *testing.T) {
	got, err := (Reporting{}).Draft(context.Background(), personas.Deps{})
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	if !got.Skipped {
		t.Errorf("expected Skipped=true, got %+v", got)
	}
	if got.SkipReason == "" {
		t.Errorf("SkipReason should be non-empty")
	}
}
