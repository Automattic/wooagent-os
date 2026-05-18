// Package reporting is the Reporting agent — a digest-style persona that
// summarizes catalog health for the operator.
//
// Reporting is the first persona that requires an optional plugin: it calls
// woocommerce/find-products from the WC AI plugin to surface products with
// data quality issues (missing image, short description, long description,
// SKU). When the plugin isn't installed on the connected store, Reporting
// skips with a clear "Requires Woo AI plugin" reason — the daemon does
// not register a fallback path on the baseline Companion Plugin abilities,
// because the data_issues filter doesn't exist there.
//
// The persona is registered at init() but is intentionally NOT seeded in
// the agents table at daemon init — it stays dormant until an operator
// opts in via the Add Agent UI, which inserts the agents row with
// enabled=1 and the cadence the operator chose.
//
// No LLM calls. The proposal is a structured digest the operator reads
// directly; no prose generation, no model API key required.
package reporting

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
	"github.com/wooagent-os/wooagent-os/daemon/internal/telemetry"
)

// requiredAbility is the WC AI plugin ability Reporting depends on.
// Surfaced in skip reasons so operators see the missing piece by name.
const requiredAbility = "woocommerce/find-products"

// digestPreviewSize is the upper bound on products listed in any single
// digest. The find-products ability accepts up to 500 in `preview_size`;
// 25 is enough for the operator to skim and decide whether to fix or
// dismiss, without making the proposal-detail UI a wall of names.
const digestPreviewSize = 25

func init() {
	personas.Register(&Reporting{})
}

type Reporting struct{}

func (Reporting) Slug() string        { return "reporting" }
func (Reporting) DisplayName() string { return "Reporting agent" }

// Cooldown is zero-value because Reporting doesn't dedup by target. A
// digest summarizes *the catalog state right now*; the cadence-based
// gating on agents.cadence_seconds (set when the operator opts in via
// Add Agent) is the only "don't propose too often" lever this persona
// uses. Draft must not call personas.RecentlyTouchedTargets — the empty
// TargetKey would return an error.
func (Reporting) Cooldown() personas.CooldownPolicy {
	return personas.CooldownPolicy{}
}

// Draft surfaces a product_health_digest proposal listing products with
// data quality issues. Skips cleanly when the WC AI plugin isn't
// installed on the connected store.
func (Reporting) Draft(ctx context.Context, deps personas.Deps) (personas.Drafted, error) {
	if deps.MCP == nil {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: "MCP client not configured (set WOOAGENT_MCP_URL/USER/APP_PASSWORD on the daemon)",
		}, nil
	}
	if deps.Abilities == nil || !deps.Abilities.Has(requiredAbility) {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("requires the Woo AI plugin (ability %q not installed on the connected store)", requiredAbility),
		}, nil
	}

	if _, err := deps.MCP.Initialize(ctx); err != nil {
		return personas.Drafted{}, fmt.Errorf("mcp initialize: %w", err)
	}

	matches, total, err := findProductsWithDataIssues(ctx, deps.MCP)
	if err != nil {
		return personas.Drafted{}, fmt.Errorf("find-products call: %w", err)
	}
	if total == 0 {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: "no products with data issues (missing image / short description / long description / SKU)",
		}, nil
	}

	title := fmt.Sprintf("Product health digest · %d product%s need attention",
		total, plural(total))
	content := renderDigest(matches, total)

	return personas.Drafted{
		Title:           title,
		Description:     fmt.Sprintf("Drafted by Reporting persona. %d total matches; previewing up to %d.", total, digestPreviewSize),
		Priority:        "low",
		ProposalType:    "product_health_digest",
		ProposalContent: content,
		Target: map[string]any{
			"digest_kind":  "data_issues",
			"total_count":  total,
			"preview_size": len(matches),
		},
	}, nil
}

// productMatch is one row of the find-products preview.
type productMatch struct {
	ProductID int    `json:"product_id"`
	Name      string `json:"name"`
}

