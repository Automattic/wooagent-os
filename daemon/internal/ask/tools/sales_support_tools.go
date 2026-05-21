package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/wooagent-os/wooagent-os/daemon/internal/llm/anthropic"
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
)

// ListOrdersTool wraps the `wooagent-orders/list` MCP ability. The
// chat-mode Sales Support agent uses it to find orders by status,
// customer email, or recency. Mirrors the cadence-mode persona's order
// list call (`daemon/internal/personas/sales-support/sales_support.go`).
type ListOrdersTool struct {
	MCP *mcp.Client
}

type listOrdersInput struct {
	Status        string `json:"status,omitempty"`
	CustomerEmail string `json:"customer_email,omitempty"`
	Since         string `json:"since,omitempty"`
	PerPage       int    `json:"per_page,omitempty"`
}

type orderListEntry struct {
	ID            int    `json:"id"`
	Number        string `json:"number,omitempty"`
	Status        string `json:"status"`
	DateCreated   string `json:"date_created,omitempty"`
	Total         string `json:"total,omitempty"`
	Currency      string `json:"currency,omitempty"`
	CustomerEmail string `json:"customer_email,omitempty"`
	BillingName   string `json:"billing_name,omitempty"`
}

func (h *ListOrdersTool) Definition() anthropic.ToolDef {
	return anthropic.ToolDef{
		Name: "list_orders",
		Description: "Find orders by status / customer / recency. `status` " +
			"is the WooCommerce order status (e.g. processing, completed, " +
			"refunded). `customer_email` filters to one customer. `since` is " +
			"an ISO8601 date or shorthand like '-7d'. `per_page` defaults to " +
			"20 and caps at 100.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"status":         { "type": "string" },
				"customer_email": { "type": "string" },
				"since":          { "type": "string" },
				"per_page":       { "type": "integer", "default": 20, "minimum": 1, "maximum": 100 }
			}
		}`),
	}
}

func (h *ListOrdersTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	if h.MCP == nil {
		return "", errors.New("list_orders: MCP client not configured")
	}
	var in listOrdersInput
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &in); err != nil {
			return "", fmt.Errorf("invalid input: %w", err)
		}
	}
	if in.PerPage <= 0 {
		in.PerPage = 20
	}
	if in.PerPage > 100 {
		in.PerPage = 100
	}
	params := map[string]any{"per_page": in.PerPage}
	if s := strings.TrimSpace(in.Status); s != "" {
		params["status"] = s
	}
	if e := strings.TrimSpace(in.CustomerEmail); e != "" {
		params["customer_email"] = e
	}
	if s := strings.TrimSpace(in.Since); s != "" {
		if t, err := parseSince(s); err == nil && !t.IsZero() {
			params["after"] = t.UTC().Format("2006-01-02T15:04:05")
		}
	}
	var data struct {
		Orders []orderListEntry `json:"orders"`
	}
	if err := callAskAbility(ctx, h.MCP, "wooagent-orders/list", params, &data); err != nil {
		return "", err
	}
	return marshalJSON(struct {
		Orders []orderListEntry `json:"orders"`
		Count  int              `json:"count"`
	}{Orders: data.Orders, Count: len(data.Orders)})
}

// GetOrderTool wraps the `wooagent-orders/get` MCP ability. Returns
// full body including customer, line items, status, total, and date —
// the substrate the model uses to ground a reply draft.
type GetOrderTool struct {
	MCP *mcp.Client
}

type getOrderInput struct {
	ID     int    `json:"id,omitempty"`
	Number string `json:"number,omitempty"`
}

type orderDetail struct {
	ID            int         `json:"id"`
	Number        string      `json:"number,omitempty"`
	Status        string      `json:"status"`
	DateCreated   string      `json:"date_created,omitempty"`
	Total         string      `json:"total,omitempty"`
	Currency      string      `json:"currency,omitempty"`
	CustomerID    int         `json:"customer_id,omitempty"`
	CustomerEmail string      `json:"customer_email,omitempty"`
	BillingName   string      `json:"billing_name,omitempty"`
	ShippingName  string      `json:"shipping_name,omitempty"`
	LineItems     []orderLine `json:"line_items,omitempty"`
	CustomerNote  string      `json:"customer_note,omitempty"`
}

type orderLine struct {
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
	Total    string `json:"total,omitempty"`
	SKU      string `json:"sku,omitempty"`
}

func (h *GetOrderTool) Definition() anthropic.ToolDef {
	return anthropic.ToolDef{
		Name: "get_order",
		Description: "Fetch a single order by id or order number. Returns " +
			"full body including customer email, billing/shipping name, line " +
			"items, status, total, currency, and date. Use before " +
			"`produce_reply_draft` so the draft is grounded in the real order.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"id":     { "type": "integer" },
				"number": { "type": "string", "description": "Order number (may differ from id; e.g. '#4521')" }
			}
		}`),
	}
}

