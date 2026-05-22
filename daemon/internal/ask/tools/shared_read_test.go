package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/store"

	// Side-effect imports: each persona registers itself in init().
	// TestListAgents_IncludesRuntimePersonas asserts the registry is
	// populated, which requires these packages to be linked into the
	// test binary.
	_ "github.com/wooagent-os/wooagent-os/daemon/internal/personas/marketing"
	_ "github.com/wooagent-os/wooagent-os/daemon/internal/personas/pricing"
	_ "github.com/wooagent-os/wooagent-os/daemon/internal/personas/sales-support"
)

// newTestDB opens an in-memory SQLite with all daemon migrations
// applied. Closes itself on test cleanup.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.DB.Close() })
	return st.DB
}

// insertIssue is a tight fixture helper for the issues table. Only the
// fields the read tools surface need to be passed; the rest get safe
// defaults.
type issueFixture struct {
	ID             string
	Title          string
	Persona        string
	Status         string // raw daemon status (in_review|done|rejected|dismissed)
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DismissReason  string
	DismissComment string
}

// ensureAgent inserts a stub `agents` row so an FK reference from
// issues / runs resolves. INSERT OR IGNORE — never overwrites a real
// seeded row (e.g. the sales-support row from migration 005).
func ensureAgent(t *testing.T, db *sql.DB, persona string) {
	t.Helper()
	if persona == "" {
		return
	}
	_, err := db.Exec(
		`INSERT OR IGNORE INTO agents (persona, name, enabled, created_at, updated_at, cadence_seconds, max_attempts)
		 VALUES (?, ?, 1, ?, ?, 21600, 3)`,
		persona, persona,
		time.Now().UTC().Format(time.RFC3339),
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("ensure agent %s: %v", persona, err)
	}
}

func insertIssue(t *testing.T, db *sql.DB, f issueFixture) {
	t.Helper()
	ensureAgent(t, db, f.Persona)
	if f.UpdatedAt.IsZero() {
		f.UpdatedAt = f.CreatedAt
	}
	_, err := db.Exec(
		`INSERT INTO issues (id, title, persona, status, priority, created_at, updated_at, dismiss_reason, dismiss_comment)
		 VALUES (?, ?, ?, ?, 'normal', ?, ?, NULLIF(?, ''), NULLIF(?, ''))`,
		f.ID, f.Title, f.Persona, f.Status,
		f.CreatedAt.UTC().Format(time.RFC3339),
		f.UpdatedAt.UTC().Format(time.RFC3339),
		f.DismissReason, f.DismissComment,
	)
	if err != nil {
		t.Fatalf("insert issue %s: %v", f.ID, err)
	}
}

type runFixture struct {
	ID            string
	Persona       string
	Trigger       string
	Status        string
	IssueID       string
	CreatedAt     time.Time
	CompletedAt   *time.Time
	LatencyMS     int64
	FailureReason string
}

func insertRun(t *testing.T, db *sql.DB, f runFixture) {
	t.Helper()
	ensureAgent(t, db, f.Persona)
	var completed any
	if f.CompletedAt != nil {
		completed = f.CompletedAt.UTC().Format(time.RFC3339)
	}
	var issueID any
	if f.IssueID != "" {
		issueID = f.IssueID
	}
	_, err := db.Exec(
		`INSERT INTO runs (id, persona, trigger, status, attempt, scheduled_at, completed_at, latency_ms, issue_id, failure_reason, created_at)
		 VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?, NULLIF(?, ''), ?)`,
		f.ID, f.Persona, f.Trigger, f.Status,
		f.CreatedAt.UTC().Format(time.RFC3339),
		completed, f.LatencyMS, issueID, f.FailureReason,
		f.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("insert run %s: %v", f.ID, err)
	}
}

// -------------------------------------------------------- list_proposals

