// Package salessupport is the Sales Support agent implementation.
//
// One-shot Phase 2 behavior: pick the most recent order in a state where a
// proactive customer note is welcome (processing / completed), pull its
// details + customer info, and draft a warm, on-brand customer-facing
// note. Lands as a customer_reply_draft proposal that the operator
// approves; approval ships the note via wooagent-orders/add-note with
// is_customer_note=true.
//
// LLM: Claude Haiku 4.5 by default. Customer-facing copy is the highest
// stakes prose the daemon writes — wrong tone is more visible than wrong
// product description. The cost-posture note in progress.md explicitly
// flagged "review/approval testing in Phase 2-4" as the time to flip to
// Claude; this persona qualifies.
//
// Skipped outcomes:
//   - MCP not configured
//   - ANTHROPIC_API_KEY not set
//   - No recent orders in a notable state (every order is fresh / cancelled)
package salessupport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
)

const (
	defaultAnthropicModel = "claude-haiku-4-5-20251001"
	anthropicAPIURL       = "https://api.anthropic.com/v1/messages"
	anthropicVersion      = "2023-06-01"
)

func init() {
	personas.Register(&SalesSupport{})
}

type SalesSupport struct{}

func (SalesSupport) Slug() string        { return "sales-support" }
func (SalesSupport) DisplayName() string { return "Sales Support agent" }

const systemPrompt = `You are the customer-facing sales-support voice for a small-batch home-goods store.

Voice: warm, personal, plainspoken. The customer is a person, not a ticket. Acknowledge what they ordered specifically. Don't oversell, don't upsell, don't promise timelines you can't keep. Avoid corporate phrases ("we appreciate your business", "thank you for your patience"). Sign off with a real first name (use "Elizabeth" as the default; the operator can swap before sending).

Length: 3 to 6 short sentences. Plain text, no markdown, no signature block beyond the closing line.

When the order status is "processing": acknowledge the order, name one specific item, set a calm next-step expectation (handcrafted, will ship soon — no exact date promises), invite questions.

When the order status is "completed": follow up briefly. Mention something specific from what they bought. Offer help with care or use, not another purchase.

When the order is in any other state (on-hold, pending, etc.): no_proposal=true with a one-sentence reason naming the status.

Output ONLY a JSON object that matches this exact shape (no preamble, no markdown fences, no commentary):

{
  "no_proposal": false,
  "note_type": "customer",
  "subject_hint": "short label for the operator (1-4 words, e.g. 'Order processing follow-up')",
  "message": "Hi Maria,\n\n…the multi-line plain-text message…\n\n— Elizabeth"
}

Or, when declining:

{
  "no_proposal": true,
  "reason_no_proposal": "Order is on-hold pending payment; a customer-facing note from us would be premature."
}`

