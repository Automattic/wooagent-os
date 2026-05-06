package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
	"github.com/zalando/go-keyring"
)

// fakeModelTester records the args it was called with and returns a
// canned outcome. Lets each test choose ok/error/models without burning
// real upstream credit.
type fakeModelTester struct {
	got  ModelTestArgs
	out  ModelTestOutcome
	calls int
}

func (f *fakeModelTester) Test(_ context.Context, a ModelTestArgs) ModelTestOutcome {
	f.calls++
	f.got = a
	return f.out
}

func newProvidersTestRig(t *testing.T, tester ModelTester) (*Server, *httptest.Server) {
	t.Helper()
	keyring.MockInit()

	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if tester == nil {
		tester = &fakeModelTester{out: ModelTestOutcome{OK: true}}
	}
	s := &Server{store: st, secrets: memSecrets{}, modelTester: tester}

	r := chi.NewRouter()
	r.Get("/v1/model-providers", s.handleListModelProviders)
	r.Post("/v1/model-providers", s.handleCreateModelProvider)
	r.Post("/v1/model-providers/test", s.handleTestModelProvider)
	r.Delete("/v1/model-providers/{id}", s.handleDeleteModelProvider)

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	t.Cleanup(func() { delete(rigs.servers, ts.URL) })
	rigs.servers[ts.URL] = s
	return s, ts
}

func postJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	return res
}

// POST /v1/model-providers/test always returns 200; ok=false carries a
// human-readable message. The endpoint itself completed; the failure is
// information about the configured upstream.
func TestTestModelProvider_OK(t *testing.T) {
	tester := &fakeModelTester{out: ModelTestOutcome{OK: true, Models: []string{"claude-sonnet-4-6"}}}
	_, ts := newProvidersTestRig(t, tester)

	res := postJSON(t, ts.URL+"/v1/model-providers/test", map[string]any{
		"kind":          "anthropic",
		"api_key":       "sk-ant-test",
		"default_model": "claude-sonnet-4-6",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", res.StatusCode)
	}
	got := decode[map[string]any](t, res)
	if got["ok"] != true {
		t.Errorf("ok=%v, want true", got["ok"])
	}
	if tester.got.Kind != "anthropic" || tester.got.APIKey != "sk-ant-test" {
		t.Errorf("tester args wrong: %+v", tester.got)
	}
}

// invalid_kind rejects at the boundary (400) before reaching the tester —
// a typo from the UI shouldn't cause a real upstream call.
func TestTestModelProvider_InvalidKind(t *testing.T) {
	tester := &fakeModelTester{}
	_, ts := newProvidersTestRig(t, tester)

	res := postJSON(t, ts.URL+"/v1/model-providers/test", map[string]any{"kind": "bogus"})
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", res.StatusCode)
	}
	if tester.calls != 0 {
		t.Errorf("tester called %d times for invalid kind", tester.calls)
	}
}

// Anthropic / OpenAI without an api_key returns ok=false (still 200).
// The UI's button uses ok to decide whether to enable Save.
func TestTestModelProvider_RequiredAPIKey(t *testing.T) {
	tester := &fakeModelTester{}
	_, ts := newProvidersTestRig(t, tester)

	res := postJSON(t, ts.URL+"/v1/model-providers/test", map[string]any{
		"kind":          "anthropic",
		"default_model": "claude-sonnet-4-6",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", res.StatusCode)
	}
	got := decode[map[string]any](t, res)
	if got["ok"] != false {
		t.Errorf("ok=%v, want false", got["ok"])
	}
	if tester.calls != 0 {
		t.Errorf("tester called %d times when validation should have short-circuited", tester.calls)
	}
}

// Ollama needs an endpoint, not an api_key. Same pattern: ok=false at
// 200 when missing.
func TestTestModelProvider_OllamaRequiresEndpoint(t *testing.T) {
	_, ts := newProvidersTestRig(t, nil)

	res := postJSON(t, ts.URL+"/v1/model-providers/test", map[string]any{
		"kind":          "ollama",
		"default_model": "llama3.2",
	})
	got := decode[map[string]any](t, res)
	if got["ok"] != false {
		t.Errorf("ok=%v, want false", got["ok"])
	}
}

// First create gets is_default=1; second create on a different kind gets
// is_default=0. The partial unique index in migration 007 enforces this.
func TestCreateModelProvider_FirstIsDefault(t *testing.T) {
	_, ts := newProvidersTestRig(t, nil)

	first := decode[ModelProvider](t, postJSON(t, ts.URL+"/v1/model-providers", map[string]any{
		"kind":          "anthropic",
		"api_key":       "sk-ant-1",
		"default_model": "claude-sonnet-4-6",
	}))
	if !first.IsDefault {
		t.Errorf("first.IsDefault=false, want true")
	}
	if first.Name == "" || first.LastTestStatus != "untested" {
		t.Errorf("expected derived name + 'untested' status, got: %+v", first)
	}

	second := decode[ModelProvider](t, postJSON(t, ts.URL+"/v1/model-providers", map[string]any{
		"kind":          "openai",
		"api_key":       "sk-oai-1",
		"default_model": "gpt-5.0",
	}))
	if second.IsDefault {
		t.Errorf("second.IsDefault=true, want false (default already taken)")
	}
}

// Create writes the API key into the keychain, never to SQLite.
func TestCreateModelProvider_WritesKeychain(t *testing.T) {
	_, ts := newProvidersTestRig(t, nil)

	created := decode[ModelProvider](t, postJSON(t, ts.URL+"/v1/model-providers", map[string]any{
		"kind":          "anthropic",
		"api_key":       "sk-ant-secret",
		"default_model": "claude-sonnet-4-6",
	}))

	got, err := keyring.Get("WooAgent OS", "wooagent.model_providers."+created.ID)
	if err != nil {
		t.Fatalf("keychain Get: %v", err)
	}
	if got != "sk-ant-secret" {
		t.Errorf("keychain value=%q, want %q", got, "sk-ant-secret")
	}
}

// Validation: missing default_model, missing api_key for anthropic, etc.
func TestCreateModelProvider_Validation(t *testing.T) {
	_, ts := newProvidersTestRig(t, nil)

	cases := []struct {
		name   string
		body   map[string]any
		want   int
		errKey string
	}{
		{"invalid kind", map[string]any{"kind": "bogus", "default_model": "x"}, 400, "invalid_kind"},
		{"missing default_model", map[string]any{"kind": "anthropic", "api_key": "k"}, 400, "missing_default_model"},
		{"missing api_key", map[string]any{"kind": "anthropic", "default_model": "x"}, 400, "missing_api_key"},
		{"missing endpoint", map[string]any{"kind": "ollama", "default_model": "x"}, 400, "missing_endpoint"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := postJSON(t, ts.URL+"/v1/model-providers", c.body)
			if res.StatusCode != c.want {
				t.Errorf("status=%d, want %d", res.StatusCode, c.want)
			}
			body := decode[map[string]any](t, res)
			errObj, _ := body["error"].(map[string]any)
			if errObj["code"] != c.errKey {
				t.Errorf("error.code=%v, want %q", errObj["code"], c.errKey)
			}
		})
	}
}

