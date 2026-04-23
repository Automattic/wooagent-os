// Command mcp-probe is a throwaway verification tool that exercises the
// mcp.Client against a live store without involving the ADK agent loop.
// Used to isolate MCP-transport bugs from model-behavior bugs.
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

func main() {
	ctx := context.Background()

	endpoint := mustEnv("WOOAGENT_MCP_URL")
	user := mustEnv("WOOAGENT_MCP_USER")
	pass := strings.ReplaceAll(mustEnv("WOOAGENT_MCP_APP_PASSWORD"), " ", "")

	c := mcp.NewClient(mcp.Config{Endpoint: endpoint, Username: user, Password: pass})

	info, err := c.Initialize(ctx)
	if err != nil {
		log.Fatalf("initialize: %v", err)
	}
	fmt.Printf("ok initialize   server=%s v%s  protocol=%s  session=%s\n",
		info.ServerInfo.Name, info.ServerInfo.Version, info.ProtocolVersion, c.SessionID())

	result, err := c.CallTool(ctx, "mcp-adapter-execute-ability", map[string]any{
		"ability_name": "wooagent-products/list",
		"parameters":   map[string]any{"per_page": 3},
	})
	if err != nil {
		log.Fatalf("call tool: %v", err)
	}
	if len(result.Content) == 0 {
		log.Fatal("no content returned")
	}

	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   string          `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &envelope); err != nil {
		log.Fatalf("decode envelope: %v", err)
	}
	if !envelope.Success {
		log.Fatalf("ability failed: %s", envelope.Error)
	}

	pretty, _ := json.MarshalIndent(json.RawMessage(envelope.Data), "", "  ")
	fmt.Println("ok tools/call   wooagent-products/list  ↓")
	fmt.Println(string(pretty))
	os.Exit(0)
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("required env var not set: %s", k)
	}
	return v
}
