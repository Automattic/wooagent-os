// Package pricing is the Pricing-agent implementation.
//
// Side-effect-registers itself with personas.Register on import. Both the
// daemon-startup goroutine and the cmd/persona-pricing CLI debug binary
// drive this package.
//
// Behavior mirrors the original cmd/persona-pricing harness:
//
//	product (MCP) → grounded benchmark (Claude w/ web_search) → structured
//	proposal (product_price_change)
//
// Differences from the binary it replaces:
//
//   - No HTTP self-loopback. The daemon-bootstrap path persists via
//     personas.RunAndPersist. The CLI debug path can do the same against
//     the daemon's SQLite (see cmd/persona-pricing/main.go).
//   - When ANTHROPIC_API_KEY is not set, Draft returns Skipped=true with a
//     human-readable reason. The daemon doesn't fail; the persona is
//     simply benched until the operator exports the key.
//   - When grounding is thin (skill returns no_proposal=true) or no
//     priceable simple products exist, Skipped=true. Refusing to propose
//     is a feature, not a failure.
package pricing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/llm"
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
	"github.com/wooagent-os/wooagent-os/daemon/internal/telemetry"
)

const (
	defaultAnthropicModel = "claude-haiku-4-5-20251001"
	anthropicAPIURL       = "https://api.anthropic.com/v1/messages"
	anthropicVersion      = "2023-06-01"
	skillName             = "pricing.benchmark"
)

func init() {
	personas.Register(&Pricing{})
}

type Pricing struct{}

func (Pricing) Slug() string        { return "pricing" }
func (Pricing) DisplayName() string { return "Pricing agent" }
func (Pricing) Addable() bool       { return false }

// Cooldown: product-centric (proposal_target.product_id). 7d after an
// approve so the new price has time to settle; 30d after a dismiss so we
// don't oscillate against the operator's "no".
func (Pricing) Cooldown() personas.CooldownPolicy {
	return personas.CooldownPolicy{
		TargetKey: "product_id",
		Approved:  7 * 24 * time.Hour,
		Dismissed: 30 * 24 * time.Hour,
	}
}

// maxDraftAttempts caps how many products one Pricing run will try.
// Pricing's web_search is the most expensive LLM call in the system
// (~20-30s per attempt), so 3 is a deliberate ceiling on a stuck run.
// See personas.Persona docstring for the canonical pattern.
const maxDraftAttempts = 3

