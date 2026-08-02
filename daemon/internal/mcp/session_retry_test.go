package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// expiringServer models the behaviour that produced DSGWOO-1473: it hands
// out a session on initialize, then declares that session gone on the first
// tools/call, exactly as a WordPress MCP Adapter does once it has expired
// the session out from under a long-lived client.
type expiringServer struct {
	mu sync.Mutex

	// expireAfter is how many tools/call requests succeed on a given
	// session before the server starts rejecting it.
	expireAfter int

	sessions     int      // initialize count
	callsOnSess  int      // tools/call count on the current session
	toolCalls    int      // tools/call count overall
	sessionsSeen []string // Mcp-Session-Id header per tools/call, in order
}

func (s *expiringServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Method string `json:"method"`
			ID     any    `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		s.mu.Lock()
		defer s.mu.Unlock()

		switch body.Method {
		case "initialize":
			s.sessions++
			s.callsOnSess = 0
			w.Header().Set("Mcp-Session-Id", "sid-"+itoa(s.sessions))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"` + protocolVersion + `","serverInfo":{"name":"wp","version":"1"}}}`))

		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)

		case "tools/call":
			s.toolCalls++
			s.callsOnSess++
			s.sessionsSeen = append(s.sessionsSeen, r.Header.Get("Mcp-Session-Id"))
			if s.callsOnSess > s.expireAfter {
				// The exact shape the store returns: a JSON-RPC error whose
				// message names the session and says the request is invalid.
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32600,"message":"Invalid Request: Missing Mcp-Session-Id header."}}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"ok"}]}}`))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func (s *expiringServer) snapshot() (sessions, toolCalls int, seen []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions, s.toolCalls, append([]string(nil), s.sessionsSeen...)
}

// The DSGWOO-1473 scenario end to end: a persona initializes once, then makes
// several tool calls, and the session expires partway through. Every call
// must still succeed.
func TestCallTool_RecoversFromExpiredSession(t *testing.T) {
	srv := &expiringServer{expireAfter: 1} // first call ok, then session gone
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	c := NewClient(Config{Endpoint: ts.URL, BearerToken: "tok"})
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("initial handshake: %v", err)
	}

	// Three tool calls, as a persona's Draft would make.
	for i := 0; i < 3; i++ {
		if _, err := c.CallTool(context.Background(), "wooagent-products/list", map[string]any{}); err != nil {
			t.Fatalf("call %d failed: %v", i+1, err)
		}
	}

	sessions, _, seen := srv.snapshot()
	if sessions < 2 {
		t.Errorf("expected at least one re-handshake, saw %d initialize(s)", sessions)
	}
	// The critical regression: no request may go out with an empty session
	// header. That empty header is what the store reported as
	// "Missing Mcp-Session-Id header".
	for i, sid := range seen {
		if sid == "" {
			t.Errorf("tools/call #%d was sent with no Mcp-Session-Id header", i+1)
		}
	}
}

// Recovery must be one attempt, not a loop, when the server rejects every
// session it issues.
func TestCallTool_RetriesOnceThenGivesUp(t *testing.T) {
	srv := &expiringServer{expireAfter: 0} // every tools/call is rejected
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	c := NewClient(Config{Endpoint: ts.URL, BearerToken: "tok"})
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}

	_, err := c.CallTool(context.Background(), "t", map[string]any{})
	if err == nil {
		t.Fatal("expected the call to fail when no session is ever accepted")
	}
	if !errors.Is(err, ErrSessionLost) {
		t.Errorf("error should still classify as session lost: %v", err)
	}

	_, toolCalls, _ := srv.snapshot()
	if toolCalls != 2 {
		t.Errorf("tools/call attempts = %d, want exactly 2 (original + one retry)", toolCalls)
	}
}

// A healthy session must not trigger extra handshakes — the retry path is
// for recovery, not something every call pays for.
func TestCallTool_HealthySessionDoesNotReinitialize(t *testing.T) {
	srv := &expiringServer{expireAfter: 100}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	c := NewClient(Config{Endpoint: ts.URL, BearerToken: "tok"})
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := c.CallTool(context.Background(), "t", map[string]any{}); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}

	sessions, toolCalls, _ := srv.snapshot()
	if sessions != 1 {
		t.Errorf("initialize count = %d, want 1", sessions)
	}
	if toolCalls != 5 {
		t.Errorf("tools/call count = %d, want 5 (no retries)", toolCalls)
	}
}

// A non-session failure must surface immediately rather than provoking a
// pointless handshake.
func TestCallTool_NonSessionErrorIsNotRetried(t *testing.T) {
	var calls int
	var mu sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		defer mu.Unlock()
		switch body.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "sid-1")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"` + protocolVersion + `","serverInfo":{"name":"wp","version":"1"}}}`))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		default:
			calls++
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"ability handler exploded"}}`))
		}
	}))
	defer ts.Close()

	c := NewClient(Config{Endpoint: ts.URL, BearerToken: "tok"})
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	_, err := c.CallTool(context.Background(), "t", map[string]any{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, ErrSessionLost) {
		t.Errorf("a handler failure must not be treated as session loss: %v", err)
	}
	if !strings.Contains(err.Error(), "exploded") {
		t.Errorf("original message should survive: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Errorf("tools/call attempts = %d, want 1 (no retry)", calls)
	}
}

// DiscoverAbilities and GetAbilityInfo route through CallTool, so they get
// recovery too. Worth pinning: discovery runs unattended on a ticker, and a
// stale session there silently empties the ability cache.
func TestDiscoverAbilities_RecoversFromExpiredSession(t *testing.T) {
	srv := &expiringServer{expireAfter: 0}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		srv.mu.Lock()
		defer srv.mu.Unlock()
		switch body.Method {
		case "initialize":
			srv.sessions++
			w.Header().Set("Mcp-Session-Id", "sid-"+itoa(srv.sessions))
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"` + protocolVersion + `","serverInfo":{"name":"wp","version":"1"}}}`))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		default:
			srv.toolCalls++
			// Reject only the first tools/call, then behave.
			if srv.toolCalls == 1 {
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32600,"message":"Invalid Request: session expired"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"success\":true,\"data\":{\"abilities\":[{\"name\":\"a\"}]}}"}]}}`))
		}
	}))
	defer ts.Close()

	c := NewClient(Config{Endpoint: ts.URL, BearerToken: "tok"})
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	abs, err := c.DiscoverAbilities(context.Background())
	if err != nil {
		t.Fatalf("DiscoverAbilities should have recovered: %v", err)
	}
	if len(abs) != 1 || abs[0].Name != "a" {
		t.Errorf("got %+v", abs)
	}
}
