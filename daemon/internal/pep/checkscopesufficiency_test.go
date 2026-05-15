package pep

import (
	"testing"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
)

// pepWithManifest builds a PEP that only has its manifest field populated —
// checkScopeSufficiency doesn't touch the DB, MCP, or audit writer, so the
// minimum useful fixture is just a Lookup.
func pepWithManifest(t *testing.T, entries []manifest.Entry) *PEP {
	t.Helper()
	lookup, err := manifest.NewLookup(&manifest.Manifest{Version: 1, Entries: entries})
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	return &PEP{manifest: lookup, schemas: &schemaCache{}}
}

func TestCheckScopeSufficiency_Table(t *testing.T) {
	entry := manifest.Entry{
		Ability:        "test/ability",
		NamespaceOwner: "test",
		SchemaHash:     "sha256:test",
		Personas:       []manifest.Persona{manifest.PersonaMarketing},
	}

	cases := []struct {
		name       string
		source     Source
		intent     Intent
		scope      manifest.Scope
		inMani     bool
		wantReason ReasonCode
	}{
		{"operator-apply-against-propose", SourceOperator, IntentApply, manifest.ScopePropose, true, ""},
		{"operator-apply-against-read", SourceOperator, IntentApply, manifest.ScopeRead, true, ""},
		{"agent-apply-against-apply", SourceAgent, IntentApply, manifest.ScopeApply, true, ""},
		{"agent-apply-against-propose", SourceAgent, IntentApply, manifest.ScopePropose, true, ReasonScopeInsufficient},
		{"agent-propose-against-propose", SourceAgent, IntentPropose, manifest.ScopePropose, true, ""},
		{"agent-propose-against-read", SourceAgent, IntentPropose, manifest.ScopeRead, true, ReasonScopeInsufficient},
		{"agent-read-against-read", SourceAgent, IntentRead, manifest.ScopeRead, true, ""},
		{"agent-read-against-propose", SourceAgent, IntentRead, manifest.ScopePropose, true, ""},
		{"unset-source-apply-propose", Source(""), IntentApply, manifest.ScopePropose, true, ReasonScopeInsufficient},
		{"unset-source-read-read", Source(""), IntentRead, manifest.ScopeRead, true, ReasonScopeInsufficient},
		{"agent-unset-intent-apply-scope", SourceAgent, Intent(""), manifest.ScopeApply, true, ReasonScopeInsufficient},
		{"no-manifest-entry-passes", SourceAgent, IntentApply, manifest.ScopePropose, false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := entry
			e.Scope = tc.scope
			entries := []manifest.Entry{e}
			req := Request{Ability: "test/ability", Persona: manifest.PersonaMarketing, Intent: tc.intent, Source: tc.source}
			if !tc.inMani {
				entries = nil
				req.Ability = "some/other-ability"
			}
			p := pepWithManifest(t, entries)
			got := p.checkScopeSufficiency(req)
			if got != tc.wantReason {
				t.Errorf("got reason %q, want %q", got, tc.wantReason)
			}
		})
	}
}
