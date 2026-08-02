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
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcpresolve"
	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
	"github.com/wooagent-os/wooagent-os/daemon/internal/registry"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"

	// Side-effect import: registers Pricing in the personas registry.
	_ "github.com/wooagent-os/wooagent-os/daemon/internal/personas/pricing"
)

const personaSlug = "pricing"

func main() {
	ctx := context.Background()

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

	// Resolve the same store the daemon would use: the paired store wins,
	// env vars are the fallback. Building a client straight from
	// WOOAGENT_MCP_URL here is how this binary could draft proposals about
	// a store the daemon isn't paired to — and since it writes into the
	// daemon's SQLite, approving one of those applies the wrong store's
	// copy or prices. See mcpresolve.OpenClient.
	mcpClient, mcpTarget, ok := mcpresolve.OpenClient(ctx, st.DB, os.Stderr)
	if !ok {
		log.Fatalf("no store to talk to: pair a store through the UI, or set " +
			"WOOAGENT_MCP_URL, WOOAGENT_MCP_USER and WOOAGENT_MCP_APP_PASSWORD")
	}
	fmt.Printf("=== store: %s (%s) ===\n", mcpTarget.Label, mcpTarget.Source)

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
