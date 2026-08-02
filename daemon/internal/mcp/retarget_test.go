package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// initServer is a minimal MCP endpoint that records the auth header and
// session id of every request and answers `initialize` with a session.
type initServer struct {
	name string

	mu       sync.Mutex
	authSeen []string
	sidSeen  []string
	hits     int
}

func (s *initServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.hits++
		s.authSeen = append(s.authSeen, r.Header.Get("Authorization"))
		s.sidSeen = append(s.sidSeen, r.Header.Get("Mcp-Session-Id"))
		s.mu.Unlock()

		w.Header().Set("Mcp-Session-Id", "sid-"+s.name)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": 1,
			"result": map[string]any{
				"protocolVersion": protocolVersion,
				"serverInfo":      map[string]any{"name": s.name, "version": "1"},
			},
		})
	})
}

func (s *initServer) snapshot() (int, []string, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits, append([]string(nil), s.authSeen...), append([]string(nil), s.sidSeen...)
}

// The scenario from DSGWOO-1470: the operator re-pairs to a different
// store while the daemon is running. The one shared *Client must start
// talking to the new store, with the new credentials, and must not carry
// the old store's session id across.
func TestRetarget_SwitchesStoreAndDropsSession(t *testing.T) {
	oldSrv, newSrv := &initServer{name: "old-store"}, &initServer{name: "new-store"}
	oldTS := httptest.NewServer(oldSrv.handler())
	defer oldTS.Close()
	newTS := httptest.NewServer(newSrv.handler())
	defer newTS.Close()

	c := NewClient(Config{Endpoint: oldTS.URL, BearerToken: "old-token"})
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("initial handshake: %v", err)
	}
	if got := c.Endpoint(); got != oldTS.URL {
		t.Fatalf("Endpoint() = %q, want %q", got, oldTS.URL)
	}

	if changed := c.Retarget(Config{Endpoint: newTS.URL, BearerToken: "new-token"}); !changed {
		t.Fatal("Retarget reported no change, want changed")
	}
	if got := c.Endpoint(); got != newTS.URL {
		t.Errorf("Endpoint() = %q, want %q", got, newTS.URL)
	}

	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("post-retarget handshake: %v", err)
	}

	// The new store must have been called, with the new credentials.
	hits, auth, sids := newSrv.snapshot()
	if hits == 0 {
		t.Fatal("new store received no requests after retarget")
	}
	if auth[0] != "Bearer new-token" {
		t.Errorf("new store saw auth %q, want the new token", auth[0])
	}
	// Critically: no session id from the old store leaked over. Reusing it
	// would make the first call against the new store fail as session-lost.
	if sids[0] != "" {
		t.Errorf("new store saw stale session id %q, want empty", sids[0])
	}

	// And the old store saw nothing after the switch.
	hitsBefore, _, _ := oldSrv.snapshot()
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("second post-retarget handshake: %v", err)
	}
	hitsAfter, _, _ := oldSrv.snapshot()
	if hitsAfter != hitsBefore {
		t.Errorf("old store still receiving traffic: %d -> %d", hitsBefore, hitsAfter)
	}
}

func TestRetarget_NoopWhenUnchanged(t *testing.T) {
	srv := &initServer{name: "s"}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	cfg := Config{Endpoint: ts.URL, BearerToken: "tok"}
	c := NewClient(cfg)
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}

	if changed := c.Retarget(cfg); changed {
		t.Error("Retarget reported a change for an identical target")
	}
	// The session must survive a no-op retarget, or the reconciler would
	// force a fresh handshake on every tick.
	if c.getSessionID() == "" {
		t.Error("no-op Retarget dropped the cached session")
	}
}

func TestRetarget_SwitchesAuthScheme(t *testing.T) {
	// Env-var (Basic) config replaced by a paired-store bearer token.
	srv := &initServer{name: "s"}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	c := NewClient(Config{Endpoint: ts.URL, Username: "u", Password: "p"})
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	c.Retarget(Config{Endpoint: ts.URL, BearerToken: "tok"})
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("post-retarget init: %v", err)
	}

	_, auth, _ := srv.snapshot()
	if auth[0] == "" || auth[0][:6] != "Basic " {
		t.Errorf("first request auth = %q, want Basic", auth[0])
	}
	if last := auth[len(auth)-1]; last != "Bearer tok" {
		t.Errorf("last request auth = %q, want Bearer tok", last)
	}
}
