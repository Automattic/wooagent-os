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
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ErrSessionLost means the MCP server rejected the request because its
// session id is unknown or expired. The scheduler treats this as transient
// — a re-Initialize on the next attempt typically recovers.
var ErrSessionLost = errors.New("mcp: session lost")

// ErrTransport means the JSON-RPC envelope never made it to the server
// (network error, TLS error, connection refused, etc.) or the response
// wasn't valid JSON. Scheduler-side, this is transient.
var ErrTransport = errors.New("mcp: transport error")

// Protocol version the client announces on initialize. The server may
// respond with a different version; we do not renegotiate.
const protocolVersion = "2025-06-18"

// Config is the bag of inputs for NewClient. Endpoint is the full URL of
// a single MCP server (e.g. .../wp-json/mcp/mcp-adapter-default-server).
//
// Auth: BearerToken takes precedence when set — that's the device-pair
// token path used by paired stores. Otherwise Username/Password drives
// HTTP Basic Auth (the WordPress Application Password path used by the
// pre-pairing spike + mcp-probe).
type Config struct {
	Endpoint    string
	Username    string
	Password    string
	BearerToken string
	Timeout     time.Duration
}

// Client is a single-session MCP client safe for concurrent use. Concurrent
// Initialize callers are serialized; only the first does the JSON-RPC
// handshake and the rest see the cached session. The session is automatically
// invalidated when the server returns ErrSessionLost, so a follow-up
// Initialize re-handshakes cleanly.
// target is the connection identity: which store this client talks to and
// how it authenticates. Mutable because the operator can re-pair to a
// different store while the daemon runs — see Client.Retarget
// (DSGWOO-1470).
type target struct {
	endpoint string
	username string
	password string
	bearer   string
}

type Client struct {
	// targetMu guards target. Held only long enough to copy the struct,
	// never across a network call.
	targetMu sync.RWMutex
	target   target

	http *http.Client

	// initMu serializes Initialize callers. Held for the whole call, including
	// the HTTP round-trip; that's fine because Initialize fast-paths once a
	// session is established.
	initMu   sync.Mutex
	initInfo ServerInfo // last successful handshake; only Initialize reads/writes

	// sessionMu protects sessionID. Held briefly — never across a network call.
	sessionMu sync.Mutex
	sessionID string

	nextID atomic.Int64
}

func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		target: target{
			endpoint: cfg.Endpoint,
			username: cfg.Username,
			password: cfg.Password,
			bearer:   cfg.BearerToken,
		},
		http: &http.Client{Timeout: timeout},
	}
}

// Endpoint reports the URL this client currently talks to. Callers use it
// to detect that the active connection no longer matches the paired store.
func (c *Client) Endpoint() string {
	c.targetMu.RLock()
	defer c.targetMu.RUnlock()
	return c.target.endpoint
}

// Retarget points the client at a different store. Returns true when the
// target actually changed.
//
// This exists because the daemon holds one *Client for its whole lifetime
// — the PEP, the personas and the Ask Agent tools all share it — while the
// operator can re-pair to a different store at any time. Without this,
// re-pairing had no effect until the daemon was restarted, and every run
// kept calling the old store (DSGWOO-1470).
//
// Any cached session id is dropped: a session established with the old
// store is meaningless to the new one, and keeping it would make the first
// post-retarget call fail with ErrSessionLost.
func (c *Client) Retarget(cfg Config) bool {
	next := target{
		endpoint: cfg.Endpoint,
		username: cfg.Username,
		password: cfg.Password,
		bearer:   cfg.BearerToken,
	}

	c.targetMu.Lock()
	changed := c.target != next
	if changed {
		c.target = next
	}
	c.targetMu.Unlock()

	if changed {
		c.setSessionID("")
	}
	return changed
}

// newRequest builds an authenticated JSON-RPC POST against the current
// target. Both the request/response and notification paths go through
// here so auth and endpoint can never drift apart between them.
func (c *Client) newRequest(ctx context.Context, body []byte) (*http.Request, error) {
	c.targetMu.RLock()
	t := c.target
	c.targetMu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if t.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+t.bearer)
	} else {
		req.SetBasicAuth(t.username, t.password)
	}
	if sid := c.getSessionID(); sid != "" {
		req.Header.Set("Mcp-Session-Id", sid)
	}
	return req, nil
}

// MCP-adapter meta-tool names. The WordPress MCP Adapter exposes a fixed
// set of dispatcher tools via tools/list; the actual abilities (e.g.
// "wooagent-products/list") sit behind these and are enumerated through
// discover/info rather than appearing as MCP tools 1:1.
const (
	ToolDiscoverAbilities = "mcp-adapter-discover-abilities"
	ToolGetAbilityInfo    = "mcp-adapter-get-ability-info"
	ToolExecuteAbility    = "mcp-adapter-execute-ability"
)

// AbilitySummary is one entry in the discover-abilities response — enough
// to render a list and detect adds/removes between discovery passes.
type AbilitySummary struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
}

// AbilityInfo is the detailed shape returned by get-ability-info — the
// full input/output schema we cache and hash to detect schema drift.
type AbilityInfo struct {
	Name            string          `json:"name"`
	Title           string          `json:"title,omitempty"`
	Description     string          `json:"description,omitempty"`
	Version         string          `json:"version,omitempty"`
	InputSchema     json.RawMessage `json:"input_schema,omitempty"`
	OutputSchema    json.RawMessage `json:"output_schema,omitempty"`
	Permissions     []string        `json:"permissions,omitempty"`
	RequiredScopes  []string        `json:"required_scopes,omitempty"`
}

