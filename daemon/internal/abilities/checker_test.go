package abilities

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

// minimalManifest builds an in-memory manifest.Lookup containing the
// provided ability names. Each ability gets a plausible Entry shape so
// the lookup's invariants hold.
func minimalManifest(t *testing.T, names ...string) *manifest.Lookup {
	t.Helper()
	entries := make([]manifest.Entry, 0, len(names))
	for _, n := range names {
		entries = append(entries, manifest.Entry{
			Ability:        n,
			NamespaceOwner: "test/test",
			SchemaHash:     "sha256:0000000000000000000000000000000000000000000000000000000000000000",
			Scope:          manifest.ScopeRead,
			Reversibility:  1.0,
		})
	}
	body, err := json.Marshal(manifest.Manifest{Version: 1, Entries: entries})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	var m manifest.Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	lookup, err := manifest.NewLookup(&m)
	if err != nil {
		t.Fatalf("NewLookup: %v", err)
	}
	return lookup
}

// seedAbility inserts a row into the abilities table. trustState should be
// one of "new" | "trusted" | "schema_changed". revokedAt empty leaves
// revoked_at NULL.
func seedAbility(t *testing.T, st *store.Store, storeID, name, trustState, revokedAt string) {
	t.Helper()
	_, err := st.DB.Exec(
		`INSERT INTO abilities(id, store_id, name, schema_hash, trust_state, last_seen_at, created_at, updated_at, revoked_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''))`,
		"ab_"+name, storeID, name,
		"sha256:1111111111111111111111111111111111111111111111111111111111111111",
		trustState,
		"2026-05-18T00:00:00Z",
		"2026-05-18T00:00:00Z",
		"2026-05-18T00:00:00Z",
		revokedAt,
	)
	if err != nil {
		t.Fatalf("seed ability %s: %v", name, err)
	}
}

func openCheckerStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	const id = "store_test_check"
	if _, err := st.DB.Exec(
		`INSERT INTO stores(id, url, mcp_endpoint, status, paired_at, created_at, updated_at)
		 VALUES(?, ?, ?, 'paired', ?, ?, ?)`,
		id, "https://test.local", "https://test.local/wp-json/mcp/v1",
		"2026-05-18T00:00:00Z", "2026-05-18T00:00:00Z", "2026-05-18T00:00:00Z",
	); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	return st, id
}

func TestChecker_NoRowNoManifest_False(t *testing.T) {
	st, _ := openCheckerStore(t)
	c := NewChecker(st.DB, nil)
	if c.Has("woocommerce/find-products") {
		t.Errorf("Has should be false when ability is unknown and manifest is nil")
	}
}

func TestChecker_NoRowButManifestPreSign_True(t *testing.T) {
	st, _ := openCheckerStore(t)
	m := minimalManifest(t, "woocommerce/find-products")
	c := NewChecker(st.DB, m)
	if !c.Has("woocommerce/find-products") {
		t.Errorf("manifest pre-sign should make Has true even without an abilities row")
	}
}

func TestChecker_TrustedNoManifest_True(t *testing.T) {
	st, id := openCheckerStore(t)
	seedAbility(t, st, id, "woocommerce/find-products", "trusted", "")
	c := NewChecker(st.DB, nil)
	if !c.Has("woocommerce/find-products") {
		t.Errorf("trust_state=trusted should make Has true")
	}
}

func TestChecker_TrustedButRevoked_False(t *testing.T) {
	st, id := openCheckerStore(t)
	seedAbility(t, st, id, "woocommerce/find-products", "trusted", "2026-05-18T01:00:00Z")
	c := NewChecker(st.DB, nil)
	if c.Has("woocommerce/find-products") {
		t.Errorf("revoked_at set should make Has false even when trust_state=trusted")
	}
}

func TestChecker_NewState_FalseWithoutManifest(t *testing.T) {
	st, id := openCheckerStore(t)
	seedAbility(t, st, id, "woocommerce/find-products", "new", "")
	c := NewChecker(st.DB, nil)
	if c.Has("woocommerce/find-products") {
		t.Errorf("trust_state=new should make Has false without a manifest pre-sign")
	}
}

func TestChecker_NewState_TrueWithManifestPreSign(t *testing.T) {
	st, id := openCheckerStore(t)
	seedAbility(t, st, id, "woocommerce/find-products", "new", "")
	m := minimalManifest(t, "woocommerce/find-products")
	c := NewChecker(st.DB, m)
	if !c.Has("woocommerce/find-products") {
		t.Errorf("manifest pre-sign should override trust_state=new (matches PEP)")
	}
}

func TestChecker_SchemaChanged_False(t *testing.T) {
	st, id := openCheckerStore(t)
	seedAbility(t, st, id, "woocommerce/find-products", "schema_changed", "")
	c := NewChecker(st.DB, nil)
	if c.Has("woocommerce/find-products") {
		t.Errorf("trust_state=schema_changed should make Has false (PEP would also deny)")
	}
}

func TestChecker_RevokedWinsOverManifestPreSign(t *testing.T) {
	st, id := openCheckerStore(t)
	seedAbility(t, st, id, "woocommerce/find-products", "trusted", "2026-05-18T01:00:00Z")
	m := minimalManifest(t, "woocommerce/find-products")
	c := NewChecker(st.DB, m)
	if c.Has("woocommerce/find-products") {
		t.Errorf("revoked_at set should make Has false even when manifest-pre-signed (matches PEP)")
	}
}

func TestChecker_NilChecker_False(t *testing.T) {
	var c *Checker
	if c.Has("woocommerce/find-products") {
		t.Errorf("nil checker should return false, not panic")
	}
}
