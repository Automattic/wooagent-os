package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/wooagent-os/wooagent-os/daemon/internal/llm/anthropic"
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
)

// ListProductsTool wraps the `wooagent-products/list` MCP ability. Both
// Marketing and Pricing chat-mode toolbelts reuse this same handler —
// product lookup is identical across personas; only the produce_* tools
// differ. Keeps the chat side aligned with the cadence-mode personas'
// list path (`pickFirstPublished` in `internal/personas/marketing`).
type ListProductsTool struct {
	MCP *mcp.Client
}

type listProductsInput struct {
	Query   string `json:"query,omitempty"`
	PerPage int    `json:"per_page,omitempty"`
}

type productListEntry struct {
	ID                     int    `json:"id"`
	Name                   string `json:"name"`
	SKU                    string `json:"sku"`
	Status                 string `json:"status"`
	DescriptionLength      int    `json:"description_length"`
	ShortDescriptionLength int    `json:"short_description_length"`
}

func (h *ListProductsTool) Definition() anthropic.ToolDef {
	return anthropic.ToolDef{
		Name: "list_products",
		Description: "Find products by name, SKU, or partial match. `query` is " +
			"matched against name/SKU server-side. `per_page` defaults to 25 " +
			"and is capped at 100. Returns a list of {id, name, sku, status, " +
			"description_length}; call `get_product` for full body.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query":    { "type": "string", "description": "Name or SKU substring; empty returns recently-modified" },
				"per_page": { "type": "integer", "default": 25, "minimum": 1, "maximum": 100 }
			}
		}`),
	}
}

func (h *ListProductsTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	if h.MCP == nil {
		return "", errors.New("list_products: MCP client not configured")
	}
	var in listProductsInput
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &in); err != nil {
			return "", fmt.Errorf("invalid input: %w", err)
		}
	}
	if in.PerPage <= 0 {
		in.PerPage = 25
	}
	if in.PerPage > 100 {
		in.PerPage = 100
	}
	params := map[string]any{"per_page": in.PerPage}
	if q := strings.TrimSpace(in.Query); q != "" {
		params["search"] = q
	} else {
		params["orderby"] = "date_modified"
		params["order"] = "desc"
	}
	var data struct {
		Products []productListEntry `json:"products"`
	}
	if err := callAskAbility(ctx, h.MCP, "wooagent-products/list", params, &data); err != nil {
		return "", err
	}
	return marshalJSON(struct {
		Products []productListEntry `json:"products"`
		Count    int                `json:"count"`
	}{Products: data.Products, Count: len(data.Products)})
}

// GetProductTool wraps the `wooagent-products/get` MCP ability. Returns
// full body including current description, regular/sale price, image,
// and status — enough that the model can produce a draft without a
// second round-trip.
type GetProductTool struct {
	MCP *mcp.Client
}

type getProductInput struct {
	ID  int    `json:"id,omitempty"`
	SKU string `json:"sku,omitempty"`
}

type productDetail struct {
	ID               int     `json:"id"`
	Name             string  `json:"name"`
	SKU              string  `json:"sku"`
	Status           string  `json:"status"`
	Description      string  `json:"description"`
	ShortDescription string  `json:"short_description"`
	RegularPrice     string  `json:"regular_price,omitempty"`
	SalePrice        string  `json:"sale_price,omitempty"`
	Price            string  `json:"price,omitempty"`
	Currency         string  `json:"currency,omitempty"`
	StockStatus      string  `json:"stock_status,omitempty"`
	StockQuantity    *int    `json:"stock_quantity,omitempty"`
	Categories       []any   `json:"categories,omitempty"`
	ImageURL         string  `json:"image_url,omitempty"`
	ImageAlt         string  `json:"image_alt,omitempty"`
	Weight           string  `json:"weight,omitempty"`
	Dimensions       any     `json:"dimensions,omitempty"`
	AverageRating    float64 `json:"average_rating,omitempty"`
	RatingCount      int     `json:"rating_count,omitempty"`
}

func (h *GetProductTool) Definition() anthropic.ToolDef {
	return anthropic.ToolDef{
		Name: "get_product",
		Description: "Fetch a single product by id or SKU. Returns full body " +
			"including description, prices, image, stock, and categories. " +
			"Use this to ground a draft against the real product data before " +
			"calling any `produce_*` tool — don't draft against a target you " +
			"couldn't pull.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"id":  { "type": "integer", "description": "Numeric product id" },
				"sku": { "type": "string",  "description": "Product SKU" }
			}
		}`),
	}
}

