// Command spike-mcp is the Phase 1 round-trip spike: ADK-Go agent loop
// driving a real MCP tool against a live WooCommerce store via the
// WordPress MCP Adapter's three-meta-tool pattern.
//
// This is the first experiment that proves the full daemon chain works
// end-to-end: model → ADK orchestrator → MCP client → wp_register_ability →
// WooCommerce → back to the model. It replaces spike-adk's stub tool with
// a real list_products call routed through mcp-adapter-execute-ability
// against wooagent-products/list.
//
// Run (LM Studio with gemma + test store credentials from the operator's
// environment; never commit credentials here):
//
//	OPENAI_API_BASE_URL=http://localhost:1234/v1 \
//	OPENAI_MODEL=google/gemma-4-e4b \
//	WOOAGENT_MCP_URL=https://<your-store>/wp-json/mcp/mcp-adapter-default-server \
//	WOOAGENT_MCP_USER=<wp-user-or-email> \
//	WOOAGENT_MCP_APP_PASSWORD=<wp-application-password> \
//	go run ./cmd/spike-mcp
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	adkopenai "github.com/huytd/adk-openai-go"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/genai"

	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
)

func main() {
	ctx := context.Background()

	mcpURL := mustEnv("WOOAGENT_MCP_URL")
	mcpUser := mustEnv("WOOAGENT_MCP_USER")
	// App passwords from wp-admin are shown with spaces for readability;
	// WordPress strips them at auth time. We do the same so operators can
	// paste either form.
	mcpPass := strings.ReplaceAll(mustEnv("WOOAGENT_MCP_APP_PASSWORD"), " ", "")

	baseURL := envOr("OPENAI_API_BASE_URL", "http://localhost:1234/v1")
	modelName := envOr("OPENAI_MODEL", "google/gemma-4-e4b")

	mcpClient := mcp.NewClient(mcp.Config{
		Endpoint: mcpURL,
		Username: mcpUser,
		Password: mcpPass,
	})

	fmt.Println("=== MCP handshake ===")
	serverInfo, err := mcpClient.Initialize(ctx)
	if err != nil {
		log.Fatalf("mcp initialize: %v", err)
	}
	fmt.Printf("  server: %s %s (protocol %s)\n",
		serverInfo.ServerInfo.Name,
		serverInfo.ServerInfo.Version,
		serverInfo.ProtocolVersion,
	)
	fmt.Printf("  session: %s\n", mcpClient.SessionID())

	model := adkopenai.NewModel(modelName, &adkopenai.Config{
		APIKey:  envOr("OPENAI_API_KEY", "lm-studio"),
		BaseURL: baseURL,
	})

	listProductsTool, err := functiontool.New(functiontool.Config{
		Name: "list_products",
		Description: "List products from the connected WooCommerce store. " +
			"Returns an array of products with id, name, sku, status, and " +
			"the total count. Call this when the operator asks about what " +
			"products exist in the store. Prefer small per_page values (3-10).",
	}, listProductsFor(ctx, mcpClient))
	if err != nil {
		log.Fatalf("functiontool.New: %v", err)
	}

	marketingAgent, err := llmagent.New(llmagent.Config{
		Name:        "marketing_mcp_spike",
		Model:       model,
		Description: "WooAgent OS Phase 1 MCP round-trip spike.",
		Instruction: "You are a marketing assistant for a WooCommerce store. " +
			"When the operator asks what products exist or asks for a summary, " +
			"call list_products. Use only fields returned by the tool — do " +
			"not invent product details, prices, or descriptions. If the " +
			"operator asks about a specific product by id, you can still " +
			"call list_products and filter the result.",
		Tools: []tool.Tool{listProductsTool},
	})
	if err != nil {
		log.Fatalf("llmagent.New: %v", err)
	}

	r, err := runner.New(runner.Config{
		AppName:           "wooagent-mcp-spike",
		Agent:             marketingAgent,
		SessionService:    session.InMemoryService(),
		AutoCreateSession: true,
	})
	if err != nil {
		log.Fatalf("runner.New: %v", err)
	}

	prompt := envOr("SPIKE_PROMPT",
		"Give me a quick summary of what's in the store — how many products, and the names of the first few.")
	userMsg := genai.NewContentFromText(prompt, genai.RoleUser)

	fmt.Println()
	fmt.Println("=== agent loop ===")
	fmt.Println("> user:", prompt)
	fmt.Println()

	toolCalled := false
	gotFinalText := false
	for event, err := range r.Run(ctx, "spike-user", "spike-session", userMsg, agent.RunConfig{}) {
		if err != nil {
			log.Fatalf("runner event error: %v", err)
		}
		if event == nil || event.Content == nil {
			continue
		}
		for _, p := range event.Content.Parts {
			switch {
			case p.FunctionCall != nil:
				toolCalled = true
				fmt.Printf("[%s] call  %s(%v)\n", event.Author, p.FunctionCall.Name, p.FunctionCall.Args)
			case p.FunctionResponse != nil:
				fmt.Printf("[%s] reply %s -> %s\n", event.Author, p.FunctionResponse.Name, truncate(fmt.Sprintf("%v", p.FunctionResponse.Response), 400))
			case p.Text != "":
				if event.IsFinalResponse() {
					gotFinalText = true
				}
				fmt.Printf("[%s] %s\n", event.Author, p.Text)
			}
		}
	}

	fmt.Println()
	fmt.Println("=== spike checks ===")
	fmt.Println(" tool invoked by model:", toolCalled)
	fmt.Println(" final text produced :", gotFinalText)
	if !toolCalled || !gotFinalText {
		os.Exit(2)
	}
}

type listProductsIn struct {
	PerPage int `json:"per_page,omitempty"`
}

type product struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	SKU    string `json:"sku"`
	Status string `json:"status"`
}

type listProductsOut struct {
	Products []product `json:"products"`
	Total    int       `json:"total"`
}

// listProductsFor returns an ADK tool function whose implementation calls
// mcp-adapter-execute-ability against the wooagent-products/list ability.
// This is the core swap vs. spike-adk: the function body now hits a real
// store instead of returning a hardcoded Stub Tee.
func listProductsFor(ctx context.Context, client *mcp.Client) func(tool.Context, listProductsIn) (listProductsOut, error) {
	return func(_ tool.Context, in listProductsIn) (listProductsOut, error) {
		if in.PerPage <= 0 {
			in.PerPage = 5
		}
		result, err := client.CallTool(ctx, "mcp-adapter-execute-ability", map[string]any{
			"ability_name": "wooagent-products/list",
			"parameters":   map[string]any{"per_page": in.PerPage},
		})
		if err != nil {
			return listProductsOut{}, fmt.Errorf("mcp call: %w", err)
		}
		if len(result.Content) == 0 {
			return listProductsOut{}, fmt.Errorf("mcp returned no content")
		}
		// The ability output is wrapped by the adapter in a text part
		// whose body is JSON: {success, data, error}.
		var envelope struct {
			Success bool            `json:"success"`
			Data    listProductsOut `json:"data"`
			Error   string          `json:"error,omitempty"`
		}
		if err := json.Unmarshal([]byte(result.Content[0].Text), &envelope); err != nil {
			return listProductsOut{}, fmt.Errorf("decode ability envelope: %w  body=%s", err, result.Content[0].Text)
		}
		if !envelope.Success {
			return listProductsOut{}, fmt.Errorf("ability failed: %s", envelope.Error)
		}
		return envelope.Data, nil
	}
}

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
