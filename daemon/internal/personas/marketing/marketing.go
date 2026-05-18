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
	"regexp"
	"strings"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
	"github.com/wooagent-os/wooagent-os/daemon/internal/telemetry"
)

// variant is the shape the UI's variantsFromProposal expects to find under
// proposal.target.variants. Keep field names in sync with
// ui/src/api/client.ts:261 — id, body required; label/charCount/recommended
// optional; seo/voice default to 0 in the UI until real scoring lands.
type variant struct {
	ID          string `json:"id"`
	Label       string `json:"label,omitempty"`
	Body        string `json:"body"`
	CharCount   int    `json:"charCount,omitempty"`
	Recommended bool   `json:"recommended,omitempty"`
	Angle       string `json:"angle,omitempty"`
}

// llmVariant is what the LLM returns inside its JSON response. Translated
// to the persisted `variant` shape after parsing.
type llmVariant struct {
	Label string `json:"label"`
	Angle string `json:"angle"`
	Body  string `json:"body"`
}

type llmVariantsResp struct {
	Variants []llmVariant `json:"variants"`
}

// jsonObjectRe extracts the first {...} block from a possibly-noisy LLM
// response. Permissive on whitespace and surrounding chatter so a stray
// "Here is the JSON:" prefix doesn't fail the parse.
var jsonObjectRe = regexp.MustCompile(`(?s)\{.*\}`)

// parseVariants pulls a structured 3-variant response out of LLM text. The
// caller falls back to single-variant on a non-nil error. Strict on count
// (must be 3) to keep the contract with the operator clear; permissive on
// the surrounding chatter via jsonObjectRe.
func parseVariants(raw string) ([]variant, error) {
	block := jsonObjectRe.FindString(raw)
	if block == "" {
		return nil, fmt.Errorf("no JSON object found in LLM output")
	}
	var parsed llmVariantsResp
	if err := json.Unmarshal([]byte(block), &parsed); err != nil {
		return nil, fmt.Errorf("decode variants JSON: %w", err)
	}
	if len(parsed.Variants) != 3 {
		return nil, fmt.Errorf("expected 3 variants, got %d", len(parsed.Variants))
	}
	out := make([]variant, 0, 3)
	for i, v := range parsed.Variants {
		body := strings.TrimSpace(v.Body)
		if body == "" {
			return nil, fmt.Errorf("variant %d has empty body", i)
		}
		label := strings.TrimSpace(v.Label)
		if label == "" {
			label = string(rune('A' + i))
		}
		out = append(out, variant{
			ID:          fmt.Sprintf("var_%s", strings.ToLower(label)),
			Label:       label,
			Body:        body,
			CharCount:   len(body),
			Recommended: i == 0,
			Angle:       strings.TrimSpace(v.Angle),
		})
	}
	return out, nil
}

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

// Cooldown: product-centric (proposal_target.product_id). 7d after an
// approve so we don't rewrite the same description we just wrote; 30d
// after a dismiss so the operator's "no" sticks.
func (Marketing) Cooldown() personas.CooldownPolicy {
	return personas.CooldownPolicy{
		TargetKey: "product_id",
		Approved:  7 * 24 * time.Hour,
		Dismissed: 30 * 24 * time.Hour,
	}
}

const skillName = "marketing.description-rewrite"

// maxDraftAttempts caps how many products a single Draft run will try
// before giving up. Each attempt costs one LLM call (~5-15s). 3 is a
// pragmatic balance: most catalogs have only a few "undraftable" products
// at any given time, and bounding latency keeps a run from hogging the
// worker. The cooldown set is appended to in-memory after each LLM
// no_proposal so the loop doesn't re-pick the same product.
const maxDraftAttempts = 3

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

	skill, ok := deps.Skills[skillName]
	if !ok {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("skill %q not found in skills registry", skillName),
		}, nil
	}

	// Debug override: always draft on the operator-supplied product,
	// bypassing both cooldown and the within-run loop.
	if deps.Env.ProductIDOverride != 0 {
		return draftForProduct(ctx, deps, deps.Env.ProductIDOverride, skill.Description)
	}

	// Skip products that already have an open issue, an approved
	// proposal within the last 7d, or a dismissed proposal within the
	// last 30d for this persona (see Marketing.Cooldown).
	m := Marketing{}
	skip, err := personas.RecentlyTouchedTargets(ctx, deps.Store, m.Slug(), m.Cooldown())
	if err != nil {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("look up recently-touched products: %v", err),
		}, nil
	}

	// Within-run iteration: if the LLM can't draft for a product (returns
	// no_proposal / empty rewrite), add it to the run-local skip set and
	// try the next eligible product. Up to maxDraftAttempts.
	return personas.IterateDraft(
		maxDraftAttempts,
		"product",
		skip,
		func(s map[int]struct{}) (int, error) { return pickFirstPublished(ctx, deps.MCP, s) },
		func(id int) (personas.Drafted, error) { return draftForProduct(ctx, deps, id, skill.Description) },
	)
}

