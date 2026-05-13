package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"
	"github.com/zalando/go-keyring"

	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

// newAbilitiesTestRig wires only the abilities routes on top of an
// in-memory store. PEP/manifest aren't needed because abilities don't
// route through PEP. The discovery runner is left at its default — these
// tests pre-populate the abilities table directly so they exercise the
// HTTP shape without bringing up a fake MCP server.
func newAbilitiesTestRig(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	keyring.MockInit()

	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	s := &Server{store: st, secrets: memSecrets{}}

	r := chi.NewRouter()
	r.Get("/v1/abilities", s.handleListAbilities)
	r.Post("/v1/abilities/{id}/trust", s.handleTrustAbility)

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	t.Cleanup(func() { delete(rigs.servers, ts.URL) })
	rigs.servers[ts.URL] = s
	return s, ts
}

// seedAbility writes a minimal abilities row for handler tests. State is
// the trust_state ('new' | 'trusted' | 'schema_changed').
func seedAbility(t *testing.T, s *Server, storeID, name, state string) string {
	t.Helper()
	const seedNow = "2026-05-07T00:00:00Z"

	// stores row first (FK).
	if _, err := s.store.DB.Exec(
		`INSERT OR IGNORE INTO stores(id, url, mcp_endpoint, status, created_at, updated_at)
		 VALUES(?, ?, ?, 'paired', ?, ?)`,
		storeID, "https://"+storeID+".local", "https://"+storeID+".local/wp-json/mcp/v1", seedNow, seedNow,
	); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	id := "ab_" + storeID + "_" + strings.ReplaceAll(name, "/", "_")
	if _, err := s.store.DB.Exec(
		`INSERT INTO abilities(id, store_id, name, schema_json, schema_hash, trust_state, last_seen_at, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, storeID, name, `{"name":"`+name+`"}`, "hash-"+name, state, seedNow, seedNow, seedNow,
	); err != nil {
		t.Fatalf("seed ability: %v", err)
	}
	return id
}

// GET /v1/abilities returns rows joined to their store. Empty filter
// returns everything.
func TestListAbilities_All(t *testing.T) {
	s, ts := newAbilitiesTestRig(t)
	seedAbility(t, s, "store_a", "ab/one", "new")
	seedAbility(t, s, "store_a", "ab/two", "trusted")
	seedAbility(t, s, "store_b", "ab/three", "schema_changed")

	res, err := http.Get(ts.URL + "/v1/abilities")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", res.StatusCode)
	}
	body := decode[struct {
		Abilities []Ability `json:"abilities"`
	}](t, res)
	if len(body.Abilities) != 3 {
		t.Fatalf("rows=%d, want 3", len(body.Abilities))
	}
	for _, a := range body.Abilities {
		if a.StoreURL == "" {
			t.Errorf("missing store_url: %+v", a)
		}
		if a.SchemaHash == "" {
			t.Errorf("missing schema_hash: %+v", a)
		}
		if a.EffectiveTrust == "" {
			t.Errorf("missing effective_trust: %+v", a)
		}
	}
	// None of the seeded abilities are in the manifest; verify the expected
	// effective_trust for each trust_state value.
	wantET := map[string]string{
		"ab/one":   "needs_review",
		"ab/two":   "trusted",
		"ab/three": "schema_changed",
	}
	for _, a := range body.Abilities {
		if want, ok := wantET[a.Name]; ok && a.EffectiveTrust != want {
			t.Errorf("effective_trust for %q = %q; want %q", a.Name, a.EffectiveTrust, want)
		}
	}
}

// trust_state filter scopes the list. Invalid values return 400 with the
// canonical error envelope.
func TestListAbilities_FilterTrustState(t *testing.T) {
	s, ts := newAbilitiesTestRig(t)
	seedAbility(t, s, "store_a", "ab/one", "new")
	seedAbility(t, s, "store_a", "ab/two", "trusted")
	seedAbility(t, s, "store_a", "ab/three", "schema_changed")

	res, err := http.Get(ts.URL + "/v1/abilities?trust_state=trusted")
	if err != nil {
		t.Fatal(err)
	}
	body := decode[struct {
		Abilities []Ability `json:"abilities"`
	}](t, res)
	if len(body.Abilities) != 1 || body.Abilities[0].Name != "ab/two" {
		t.Errorf("got %+v, want one row name=ab/two", body.Abilities)
	}

	res2, err := http.Get(ts.URL + "/v1/abilities?trust_state=bogus")
	if err != nil {
		t.Fatal(err)
	}
	if res2.StatusCode != http.StatusBadRequest {
		t.Errorf("invalid filter status=%d, want 400", res2.StatusCode)
	}
}

// store_id filter scopes by store.
func TestListAbilities_FilterStoreID(t *testing.T) {
	s, ts := newAbilitiesTestRig(t)
	seedAbility(t, s, "store_a", "ab/one", "new")
	seedAbility(t, s, "store_b", "ab/two", "new")

	res, err := http.Get(ts.URL + "/v1/abilities?store_id=store_a")
	if err != nil {
		t.Fatal(err)
	}
	body := decode[struct {
		Abilities []Ability `json:"abilities"`
	}](t, res)
	if len(body.Abilities) != 1 || body.Abilities[0].StoreID != "store_a" {
		t.Errorf("got %+v, want one row from store_a", body.Abilities)
	}
}

// POST trust flips a 'new' row to 'trusted' and stamps trusted_hash +
// trusted_at. Returns the updated row.
func TestTrustAbility_FromNew(t *testing.T) {
	s, ts := newAbilitiesTestRig(t)
	id := seedAbility(t, s, "store_a", "ab/one", "new")

	body, _ := json.Marshal(map[string]any{})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/abilities/"+id+"/trust", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", res.StatusCode)
	}
	got := decode[Ability](t, res)
	if got.TrustState != "trusted" {
		t.Errorf("trust_state=%q, want trusted", got.TrustState)
	}
	if got.TrustedAt == "" {
		t.Errorf("trusted_at not stamped: %+v", got)
	}

	// trusted_hash should equal the row's current schema_hash.
	var schemaHash, trustedHash string
	if err := s.store.DB.QueryRow(
		`SELECT schema_hash, trusted_hash FROM abilities WHERE id=?`, id,
	).Scan(&schemaHash, &trustedHash); err != nil {
		t.Fatal(err)
	}
	if trustedHash != schemaHash {
		t.Errorf("trusted_hash=%q, want schema_hash=%q", trustedHash, schemaHash)
	}
}

// POST trust on a non-existent id returns 404 with the canonical envelope.
func TestTrustAbility_NotFound(t *testing.T) {
	_, ts := newAbilitiesTestRig(t)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/abilities/ab_missing/trust", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status=%d, want 404", res.StatusCode)
	}
	body := decode[map[string]any](t, res)
	if errObj, ok := body["error"].(map[string]any); !ok || errObj["code"] != "ability_not_found" {
		t.Errorf("error envelope = %+v, want code=ability_not_found", body)
	}
}

// POST trust on a 'schema_changed' row re-promotes it to trusted at the
// current schema_hash. trusted_hash bumps to the new hash.
func TestTrustAbility_FromSchemaChanged(t *testing.T) {
	s, ts := newAbilitiesTestRig(t)
	id := seedAbility(t, s, "store_a", "ab/one", "schema_changed")
	// Pretend the schema drifted from a previous trust; trusted_hash exists
	// but doesn't match schema_hash.
	if _, err := s.store.DB.Exec(
		`UPDATE abilities SET trusted_hash='old-hash', schema_hash='new-hash' WHERE id=?`, id,
	); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/abilities/"+id+"/trust", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", res.StatusCode)
	}
	var trustedHash, state string
	if err := s.store.DB.QueryRow(
		`SELECT trusted_hash, trust_state FROM abilities WHERE id=?`, id,
	).Scan(&trustedHash, &state); err != nil {
		t.Fatal(err)
	}
	if state != "trusted" || trustedHash != "new-hash" {
		t.Errorf("state=%q trusted_hash=%q, want trusted/new-hash", state, trustedHash)
	}
}

func TestComputeEffectiveTrust(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name           string
		trustState     string
		manifestSigned bool
		want           string
	}{
		{"manifest pre-signed, fresh", "new", true, "built-in"},
		{"manifest pre-signed, schema drift wins", "schema_changed", true, "schema_changed"},
		{"manifest pre-signed, also operator-trusted", "trusted", true, "built-in"},
		{"not in manifest, operator trusted", "trusted", false, "trusted"},
		{"not in manifest, schema changed", "schema_changed", false, "schema_changed"},
		{"not in manifest, fresh discovery", "new", false, "needs_review"},
		{"not in manifest, unknown state defaults to needs_review", "garbage", false, "needs_review"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := computeEffectiveTrust(tc.trustState, tc.manifestSigned)
			if got != tc.want {
				t.Fatalf("computeEffectiveTrust(%q, %v) = %q; want %q",
					tc.trustState, tc.manifestSigned, got, tc.want)
			}
		})
	}
}
