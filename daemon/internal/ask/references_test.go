package ask

import (
	"testing"

	"github.com/wooagent-os/wooagent-os/daemon/internal/llm/anthropic"
)

func toolResult(text string) anthropic.Message {
	return anthropic.Message{
		Role:    "user",
		Content: []anthropic.ContentBlock{anthropic.ToolResultBlock("t1", text, false)},
	}
}

func errResult(text string) anthropic.Message {
	return anthropic.Message{
		Role:    "user",
		Content: []anthropic.ContentBlock{anthropic.ToolResultBlock("t1", text, true)},
	}
}

func TestExtract_ProposalReferenceFromListProposals(t *testing.T) {
	trace := anthropic.Trace{Messages: []anthropic.Message{
		anthropic.UserMessage("what's pending?"),
		toolResult(`{"proposals":[{"id":"1247","title":"Linen Napkin","state":"pending"}],"count":1}`),
	}}
	refs, dispatched := ExtractReferencesAndDispatched(
		"You should look at [proposal #1247] first.",
		trace,
	)
	if len(refs) != 1 || refs[0].ID != "1247" || refs[0].Title != "Linen Napkin" || refs[0].State != "pending" {
		t.Fatalf("unexpected refs: %+v", refs)
	}
	if len(dispatched) != 0 {
		t.Fatalf("expected no dispatched, got %+v", dispatched)
	}
}

func TestExtract_RunReferenceFromGetRun(t *testing.T) {
	trace := anthropic.Trace{Messages: []anthropic.Message{
		anthropic.UserMessage("what's that run?"),
		toolResult(`{"id":"rn_abc","persona":"pricing","status":"running"}`),
	}}
	refs, _ := ExtractReferencesAndDispatched(
		"See [run rn_abc] — it's still going.",
		trace,
	)
	if len(refs) != 1 || refs[0].Kind != "run" || refs[0].ID != "rn_abc" || refs[0].State != "running" {
		t.Fatalf("unexpected refs: %+v", refs)
	}
}

func TestExtract_DispatchedFromDispatchTool(t *testing.T) {
	trace := anthropic.Trace{Messages: []anthropic.Message{
		anthropic.UserMessage("pricing, look at SKU-1234"),
		toolResult(`{"ok":true,"persona":"pricing","run_id":"rn_xyz","eta_seconds":60,"target":"SKU-1234","brief":"check"}`),
	}}
	refs, dispatched := ExtractReferencesAndDispatched(
		"Asked Pricing to look at SKU-1234 — [run rn_xyz] will land in about a minute.",
		trace,
	)
	if len(dispatched) != 1 {
		t.Fatalf("expected 1 dispatched, got %+v", dispatched)
	}
	if dispatched[0].RunID != "rn_xyz" || dispatched[0].Persona != "pricing" || dispatched[0].ETASeconds != 60 {
		t.Errorf("unexpected dispatched: %+v", dispatched[0])
	}
	// Run reference should also resolve, with title derived from the
	// dispatch (persona + target) and state="working".
	if len(refs) != 1 || refs[0].ID != "rn_xyz" || refs[0].Title != "pricing on SKU-1234" || refs[0].State != "working" {
		t.Errorf("unexpected refs: %+v", refs)
	}
}

func TestExtract_FailedDispatchProducesNoDispatched(t *testing.T) {
	trace := anthropic.Trace{Messages: []anthropic.Message{
		toolResult(`{"ok":false,"persona":"reporting","reason":"not a dispatchable specialist"}`),
	}}
	_, dispatched := ExtractReferencesAndDispatched("Reporting isn't available yet.", trace)
	if len(dispatched) != 0 {
		t.Errorf("expected no dispatched for ok=false, got %+v", dispatched)
	}
}

func TestExtract_ErroredToolResultsIgnored(t *testing.T) {
	trace := anthropic.Trace{Messages: []anthropic.Message{
		errResult(`{"proposals":[{"id":"1247","title":"Linen Napkin","state":"pending"}]}`),
	}}
	refs, _ := ExtractReferencesAndDispatched("[proposal #1247]", trace)
	// The reference should still emit (model mentioned it) but with no
	// title/state because the errored tool_result was ignored.
	if len(refs) != 1 || refs[0].ID != "1247" || refs[0].Title != "" || refs[0].State != "" {
		t.Fatalf("expected unresolved ref, got %+v", refs)
	}
}

func TestExtract_DedupesByID(t *testing.T) {
	trace := anthropic.Trace{Messages: []anthropic.Message{
		toolResult(`{"proposals":[{"id":"1247","title":"X","state":"pending"}]}`),
	}}
	refs, _ := ExtractReferencesAndDispatched(
		"[proposal #1247] is older than [proposal #1247] would suggest.",
		trace,
	)
	if len(refs) != 1 {
		t.Errorf("expected dedup, got %+v", refs)
	}
}

func TestExtract_MentionOrderPreserved(t *testing.T) {
	trace := anthropic.Trace{Messages: []anthropic.Message{
		toolResult(`{"proposals":[
			{"id":"A","title":"A","state":"pending"},
			{"id":"B","title":"B","state":"pending"}
		]}`),
	}}
	refs, _ := ExtractReferencesAndDispatched(
		"Look at [proposal #B] then [proposal #A].",
		trace,
	)
	if len(refs) != 2 || refs[0].ID != "B" || refs[1].ID != "A" {
		t.Fatalf("expected B then A, got %+v", refs)
	}
}

func TestExtract_RegexAcceptsBracketVariants(t *testing.T) {
	trace := anthropic.Trace{Messages: []anthropic.Message{
		toolResult(`{"proposals":[{"id":"1247","title":"X","state":"pending"}]}`),
	}}
	for _, citation := range []string{
		"[proposal #1247]",
		"[proposal 1247]",
		"[Proposal #1247]",
	} {
		refs, _ := ExtractReferencesAndDispatched(citation, trace)
		if len(refs) != 1 || refs[0].ID != "1247" {
			t.Errorf("citation %q did not produce a reference: %+v", citation, refs)
		}
	}
}
