// Command persona-pricing is a debug-only one-shot for the Pricing
// persona. The daemon's `wooagent run` already spawns this persona
// automatically on startup (see internal/cli/run.go); this binary stays
// because it's useful for iterating on the pricing skill or harness
// without restarting the daemon, and because it prints richer diagnostic
// output than the daemon's per-persona log line.
//
// Both the binary and the daemon-startup path drive the same code in
// internal/personas/pricing/ — fix bugs there, not here.
//
// Run:
//
//	WOOAGENT_MCP_URL=https://<store>/wp-json/mcp/mcp-adapter-default-server \
//	WOOAGENT_MCP_USER=<wp-user-or-email> \
//	WOOAGENT_MCP_APP_PASSWORD=<wp-application-password> \
//	ANTHROPIC_API_KEY=<sk-ant-...> \
//	go run ./cmd/persona-pricing
//
// Optional: PERSONA_PRODUCT_ID picks a specific product. PERSONA_CURRENCY
// (default USD) labels the proposal. ANTHROPIC_MODEL overrides the
// default Haiku 4.5 model id. WOOAGENT_SKILLS_DIR overrides the skill
// registry path.
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
	"github.com/wooagent-os/wooagent-os/daemon/internal/registry"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"

	// Side-effect import: registers Pricing in the personas registry.
	_ "github.com/wooagent-os/wooagent-os/daemon/internal/personas/pricing"
)

const personaSlug = "pricing"

func main() {
	ctx := context.Background()

	mcpURL := mustEnv("WOOAGENT_MCP_URL")
	mcpUser := mustEnv("WOOAGENT_MCP_USER")
	mcpPass := strings.ReplaceAll(mustEnv("WOOAGENT_MCP_APP_PASSWORD"), " ", "")
	apiKey := mustEnv("ANTHROPIC_API_KEY")

	skillsDir := envOr("WOOAGENT_SKILLS_DIR", "skills")
	skills, err := registry.LoadSkills(skillsDir)
	if err != nil {
		log.Fatalf("load skills (%s): %v", skillsDir, err)
	}

	// Open the daemon's SQLite directly. This means the binary writes
	// issues to the same DB the running daemon serves — no HTTP
	// round-trip, no separate auth dance. Concurrent access is safe under
	// modernc.org/sqlite's WAL mode.
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
		Store:  st,
		MCP:    mcpClient,
		Skills: skills,
		Env: personas.Env{
			AnthropicAPIKey:   apiKey,
			AnthropicModel:    os.Getenv("ANTHROPIC_MODEL"),
			DefaultCurrency:   envOr("PERSONA_CURRENCY", "USD"),
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

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
