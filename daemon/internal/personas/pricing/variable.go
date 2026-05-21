package pricing

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
)

// variation is the wooagent-products/variations-list row shape. Mirrors the
// Companion Plugin's output_schema for that ability.
type variation struct {
	ID              int    `json:"id"`
	AttributesLabel string `json:"attributes_label"`
	RegularPrice    string `json:"regular_price"`
	SalePrice       string `json:"sale_price"`
	StockStatus     string `json:"stock_status"`
	MenuOrder       int    `json:"menu_order"`
}

// pickVariationAnchor returns the price the Pricing benchmark anchors
// against for this variation: sale_price if set and parseable to > 0,
// else regular_price. Returns (value, field, true) on success; the third
// return is false when neither field yields a positive decimal — caller
// skips that variation.
func pickVariationAnchor(v variation) (float64, string, bool) {
	regular, _ := strconv.ParseFloat(strings.TrimSpace(v.RegularPrice), 64)
	sale, _ := strconv.ParseFloat(strings.TrimSpace(v.SalePrice), 64)
	if sale > 0 {
		return sale, "sale_price", true
	}
	if regular > 0 {
		return regular, "regular_price", true
	}
	return 0, "", false
}

// medianAnchor computes the median of pickVariationAnchor across the
// given variations. Returns (median, true) when at least one variation
// has a positive anchor; (0, false) when none do (caller emits NoProposal).
func medianAnchor(vs []variation) (float64, bool) {
	anchors := make([]float64, 0, len(vs))
	for _, v := range vs {
		a, _, ok := pickVariationAnchor(v)
		if !ok {
			continue
		}
		anchors = append(anchors, a)
	}
	if len(anchors) == 0 {
		return 0, false
	}
	sort.Float64s(anchors)
	n := len(anchors)
	if n%2 == 1 {
		return anchors[n/2], true
	}
	return (anchors[n/2-1] + anchors[n/2]) / 2, true
}

// roundCents rounds a value to 2 decimal places.
func roundCents(v float64) float64 {
	return math.Round(v*100) / 100
}

// listVariations fetches all enabled, priced variations of a parent via
// wooagent-products/variations-list.
func listVariations(ctx context.Context, c *mcp.Client, parentID int) ([]variation, error) {
	var out struct {
		ParentID   int         `json:"parent_id"`
		Variations []variation `json:"variations"`
	}
	if err := callAbility(ctx, c, "wooagent-products/variations-list",
		map[string]any{"product_id": parentID}, &out); err != nil {
		return nil, fmt.Errorf("variations-list for parent %d: %w", parentID, err)
	}
	return out.Variations, nil
}

// buildPricingTargetVariable composes the proposal.target payload for a
// variable-product price change. percent_change is parent-level; each
// variations[] entry carries its own previous/proposed_price + target_field.
// The decimal string keyed "regular_price" inside each variation is what
// the dispatcher writes — keyed for parity with the simple shape's target
// payload regardless of which Woo field it lands in.
func buildPricingTargetVariable(p product, vs []variation, out proposalOut, currency string) map[string]any {
	pct := out.PercentChange
	variationsOut := make([]map[string]any, 0, len(vs))

	var prevMin, prevMax, propMin, propMax float64
	first := true
	for _, v := range vs {
		anchor, field, ok := pickVariationAnchor(v)
		if !ok {
			continue
		}
		proposed := roundCents(anchor * (1 + pct/100.0))
		variationsOut = append(variationsOut, map[string]any{
			"variation_id":     v.ID,
			"attributes_label": v.AttributesLabel,
			"target_field":     field,
			"previous_price":   anchor,
			"proposed_price":   proposed,
			"regular_price":    strconv.FormatFloat(proposed, 'f', 2, 64),
			"stock_status":     v.StockStatus,
		})
		if first {
			prevMin, prevMax = anchor, anchor
			propMin, propMax = proposed, proposed
			first = false
			continue
		}
		if anchor < prevMin {
			prevMin = anchor
		}
		if anchor > prevMax {
			prevMax = anchor
		}
		if proposed < propMin {
			propMin = proposed
		}
		if proposed > propMax {
			propMax = proposed
		}
	}

	return map[string]any{
		"product_id":         p.ID,
		"product_name":       p.Name,
		"product_sku":        p.SKU,
		"image_url":          p.ImageURL,
		"image_alt":          p.ImageAlt,
		"currency":           currency,
		"percent_change":     out.PercentChange,
		"direction":          out.Direction,
		"observed_median":    out.ObservedMedian,
		"observed_low":       out.ObservedLow,
		"observed_high":      out.ObservedHigh,
		"sources":            out.Sources,
		"variations":         variationsOut,
		"variation_count":    len(variationsOut),
		"previous_price_min": prevMin,
		"previous_price_max": prevMax,
		"proposed_price_min": propMin,
		"proposed_price_max": propMax,
	}
}

