package reporting

import (
	"context"
	"strings"
	"testing"

	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
)

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

// The data-issue detection is pure-function and exercised here without
// MCP. dataIssuesFor is the heart of the persona: it decides what counts
// as "needs attention." Test that the two checks fire independently and
// that a fully-filled product produces an empty issue list.
func TestDataIssuesFor_BothMissing(t *testing.T) {
	issues := dataIssuesFor(productSummary{ID: 1, Name: "X", Status: "publish"})
	if len(issues) != 2 {
		t.Errorf("expected 2 issues for zero-length descriptions; got %v", issues)
	}
}

func TestDataIssuesFor_LongOnly(t *testing.T) {
	issues := dataIssuesFor(productSummary{ID: 1, Name: "X", Status: "publish", ShortDescriptionLength: 50})
	if len(issues) != 1 || !strings.Contains(issues[0], "long description") {
		t.Errorf("expected just \"missing long description\"; got %v", issues)
	}
}

func TestDataIssuesFor_ShortOnly(t *testing.T) {
	issues := dataIssuesFor(productSummary{ID: 1, Name: "X", Status: "publish", DescriptionLength: 200})
	if len(issues) != 1 || !strings.Contains(issues[0], "short description") {
		t.Errorf("expected just \"missing short description\"; got %v", issues)
	}
}

func TestDataIssuesFor_AllPresent_NoIssues(t *testing.T) {
	issues := dataIssuesFor(productSummary{
		ID: 1, Name: "X", Status: "publish",
		DescriptionLength: 200, ShortDescriptionLength: 50,
	})
	if len(issues) != 0 {
		t.Errorf("expected zero issues for a fully-filled product; got %v", issues)
	}
}

func TestRenderDigest_NotTruncated(t *testing.T) {
	matches := []productMatch{
		{ProductID: 1, Name: "Cashmere Scarf", Issues: []string{"missing long description"}},
		{ProductID: 2, Name: "Wool Cardigan", Issues: []string{"missing short description"}},
	}
	body := renderDigest(matches, 2, false)
	if !strings.Contains(body, "**2 products:**") {
		t.Errorf("body should say \"2 products\" when total equals preview length; got:\n%s", body)
	}
	if !strings.Contains(body, "Cashmere Scarf") || !strings.Contains(body, "product #1") {
		t.Errorf("body should include product name + ID; got:\n%s", body)
	}
	if !strings.Contains(body, "missing long description") {
		t.Errorf("body should include the issue text; got:\n%s", body)
	}
}

func TestRenderDigest_Truncated(t *testing.T) {
	matches := []productMatch{
		{ProductID: 1, Name: "Cashmere Scarf", Issues: []string{"missing long description"}},
		{ProductID: 2, Name: "Wool Cardigan", Issues: []string{"missing short description"}},
	}
	body := renderDigest(matches, 47, true)
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