// draftForProduct does the per-product work: fetch via MCP, call the
// LLM, parse variants, assemble Drafted. Returns Drafted{Skipped:true}
// when the LLM yields no_proposal or an empty rewrite — the outer Draft
// loop treats that as "try the next product" rather than ending the run.
func draftForProduct(ctx context.Context, deps personas.Deps, productID int, skillDescription string) (personas.Drafted, error) {
	p, err := getProduct(ctx, deps.MCP, productID)
	if err != nil {
		return personas.Drafted{}, fmt.Errorf("get product %d: %w", productID, err)
	}

	rawOutput, skipReason, err := draftWithFallback(ctx, deps.Env, p, skillDescription)
	if err != nil {
		return personas.Drafted{}, err
	}
	if skipReason != "" {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: skipReason,
		}, nil
	}

	// Try to parse the structured 3-variant response. On parse failure,
	// fall back to single-variant with the raw text as content so the
	// operator still gets something actionable instead of a skipped run.
	variants, parseErr := parseVariants(rawOutput)
	target := map[string]any{
		"product_id":   p.ID,
		"product_name": p.Name,
		"product_sku":  p.SKU,
		"previous":     p.Description,
		"image_url":    p.ImageURL,
		"image_alt":    p.ImageAlt,
	}
	var content string
	if parseErr == nil {
		target["variants"] = variants
		// proposal.content is the recommended variant's body. The UI's
		// multi-variant view reads target.variants; surfaces that handle
		// single-content (run log, archived row, dismiss dialog body) get
		// a coherent string instead of raw JSON.
		content = variants[0].Body
	} else {
		// Single-variant fallback — surface the parse failure in the
		// daemon log so it's visible during the testing-call pass.
		fmt.Printf("marketing: variants parse failed (%v); falling back to single-variant\n", parseErr)
		content = strings.TrimSpace(rawOutput)
	}

	return personas.Drafted{
		Title: fmt.Sprintf("Product description rewrite · %s", p.Name),
		Description: fmt.Sprintf(
			"Drafted by Marketing agent for product #%d (%s).",
			p.ID, p.SKU,
		),
		Priority:        "medium",
		ProposalType:    "product_description_rewrite",
		ProposalContent: content,
		Target:          target,
		// Belt over the existing product_id Cooldown — the picker
		// already skips products with open issues, but RunAndPersist's
		// insert-time guard catches races that bypass the picker.
		// See docs/specs/2026-05-18-agent-proposal-dedup-design.md.
		DedupKey: fmt.Sprintf("product:%d", p.ID),
	}, nil
}

// ---- MCP helpers (private to this package) ----

type abilityEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error,omitempty"`
}

// callAbility wraps mcp.CallTool with TurnEvent skill-call recording. The
// inner func does the actual work; the wrapper measures latency + reports
// status to the tracker on ctx (no-op if no tracker is attached).
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
	ID                     int    `json:"id"`
	Name                   string `json:"name"`
	SKU                    string `json:"sku"`
	Status                 string `json:"status"`
	DescriptionLength      int    `json:"description_length"`
	ShortDescriptionLength int    `json:"short_description_length"`
}

