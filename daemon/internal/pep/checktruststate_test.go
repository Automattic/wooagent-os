package pep

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
)

// manifestSignedHash is the default SchemaHash used by buildTestManifest for
// real-hash entries. Tests that exercise drift override on a per-entry basis.
const manifestSignedHash = "sha256:test"

// buildTestManifest constructs a minimal *manifest.Manifest. For each (name,
// hash) pair, an entry is added with that SchemaHash; if hash is empty the
// default manifestSignedHash is used. Only the ability name + hash matter
// for the trust-state branch logic under test.
func buildTestManifest(entries map[string]string) *manifest.Manifest {
	es := make([]manifest.Entry, 0, len(entries))
	for name, hash := range entries {
		if hash == "" {
			hash = manifestSignedHash
		}
		es = append(es, manifest.Entry{
			Ability:        name,
			NamespaceOwner: "test",
			SchemaHash:     hash,
			Scope:          manifest.ScopePropose,
			Reversibility:  0.5,
			Personas:       []manifest.Persona{manifest.PersonaMarketing},
		})
	}
	return &manifest.Manifest{Version: 1, Entries: es}
}

func TestCheckTrustState_Branches(t *testing.T) {
	t.Parallel()

	// The live store seeded by openTestDB. Rows that don't set storeID belong
	// to it; rows that name a different (un-seeded) store_id are orphans left
	// behind by a store that no longer exists in `stores` — checkTrustState
	// must ignore them.
	const liveStore = "store_live"

	openTestDB := func(t *testing.T) *sql.DB {
		t.Helper()
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		t.Cleanup(func() { db.Close() })
		for _, stmt := range []string{
			`CREATE TABLE stores (id TEXT PRIMARY KEY, status TEXT)`,
			`CREATE TABLE abilities (
				store_id TEXT,
				name TEXT,
				trust_state TEXT,
				revoked_at TEXT,
				schema_hash TEXT
			)`,
			// The connected store, plus an unpaired store whose row lingers
			// (token-revoke keeps the row) — its abilities must be ignored.
			`INSERT INTO stores(id, status) VALUES('store_live', 'paired')`,
			`INSERT INTO stores(id, status) VALUES('store_unpaired', 'unpaired')`,
		} {
			if _, err := db.ExecContext(context.Background(), stmt); err != nil {
				t.Fatalf("setup %q: %v", stmt, err)
			}
		}
		return db
	}

	type row struct {
		storeID    string // empty = liveStore
		name       string
		trustState string
		revokedAt  string // empty = NULL
		schemaHash string // empty = NULL
	}

	cases := []struct {
		desc       string
		rows       []row
		manifest   map[string]string // name → SchemaHash; empty hash = manifestSignedHash
		ability    string
		wantReason ReasonCode // "" means allowed
	}{
		{
			desc:       "revoked wins over manifest pre-signed",
			rows:       []row{{name: "x", trustState: "new", revokedAt: "2026-05-13T00:00:00Z", schemaHash: manifestSignedHash}},
			manifest:   map[string]string{"x": ""},
			ability:    "x",
			wantReason: ReasonAbilityRevoked,
		},
		{
			desc:       "manifest pre-signed, discovered hash matches",
			rows:       []row{{name: "x", trustState: "new", schemaHash: manifestSignedHash}},
			manifest:   map[string]string{"x": ""},
			ability:    "x",
			wantReason: "",
		},
		{
			desc:       "manifest pre-signed, discovered hash diverges: schema drift",
			rows:       []row{{name: "x", trustState: "new", schemaHash: "sha256:drifted"}},
			manifest:   map[string]string{"x": ""},
			ability:    "x",
			wantReason: ReasonSchemaDrift,
		},
		{
			desc:       "manifest pre-signed but no DB row yet: not yet discovered",
			manifest:   map[string]string{"manifest-only": ""},
			ability:    "manifest-only",
			wantReason: ReasonAbilityNotYetDiscovered,
		},
		{
			desc:       "manifest pre-signed, DB row but no hash captured: not yet discovered",
			rows:       []row{{name: "x", trustState: "new"}},
			manifest:   map[string]string{"x": ""},
			ability:    "x",
			wantReason: ReasonAbilityNotYetDiscovered,
		},
		{
			desc:       "manifest placeholder hash bypasses drift check (WC 10.9 canonicals, no DB row)",
			manifest:   map[string]string{"woocommerce/product-update": manifest.PlaceholderSchemaHash},
			ability:    "woocommerce/product-update",
			wantReason: "",
		},
		{
			desc:       "manifest placeholder hash bypasses drift check (WC 10.9 canonicals, any DB hash)",
			rows:       []row{{name: "woocommerce/product-update", trustState: "new", schemaHash: "sha256:anything"}},
			manifest:   map[string]string{"woocommerce/product-update": manifest.PlaceholderSchemaHash},
			ability:    "woocommerce/product-update",
			wantReason: "",
		},
		{
			desc:       "operator-trusted, not in manifest",
			rows:       []row{{name: "y", trustState: "trusted"}},
			ability:    "y",
			wantReason: "",
		},
		{
			desc:       "needs review (no manifest, no trusted, no revoke)",
			rows:       []row{{name: "z", trustState: "new"}},
			ability:    "z",
			wantReason: ReasonAbilityUnapproved,
		},
		{
			desc:       "ability not discovered, not in manifest: deny",
			ability:    "unknown",
			wantReason: ReasonAbilityUnapproved,
		},
		{
			desc:       "revoked but not in manifest, trust_state=trusted: still revoked",
			rows:       []row{{name: "k", trustState: "trusted", revokedAt: "2026-05-13T01:02:03Z"}},
			ability:    "k",
			wantReason: ReasonAbilityRevoked,
		},
		{
			// Regression: an orphaned row from a store that no
			// longer exists (here with a stale/old-format hash) must be
			// ignored. The live store's matching row decides the call.
			desc: "orphan store row with drifted hash is ignored; live row matches",
			rows: []row{
				{storeID: "store_gone", name: "x", trustState: "new", schemaHash: "old-format-no-prefix"},
				{storeID: liveStore, name: "x", trustState: "new", schemaHash: manifestSignedHash},
			},
			manifest:   map[string]string{"x": ""},
			ability:    "x",
			wantReason: "",
		},
		{
			// Only an orphan row exists — the live store hasn't discovered
			// this ability. Must read as not-yet-discovered, never as the
			// orphan's (mismatched) hash → schema_drift.
			desc:       "only an orphan store row exists: not yet discovered",
			rows:       []row{{storeID: "store_gone", name: "x", trustState: "new", schemaHash: "old-format-no-prefix"}},
			manifest:   map[string]string{"x": ""},
			ability:    "x",
			wantReason: ReasonAbilityNotYetDiscovered,
		},
		{
			// A store whose row lingers as 'unpaired' (token-revoke path) must
			// be excluded just like a deleted one — the paired store decides.
			desc: "unpaired store row with drifted hash is ignored; paired row matches",
			rows: []row{
				{storeID: "store_unpaired", name: "x", trustState: "new", schemaHash: "old-format-no-prefix"},
				{storeID: liveStore, name: "x", trustState: "new", schemaHash: manifestSignedHash},
			},
			manifest:   map[string]string{"x": ""},
			ability:    "x",
			wantReason: "",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			for _, r := range tc.rows {
				var revoked, hash any
				if r.revokedAt != "" {
					revoked = r.revokedAt
				}
				if r.schemaHash != "" {
					hash = r.schemaHash
				}
				storeID := r.storeID
				if storeID == "" {
					storeID = liveStore
				}
				if _, err := db.ExecContext(context.Background(),
					`INSERT INTO abilities(store_id, name, trust_state, revoked_at, schema_hash) VALUES(?, ?, ?, ?, ?)`,
					storeID, r.name, r.trustState, revoked, hash); err != nil {
					t.Fatalf("insert: %v", err)
				}
			}
			lookup, err := manifest.NewLookup(buildTestManifest(tc.manifest))
			if err != nil {
				t.Fatalf("manifest lookup: %v", err)
			}
			p := &PEP{manifest: lookup, db: db, schemas: &schemaCache{}}
			got := p.checkTrustState(context.Background(), Request{Ability: tc.ability})
			if got != tc.wantReason {
				t.Fatalf("checkTrustState(%q) = %q; want %q", tc.ability, got, tc.wantReason)
			}
		})
	}
}
