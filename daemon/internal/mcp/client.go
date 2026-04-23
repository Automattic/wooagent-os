// Package mcp implements a minimal MCP (Model Context Protocol) client for
// the HTTP+JSON-RPC transport that WordPress's MCP Adapter serves. It is
// scoped to what the Phase 1 spike needs: initialize, session tracking,
// and tools/call. The wider set (resources, prompts, SSE streaming,
// OAuth 2.1) will land as those features become load-bearing.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

// Protocol version the client announces on initialize. The server may
// respond with a different version; we do not renegotiate.
const protocolVersion = "2025-06-18"

// Config is the bag of inputs for NewClient. Endpoint is the full URL of
// a single MCP server (e.g. .../wp-json/mcp/mcp-adapter-default-server).
// Username/Password drive HTTP Basic Auth — the WordPress Application
// Password path for v0.1. Device-pair tokens (§11.4) slot in here later
// as a different AuthScheme.
type Config struct {
	Endpoint string
	Username string
	Password string
	Timeout  time.Duration
}

// Client is a single-session MCP client. Not safe for concurrent Initialize,
// but tools/call is safe after Initialize returns.
type Client struct {
	endpoint  string
	username  string
	password  string
	http      *http.Client
	sessionID string
	nextID    atomic.Int64
}

func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		endpoint: cfg.Endpoint,
		username: cfg.Username,
		password: cfg.Password,
		http:     &http.Client{Timeout: timeout},
	}
}

// ServerInfo is what the server reports on initialize.
type ServerInfo struct {
	ProtocolVersion string `json:"protocolVersion"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
	Instructions string `json:"instructions,omitempty"`
}

// ToolCallResult is the envelope the server returns from tools/call. The
// interesting payload is usually a single text part whose body is the
// ability's JSON output.
type ToolCallResult struct {
	Content []ContentPart `json:"content"`
	IsError bool          `json:"isError"`
}

type ContentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Initialize performs the MCP handshake. Must be called once before
// CallTool. Populates the session id captured from the response header.
func (c *Client) Initialize(ctx context.Context) (ServerInfo, error) {
	var info ServerInfo
	err := c.doRequest(ctx, "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "wooagent-os",
			"version": "0.1-spike",
		},
	}, &info, true)
	if err != nil {
		return ServerInfo{}, fmt.Errorf("initialize: %w", err)
	}
	if err := c.doNotification(ctx, "notifications/initialized", nil); err != nil {
		return ServerInfo{}, fmt.Errorf("notifications/initialized: %w", err)
	}
	return info, nil
}

// CallTool invokes a tool by name with the given arguments. The result is
// the raw tools/call envelope; callers unwrap the content payload.
func (c *Client) CallTool(ctx context.Context, name string, args any) (ToolCallResult, error) {
	var result ToolCallResult
	err := c.doRequest(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	}, &result, false)
	if err != nil {
		return ToolCallResult{}, err
	}
	if result.IsError {
		msg := "unknown error"
		if len(result.Content) > 0 {
			msg = result.Content[0].Text
		}
		return result, fmt.Errorf("tool %q returned error: %s", name, msg)
	}
	return result, nil
}

// SessionID returns the session id captured during Initialize. Empty
// before Initialize completes.
func (c *Client) SessionID() string { return c.sessionID }

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *Client) doRequest(ctx context.Context, method string, params any, out any, captureSession bool) error {
	id := c.nextID.Add(1)
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.SetBasicAuth(c.username, c.password)
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if captureSession {
		if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
			c.sessionID = sid
		}
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("http %d: %s", resp.StatusCode, string(raw))
	}
	var rpcResp rpcResponse
	if err := json.Unmarshal(raw, &rpcResp); err != nil {
		return fmt.Errorf("decode response: %w  body=%s", err, string(raw))
	}
	if rpcResp.Error != nil {
		return fmt.Errorf("jsonrpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	if out != nil && len(rpcResp.Result) > 0 {
		if err := json.Unmarshal(rpcResp.Result, out); err != nil {
			return fmt.Errorf("decode result: %w", err)
		}
	}
	return nil
}

func (c *Client) doNotification(ctx context.Context, method string, params any) error {
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.SetBasicAuth(c.username, c.password)
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}