// pickFirstPublished returns the next published product whose ID is not
// in skip. Surfaces stale + content-thin products first (orderby=date_modified
// asc, then a client-side bias toward products with empty descriptions —
// "data_issues" in the WC AI plugin's behavioral vocabulary). Marketing's
// job is to provide descriptions for products that need them most; sorting
// by date_modified ascending puts the products no one's touched in a while
// at the top, and the data_issues bias prefers ones missing the content
// Marketing can supply.
//
// orderby is passed through to the Companion Plugin (v0.2+ accepts it). On
// stores still running v0.1 of the plugin the arg is rejected by the
// ability's additionalProperties:false schema — surfaces as an explicit
// error rather than a silent mis-order.
func pickFirstPublished(ctx context.Context, c *mcp.Client, skip map[int]struct{}) (int, error) {
	var listOut struct {
		Products []productSummary `json:"products"`
	}
	if err := callAbility(ctx, c, "wooagent-products/list",
		map[string]any{"per_page": 100, "orderby": "date_modified", "order": "asc"}, &listOut); err != nil {
		return 0, err
	}
	if len(listOut.Products) == 0 {
		return 0, fmt.Errorf("no products in store")
	}
	// Two-pass: first prefer products with an obvious data_issue (empty
	// description); fall back to first-eligible if no data_issues remain.
	skipped := 0
	for _, p := range listOut.Products {
		if p.Status != "publish" && p.Status != "" {
			continue
		}
		if _, inCooldown := skip[p.ID]; inCooldown {
			skipped++
			continue
		}
		if p.DescriptionLength == 0 || p.ShortDescriptionLength == 0 {
			return p.ID, nil
		}
	}
	for _, p := range listOut.Products {
		if p.Status != "publish" && p.Status != "" {
			continue
		}
		if _, inCooldown := skip[p.ID]; inCooldown {
			continue
		}
		return p.ID, nil
	}
	if skipped > 0 {
		return 0, fmt.Errorf(
			"every published product in the first %d is in cooldown (%d skipped); "+
				"approved proposals cool down for 7d, dismissed for 30d",
			len(listOut.Products), skipped,
		)
	}
	return 0, fmt.Errorf("no published products in store")
}

type product struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	SKU         string `json:"sku"`
	Status      string `json:"status"`
	Description string `json:"description"`
	ShortDesc   string `json:"short_description"`
	Permalink   string `json:"permalink"`
	ImageURL    string `json:"image_url"`
	ImageAlt    string `json:"image_alt"`
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
func draftWithFallback(ctx context.Context, env personas.Env, p product, skillDescription string) (string, string, error) {
	if strings.TrimSpace(env.AnthropicAPIKey) != "" {
		model := env.AnthropicModel
		if model == "" {
			model = defaultAnthropicModel
		}
		rewrite, err := draftRewriteAnthropic(ctx, env.AnthropicAPIKey, model, p, skillDescription)
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
	rewrite, err := draftRewriteOpenAI(ctx, base, apiKey, model, p, skillDescription)
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
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage,omitempty"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func draftRewriteAnthropic(ctx context.Context, apiKey, model string, p product, skillDescription string) (string, error) {
	user := fmt.Sprintf(
		"Product: %s\nSKU: %s\nCurrent description: %s\n\nWrite a new description.",
		p.Name, p.SKU, strings.TrimSpace(p.Description),
	)

	body, _ := json.Marshal(anthropicReq{
		Model:     model,
		MaxTokens: 2048, // headroom for 3 variants × ~220 chars + JSON overhead
		System:    skillDescription,
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

	if t := telemetry.TrackerFromContext(ctx); t != nil {
		t.RecordModelCall(telemetry.ModelCall{
			Provider:     "anthropic",
			Model:        model,
			InputTokens:  parsed.Usage.InputTokens,
			OutputTokens: parsed.Usage.OutputTokens,
		})
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
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage,omitempty"`
}

func draftRewriteOpenAI(
	ctx context.Context,
	base, apiKey, model string,
	p product,
	skillDescription string,
) (string, error) {
	user := fmt.Sprintf(
		"Product: %s\nSKU: %s\nCurrent description: %s\n\nWrite a new description.",
		p.Name, p.SKU, strings.TrimSpace(p.Description),
	)
	body, _ := json.Marshal(chatReq{
		Model: model,
		Messages: []chatMsg{
			{Role: "system", Content: skillDescription},
			{Role: "user", Content: user},
		},
		Temperature: 0.7,
		MaxTokens:   2048, // headroom for 3 variants × ~220 chars + JSON overhead
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
	if t := telemetry.TrackerFromContext(ctx); t != nil {
		t.RecordModelCall(telemetry.ModelCall{
			Provider:     "openai",
			Model:        model,
			InputTokens:  parsed.Usage.PromptTokens,
			OutputTokens: parsed.Usage.CompletionTokens,
		})
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}
