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
		nil,
		AgentChiefOfStaff,
	)
	if len(refs) != 1 || refs[0].ID != "1247" || refs[0].Title != "Linen Napkin" || refs[0].State != "pending" {
		t.Fatalf("unexpected refs: %+v", refs)
	}
	if len(dispatched) != 0 {
		t.Fatalf("expected no dispatched, got %+v", dispatched)
	}
}

// TestExtract_HallucinatedProposalIDDropped guards against DSGWOO-1362:
// a proposal ID the model mentions in prose but never saw in any tool
// result must NOT make it into the references chip array. Surfacing a
// chip that navigates to "no issue with that id" reads as a broken
// product.
func TestExtract_HallucinatedProposalIDDropped(t *testing.T) {
	trace := anthropic.Trace{Messages: []anthropic.Message{
		anthropic.UserMessage("what needs my attention?"),
		toolResult(`{"proposals":[{"id":"real-a","title":"Real A","state":"pending"}]}`),
	}}
	refs, _ := ExtractReferencesAndDispatched(
		"Look at [proposal #real-a] and also [proposal #fake-b].",
		trace,
		nil,
		AgentChiefOfStaff,
	)
	if len(refs) != 1 || refs[0].ID != "real-a" {
		t.Fatalf("expected only real-a to survive validation, got %+v", refs)
	}
}

// TestExtract_HallucinatedRunIDDropped is the run-side mirror of the
// proposal hallucination guard.
func TestExtract_HallucinatedRunIDDropped(t *testing.T) {
	trace := anthropic.Trace{Messages: []anthropic.Message{
		toolResult(`{"runs":[{"id":"rn_real","persona":"pricing","status":"succeeded"}]}`),
	}}
	refs, _ := ExtractReferencesAndDispatched(
		"See [run rn_real] and the older [run rn_fake].",
		trace,
		nil,
		AgentChiefOfStaff,
	)
	if len(refs) != 1 || refs[0].ID != "rn_real" {
		t.Fatalf("expected only rn_real to survive validation, got %+v", refs)
	}
}

// TestExtract_ProposalRefDerivedFromListRuns: when list_runs carries
// issue_id (+ title/state), the model can pivot from a run record to
// a proposal reference. Validation must accept that proposal id — the
// "prefer-proposals" prompt rule depends on this.
func TestExtract_ProposalRefDerivedFromListRuns(t *testing.T) {
	trace := anthropic.Trace{Messages: []anthropic.Message{
		anthropic.UserMessage("what did pricing do?"),
		toolResult(`{"runs":[{"id":"rn-1","persona":"pricing","status":"succeeded","issue_id":"iss-99","issue_title":"T-Shirt","issue_state":"pending"}]}`),
	}}
	refs, _ := ExtractReferencesAndDispatched(
		"Pricing landed [proposal #iss-99] (T-Shirt).",
		trace,
		nil,
		AgentChiefOfStaff,
	)
	if len(refs) != 1 || refs[0].Kind != "proposal" || refs[0].ID != "iss-99" {
		t.Fatalf("expected proposal ref derived from list_runs, got %+v", refs)
	}
	if refs[0].Title != "T-Shirt" || refs[0].State != "pending" {
		t.Errorf("expected proposal chip to carry title+state from list_runs join, got %+v", refs[0])
	}
}

// TestExtract_VisibleItemIDAccepted covers the case where the operator
// has the board open: the model sees proposal IDs in PageContext and
// can cite them without calling list_proposals. Those IDs are legit and
// must survive validation.
func TestExtract_VisibleItemIDAccepted(t *testing.T) {
	pageCtx := &PageContext{
		Page: "board",
		VisibleItems: []VisibleItem{
			{ID: "vi-1", Title: "Visible One", Kind: "proposal", State: "pending"},
		},
	}
	refs, _ := ExtractReferencesAndDispatched(
		"Take a look at [proposal #vi-1].",
		anthropic.Trace{},
		pageCtx,
		AgentChiefOfStaff,
	)
	if len(refs) != 1 || refs[0].ID != "vi-1" || refs[0].Title != "Visible One" || refs[0].State != "pending" {
		t.Fatalf("expected visible-item ref to survive with full metadata, got %+v", refs)
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
		nil,
		AgentChiefOfStaff,
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
		nil,
		AgentChiefOfStaff,
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
	_, dispatched := ExtractReferencesAndDispatched("Reporting isn't available yet.", trace, nil, AgentChiefOfStaff)
	if len(dispatched) != 0 {
		t.Errorf("expected no dispatched for ok=false, got %+v", dispatched)
	}
}

// TestExtract_ErroredToolResultsDropReference: when the tool errored,
// the ID was never confirmed seen. Emitting an unresolved chip used to
// be the documented behavior, but DSGWOO-1362 flipped that — a chip
// without title/state navigates the operator to a 404. Now: drop it.
func TestExtract_ErroredToolResultsDropReference(t *testing.T) {
	trace := anthropic.Trace{Messages: []anthropic.Message{
		errResult(`{"proposals":[{"id":"1247","title":"Linen Napkin","state":"pending"}]}`),
	}}
	refs, _ := ExtractReferencesAndDispatched("[proposal #1247]", trace, nil, AgentChiefOfStaff)
	if len(refs) != 0 {
		t.Fatalf("expected dropped ref (errored tool means unconfirmed id), got %+v", refs)
	}
}

func TestExtract_DedupesByID(t *testing.T) {
	trace := anthropic.Trace{Messages: []anthropic.Message{
		toolResult(`{"proposals":[{"id":"1247","title":"X","state":"pending"}]}`),
	}}
	refs, _ := ExtractReferencesAndDispatched(
		"[proposal #1247] is older than [proposal #1247] would suggest.",
		trace,
		nil,
		AgentChiefOfStaff,
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
		nil,
		AgentChiefOfStaff,
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
		refs, _ := ExtractReferencesAndDispatched(citation, trace, nil, AgentChiefOfStaff)
		if len(refs) != 1 || refs[0].ID != "1247" {
			t.Errorf("citation %q did not produce a reference: %+v", citation, refs)
		}
	}
}
