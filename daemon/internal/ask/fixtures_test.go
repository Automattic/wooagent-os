package ask_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"
)

// fixtureSet bundles all the seed data the eval harness needs.
// Fixtures are Go literals rather than checked-in JSON so test edits +
// schema migrations stay in one place. If the schema drifts, this file
// fails to compile and the eval is unrunnable until updated.
//
// Naming:
// - Proposal IDs are "i_<persona>_<n>" (i_mk_1, i_pr_2, ...).
// - Run IDs are "r_<persona>_<n>".
//
// All timestamps are anchored to a reference "now" passed in by the
// harness so age-based assertions ("oldest pending", "stuck >24h") are
// reproducible.
type fixtureSet struct {
	now      time.Time
	issues   []issueFixture
	runs     []runFixture
}

type issueFixture struct {
	ID              string
	Title           string
	Description     string
	Persona         string
	Status          string // raw daemon status (in_review|done|rejected|dismissed)
	Priority        string
	CreatedAgo      time.Duration // relative to fixtureSet.now
	UpdatedAgo      time.Duration
	ProposalType    string
	ProposalContent string
	DismissReason   string
	DismissComment  string
	DismissedAgo    time.Duration
}

type runFixture struct {
	ID            string
	Persona       string
	Trigger       string
	Status        string
	IssueID       string
	CreatedAgo    time.Duration
	CompletedAgo  time.Duration
	LatencyMS     int64
	FailureReason string
}

