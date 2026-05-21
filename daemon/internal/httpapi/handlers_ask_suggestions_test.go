package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// TestHandleAskSuggestions_SpecialistReturnsEmpty asserts that
// specialist agents get an empty list back — the UI handles
// specialist defaults locally.
func TestHandleAskSuggestions_SpecialistReturnsEmpty(t *testing.T) {
	srv, _ := newSuggestionsHarness(t)
	resp := suggestionsRequest(t, srv, "marketing", "agents")
	if len(resp) != 0 {
		t.Errorf("expected empty list for specialist; got %v", resp)
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
