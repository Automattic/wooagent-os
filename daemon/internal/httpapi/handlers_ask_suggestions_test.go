package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/ask"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

// TestHandleAskSuggestions_EmptyQueueShifts asserts the endpoint
// changes its CoS shape when no items are pending — "What did I
// approve today?" replaces "What needs my attention first?".
func TestHandleAskSuggestions_EmptyQueueShifts(t *testing.T) {
	srv, _ := newSuggestionsHarness(t)
	resp := suggestionsRequest(t, srv, "chief_of_staff", "needs-review")
	if has(resp, "What needs my attention first?") {
		t.Errorf("expected the empty-queue branch; got %v", resp)
	}
	if !has(resp, "What did I approve today?") {
		t.Errorf("expected the approved-today suggestion when queue is empty; got %v", resp)
	}
}

// TestHandleAskSuggestions_PendingPromotesAttention asserts the
// "needs attention" suggestion fires when at least one item is
// pending.
func TestHandleAskSuggestions_PendingPromotesAttention(t *testing.T) {
	srv, db := newSuggestionsHarness(t)
	seedIssue(t, db, "i_1", "marketing", "in_review", time.Now().Add(-30*time.Minute))
	resp := suggestionsRequest(t, srv, "chief_of_staff", "needs-review")
	if !has(resp, "What needs my attention first?") {
		t.Errorf("expected pending-aware suggestion; got %v", resp)
	}
}

// TestHandleAskSuggestions_StuckSurfacesWhenOverThreshold seeds a
// >24h pending item; the "stuck" suggestion should appear.
func TestHandleAskSuggestions_StuckSurfacesWhenOverThreshold(t *testing.T) {
	srv, db := newSuggestionsHarness(t)
	seedIssue(t, db, "i_old", "marketing", "in_review", time.Now().Add(-26*time.Hour))
	resp := suggestionsRequest(t, srv, "chief_of_staff", "needs-review")
	if !has(resp, "What's stuck?") {
		t.Errorf("expected stuck suggestion; got %v", resp)
	}
}

// TestHandleAskSuggestions_SpecialistFallsBackToGenerics asserts that a
// specialist with nothing proposed yet — a fresh install — still gets
// the always-deliverable capability chips rather than an empty row.
func TestHandleAskSuggestions_SpecialistFallsBackToGenerics(t *testing.T) {
	srv, _ := newSuggestionsHarness(t)
	resp := suggestionsRequest(t, srv, "marketing", "agents")
	if !has(resp, "What’s our brand voice?") {
		t.Errorf("expected generic marketing suggestions; got %v", resp)
	}
	for _, s := range resp {
		if strings.Contains(s, "Walk me through") {
			t.Errorf("expected no grounded chips with an empty queue; got %q", s)
		}
	}
}

// TestHandleAskSuggestions_SpecialistGroundsInRealProducts is the point
// of DSGWOO-1371: a Pricing chip should name a product the store
// actually carries, read off the persona's own recent proposal.
func TestHandleAskSuggestions_SpecialistGroundsInRealProducts(t *testing.T) {
	srv, db := newSuggestionsHarness(t)
	seedProposal(t, db, "i_pr_1", "pricing", "in_review", "product_price_change",
		"Price change · Heavyweight Wool Cardigan · $98.00 → $104.00 (+6.1%)",
		time.Now().Add(-20*time.Minute))

	resp := suggestionsRequest(t, srv, "pricing", "board")
	if !has(resp, "Why did you propose that price for the Heavyweight Wool Cardigan?") {
		t.Errorf("expected a product-grounded pricing chip; got %v", resp)
	}
	// The generic chip still backfills behind it, so the operator sees
	// what the agent can do in general as well as what it just did.
	if !has(resp, "How do you decide what to reprice?") {
		t.Errorf("expected a generic chip alongside the grounded one; got %v", resp)
	}
}

// TestHandleAskSuggestions_GroundedChipsLeadTheList asserts ordering:
// the store-aware chips are the reason this endpoint exists, so they
// come before the generics.
func TestHandleAskSuggestions_GroundedChipsLeadTheList(t *testing.T) {
	srv, db := newSuggestionsHarness(t)
	seedProposal(t, db, "i_mk_1", "marketing", "in_review", "product_description_rewrite",
		"Product description rewrite · Merino Beanie", time.Now().Add(-5*time.Minute))

	resp := suggestionsRequest(t, srv, "marketing", "board")
	if len(resp) == 0 || resp[0] != "Walk me through your Merino Beanie description." {
		t.Errorf("expected the grounded chip first; got %v", resp)
	}
}

// TestHandleAskSuggestions_SkipsDismissedProposals asserts we don't
// re-surface something the operator already said no to.
func TestHandleAskSuggestions_SkipsDismissedProposals(t *testing.T) {
	srv, db := newSuggestionsHarness(t)
	seedProposal(t, db, "i_pr_x", "pricing", "dismissed", "product_price_change",
		"Price change · Rejected Item · $10.00 → $12.00 (+20.0%)",
		time.Now().Add(-10*time.Minute))

	resp := suggestionsRequest(t, srv, "pricing", "board")
	for _, s := range resp {
		if strings.Contains(s, "Rejected Item") {
			t.Errorf("dismissed proposal should not seed a chip; got %q", s)
		}
	}
}

