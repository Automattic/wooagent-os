package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"
	"github.com/zalando/go-keyring"

	"github.com/wooagent-os/wooagent-os/daemon/internal/abilities"
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

// fakeRefreshMCP is the minimal mcp.Client surface RunForStore needs.
// Pre-fab a small response set so reconcile has something to commit.
type fakeRefreshMCP struct {
	abilities []mcp.AbilitySummary
	infos     map[string]mcp.AbilityInfo
}

func (f *fakeRefreshMCP) Initialize(_ context.Context) (mcp.ServerInfo, error) {
	return mcp.ServerInfo{}, nil
}
func (f *fakeRefreshMCP) DiscoverAbilities(_ context.Context) ([]mcp.AbilitySummary, error) {
	return f.abilities, nil
}
func (f *fakeRefreshMCP) GetAbilityInfo(_ context.Context, name string) (mcp.AbilityInfo, error) {
	if info, ok := f.infos[name]; ok {
		return info, nil
	}
	return mcp.AbilityInfo{Name: name}, nil
}

// newRefreshAbilitiesRig wires a Server with the refresh-abilities route
// and an abilities.Runner backed by a programmable fake MCP client. The
// rig pre-seeds one paired store row + keychain entry so RunForStore can
// reach the "load token, build client, discover" happy path. Tests that
// want a non-paired status can mutate srv.store.DB directly via the
// returned Server pointer.
func newRefreshAbilitiesRig(t *testing.T, fake *fakeRefreshMCP) (*httptest.Server, *Server, string) {
	t.Helper()
	keyring.MockInit()

	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	runner := abilities.New(st.DB, memSecrets{}, nil)
	runner.PollInterval = 0
	runner.NewClient = func(_ context.Context, _, _ string) (abilities.Client, error) {
		return fake, nil
	}

	s := &Server{store: st, secrets: memSecrets{}, abilities: runner}

	r := chi.NewRouter()
	r.Post("/v1/stores/{id}/refresh-abilities", s.handleRefreshStoreAbilities)

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	const id = "store_refresh_test"
	if _, err := st.DB.Exec(
		`INSERT INTO stores(id, url, mcp_endpoint, status, token_ref,
		                    paired_at, created_at, updated_at)
		 VALUES(?, ?, ?, 'paired', ?, ?, ?, ?)`,
		id, "https://refresh.test", "https://refresh.test/wp-json/mcp/v1",
		"wooagent.stores."+id,
		"2026-05-22T00:00:00Z", "2026-05-22T00:00:00Z", "2026-05-22T00:00:00Z",
	); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	if err := keyring.Set("WooAgent OS", "wooagent.stores."+id, "device-token"); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	return ts, s, id
}

// Happy path: paired store + reachable MCP → 200 with a fresh
// ability_count and a populated last_discovered_at.
func TestRefreshAbilities_HappyPath(t *testing.T) {
	const a, b = "wooagent-products/list", "wooagent-orders/get"
	ts, _, storeID := newRefreshAbilitiesRig(t, &fakeRefreshMCP{
		abilities: []mcp.AbilitySummary{{Name: a}, {Name: b}},
		infos: map[string]mcp.AbilityInfo{
			a: {Name: a, Description: "list"},
			b: {Name: b, Description: "get"},
		},
	})

	res, err := http.Post(ts.URL+"/v1/stores/"+storeID+"/refresh-abilities",
		"application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", res.StatusCode)
	}
	var body struct {
		AbilityCount     int    `json:"ability_count"`
		LastDiscoveredAt string `json:"last_discovered_at"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.AbilityCount != 2 {
		t.Errorf("ability_count=%d, want 2", body.AbilityCount)
	}
	if body.LastDiscoveredAt == "" {
		t.Errorf("last_discovered_at empty after successful refresh")
	}
}

// Unknown store id → 404 with the canonical envelope.
func TestRefreshAbilities_StoreNotFound(t *testing.T) {
	ts, _, _ := newRefreshAbilitiesRig(t, &fakeRefreshMCP{})

	res, err := http.Post(ts.URL+"/v1/stores/store_nope/refresh-abilities",
		"application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status=%d, want 404", res.StatusCode)
	}
}

// Store in non-paired status (still pairing / expired / failed) → 409
// so the UI surfaces "pair the store first" rather than spinning on a
// discovery call that can't possibly succeed.
func TestRefreshAbilities_StoreNotPaired(t *testing.T) {
	ts, srv, storeID := newRefreshAbilitiesRig(t, &fakeRefreshMCP{})

	if _, err := srv.store.DB.Exec(
		`UPDATE stores SET status='pairing' WHERE id=?`, storeID,
	); err != nil {
		t.Fatalf("demote store: %v", err)
	}

	res, err := http.Post(ts.URL+"/v1/stores/"+storeID+"/refresh-abilities",
		"application/json", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if res.StatusCode != http.StatusConflict {
		t.Errorf("status=%d, want 409", res.StatusCode)
	}
	var body struct {
		Err struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Err.Code != "store_not_paired" {
		t.Errorf("error code=%q, want store_not_paired", body.Err.Code)
	}
}
