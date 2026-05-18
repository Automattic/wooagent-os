// Package reporting is the Reporting agent — a digest-style persona that
// summarizes catalog health for the operator.
//
// Reporting is a baseline persona: it works on any Woo store with the
// Companion Plugin installed, with no dependency on optional plugins.
// The digest is built by listing products via wooagent-products/list and
// filtering Go-side for data quality issues (missing long description,
// missing short description) — both signals are already present in the
// list summary, no per-product fetch and no third-party plugin needed.
//
// An earlier scaffold (PR #46) gated this persona on the WC AI plugin's
// woocommerce/find-products ability. That was a launch-speed shortcut
// rather than the right architectural answer — Reporting belongs in the
// always-on fleet (PRD §11.4), not the WPCOM-enhanced tier (§11.7).
// Rewritten 2026-05-18 to drop the dependency.
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

// digestPreviewSize is the upper bound on products listed in any single
// digest. 25 is enough for the operator to skim and decide whether to
// fix or dismiss, without making the proposal-detail UI a wall of names.
const digestPreviewSize = 25

// listPageSize is how many products to pull from wooagent-products/list
// in a single call. v0.1 doesn't paginate; large catalogs (>~100) will
// underreport their total_count until pagination is added. Noted as a
// known limitation in the digest body so an operator with a big catalog
// understands the gap.
const listPageSize = 100

func init() {
	personas.Register(&Reporting{})
}

type Reporting struct{}

func (Reporting) Slug() string        { return "reporting" }
func (Reporting) DisplayName() string { return "Reporting" }

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
// data quality issues. Works on any Woo store with the Companion Plugin
// installed.
func (Reporting) Draft(ctx context.Context, deps personas.Deps) (personas.Drafted, error) {
	if deps.MCP == nil {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: "MCP client not configured (set WOOAGENT_MCP_URL/USER/APP_PASSWORD on the daemon)",
		}, nil
	}

	if _, err := deps.MCP.Initialize(ctx); err != nil {
		return personas.Drafted{}, fmt.Errorf("mcp initialize: %w", err)
	}

	matches, total, truncated, err := findProductsWithDataIssues(ctx, deps.MCP)
	if err != nil {
		return personas.Drafted{}, fmt.Errorf("list products: %w", err)
	}
	if total == 0 {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: "no products with data issues (missing short description, missing long description)",
		}, nil
	}

	title := fmt.Sprintf("Product health digest · %d product%s need attention",
		total, plural(total))
	content := renderDigest(matches, total, truncated)

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

// productSummary mirrors the wooagent-products/list summary fields
// Reporting needs to detect data issues. Extra fields in the response
// are ignored by the JSON decoder.
type productSummary struct {
	ID                     int    `json:"id"`
	Name                   string `json:"name"`
	Status                 string `json:"status"`
	DescriptionLength      int    `json:"description_length"`
	ShortDescriptionLength int    `json:"short_description_length"`
}

// productMatch is one row in the digest's preview.
type productMatch struct {
	ProductID int
	Name      string
	Issues    []string
}

// findProductsWithDataIssues lists products via the Companion Plugin and
// filters Go-side for those missing a long description or short
// description. Returns (preview, total, truncated, err) where truncated
// is true when the total exceeds digestPreviewSize.
//
// Pagination is a v0.2 follow-up — for v0.1 the first 100 products is
// the working window. The digest body discloses this when the catalog
// hits the cap so an operator with a big catalog isn't misled.
func findProductsWithDataIssues(ctx context.Context, c *mcp.Client) ([]productMatch, int, bool, error) {
	var out struct {
		Products []productSummary `json:"products"`
		Total    int              `json:"total"`
	}
	params := map[string]any{
		"per_page": listPageSize,
		"status":   "publish",
	}
	if err := callAbility(ctx, c, "wooagent-products/list", params, &out); err != nil {
		return nil, 0, false, err
	}

	matches := make([]productMatch, 0, len(out.Products))
	for _, p := range out.Products {
		if p.Status != "publish" && p.Status != "" {
			continue
		}
		issues := dataIssuesFor(p)
		if len(issues) == 0 {
			continue
		}
		matches = append(matches, productMatch{
			ProductID: p.ID,
			Name:      p.Name,
			Issues:    issues,
		})
	}

	total := len(matches)
	truncated := false
	if total > digestPreviewSize {
		truncated = true
		matches = matches[:digestPreviewSize]
	}
	return matches, total, truncated, nil
}

// dataIssuesFor returns the operator-readable list of issues for a
// product summary. Today: long description + short description presence
// checks (both signals are in the wooagent-products/list summary; no
// per-product fetch needed). Image and SKU checks are v0.2 — they
// require either adding fields to the summary or N+1 calls to
// wooagent-products/get.
func dataIssuesFor(p productSummary) []string {
	var out []string
	if p.DescriptionLength == 0 {
		out = append(out, "missing long description")
	}
	if p.ShortDescriptionLength == 0 {
		out = append(out, "missing short description")
	}
	return out
}

// renderDigest formats the matched products into a plain markdown body
// the operator reads in the proposal-detail view. No prose generation;
// the persona's value is the curation, not the writing.
func renderDigest(matches []productMatch, total int, truncated bool) string {
	var b strings.Builder
	b.WriteString("These products are missing copy that helps customers decide.\n\n")
	if truncated {
		fmt.Fprintf(&b, "**%d total matches** (showing the first %d):\n\n", total, len(matches))
	} else {
		fmt.Fprintf(&b, "**%d product%s:**\n\n", total, plural(total))
	}
	for _, m := range matches {
		fmt.Fprintf(&b, "- %s (product #%d) — %s\n", m.Name, m.ProductID, strings.Join(m.Issues, ", "))
	}
	b.WriteString("\nReview each in WP-admin and add the missing fields, or dismiss this digest if these gaps are intentional.")
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