// TestHandleAskSuggestions_DedupesRepeatedSubjects asserts a product the
// persona has proposed on several times yields one chip, not three.
func TestHandleAskSuggestions_DedupesRepeatedSubjects(t *testing.T) {
	srv, db := newSuggestionsHarness(t)
	for i, ago := range []time.Duration{5 * time.Minute, 2 * time.Hour, 3 * time.Hour} {
		seedProposal(t, db, fmt.Sprintf("i_pr_%d", i), "pricing", "in_review",
			"product_price_change",
			"Price change · Wool Throw · $28.00 → $24.00 (-14.3%)",
			time.Now().Add(-ago))
	}

	resp := suggestionsRequest(t, srv, "pricing", "board")
	var count int
	for _, s := range resp {
		if strings.Contains(s, "Wool Throw") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one Wool Throw chip, got %d: %v", count, resp)
	}
}

// TestSubjectFromTitle covers the title-parsing edge cases directly —
// cheaper than driving each one through the handler.
func TestSubjectFromTitle(t *testing.T) {
	cases := []struct {
		name  string
		title string
		want  string
	}{
		{"marketing rewrite", "Product description rewrite · Linen Napkin", "Linen Napkin"},
		{"pricing with detail", "Price change · Polo · $20.00 → $25.00 (+25.0%)", "Polo"},
		{"sales support order", "Customer note · order #4521 · Re: shipping", "order #4521"},
		{"variable product drops the axis", "Price change · Polo — Large / Blue · $20.00 → $25.00 (+25.0%)", "Polo"},
		{"no separator", "Something unstructured", ""},
		{"empty subject", "Price change ·  · $1.00 → $2.00", ""},
		{"subject too long for a chip", "Price change · " + strings.Repeat("x", maxSubjectLen+1) + " · $1.00", ""},
		{"subject at the length limit", "Price change · " + strings.Repeat("x", maxSubjectLen) + " · $1.00", strings.Repeat("x", maxSubjectLen)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := subjectFromTitle(tc.title); got != tc.want {
				t.Errorf("subjectFromTitle(%q) = %q, want %q", tc.title, got, tc.want)
			}
		})
	}
}

// TestGroundedTemplatesCoverEveryPersona guards the seam between the
// persona code and this map: if a persona starts emitting a new
// proposal_type, its chips silently fall back to generics. This asserts
// the types we know about today are all still mapped.
func TestGroundedTemplatesCoverEveryPersona(t *testing.T) {
	for _, pt := range []string{
		"product_description_rewrite",
		"product_cold_draft",
		"product_price_change",
		"customer_reply_draft",
	} {
		if _, ok := groundedTemplates[pt]; !ok {
			t.Errorf("proposal type %q has no grounded question template", pt)
		}
	}
}

// TestHandleAskSuggestions_UnknownPageReturnsDefaults asserts that an
// unrecognized page falls through to safe defaults.
func TestHandleAskSuggestions_UnknownPageReturnsDefaults(t *testing.T) {
	srv, _ := newSuggestionsHarness(t)
	resp := suggestionsRequest(t, srv, "chief_of_staff", "completely-unknown-page")
	if !has(resp, "What needs my attention?") {
		t.Errorf("expected default suggestions for unknown page; got %v", resp)
	}
}

// TestSuggestionsFor_CapsAtFour asserts the max-4 trim works even when
// multiple state branches stack.
func TestSuggestionsFor_CapsAtFour(t *testing.T) {
	st := queueState{Pending: 5, Stuck: 2, FailedRunsRecent: 1}
	got := suggestionsFor(ask.AgentChiefOfStaff, "board", st)
	if len(got) > 4 {
		t.Errorf("expected <=4 suggestions, got %d: %v", len(got), got)
	}
}

// ------------------------------------------------------------- harness

func newSuggestionsHarness(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.DB.Close() })
	srv := &Server{store: st}
	srv.SetAsk(AskConfig{})
	return srv, st
}

func seedIssue(t *testing.T, st *store.Store, id, persona, status string, createdAt time.Time) {
	t.Helper()
	now := createdAt.UTC().Format(time.RFC3339)
	// Ensure agent FK exists.
	_, _ = st.DB.Exec(`INSERT OR IGNORE INTO agents (persona, name, enabled, created_at, updated_at, cadence_seconds, max_attempts) VALUES (?, ?, 1, ?, ?, 21600, 3)`,
		persona, persona, now, now)
	_, err := st.DB.Exec(
		`INSERT INTO issues (id, title, persona, status, priority, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'medium', ?, ?)`,
		id, "fixture", persona, status, now, now,
	)
	if err != nil {
		t.Fatalf("seed issue: %v", err)
	}
}

// seedProposal is seedIssue plus the two columns the specialist chips
// read: a real title to parse a subject out of, and the proposal_type
// that selects the question template.
func seedProposal(t *testing.T, st *store.Store, id, persona, status, proposalType, title string, createdAt time.Time) {
	t.Helper()
	now := createdAt.UTC().Format(time.RFC3339)
	_, _ = st.DB.Exec(`INSERT OR IGNORE INTO agents (persona, name, enabled, created_at, updated_at, cadence_seconds, max_attempts) VALUES (?, ?, 1, ?, ?, 21600, 3)`,
		persona, persona, now, now)
	_, err := st.DB.Exec(
		`INSERT INTO issues (id, title, persona, status, priority, created_at, updated_at, proposal_type)
		 VALUES (?, ?, ?, ?, 'medium', ?, ?, ?)`,
		id, title, persona, status, now, now, proposalType,
	)
	if err != nil {
		t.Fatalf("seed proposal: %v", err)
	}
}

func suggestionsRequest(t *testing.T, srv *Server, agent, page string) []string {
	t.Helper()
	req := httptest.NewRequest("GET", "/v1/ask/suggestions?agent="+agent+"&page="+page, nil)
	rec := httptest.NewRecorder()
	srv.handleAskSuggestions(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Suggestions []string `json:"suggestions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse response: %v (body: %s)", err, rec.Body.String())
	}
	return body.Suggestions
}

func has(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
