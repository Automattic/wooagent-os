package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/ask"
)

// handleAskSuggestions serves dynamic, queue-aware chat suggestions
// for the Ask Agent drawer's empty-thread state (DSGWOO-1357 B5).
//
// Request:    GET /v1/ask/suggestions?agent=<slug>&page=<slug>
// Response:   { "suggestions": ["...", "..."] }
//
// Why server-side: the static UI map can't see queue state — it
// suggests "What needs my attention first?" even when 0 items are
// pending. A daemon-side endpoint can shape the list to what's
// actually useful right now, and the UI caches aggressively (60s) so
// the cost is negligible.
//
// Vetting contract (carried over from the static map): every
// suggestion the daemon may return is a query the CoS / specialist
// eval passes. Don't add a suggestion that the LLM regresses on.
func (s *Server) handleAskSuggestions(w http.ResponseWriter, r *http.Request) {
	if s.askCfg == nil {
		writeError(w, http.StatusServiceUnavailable, "ask_unavailable", "ask endpoint not configured")
		return
	}
	agent := r.URL.Query().Get("agent")
	if agent == "" {
		agent = string(ask.AgentChiefOfStaff)
	}
	page := r.URL.Query().Get("page")

	state := readQueueState(r.Context(), s.store.DB)
	out := suggestionsFor(ask.AgentSlug(agent), page, state)

	writeJSON(w, http.StatusOK, map[string]any{
		"suggestions": out,
		// State surfaces in the response so a UI debugger can verify
		// the daemon's view of "what's pending"; the field is
		// undocumented for callers — treat as opaque debug data.
		"_state": state,
	})
}

// queueState is a thin snapshot of issue counts driving the
// suggestion shape. Cheap query; rebuilt on every call (cache lives
// on the UI side at 60s TTL).
type queueState struct {
	Pending         int `json:"pending"`
	Stuck           int `json:"stuck"`    // pending > 24h
	ApprovedToday   int `json:"approved_today"`
	RecentRejected  int `json:"recent_rejected"`  // rejected in last 7d
	FailedRunsRecent int `json:"failed_runs_recent"` // failed in last 24h
}

// readQueueState computes the snapshot. Cutoffs computed in Go and
// passed as parameters because daemon-written timestamps are RFC3339
// (`2026-05-21T13:00:00Z`) while SQLite's `datetime('now', ...)` is
// `YYYY-MM-DD hh:mm:ss` (space separator) — lexicographic comparison
// across that format mismatch silently returns wrong counts.
func readQueueState(ctx context.Context, db *sql.DB) queueState {
	var s queueState
	now := time.Now().UTC()
	cutoff24h := now.Add(-24 * time.Hour).Format(time.RFC3339)
	cutoff7d := now.Add(-7 * 24 * time.Hour).Format(time.RFC3339)
	cutoffToday := now.Truncate(24 * time.Hour).Format(time.RFC3339)
	row := db.QueryRowContext(ctx, `
		SELECT
		  (SELECT count(*) FROM issues WHERE status = 'in_review'),
		  (SELECT count(*) FROM issues WHERE status = 'in_review' AND created_at < ?),
		  (SELECT count(*) FROM issues WHERE status = 'done' AND updated_at >= ?),
		  (SELECT count(*) FROM issues WHERE status IN ('rejected','dismissed') AND updated_at >= ?),
		  (SELECT count(*) FROM runs   WHERE status IN ('failed','failed_permanent') AND created_at >= ?)
	`, cutoff24h, cutoffToday, cutoff7d, cutoff24h)
	// Errors are non-fatal — fall through with zero counts. The
	// suggestion map handles that case via the "empty" branches.
	_ = row.Scan(&s.Pending, &s.Stuck, &s.ApprovedToday, &s.RecentRejected, &s.FailedRunsRecent)
	return s
}

// suggestionsFor maps (agent, page, state) → a vetted suggestion
// list. CoS suggestions are state-aware; specialists keep their
// static lists from v1 (specialist queue-state shaping doesn't
// have a clear product story yet).
//
// Stays in sync with the UI's suggestions.ts default lists — if you
// add a suggestion here, add it to the UI as a fallback so a 503 from
// this endpoint still surfaces something useful.
func suggestionsFor(agent ask.AgentSlug, page string, st queueState) []string {
	if agent != ask.AgentChiefOfStaff {
		// Specialists: static lists, owned UI-side. Returning empty
		// signals the UI to fall through to its own defaults.
		return nil
	}

	// CoS — state-aware per page.
	switch page {
	case "needs-review":
		return cosSuggestionsForNeedsReview(st)
	case "board":
		return cosSuggestionsForBoard(st)
	case "done":
		return cosSuggestionsForDone(st)
	case "runs":
		return cosSuggestionsForRuns(st)
	case "agents":
		return []string{
			"What is each agent for?",
			"Which specialists are available?",
		}
	default:
		// Unrecognized page → safe defaults.
		return []string{
			"What needs my attention?",
			"What did the agents do today?",
		}
	}
}

func cosSuggestionsForNeedsReview(st queueState) []string {
	out := []string{}
	if st.Pending > 0 {
		out = append(out, "What needs my attention first?")
	}
	if st.Stuck > 0 {
		out = append(out, "What's stuck?")
	}
	if st.Pending == 0 {
		out = append(out, "What did I approve today?")
	}
	if len(out) < 2 {
		out = append(out, "Summarize the queue in one sentence.")
	}
	return trimSuggestions(out, 4)
}

func cosSuggestionsForBoard(st queueState) []string {
	out := []string{
		"What did the agents do overnight?",
	}
	if st.Pending > 0 {
		out = append(out, "What's pending right now?")
	}
	if st.Stuck > 0 {
		out = append(out, "What's stuck?")
	}
	return trimSuggestions(out, 4)
}

func cosSuggestionsForDone(st queueState) []string {
	out := []string{
		"What did I approve yesterday?",
	}
	if st.RecentRejected > 0 {
		out = append(out, "Anything rejected this week?")
	}
	return trimSuggestions(out, 4)
}

func cosSuggestionsForRuns(st queueState) []string {
	out := []string{
		"Show me the most recent runs across all personas.",
	}
	if st.FailedRunsRecent > 0 {
		out = append(out, "Did anything fail recently?")
	}
	return trimSuggestions(out, 4)
}

// trimSuggestions caps the list at max entries. The product spec calls
// for 2-4; this guards against accidental growth.
func trimSuggestions(s []string, max int) []string {
	if len(s) > max {
		return s[:max]
	}
	return s
}
