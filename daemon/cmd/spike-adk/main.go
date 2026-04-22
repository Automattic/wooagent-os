// Command spike-adk is the ADK-Go spike from WooAgent_plan.md Phase 1.
// It exists to answer one question: can we drive an ADK-Go agent loop
// with a real model + a stub tool from inside this daemon's Go module?
//
// Initial run targets a local LM Studio model over its OpenAI-compatible
// endpoint. The Anthropic path is wired into go.mod already (Alcova-AI
// adapter) and will slot in once the local loop is green.
//
// The spike is throwaway. The real artifacts are (a) dependency acquisition
// — google.golang.org/adk + OpenAI-compatible + Anthropic adapters are all
// in go.mod — and (b) the notes this run produces. Mon Apr 27's MCP client
// + Marketing persona work consume both.
//
// Run (LM Studio on default port, gemma loaded):
//
//	OPENAI_API_BASE_URL=http://localhost:1234/v1 \
//	OPENAI_MODEL=google/gemma-4-e4b \
//	go run ./cmd/spike-adk
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	adkopenai "github.com/huytd/adk-openai-go"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/genai"
)

func main() {
	ctx := context.Background()

	baseURL := envOr("OPENAI_API_BASE_URL", "http://localhost:1234/v1")
	modelName := envOr("OPENAI_MODEL", "google/gemma-4-e4b")

	// LM Studio ignores the API key but the OpenAI client rejects empty
	// strings unless BaseURL is set; adkopenai reads the env vars if the
	// config fields are empty, so an explicit placeholder keeps intent clear.
	model := adkopenai.NewModel(modelName, &adkopenai.Config{
		APIKey:  envOr("OPENAI_API_KEY", "lm-studio"),
		BaseURL: baseURL,
	})

	productTool, err := functiontool.New(functiontool.Config{
		Name: "get_product_stub",
		Description: "Fetch a single product by numeric ID. Returns name, " +
			"price, and description. Use this when the user asks about a " +
			"specific product — do not invent product details.",
	}, getProductStub)
	if err != nil {
		log.Fatalf("functiontool.New: %v", err)
	}

	marketingAgent, err := llmagent.New(llmagent.Config{
		Name:        "marketing_spike",
		Model:       model,
		Description: "WooAgent OS Phase 1 ADK-Go spike: Marketing persona prototype.",
		Instruction: "You are a marketing assistant for a WooCommerce store. " +
			"When the operator asks about a product, call get_product_stub with " +
			"its numeric ID, then write one sentence of friendly marketing copy " +
			"using ONLY the returned fields. Do not invent product details.",
		Tools: []tool.Tool{productTool},
	})
	if err != nil {
		log.Fatalf("llmagent.New: %v", err)
	}

	r, err := runner.New(runner.Config{
		AppName:           "wooagent-spike",
		Agent:             marketingAgent,
		SessionService:    session.InMemoryService(),
		AutoCreateSession: true,
	})
	if err != nil {
		log.Fatalf("runner.New: %v", err)
	}

	userMsg := genai.NewContentFromText(
		"Write a one-sentence marketing line for product 821.",
		genai.RoleUser,
	)

	fmt.Println("=== WooAgent ADK-Go spike ===")
	fmt.Println("> user:", "Write a one-sentence marketing line for product 821.")
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
				fmt.Printf("[%s] reply %s -> %v\n", event.Author, p.FunctionResponse.Name, p.FunctionResponse.Response)
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

// productStubIn / productStubOut define the JSON schema the model sees.
// Field names/tags drive the auto-generated tool schema in ADK-Go.
type productStubIn struct {
	ProductID int `json:"product_id"`
}

type productStubOut struct {
	Name        string `json:"name"`
	Price       string `json:"price"`
	Description string `json:"description"`
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getProductStub(_ tool.Context, in productStubIn) (productStubOut, error) {
	return productStubOut{
		Name:        fmt.Sprintf("Stub Tee #%d", in.ProductID),
		Price:       "29.99",
		Description: "Lightweight cotton tee in heather grey, seeded by the ADK-Go spike harness.",
	}, nil
}
