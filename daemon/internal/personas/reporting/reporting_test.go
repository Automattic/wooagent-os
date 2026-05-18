package reporting

import (
	"context"
	"strings"
	"testing"

	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
)

// stubAbilities is the test double for personas.Abilities — answers true
// for any name in have, false for everything else.
type stubAbilities struct {
	have map[string]bool
}

func (s stubAbilities) Has(name string) bool { return s.have[name] }

func TestReporting_SlugAndDisplayName(t *testing.T) {
	r := Reporting{}
	if r.Slug() != "reporting" {
		t.Errorf("Slug() = %q, want \"reporting\"", r.Slug())
	}
	if r.DisplayName() != "Reporting agent" {
		t.Errorf("DisplayName() = %q, want \"Reporting agent\"", r.DisplayName())
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

func TestReporting_Draft_NoMCP_SkippedWithReason(t *testing.T) {
	res, err := Reporting{}.Draft(context.Background(), personas.Deps{})
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if !res.Skipped {
		t.Errorf("expected Skipped when MCP is nil; got %+v", res)
	}
	if !strings.Contains(res.SkipReason, "MCP") {
		t.Errorf("SkipReason should mention MCP; got %q", res.SkipReason)
	}
}

// Reporting requires woocommerce/find-products (a WC AI plugin ability).
// Without it, the persona MUST skip rather than fall back — there's no
// baseline Companion Plugin ability that exposes the data_issues filter.
func TestReporting_Draft_NoAbilities_SkippedWithReason(t *testing.T) {
	deps := personas.Deps{
		MCP:       &mcp.Client{},
		Abilities: personas.NilAbilities{},
	}
	res, err := Reporting{}.Draft(context.Background(), deps)
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if !res.Skipped {
		t.Errorf("expected Skipped when Abilities.Has returns false for everything")
	}
	if !strings.Contains(res.SkipReason, "Woo AI") {
		t.Errorf("SkipReason should call out the Woo AI plugin by name (operator-facing); got %q", res.SkipReason)
	}
	if !strings.Contains(res.SkipReason, requiredAbility) {
		t.Errorf("SkipReason should name the missing ability for diagnosability; got %q", res.SkipReason)
	}
}

// When deps.Abilities is left nil (rather than NilAbilities), Draft must
// still skip gracefully — never panic on the nil-check path. Guards
// against a future caller that forgets to wire the field.
func TestReporting_Draft_NilAbilitiesField_SkippedNotPanic(t *testing.T) {
	deps := personas.Deps{
		MCP:       &mcp.Client{},
		Abilities: nil,
	}
	res, err := Reporting{}.Draft(context.Background(), deps)
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if !res.Skipped {
		t.Errorf("expected Skipped when Abilities field is nil")
	}
}

func TestRenderDigest_TotalAndPreviewMatch(t *testing.T) {
	matches := []productMatch{
		{ProductID: 1, Name: "Cashmere Scarf"},
		{ProductID: 2, Name: "Wool Cardigan"},
	}
	body := renderDigest(matches, 2)
	if !strings.Contains(body, "**2 products:**") {
		t.Errorf("body should say \"2 products\" when total equals preview length; got:\n%s", body)
	}
	if !strings.Contains(body, "Cashmere Scarf") || !strings.Contains(body, "product #1") {
		t.Errorf("body should include product name + ID; got:\n%s", body)
	}
}

func TestRenderDigest_TotalExceedsPreview_NotesTruncation(t *testing.T) {
	matches := []productMatch{
		{ProductID: 1, Name: "Cashmere Scarf"},
		{ProductID: 2, Name: "Wool Cardigan"},
	}
	body := renderDigest(matches, 47)
	if !strings.Contains(body, "**47 total matches**") {
		t.Errorf("body should call out the truncated total; got:\n%s", body)
	}
	if !strings.Contains(body, "showing the first 2") {
		t.Errorf("body should mention how many are previewed; got:\n%s", body)
	}
}

func TestPlural(t *testing.T) {
	if plural(1) != "" {
		t.Errorf("plural(1) should be empty; got %q", plural(1))
	}
	for _, n := range []int{0, 2, 5, 100} {
		if plural(n) != "s" {
			t.Errorf("plural(%d) should be \"s\"; got %q", n, plural(n))
		}
	}
}
