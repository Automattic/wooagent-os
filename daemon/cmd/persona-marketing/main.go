// Command persona-marketing is a debug-only one-shot for the Marketing
// persona. The daemon's `wooagent run` already spawns this persona
// automatically on startup (see internal/cli/run.go); this binary stays
// because it's useful for iterating on the marketing prompt without
// restarting the daemon.
//
// Both the binary and the daemon-startup path drive the same code in
// internal/personas/marketing/ — fix bugs there, not here.
//
// Run:
//
//	WOOAGENT_MCP_URL=https://<store>/wp-json/mcp/mcp-adapter-default-server \
//	WOOAGENT_MCP_USER=<wp-user-or-email> \
//	WOOAGENT_MCP_APP_PASSWORD=<wp-application-password> \
//	go run ./cmd/persona-marketing
//
// LLM routing: when ANTHROPIC_API_KEY is set, calls Claude (default
// claude-sonnet-4-6, override with ANTHROPIC_MODEL). Otherwise falls back
// to the OpenAI-compatible endpoint — OPENAI_API_BASE_URL, OPENAI_MODEL,
// OPENAI_API_KEY override the LM Studio defaults.
//
// Optional: PERSONA_PRODUCT_ID picks a specific product.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/wooagent-os/wooagent-os/daemon/internal/config"
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"

	// Side-effect import: registers Marketing in the personas registry.
	_ "github.com/wooagent-os/wooagent-os/daemon/internal/personas/marketing"
)

const personaSlug = "marketing"

func main() {
	ctx := context.Background()

	mcpURL := mustEnv("WOOAGENT_MCP_URL")
	mcpUser := mustEnv("WOOAGENT_MCP_USER")
	mcpPass := strings.ReplaceAll(mustEnv("WOOAGENT_MCP_APP_PASSWORD"), " ", "")

	paths, err := config.DefaultPaths()
	if err != nil {
		log.Fatalf("resolve paths: %v", err)
	}
	st, err := store.Open(ctx, paths.DBFile)
	if err != nil {
		log.Fatalf("open store at %s: %v (did you run `wooagent init`?)", paths.DBFile, err)
	}
	defer st.Close()

	mcpClient := mcp.NewClient(mcp.Config{
		Endpoint: mcpURL,
		Username: mcpUser,
		Password: mcpPass,
	})

	productID := 0
	if v := strings.TrimSpace(os.Getenv("PERSONA_PRODUCT_ID")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			productID = n
		}
	}

	deps := personas.Deps{
		Store: st,
		MCP:   mcpClient,
		Env: personas.Env{
			AnthropicAPIKey:   os.Getenv("ANTHROPIC_API_KEY"),
			AnthropicModel:    os.Getenv("ANTHROPIC_MODEL"),
			OpenAIAPIBase:     os.Getenv("OPENAI_API_BASE_URL"),
			OpenAIAPIKey:      os.Getenv("OPENAI_API_KEY"),
			OpenAIModel:       os.Getenv("OPENAI_MODEL"),
			ProductIDOverride: productID,
		},
	}

	p, ok := personas.Lookup(personaSlug)
	if !ok {
		log.Fatalf("persona %q not registered", personaSlug)
	}
	fmt.Printf("=== %s · %s ===\n", p.Slug(), p.DisplayName())

	res, err := personas.RunAndPersist(ctx, p, deps)
	if err != nil {
		log.Fatalf("run persona: %v", err)
	}
	if res.Skipped {
		fmt.Printf("  skipped: %s\n", res.SkipReason)
		fmt.Println("  exit cleanly without creating an issue")
		return
	}
	fmt.Println("  ok · issue:", res.IssueID)
	fmt.Println("  status: in_review")
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var not set: %s", key)
	}
	return v
}