// DiscoverAbilities returns the abilities the paired store exposes. Wraps
// tools/call with name=ToolDiscoverAbilities and unwraps the JSON envelope
// the adapter returns in result.content[0].text.
func (c *Client) DiscoverAbilities(ctx context.Context) ([]AbilitySummary, error) {
	res, err := c.CallTool(ctx, ToolDiscoverAbilities, map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("discover-abilities: %w", err)
	}
	if len(res.Content) == 0 {
		return nil, fmt.Errorf("discover-abilities: empty content")
	}
	// The adapter wraps the payload as {"success": bool, "data": {...},
	// "error": "..."} inside the first text part. Tolerate both that
	// envelope and a raw [{...}] for portability against alt servers.
	raw := []byte(res.Content[0].Text)
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   string          `json:"error,omitempty"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && envelope.Data != nil {
		if !envelope.Success {
			return nil, fmt.Errorf("discover-abilities: %s", envelope.Error)
		}
		raw = envelope.Data
	}
	// data may be {"abilities": [...]} or [...] — accept either.
	var asObject struct {
		Abilities []AbilitySummary `json:"abilities"`
	}
	if err := json.Unmarshal(raw, &asObject); err == nil && asObject.Abilities != nil {
		return asObject.Abilities, nil
	}
	var asArray []AbilitySummary
	if err := json.Unmarshal(raw, &asArray); err != nil {
		return nil, fmt.Errorf("discover-abilities: decode payload: %w", err)
	}
	return asArray, nil
}

// GetAbilityInfo returns the full schema for one ability. Same envelope
// handling as DiscoverAbilities.
func (c *Client) GetAbilityInfo(ctx context.Context, name string) (AbilityInfo, error) {
	res, err := c.CallTool(ctx, ToolGetAbilityInfo, map[string]any{"ability_name": name})
	if err != nil {
		return AbilityInfo{}, fmt.Errorf("get-ability-info %s: %w", name, err)
	}
	if len(res.Content) == 0 {
		return AbilityInfo{}, fmt.Errorf("get-ability-info %s: empty content", name)
	}
	raw := []byte(res.Content[0].Text)
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   string          `json:"error,omitempty"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && envelope.Data != nil {
		if !envelope.Success {
			return AbilityInfo{}, fmt.Errorf("get-ability-info %s: %s", name, envelope.Error)
		}
		raw = envelope.Data
	}
	var info AbilityInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return AbilityInfo{}, fmt.Errorf("get-ability-info %s: decode: %w", name, err)
	}
	if info.Name == "" {
		info.Name = name
	}
	return info, nil
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

// Initialize performs the MCP handshake. Safe for concurrent callers and
// idempotent — only the first caller does the network round-trip; subsequent
// callers see the cached ServerInfo until the session is invalidated. PEP
// and the personas may call this on every dispatch without paying handshake
// cost beyond the first.
func (c *Client) Initialize(ctx context.Context) (ServerInfo, error) {
	c.initMu.Lock()
	defer c.initMu.Unlock()

	if c.getSessionID() != "" {
		return c.initInfo, nil
	}

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
	c.initInfo = info
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
// before Initialize completes, or after the server returns ErrSessionLost.
func (c *Client) SessionID() string { return c.getSessionID() }

func (c *Client) getSessionID() string {
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()
	return c.sessionID
}

func (c *Client) setSessionID(sid string) {
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()
	c.sessionID = sid
}

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
	req, err := c.newRequest(ctx, body)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return wrapTransportError(method, err)
	}
	defer resp.Body.Close()

	if captureSession {
		if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
			c.setSessionID(sid)
		}
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return wrapTransportError(method, err)
	}
	if resp.StatusCode >= 400 {
		// Attempt to parse a JSON-RPC error body even on HTTP error status —
		// some servers (e.g. the WP MCP Adapter) return 400 with a valid
		// JSON-RPC error envelope for session-related rejections.
		var rpcResp rpcResponse
		if jsonErr := json.Unmarshal(raw, &rpcResp); jsonErr == nil && rpcResp.Error != nil {
			return c.handleEnvelopeError(method, rpcResp.Error.Message)
		}
		return NewStatusError(c.Endpoint(), resp.StatusCode, raw)
	}
	var rpcResp rpcResponse
	if err := json.Unmarshal(raw, &rpcResp); err != nil {
		return wrapTransportError(method, fmt.Errorf("decode response: %w  body=%s", err, string(raw)))
	}
	if rpcResp.Error != nil {
		return c.handleEnvelopeError(method, rpcResp.Error.Message)
	}
	if out != nil && len(rpcResp.Result) > 0 {
		if err := json.Unmarshal(rpcResp.Result, out); err != nil {
			return fmt.Errorf("decode result: %w", err)
		}
	}
	return nil
}

func wrapTransportError(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w: %v", op, ErrTransport, err)
}

func wrapEnvelopeError(op, jsonRPCError string) error {
	lower := strings.ToLower(jsonRPCError)
	if strings.Contains(lower, "session") && (strings.Contains(lower, "expired") || strings.Contains(lower, "invalid")) {
		return fmt.Errorf("%s: %w: %s", op, ErrSessionLost, jsonRPCError)
	}
	return fmt.Errorf("%s: %s", op, jsonRPCError)
}

// handleEnvelopeError wraps a JSON-RPC error message and, if it classifies as
// ErrSessionLost, clears the cached session id so the next Initialize call
// triggers a fresh handshake. Scheduler-side retries rely on this: the
// classifier returns FailureTransient on ErrSessionLost, and a subsequent
// dispatch re-Initializes via the now-empty session.
func (c *Client) handleEnvelopeError(op, jsonRPCError string) error {
	err := wrapEnvelopeError(op, jsonRPCError)
	if errors.Is(err, ErrSessionLost) {
		c.setSessionID("")
	}
	return err
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
	req, err := c.newRequest(ctx, body)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}
