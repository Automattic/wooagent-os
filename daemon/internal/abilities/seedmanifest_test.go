package abilities

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
)

// manifestWithHashes returns a Manifest with one entry per (name, hash) pair.
// Only the fields the seeder reads matter; everything else is a stub.
func manifestWithHashes(entries map[string]string) *manifest.Manifest {
	es := make([]manifest.Entry, 0, len(entries))
	for name, hash := range entries {
		es = append(es, manifest.Entry{
			Ability:        name,
			NamespaceOwner: "test",
			SchemaHash:     hash,
			Scope:          manifest.ScopePropose,
			Reversibility:  0.5,
			Personas:       []manifest.Persona{manifest.PersonaMarketing},
			Description:    "test ability " + name,
			PluginVersion:  "1.0.0",
		})
	}
	return &manifest.Manifest{Version: 1, Entries: es}
}

// Fresh table + manifest with three entries → SeedManifest inserts a row
// per entry, all with the manifest's schema_hash and trust_state="new".
func TestSeedManifest_FreshTable_InsertsAllEntries(t *testing.T) {
	r, st := newRunner(t, &fakeMCP{})
	storeID := seedPairedStore(t, st)
	r.Manifest = manifestWithHashes(map[string]string{
		"wooagent-products/list":  "sha256:aaa",
		"wooagent-orders/get":     "sha256:bbb",
		"wooagent-customers/list": "sha256:ccc",
	})

	inserted, err := r.SeedManifest(context.Background(), storeID)
	if err != nil {
		t.Fatalf("SeedManifest: %v", err)
	}
	if inserted != 3 {
		t.Fatalf("inserted=%d, want 3", inserted)
	}

	rows, err := st.DB.Query(
		`SELECT name, schema_hash, trust_state, trusted_hash
		 FROM abilities WHERE store_id = ? ORDER BY name`, storeID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]struct {
		hash, state string
		trusted     sql.NullString
	}{}
	for rows.Next() {
		var n, h, s string
		var th sql.NullString
		if err := rows.Scan(&n, &h, &s, &th); err != nil {
			t.Fatal(err)
		}
		got[n] = struct {
			hash, state string
			trusted     sql.NullString
		}{h, s, th}
	}
	if len(got) != 3 {
		t.Fatalf("rows=%d, want 3", len(got))
	}
	for name, want := range map[string]string{
		"wooagent-products/list":  "sha256:aaa",
		"wooagent-orders/get":     "sha256:bbb",
		"wooagent-customers/list": "sha256:ccc",
	} {
		g, ok := got[name]
		if !ok {
			t.Errorf("missing seeded row for %s", name)
			continue
		}
		if g.hash != want {
			t.Errorf("%s: hash=%q, want %q", name, g.hash, want)
		}
		if g.state != "new" {
			t.Errorf("%s: state=%q, want new", name, g.state)
		}
		if g.trusted.Valid {
			t.Errorf("%s: trusted_hash set on seeded row", name)
		}
	}
}

// SeedManifest must not overwrite a row that's already present — the
// operator-approved trusted_hash and a previously-discovered schema_hash
// outrank the manifest's claim.
func TestSeedManifest_PreservesExistingRows(t *testing.T) {
	r, st := newRunner(t, &fakeMCP{})
	storeID := seedPairedStore(t, st)
	r.Manifest = manifestWithHashes(map[string]string{
		"wooagent-products/list": "sha256:manifest-hash",
		"wooagent-orders/get":    "sha256:other-manifest-hash",
	})

	// Existing row for products/list: trusted at the live hash, distinct
	// from the manifest's hash. Seeding must leave this row untouched.
	_, err := st.DB.Exec(
		`INSERT INTO abilities(id, store_id, name, schema_hash, trust_state,
		                       trusted_hash, trusted_at, last_seen_at,
		                       created_at, updated_at)
		 VALUES('ab_existing', ?, 'wooagent-products/list', 'sha256:live-hash',
		        'trusted', 'sha256:live-hash', '2026-05-21T00:00:00Z',
		        '2026-05-21T00:00:00Z', '2026-05-21T00:00:00Z', '2026-05-21T00:00:00Z')`,
		storeID,
	)
	if err != nil {
		t.Fatalf("seed existing row: %v", err)
	}

	inserted, err := r.SeedManifest(context.Background(), storeID)
	if err != nil {
		t.Fatalf("SeedManifest: %v", err)
	}
	if inserted != 1 {
		t.Fatalf("inserted=%d, want 1 (only orders/get is missing)", inserted)
	}

	// The pre-existing row keeps its live hash + trusted state.
	var hash, state string
	var trusted sql.NullString
	if err := st.DB.QueryRow(
		`SELECT schema_hash, trust_state, trusted_hash FROM abilities
		 WHERE store_id = ? AND name = 'wooagent-products/list'`, storeID,
	).Scan(&hash, &state, &trusted); err != nil {
		t.Fatal(err)
	}
	if hash != "sha256:live-hash" {
		t.Errorf("existing row hash overwritten: %q", hash)
	}
	if state != "trusted" {
		t.Errorf("existing row state overwritten: %q", state)
	}
	if !trusted.Valid || trusted.String != "sha256:live-hash" {
		t.Errorf("existing row trusted_hash overwritten: %+v", trusted)
	}
}

