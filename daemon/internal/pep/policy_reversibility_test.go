package pep

import (
	"strings"
	"testing"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
)

// reversibilityFixture builds a manifest.Lookup with one ability at the
// given Reversibility. Used by every reversibility-policy test.
func reversibilityFixture(t *testing.T, rev float64) *manifest.Lookup {
	t.Helper()
	m := &manifest.Manifest{
		Version: 1,
		Entries: []manifest.Entry{{
			Ability:        "wooagent-products/update",
			NamespaceOwner: "test",
			SchemaHash:     "sha256:test",
			Scope:          manifest.ScopeApply,
			Reversibility:  rev,
			Personas:       []manifest.Persona{manifest.PersonaMarketing},
		}},
	}
	l, err := manifest.NewLookup(m)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	return l
}

func TestReversibilityPolicy_AgentApplyOnLowRevDenies(t *testing.T) {
	pol := &reversibilityBeltPolicy{}
	req := Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1},
		Intent:  IntentApply,
		Source:  SourceAgent,
	}
	deny, reason := pol.Evaluate(req, EvalContext{Manifest: reversibilityFixture(t, 0.2)})
	if !deny {
		t.Fatal("agent + apply + reversibility=0.2 should deny")
	}
	if !strings.Contains(reason, "operator") {
		t.Errorf("reason should reference operator mediation, got %q", reason)
	}
}

func TestReversibilityPolicy_OperatorSourcedAllow(t *testing.T) {
	pol := &reversibilityBeltPolicy{}
	req := Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1},
		Intent:  IntentApply,
		Source:  SourceOperator,
	}
	deny, _ := pol.Evaluate(req, EvalContext{Manifest: reversibilityFixture(t, 0.2)})
	if deny {
		t.Error("operator-sourced apply should allow regardless of reversibility")
	}
}

func TestReversibilityPolicy_NonApplyAllow(t *testing.T) {
	pol := &reversibilityBeltPolicy{}
	req := Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1},
		Intent:  IntentPropose,
		Source:  SourceAgent,
	}
	deny, _ := pol.Evaluate(req, EvalContext{Manifest: reversibilityFixture(t, 0.2)})
	if deny {
		t.Error("non-apply intent should allow")
	}
}

func TestReversibilityPolicy_HighRevAllow(t *testing.T) {
	pol := &reversibilityBeltPolicy{}
	req := Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1},
		Intent:  IntentApply,
		Source:  SourceAgent,
	}
	deny, _ := pol.Evaluate(req, EvalContext{Manifest: reversibilityFixture(t, 0.8)})
	if deny {
		t.Error("high-reversibility ability should allow agent-apply")
	}
}

func TestReversibilityPolicy_NoManifestEntryAllow(t *testing.T) {
	pol := &reversibilityBeltPolicy{}
	// Operator-approved-at-runtime abilities aren't in the manifest.
	// They're scoped by checkPersonaScope's nil-entry branch already.
	req := Request{
		Persona: manifest.PersonaMarketing,
		Ability: "not-in-manifest/op-approved",
		Args:    map[string]any{"id": 1},
		Intent:  IntentApply,
		Source:  SourceAgent,
	}
	// Empty manifest.
	m := &manifest.Manifest{Version: 1}
	l, err := manifest.NewLookup(m)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	deny, _ := pol.Evaluate(req, EvalContext{Manifest: l})
	if deny {
		t.Error("ability without manifest entry should allow")
	}
}