func (Pricing) Draft(ctx context.Context, deps personas.Deps) (personas.Drafted, error) {
	if deps.MCP == nil {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: "MCP client not configured (set WOOAGENT_MCP_URL/USER/APP_PASSWORD on the daemon)",
		}, nil
	}
	if strings.TrimSpace(deps.Env.AnthropicAPIKey) == "" {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: "ANTHROPIC_API_KEY not set; pricing requires Claude with web_search for grounded comps",
		}, nil
	}
	skill, ok := deps.Skills[skillName]
	if !ok {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("skill %q not found in skills registry", skillName),
		}, nil
	}

	currency := deps.Env.DefaultCurrency
	if currency == "" {
		currency = "USD"
	}

	model := deps.Env.AnthropicModel
	if model == "" {
		model = defaultAnthropicModel
	}

	// MCP handshake — idempotent; safe to call even if a parent already did.
	if _, err := deps.MCP.Initialize(ctx); err != nil {
		return personas.Drafted{}, fmt.Errorf("mcp initialize: %w", err)
	}

	// Debug override: always draft on the operator-supplied product,
	// bypassing both cooldown and within-run iteration.
	if deps.Env.ProductIDOverride != 0 {
		return draftForProduct(ctx, deps, deps.Env.ProductIDOverride, skill.Description, model, currency)
	}

	// Persistent cooldown set (see Pricing.Cooldown).
	pr := Pricing{}
	skip, err := personas.RecentlyTouchedTargets(ctx, deps.Store, pr.Slug(), pr.Cooldown())
	if err != nil {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("look up recently-touched products: %v", err),
		}, nil
	}

	// Batch pre-check: do we have a category with >= batchThreshold
	// eligible products? If yes, run the expensive web_search loop on
	// the bucket, capped at maxBatchSize. Cost: ~1 Claude+web_search
	// call per product (~20-30s each), so the cap is what keeps a giant
	// catalog from running 11+ LLM calls in a single bootstrap tick.
	// Leftovers (bucket size - maxBatchSize) get picked up on the next
	// Pricing run once cooldown rotates.
	const batchThreshold = 5
	const maxBatchSize = 8
	eligible, eligErr := listEligibleProducts(ctx, deps.MCP, skip)
	if eligErr != nil {
		// Pre-check failed; fall through to single-product path
		// (don't bail — single-product mode still works when the
		// catalog list is unreachable for a transient reason).
		_ = eligErr
	} else if bucket := findLargestEligibleBucket(eligible, batchThreshold); bucket != nil {
		var drafts []personas.Drafted
		for _, prod := range bucket.Products {
			if len(drafts) >= maxBatchSize {
				break
			}
			d, err := draftForProduct(ctx, deps, prod.ID, skill.Description, model, currency)
			if err != nil {
				// Per-product error: log via continue and move on;
				// one bad product shouldn't kill the batch.
				continue
			}
			if d.Skipped {
				continue
			}
			drafts = append(drafts, d)
		}
		if len(drafts) >= batchThreshold {
			return packAsBatch(drafts, bucket.Category), nil
		}
		// Batch path didn't yield enough successful drafts (too many
		// LLM-level skips). Fall through to single-product mode.
	}

	// Within-run iteration: if the LLM yields no_proposal (or any other
	// LLM-level skip) for a product, add it to the run-local skip set and
	// try the next eligible one. Up to maxDraftAttempts.
	return personas.IterateDraft(
		maxDraftAttempts,
		"product",
		skip,
		func(s map[int]struct{}) (int, error) { return pickFirstProduct(ctx, deps.MCP, s) },
		func(id int) (personas.Drafted, error) {
			return draftForProduct(ctx, deps, id, skill.Description, model, currency)
		},
	)
}

// pickAnchorPrice chooses which price field the Pricing benchmark anchors
// to. Sale price wins when set and parseable to a positive decimal —
// that's what the customer pays right now, so it's the right reference
// for "what should this product cost." Regular price is the fallback.
// Returns (value, field, true) on success; (0, "", false) when neither
// regular nor sale yields a usable positive decimal — in that case the
// caller must skip the product.
//
// Note: a product with sale_price set but no regular_price is malformed
// (Woo's UI doesn't let you do this); we treat it as "skip" rather than
// silently anchoring to sale_price, because the dispatcher will need
// regular_price downstream for back-compat reasons.
func pickAnchorPrice(p product) (float64, string, bool) {
	regular, _ := strconv.ParseFloat(strings.TrimSpace(p.RegularPrice), 64)
	if regular <= 0 {
		return 0, "", false
	}
	sale, _ := strconv.ParseFloat(strings.TrimSpace(p.SalePrice), 64)
	if sale > 0 {
		return sale, "sale_price", true
	}
	return regular, "regular_price", true
}

