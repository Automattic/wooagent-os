package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/wooagent-os/wooagent-os/daemon/internal/pairing"
	"github.com/wooagent-os/wooagent-os/daemon/internal/secrets"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
	"github.com/zalando/go-keyring"
)

// fakePairing is a programmable PairingClient. Tests set RequestErr,
// PollResult, etc. to simulate the Companion Plugin's responses without
// spinning up a real plugin. Calls are recorded for assertions.
type fakePairing struct {
	mu sync.Mutex

	RequestErr error
	PollResult pairing.PollResult
	PollErr    error
	RevokeErr  error

	RequestCalls []fakePairingCall
	PollCalls    []fakePairingCall
	RevokeCalls  []fakePairingCall
}

type fakePairingCall struct {
	StoreURL    string
	Code        string
	DeviceName  string
	DeviceToken string
}

func (f *fakePairing) Request(_ context.Context, storeURL, code, deviceName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.RequestCalls = append(f.RequestCalls, fakePairingCall{StoreURL: storeURL, Code: code, DeviceName: deviceName})
	return f.RequestErr
}

func (f *fakePairing) Poll(_ context.Context, storeURL, code string) (pairing.PollResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.PollCalls = append(f.PollCalls, fakePairingCall{StoreURL: storeURL, Code: code})
	return f.PollResult, f.PollErr
}

