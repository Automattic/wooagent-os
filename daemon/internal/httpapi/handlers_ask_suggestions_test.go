package httpapi

import (
	"context"
	"database/sql"
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

// TestHandleAskSuggestions_IgnoresProposalsFromAPreviousStore is the
// regression test for the bug this shipped with: `issues` has no store
// column, so before the paired_at floor a re-pair left specialists
// naming products from the catalog they used to be connected to.
//
// Observed live — a store paired 2026-08-02 served Pricing chips seeded
// from May and June proposals made against a different store.
func TestHandleAskSuggestions_IgnoresProposalsFromAPreviousStore(t *testing.T) {
	srv, db := newSuggestionsHarness(t)
	pairedAt := time.Now().Add(-2 * time.Hour)
	seedPairedStore(t, db, "store_current", pairedAt)

	// Made against the store we used to be paired with.
	seedProposal(t, db, "i_old", "pricing", "in_review", "product_price_change",
		"Price change · Product From Old Store · $10.00 → $12.00 (+20.0%)",
		pairedAt.Add(-30*24*time.Hour))
	// Made against the store we're paired with now.
	seedProposal(t, db, "i_new", "pricing", "in_review", "product_price_change",
		"Price change · Product From Current Store · $20.00 → $22.00 (+10.0%)",
		pairedAt.Add(10*time.Minute))

	resp := suggestionsRequest(t, srv, "pricing", "board")
	for _, s := range resp {
		if strings.Contains(s, "Product From Old Store") {
			t.Errorf("chip names a product from a previously paired store: %q", s)
		}
	}
	if !has(resp, "Why did you propose that price for the Product From Current Store?") {
		t.Errorf("expected a chip from the current store's proposals; got %v", resp)
	}
}

// TestHandleAskSuggestions_FreshPairingShowsGenerics asserts the state
// right after re-pairing: nothing has been proposed against the new
// catalog yet, so we stay generic rather than guessing.
func TestHandleAskSuggestions_FreshPairingShowsGenerics(t *testing.T) {
	srv, db := newSuggestionsHarness(t)
	pairedAt := time.Now().Add(-1 * time.Minute)
	seedPairedStore(t, db, "store_fresh", pairedAt)
	seedProposal(t, db, "i_stale", "marketing", "in_review", "product_description_rewrite",
		"Product description rewrite · Product From Old Store",
		pairedAt.Add(-90*24*time.Hour))

	resp := suggestionsRequest(t, srv, "marketing", "board")
	if !has(resp, "What did you draft this week?") {
		t.Errorf("expected generics after a fresh pairing; got %v", resp)
	}
	for _, s := range resp {
		if strings.Contains(s, "Product From Old Store") {
			t.Errorf("expected no grounded chip after a fresh pairing; got %q", s)
		}
	}
}

// TestHandleAskSuggestions_NoPairedStoreDisablesTheFloor covers the
// headless / CI case: MCP resolves from the environment, no store row
// exists, and there's no catalog to contradict — so proposals stay
// eligible rather than the floor silently blanking every chip.
func TestHandleAskSuggestions_NoPairedStoreDisablesTheFloor(t *testing.T) {
	srv, db := newSuggestionsHarness(t)
	seedProposal(t, db, "i_env", "pricing", "in_review", "product_price_change",
		"Price change · Env Store Product · $5.00 → $6.00 (+20.0%)",
		time.Now().Add(-48*time.Hour))

	resp := suggestionsRequest(t, srv, "pricing", "board")
	if !has(resp, "Why did you propose that price for the Env Store Product?") {
		t.Errorf("expected grounded chip when no store is paired; got %v", resp)
	}
}

// TestHandleAskSuggestions_UsesMostRecentPairing asserts we follow the
// same row the daemon's MCP client resolves to — paired_at DESC — so
// chips describe the store approvals actually write to.
func TestHandleAskSuggestions_UsesMostRecentPairing(t *testing.T) {
	srv, db := newSuggestionsHarness(t)
	older := time.Now().Add(-10 * 24 * time.Hour)
	newer := time.Now().Add(-1 * time.Hour)
	seedPairedStore(t, db, "store_older", older)
	seedPairedStore(t, db, "store_newer", newer)

	// Between the two pairings — belongs to the older store only.
	seedProposal(t, db, "i_between", "pricing", "in_review", "product_price_change",
		"Price change · Between Pairings · $1.00 → $2.00 (+100.0%)",
		older.Add(24*time.Hour))

	resp := suggestionsRequest(t, srv, "pricing", "board")
	for _, s := range resp {
		if strings.Contains(s, "Between Pairings") {
			t.Errorf("floor should track the newest pairing; got %q", s)
		}
	}
}

// TestHandleAskSuggestions_StoreIDBeatsTheTimestampFloor asserts the
// precise path from migration 023: a proposal stamped with the current
// store is eligible even when it predates the pairing timestamp.
//
// That combination is real, not contrived — re-pairing the same store
// (disconnect, reconnect) mints a fresh paired_at while the proposals are
// still about that store's catalog. The timestamp floor alone throws them
// away; store_id keeps them.
func TestHandleAskSuggestions_StoreIDBeatsTheTimestampFloor(t *testing.T) {
	srv, db := newSuggestionsHarness(t)
	pairedAt := time.Now().Add(-1 * time.Hour)
	seedPairedStore(t, db, "store_current", pairedAt)

	seedProposalForStore(t, db, "i_old_same_store", "pricing", "in_review",
		"product_price_change",
		"Price change · Reconnected Store Product · $10.00 → $11.00 (+10.0%)",
		pairedAt.Add(-30*24*time.Hour), "store_current")

	resp := suggestionsRequest(t, srv, "pricing", "board")
	if !has(resp, "Why did you propose that price for the Reconnected Store Product?") {
		t.Errorf("a proposal stamped with the current store should stay eligible; got %v", resp)
	}
}

// TestHandleAskSuggestions_StoreIDExcludesOtherStores asserts the reverse:
// a proposal stamped with a *different* store is excluded even when it
// clears the timestamp floor.
func TestHandleAskSuggestions_StoreIDExcludesOtherStores(t *testing.T) {
	srv, db := newSuggestionsHarness(t)
	pairedAt := time.Now().Add(-2 * time.Hour)
	seedPairedStore(t, db, "store_current", pairedAt)

	seedProposalForStore(t, db, "i_other", "pricing", "in_review",
		"product_price_change",
		"Price change · Other Store Product · $10.00 → $11.00 (+10.0%)",
		pairedAt.Add(10*time.Minute), "store_other")

	resp := suggestionsRequest(t, srv, "pricing", "board")
	for _, s := range resp {
		if strings.Contains(s, "Other Store Product") {
			t.Errorf("a proposal from another store should be excluded even after the floor; got %q", s)
		}
	}
}

// TestHandleAskSuggestions_NullStoreIDStillUsesTheFloor pins the
// pre-migration path. Migration 023 deliberately doesn't backfill, so rows
// written before it have store_id NULL and can only be judged on
// created_at — old rows excluded, recent ones kept.
func TestHandleAskSuggestions_NullStoreIDStillUsesTheFloor(t *testing.T) {
	srv, db := newSuggestionsHarness(t)
	pairedAt := time.Now().Add(-2 * time.Hour)
	seedPairedStore(t, db, "store_current", pairedAt)

	// Unstamped and older than the pairing — excluded.
	seedProposal(t, db, "i_legacy_old", "marketing", "in_review",
		"product_description_rewrite",
		"Product description rewrite · Legacy Old Product",
		pairedAt.Add(-30*24*time.Hour))
	// Unstamped but newer than the pairing — kept.
	seedProposal(t, db, "i_legacy_new", "marketing", "in_review",
		"product_description_rewrite",
		"Product description rewrite · Legacy New Product",
		pairedAt.Add(30*time.Minute))

	resp := suggestionsRequest(t, srv, "marketing", "board")
	for _, s := range resp {
		if strings.Contains(s, "Legacy Old Product") {
			t.Errorf("pre-migration row older than the pairing should be excluded; got %q", s)
		}
	}
	if !has(resp, "Walk me through your Legacy New Product description.") {
		t.Errorf("pre-migration row newer than the pairing should be kept; got %v", resp)
	}
}

// TestCurrentStoreID_MatchesTheSuggestionFloor guards the seam that makes
// all of this coherent: store.CurrentStoreID stamps inserts, and the
// suggestion query filters on it. If the two ever select different rows,
// proposals get stamped with one store while chips describe another.
func TestCurrentStoreID_MatchesTheSuggestionFloor(t *testing.T) {
	_, db := newSuggestionsHarness(t)
	older := time.Now().Add(-10 * 24 * time.Hour)
	newer := time.Now().Add(-1 * time.Hour)
	seedPairedStore(t, db, "store_older", older)
	seedPairedStore(t, db, "store_newer", newer)

	// Both resolve "the paired store" by paired_at DESC, so both must land
	// on store_newer.
	if got := store.CurrentStoreID(context.Background(), db.DB); got != "store_newer" {
		t.Errorf("CurrentStoreID = %q, want store_newer", got)
	}
	if got := pairedSince(context.Background(), db.DB); got != newer.UTC().Format(time.RFC3339) {
		t.Errorf("pairedSince = %q, want the newest pairing's timestamp", got)
	}
}

// TestCreateIssue_StampsStoreID drives the real create-issue handler and
// asserts the row lands with store_id set.
//
// The unit tests above seed store_id directly, which proves the *query*
// is right but not that anything ever writes the column. This closes that
// gap on the one insert path reachable over HTTP; the persona paths are
// covered by their own packages' tests.
func TestCreateIssue_StampsStoreID(t *testing.T) {
	srv, db := newSuggestionsHarness(t)
	seedPairedStore(t, db, "store_current", time.Now().Add(-1*time.Hour))
	// The FK on issues.persona needs the agent row to exist.
	seedIssue(t, db, "i_seed", "marketing", "backlog", time.Now())

	body := `{"title":"Product description rewrite · Stamped Product",` +
		`"persona":"marketing","status":"in_review","priority":"medium",` +
		`"proposal":{"type":"product_description_rewrite","content":"copy"}}`
	req := httptest.NewRequest("POST", "/v1/issues", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.handleCreateIssue(rec, req)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create issue: status %d: %s", rec.Code, rec.Body.String())
	}

	var storeID sql.NullString
	if err := db.DB.QueryRow(
		`SELECT store_id FROM issues WHERE title = ?`,
		"Product description rewrite · Stamped Product",
	).Scan(&storeID); err != nil {
		t.Fatalf("read back store_id: %v", err)
	}
	if !storeID.Valid || storeID.String != "store_current" {
		t.Errorf("store_id = %v, want store_current", storeID)
	}

	// And the round trip: a freshly stamped proposal is immediately eligible
	// to seed a chip, without waiting on the timestamp floor.
	resp := suggestionsRequest(t, srv, "marketing", "board")
	if !has(resp, "Walk me through your Stamped Product description.") {
		t.Errorf("expected the new proposal to ground a chip; got %v", resp)
	}
}

// TestCreateIssue_StoreIDNullWhenNothingPaired asserts the column stays
// NULL rather than becoming an empty string when no store is paired.
// Readers distinguish NULL ("unknown provenance") from a real id, and an
// empty string would satisfy neither branch of the suggestion query.
func TestCreateIssue_StoreIDNullWhenNothingPaired(t *testing.T) {
	srv, db := newSuggestionsHarness(t)
	seedIssue(t, db, "i_seed", "marketing", "backlog", time.Now())

	body := `{"title":"Product description rewrite · Unpaired Product",` +
		`"persona":"marketing","status":"in_review","priority":"medium"}`
	req := httptest.NewRequest("POST", "/v1/issues", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.handleCreateIssue(rec, req)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create issue: status %d: %s", rec.Code, rec.Body.String())
	}

	var storeID sql.NullString
	if err := db.DB.QueryRow(
		`SELECT store_id FROM issues WHERE title = ?`,
		"Product description rewrite · Unpaired Product",
	).Scan(&storeID); err != nil {
		t.Fatalf("read back store_id: %v", err)
	}
	if storeID.Valid {
		t.Errorf("store_id = %q, want NULL when nothing is paired", storeID.String)
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

// seedPairedStore inserts a paired store row. Only the columns the
// suggestion floor reads are populated — status and paired_at.
func seedPairedStore(t *testing.T, st *store.Store, id string, pairedAt time.Time) {
	t.Helper()
	ts := pairedAt.UTC().Format(time.RFC3339)
	_, err := st.DB.Exec(
		`INSERT INTO stores (id, url, mcp_endpoint, status, paired_at, created_at, updated_at)
		 VALUES (?, ?, ?, 'paired', ?, ?, ?)`,
		id, "https://"+id+".example.com",
		"https://"+id+".example.com/wp-json/mcp/mcp-adapter-default-server",
		ts, ts, ts,
	)
	if err != nil {
		t.Fatalf("seed paired store: %v", err)
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

// seedProposalForStore is seedProposal plus an explicit store_id, for
// exercising the post-migration-023 provenance path.
func seedProposalForStore(t *testing.T, st *store.Store, id, persona, status, proposalType, title string, createdAt time.Time, storeID string) {
	t.Helper()
	seedProposal(t, st, id, persona, status, proposalType, title, createdAt)
	if _, err := st.DB.Exec(`UPDATE issues SET store_id = ? WHERE id = ?`, storeID, id); err != nil {
		t.Fatalf("stamp store_id: %v", err)
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