// draftForProduct does the per-product Pricing work: fetch + parse the
// current price, call the LLM-with-web_search skill, validate the
// proposal, assemble Drafted. Returns Drafted{Skipped:true} for any
// LLM-level skip (no_proposal, insufficient sources, no usable
// regular_price) — the outer Draft loop treats that as "try the next
// product" rather than ending the run.
func draftForProduct(
	ctx context.Context,
	deps personas.Deps,
	productID int,
	skillDescription, model, currency string,
) (personas.Drafted, error) {
	p, err := getProduct(ctx, deps.MCP, productID)
	if err != nil {
		return personas.Drafted{}, fmt.Errorf("get product %d: %w", productID, err)
	}
	currentPrice, targetField, ok := pickAnchorPrice(p)
	if !ok {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("product %d (%q) type=%q has no usable regular_price (raw regular=%q sale=%q)", p.ID, p.Name, p.Type, p.RegularPrice, p.SalePrice),
		}, nil
	}

	out, raw, err := draftProposal(ctx, deps.Env.AnthropicAPIKey, model, skillDescription, p, currency, currentPrice, targetField)
	if err != nil {
		return personas.Drafted{}, fmt.Errorf("draft proposal: %w (raw=%s)", err, truncate(raw, 400))
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
		out.PreviousPrice = currentPrice
	}
	if absFloat(out.PercentChange) > 25.0+0.01 {
		return personas.Drafted{}, fmt.Errorf("percent_change %.2f exceeds ±25%% step cap", out.PercentChange)
	}

	regularPriceStr := strconv.FormatFloat(out.ProposedPrice, 'f', 2, 64)
	title := fmt.Sprintf("Price change · %s · %s%.2f → %s%.2f (%+.1f%%)",
		p.Name, currencySymbol(currency), out.PreviousPrice,
		currencySymbol(currency), out.ProposedPrice, out.PercentChange)

	return personas.Drafted{
		Title: title,
		Description: fmt.Sprintf(
			"Drafted by Pricing agent for product #%d (%s). %d benchmarked sources.",
			p.ID, p.SKU, len(out.Sources),
		),
		Priority:        "medium",
		ProposalType:    "product_price_change",
		ProposalContent: out.Rationale,
		// Belt over the existing product_id Cooldown.
		DedupKey: fmt.Sprintf("product:%d", p.ID),
		Target: map[string]any{
			"product_id":      p.ID,
			"product_name":    p.Name,
			"product_sku":     p.SKU,
			"image_url":       p.ImageURL,
			"image_alt":       p.ImageAlt,
			"currency":        currency,
			"previous_price":  out.PreviousPrice,
			"proposed_price":  out.ProposedPrice,
			"regular_price":   regularPriceStr,
			"percent_change":  out.PercentChange,
			"direction":       out.Direction,
			"observed_median": out.ObservedMedian,
			"observed_low":    out.ObservedLow,
			"observed_high":   out.ObservedHigh,
			"sources":         out.Sources,
		},
	}, nil
}

// ---------------------------------------------------------------- MCP read

type abilityEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error,omitempty"`
}

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

type productSummary struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	SKU          string `json:"sku"`
	Status       string `json:"status"`
	Type         string `json:"type"`
	RegularPrice string `json:"regular_price"`
	TotalSales   int    `json:"total_sales"`
}

// listProductSummaries fetches the first page of products from MCP as
// lightweight summaries (no descriptions, no categories). Cheap call —
// no LLM, no web_search. v0.1 doesn't paginate; large catalogs (>~100
// products) may need a follow-up.
//
// orderby+order are passed through to the Companion Plugin (v0.2+ accepts
// total_sales / date_modified / date / title / menu_order). On stores
// still running the v0.1 plugin the orderby arg will be rejected by the
// ability's additionalProperties:false schema — that surfaces as an
// explicit error rather than a silent mis-order.
func listProductSummaries(ctx context.Context, c *mcp.Client, orderby, order string) ([]productSummary, error) {
	args := map[string]any{"per_page": 100}
	if orderby != "" {
		args["orderby"] = orderby
	}
	if order != "" {
		args["order"] = order
	}
	var listOut struct {
		Products []productSummary `json:"products"`
		Total    int              `json:"total"`
	}
	if err := callAbility(ctx, c, "wooagent-products/list", args, &listOut); err != nil {
		return nil, err
	}
	return listOut.Products, nil
}

