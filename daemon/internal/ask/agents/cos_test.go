package agents

import (
	"strings"
	"testing"
)

func TestCoSPrompt_SubstitutesStoreName(t *testing.T) {
	out := CoSPrompt("Linenly")
	if strings.Contains(out, "{{store_name}}") {
		t.Fatalf("unsubstituted token in prompt")
	}
	if !strings.Contains(out, "Linenly") {
		t.Fatalf("store name not present in prompt")
	}
}

func TestCoSPrompt_FallsBackOnEmpty(t *testing.T) {
	out := CoSPrompt("")
	if strings.Contains(out, "{{store_name}}") {
		t.Fatalf("unsubstituted token when name empty")
	}
	if !strings.Contains(out, "the store") {
		t.Fatalf("expected fallback 'the store' in prompt")
	}
}

func TestCoSPrompt_FallsBackOnWhitespace(t *testing.T) {
	out := CoSPrompt("   ")
	if !strings.Contains(out, "the store") {
		t.Fatalf("expected fallback for whitespace-only name")
	}
}

func TestCoSPrompt_ContainsCoreSections(t *testing.T) {
	out := CoSPrompt("Linenly")
	// Sanity-check that the spike-vetted v2 prompt was loaded.
	for _, want := range []string{
		"# Your job",
		"# Your team",
		"dispatch_persona",
		"# Heuristics",
		"Time interpretation",
		"Marketing — about ten seconds",
		"Pricing — about a minute",
		"Sales Support — about fifteen seconds",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("prompt missing expected section: %q", want)
		}
	}
}
