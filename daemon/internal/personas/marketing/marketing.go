// Package marketing is the Marketing-agent implementation.
//
// Side-effect-registers itself with personas.Register on import. Reads a
// product via MCP, drafts a brand-voice rewrite, lands a proposal of type
// product_description_rewrite.
//
// LLM routing:
//   - When ANTHROPIC_API_KEY is set, calls Claude directly via /v1/messages
//     (matches pricing and sales-support; this is the production path).
//   - Otherwise falls back to an OpenAI-compatible chat-completions
//     endpoint (LM Studio's gemma by default) — kept as a no-key escape
//     hatch for local spike work.
//
// Skipped outcomes (no daemon failure):
//   - MCP not configured
//   - LLM call errored (Claude rate-limit, LM Studio not running, etc.)
//   - The store has no published products
package marketing

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
	defaultAnthropicModel = "claude-sonnet-4-6"
	anthropicAPIURL       = "https://api.anthropic.com/v1/messages"
	anthropicVersion      = "2023-06-01"

	defaultOpenAIBase  = "http://localhost:1234/v1"
	defaultOpenAIModel = "google/gemma-4-e4b"
	defaultOpenAIKey   = "lm-studio"
)

func init() {
	personas.Register(&Marketing{})
}

type Marketing struct{}

func (Marketing) Slug() string        { return "marketing" }
func (Marketing) DisplayName() string { return "Marketing agent" }

const systemPrompt = `You are a copywriter for a small-batch home-goods store.
Voice: warm, sincere, concrete. Avoid the words "luxe", "premium", "elevate",
"curated". Prefer "small-batch", "handcrafted", "made to last". Lead with the
material or the use, not adjectives. Two to four short sentences, 140-220
characters total. Output ONLY the new description — no preamble, no quotes,
no labels.`

func (Marketing) Draft(ctx context.Context, deps personas.Deps) (personas.Drafted, error) {
	if deps.MCP == nil {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: "MCP client not configured (set WOOAGENT_MCP_URL/USER/APP_PASSWORD on the daemon)",
		}, nil
	}

	if _, err := deps.MCP.Initialize(ctx); err != nil {
		return personas.Drafted{}, fmt.Errorf("mcp initialize: %w", err)
	}

	productID := deps.Env.ProductIDOverride
	if productID == 0 {
		first, err := pickFirstPublished(ctx, deps.MCP)
		if err != nil {
			return personas.Drafted{
				Skipped:    true,
				SkipReason: err.Error(),
			}, nil
		}
		productID = first
	}
	p, err := getProduct(ctx, deps.MCP, productID)
	if err != nil {
		return personas.Drafted{}, fmt.Errorf("get product %d: %w", productID, err)
	}

	rewrite, skipReason, err := draftWithFallback(ctx, deps.Env, p)
	if err != nil {
		return personas.Drafted{}, err
	}
	if skipReason != "" {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: skipReason,
		}, nil
	}

	return personas.Drafted{
		Title: fmt.Sprintf("Product description rewrite · %s", p.Name),
		Description: fmt.Sprintf(
			"Drafted by Marketing persona for product #%d (%s).",
			p.ID, p.SKU,
		),
		Priority:        "medium",
		ProposalType:    "product_description_rewrite",
		ProposalContent: rewrite,
		Target: map[string]any{
			"product_id":   p.ID,
			"product_name": p.Name,
			"product_sku":  p.SKU,
			"previous":     p.Description,
		},
	}, nil
}

// ---- MCP helpers (private to this package) ----

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

type productSummary struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	SKU    string `json:"sku"`
	Status string `json:"status"`
}

func pickFirstPublished(ctx context.Context, c *mcp.Client) (int, error) {
	var listOut struct {
		Products []productSummary `json:"products"`
	}
	if err := callAbility(ctx, c, "wooagent-products/list",
		map[string]any{"per_page": 5}, &listOut); err != nil {
		return 0, err
	}
	for _, p := range listOut.Products {
		if p.Status == "publish" || p.Status == "" {
			return p.ID, nil
		}
	}
	if len(listOut.Products) == 0 {
		return 0, fmt.Errorf("no products in store")
	}
	return listOut.Products[0].ID, nil
}