// pickFirstProduct returns the first simple, priced, published product
// whose ID is not in skip. Surfaces slow movers first (orderby=total_sales
// asc) so Pricing benchmarks the catalog's revenue-soft tail before its
// best sellers — that's where a re-priced run typically moves the needle.
func pickFirstProduct(ctx context.Context, c *mcp.Client, skip map[int]struct{}) (int, error) {
	summaries, err := listProductSummaries(ctx, c, "total_sales", "asc")
	if err != nil {
		return 0, err
	}
	if len(summaries) == 0 {
		return 0, fmt.Errorf("no products in store")
	}
	skippedVariable, skippedNoPrice, skippedCooldown := 0, 0, 0
	for _, p := range summaries {
		if p.Status != "publish" && p.Status != "" {
			continue
		}
		if p.Type == "variable" {
			skippedVariable++
			continue
		}
		if strings.TrimSpace(p.RegularPrice) == "" {
			skippedNoPrice++
			continue
		}
		if _, inCooldown := skip[p.ID]; inCooldown {
			skippedCooldown++
			continue
		}
		return p.ID, nil
	}
	return 0, fmt.Errorf(
		"no priceable simple products in first %d (skipped %d variable, %d without regular_price, %d in cooldown); "+
			"approved proposals cool down for 7d, dismissed for 30d; "+
			"set a regular_price on a simple product in wp-admin or pass PERSONA_PRODUCT_ID=<id>",
		len(summaries), skippedVariable, skippedNoPrice, skippedCooldown,
	)
}