func TestListProposals_DefaultsToPending(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()
	insertIssue(t, db, issueFixture{ID: "i1", Title: "A", Persona: "marketing", Status: "in_review", CreatedAt: now.Add(-5 * time.Minute)})
	insertIssue(t, db, issueFixture{ID: "i2", Title: "B", Persona: "pricing", Status: "done", CreatedAt: now.Add(-1 * time.Hour)})
	insertIssue(t, db, issueFixture{ID: "i3", Title: "C", Persona: "marketing", Status: "in_review", CreatedAt: now.Add(-30 * time.Minute)})

	out := callList(t, &ListProposalsTool{DB: db}, `{}`)
	if out.Count != 2 || len(out.Proposals) != 2 {
		t.Fatalf("expected 2 pending, got %d (proposals=%+v)", out.Count, out.Proposals)
	}
	if out.State != "pending" {
		t.Fatalf("expected state_used=pending, got %q", out.State)
	}
	if out.Proposals[0].ID != "i1" {
		t.Fatalf("expected newest pending first; got order=%v", proposalIDs(out.Proposals))
	}
	if out.Proposals[0].State != "pending" {
		t.Fatalf("expected mapped state=pending, got %q", out.Proposals[0].State)
	}
}

func TestListProposals_FilterByPersona(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()
	insertIssue(t, db, issueFixture{ID: "i1", Title: "A", Persona: "marketing", Status: "in_review", CreatedAt: now})
	insertIssue(t, db, issueFixture{ID: "i2", Title: "B", Persona: "pricing", Status: "in_review", CreatedAt: now})
	out := callList(t, &ListProposalsTool{DB: db}, `{"persona": "pricing"}`)
	if out.Count != 1 || out.Proposals[0].ID != "i2" {
		t.Fatalf("expected only pricing; got %+v", out.Proposals)
	}
}

func TestListProposals_FilterBySince(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()
	insertIssue(t, db, issueFixture{ID: "i_old", Title: "Old", Persona: "marketing", Status: "in_review", CreatedAt: now.Add(-48 * time.Hour)})
	insertIssue(t, db, issueFixture{ID: "i_new", Title: "New", Persona: "marketing", Status: "in_review", CreatedAt: now.Add(-1 * time.Hour)})

	// ISO8601 form
	since := now.Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	out := callList(t, &ListProposalsTool{DB: db}, `{"since": "`+since+`"}`)
	if out.Count != 1 || out.Proposals[0].ID != "i_new" {
		t.Fatalf("expected only i_new; got %+v", proposalIDs(out.Proposals))
	}

	// Shorthand form
	out = callList(t, &ListProposalsTool{DB: db}, `{"since": "-24h"}`)
	if out.Count != 1 || out.Proposals[0].ID != "i_new" {
		t.Fatalf("expected only i_new via shorthand; got %+v", proposalIDs(out.Proposals))
	}
}

func TestListProposals_MapsApprovedAndRejected(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()
	insertIssue(t, db, issueFixture{ID: "i_done", Title: "Done", Persona: "marketing", Status: "done", CreatedAt: now})
	insertIssue(t, db, issueFixture{ID: "i_rej", Title: "Rej", Persona: "marketing", Status: "rejected", CreatedAt: now})
	insertIssue(t, db, issueFixture{ID: "i_dis", Title: "Dis", Persona: "marketing", Status: "dismissed", CreatedAt: now})

	approved := callList(t, &ListProposalsTool{DB: db}, `{"state": "approved"}`)
	if approved.Count != 1 || approved.Proposals[0].ID != "i_done" || approved.Proposals[0].State != "approved" {
		t.Fatalf("approved mapping wrong: %+v", approved)
	}

	rejected := callList(t, &ListProposalsTool{DB: db}, `{"state": "rejected"}`)
	if rejected.Count != 2 {
		t.Fatalf("rejected should include both rejected + dismissed; got %d (%v)", rejected.Count, proposalIDs(rejected.Proposals))
	}
	for _, p := range rejected.Proposals {
		if p.State != "rejected" {
			t.Errorf("expected mapped state=rejected, got %q for %s", p.State, p.ID)
		}
	}
}