func (h *GetProductTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	if h.MCP == nil {
		return "", errors.New("get_product: MCP client not configured")
	}
	var in getProductInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	params := map[string]any{}
	switch {
	case in.ID > 0:
		params["id"] = in.ID
	case strings.TrimSpace(in.SKU) != "":
		params["sku"] = strings.TrimSpace(in.SKU)
	default:
		return "", errors.New("get_product: provide id or sku")
	}
	var p productDetail
	if err := callAskAbility(ctx, h.MCP, "wooagent-products/get", params, &p); err != nil {
		return "", err
	}
	return marshalJSON(p)
}

// --------------------------------------------------------- produce_description_rewrite

// ProduceDescriptionRewriteTool writes a Marketing product-description
// rewrite proposal to the board, mirroring the shape the cadence-mode
// `marketing-description-rewrite` skill produces. The model supplies the
// three variants in chat; this tool is a pure writer (no LLM under the
// hood) — it just persists what was already drafted into the Issue + Run
// pair the board reads from.
type ProduceDescriptionRewriteTool struct {
	DB *sql.DB
}

type produceDescriptionRewriteInput struct {
	ProductID   int     `json:"product_id"`
	ProductName string  `json:"product_name,omitempty"`
	ProductSKU  string  `json:"product_sku,omitempty"`
	Previous    string  `json:"previous,omitempty"`
	ImageURL    string  `json:"image_url,omitempty"`
	ImageAlt    string  `json:"image_alt,omitempty"`
	Variants    []rewriteVariant `json:"variants"`
}

type rewriteVariant struct {
	Body string `json:"body"`
}

func (h *ProduceDescriptionRewriteTool) Definition() anthropic.ToolDef {
	return anthropic.ToolDef{
		Name: "produce_description_rewrite",
		Description: "Create a 3-variant product-description rewrite proposal on " +
			"the board for the operator to review. Call this AFTER `get_product` " +
			"so you have the real product_id/name/SKU + previous description. " +
			"`variants` must be exactly three bodies in your voice, 140-220 " +
			"characters each. Returns proposal_id; surface it as [proposal #<id>] " +
			"in your one-line chat receipt — do NOT paste the variants in chat.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"product_id":   { "type": "integer" },
				"product_name": { "type": "string" },
				"product_sku":  { "type": "string" },
				"previous":     { "type": "string", "description": "Current product description (so the proposal stores a before/after)" },
				"image_url":    { "type": "string" },
				"image_alt":    { "type": "string" },
				"variants": {
					"type": "array",
					"minItems": 3,
					"maxItems": 3,
					"items": {
						"type": "object",
						"properties": {
							"body": { "type": "string" }
						},
						"required": ["body"]
					}
				}
			},
			"required": ["product_id", "product_name", "variants"]
		}`),
	}
}

func (h *ProduceDescriptionRewriteTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	if h.DB == nil {
		return "", errors.New("produce_description_rewrite: DB not configured")
	}
	var in produceDescriptionRewriteInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if in.ProductID <= 0 {
		return "", errors.New("product_id is required (call get_product first to resolve)")
	}
	if strings.TrimSpace(in.ProductName) == "" {
		return "", errors.New("product_name is required")
	}
	if len(in.Variants) != 3 {
		return "", fmt.Errorf("variants must be exactly 3 entries (got %d)", len(in.Variants))
	}
	for i, v := range in.Variants {
		if strings.TrimSpace(v.Body) == "" {
			return "", fmt.Errorf("variant %d body is empty", i+1)
		}
	}
	target := map[string]any{
		"product_id":   in.ProductID,
		"product_name": in.ProductName,
		"product_sku":  in.ProductSKU,
		"previous":     in.Previous,
		"image_url":    in.ImageURL,
		"image_alt":    in.ImageAlt,
		"variants":     in.Variants,
	}
	title := fmt.Sprintf("Product description rewrite · %s", in.ProductName)
	description := fmt.Sprintf("Drafted by Marketing agent (chat) for product #%d.", in.ProductID)
	issueID, err := insertOperatorAskedProposal(ctx, h.DB, "marketing", title, description,
		"product_description_rewrite", in.Variants[0].Body, target,
		fmt.Sprintf("product:%d", in.ProductID))
	if err != nil {
		return "", err
	}
	return marshalJSON(produceOutput{
		OK:           true,
		ProposalID:   issueID,
		ProposalType: "product_description_rewrite",
	})
}

// ------------------------------------------------------------ produce_social_post

// ProduceSocialPostTool writes a social-post draft proposal — one body,
// one or more target products. Lighter-weight than the description
// rewrite (no variants; no current-copy diff).
type ProduceSocialPostTool struct {
	DB *sql.DB
}

type produceSocialPostInput struct {
	TargetProducts []productRef `json:"target_products"`
	Body           string       `json:"body"`
}

type productRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	SKU  string `json:"sku,omitempty"`
}

func (h *ProduceSocialPostTool) Definition() anthropic.ToolDef {
	return anthropic.ToolDef{
		Name: "produce_social_post",
		Description: "Create a social-post draft proposal on the board. " +
			"`target_products` is one or more products the post is about " +
			"(resolve via get_product first). `body` is your draft in the " +
			"brand voice. Returns proposal_id; surface as [proposal #<id>] " +
			"in your one-line chat receipt — do NOT paste body in chat.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"target_products": {
					"type": "array",
					"minItems": 1,
					"items": {
						"type": "object",
						"properties": {
							"id":   { "type": "integer" },
							"name": { "type": "string" },
							"sku":  { "type": "string" }
						},
						"required": ["id", "name"]
					}
				},
				"body": { "type": "string", "description": "The post copy in brand voice" }
			},
			"required": ["target_products", "body"]
		}`),
	}
}

