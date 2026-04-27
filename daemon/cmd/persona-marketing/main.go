// Command persona-marketing is the Phase 1 Marketing-agent harness.
//
// Pulls a single product from the connected store via MCP, asks an
// OpenAI-compat model (LM Studio's gemma by default — see the
// "prefer-local-LLM-for-spikes" stance) to draft a brand-voice rewrite of
// the description, and POSTs an issue to the daemon with that draft attached
// as a proposal so the operator sees it in `In Review` on the kanban.
//
// The persona is *one-shot* on purpose. Phase 1 only has to prove that:
//
//   product (MCP) → drafted rewrite (LLM) → issue with proposal (HTTP)
//
// works end-to-end. The persistent loop, multi-variant proposals, and
// scheduled scans are Phase 2 concerns.
//
// Run:
//
//	WOOAGENT_MCP_URL=https://<store>/wp-json/mcp/mcp-adapter-default-server \
//	WOOAGENT_MCP_USER=<wp-user-or-email> \
//	WOOAGENT_MCP_APP_PASSWORD=<wp-application-password> \
//	WOOAGENT_DAEMON_TOKEN=<bearer-from-`wooagent auth token create`> \
//	go run ./cmd/persona-marketing
//
// Optional: PERSONA_PRODUCT_ID picks a specific product; without it the
// persona drafts for the first published product returned by the store.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
)

func main() {
	ctx := context.Background()

	mcpURL := mustEnv("WOOAGENT_MCP_URL")
	mcpUser := mustEnv("WOOAGENT_MCP_USER")
	mcpPass := strings.ReplaceAll(mustEnv("WOOAGENT_MCP_APP_PASSWORD"), " ", "")

	daemonURL := envOr("WOOAGENT_DAEMON_URL", "http://localhost:7777")
	daemonToken := mustEnv("WOOAGENT_DAEMON_TOKEN")

	llmBase := envOr("OPENAI_API_BASE_URL", "http://localhost:1234/v1")
	llmModel := envOr("OPENAI_MODEL", "google/gemma-4-e4b")
	llmKey := envOr("OPENAI_API_KEY", "lm-studio")

	productID, _ := strconv.Atoi(envOr("PERSONA_PRODUCT_ID", "0"))

	client := mcp.NewClient(mcp.Config{
		Endpoint: mcpURL,
		Username: mcpUser,
		Password: mcpPass,
	})

	fmt.Println("=== MCP handshake ===")
	if _, err := client.Initialize(ctx); err != nil {
		log.Fatalf("mcp initialize: %v", err)
	}
	fmt.Println("  ok · session:", client.SessionID())

	fmt.Println("\n=== fetch product ===")
	if productID == 0 {
		first, err := pickFirstProduct(ctx, client)
		if err != nil {
			log.Fatalf("pick first product: %v", err)
		}
		productID = first
	}
	product, err := getProduct(ctx, client, productID)
	if err != nil {
		log.Fatalf("get product %d: %v", productID, err)
	}
	fmt.Printf("  id=%d name=%q\n", product.ID, product.Name)
	fmt.Printf("  current description (%d chars): %s\n",
		len(product.Description), truncate(product.Description, 160))

	fmt.Println("\n=== draft rewrite ===")
	rewrite, err := draftRewrite(ctx, llmBase, llmKey, llmModel, product)
	if err != nil {
		log.Fatalf("draft rewrite: %v", err)
	}
	fmt.Printf("  draft (%d chars): %s\n", len(rewrite), truncate(rewrite, 200))

	fmt.Println("\n=== land issue ===")
	issueID, err := createIssue(ctx, daemonURL, daemonToken, product, rewrite)
	if err != nil {
		log.Fatalf("create issue: %v", err)
	}
	fmt.Println("  issue:", issueID)
	fmt.Println("  status: in_review")
	fmt.Printf("  view at: %s/v1/issues/%s\n", daemonURL, issueID)
}

// --- MCP helpers --------------------------------------------------------

type abilityEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error,omitempty"`
}

func callAbility(
	ctx context.Context,
	client *mcp.Client,
	ability string,
	params map[string]any,
	out any,
) error {
	res, err := client.CallTool(ctx, "mcp-adapter-execute-ability", map[string]any{
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

func pickFirstProduct(ctx context.Context, client *mcp.Client) (int, error) {
	var listOut struct {
		Products []productSummary `json:"products"`
	}
	if err := callAbility(ctx, client, "wooagent-products/list",
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

func getProduct(ctx context.Context, client *mcp.Client, id int) (product, error) {
	var p product
	if err := callAbility(ctx, client, "wooagent-products/get",
		map[string]any{"id": id}, &p); err != nil {
		return p, err
	}
	if p.ID == 0 {
		p.ID = id // ability returned a thinner shape; preserve the id we asked for
	}
	return p, nil
}

// --- LLM ---------------------------------------------------------------

const systemPrompt = `You are a copywriter for a small-batch home-goods store.
Voice: warm, sincere, concrete. Avoid the words "luxe", "premium", "elevate",
"curated". Prefer "small-batch", "handcrafted", "made to last". Lead with the
material or the use, not adjectives. Two to four short sentences, 140-220
characters total. Output ONLY the new description — no preamble, no quotes,
no labels.`

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

func draftRewrite(
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

// --- daemon ------------------------------------------------------------

type daemonProposal struct {
	Type    string         `json:"type"`
	Content string         `json:"content"`
	Target  map[string]any `json:"target,omitempty"`
}

type createIssueBody struct {
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Persona     string          `json:"persona"`
	Status      string          `json:"status"`
	Priority    string          `json:"priority"`
	Proposal    *daemonProposal `json:"proposal"`
}

type createIssueResp struct {
	Issue struct {
		ID string `json:"id"`
	} `json:"issue"`
}

func createIssue(
	ctx context.Context,
	daemonURL, token string,
	p product,
	rewrite string,
) (string, error) {
	body, _ := json.Marshal(createIssueBody{
		Title: fmt.Sprintf("Product description rewrite · %s", p.Name),
		Description: fmt.Sprintf(
			"Drafted by Marketing persona for product #%d (%s).",
			p.ID, p.SKU,
		),
		Persona:  "marketing",
		Status:   "in_review",
		Priority: "medium",
		Proposal: &daemonProposal{
			Type:    "product_description_rewrite",
			Content: rewrite,
			Target: map[string]any{
				"product_id":   p.ID,
				"product_name": p.Name,
				"product_sku":  p.SKU,
				"previous":     p.Description,
			},
		},
	})

	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(cctx, "POST",
		strings.TrimRight(daemonURL, "/")+"/v1/issues", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("daemon http: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(res.Body)
		return "", fmt.Errorf("daemon http %d: %s", res.StatusCode, raw)
	}
	var parsed createIssueResp
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("daemon decode: %w", err)
	}
	return parsed.Issue.ID, nil
}

// --- env helpers -------------------------------------------------------

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var not set: %s", key)
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
