package httpapi

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
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

	// CoS shapes its list from queue counts; specialists shape theirs
	// from what they've actually proposed on. Split here rather than
	// inside one function so the CoS branch stays a pure (state → list)
	// mapping that's cheap to unit-test.
	state := readQueueState(r.Context(), s.store.DB)
	slug := ask.AgentSlug(agent)
	out := suggestionsFor(slug, page, state)
	if slug != ask.AgentChiefOfStaff {
		out = specialistSuggestions(r.Context(), s.store.DB, slug)
	}

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
	Pending          int `json:"pending"`
	Stuck            int `json:"stuck"` // pending > 24h
	ApprovedToday    int `json:"approved_today"`
	RecentRejected   int `json:"recent_rejected"`    // rejected in last 7d
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

// suggestionsFor maps (page, state) → a vetted CoS suggestion list.
// Specialists return nil here and are shaped by specialistSuggestions
// instead — see the DSGWOO-1371 section below.
//
// Stays in sync with the UI's suggestions.ts default lists — if you
// add a suggestion here, add it to the UI as a fallback so a 503 from
// this endpoint still surfaces something useful.
func suggestionsFor(agent ask.AgentSlug, page string, st queueState) []string {
	if agent != ask.AgentChiefOfStaff {
		// Specialists are shaped by specialistSuggestions, which needs
		// DB access this function deliberately doesn't take.
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

// ------------------------------------------------- specialist (DSGWOO-1371)
//
// Specialist chips used to be a static UI map, which meant they named
// products from the demo catalog — "What's our voice for towels?" on a
// store that sells merino wool. The operator taps it, the agent can't
// ground it, and the affordance stops being worth tapping.
//
// So we seed them from the persona's own recent proposals instead. That
// keeps the vetting rule intact for free: the subject came out of a
// proposal sitting in this database, and every specialist carries
// `list_proposals` / `get_proposal`, so the question is answerable
// without an MCP round-trip.

const (
	// subjectScanLimit is how many recent proposals we read to find
	// distinct subjects. A persona often proposes on the same product
	// repeatedly (a price walked down over several runs), so the scan
	// has to be wider than the number of chips we want.
	subjectScanLimit = 40

	// maxSubjectLen caps how long a subject can be before we skip it.
	// Chips are one line in a 420px drawer; a 60-character product name
	// wraps into a paragraph and reads as a bug.
	maxSubjectLen = 40

	// groundedPerAgent is how many store-aware chips to surface. Two
	// leaves room for a generic capability chip underneath, so the row
	// still says something about what the agent can do in general.
	groundedPerAgent = 2
)

// groundedTemplates maps a proposal type to the question we can safely
// ask about it. One %s, filled with the subject parsed off the title.
//
// Keyed on proposal_type rather than a title prefix because the title is
// display copy that drifts, while proposal_type is the persisted
// contract the approve and undo paths already switch on. Marketing's two
// shapes share a template — whether the agent rewrote an existing
// description or drafted one for a product that had none, the operator's
// question is the same.
var groundedTemplates = map[string]string{
	"product_description_rewrite": "Walk me through your %s description.",
	"product_cold_draft":          "Walk me through your %s description.",
	"product_price_change":        "Why did you propose that price for the %s?",
	"customer_reply_draft":        "What did you draft for %s?",
}

// specialistGenerics are the always-deliverable capability questions —
// no product name, nothing to get wrong. They carry a brand-new install
// where nothing has been proposed yet, and they backfill the row when
// there aren't enough distinct subjects.
//
// Mirrors ui/src/components/AskAgentDrawer/suggestions.ts. That map is
// the offline fallback for when this endpoint is unreachable; if you
// change a string here, change it there too.
var specialistGenerics = map[ask.AgentSlug][]string{
	ask.AgentMarketing: {
		"What did you draft this week?",
		"What’s our brand voice?",
	},
	ask.AgentPricing: {
		"What price changes have you recommended this month?",
		"How do you decide what to reprice?",
	},
	ask.AgentSalesSupport: {
		"What customer notes did you draft last week?",
		"What’s our usual response to shipping delays?",
	},
}

// specialistSuggestions builds a specialist's chip list: store-aware
// questions first, generic capability questions backfilling behind them.
// Returns nil for an agent we have no seeds for, which the UI reads as
// "use your own defaults".
func specialistSuggestions(ctx context.Context, db *sql.DB, agent ask.AgentSlug) []string {
	generics, known := specialistGenerics[agent]
	if !known {
		return nil
	}

	out := groundedSuggestions(ctx, db, string(agent), groundedPerAgent)
	for _, g := range generics {
		if len(out) >= 3 {
			break
		}
		out = append(out, g)
	}
	return trimSuggestions(out, 4)
}

// groundedSuggestions turns this persona's recent proposals into
// store-aware questions, at most one per distinct subject.
//
// Dismissed and rejected proposals are skipped: the operator has already
// said no to those, and leading the empty-thread state with them invites
// a re-litigation rather than a useful question.
//
// A query error yields no grounded chips rather than an error — the
// caller backfills with generics, so a locked database degrades to the
// v1 behavior instead of an empty drawer.
func groundedSuggestions(ctx context.Context, db *sql.DB, persona string, want int) []string {
	rows, err := db.QueryContext(ctx, `
		SELECT COALESCE(proposal_type, ''), title
		FROM issues
		WHERE persona = ?
		  AND status NOT IN ('dismissed', 'rejected')
		ORDER BY created_at DESC
		LIMIT ?
	`, persona, subjectScanLimit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []string
	seen := map[string]bool{}
	for rows.Next() {
		var proposalType, title string
		if err := rows.Scan(&proposalType, &title); err != nil {
			return out
		}
		template, ok := groundedTemplates[proposalType]
		if !ok {
			// A proposal shape with no vetted question — Marketing's
			// social posts and launch copy land here. Skipping is
			// correct: a generic chip beats an unvetted one.
			continue
		}
		subject := subjectFromTitle(title)
		if subject == "" || seen[subject] {
			continue
		}
		seen[subject] = true
		out = append(out, fmt.Sprintf(template, subject))
		if len(out) >= want {
			break
		}
	}
	return out
}

// subjectFromTitle pulls the human-readable subject out of a proposal
// title. Every persona formats titles as "<what> · <subject> · <detail>"
// — "Price change · Polo · $20.00 → $25.00 (+25.0%)" — so the subject is
// the second segment.
//
// Reading the title rather than proposal_target is deliberate: the
// target JSON carries an integer id ({"product_id": 42}), and resolving
// that to a name the operator recognizes would mean an MCP round-trip
// inside a request the drawer blocks its first paint on. The persona
// already wrote the name into the title.
//
// Returns "" when the title doesn't have the expected shape or the
// subject is too long to fit a chip, which drops it from the results.
func subjectFromTitle(title string) string {
	parts := strings.Split(title, " · ")
	if len(parts) < 2 {
		return ""
	}
	subject := strings.TrimSpace(parts[1])
	// Variable products title as "Polo — Large / Blue". The parent name
	// is the half worth asking about; the variation axis just makes the
	// chip longer without making it more answerable.
	if i := strings.Index(subject, " — "); i > 0 {
		subject = strings.TrimSpace(subject[:i])
	}
	if subject == "" || len([]rune(subject)) > maxSubjectLen {
		return ""
	}
	return subject
}
