package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/wooagent-os/wooagent-os/daemon/internal/secrets"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
	"github.com/zalando/go-keyring"
)

// memSecrets is a thin wrapper around the zalando/go-keyring mock backend.
// We don't use osKeyring directly because it lives in another package; the
// mock is process-global so we still get the in-memory storage by calling
// keyring.MockInit() at test start.
type memSecrets struct{}

func (memSecrets) Set(_ context.Context, k, v string) error {
	return keyring.Set("WooAgent OS", k, v)
}
func (memSecrets) Get(_ context.Context, k string) (string, error) {
	v, err := keyring.Get("WooAgent OS", k)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", secrets.ErrNotFound
	}
	return v, err
}
func (memSecrets) Delete(_ context.Context, k string) error {
	err := keyring.Delete("WooAgent OS", k)
	if errors.Is(err, keyring.ErrNotFound) {
		return secrets.ErrNotFound
	}
	return err
}

// newStoresTestRig wires a Server with only the /v1/stores routes plus a
// mocked keychain backend. PEP/manifest are nil because the stores
// endpoints don't go through PEP; the existing newTestRig is heavier and
// would couple stores tests to the issues fixture.
func newStoresTestRig(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	keyring.MockInit()

	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	s := &Server{store: st, secrets: memSecrets{}}

	r := chi.NewRouter()
	r.Get("/v1/stores", s.handleListStores)
	r.Post("/v1/stores", s.handleCreateStore)
	r.Get("/v1/stores/{id}", s.handleGetStore)
	r.Delete("/v1/stores/{id}", s.handleDeleteStore)

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	t.Cleanup(func() { delete(rigs.servers, ts.URL) })
	rigs.servers[ts.URL] = s
	return s, ts
}

func postStore(t *testing.T, ts *httptest.Server, url string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"url": url})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/stores", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	return res
}

// POST /v1/stores happy path: a fresh URL produces a pairing-state row with
// a code, an expires_at timestamp roughly pairingTTL in the future, and a
// pair_url derived from the store URL.
func TestCreateStore_HappyPath(t *testing.T) {
	_, ts := newStoresTestRig(t)

	res := postStore(t, ts, "https://mystore.com")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status=%d, want 201", res.StatusCode)
	}
	got := decode[Store](t, res)
	if got.ID == "" || got.URL != "https://mystore.com" {
		t.Errorf("unexpected payload: %+v", got)
	}
	if got.Status != "pairing" {
		t.Errorf("status=%q, want pairing", got.Status)
	}
	if got.PairingCode == "" || got.ExpiresAt == "" || got.PairURL == "" {
		t.Errorf("missing pairing fields: %+v", got)
	}
	if got.MCPEndpoint != "https://mystore.com/wp-json/mcp/v1" {
		t.Errorf("mcp_endpoint=%q", got.MCPEndpoint)
	}
	exp, err := time.Parse(time.RFC3339, got.ExpiresAt)
	if err != nil {
		t.Fatalf("parse expires_at: %v", err)
	}
	delta := time.Until(exp)
	if delta < 8*time.Minute || delta > pairingTTL {
		t.Errorf("expires_at=%s (%.0fs from now), want ~10m", got.ExpiresAt, delta.Seconds())
	}
}

// POST is idempotent on URL while a pairing row is in flight: the existing
// row's id is returned with a freshly-rotated pairing_code. The UI's
// "regen-once-on-expiry" flow depends on this.
func TestCreateStore_IdempotentDuringPairing(t *testing.T) {
	_, ts := newStoresTestRig(t)

	first := decode[Store](t, postStore(t, ts, "https://mystore.com"))
	res := postStore(t, ts, "https://mystore.com")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", res.StatusCode)
	}
	got := decode[Store](t, res)
	if got.ID != first.ID {
		t.Errorf("id rotated: %q vs %q", got.ID, first.ID)
	}
	if got.PairingCode == first.PairingCode {
		t.Errorf("pairing code did not rotate: %q", got.PairingCode)
	}
}

// A 'paired' row blocks fresh POSTs on the same URL — the operator must
// DELETE first. Distinguishes "fix a stuck pairing" (idempotent) from
// "replace a working connection" (explicit).
func TestCreateStore_ConflictWhenPaired(t *testing.T) {
	_, ts := newStoresTestRig(t)

	first := decode[Store](t, postStore(t, ts, "https://mystore.com"))

	// Promote the row to paired directly — exercising the post-pair path
	// without depending on the not-yet-implemented Companion Plugin handshake.
	if err := promoteToPaired(ts, first.ID); err != nil {
		t.Fatalf("promote: %v", err)
	}

	res := postStore(t, ts, "https://mystore.com")
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("status=%d, want 409", res.StatusCode)
	}
}

// Invalid URLs (non-https, with paths/queries, missing host) are rejected
// at the boundary. The UI's onboarding flow surfaces this code as a
// validation message under the URL input.
func TestCreateStore_InvalidURL(t *testing.T) {
	_, ts := newStoresTestRig(t)
	cases := []string{
		"",
		"http://insecure.com",
		"https://",
		"https://x.com/some/path",
		"https://x.com?q=1",
		"https://user:pass@x.com",
	}
	for _, raw := range cases {
		res := postStore(t, ts, raw)
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("url=%q status=%d, want 400", raw, res.StatusCode)
		}
		_ = res.Body.Close()
	}
}

