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
		ParentID   int         `json:"parent_id"` // decoded for envelope symmetry; caller already has parentID
		Variations []variation `json:"variations"`
	}
	if err := callAbility(ctx, c, "wooagent-products/variations-list",
		map[string]any{"product_id": parentID}, &out); err != nil {
		return nil, fmt.Errorf("variations-list for parent %d: %w", parentID, err)
	}
	return out.Variations, nil
}

// buildVariationChildTarget composes the proposal.target payload for ONE
// variation, in the same simple-product shape buildPricingTarget produces
// for non-variable products. The dispatcher's existing
// "product_price_change" entry then writes via wooagent-products/update
// with id=<variation_id> — WC_Product_Variation supports set_regular_price
// / set_sale_price exactly like simple WC_Product does.
//
// product_id is set to the variation_id (not the parent), so the existing
// dispatcher, undo handler, and Pricing cooldown logic all work without
// further changes.
func buildVariationChildTarget(parent product, v variation, out proposalOut, currency string, anchor float64, field string, proposed float64) map[string]any {
	t := map[string]any{
		"product_id":             v.ID,
		"product_name":           fmt.Sprintf("%s — %s", parent.Name, v.AttributesLabel),
		"product_sku":            parent.SKU,
		"image_url":              parent.ImageURL,
		"image_alt":              parent.ImageAlt,
		"currency":               currency,
		"previous_price":         anchor,
		"proposed_price":         proposed,
		"regular_price":          strconv.FormatFloat(proposed, 'f', 2, 64),
		"target_field":           field,
		"regular_price_observed": canonDecimal(v.RegularPrice),
		"percent_change":         out.PercentChange,
		"direction":              out.Direction,
		"observed_median":        out.ObservedMedian,
		"observed_low":           out.ObservedLow,
		"observed_high":          out.ObservedHigh,
		"sources":                out.Sources,
	}
	if s := canonDecimal(v.SalePrice); s != "" {
		t["sale_price_observed"] = s
	}
	return t
}

// draftForVariableParent fetches the parent's variations, runs the LLM
// once against the median anchor for benchmarking, then emits a BATCH of
// product_price_change child drafts — one per variation. The existing
// BatchReview UI renders this exactly like a category-batch pricing run,
// and the existing per-issue approve/undo dispatcher writes each variation
// via wooagent-products/update on approve.
//
// Returns Drafted{Skipped:true} for any LLM-level skip (no_proposal,
// insufficient sources) or when no variation has a usable anchor.
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
	// in RegularPrice. The model doesn't need to know it's variable — it
	// benchmarks one price; we fan the returned percent out below.
	parentForLLM := parent
	parentForLLM.RegularPrice = strconv.FormatFloat(anchor, 'f', 2, 64)
	parentForLLM.SalePrice = ""

	// TODO(DSGWOO-1352 follow-up): variations-list doesn't yet surface
	// COGS per variation (the field is variation-only on Woo 10.3+ via
	// "values: [...]"). When the companion plugin's variations-list adds
	// it, anchor the floor on the median variation cost and reject the
	// run when any fanned-out child lands sub-cost. For now: no floor on
	// variable products.
	out, raw, err := draftProposal(ctx, deps.Env.AnthropicAPIKey, model, skillDescription, parentForLLM, currency, anchor, "regular_price", 0)
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
	if len(out.Sources) < 2 {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("proposal has %d sources, skill requires at least 2", len(out.Sources)),
		}, nil
	}
	if n := distinctSources(out.Sources); n < 2 {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("proposal cites %d comparable(s) but only %d distinct retailer(s); skill requires at least 2 different retailers", len(out.Sources), n),
		}, nil
	}
	if out.ProposedPrice <= 0 {
		return personas.Drafted{}, fmt.Errorf("proposed_price must be > 0")
	}
	if absFloat(out.PercentChange) > 25.0+0.01 {
		return personas.Drafted{}, fmt.Errorf("percent_change %.2f exceeds ±25%% step cap", out.PercentChange)
	}
	// Soft-skip no-change proposals — a 0% parent percent leaves every
	// child variation unchanged and gives the operator nothing to approve.
	if absFloat(out.PercentChange) < 0.05 {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("no-change: parent percent %.2f%% rounds to +0.0%%", out.PercentChange),
		}, nil
	}

	pct := out.PercentChange
	children := make([]personas.Drafted, 0, len(vs))
	for _, v := range vs {
		varAnchor, field, ok := pickVariationAnchor(v)
		if !ok {
			continue
		}
		proposed := roundCents(varAnchor * (1 + pct/100.0))
		target := buildVariationChildTarget(parent, v, out, currency, varAnchor, field, proposed)

		saleSuffix := ""
		if field == "sale_price" {
			saleSuffix = " (sale)"
		}
		title := fmt.Sprintf("Price change · %s — %s · %s%.2f → %s%.2f (%+.1f%%)%s",
			parent.Name, v.AttributesLabel,
			currencySymbol(currency), varAnchor,
			currencySymbol(currency), proposed,
			pct, saleSuffix)

		children = append(children, personas.Drafted{
			Title:           title,
			Description:     fmt.Sprintf("Drafted by Pricing agent for variation #%d (%s · %s). %d benchmarked sources.", v.ID, parent.SKU, v.AttributesLabel, len(out.Sources)),
			Priority:        "medium",
			ProposalType:    "product_price_change",
			ProposalContent: out.Rationale,
			DedupKey:        fmt.Sprintf("product:%d", v.ID),
			Target:          target,
		})
	}

	if len(children) == 0 {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("variable parent %d (%q) had variations but none with usable anchors", parent.ID, parent.Name),
		}, nil
	}

	// Pack as a batch. We reuse packAsBatch's plumbing but override the
	// title + intent — this isn't a "category seasonal parity run", it's a
	// "variable parent" batch with N variations.
	batch := packAsBatch(children, parent.Name)
	batch.BatchTitle = fmt.Sprintf("Pricing · %s · %d variation%s",
		parent.Name, len(children), plural(len(children)))
	batch.BatchIntent = "pricing_variable"
	return batch, nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