func (h *GetOrderTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	if h.MCP == nil {
		return "", errors.New("get_order: MCP client not configured")
	}
	var in getOrderInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	params := map[string]any{}
	switch {
	case in.ID > 0:
		params["id"] = in.ID
	case strings.TrimSpace(in.Number) != "":
		params["number"] = strings.TrimPrefix(strings.TrimSpace(in.Number), "#")
	default:
		return "", errors.New("get_order: provide id or number")
	}
	var o orderDetail
	if err := callAskAbility(ctx, h.MCP, "wooagent-orders/get", params, &o); err != nil {
		return "", err
	}
	return marshalJSON(o)
}

// --------------------------------------------------------- produce_reply_draft

// ProduceReplyDraftTool writes a Sales Support customer-reply draft
// proposal. Mirrors the cadence-mode `customer_reply_draft` proposal
// shape so the board renders chat-mode and cadence-mode drafts
// identically. The model supplies the body in chat; this tool is a
// pure writer.
type ProduceReplyDraftTool struct {
	DB *sql.DB
}

type produceReplyDraftInput struct {
	OrderID      int    `json:"order_id"`
	OrderNumber  string `json:"order_number,omitempty"`
	OrderStatus  string `json:"order_status,omitempty"`
	OrderTotal   string `json:"order_total,omitempty"`
	OrderCurrency string `json:"order_currency,omitempty"`
	CustomerEmail string `json:"customer_email,omitempty"`
	CustomerName string `json:"customer_name,omitempty"`
	NoteType     string `json:"note_type"`
	SubjectHint  string `json:"subject_hint"`
	Body         string `json:"body"`
}

var allowedNoteTypes = map[string]bool{
	"shipping_update":  true,
	"refund_apology":   true,
	"product_question": true,
	"general_followup": true,
}

func (h *ProduceReplyDraftTool) Definition() anthropic.ToolDef {
	return anthropic.ToolDef{
		Name: "produce_reply_draft",
		Description: "Create a customer-reply draft proposal on the board. " +
			"Call after `get_order` so order_id/number/status/total/customer " +
			"reflect the real order. `note_type` is one of " +
			"shipping_update | refund_apology | product_question | general_followup. " +
			"`body` is 3-6 short sentences in your voice. `subject_hint` is " +
			"the suggested email subject. Returns proposal_id; surface as " +
			"[proposal #<id>] in your one-line chat receipt — do NOT paste " +
			"the body in chat.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"order_id":      { "type": "integer" },
				"order_number":  { "type": "string" },
				"order_status":  { "type": "string" },
				"order_total":   { "type": "string" },
				"order_currency":{ "type": "string" },
				"customer_email":{ "type": "string" },
				"customer_name": { "type": "string" },
				"note_type":     {
					"type": "string",
					"enum": ["shipping_update", "refund_apology", "product_question", "general_followup"]
				},
				"subject_hint": { "type": "string" },
				"body":         { "type": "string", "description": "3-6 short sentences, plain text, no signature beyond the closing line" }
			},
			"required": ["order_id", "note_type", "subject_hint", "body"]
		}`),
	}
}

func (h *ProduceReplyDraftTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	if h.DB == nil {
		return "", errors.New("produce_reply_draft: DB not configured")
	}
	var in produceReplyDraftInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if in.OrderID <= 0 {
		return "", errors.New("order_id is required (call get_order first to resolve)")
	}
	if !allowedNoteTypes[in.NoteType] {
		return "", fmt.Errorf("note_type %q not in {shipping_update, refund_apology, product_question, general_followup}", in.NoteType)
	}
	if strings.TrimSpace(in.SubjectHint) == "" {
		return "", errors.New("subject_hint is required")
	}
	if strings.TrimSpace(in.Body) == "" {
		return "", errors.New("body is required")
	}
	orderRef := in.OrderNumber
	if orderRef == "" {
		orderRef = fmt.Sprintf("%d", in.OrderID)
	}
	title := fmt.Sprintf("Customer note · order #%s · %s", orderRef, in.SubjectHint)
	description := fmt.Sprintf(
		"Drafted by Sales Support agent (chat) for order #%s.", orderRef,
	)
	target := map[string]any{
		"order_id":       in.OrderID,
		"order_number":   in.OrderNumber,
		"order_status":   in.OrderStatus,
		"order_total":    in.OrderTotal,
		"order_currency": in.OrderCurrency,
		"customer_email": in.CustomerEmail,
		"customer_name":  in.CustomerName,
		"note_type":      in.NoteType,
		"subject_hint":   in.SubjectHint,
	}
	dedup := fmt.Sprintf("order:%d", in.OrderID)
	issueID, err := insertOperatorAskedProposal(ctx, h.DB, "sales-support", title, description,
		"customer_reply_draft", in.Body, target, dedup)
	if err != nil {
		return "", err
	}
	return marshalJSON(produceOutput{
		OK:           true,
		ProposalID:   issueID,
		ProposalType: "customer_reply_draft",
	})
}

// Compile-time tool interface assertions.
var (
	_ anthropic.ToolHandler = (*ListOrdersTool)(nil)
	_ anthropic.ToolHandler = (*GetOrderTool)(nil)
	_ anthropic.ToolHandler = (*ProduceReplyDraftTool)(nil)
)
