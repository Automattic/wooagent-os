package manifest

import (
	"strings"
	"testing"
)

// TestDefaultLoads is the primary smoke test for the embedded seed manifest.
// If this fails, the binary cannot start — the shipped default is broken.
func TestDefaultLoads(t *testing.T) {
	m, err := Default()
	if err != nil {
		t.Fatalf("Default() err: %v", err)
	}
	if m.Version != 1 {
		t.Errorf("unexpected version: %d", m.Version)
	}
	if len(m.Entries) == 0 {
		t.Fatal("default manifest has no entries")
	}

	lookup, err := NewLookup(m)
	if err != nil {
		t.Fatalf("NewLookup: %v", err)
	}
	if lookup.Size() != len(m.Entries) {
		t.Errorf("lookup size %d != entries %d", lookup.Size(), len(m.Entries))
	}
}

// TestCompanionPluginBaselineEntries ensures the ten Companion-Plugin
// abilities we promise as the §11.4 baseline are all pre-signed. A missing
// entry here means we shipped a daemon that can't invoke something our own
// plugin registers.
func TestCompanionPluginBaselineEntries(t *testing.T) {
	m, err := Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	lookup, _ := NewLookup(m)
	baseline := []string{
		"wooagent-products/list",
		"wooagent-products/get",
		"wooagent-products/update",
		"wooagent-orders/list",
		"wooagent-orders/get",
		"wooagent-orders/add-note",
		"wooagent-customers/get",
		"wooagent-device-pair/request",
		"wooagent-device-pair/confirm",
		"wooagent-device-pair/revoke",
	}
	for _, name := range baseline {
		e := lookup.Get(name)
		if e == nil {
			t.Errorf("missing baseline ability: %s", name)
			continue
		}
		if e.NamespaceOwner != "wooagent-os/companion-plugin" {
			t.Errorf("%s: unexpected owner %q", name, e.NamespaceOwner)
		}
		if !strings.HasPrefix(e.SchemaHash, "sha256:") {
			t.Errorf("%s: schema_hash not sha256-prefixed: %q", name, e.SchemaHash)
		}
	}
}

// TestReadAbilitiesAreFullyReversible confirms that every read-scope entry
// has reversibility 1.0 — reads can never cause state change, so this is
// an invariant.
func TestReadAbilitiesAreFullyReversible(t *testing.T) {
	m, _ := Default()
	for _, e := range m.Entries {
		if e.Scope == ScopeRead && e.Reversibility != 1.0 {
			t.Errorf("%s: scope=read but reversibility=%.2f (must be 1.0)", e.Ability, e.Reversibility)
		}
	}
}

// TestDevicePairAbilitiesHaveNoPersonas guards the design choice that
// device-pair/* is daemon-bootstrapping, never an agent path. If an
// enthusiastic future entry adds a persona here, invoke-by-agent becomes
// possible and the whole auth flow is at risk.
func TestDevicePairAbilitiesHaveNoPersonas(t *testing.T) {
	m, _ := Default()
	for _, e := range m.Entries {
		if strings.HasPrefix(e.Ability, "wooagent-device-pair/") && len(e.Personas) != 0 {
			t.Errorf("%s must have no personas; got %v", e.Ability, e.Personas)
		}
	}
}

// TestSchemaHashCanonical verifies that semantically-equivalent schemas with
// different key ordering / whitespace produce the same hash. This is the
// property drift detection relies on.
func TestSchemaHashCanonical(t *testing.T) {
	a := []byte(`{"type":"object","properties":{"id":{"type":"integer"},"name":{"type":"string"}}}`)
	b := []byte(`
		{
		  "properties": {
		    "name": { "type":"string" },
		    "id":   { "type":"integer" }
		  },
		  "type": "object"
		}`)
	out := []byte(`{"type":"object"}`)
	ha, err := SchemaHash(a, out)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := SchemaHash(b, out)
	if err != nil {
		t.Fatal(err)
	}
	if ha != hb {
		t.Errorf("hash mismatch:\n  a=%s\n  b=%s", ha, hb)
	}
}

// TestSchemaHashDetectsChange verifies that adding a field produces a
// different hash — the basic requirement of drift detection.
func TestSchemaHashDetectsChange(t *testing.T) {
	original := []byte(`{"type":"object","properties":{"id":{"type":"integer"}}}`)
	changed := []byte(`{"type":"object","properties":{"id":{"type":"integer"},"new_field":{"type":"string"}}}`)
	out := []byte(`null`)
	h1, _ := SchemaHash(original, out)
	h2, _ := SchemaHash(changed, out)
	if h1 == h2 {
		t.Error("expected different hashes after field addition, got same")
	}
}
