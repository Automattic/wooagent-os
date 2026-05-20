package pep

import (
	"strings"
	"testing"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
)

func TestDefensiveArgsPolicy_EmptyArgsOnApplyDenies(t *testing.T) {
	pol := &defensiveArgsPolicy{}
	req := Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    nil,
		Intent:  IntentApply,
		Source:  SourceOperator,
	}
	deny, reason := pol.Evaluate(req, EvalContext{})
	if !deny {
		t.Fatal("empty args + Intent=apply should deny")
	}
	if !strings.Contains(reason, "non-empty") {
		t.Errorf("reason should mention non-empty args, got %q", reason)
	}
}

func TestDefensiveArgsPolicy_EmptyArgsOnReadAllows(t *testing.T) {
	pol := &defensiveArgsPolicy{}
	req := Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/list",
		Args:    nil,
		Intent:  IntentRead,
		Source:  SourceAgent,
	}
	deny, _ := pol.Evaluate(req, EvalContext{})
	if deny {
		t.Error("empty args + Intent=read should allow")
	}
}

func TestDefensiveArgsPolicy_NormalArgsAllow(t *testing.T) {
	pol := &defensiveArgsPolicy{}
	req := Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1, "description": "x"},
		Intent:  IntentApply,
		Source:  SourceOperator,
	}
	deny, _ := pol.Evaluate(req, EvalContext{})
	if deny {
		t.Error("normal args should allow")
	}
}

func TestDefensiveArgsPolicy_OversizedArgsDenies(t *testing.T) {
	pol := &defensiveArgsPolicy{}
	// Build a payload >256 KiB by stuffing a long string into one field.
	huge := strings.Repeat("x", 300*1024) // 300 KiB of x's
	req := Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"description": huge},
		Intent:  IntentApply,
		Source:  SourceOperator,
	}
	deny, reason := pol.Evaluate(req, EvalContext{})
	if !deny {
		t.Fatal("oversized args should deny")
	}
	if !strings.Contains(reason, "256") {
		t.Errorf("reason should mention 256 KiB threshold, got %q", reason)
	}
}
