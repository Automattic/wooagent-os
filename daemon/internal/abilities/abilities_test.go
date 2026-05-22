package abilities

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"
	"github.com/zalando/go-keyring"

	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/secrets"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

// fakeMCP is the test double for the abilities.Client interface.
type fakeMCP struct {
	abilities []mcp.AbilitySummary
	infos     map[string]mcp.AbilityInfo
	infoErr   error
}

func (f *fakeMCP) Initialize(_ context.Context) (mcp.ServerInfo, error) {
	return mcp.ServerInfo{}, nil
}
func (f *fakeMCP) DiscoverAbilities(_ context.Context) ([]mcp.AbilitySummary, error) {
	return f.abilities, nil
}
func (f *fakeMCP) GetAbilityInfo(_ context.Context, name string) (mcp.AbilityInfo, error) {
	if f.infoErr != nil {
		return mcp.AbilityInfo{}, f.infoErr
	}
	if info, ok := f.infos[name]; ok {
		return info, nil
	}
	return mcp.AbilityInfo{Name: name}, nil
}

// memSecrets is the same in-process keychain backend the httpapi tests use.
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
	return keyring.Delete("WooAgent OS", k)
}

func newRunner(t *testing.T, fake *fakeMCP) (*Runner, *store.Store) {
	t.Helper()
	keyring.MockInit()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r := New(st.DB, memSecrets{}, nil)
	r.PollInterval = 0 // tests drive RunForStore explicitly
	r.NewClient = func(_ context.Context, _, _ string) (Client, error) {
		return fake, nil
	}
	return r, st
}

// seedPairedStore inserts a paired stores row + token in the keychain so
// RunForStore has a complete pairing to discover against.
func seedPairedStore(t *testing.T, st *store.Store) string {
	t.Helper()
	const id = "store_test_1"
	_, err := st.DB.Exec(
		`INSERT INTO stores(id, url, mcp_endpoint, status, token_ref, paired_at, created_at, updated_at)
		 VALUES(?, ?, ?, 'paired', ?, ?, ?, ?)`,
		id, "https://test.local", "https://test.local/wp-json/mcp/v1",
		"wooagent.stores."+id, "2026-05-07T00:00:00Z", "2026-05-07T00:00:00Z", "2026-05-07T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("seed store: %v", err)
	}
	if err := keyring.Set("WooAgent OS", "wooagent.stores."+id, "device-token-xyz"); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	return id
}