// findProductsWithDataIssues calls woocommerce/find-products with the
// full set of data-issue flags. Returns the preview slice and the total
// match count. A nil error with empty matches + total=0 means the call
// succeeded but the catalog is clean.
func findProductsWithDataIssues(ctx context.Context, c *mcp.Client) ([]productMatch, int, error) {
	params := map[string]any{
		"filters": map[string]any{
			"data_issues": []string{
				"missing_image",
				"missing_short_description",
				"missing_long_description",
				"missing_sku",
			},
		},
		"preview_size": digestPreviewSize,
	}
	var out struct {
		Success      bool           `json:"success"`
		Preview      []productMatch `json:"preview"`
		TotalMatched int            `json:"total_matched"`
		ReturnedCount int           `json:"returned_count"`
		Code         string         `json:"code"`
		Message      string         `json:"message"`
	}
	if err := callAbility(ctx, c, "woocommerce/find-products", params, &out); err != nil {
		return nil, 0, err
	}
	if !out.Success {
		return nil, 0, fmt.Errorf("find-products returned success=false: %s (%s)", out.Message, out.Code)
	}
	total := out.TotalMatched
	if total == 0 && out.ReturnedCount > 0 {
		total = out.ReturnedCount
	}
	return out.Preview, total, nil
}

// renderDigest formats the matched products into a plain markdown body
// the operator reads in the proposal-detail view. No prose generation;
// the persona's value is the curation, not the writing.
func renderDigest(matches []productMatch, total int) string {
	var b strings.Builder
	b.WriteString("The Woo AI plugin found products with one or more of these gaps: missing image, missing short description, missing long description, missing SKU.\n\n")
	if total > len(matches) {
		fmt.Fprintf(&b, "**%d total matches** (showing the first %d):\n\n", total, len(matches))
	} else {
		fmt.Fprintf(&b, "**%d products:**\n\n", total)
	}
	for _, m := range matches {
		fmt.Fprintf(&b, "- %s (product #%d)\n", m.Name, m.ProductID)
	}
	b.WriteString("\nReview each in WP-admin and add the missing fields, or dismiss this digest if these are intentional.")
	return b.String()
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// ----------------------------------------------------------------- MCP

// abilityEnvelope mirrors the JSON the MCP adapter wraps every ability
// result in — { "success": bool, "data": ..., "error": "..." }. Same
// shape Marketing/Pricing/Sales Support unwrap.
type abilityEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error,omitempty"`
}

// callAbility wraps mcp.CallTool with TurnEvent skill-call recording so
// the GEPA pipeline gets per-call telemetry. The inner func is what
// matches the marketing/pricing/sales-support pattern; lifting it into
// personas/ is a future-cleanup candidate (intentional duplication for
// now to keep blast radius bounded).
func callAbility(ctx context.Context, c *mcp.Client, ability string, params map[string]any, out any) error {
	start := time.Now()
	err := callAbilityInner(ctx, c, ability, params, out)
	telemetry.RecordSkillCallFromError(ctx, ability, time.Since(start), err)
	return err
}

func callAbilityInner(ctx context.Context, c *mcp.Client, ability string, params map[string]any, out any) error {
	res, err := c.CallTool(ctx, "mcp-adapter-execute-ability", map[string]any{
		"ability_name": ability,
		"parameters":   params,
	})
	if err != nil {
		return fmt.Errorf("mcp call %s: %w", ability, err)
	}
	if len(res.Content) == 0 {
		return fmt.Errorf("mcp %s: empty content", ability)
	}
	var env abilityEnvelope
	if err := json.Unmarshal([]byte(res.Content[0].Text), &env); err != nil {
		return fmt.Errorf("decode envelope (%s): %w body=%s", ability, err, res.Content[0].Text)
	}
	if !env.Success {
		return fmt.Errorf("ability %s failed: %s", ability, env.Error)
	}
	if out != nil {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("decode data (%s): %w body=%s", ability, err, string(env.Data))
		}
	}
	return nil
}