func (h *ProduceSocialPostTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	return executeProduceMarketingCopy(ctx, h.DB, raw, "social_post", "Social post")
}

// ------------------------------------------------------------ produce_launch_copy

// ProduceLaunchCopyTool writes a launch-copy draft. Same shape as
// social_post (single body, target products); separate tool so the
// proposal carries the right kind for the board's filter / display.
type ProduceLaunchCopyTool struct {
	DB *sql.DB
}

func (h *ProduceLaunchCopyTool) Definition() anthropic.ToolDef {
	return anthropic.ToolDef{
		Name: "produce_launch_copy",
		Description: "Create a launch-copy draft proposal on the board. " +
			"`target_products` is one or more products the launch is about " +
			"(resolve via get_product first). `body` is your draft in the " +
			"brand voice. Returns proposal_id; surface as [proposal #<id>] " +
			"in your one-line chat receipt — do NOT paste body in chat.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"target_products": {
					"type": "array",
					"minItems": 1,
					"items": {
						"type": "object",
						"properties": {
							"id":   { "type": "integer" },
							"name": { "type": "string" },
							"sku":  { "type": "string" }
						},
						"required": ["id", "name"]
					}
				},
				"body": { "type": "string", "description": "Launch copy in brand voice" }
			},
			"required": ["target_products", "body"]
		}`),
	}
}

func (h *ProduceLaunchCopyTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	return executeProduceMarketingCopy(ctx, h.DB, raw, "launch_copy", "Launch copy")
}

// executeProduceMarketingCopy is the shared write path for social_post
// and launch_copy — both have identical input shape (target_products + body)
// and differ only in the `kind` stamped onto the proposal record. The
// dedup_key folds in the kind so two different post types on the same
// product can coexist in the in-review queue.
func executeProduceMarketingCopy(ctx context.Context, db *sql.DB, raw json.RawMessage, kind, label string) (string, error) {
	if db == nil {
		return "", fmt.Errorf("produce_%s: DB not configured", kind)
	}
	var in produceSocialPostInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if len(in.TargetProducts) == 0 {
		return "", errors.New("target_products is required (at least one product)")
	}
	if strings.TrimSpace(in.Body) == "" {
		return "", errors.New("body is required")
	}
	for i, p := range in.TargetProducts {
		if p.ID <= 0 {
			return "", fmt.Errorf("target_products[%d].id is required", i)
		}
		if strings.TrimSpace(p.Name) == "" {
			return "", fmt.Errorf("target_products[%d].name is required", i)
		}
	}
	names := make([]string, 0, len(in.TargetProducts))
	for _, p := range in.TargetProducts {
		names = append(names, p.Name)
	}
	target := map[string]any{
		"target_products": in.TargetProducts,
		"body":            in.Body,
		"kind":            kind,
	}
	title := fmt.Sprintf("%s · %s", label, strings.Join(names, ", "))
	description := fmt.Sprintf("Drafted by Marketing agent (chat). Featuring %d product(s).",
		len(in.TargetProducts))
	// Dedup key includes the kind so a social-post + launch-copy for the
	// same product set can both be in-review simultaneously.
	productIDs := make([]string, 0, len(in.TargetProducts))
	for _, p := range in.TargetProducts {
		productIDs = append(productIDs, fmt.Sprintf("%d", p.ID))
	}
	dedupKey := fmt.Sprintf("%s:products:%s", kind, strings.Join(productIDs, ","))
	issueID, err := insertOperatorAskedProposal(ctx, db, "marketing", title, description,
		kind, in.Body, target, dedupKey)
	if err != nil {
		return "", err
	}
	return marshalJSON(produceOutput{
		OK:           true,
		ProposalID:   issueID,
		ProposalType: kind,
	})
}

// ----------------------------------------------------------- shared write helpers

// produceOutput is the response shape every produce_* tool returns to
// the model — a small JSON blob with the new proposal id. The model is
// instructed (via prompt) to surface this as `[proposal #<id>]` in its
// one-line receipt so the UI renders a chip.
type produceOutput struct {
	OK           bool   `json:"ok"`
	ProposalID   string `json:"proposal_id"`
	ProposalType string `json:"proposal_type"`
}

// insertOperatorAskedProposal writes the Issue + Run pair that backs an
// operator-asked produce_* call. The Run row exists so the proposal is
// attributable to a real run (board cards link out to runs; telemetry
// counts include chat-mode drafts). The Run is synthetic — no scheduler
// involvement, no Anthropic call charged here (the chat-mode LLM call
// is already accounted for at the /v1/ask level).
//
// The two writes are not wrapped in a transaction: if the Run insert
// fails after a successful Issue insert, the Issue still appears on the
// board attributed to "marketing" (or whichever persona) without a Run
// link. That's a recoverable inconsistency, and SQLite doesn't give us
// cheap transactions across this path in v1. If the Issue insert fails
// (e.g. dedup_key collision), no Run is written.
func insertOperatorAskedProposal(
	ctx context.Context,
	db *sql.DB,
	persona, title, description, proposalType, proposalContent string,
	target map[string]any,
	dedupKey string,
) (string, error) {
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)

	targetJSON, err := json.Marshal(target)
	if err != nil {
		return "", fmt.Errorf("marshal target: %w", err)
	}

	var dedup sql.NullString
	if dedupKey != "" {
		dedup = sql.NullString{String: dedupKey, Valid: true}
	}

	_, err = db.ExecContext(ctx,
		`INSERT INTO issues(id, title, description, persona, status, priority,
		                    created_at, updated_at, proposal_type, proposal_content,
		                    proposal_target, dedup_key)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, title, description, persona, "in_review", "medium", now, now,
		proposalType, proposalContent, string(targetJSON), dedup,
	)
	if err != nil {
		return "", fmt.Errorf("insert issue: %w", err)
	}

	runID := uuid.NewString()
	_, err = db.ExecContext(ctx,
		`INSERT INTO runs(id, persona, trigger, status, attempt,
		                  scheduled_at, claimed_at, completed_at, latency_ms,
		                  issue_id, created_at)
		 VALUES(?, ?, ?, ?, 1, ?, ?, ?, 0, ?, ?)`,
		runID, persona, "operator-asked", "succeeded",
		now, now, now, id, now,
	)
	if err != nil {
		// Soft-log: the issue is still on the board, just without a Run
		// row. Surface the error so the caller can decide whether to
		// retry; the model will see this as a partial success.
		return id, fmt.Errorf("insert run (issue %s landed): %w", id, err)
	}
	return id, nil
}

// callAskAbility is the ask-side wrapper around mcp.CallTool. Mirrors the
// persona-side `callAbility` helper but without the per-turn telemetry
// recorder — chat-mode tool calls are logged at the /v1/ask handler
// level (handler_ask.go logs iteration count + token usage), not
// per-tool. Keeping the wrapper local avoids leaking an internal
// persona-package symbol.
func callAskAbility(ctx context.Context, c *mcp.Client, ability string, params map[string]any, out any) error {
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
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   string          `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(res.Content[0].Text), &env); err != nil {
		return fmt.Errorf("decode envelope (%s): %w", ability, err)
	}
	if !env.Success {
		return fmt.Errorf("ability %s failed: %s", ability, env.Error)
	}
	if out != nil {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("decode data (%s): %w", ability, err)
		}
	}
	return nil
}

// Compile-time tool interface assertions.
var (
	_ anthropic.ToolHandler = (*ListProductsTool)(nil)
	_ anthropic.ToolHandler = (*GetProductTool)(nil)
	_ anthropic.ToolHandler = (*ProduceDescriptionRewriteTool)(nil)
	_ anthropic.ToolHandler = (*ProduceSocialPostTool)(nil)
	_ anthropic.ToolHandler = (*ProduceLaunchCopyTool)(nil)
)
