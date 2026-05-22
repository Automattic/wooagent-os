package pricing

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
)

// medianAnchorProducts computes the median of pickAnchorPrice across the
// given products. Returns (median, true) when at least one product has a
// positive anchor; (0, false) when none do (caller emits NoProposal). The
// children of a grouped parent are full WC_Product objects with their own
// regular/sale prices, so we anchor on each child via pickAnchorPrice
// (which is the same helper used for simple products).
func medianAnchorProducts(ps []product) (float64, bool) {
	anchors := make([]float64, 0, len(ps))
	for _, p := range ps {
		a, _, ok := pickAnchorPrice(p)
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

// buildGroupedChildTarget composes the proposal.target payload for ONE
// child product of a grouped parent. Each child is a full WC_Product with
// its own SKU, image, and name, so the target carries the CHILD's own
// identity — the batch header (BatchTitle) is what surfaces the parent's
// "Logo Collection" framing in the UI.
//
// product_id is set to the child_id (not the parent), so the existing
// "product_price_change" dispatcher entry, undo handler, and Pricing
// per-target cooldown all work without further changes.
func buildGroupedChildTarget(parent, child product, out proposalOut, currency string, anchor float64, field string, proposed float64) map[string]any {
	t := map[string]any{
		"product_id":             child.ID,
		"product_name":           child.Name,
		"product_sku":            child.SKU,
		"image_url":              child.ImageURL,
		"image_alt":              child.ImageAlt,
		"currency":               currency,
		"previous_price":         anchor,
		"proposed_price":         proposed,
		"regular_price":          strconv.FormatFloat(proposed, 'f', 2, 64),
		"target_field":           field,
		"regular_price_observed": canonDecimal(child.RegularPrice),
		"percent_change":         out.PercentChange,
		"direction":              out.Direction,
		"observed_median":        out.ObservedMedian,
		"observed_low":           out.ObservedLow,
		"observed_high":          out.ObservedHigh,
		"sources":                out.Sources,
	}
	if s := canonDecimal(child.SalePrice); s != "" {
		t["sale_price_observed"] = s
	}
	return t
}

// draftForGroupedParent fetches the parent's child products, runs the LLM
// once against the median anchor for benchmarking, then emits a BATCH of
// product_price_change child drafts — one per priced child. Mirrors
// draftForVariableParent's shape (single LLM call + fan-out) so the
// existing BatchReview UI renders it identically to a variable batch.
//
// Returns Drafted{Skipped:true} for any LLM-level skip (no_proposal,
// insufficient sources) or when no child has a usable anchor.
func draftForGroupedParent(
	ctx context.Context,
	deps personas.Deps,
	parent product,
	skillDescription, model, currency string,
) (personas.Drafted, error) {
	if len(parent.GroupedProducts) == 0 {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("grouped parent %d (%q) has no grouped_products children", parent.ID, parent.Name),
		}, nil
	}

	// Fetch each child. One MCP call per child — that's the same shape
	// as a normal Pricing run's wooagent-products/get, and grouped
	// catalogs are typically small (3-6 children).
	children := make([]product, 0, len(parent.GroupedProducts))
	for _, childID := range parent.GroupedProducts {
		c, err := getProduct(ctx, deps.MCP, childID)
		if err != nil {
			// One bad child shouldn't kill the whole grouped run.
			continue
		}
		children = append(children, c)
	}
	if len(children) == 0 {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("grouped parent %d (%q): all %d child fetches failed", parent.ID, parent.Name, len(parent.GroupedProducts)),
		}, nil
	}

	anchor, ok := medianAnchorProducts(children)
	if !ok {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("grouped parent %d (%q) has no priced children", parent.ID, parent.Name),
		}, nil
	}

	// Synthesize a parent struct for the LLM prompt with the median
	// anchor in RegularPrice. The model doesn't need to know it's
	// grouped — it benchmarks one price; we fan the returned percent
	// out below across the children.
	parentForLLM := parent
	parentForLLM.RegularPrice = strconv.FormatFloat(anchor, 'f', 2, 64)
	parentForLLM.SalePrice = ""

	// No cost floor in the LLM prompt for grouped: children have their
	// own individual COGS, and a single synthetic floor (max or median)
	// would either force unnecessary no_proposals (if too high) or fail
	// to prevent sub-cost fan-outs (if too low). We enforce per-child
	// sub-cost rejection below — skipping offenders rather than killing
	// the whole batch when a child's COGS is unusually close to its
	// anchor.
	out, raw, err := draftProposal(ctx, deps.Env.AnthropicAPIKey, model, skillDescription, parentForLLM, currency, anchor, "regular_price", 0)
	if err != nil {
		return personas.Drafted{}, fmt.Errorf("draft grouped proposal: %w (raw=%s)", err, truncate(raw, 400))
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
	if len(out.Sources) < 2 {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("proposal has %d sources, skill requires at least 2", len(out.Sources)),
		}, nil
	}
	if out.ProposedPrice <= 0 {
		return personas.Drafted{}, fmt.Errorf("proposed_price must be > 0")
	}
	if absFloat(out.PercentChange) > 25.0+0.01 {
		return personas.Drafted{}, fmt.Errorf("percent_change %.2f exceeds ±25%% step cap", out.PercentChange)
	}

	pct := out.PercentChange
	drafts := make([]personas.Drafted, 0, len(children))
	for _, c := range children {
		childAnchor, field, ok := pickAnchorPrice(c)
		if !ok {
			continue
		}
		proposed := roundCents(childAnchor * (1 + pct/100.0))
		// Sub-cost floor: when this child carries Woo 10.3+ COGS, drop
		// it from the batch rather than emit a price below cost. Other
		// children with healthy margins still ship. Logged via the
		// remaining-children check below when the whole batch empties.
		if childCost := productCost(c); childCost > 0 && proposed < childCost {
			continue
		}
		target := buildGroupedChildTarget(parent, c, out, currency, childAnchor, field, proposed)

		saleSuffix := ""
		if field == "sale_price" {
			saleSuffix = " (sale)"
		}
		title := fmt.Sprintf("Price change · %s · %s%.2f → %s%.2f (%+.1f%%)%s",
			c.Name,
			currencySymbol(currency), childAnchor,
			currencySymbol(currency), proposed,
			pct, saleSuffix)

		drafts = append(drafts, personas.Drafted{
			Title:           title,
			Description:     fmt.Sprintf("Drafted by Pricing agent for grouped child #%d (%s · %s). %d benchmarked sources.", c.ID, c.SKU, parent.Name, len(out.Sources)),
			Priority:        "medium",
			ProposalType:    "product_price_change",
			ProposalContent: out.Rationale,
			DedupKey:        fmt.Sprintf("product:%d", c.ID),
			Target:          target,
		})
	}

	if len(drafts) == 0 {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("grouped parent %d (%q) had children but none with usable anchors", parent.ID, parent.Name),
		}, nil
	}

	// Pack as a batch, overriding title + intent — this isn't a
	// "category seasonal parity run", it's a "grouped parent" batch
	// with N children.
	batch := packAsBatch(drafts, parent.Name)
	batch.BatchTitle = fmt.Sprintf("Pricing · %s · %d product%s",
		parent.Name, len(drafts), plural(len(drafts)))
	batch.BatchIntent = "pricing_grouped"
	return batch, nil
}