type product struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	SKU         string `json:"sku"`
	Status      string `json:"status"`
	Description string `json:"description"`
	ShortDesc   string `json:"short_description"`
	Permalink   string `json:"permalink"`
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

// ---- LLM ----

// draftWithFallback prefers Anthropic when AnthropicAPIKey is set, and
// falls back to the OpenAI-compatible endpoint (LM Studio by default)
// otherwise. Returns (rewrite, skipReason, err): a non-empty skipReason
// means the caller should mark the run Skipped.
func draftWithFallback(ctx context.Context, env personas.Env, p product) (string, string, error) {
	if strings.TrimSpace(env.AnthropicAPIKey) != "" {
		model := env.AnthropicModel
		if model == "" {
			model = defaultAnthropicModel
		}
		rewrite, err := draftRewriteAnthropic(ctx, env.AnthropicAPIKey, model, p)
		if err != nil {
			return "", fmt.Sprintf("Claude API errored: %v", err), nil
		}
		if rewrite = strings.TrimSpace(rewrite); rewrite == "" {
			return "", "Claude returned empty rewrite", nil
		}
		return rewrite, "", nil
	}

	base := env.OpenAIAPIBase
	if base == "" {
		base = defaultOpenAIBase
	}
	model := env.OpenAIModel
	if model == "" {
		model = defaultOpenAIModel
	}
	apiKey := env.OpenAIAPIKey
	if apiKey == "" {
		apiKey = defaultOpenAIKey
	}
	rewrite, err := draftRewriteOpenAI(ctx, base, apiKey, model, p)
	if err != nil {
		return "", fmt.Sprintf("LLM endpoint at %s unreachable or errored: %v", base, err), nil
	}
	if rewrite = strings.TrimSpace(rewrite); rewrite == "" {
		return "", "LLM returned empty rewrite", nil
	}
	return rewrite, "", nil
}

// ---- Anthropic ----

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

func draftRewriteAnthropic(ctx context.Context, apiKey, model string, p product) (string, error) {
	user := fmt.Sprintf(
		"Product: %s\nSKU: %s\nCurrent description: %s\n\nWrite a new description.",
		p.Name, p.SKU, strings.TrimSpace(p.Description),
	)

	body, _ := json.Marshal(anthropicReq{
		Model:     model,
		MaxTokens: 512,
		System:    systemPrompt,
		Messages:  []anthropicMsg{{Role: "user", Content: user}},
	})

	cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(cctx, "POST", anthropicAPIURL, bytes.NewReader(body))
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("content-type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic http: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		return "", fmt.Errorf("anthropic http %d: %s", res.StatusCode, raw)
	}
	var parsed anthropicResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("anthropic decode: %w", err)
	}
	if parsed.Error.Type != "" {
		return "", fmt.Errorf("anthropic error: %s · %s", parsed.Error.Type, parsed.Error.Message)
	}

	var sb strings.Builder
	for _, b := range parsed.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

// ---- OpenAI-compatible (LM Studio fallback) ----

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatReq struct {
	Model       string    `json:"model"`
	Messages    []chatMsg `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
}

type chatResp struct {
	Choices []struct {
		Message chatMsg `json:"message"`
	} `json:"choices"`
}

func draftRewriteOpenAI(
	ctx context.Context,
	base, apiKey, model string,
	p product,
) (string, error) {
	user := fmt.Sprintf(
		"Product: %s\nSKU: %s\nCurrent description: %s\n\nWrite a new description.",
		p.Name, p.SKU, strings.TrimSpace(p.Description),
	)
	body, _ := json.Marshal(chatReq{
		Model: model,
		Messages: []chatMsg{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: user},
		},
		Temperature: 0.7,
		MaxTokens:   220,
	})

	cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(cctx, "POST",
		strings.TrimRight(base, "/")+"/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm http: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		raw, _ := io.ReadAll(res.Body)
		return "", fmt.Errorf("llm http %d: %s", res.StatusCode, raw)
	}
	var parsed chatResp
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("llm decode: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices")
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}
