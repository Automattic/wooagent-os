package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeAdapter is a minimal stand-in for the WP MCP Adapter's JSON-RPC
// endpoint. It records the last Authorization header and dispatches
// tools/call to per-name handlers so tests assert both the wire format
// and the auth scheme.
type fakeAdapter struct {
	lastAuth string
	handlers map[string]func(args map[string]any) (string, error)
}

func newFakeAdapter() *fakeAdapter {
	return &fakeAdapter{handlers: map[string]func(map[string]any) (string, error){}}
}

func (f *fakeAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.lastAuth = r.Header.Get("Authorization")
	body, _ := io.ReadAll(r.Body)
	var rpc struct {
		ID     int64           `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	_ = json.Unmarshal(body, &rpc)

	switch rpc.Method {
	case "initialize":
		w.Header().Set("Mcp-Session-Id", "test-session")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      rpc.ID,
			"result": map[string]any{
				"protocolVersion": protocolVersion,
				"serverInfo":      map[string]any{"name": "fake", "version": "0"},
			},
		})
	case "notifications/initialized":
		w.WriteHeader(http.StatusOK)
	case "tools/call":
		var p struct {
			Name      string                 `json:"name"`
			Arguments map[string]any         `json:"arguments"`
		}
		_ = json.Unmarshal(rpc.Params, &p)
		h, ok := f.handlers[p.Name]
		if !ok {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": rpc.ID,
				"error": map[string]any{"code": -32601, "message": "no handler for " + p.Name},
			})
			return
		}
		text, err := h(p.Arguments)
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": rpc.ID,
				"result": map[string]any{
					"isError": true,
					"content": []map[string]any{{"type": "text", "text": err.Error()}},
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": rpc.ID,
			"result": map[string]any{
				"content": []map[string]any{{"type": "text", "text": text}},
			},
		})
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

// DiscoverAbilities accepts both shapes the adapter might return: a
// {"success", "data"} envelope wrapping an {"abilities": [...]} object,
// or a bare array.
func TestDiscoverAbilities_AcceptsBothShapes(t *testing.T) {
	cases := []struct {
		name    string
		payload string
	}{
		{
			name: "envelope_with_object",
			payload: `{"success":true,"data":{"abilities":[
				{"name":"a","description":"A"},
				{"name":"b","description":"B"}
			]}}`,
		},
		{
			name:    "bare_array",
			payload: `[{"name":"a","description":"A"},{"name":"b","description":"B"}]`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fa := newFakeAdapter()
			fa.handlers[ToolDiscoverAbilities] = func(_ map[string]any) (string, error) {
				return c.payload, nil
			}
			ts := httptest.NewServer(fa)
			defer ts.Close()

			c := NewClient(Config{Endpoint: ts.URL, BearerToken: "tok"})
			if _, err := c.Initialize(context.Background()); err != nil {
				t.Fatalf("init: %v", err)
			}
			abs, err := c.DiscoverAbilities(context.Background())
			if err != nil {
				t.Fatalf("discover: %v", err)
			}
			if len(abs) != 2 || abs[0].Name != "a" || abs[1].Name != "b" {
				t.Errorf("got %+v", abs)
			}
		})
	}
}

// GetAbilityInfo unwraps the envelope and falls back to the requested
// name when the server omits it from the payload.
func TestGetAbilityInfo_UnwrapsEnvelope(t *testing.T) {
	fa := newFakeAdapter()
	fa.handlers[ToolGetAbilityInfo] = func(args map[string]any) (string, error) {
		// Echo the requested name back. Permissions left in two orders to
		// confirm we don't sort here — that's the runner's responsibility.
		return `{"success":true,"data":{
			"description":"a description",
			"input_schema":{"type":"object"},
			"permissions":["x","y"]
		}}`, nil
	}
	ts := httptest.NewServer(fa)
	defer ts.Close()

	c := NewClient(Config{Endpoint: ts.URL, BearerToken: "tok"})
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	info, err := c.GetAbilityInfo(context.Background(), "wooagent-products/list")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if info.Name != "wooagent-products/list" {
		t.Errorf("name fallback failed: %q", info.Name)
	}
	if info.Description != "a description" {
		t.Errorf("description=%q", info.Description)
	}
	if len(info.InputSchema) == 0 {
		t.Errorf("input_schema missing")
	}
}

// BearerToken takes precedence over Username/Password — the device-pair
// path must use Authorization: Bearer, not Basic. (Basic only reappears
// when BearerToken is empty.)
func TestAuthScheme_BearerWinsOverBasic(t *testing.T) {
	fa := newFakeAdapter()
	fa.handlers[ToolDiscoverAbilities] = func(_ map[string]any) (string, error) {
		return `[]`, nil
	}
	ts := httptest.NewServer(fa)
	defer ts.Close()

	c := NewClient(Config{
		Endpoint:    ts.URL,
		Username:    "user", Password: "pw",
		BearerToken: "device-token",
	})
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := c.DiscoverAbilities(context.Background()); err != nil {
		t.Fatalf("discover: %v", err)
	}
	if !strings.HasPrefix(fa.lastAuth, "Bearer ") {
		t.Errorf("auth=%q, want Bearer-prefixed", fa.lastAuth)
	}
	if !strings.Contains(fa.lastAuth, "device-token") {
		t.Errorf("token not on the wire: %q", fa.lastAuth)
	}
}

// Basic auth is still used when only Username/Password are set — the
// pre-pair fallback path the original mcp-probe relied on.
func TestAuthScheme_BasicWhenNoBearer(t *testing.T) {
	fa := newFakeAdapter()
	fa.handlers[ToolDiscoverAbilities] = func(_ map[string]any) (string, error) {
		return `[]`, nil
	}
	ts := httptest.NewServer(fa)
	defer ts.Close()

	c := NewClient(Config{Endpoint: ts.URL, Username: "user", Password: "pw"})
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := c.DiscoverAbilities(context.Background()); err != nil {
		t.Fatalf("discover: %v", err)
	}
	if !strings.HasPrefix(fa.lastAuth, "Basic ") {
		t.Errorf("auth=%q, want Basic-prefixed", fa.lastAuth)
	}
}

// A failed envelope (success=false) surfaces as a Go error so callers
// don't accidentally cache a malformed schema.
func TestDiscoverAbilities_FailedEnvelope_ReturnsError(t *testing.T) {
	fa := newFakeAdapter()
	fa.handlers[ToolDiscoverAbilities] = func(_ map[string]any) (string, error) {
		return `{"success":false,"data":{},"error":"plugin disabled"}`, nil
	}
	ts := httptest.NewServer(fa)
	defer ts.Close()

	c := NewClient(Config{Endpoint: ts.URL, BearerToken: "tok"})
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := c.DiscoverAbilities(context.Background()); err == nil {
		t.Errorf("expected error on success=false")
	}
}

func TestCallTool_SessionLost_ReturnsSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mimic the server's "invalid session" response shape.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":-32000,"message":"invalid or expired session"}}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Endpoint: srv.URL})
	// Bypass Initialize: set a stale session id directly so the next CallTool
	// hits the "session invalid" server response.
	c.sessionID = "stale-session"
	_, err := c.CallTool(context.Background(), "any", nil)
	if !errors.Is(err, ErrSessionLost) {
		t.Fatalf("want ErrSessionLost, got %v", err)
	}
}

func TestCallTool_ConnectionRefused_ReturnsTransportSentinel(t *testing.T) {
	// Closed listener — Dial will fail.
	c := NewClient(Config{Endpoint: "http://127.0.0.1:1"})
	_, err := c.Initialize(context.Background())
	if !errors.Is(err, ErrTransport) {
		t.Fatalf("want ErrTransport, got %v", err)
	}
}