// First-time discovery inserts every ability in 'new' state. Each row has
// a populated schema_hash; trusted_hash is NULL until the operator approves.
func TestRunForStore_FreshDiscovery_AllNew(t *testing.T) {
	fake := &fakeMCP{
		abilities: []mcp.AbilitySummary{
			{Name: "wooagent-products/list"},
			{Name: "wooagent-orders/get"},
		},
		infos: map[string]mcp.AbilityInfo{
			"wooagent-products/list": {Name: "wooagent-products/list", Description: "list products"},
			"wooagent-orders/get":    {Name: "wooagent-orders/get", Description: "get order"},
		},
	}
	r, st := newRunner(t, fake)
	id := seedPairedStore(t, st)

	count, err := r.RunForStore(context.Background(), id)
	if err != nil {
		t.Fatalf("RunForStore: %v", err)
	}
	if count != 2 {
		t.Fatalf("count=%d, want 2", count)
	}

	rows, err := st.DB.Query(`SELECT name, trust_state, trusted_hash, schema_hash FROM abilities WHERE store_id = ? ORDER BY name`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := []struct {
		name, state, hash string
		trusted           sql.NullString
	}{}
	for rows.Next() {
		var n, s, h string
		var th sql.NullString
		if err := rows.Scan(&n, &s, &th, &h); err != nil {
			t.Fatal(err)
		}
		got = append(got, struct {
			name, state, hash string
			trusted           sql.NullString
		}{n, s, h, th})
	}
	if len(got) != 2 {
		t.Fatalf("rows=%d, want 2", len(got))
	}
	for _, g := range got {
		if g.state != "new" {
			t.Errorf("%s: state=%q, want new", g.name, g.state)
		}
		if g.trusted.Valid {
			t.Errorf("%s: trusted_hash set on first sighting", g.name)
		}
		if g.hash == "" {
			t.Errorf("%s: missing schema_hash", g.name)
		}
	}

	// stores.ability_count + last_discovered_at are denormalized after a
	// successful run.
	var count2 sql.NullInt64
	var disc sql.NullString
	if err := st.DB.QueryRow(`SELECT ability_count, last_discovered_at FROM stores WHERE id=?`, id).Scan(&count2, &disc); err != nil {
		t.Fatal(err)
	}
	if !count2.Valid || count2.Int64 != 2 {
		t.Errorf("ability_count=%v, want 2", count2)
	}
	if !disc.Valid || disc.String == "" {
		t.Errorf("last_discovered_at not set: %+v", disc)
	}
}

// A re-discover that returns the same schemas leaves trust state alone:
// 'new' rows stay 'new', and an externally-trusted row stays 'trusted'.
func TestRunForStore_ReDiscoverNoChange_PreservesTrust(t *testing.T) {
	fake := &fakeMCP{
		abilities: []mcp.AbilitySummary{{Name: "ab/one"}},
		infos:     map[string]mcp.AbilityInfo{"ab/one": {Name: "ab/one", Description: "v1"}},
	}
	r, st := newRunner(t, fake)
	id := seedPairedStore(t, st)

	if _, err := r.RunForStore(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	// Promote to trusted as if the operator clicked Approve.
	if _, err := st.DB.Exec(
		`UPDATE abilities SET trust_state='trusted', trusted_hash=schema_hash, trusted_at='2026-05-07T00:00:00Z'`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := r.RunForStore(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	var state string
	var trustedHash sql.NullString
	if err := st.DB.QueryRow(`SELECT trust_state, trusted_hash FROM abilities WHERE name='ab/one'`).Scan(&state, &trustedHash); err != nil {
		t.Fatal(err)
	}
	if state != "trusted" {
		t.Errorf("state=%q after re-discover, want trusted", state)
	}
	if !trustedHash.Valid || trustedHash.String == "" {
		t.Errorf("trusted_hash lost: %+v", trustedHash)
	}
}

// A previously-trusted ability whose schema drifts upstream flips to
// 'schema_changed'. trusted_hash is preserved so the UI can show the diff
// + so a rollback automatically re-promotes to 'trusted' on next discovery.
func TestRunForStore_SchemaDrift_FlipsToSchemaChanged(t *testing.T) {
	fake := &fakeMCP{
		abilities: []mcp.AbilitySummary{{Name: "ab/one"}},
		infos:     map[string]mcp.AbilityInfo{"ab/one": {Name: "ab/one", Description: "v1"}},
	}
	r, st := newRunner(t, fake)
	id := seedPairedStore(t, st)

	if _, err := r.RunForStore(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(
		`UPDATE abilities SET trust_state='trusted', trusted_hash=schema_hash, trusted_at='2026-05-07T00:00:00Z'`,
	); err != nil {
		t.Fatal(err)
	}

	// Schema drift: same ability, different description → different hash.
	fake.infos["ab/one"] = mcp.AbilityInfo{Name: "ab/one", Description: "v2 — added a required arg"}
	if _, err := r.RunForStore(context.Background(), id); err != nil {
		t.Fatal(err)
	}

	var state, schemaHash string
	var trustedHash sql.NullString
	if err := st.DB.QueryRow(`SELECT trust_state, schema_hash, trusted_hash FROM abilities WHERE name='ab/one'`).Scan(&state, &schemaHash, &trustedHash); err != nil {
		t.Fatal(err)
	}
	if state != "schema_changed" {
		t.Errorf("state=%q, want schema_changed", state)
	}
	if !trustedHash.Valid || trustedHash.String == "" || trustedHash.String == schemaHash {
		t.Errorf("trusted_hash should be preserved and differ from schema_hash; got trusted=%v schema=%q", trustedHash, schemaHash)
	}
}

// A schema rollback (upstream reverts to the previously-trusted hash) flips
// the row from 'schema_changed' back to 'trusted' automatically — no
// operator action required.
func TestRunForStore_SchemaRollback_ReturnsToTrusted(t *testing.T) {
	fake := &fakeMCP{
		abilities: []mcp.AbilitySummary{{Name: "ab/one"}},
		infos:     map[string]mcp.AbilityInfo{"ab/one": {Name: "ab/one", Description: "v1"}},
	}
	r, st := newRunner(t, fake)
	id := seedPairedStore(t, st)

	// 1: discover v1, promote to trusted.
	if _, err := r.RunForStore(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`UPDATE abilities SET trust_state='trusted', trusted_hash=schema_hash, trusted_at='2026-05-07T00:00:00Z'`); err != nil {
		t.Fatal(err)
	}
	// 2: drift to v2.
	fake.infos["ab/one"] = mcp.AbilityInfo{Name: "ab/one", Description: "v2"}
	if _, err := r.RunForStore(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	// 3: rollback to v1.
	fake.infos["ab/one"] = mcp.AbilityInfo{Name: "ab/one", Description: "v1"}
	if _, err := r.RunForStore(context.Background(), id); err != nil {
		t.Fatal(err)
	}

	var state string
	if err := st.DB.QueryRow(`SELECT trust_state FROM abilities WHERE name='ab/one'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "trusted" {
		t.Errorf("state=%q after rollback, want trusted", state)
	}
}

// Abilities removed upstream are deleted from the cache. Trust does not
// outlive removal; if the ability is reinstalled it comes back as 'new'.
func TestRunForStore_RemovedAbility_IsDeleted(t *testing.T) {
	fake := &fakeMCP{
		abilities: []mcp.AbilitySummary{{Name: "ab/one"}, {Name: "ab/two"}},
		infos: map[string]mcp.AbilityInfo{
			"ab/one": {Name: "ab/one"},
			"ab/two": {Name: "ab/two"},
		},
	}
	r, st := newRunner(t, fake)
	id := seedPairedStore(t, st)

	if _, err := r.RunForStore(context.Background(), id); err != nil {
		t.Fatal(err)
	}

	fake.abilities = []mcp.AbilitySummary{{Name: "ab/one"}}
	delete(fake.infos, "ab/two")
	if _, err := r.RunForStore(context.Background(), id); err != nil {
		t.Fatal(err)
	}

	var n int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM abilities WHERE store_id=?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("rows=%d after removal, want 1", n)
	}
}

// Non-paired stores are a no-op. The pairing approval path can call
// RunForStore without first checking status.
func TestRunForStore_SkipsUnpairedStore(t *testing.T) {
	fake := &fakeMCP{}
	r, st := newRunner(t, fake)
	const id = "store_pending"
	_, err := st.DB.Exec(
		`INSERT INTO stores(id, url, mcp_endpoint, status, created_at, updated_at)
		 VALUES(?, 'https://x', 'https://x/wp-json/mcp/v1', 'pairing', '2026-05-07', '2026-05-07')`,
		id,
	)
	if err != nil {
		t.Fatal(err)
	}
	count, err := r.RunForStore(context.Background(), id)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if count != 0 {
		t.Errorf("count=%d, want 0", count)
	}
}

// canonicalize is order-independent for permissions/scopes and stable
// across re-marshals — same logical schema, same hash.
func TestCanonicalize_StableHash(t *testing.T) {
	a := mcp.AbilityInfo{
		Name:           "ab",
		Permissions:    []string{"manage_options", "edit_posts"},
		RequiredScopes: []string{"products:write", "orders:read"},
	}
	b := mcp.AbilityInfo{
		Name:           "ab",
		Permissions:    []string{"edit_posts", "manage_options"},
		RequiredScopes: []string{"orders:read", "products:write"},
	}
	_, ha, err := canonicalize(a)
	if err != nil {
		t.Fatal(err)
	}
	_, hb, err := canonicalize(b)
	if err != nil {
		t.Fatal(err)
	}
	if ha != hb {
		t.Errorf("hash should be order-independent: %s vs %s", ha, hb)
	}
}