// draftForVariableParent fetches variations, asks the LLM to benchmark
// against the median anchor (one parent-level call), then fans the
// returned percent out to every variation. Returns Drafted{Skipped:true}
// for any LLM-level skip (no_proposal, insufficient sources, etc.) —
// the outer Draft loop treats that as "try the next product."
func draftForVariableParent(
	ctx context.Context,
	deps personas.Deps,
	parent product,
	skillDescription, model, currency string,
) (personas.Drafted, error) {
	vs, err := listVariations(ctx, deps.MCP, parent.ID)
	if err != nil {
		return personas.Drafted{}, fmt.Errorf("list variations for %d: %w", parent.ID, err)
	}
	anchor, ok := medianAnchor(vs)
	if !ok {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("variable parent %d (%q) has no priced variations", parent.ID, parent.Name),
		}, nil
	}

	// Synthesize a parent struct for the LLM prompt with the median anchor
	// in RegularPrice. This reuses draftProposal's prompt verbatim — the
	// model doesn't need to know it's variable; it benchmarks one price
	// and we fan the percent out below.
	parentForLLM := parent
	parentForLLM.RegularPrice = strconv.FormatFloat(anchor, 'f', 2, 64)
	parentForLLM.SalePrice = ""

	out, raw, err := draftProposal(ctx, deps.Env.AnthropicAPIKey, model, skillDescription, parentForLLM, currency, anchor, "regular_price")
	if err != nil {
		return personas.Drafted{}, fmt.Errorf("draft variable proposal: %w (raw=%s)", err, truncate(raw, 400))
	}
	if out.NoProposal {
		reason := strings.TrimSpace(out.ReasonNoProposal)
		if reason == "" {
			reason = "model returned no_proposal=true with no reason"
		}
		return personas.Drafted{
			Skipped:    true,
			SkipReason: "no_proposal: " + reason,
		}, nil
	}
	if len(out.Sources) < 3 {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("proposal has %d sources, skill requires at least 3", len(out.Sources)),
		}, nil
	}
	if out.ProposedPrice <= 0 {
		return personas.Drafted{}, fmt.Errorf("proposed_price must be > 0")
	}
	if out.PreviousPrice == 0 {
		out.PreviousPrice = anchor
	}
	if absFloat(out.PercentChange) > 25.0+0.01 {
		return personas.Drafted{}, fmt.Errorf("percent_change %.2f exceeds ±25%% step cap", out.PercentChange)
	}

	target := buildPricingTargetVariable(parent, vs, out, currency)
	// Count the variations that actually appear in the target (after
	// pickVariationAnchor filtering) rather than the raw variations list.
	vCount := target["variation_count"].(int)
	if vCount == 0 {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("variable parent %d (%q) had variations but none with usable anchors", parent.ID, parent.Name),
		}, nil
	}

	title := fmt.Sprintf("Price change · %s · %+.1f%% across %d variation%s",
		parent.Name, out.PercentChange, vCount, plural(vCount))

	return personas.Drafted{
		Title:           title,
		Description:     fmt.Sprintf("Drafted by Pricing agent for variable product #%d (%s). %d benchmarked sources. Applies +%.1f%% to %d variations.", parent.ID, parent.SKU, len(out.Sources), out.PercentChange, vCount),
		Priority:        "medium",
		ProposalType:    "product_price_change_variable",
		ProposalContent: out.Rationale,
		DedupKey:        fmt.Sprintf("product:%d", parent.ID),
		Target:          target,
	}, nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