func (SalesSupport) Draft(ctx context.Context, deps personas.Deps) (personas.Drafted, error) {
	if deps.MCP == nil {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: "MCP client not configured (set WOOAGENT_MCP_URL/USER/APP_PASSWORD on the daemon)",
		}, nil
	}
	if strings.TrimSpace(deps.Env.AnthropicAPIKey) == "" {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: "ANTHROPIC_API_KEY not set; sales support uses Claude for customer-facing tone",
		}, nil
	}

	if _, err := deps.MCP.Initialize(ctx); err != nil {
		return personas.Drafted{}, fmt.Errorf("mcp initialize: %w", err)
	}

	orderID, status, err := pickOrder(ctx, deps.MCP)
	if err != nil {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: err.Error(),
		}, nil
	}
	o, err := getOrder(ctx, deps.MCP, orderID)
	if err != nil {
		return personas.Drafted{}, fmt.Errorf("get order %d: %w", orderID, err)
	}
	// pickOrder told us the status; trust the get response if it differs.
	if strings.TrimSpace(o.Status) != "" {
		status = o.Status
	}

	model := deps.Env.AnthropicModel
	if model == "" {
		model = defaultAnthropicModel
	}

	out, raw, err := draftMessage(ctx, deps.Env.AnthropicAPIKey, model, o)
	if err != nil {
		return personas.Drafted{}, fmt.Errorf("draft message: %w (raw=%s)", err, truncate(raw, 400))
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
	if strings.TrimSpace(out.Message) == "" {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: "model returned empty message",
		}, nil
	}

	noteType := strings.ToLower(strings.TrimSpace(out.NoteType))
	if noteType != "internal" {
		// Default to customer-facing — that's what this persona is for.
		noteType = "customer"
	}

	customerName := firstNonEmpty(o.BillingName, o.ShippingName, "the customer")
	titleSubject := strings.TrimSpace(out.SubjectHint)
	if titleSubject == "" {
		titleSubject = humanStatus(status) + " follow-up"
	}
	title := fmt.Sprintf("Customer note · order #%s · %s",
		firstNonEmpty(o.Number, fmt.Sprintf("%d", o.ID)),
		titleSubject,
	)

	return personas.Drafted{
		Title: title,
		Description: fmt.Sprintf(
			"Drafted by Sales Support persona for order #%s (%s · %s %s).",
			firstNonEmpty(o.Number, fmt.Sprintf("%d", o.ID)),
			status, o.Total, o.Currency,
		),
		Priority:        "medium",
		ProposalType:    "customer_reply_draft",
		ProposalContent: out.Message,
		Target: map[string]any{
			"order_id":       o.ID,
			"order_number":   o.Number,
			"order_status":   status,
			"order_total":    o.Total,
			"order_currency": o.Currency,
			"order_date":     o.DateCreated,
			"customer_id":    o.CustomerID,
			"customer_email": o.CustomerEmail,
			"customer_name":  customerName,
			"line_items":     summarizeLineItems(o.LineItems),
			"note_type":      noteType,
			"subject_hint":   titleSubject,
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

type orderSummary struct {
	ID            int    `json:"id"`
	Number        string `json:"number"`
	Status        string `json:"status"`
	Total         string `json:"total"`
	Currency      string `json:"currency"`
	CustomerID    int    `json:"customer_id"`
	LineItemCount int    `json:"line_item_count"`
	DateCreated   string `json:"date_created"`
}

// pickOrder returns the most recent order in a state where a customer
// note is welcome. Skips on-hold / pending / cancelled / refunded — those
// either need different copy or shouldn't get a proactive note from us.
func pickOrder(ctx context.Context, c *mcp.Client) (int, string, error) {
	var listOut struct {
		Orders []orderSummary `json:"orders"`
		Total  int            `json:"total"`
	}
	if err := callAbility(ctx, c, "wooagent-orders/list",
		map[string]any{"per_page": 25}, &listOut); err != nil {
		return 0, "", err
	}
	if len(listOut.Orders) == 0 {
		return 0, "", fmt.Errorf("no orders in store")
	}
	for _, o := range listOut.Orders {
		switch strings.ToLower(strings.TrimSpace(o.Status)) {
		case "processing", "completed":
			return o.ID, o.Status, nil
		}
	}
	return 0, "", fmt.Errorf(
		"no recent orders in 'processing' or 'completed' (looked at %d orders); set PERSONA_PRODUCT_ID is not applicable here, place a real order or seed one to demo",
		len(listOut.Orders),
	)
}

type lineItem struct {
	ProductID int    `json:"product_id"`
	Name      string `json:"name"`
	Quantity  int    `json:"quantity"`
	Total     string `json:"total"`
	SKU       string `json:"sku"`
}

type order struct {
	ID            int        `json:"id"`
	Number        string     `json:"number"`
	Status        string     `json:"status"`
	Total         string     `json:"total"`
	Currency      string     `json:"currency"`
	CustomerID    int        `json:"customer_id"`
	CustomerEmail string     `json:"customer_email"`
	BillingName   string     `json:"billing_name"`
	ShippingName  string     `json:"shipping_name"`
	PaymentMethod string     `json:"payment_method"`
	LineItems     []lineItem `json:"line_items"`
	DateCreated   string     `json:"date_created"`
	DateModified  string     `json:"date_modified"`
}

func getOrder(ctx context.Context, c *mcp.Client, id int) (order, error) {
	var o order
	if err := callAbility(ctx, c, "wooagent-orders/get",
		map[string]any{"id": id}, &o); err != nil {
		return o, err
	}
	if o.ID == 0 {
		o.ID = id
	}
	return o, nil
}

func summarizeLineItems(items []lineItem) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, li := range items {
		out = append(out, map[string]any{
			"product_id": li.ProductID,
			"name":       li.Name,
			"quantity":   li.Quantity,
			"total":      li.Total,
			"sku":        li.SKU,
		})
	}
	return out
}

// ---------------------------------------------------------------- Anthropic

type messageOut struct {
	NoProposal       bool   `json:"no_proposal"`
	ReasonNoProposal string `json:"reason_no_proposal,omitempty"`
	NoteType         string `json:"note_type,omitempty"`
	SubjectHint      string `json:"subject_hint,omitempty"`
	Message          string `json:"message,omitempty"`
}

type anthropicMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicReq struct {
	Model     string         `json:"model"`
	MaxTokens int            `json:"max_tokens"`
	System    string         `json:"system"`
	Messages  []anthropicMsg `json:"messages"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type anthropicResp struct {
	Content []anthropicContentBlock `json:"content"`
	Error   struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func draftMessage(ctx context.Context, apiKey, model string, o order) (messageOut, string, error) {
	customerName := firstNonEmpty(o.BillingName, o.ShippingName, "the customer")
	itemsText := formatLineItemsForPrompt(o.LineItems)
	user := fmt.Sprintf(
		`Order to write a note for:

- order_number: %s
- status: %s
- total: %s %s
- date_created: %s
- customer_name: %s
- customer_email: %s
- line_items:
%s

Write a customer-facing note matching the tone in the system prompt. Output the JSON object only.`,
		firstNonEmpty(o.Number, fmt.Sprintf("%d", o.ID)),
		o.Status, o.Total, o.Currency, o.DateCreated,
		customerName, o.CustomerEmail, itemsText,
	)

	body, _ := json.Marshal(anthropicReq{
		Model:     model,
		MaxTokens: 1024,
		System:    systemPrompt,
		Messages:  []anthropicMsg{{Role: "user", Content: user}},
	})

	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(cctx, "POST", anthropicAPIURL, bytes.NewReader(body))
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("content-type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return messageOut{}, "", fmt.Errorf("anthropic http: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		return messageOut{}, string(raw), fmt.Errorf("anthropic http %d", res.StatusCode)
	}
	var parsed anthropicResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return messageOut{}, string(raw), fmt.Errorf("anthropic decode: %w", err)
	}
	if parsed.Error.Type != "" {
		return messageOut{}, string(raw), fmt.Errorf("anthropic error: %s · %s", parsed.Error.Type, parsed.Error.Message)
	}

	var sb strings.Builder
	for _, b := range parsed.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	jsonBlob := extractJSONObject(strings.TrimSpace(sb.String()))
	if jsonBlob == "" {
		return messageOut{}, sb.String(), fmt.Errorf("no JSON object found in model output")
	}
	var out messageOut
	if err := json.Unmarshal([]byte(jsonBlob), &out); err != nil {
		return messageOut{}, jsonBlob, fmt.Errorf("decode message JSON: %w", err)
	}
	return out, jsonBlob, nil
}

func formatLineItemsForPrompt(items []lineItem) string {
	if len(items) == 0 {
		return "  (no line items)"
	}
	var sb strings.Builder
	for _, li := range items {
		sb.WriteString(fmt.Sprintf("  - %s × %d (sku %s)\n", li.Name, li.Quantity, firstNonEmpty(li.SKU, "—")))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func humanStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "processing":
		return "Order processing"
	case "completed":
		return "Order completed"
	case "on-hold":
		return "Order on hold"
	case "pending":
		return "Pending payment"
	}
	return s
}

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

func firstNonEmpty(args ...string) string {
	for _, a := range args {
		if strings.TrimSpace(a) != "" {
			return a
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