// A nil Manifest (or a Manifest with zero entries) is a valid configuration
// — tests construct Runners that don't exercise seeding. The method must
// short-circuit cleanly without touching the DB.
func TestSeedManifest_NilManifest_NoOp(t *testing.T) {
	r, st := newRunner(t, &fakeMCP{})
	storeID := seedPairedStore(t, st)
	r.Manifest = nil

	inserted, err := r.SeedManifest(context.Background(), storeID)
	if err != nil {
		t.Fatalf("SeedManifest with nil manifest: %v", err)
	}
	if inserted != 0 {
		t.Errorf("inserted=%d, want 0", inserted)
	}
}

// Idempotency: calling SeedManifest twice with the same manifest inserts
// rows on the first call and zero on the second.
func TestSeedManifest_IsIdempotent(t *testing.T) {
	r, st := newRunner(t, &fakeMCP{})
	storeID := seedPairedStore(t, st)
	r.Manifest = manifestWithHashes(map[string]string{
		"wooagent-products/list": "sha256:aaa",
		"wooagent-orders/get":    "sha256:bbb",
	})

	first, err := r.SeedManifest(context.Background(), storeID)
	if err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if first != 2 {
		t.Fatalf("first inserted=%d, want 2", first)
	}

	second, err := r.SeedManifest(context.Background(), storeID)
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if second != 0 {
		t.Errorf("second inserted=%d, want 0 (idempotent)", second)
	}
}

// SeedAll iterates paired stores and seeds each. Non-paired rows in the
// stores table are skipped (a SELECT WHERE status='paired' guards this).
func TestSeedAll_SeedsEveryPairedStore(t *testing.T) {
	r, st := newRunner(t, &fakeMCP{})

	// Two paired stores + one not-yet-paired store.
	_, err := st.DB.Exec(
		`INSERT INTO stores(id, url, mcp_endpoint, status, token_ref,
		                    paired_at, created_at, updated_at)
		 VALUES
		   ('s1', 'https://a.example', 'https://a.example/mcp', 'paired',
		    'wooagent.stores.s1', '2026-05-21T00:00:00Z',
		    '2026-05-21T00:00:00Z', '2026-05-21T00:00:00Z'),
		   ('s2', 'https://b.example', 'https://b.example/mcp', 'paired',
		    'wooagent.stores.s2', '2026-05-21T00:00:00Z',
		    '2026-05-21T00:00:00Z', '2026-05-21T00:00:00Z'),
		   ('s3', 'https://c.example', 'https://c.example/mcp', 'pending',
		    NULL, NULL, '2026-05-21T00:00:00Z', '2026-05-21T00:00:00Z')`,
	)
	if err != nil {
		t.Fatalf("seed stores: %v", err)
	}

	r.Manifest = manifestWithHashes(map[string]string{
		"wooagent-products/list": "sha256:aaa",
		"wooagent-orders/get":    "sha256:bbb",
	})

	r.SeedAll(context.Background())

	// Both paired stores get 2 rows. The pending store gets 0.
	for storeID, want := range map[string]int{"s1": 2, "s2": 2, "s3": 0} {
		var got int
		if err := st.DB.QueryRow(
			`SELECT COUNT(*) FROM abilities WHERE store_id = ?`, storeID,
		).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("store %s: rows=%d, want %d", storeID, got, want)
		}
	}
}

// End-to-end story: seed sets the row's schema_hash to the manifest hash;
// a follow-up discovery against a live store that disagrees flips the
// row's schema_hash to the live (mismatched) value. From the PEP's view,
// the row now has schema_hash != entry.SchemaHash → drift. This test
// stops short of running the PEP itself (pep_test.go covers the gate
// logic); it asserts the DB-level transition that drives the gate.
func TestSeedThenDiscover_FlipsHashOnDrift(t *testing.T) {
	const ability = "wooagent-products/list"
	fake := &fakeMCP{
		abilities: []mcp.AbilitySummary{{Name: ability}},
		infos: map[string]mcp.AbilityInfo{
			// Distinct description from the manifest entry forces the
			// canonical hash to differ from the seeded manifest hash.
			ability: {Name: ability, Description: "live-schema-different-from-manifest"},
		},
	}
	r, st := newRunner(t, fake)
	storeID := seedPairedStore(t, st)
	r.Manifest = manifestWithHashes(map[string]string{
		ability: "sha256:manifest-claims-this",
	})

	// Step 1: seed → row has the manifest's hash.
	if _, err := r.SeedManifest(context.Background(), storeID); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var hashAfterSeed string
	if err := st.DB.QueryRow(
		`SELECT schema_hash FROM abilities WHERE store_id = ? AND name = ?`,
		storeID, ability,
	).Scan(&hashAfterSeed); err != nil {
		t.Fatal(err)
	}
	if hashAfterSeed != "sha256:manifest-claims-this" {
		t.Fatalf("post-seed hash=%q, want manifest hash", hashAfterSeed)
	}

	// Step 2: discovery runs → row's hash is replaced with the live
	// canonical hash, which is some real sha256 value that's definitely
	// not the manifest's "sha256:manifest-claims-this".
	if _, err := r.RunForStore(context.Background(), storeID); err != nil {
		t.Fatalf("RunForStore: %v", err)
	}
	var hashAfterDiscover string
	if err := st.DB.QueryRow(
		`SELECT schema_hash FROM abilities WHERE store_id = ? AND name = ?`,
		storeID, ability,
	).Scan(&hashAfterDiscover); err != nil {
		t.Fatal(err)
	}
	if hashAfterDiscover == "sha256:manifest-claims-this" {
		t.Errorf("post-discover hash unchanged from seed; drift not captured")
	}
	if hashAfterDiscover == "" {
		t.Errorf("post-discover hash empty")
	}
}
