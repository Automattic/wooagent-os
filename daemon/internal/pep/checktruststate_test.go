package pep

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
)

// buildTestManifest constructs a minimal *manifest.Manifest with one entry
// per name in the slice. Schema fields are filled with stub values; only the
// ability name matters for the trust-state branch logic under test.
func buildTestManifest(names []string) *manifest.Manifest {
	entries := make([]manifest.Entry, 0, len(names))
	for _, n := range names {
		entries = append(entries, manifest.Entry{
			Ability:        n,
			NamespaceOwner: "test",
			SchemaHash:     "sha256:test",
			Scope:          manifest.ScopePropose,
			Reversibility:  0.5,
			Personas:       []manifest.Persona{manifest.PersonaMarketing},
		})
	}
	return &manifest.Manifest{Version: 1, Entries: entries}
}

func TestCheckTrustState_Branches(t *testing.T) {
	t.Parallel()

	openTestDB := func(t *testing.T) *sql.DB {
		t.Helper()
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		t.Cleanup(func() { db.Close() })
		if _, err := db.ExecContext(context.Background(),
			`CREATE TABLE abilities (
				name TEXT,
				trust_state TEXT,
				revoked_at TEXT
			)`); err != nil {
			t.Fatalf("create: %v", err)
		}
		return db
	}

	type row struct {
		name       string
		trustState string
		revokedAt  string // empty = NULL
	}

	cases := []struct {
		desc        string
		rows        []row
		manifestSet []string
		ability     string
		wantReason  ReasonCode // "" means allowed
	}{
		{
			desc:        "revoked wins over manifest pre-signed",
			rows:        []row{{name: "x", trustState: "new", revokedAt: "2026-05-13T00:00:00Z"}},
			manifestSet: []string{"x"},
			ability:     "x",
			wantReason:  ReasonAbilityRevoked,
		},
		{
			desc:        "manifest pre-signed, never revoked",
			rows:        []row{{name: "x", trustState: "new"}},
			manifestSet: []string{"x"},
			ability:     "x",
			wantReason:  "",
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
			desc:        "ability not discovered yet but manifest-signed: allow via manifest fallback",
			manifestSet: []string{"manifest-only"},
			ability:     "manifest-only",
			wantReason:  "",
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
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			db := openTestDB(t)
			for _, r := range tc.rows {
				var revoked any
				if r.revokedAt != "" {
					revoked = r.revokedAt
				}
				if _, err := db.ExecContext(context.Background(),
					`INSERT INTO abilities(name, trust_state, revoked_at) VALUES(?, ?, ?)`,
					r.name, r.trustState, revoked); err != nil {
					t.Fatalf("insert: %v", err)
				}
			}
			lookup, err := manifest.NewLookup(buildTestManifest(tc.manifestSet))
			if err != nil {
				t.Fatalf("manifest lookup: %v", err)
			}
			p := &PEP{manifest: lookup, db: db}
			got := p.checkTrustState(context.Background(), Request{Ability: tc.ability})
			if got != tc.wantReason {
				t.Fatalf("checkTrustState(%q) = %q; want %q", tc.ability, got, tc.wantReason)
			}
		})
	}
}