// listEligibleProducts returns the catalog products that are eligible for
// pricing (simple, published, with a regular_price) and are not in the
// cooldown skip set. Single cheap MCP call — no LLM, no web_search.
// Decodes directly into the full `product` struct so Categories ride
// along for bucket grouping (the WooCommerce REST list response includes
// categories per product). v0.1 doesn't paginate; large catalogs (>~100
// products) may need a follow-up.
func listEligibleProducts(ctx context.Context, c *mcp.Client, skip map[int]struct{}) ([]product, error) {
	var listOut struct {
		Products []product `json:"products"`
		Total    int       `json:"total"`
	}
	if err := callAbility(ctx, c, "wooagent-products/list",
		map[string]any{"per_page": 100}, &listOut); err != nil {
		return nil, err
	}
	out := make([]product, 0, len(listOut.Products))
	for _, p := range listOut.Products {
		if p.Status != "publish" && p.Status != "" {
			continue
		}
		if p.Type == "variable" {
			continue
		}
		if strings.TrimSpace(p.RegularPrice) == "" {
			continue
		}
		if _, blocked := skip[p.ID]; blocked {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

type product struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	SKU          string `json:"sku"`
	Status       string `json:"status"`
	Type         string `json:"type"`
	Description  string `json:"description"`
	ShortDesc    string `json:"short_description"`
	Permalink    string `json:"permalink"`
	RegularPrice string `json:"regular_price"`
	SalePrice    string `json:"sale_price"`
	ImageURL     string `json:"image_url"`
	ImageAlt     string `json:"image_alt"`
	Categories   []struct {
		Name string `json:"name"`
	} `json:"categories"`
}

func getProduct(ctx context.Context, c *mcp.Client, id int) (product, error) {
	var p product
	if err := callAbility(ctx, c, "wooagent-products/get",
		map[string]any{"id": id}, &p); err != nil {
		return p, err
	}
	if p.ID == 0 {
		p.ID = id
	}
	return p, nil
}

func categoryString(p product) string {
	if len(p.Categories) == 0 {
		return ""
	}
	names := make([]string, 0, len(p.Categories))
	for _, c := range p.Categories {
		if c.Name != "" {
			names = append(names, c.Name)
		}
	}
	return strings.Join(names, ", ")
}

// categoryOf returns the product's primary category name (the first
// non-empty entry in p.Categories), or "" if the product is uncategorized.
// Used by findLargestEligibleBucket to group products for batch pricing.
func categoryOf(p product) string {
	for _, c := range p.Categories {
		if c.Name != "" {
			return c.Name
		}
	}
	return ""
}

// Bucket is a set of eligible products grouped by category, used by
// Pricing.Draft to decide whether to emit a batch.
type Bucket struct {
	Category string
	Products []product
}

// findLargestEligibleBucket buckets the given products by primary category
// and returns the largest bucket whose size is >= threshold. Uncategorized
// products (no category) never form a batch — they fall through to the
// single-product path. Ties are broken alphabetically by category name so
// the choice is deterministic across runs.
func findLargestEligibleBucket(products []product, threshold int) *Bucket {
	if threshold < 1 {
		threshold = 1
	}
	byCat := map[string][]product{}
	for _, p := range products {
		cat := categoryOf(p)
		if cat == "" {
			continue
		}
		byCat[cat] = append(byCat[cat], p)
	}
	cats := make([]string, 0, len(byCat))
	for c := range byCat {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	var best *Bucket
	for _, c := range cats {
		b := byCat[c]
		if len(b) < threshold {
			continue
		}
		if best == nil || len(b) > len(best.Products) {
			best = &Bucket{Category: c, Products: b}
		}
	}
	return best
}

// packAsBatch takes N successful Drafted results from the batch-path loop
// and packs them into a single batch-shaped Drafted (one primary + N-1
// siblings + BatchTitle + BatchIntent). Caller guarantees len(drafts) >= 1
// and all drafts share the given category.
func packAsBatch(drafts []personas.Drafted, category string) personas.Drafted {
	primary := drafts[0]
	primary.BatchSiblings = drafts[1:]
	primary.BatchTitle = fmt.Sprintf(
		"Pricing · %s seasonal parity run (%d products)",
		category, len(drafts),
	)
	primary.BatchIntent = "pricing_bulk"
	return primary
}

// ---------------------------------------------------------------- Anthropic

type proposalSource struct {
	URL               string  `json:"url"`
	Retailer          string  `json:"retailer,omitempty"`
	ComparableProduct string  `json:"comparable_product"`
	ObservedPrice     float64 `json:"observed_price"`
	Currency          string  `json:"currency,omitempty"`
	Note              string  `json:"note,omitempty"`
}

type proposalOut struct {
	NoProposal       bool             `json:"no_proposal"`
	ReasonNoProposal string           `json:"reason_no_proposal,omitempty"`
	PreviousPrice    float64          `json:"previous_price,omitempty"`
	ProposedPrice    float64          `json:"proposed_price,omitempty"`
	PercentChange    float64          `json:"percent_change,omitempty"`
	Direction        string           `json:"direction,omitempty"`
	ObservedMedian   float64          `json:"observed_median,omitempty"`
	ObservedLow      float64          `json:"observed_low,omitempty"`
	ObservedHigh     float64          `json:"observed_high,omitempty"`
	Rationale        string           `json:"rationale,omitempty"`
	Sources          []proposalSource `json:"sources,omitempty"`
}

// proposalOutAlias prevents UnmarshalJSON from recursing into itself when
// we re-decode the normalized payload back into the typed struct.
type proposalOutAlias proposalOut

// UnmarshalJSON tolerates the most common drifts we've observed from the
// model — synonyms for direction/percent_change and rationale as an array
// of strings instead of a single string. We don't accept arbitrary aliases;
// the synonyms here are ones the prompt explicitly bans but the model
// occasionally produces anyway. Keeping the lenient layer narrow keeps the
// integrity contract intact.
func (p *proposalOut) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Synonym: recommendation → direction
	if _, ok := raw["direction"]; !ok {
		if v, ok := raw["recommendation"]; ok {
			raw["direction"] = v
		}
	}
	// Synonym: price_change_percent → percent_change
	if _, ok := raw["percent_change"]; !ok {
		if v, ok := raw["price_change_percent"]; ok {
			raw["percent_change"] = v
		}
	}
	// rationale: accept either string or []string. When array, join with
	// double newline so paragraph structure is preserved.
	if v, ok := raw["rationale"]; ok {
		var asString string
		if err := json.Unmarshal(v, &asString); err != nil {
			var asArray []string
			if err2 := json.Unmarshal(v, &asArray); err2 == nil {
				joined := strings.Join(asArray, "\n\n")
				if b, mErr := json.Marshal(joined); mErr == nil {
					raw["rationale"] = b
				}
			}
		}
	}

	normalized, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	var alias proposalOutAlias
	if err := json.Unmarshal(normalized, &alias); err != nil {
		return err
	}
	*p = proposalOut(alias)
	return nil
}

type anthropicTool struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	MaxUses int    `json:"max_uses,omitempty"`
}

type anthropicMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicReq struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    string          `json:"system"`
	Messages  []anthropicMsg  `json:"messages"`
	Tools     []anthropicTool `json:"tools,omitempty"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type anthropicResp struct {
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason,omitempty"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage,omitempty"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

const userPromptTemplate = `Product to analyze:
- id: %d
- name: %s
- sku: %s
- category: %s
- current regular_price: %.2f %s
- current sale_price:    %s
- anchor (the price customers pay today): %s = %.2f %s
- description: %s

Anchor your benchmark and the previous_price field of your output to the **anchor** value above — that is the price customers see right now. When the anchor is sale_price, your proposal updates the active sale; when the anchor is regular_price, the product is not on sale.

Search the preferred retailers from the skill — start with site:-scoped queries against J.Crew, Madewell, Aritzia, Everlane, Quince, COS for apparel; Parachute, Anthropologie, West Elm, Crate & Barrel, Coyuchi for home goods. Pick the 4–6 retailers most likely to carry this product and run site:<retailer>.com <noun phrase> queries. Match on category, material, and tier — not just keywords.

Required: at least 3 qualifying comparables before proposing. When you decline, reason_no_proposal must name what you searched, what came back, and why it doesn't qualify. When you propose, name the retailer in each sources[] entry.

OUTPUT FORMAT — EXACT field names. Do not rename, do not nest differently, do not add fields not listed below. rationale is a single STRING (use \n for paragraph breaks if needed), NOT an array. Example of a propose-yes response:

{
  "no_proposal": false,
  "previous_price": 48.00,
  "proposed_price": 56.00,
  "percent_change": 16.67,
  "direction": "increase",
  "observed_median": 65.00,
  "observed_low": 59.95,
  "observed_high": 72.00,
  "rationale": "Mid-tier indigo throw pillows at Crate & Barrel ($59.95), Anthropologie ($68.00), and Parachute ($72.00) cluster around $65. Current $48 trails the band by ~26%%. Recommend a +16.67%% step toward the median, capped at the skill's ±25%% per-step rule.",
  "sources": [
    {"url": "https://www.crateandbarrel.com/...", "retailer": "Crate & Barrel", "comparable_product": "Indigo Block-Print Pillow", "observed_price": 59.95, "currency": "USD"},
    {"url": "https://www.anthropologie.com/...", "retailer": "Anthropologie", "comparable_product": "Hand-Dyed Indigo Pillow", "observed_price": 68.00, "currency": "USD"},
    {"url": "https://www.parachutehome.com/...", "retailer": "Parachute", "comparable_product": "Linen Throw Pillow Cover", "observed_price": 72.00, "currency": "USD"}
  ]
}

Example of a decline:

{
  "no_proposal": true,
  "reason_no_proposal": "Searched site:jcrew.com sock subscription (0 results), site:bombas.com sock subscription (returned 6-pair bundles, not one-shot pricing), site:stance.com sock subscription (subscription pricing only). No one-shot retail comparables exist in the preferred mid-tier band for subscription products."
}

Output ONE JSON object. No prose outside the JSON. No markdown fences. All prices in %s, 2 decimals.`

func draftProposal(
	ctx context.Context,
	apiKey, model, skillSystem string,
	p product,
	currency string,
	currentPrice float64,
	anchorField string,
) (proposalOut, string, error) {
	system := skillSystem + "\n\nWhen you respond, output ONLY a JSON object that matches the schema in skills/pricing-benchmark/v1.yaml output. No prose outside the JSON. No markdown code fences."

	salePriceDisplay := "—"
	if s := strings.TrimSpace(p.SalePrice); s != "" {
		if sv, err := strconv.ParseFloat(s, 64); err == nil && sv > 0 {
			salePriceDisplay = fmt.Sprintf("%.2f %s", sv, currency)
		}
	}
	regularPrice, _ := strconv.ParseFloat(strings.TrimSpace(p.RegularPrice), 64)

	user := fmt.Sprintf(
		userPromptTemplate,
		p.ID, p.Name, p.SKU, categoryString(p),
		regularPrice, currency,
		salePriceDisplay,
		anchorField, currentPrice, currency,
		strings.TrimSpace(p.Description),
		currency,
	)

	body, _ := json.Marshal(anthropicReq{
		Model: model,
		// max_tokens counts EVERY output token: the model's thinking text
		// between web_search calls, the tool_use blocks themselves, and the
		// final JSON. A 1024 cap proved too tight in practice — the model
		// often spent its budget on inter-search reasoning ("I'll try X,
		// then Y…") and got cut off before producing the structured JSON,
		// which the parser rejected with "no JSON object found in model
		// output". 2048 is the new floor: still half of the original 4096,
		// but with enough headroom for 3-4 search rounds + the final JSON.
		MaxTokens: 2048,
		System:    system,
		Messages:  []anthropicMsg{{Role: "user", Content: user}},
		// MaxUses caps how many web_search calls the model can issue. The
		// skill requires ≥3 sources in the response. We give the model 4
		// uses so one search can fail or come back thin without forcing a
		// no_proposal skip — gives the planner some breathing room.
		Tools: []anthropicTool{{Type: "web_search_20250305", Name: "web_search", MaxUses: 4}},
	})

	cctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(cctx, "POST", anthropicAPIURL, bytes.NewReader(body))
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("content-type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return proposalOut{}, "", fmt.Errorf("anthropic http: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		return proposalOut{}, string(raw), fmt.Errorf("anthropic http %d", res.StatusCode)
	}
	var parsed anthropicResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return proposalOut{}, string(raw), fmt.Errorf("anthropic decode: %w", err)
	}
	if parsed.Error.Type != "" {
		return proposalOut{}, string(raw), fmt.Errorf("anthropic error: %s · %s", parsed.Error.Type, parsed.Error.Message)
	}

	if t := telemetry.TrackerFromContext(ctx); t != nil {
		if err := t.RecordModelCall(telemetry.ModelCall{
			Provider:     "anthropic",
			Model:        model,
			InputTokens:  parsed.Usage.InputTokens,
			OutputTokens: parsed.Usage.OutputTokens,
			CostUSD:      llm.CostUSD("anthropic", model, parsed.Usage.InputTokens, parsed.Usage.OutputTokens),
		}); err != nil {
			return proposalOut{}, "", err
		}
	}

	var textOut strings.Builder
	for _, block := range parsed.Content {
		if block.Type == "text" {
			textOut.WriteString(block.Text)
		}
	}
	finalText := strings.TrimSpace(textOut.String())
	jsonBlob := extractJSONObject(finalText)
	if jsonBlob == "" {
		return proposalOut{}, finalText, fmt.Errorf("no JSON object found in model output")
	}
	var out proposalOut
	if err := json.Unmarshal([]byte(jsonBlob), &out); err != nil {
		return proposalOut{}, jsonBlob, fmt.Errorf("decode proposal JSON: %w", err)
	}
	return out, jsonBlob, nil
}

// extractJSONObject pulls the first balanced top-level JSON object out of
// s. Tolerates accidental prose before/after the object and stripped or
// kept markdown fences. Returns "" when no balanced object is found.
func extractJSONObject(s string) string {
	start := strings.Index(s, "{")
	if start == -1 {
		return ""
	}
	depth, inStr, esc := 0, false, false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			if c == '\\' {
				esc = true
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
			continue
		}
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func currencySymbol(c string) string {
	switch strings.ToUpper(c) {
	case "USD", "CAD", "AUD":
		return "$"
	case "GBP":
		return "£"
	case "EUR":
		return "€"
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
