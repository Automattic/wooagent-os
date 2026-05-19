// Command probe-cold-draft is a one-shot diagnostic that calls the
// wooagent-products/list MCP ability and prints the raw response shape +
// what pickColdDraftCandidates would return. No DB writes, no LLM calls,
// no gates. Use when the cold-draft batch path silently falls through and
// the per-tick logging hasn't been deployed yet.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
)

type product struct {
	ID                     int    `json:"id"`
	Name                   string `json:"name"`
	SKU                    string `json:"sku"`
	Status                 string `json:"status"`
	DescriptionLength      int    `json:"description_length"`
	ShortDescriptionLength int    `json:"short_description_length"`
	Description            string `json:"description"`
	ShortDescription       string `json:"short_description"`
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error,omitempty"`
}

func main() {
	ctx := context.Background()

	mcpURL := mustEnv("WOOAGENT_MCP_URL")
	mcpUser := mustEnv("WOOAGENT_MCP_USER")
	mcpPass := strings.ReplaceAll(mustEnv("WOOAGENT_MCP_APP_PASSWORD"), " ", "")

	client := mcp.NewClient(mcp.Config{
		Endpoint: mcpURL,
		Username: mcpUser,
		Password: mcpPass,
	})

	if _, err := client.Initialize(ctx); err != nil {
		log.Fatalf("mcp init: %v", err)
	}

	res, err := client.CallTool(ctx, "mcp-adapter-execute-ability", map[string]any{
		"ability_name": "wooagent-products/list",
		"parameters": map[string]any{
			"per_page": 100,
			"orderby":  "date_modified",
			"order":    "asc",
		},
	})
	if err != nil {
		log.Fatalf("CallTool: %v", err)
	}
	if len(res.Content) == 0 {
		log.Fatalf("empty content")
	}

	var env envelope
	if err := json.Unmarshal([]byte(res.Content[0].Text), &env); err != nil {
		log.Fatalf("decode envelope: %v", err)
	}
	if !env.Success {
		log.Fatalf("ability failed: %s", env.Error)
	}

	var out struct {
		Products []product `json:"products"`
	}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		log.Fatalf("decode products: %v", err)
	}

	fmt.Printf("=== wooagent-products/list returned %d products ===\n\n", len(out.Products))

	emptyShort := 0
	emptyLong := 0
	emptyAny := 0
	emptyBoth := 0
	for _, p := range out.Products {
		s := p.ShortDescriptionLength == 0
		l := p.DescriptionLength == 0
		if s {
			emptyShort++
		}
		if l {
			emptyLong++
		}
		if s || l {
			emptyAny++
		}
		if s && l {
			emptyBoth++
		}
	}
	fmt.Printf("Aggregate (per the length fields the picker uses):\n")
	fmt.Printf("  short_description_length == 0: %d\n", emptyShort)
	fmt.Printf("  description_length == 0:       %d\n", emptyLong)
	fmt.Printf("  at least one == 0 (cold-draft candidates):  %d\n", emptyAny)
	fmt.Printf("  both == 0:                                   %d\n\n", emptyBoth)

	fmt.Printf("Cold-draft candidates (published, at least one empty, first 10):\n")
	shown := 0
	for _, p := range out.Products {
		if shown >= 10 {
			break
		}
		if p.Status != "publish" && p.Status != "" {
			continue
		}
		if p.DescriptionLength == 0 || p.ShortDescriptionLength == 0 {
			fmt.Printf("  #%d %q (status=%s, short_len=%d, long_len=%d)\n",
				p.ID, p.Name, p.Status, p.ShortDescriptionLength, p.DescriptionLength)
			shown++
		}
	}
	if shown == 0 {
		fmt.Println("  (none — picker returns 0 candidates)")
	}

	fmt.Printf("\nRaw first-3 product entries (full JSON, including string description fields):\n")
	for i, p := range out.Products {
		if i >= 3 {
			break
		}
		blob, _ := json.MarshalIndent(p, "  ", "  ")
		fmt.Printf("  [%d] %s\n", i+1, blob)
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env: %s", key)
	}
	return v
}