// Trailing slashes and surrounding whitespace canonicalize to the same
// row — so the UNIQUE(url) index does its job and the operator can't
// accidentally keep two near-duplicate rows.
func TestCreateStore_NormalizesURL(t *testing.T) {
	_, ts := newStoresTestRig(t)

	a := decode[Store](t, postStore(t, ts, "https://mystore.com"))
	b := decode[Store](t, postStore(t, ts, "  https://mystore.com/  "))
	if a.ID != b.ID {
		t.Errorf("normalization failed: %q vs %q", a.ID, b.ID)
	}
}

// GET /v1/stores returns every row. Empty case returns an empty array
// (not null) so the UI's .map() works without a nil-guard.
func TestListStores(t *testing.T) {
	_, ts := newStoresTestRig(t)

	res, err := http.Get(ts.URL + "/v1/stores")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	empty := decode[struct{ Stores []Store }](t, res)
	if empty.Stores == nil {
		t.Errorf("stores=nil, want []")
	}

	postStore(t, ts, "https://a.com").Body.Close()
	postStore(t, ts, "https://b.com").Body.Close()

	res2, _ := http.Get(ts.URL + "/v1/stores")
	got := decode[struct{ Stores []Store }](t, res2)
	if len(got.Stores) != 2 {
		t.Errorf("got %d stores, want 2", len(got.Stores))
	}
}

// GET /v1/stores/:id transitions a 'pairing' row to 'expired' when its
// window has closed. The lazy-on-read transition keeps state changes in
// one place and means the daemon doesn't run a goroutine per pending pair.
func TestGetStore_ExpiresOnRead(t *testing.T) {
	_, ts := newStoresTestRig(t)

	created := decode[Store](t, postStore(t, ts, "https://mystore.com"))

	// Backdate the expires_at by hand. Avoids time.Sleep in tests.
	if err := backdateExpiry(ts, created.ID); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	res, err := http.Get(ts.URL + "/v1/stores/" + created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	got := decode[Store](t, res)
	if got.Status != "expired" {
		t.Errorf("status=%q, want expired", got.Status)
	}
	if got.PairingCode != "" {
		t.Errorf("expired row still carries pairing_code=%q", got.PairingCode)
	}
}

func TestGetStore_NotFound(t *testing.T) {
	_, ts := newStoresTestRig(t)

	res, err := http.Get(ts.URL + "/v1/stores/store_does-not-exist")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status=%d, want 404", res.StatusCode)
	}
}

// DELETE /v1/stores/:id wipes the row + the keychain entry for its
// device token. No keychain leak for a token whose row was deleted.
func TestDeleteStore_HappyPath(t *testing.T) {
	_, ts := newStoresTestRig(t)

	created := decode[Store](t, postStore(t, ts, "https://mystore.com"))

	const tokenRef = "wooagent.stores." + "fake"
	if err := keyring.Set("WooAgent OS", tokenRef, "device-token-secret"); err != nil {
		t.Fatalf("seed keyring: %v", err)
	}
	if err := setTokenRef(ts, created.ID, tokenRef); err != nil {
		t.Fatalf("set token_ref: %v", err)
	}

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/v1/stores/"+created.ID, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status=%d, want 204", res.StatusCode)
	}

	getRes, _ := http.Get(ts.URL + "/v1/stores/" + created.ID)
	if getRes.StatusCode != http.StatusNotFound {
		t.Errorf("row still present: status=%d", getRes.StatusCode)
	}

	if _, err := keyring.Get("WooAgent OS", tokenRef); !errors.Is(err, keyring.ErrNotFound) {
		t.Errorf("keychain entry still present: %v", err)
	}
}

func TestDeleteStore_NotFound(t *testing.T) {
	_, ts := newStoresTestRig(t)
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/v1/stores/store_nope", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status=%d, want 404", res.StatusCode)
	}
}

// DELETE with no token_ref (i.e., row never finished pairing) still
// succeeds — the keychain step is skipped, the row is removed.
func TestDeleteStore_NoTokenRef(t *testing.T) {
	_, ts := newStoresTestRig(t)
	created := decode[Store](t, postStore(t, ts, "https://mystore.com"))

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/v1/stores/"+created.ID, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if res.StatusCode != http.StatusNoContent {
		t.Errorf("status=%d, want 204", res.StatusCode)
	}
}

// Helpers below mutate the DB directly to set up state the HTTP API doesn't
// yet expose (paired transitions need the Companion Plugin handshake;
// backdating expires_at avoids time.Sleep). The rig registry indexes the
// underlying *Server by ts.URL so tests can reach the DB without the rig
// returning two values from a one-line setup.

var rigs = struct {
	servers map[string]*Server
}{servers: map[string]*Server{}}

func promoteToPaired(ts *httptest.Server, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := rigs.servers[ts.URL].store.DB.Exec(
		`UPDATE stores SET status='paired', pairing_code=NULL, paired_at=?, updated_at=? WHERE id=?`,
		now, now, id,
	)
	return err
}

func backdateExpiry(ts *httptest.Server, id string) error {
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	_, err := rigs.servers[ts.URL].store.DB.Exec(`UPDATE stores SET expires_at=? WHERE id=?`, past, id)
	return err
}

func setTokenRef(ts *httptest.Server, id, ref string) error {
	_, err := rigs.servers[ts.URL].store.DB.Exec(`UPDATE stores SET token_ref=? WHERE id=?`, ref, id)
	return err
}