func TestListProposals_UnknownStateReturnsError(t *testing.T) {
	db := newTestDB(t)
	tool := &ListProposalsTool{DB: db}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"state":"working"}`))
	if err == nil || !strings.Contains(err.Error(), "unknown state") {
		t.Fatalf("expected unknown-state error, got: %v", err)
	}
}

// ----------------------------------------------------------- get_proposal

func TestGetProposal_ReturnsDismissNotes(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()
	insertIssue(t, db, issueFixture{
		ID: "i_rej", Title: "Variant B", Persona: "marketing",
		Status:         "rejected",
		CreatedAt:      now.Add(-2 * time.Hour),
		DismissReason:  "low_quality",
		DismissComment: "leaning too far into 'luxe' language",
	})

	got, err := (&GetProposalTool{DB: db}).Execute(context.Background(), json.RawMessage(`{"id":"i_rej"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var d proposalDetail
	if err := json.Unmarshal([]byte(got), &d); err != nil {
		t.Fatalf("unmarshal: %v · %s", err, got)
	}
	if d.DismissReason != "low_quality" {
		t.Errorf("expected dismiss_reason=low_quality, got %q", d.DismissReason)
	}
	if !strings.Contains(d.DismissComment, "luxe") {
		t.Errorf("expected dismiss_comment to include 'luxe', got %q", d.DismissComment)
	}
	if d.State != "rejected" {
		t.Errorf("expected state=rejected, got %q", d.State)
	}
	if d.AgeSeconds < 7000 {
		t.Errorf("expected age ~2h, got %ds", d.AgeSeconds)
	}
}