// canonicalFixtures returns the seed set used by every eval test in
// this package. 12 proposals (oldest pending = 47 min; one stuck >24h;
// mix of personas + states + rejected with notes), 8 runs (varied
// triggers + outcomes including a failed Pricing run).
//
// Keep edits surgical: each fixture is referenced by ID in eval-query
// assertions, so renaming "i_mk_1" forces a sweep of the test file.
// When adding new fixtures, prefer extending the slice over reshuffling.
func canonicalFixtures(now time.Time) fixtureSet {
	return fixtureSet{
		now: now,
		issues: []issueFixture{
			{
				ID: "i_mk_1", Title: "Product description rewrite · Linen Napkin",
				Persona: "marketing", Status: "in_review", Priority: "medium",
				CreatedAgo: 47 * time.Minute, UpdatedAgo: 47 * time.Minute,
				ProposalType:    "product_description_rewrite",
				ProposalContent: "Woven from European flax for everyday meals.",
			},
			{
				ID: "i_pr_1", Title: "Price change · Wool Throw · $28.00 → $24.00 (-14.3%)",
				Persona: "pricing", Status: "in_review", Priority: "medium",
				CreatedAgo: 30 * time.Minute, UpdatedAgo: 30 * time.Minute,
				ProposalType:    "product_price_change",
				ProposalContent: "Mid-tier retailers price comparable throws $22-$26.",
			},
			{
				ID: "i_ss_1", Title: "Customer note · order #4521 · Re: shipping",
				Persona: "sales-support", Status: "in_review", Priority: "high",
				CreatedAgo: 12 * time.Minute, UpdatedAgo: 12 * time.Minute,
				ProposalType:    "customer_reply_draft",
				ProposalContent: "Hi Priya, your napkins shipped this morning — tracking is on the way.",
			},
			{
				ID: "i_mk_2", Title: "Product description rewrite · Cotton Pillow",
				Persona: "marketing", Status: "in_review", Priority: "medium",
				CreatedAgo: 26 * time.Hour, UpdatedAgo: 26 * time.Hour, // stuck >24h
				ProposalType:    "product_description_rewrite",
				ProposalContent: "Hand-stitched in a Brooklyn studio.",
			},
			{
				ID: "i_pr_2", Title: "Price change · Brass Candlestick · $42.00 → $38.00 (-9.5%)",
				Persona: "pricing", Status: "done", Priority: "medium",
				CreatedAgo: 3 * 24 * time.Hour, UpdatedAgo: 2 * 24 * time.Hour,
				ProposalType:    "product_price_change",
				ProposalContent: "Approved last weekend after benchmarking against 4 retailers.",
			},
			{
				ID: "i_mk_3", Title: "Product description rewrite · Stoneware Bowl",
				Persona: "marketing", Status: "rejected", Priority: "medium",
				CreatedAgo:      5 * 24 * time.Hour,
				UpdatedAgo:      4 * 24 * time.Hour,
				ProposalType:    "product_description_rewrite",
				ProposalContent: "Variant B body emphasized provenance.",
				DismissReason:   "tone_off",
				DismissComment:  "Too florid for this product line.",
				DismissedAgo:    4 * 24 * time.Hour,
			},
			{
				ID: "i_ss_2", Title: "Customer note · order #4499 · Re: refund",
				Persona: "sales-support", Status: "done", Priority: "high",
				CreatedAgo: 2 * 24 * time.Hour, UpdatedAgo: 2 * 24 * time.Hour,
				ProposalType:    "customer_reply_draft",
				ProposalContent: "Refund issued and replacement set out same-day.",
			},
			{
				ID: "i_pr_3", Title: "Price change · Linen Tea Towel · $14.00 → $12.00 (-14.3%)",
				Persona: "pricing", Status: "in_review", Priority: "low",
				CreatedAgo: 4 * time.Hour, UpdatedAgo: 4 * time.Hour,
				ProposalType:    "product_price_change",
				ProposalContent: "Tea towels at $12 match the mid-tier band.",
			},
			{
				ID: "i_mk_4", Title: "Social post · Mother's Day line",
				Persona: "marketing", Status: "in_review", Priority: "medium",
				CreatedAgo: 6 * time.Hour, UpdatedAgo: 6 * time.Hour,
				ProposalType:    "social_post",
				ProposalContent: "A small set for the people who keep the house running.",
			},
			{
				ID: "i_ss_3", Title: "Customer note · order #4470 · Re: damaged on arrival",
				Persona: "sales-support", Status: "in_review", Priority: "high",
				CreatedAgo: 2 * time.Hour, UpdatedAgo: 2 * time.Hour,
				ProposalType:    "customer_reply_draft",
				ProposalContent: "Apologies for the damaged shipment — sending a replacement.",
			},
			{
				ID: "i_pr_4", Title: "Price change · Beeswax Candle · $18.00 → $16.00 (-11.1%)",
				Persona: "pricing", Status: "rejected", Priority: "medium",
				CreatedAgo:      8 * 24 * time.Hour,
				UpdatedAgo:      7 * 24 * time.Hour,
				ProposalType:    "product_price_change",
				ProposalContent: "Sources skewed to a single retailer's clearance pricing.",
				DismissReason:   "needs_brand_review",
				DismissedAgo:    7 * 24 * time.Hour,
			},
			{
				ID: "i_mk_5", Title: "Launch copy · Spring tabletop collection",
				Persona: "marketing", Status: "in_review", Priority: "medium",
				CreatedAgo: 90 * time.Minute, UpdatedAgo: 90 * time.Minute,
				ProposalType:    "launch_copy",
				ProposalContent: "Five pieces for the way you actually set the table.",
			},
		},
		runs: []runFixture{
			{ID: "r_mk_1", Persona: "marketing", Trigger: "tick", Status: "succeeded", IssueID: "i_mk_1", CreatedAgo: 47 * time.Minute, CompletedAgo: 46 * time.Minute, LatencyMS: 8_500},
			{ID: "r_pr_1", Persona: "pricing", Trigger: "tick", Status: "succeeded", IssueID: "i_pr_1", CreatedAgo: 30 * time.Minute, CompletedAgo: 29 * time.Minute, LatencyMS: 55_400},
			{ID: "r_ss_1", Persona: "sales-support", Trigger: "operator-asked", Status: "succeeded", IssueID: "i_ss_1", CreatedAgo: 12 * time.Minute, CompletedAgo: 11 * time.Minute, LatencyMS: 14_700},
			{ID: "r_pr_2", Persona: "pricing", Trigger: "tick", Status: "skipped", CreatedAgo: 10 * time.Minute, CompletedAgo: 10 * time.Minute, LatencyMS: 6_200, FailureReason: ""},
			{ID: "r_pr_3", Persona: "pricing", Trigger: "tick", Status: "failed", CreatedAgo: 3 * time.Hour, CompletedAgo: 3 * time.Hour, LatencyMS: 12_000, FailureReason: "web_search returned 0 comparables; insufficient grounding"},
			{ID: "r_mk_2", Persona: "marketing", Trigger: "tick", Status: "succeeded", IssueID: "i_mk_2", CreatedAgo: 26 * time.Hour, CompletedAgo: 26 * time.Hour, LatencyMS: 9_100},
			{ID: "r_ss_2", Persona: "sales-support", Trigger: "tick", Status: "succeeded", IssueID: "i_ss_3", CreatedAgo: 2 * time.Hour, CompletedAgo: 2 * time.Hour, LatencyMS: 13_200},
			{ID: "r_mk_3", Persona: "marketing", Trigger: "operator-asked", Status: "succeeded", IssueID: "i_mk_4", CreatedAgo: 6 * time.Hour, CompletedAgo: 6 * time.Hour, LatencyMS: 10_500},
		},
	}
}