func (f *fakePairing) Revoke(_ context.Context, storeURL, deviceToken string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.RevokeCalls = append(f.RevokeCalls, fakePairingCall{StoreURL: storeURL, DeviceToken: deviceToken})
	return f.RevokeErr
}

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
// mocked keychain backend and an injectable PairingClient. PEP/manifest
// are nil because stores endpoints don't go through PEP; the existing
// newTestRig is heavier and would couple stores tests to the issues
// fixture.
//
// If pair is nil the rig installs a fakePairing that returns "pending"
// — handler tests that don't care about pairing get a no-op stub.
func newStoresTestRig(t *testing.T, pair PairingClient) (*Server, *httptest.Server) {
	t.Helper()
	keyring.MockInit()

	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if pair == nil {
		pair = &fakePairing{PollResult: pairing.PollResult{Status: pairing.StatusPending}}
	}
	s := &Server{store: st, secrets: memSecrets{}, pairing: pair}

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
	_, ts := newStoresTestRig(t, nil)

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
	if got.MCPEndpoint != "https://mystore.com/wp-json/mcp/mcp-adapter-default-server" {
		t.Errorf("mcp_endpoint=%q, want https://mystore.com/wp-json/mcp/mcp-adapter-default-server", got.MCPEndpoint)
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
	_, ts := newStoresTestRig(t, nil)

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
	_, ts := newStoresTestRig(t, nil)

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
	_, ts := newStoresTestRig(t, nil)
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
	_, ts := newStoresTestRig(t, nil)

	a := decode[Store](t, postStore(t, ts, "https://mystore.com"))
	b := decode[Store](t, postStore(t, ts, "  https://mystore.com/  "))
	if a.ID != b.ID {
		t.Errorf("normalization failed: %q vs %q", a.ID, b.ID)
	}
}

// GET /v1/stores returns every row. Empty case returns an empty array
// (not null) so the UI's .map() works without a nil-guard.
func TestListStores(t *testing.T) {
	_, ts := newStoresTestRig(t, nil)

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
	_, ts := newStoresTestRig(t, nil)

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
	_, ts := newStoresTestRig(t, nil)

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
	_, ts := newStoresTestRig(t, nil)

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
	_, ts := newStoresTestRig(t, nil)
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
	_, ts := newStoresTestRig(t, nil)
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

// ---------- pairing handshake integration ----------

// POST /v1/stores calls Request on the pairing client with the generated
// code. The UI should be able to show the code immediately while the
// plugin-side transient is registered in parallel.
func TestCreateStore_CallsPairingRequest(t *testing.T) {
	pair := &fakePairing{PollResult: pairing.PollResult{Status: pairing.StatusPending}}
	_, ts := newStoresTestRig(t, pair)

	created := decode[Store](t, postStore(t, ts, "https://mystore.com"))
	if len(pair.RequestCalls) != 1 {
		t.Fatalf("RequestCalls=%d, want 1", len(pair.RequestCalls))
	}
	got := pair.RequestCalls[0]
	if got.StoreURL != "https://mystore.com" || got.Code != created.PairingCode {
		t.Errorf("Request args wrong: %+v vs created %+v", got, created)
	}
	if got.DeviceName == "" {
		t.Errorf("DeviceName empty, want hostname or fallback")
	}
}

// PluginNotInstalled (404 from /pair/request) flips the row to failed
// with reason 'companion_plugin_missing'. The UI surfaces this as a
// targeted error rather than a generic transient one.
func TestCreateStore_PluginMissing(t *testing.T) {
	pair := &fakePairing{RequestErr: pairing.PluginNotInstalled}
	_, ts := newStoresTestRig(t, pair)

	created := decode[Store](t, postStore(t, ts, "https://mystore.com"))
	if created.Status != "failed" {
		t.Errorf("status=%q, want failed", created.Status)
	}
}

// GET /v1/stores/:id polls the plugin while pairing. When the plugin
// returns approved + a device token, the daemon writes the token to the
// keychain and flips the row to 'paired'.
func TestGetStore_PollApprovedTransitions(t *testing.T) {
	pair := &fakePairing{PollResult: pairing.PollResult{
		Status:      pairing.StatusApproved,
		DeviceID:    "dev_abc",
		DeviceName:  "test-device",
		DeviceToken: "secret-token",
	}}
	_, ts := newStoresTestRig(t, pair)

	created := decode[Store](t, postStore(t, ts, "https://mystore.com"))

	res, err := http.Get(ts.URL + "/v1/stores/" + created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	got := decode[Store](t, res)
	if got.Status != "paired" {
		t.Errorf("status=%q, want paired", got.Status)
	}
	if got.PairedAt == "" {
		t.Errorf("paired_at empty")
	}
	if got.DeviceName != "test-device" {
		t.Errorf("device_name=%q, want test-device", got.DeviceName)
	}

	// Token landed in the keychain under the per-store key.
	stored, err := keyring.Get("WooAgent OS", "wooagent.stores."+created.ID)
	if err != nil {
		t.Fatalf("keychain Get: %v", err)
	}
	if stored != "secret-token" {
		t.Errorf("keychain token=%q, want secret-token", stored)
	}
}

// Operator clicks Reject in wp-admin → plugin returns rejected → row
// flips to failed with reason 'operator_rejected'. The UI maps this to
// a targeted "you rejected this device" message.
func TestGetStore_PollRejectedTransitions(t *testing.T) {
	pair := &fakePairing{PollResult: pairing.PollResult{Status: pairing.StatusRejected}}
	_, ts := newStoresTestRig(t, pair)

	created := decode[Store](t, postStore(t, ts, "https://mystore.com"))

	res, err := http.Get(ts.URL + "/v1/stores/" + created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	got := decode[Store](t, res)
	if got.Status != "failed" {
		t.Errorf("status=%q, want failed", got.Status)
	}
}

// Pending poll keeps the row in 'pairing' — the UI continues to display
// the code + countdown. The lazy expiry path (backdate expires_at) still
// transitions to 'expired' even when the plugin would return pending.
func TestGetStore_PendingThenExpiry(t *testing.T) {
	pair := &fakePairing{PollResult: pairing.PollResult{Status: pairing.StatusPending}}
	_, ts := newStoresTestRig(t, pair)

	created := decode[Store](t, postStore(t, ts, "https://mystore.com"))

	// First read: still pairing.
	res, _ := http.Get(ts.URL + "/v1/stores/" + created.ID)
	got := decode[Store](t, res)
	if got.Status != "pairing" {
		t.Errorf("status=%q, want pairing", got.Status)
	}

	// Backdate the window and read again — lazy expiry overrides the poll.
	if err := backdateExpiry(ts, created.ID); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	res2, _ := http.Get(ts.URL + "/v1/stores/" + created.ID)
	got2 := decode[Store](t, res2)
	if got2.Status != "expired" {
		t.Errorf("post-backdate status=%q, want expired", got2.Status)
	}
}

// DELETE on a paired row calls Revoke with the keychain'd token before
// wiping local state. The plugin then drops the device from its devices
// list — no orphans.
func TestDeleteStore_CallsRevokeForPaired(t *testing.T) {
	pair := &fakePairing{PollResult: pairing.PollResult{
		Status:      pairing.StatusApproved,
		DeviceID:    "dev_abc",
		DeviceName:  "test",
		DeviceToken: "secret-token",
	}}
	_, ts := newStoresTestRig(t, pair)

	created := decode[Store](t, postStore(t, ts, "https://mystore.com"))
	// Drive through to paired.
	http.Get(ts.URL + "/v1/stores/" + created.ID)

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/v1/stores/"+created.ID, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status=%d, want 204", res.StatusCode)
	}

	if len(pair.RevokeCalls) != 1 {
		t.Fatalf("RevokeCalls=%d, want 1", len(pair.RevokeCalls))
	}
	if pair.RevokeCalls[0].DeviceToken != "secret-token" {
		t.Errorf("Revoke token=%q, want secret-token", pair.RevokeCalls[0].DeviceToken)
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
