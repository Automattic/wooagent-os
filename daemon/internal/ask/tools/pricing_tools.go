package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/wooagent-os/wooagent-os/daemon/internal/llm/anthropic"
)

// WebSearchToolDef is the Anthropic server-managed web_search tool spec
// the Pricing chat-mode agent advertises to the model. Anthropic executes
// the search server-side; results return inline with the assistant's
// response. Wired via AskAgent.ServerTools, not the local-handler Tools
// array — see `daemon/internal/httpapi/handlers_ask.go`.
//
// MaxUses mirrors the cadence-mode Pricing persona's cap of 4: enough
// breathing room for one search to come back thin without forcing a
// no-grounding skip.
var WebSearchToolDef = anthropic.ToolDef{
	Type:    "web_search_20250305",
	Name:    "web_search",
	MaxUses: 4,
}

// --------------------------------------------------------- produce_recommendation

// ProduceRecommendationTool writes a Pricing price-change proposal to
// the board, mirroring the shape the cadence-mode `pricing-benchmark`
// skill produces. The model supplies current_price, proposed_price,
// rationale, and sources in chat (after a `web_search` sequence); this
// tool is a pure writer.
type ProduceRecommendationTool struct {
	DB *sql.DB
}

type produceRecommendationInput struct {
	ProductID     int     `json:"product_id"`
	ProductName   string  `json:"product_name"`
	ProductSKU    string  `json:"product_sku,omitempty"`
	Currency      string  `json:"currency,omitempty"`
	CurrentPrice  float64 `json:"current_price"`
	ProposedPrice float64 `json:"proposed_price"`
	Rationale     string  `json:"rationale"`
	Sources       []pricingSource `json:"sources"`
	// TargetField names which price column the operator is targeting:
	// "regular_price" (default) or "sale_price". Surfaced in the proposal
	// title for clarity.
	TargetField string `json:"target_field,omitempty"`
}

// pricingSource mirrors the shape the cadence-mode Pricing persona
// records — at least 3 entries with a retailer + URL + observed price.
type pricingSource struct {
	Retailer string  `json:"retailer"`
	URL      string  `json:"url,omitempty"`
	Title    string  `json:"title,omitempty"`
	Price    float64 `json:"price"`
}

func (h *ProduceRecommendationTool) Definition() anthropic.ToolDef {
	return anthropic.ToolDef{
		Name: "produce_recommendation",
		Description: "Create a price-change proposal on the board. Call this " +
			"after `get_product` (for current_price + currency) and a `web_search` " +
			"sequence that produced at least 3 mid-tier comparables. `sources` " +
			"must contain ≥3 entries with retailer + observed price; thinner " +
			"grounding means decline in chat, don't call this tool. `rationale` " +
			"explains the recommendation in 2-4 sentences. Returns proposal_id; " +
			"surface as [proposal #<id>] in your one-line chat receipt.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"product_id":     { "type": "integer" },
				"product_name":   { "type": "string" },
				"product_sku":    { "type": "string" },
				"currency":       { "type": "string", "description": "ISO 4217 code, e.g. USD" },
				"current_price":  { "type": "number" },
				"proposed_price": { "type": "number" },
				"rationale":      { "type": "string" },
				"target_field":   { "type": "string", "enum": ["regular_price", "sale_price"], "default": "regular_price" },
				"sources": {
					"type": "array",
					"minItems": 3,
					"items": {
						"type": "object",
						"properties": {
							"retailer": { "type": "string" },
							"url":      { "type": "string" },
							"title":    { "type": "string" },
							"price":    { "type": "number" }
						},
						"required": ["retailer", "price"]
					}
				}
			},
			"required": ["product_id", "product_name", "current_price", "proposed_price", "rationale", "sources"]
		}`),
	}
}

func (h *ProduceRecommendationTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	if h.DB == nil {
		return "", errors.New("produce_recommendation: DB not configured")
	}
	var in produceRecommendationInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if in.ProductID <= 0 {
		return "", errors.New("product_id is required (call get_product first to resolve)")
	}
	if strings.TrimSpace(in.ProductName) == "" {
		return "", errors.New("product_name is required")
	}
	if in.CurrentPrice <= 0 {
		return "", errors.New("current_price must be > 0")
	}
	if in.ProposedPrice <= 0 {
		return "", errors.New("proposed_price must be > 0")
	}
	if strings.TrimSpace(in.Rationale) == "" {
		return "", errors.New("rationale is required")
	}
	if len(in.Sources) < 3 {
		return "", fmt.Errorf("sources must contain at least 3 entries (got %d)", len(in.Sources))
	}
	for i, s := range in.Sources {
		if strings.TrimSpace(s.Retailer) == "" {
			return "", fmt.Errorf("sources[%d].retailer is empty", i)
		}
		if s.Price <= 0 {
			return "", fmt.Errorf("sources[%d].price must be > 0", i)
		}
	}
	// Match the cadence-mode 25% step cap — defense in depth so a runaway
	// model can't land a wildly out-of-range proposal.
	pct := ((in.ProposedPrice - in.CurrentPrice) / in.CurrentPrice) * 100.0
	if math.Abs(pct) > 25.0+0.01 {
		return "", fmt.Errorf("percent_change %.2f exceeds ±25%% step cap", pct)
	}

	targetField := in.TargetField
	if targetField == "" {
		targetField = "regular_price"
	}
	currency := in.Currency
	if currency == "" {
		currency = "USD"
	}
	saleSuffix := ""
	if targetField == "sale_price" {
		saleSuffix = " (sale)"
	}
	title := fmt.Sprintf("Price change · %s · %s%.2f → %s%.2f (%+.1f%%)%s",
		in.ProductName, currencySymbol(currency), in.CurrentPrice,
		currencySymbol(currency), in.ProposedPrice, pct, saleSuffix)
	description := fmt.Sprintf(
		"Drafted by Pricing agent (chat) for product #%d. %d benchmarked sources.",
		in.ProductID, len(in.Sources),
	)

	target := map[string]any{
		"product_id":     in.ProductID,
		"product_name":   in.ProductName,
		"product_sku":    in.ProductSKU,
		"currency":       currency,
		"current_price":  in.CurrentPrice,
		"proposed_price": in.ProposedPrice,
		"percent_change": math.Round(pct*10) / 10,
		"target_field":   targetField,
		"sources":        in.Sources,
	}
	dedup := fmt.Sprintf("product:%d", in.ProductID)
	issueID, err := insertOperatorAskedProposal(ctx, h.DB, "pricing", title, description,
		"product_price_change", in.Rationale, target, dedup)
	if err != nil {
		return "", err
	}
	return marshalJSON(produceOutput{
		OK:           true,
		ProposalID:   issueID,
		ProposalType: "product_price_change",
	})
}

// currencySymbol maps an ISO 4217 code to a symbol for the proposal
// title. Unknown codes pass through as-is so a non-supported currency
// still produces a readable title (e.g. "NOK 250.00" instead of breaking).
func currencySymbol(code string) string {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "USD", "":
		return "$"
	case "EUR":
		return "€"
	case "GBP":
		return "£"
	case "JPY":
		return "¥"
	case "CAD":
		return "CA$"
	case "AUD":
		return "A$"
	default:
		return code + " "
	}
}

// Compile-time tool interface assertion.
var _ anthropic.ToolHandler = (*ProduceRecommendationTool)(nil)