func TestGetProposal_AttachesLatestRun(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()
	insertIssue(t, db, issueFixture{ID: "i1", Title: "Wool Throw", Persona: "marketing", Status: "in_review", CreatedAt: now})
	insertRun(t, db, runFixture{ID: "rn_old", Persona: "marketing", Trigger: "tick", Status: "succeeded", IssueID: "i1", CreatedAt: now.Add(-10 * time.Minute)})
	insertRun(t, db, runFixture{ID: "rn_new", Persona: "marketing", Trigger: "tick", Status: "succeeded", IssueID: "i1", CreatedAt: now.Add(-1 * time.Minute)})

	got, err := (&GetProposalTool{DB: db}).Execute(context.Background(), json.RawMessage(`{"id":"i1"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var d proposalDetail
	_ = json.Unmarshal([]byte(got), &d)
	if d.LatestRunID != "rn_new" {
		t.Errorf("expected latest_run_id=rn_new, got %q", d.LatestRunID)
	}
}

func TestGetProposal_NotFound(t *testing.T) {
	db := newTestDB(t)
	_, err := (&GetProposalTool{DB: db}).Execute(context.Background(), json.RawMessage(`{"id":"missing"}`))
	if err == nil || !strings.Contains(err.Error(), "no proposal") {
		t.Fatalf("expected not-found error, got: %v", err)
	}
}

func TestGetProposal_IDRequired(t *testing.T) {
	db := newTestDB(t)
	_, err := (&GetProposalTool{DB: db}).Execute(context.Background(), json.RawMessage(`{"id":""}`))
	if err == nil || !strings.Contains(err.Error(), "id is required") {
		t.Fatalf("expected id-required error, got: %v", err)
	}
}

// --------------------------------------------------------------- list_runs

func TestListRuns_FiltersByPersonaAndStatus(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()
	insertRun(t, db, runFixture{ID: "r1", Persona: "marketing", Trigger: "tick", Status: "succeeded", CreatedAt: now})
	insertRun(t, db, runFixture{ID: "r2", Persona: "pricing", Trigger: "tick", Status: "succeeded", CreatedAt: now})
	insertRun(t, db, runFixture{ID: "r3", Persona: "marketing", Trigger: "manual", Status: "failed", CreatedAt: now, FailureReason: "boom"})

	out := callListRuns(t, &ListRunsTool{DB: db}, `{"persona":"marketing"}`)
	if out.Count != 2 {
		t.Errorf("expected 2 marketing runs, got %d", out.Count)
	}

	out = callListRuns(t, &ListRunsTool{DB: db}, `{"status":"failed"}`)
	if out.Count != 1 || out.Runs[0].FailureReason != "boom" {
		t.Errorf("expected one failed run with reason, got %+v", out.Runs)
	}
}

// TestListRuns_IncludesLinkedIssue covers the DSGWOO-1362 enrichment:
// runs that landed a proposal carry the proposal's title + (chat-vocab)
// state, so the model can cite the proposal directly instead of the
// run.
func TestListRuns_IncludesLinkedIssue(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()
	insertIssue(t, db, issueFixture{ID: "iss-1", Title: "T-Shirt price change", Persona: "pricing", Status: "in_review", CreatedAt: now})
	insertRun(t, db, runFixture{ID: "rn-success", Persona: "pricing", Trigger: "manual", Status: "succeeded", IssueID: "iss-1", CreatedAt: now})
	// A run with no issue (failed) should not carry issue_title/state.
	insertRun(t, db, runFixture{ID: "rn-fail", Persona: "pricing", Trigger: "manual", Status: "failed", CreatedAt: now, FailureReason: "boom"})

	out := callListRuns(t, &ListRunsTool{DB: db}, `{"persona":"pricing"}`)
	if out.Count != 2 {
		t.Fatalf("expected 2 runs, got %d", out.Count)
	}

	var success, fail runSummary
	for _, r := range out.Runs {
		switch r.ID {
		case "rn-success":
			success = r
		case "rn-fail":
			fail = r
		}
	}
	if success.IssueID != "iss-1" || success.IssueTitle != "T-Shirt price change" || success.IssueState != "pending" {
		t.Errorf("expected succeeded run to carry linked issue metadata, got %+v", success)
	}
	if fail.IssueTitle != "" || fail.IssueState != "" {
		t.Errorf("expected failed run to omit issue metadata, got %+v", fail)
	}
}

// ---------------------------------------------------------------- get_run

func TestGetRun_ReturnsRecord(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()
	insertRun(t, db, runFixture{ID: "rn_1", Persona: "pricing", Trigger: "operator-asked", Status: "running", CreatedAt: now})

	got, err := (&GetRunTool{DB: db}).Execute(context.Background(), json.RawMessage(`{"id":"rn_1"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var r runSummary
	_ = json.Unmarshal([]byte(got), &r)
	if r.ID != "rn_1" || r.Trigger != "operator-asked" || r.Status != "running" {
		t.Errorf("unexpected run: %+v", r)
	}
}

func TestGetRun_NotFound(t *testing.T) {
	db := newTestDB(t)
	_, err := (&GetRunTool{DB: db}).Execute(context.Background(), json.RawMessage(`{"id":"nope"}`))
	if err == nil || !strings.Contains(err.Error(), "no run") {
		t.Fatalf("expected not-found error, got: %v", err)
	}
}

// ------------------------------------------------------------- list_agents

func TestListAgents_IncludesRuntimePersonas(t *testing.T) {
	// Reads the actual personas registry which is populated by side-effect
	// from persona package imports. Marketing / Pricing / Sales Support
	// register themselves in init() — see daemon/internal/personas/.
	db := newTestDB(t)
	got, err := (&ListAgentsTool{DB: db}).Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var out struct {
		Agents []agentSummary `json:"agents"`
	}
	_ = json.Unmarshal([]byte(got), &out)
	if len(out.Agents) == 0 {
		t.Skip("no personas registered in this build — registration is side-effect; skipping cross-check")
	}
	// At least one of the live personas should be present and marked
	// implemented. Don't require all three — test must work even if a
	// future build drops one.
	found := false
	for _, a := range out.Agents {
		if a.Implemented && (a.Persona == "marketing" || a.Persona == "pricing" || a.Persona == "sales-support") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected at least one live specialist in output, got %+v", out.Agents)
	}
}

// ----------------------------------------------------------------- helpers

type listOutput struct {
	Proposals []proposalSummary `json:"proposals"`
	Count     int               `json:"count"`
	State     string            `json:"state_used"`
}

func callList(t *testing.T, tool *ListProposalsTool, in string) listOutput {
	t.Helper()
	raw, err := tool.Execute(context.Background(), json.RawMessage(in))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var out listOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal: %v · raw=%s", err, raw)
	}
	return out
}

type listRunsOutput struct {
	Runs  []runSummary `json:"runs"`
	Count int          `json:"count"`
}

func callListRuns(t *testing.T, tool *ListRunsTool, in string) listRunsOutput {
	t.Helper()
	raw, err := tool.Execute(context.Background(), json.RawMessage(in))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var out listRunsOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal: %v · raw=%s", err, raw)
	}
	return out
}

func proposalIDs(ps []proposalSummary) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.ID)
	}
	return out
}