// seed installs the fixture set into an in-memory store. Idempotent —
// safe to call from within sub-tests that share a parent harness DB.
func (f fixtureSet) seed(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	seen := map[string]bool{}
	for _, iss := range f.issues {
		if !seen[iss.Persona] {
			ensureAgentRow(t, ctx, db, iss.Persona)
			seen[iss.Persona] = true
		}
		created := f.now.Add(-iss.CreatedAgo).UTC().Format(time.RFC3339)
		updated := f.now.Add(-iss.UpdatedAgo).UTC().Format(time.RFC3339)
		var dismissedAt any
		if iss.DismissedAgo > 0 {
			dismissedAt = f.now.Add(-iss.DismissedAgo).UTC().Format(time.RFC3339)
		}
		var dismissReason, dismissComment any
		if iss.DismissReason != "" {
			dismissReason = iss.DismissReason
		}
		if iss.DismissComment != "" {
			dismissComment = iss.DismissComment
		}
		_, err := db.ExecContext(ctx, `
			INSERT INTO issues
			    (id, title, description, persona, status, priority,
			     created_at, updated_at, proposal_type, proposal_content,
			     dismiss_reason, dismiss_comment, dismissed_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
		`, iss.ID, iss.Title, iss.Description, iss.Persona, iss.Status, iss.Priority,
			created, updated, iss.ProposalType, iss.ProposalContent,
			dismissReason, dismissComment, dismissedAt)
		if err != nil {
			t.Fatalf("seed issue %s: %v", iss.ID, err)
		}
	}
	for _, run := range f.runs {
		if !seen[run.Persona] {
			ensureAgentRow(t, ctx, db, run.Persona)
			seen[run.Persona] = true
		}
		created := f.now.Add(-run.CreatedAgo).UTC().Format(time.RFC3339)
		var completed any
		if run.CompletedAgo > 0 {
			completed = f.now.Add(-run.CompletedAgo).UTC().Format(time.RFC3339)
		}
		var issueID any
		if run.IssueID != "" {
			issueID = run.IssueID
		}
		_, err := db.ExecContext(ctx, `
			INSERT INTO runs
			    (id, persona, trigger, status, attempt,
			     scheduled_at, claimed_at, completed_at, latency_ms,
			     issue_id, failure_reason, created_at)
			VALUES (?,?,?,?,1,?,?,?,?,?,NULLIF(?, ''),?)
		`, run.ID, run.Persona, run.Trigger, run.Status,
			created, created, completed, run.LatencyMS,
			issueID, run.FailureReason, created)
		if err != nil {
			t.Fatalf("seed run %s: %v", run.ID, err)
		}
	}
}

// ensureAgentRow guarantees there's an `agents` row for slug so FKs
// from issues / runs resolve. INSERT OR IGNORE so it's safe to call
// multiple times.
func ensureAgentRow(t *testing.T, ctx context.Context, db *sql.DB, slug string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.ExecContext(ctx, `
		INSERT OR IGNORE INTO agents
		    (persona, name, enabled, created_at, updated_at,
		     cadence_seconds, max_attempts)
		VALUES (?, ?, 1, ?, ?, 21600, 3)
	`, slug, slug, now, now)
	if err != nil {
		t.Fatalf("ensure agent %s: %v", slug, err)
	}
}

// fixtureSummary returns a short human-readable summary used by the
// eval-log header line. Helps debugging when a query returns
// surprising results — the operator can see which fixtures were in
// scope.
func (f fixtureSet) summary() string {
	var pending, approved, rejected int
	for _, iss := range f.issues {
		switch iss.Status {
		case "in_review":
			pending++
		case "done":
			approved++
		case "rejected", "dismissed":
			rejected++
		}
	}
	return fmt.Sprintf("%d issues (%d pending / %d approved / %d rejected) · %d runs",
		len(f.issues), pending, approved, rejected, len(f.runs))
}