// List orders is_default first, then created_at ASC. Onboarding's "fleet
// default" is what the UI shows at the top of Settings → Models.
func TestListModelProviders_OrdersDefaultFirst(t *testing.T) {
	_, ts := newProvidersTestRig(t, nil)

	postJSON(t, ts.URL+"/v1/model-providers", map[string]any{
		"kind": "anthropic", "api_key": "k1", "default_model": "claude-sonnet-4-6",
	}).Body.Close()
	postJSON(t, ts.URL+"/v1/model-providers", map[string]any{
		"kind": "openai", "api_key": "k2", "default_model": "gpt-5.0",
	}).Body.Close()

	// Now manually flip the second to default to confirm ordering follows
	// is_default rather than insert order.
	if err := flipDefault(ts, "openai"); err != nil {
		t.Fatalf("flip: %v", err)
	}

	res, _ := http.Get(ts.URL + "/v1/model-providers")
	got := decode[struct{ Providers []ModelProvider }](t, res)
	if len(got.Providers) != 2 {
		t.Fatalf("len=%d, want 2", len(got.Providers))
	}
	if got.Providers[0].Kind != "openai" {
		t.Errorf("first kind=%q, want openai (the default)", got.Providers[0].Kind)
	}
}

// DELETE wipes the keychain entry + the row. No keychain leak when an
// operator removes a provider.
func TestDeleteModelProvider_HappyPath(t *testing.T) {
	_, ts := newProvidersTestRig(t, nil)

	created := decode[ModelProvider](t, postJSON(t, ts.URL+"/v1/model-providers", map[string]any{
		"kind": "anthropic", "api_key": "sk-ant-x", "default_model": "claude-sonnet-4-6",
	}))

	ref := "wooagent.model_providers." + created.ID
	if _, err := keyring.Get("WooAgent OS", ref); err != nil {
		t.Fatalf("keychain seed missing: %v", err)
	}

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/v1/model-providers/"+created.ID, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status=%d, want 204", res.StatusCode)
	}

	if _, err := keyring.Get("WooAgent OS", ref); !errors.Is(err, keyring.ErrNotFound) {
		t.Errorf("keychain still has secret: %v", err)
	}

	listRes, _ := http.Get(ts.URL + "/v1/model-providers")
	got := decode[struct{ Providers []ModelProvider }](t, listRes)
	if len(got.Providers) != 0 {
		t.Errorf("got %d providers, want 0", len(got.Providers))
	}
}

func TestDeleteModelProvider_NotFound(t *testing.T) {
	_, ts := newProvidersTestRig(t, nil)
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/v1/model-providers/mp_nope", nil)
	res, _ := http.DefaultClient.Do(req)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status=%d, want 404", res.StatusCode)
	}
}

// flipDefault promotes the row of the named kind to is_default=1 in a
// transaction (clear-then-set so the partial unique index never sees a
// two-defaults state). Used in TestListModelProviders_OrdersDefaultFirst
// to check that ordering follows the column, not insert order.
func flipDefault(ts *httptest.Server, kind string) error {
	db := rigs.servers[ts.URL].store.DB
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE model_providers SET is_default=0 WHERE is_default=1`); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`UPDATE model_providers SET is_default=1 WHERE kind=?`, kind); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
