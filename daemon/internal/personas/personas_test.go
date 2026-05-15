package personas

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

// fakePersona is a deterministic Persona for the registry/guard tests.
// Drafted is whatever the test wants; no MCP, no LLM.
type fakePersona struct {
	slug    string
	drafted Drafted
	err     error
}

func (f fakePersona) Slug() string                                  { return f.slug }
func (f fakePersona) DisplayName() string                           { return "fake " + f.slug }
func (f fakePersona) Draft(_ context.Context, _ Deps) (Drafted, error) { return f.drafted, f.err }

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func seedAgent(t *testing.T, st *store.Store, slug string, enabled int) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := st.DB.ExecContext(context.Background(),
		`INSERT INTO agents(persona, name, enabled, created_at, updated_at) VALUES(?, ?, ?, ?, ?)`,
		slug, "Test "+slug, enabled, now, now,
	); err != nil {
		t.Fatalf("seed agent %q: %v", slug, err)
	}
}

func TestRunAndPersist_DisabledPersonaIsSkipped(t *testing.T) {
	st := newStore(t)
	seedAgent(t, st, "fake-disabled", 0)
	p := fakePersona{slug: "fake-disabled", drafted: Drafted{
		Title: "shouldn't be inserted", ProposalType: "x", ProposalContent: "y",
	}}
	res, err := RunAndPersist(context.Background(), p, Deps{Store: st})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Skipped {
		t.Errorf("expected skipped, got %+v", res)
	}
	if res.IssueID != "" {
		t.Errorf("expected no IssueID; got %q", res.IssueID)
	}
}

func TestRunAndPersist_HappyPath(t *testing.T) {
	st := newStore(t)
	seedAgent(t, st, "fake-ok", 1)
	p := fakePersona{slug: "fake-ok", drafted: Drafted{
		Title:           "Test issue",
		Priority:        "medium",
		ProposalType:    "test_proposal",
		ProposalContent: "body text",
		Target:          map[string]any{"product_id": 1},
	}}
	res, err := RunAndPersist(context.Background(), p, Deps{Store: st})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Skipped {
		t.Errorf("expected not skipped; got %+v", res)
	}
	if res.IssueID == "" {
		t.Errorf("expected issue id, got empty")
	}

	// Inserted with the expected shape.
	var status, persona, ptype, pcontent string
	if err := st.DB.QueryRowContext(context.Background(),
		`SELECT status, persona, proposal_type, proposal_content FROM issues WHERE id = ?`,
		res.IssueID,
	).Scan(&status, &persona, &ptype, &pcontent); err != nil {
		t.Fatalf("read issue: %v", err)
	}
	if status != "in_review" {
		t.Errorf("status=%q, want in_review", status)
	}
	if persona != "fake-ok" {
		t.Errorf("persona=%q, want fake-ok", persona)
	}
	if ptype != "test_proposal" {
		t.Errorf("proposal_type=%q, want test_proposal", ptype)
	}
	if pcontent != "body text" {
		t.Errorf("proposal_content=%q, want %q", pcontent, "body text")
	}
}

func TestRunAndPersist_DuplicateGuard(t *testing.T) {
	st := newStore(t)
	seedAgent(t, st, "fake-dup", 1)
	p := fakePersona{slug: "fake-dup", drafted: Drafted{
		Title: "first", Priority: "medium",
		ProposalType: "x", ProposalContent: "1",
	}}
	if _, err := RunAndPersist(context.Background(), p, Deps{Store: st}); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Second run should skip — there's already an in_review issue.
	p2 := fakePersona{slug: "fake-dup", drafted: Drafted{
		Title: "second", ProposalType: "x", ProposalContent: "2",
	}}
	res, err := RunAndPersist(context.Background(), p2, Deps{Store: st})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if !res.Skipped {
		t.Errorf("expected duplicate guard to skip; got %+v", res)
	}
	if res.IssueID != "" {
		t.Errorf("expected no IssueID on skip; got %q", res.IssueID)
	}

	// Move the first issue to done — guard should now allow another seed.
	if _, err := st.DB.ExecContext(context.Background(),
		`UPDATE issues SET status='done' WHERE persona='fake-dup'`,
	); err != nil {
		t.Fatalf("flip status: %v", err)
	}
	res2, err := RunAndPersist(context.Background(), p2, Deps{Store: st})
	if err != nil {
		t.Fatalf("third run: %v", err)
	}
	if res2.Skipped {
		t.Errorf("expected new issue after first one done; got skipped: %s", res2.SkipReason)
	}
	if res2.IssueID == "" {
		t.Errorf("expected new IssueID; got empty")
	}
}

// seedIssue inserts a fixture row directly so the test controls the
// status, dismissed_at, updated_at, and proposal_target shape — bypassing
// RunAndPersist's auto-set values. productID == 0 means "omit product_id
// from proposal_target"; dismissedAt == "" means leave NULL.
func seedIssue(
	t *testing.T,
	st *store.Store,
	persona, status string,
	productID int,
	updatedAt, dismissedAt string,
) {
	t.Helper()
	var target string
	if productID > 0 {
		target = `{"product_id":` + strconv.Itoa(productID) + `}`
	}
	var targetArg any
	if target != "" {
		targetArg = target
	}
	_, err := st.DB.ExecContext(context.Background(),
		`INSERT INTO issues(id, title, description, persona, status, priority, created_at, updated_at, proposal_type, proposal_content, proposal_target, dismissed_at)
		 VALUES(?, 'fixture', '', ?, ?, 'medium', ?, ?, 'x', '', ?, NULLIF(?, ''))`,
		uuid.NewString(), persona, status, updatedAt, updatedAt, targetArg, dismissedAt,
	)
	if err != nil {
		t.Fatalf("seed issue: %v", err)
	}
}

func TestRecentlyTouchedProductIDs(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	// issues.persona has a FK to agents.persona, so seed both personas
	// referenced by the fixtures below.
	seedAgent(t, st, "marketer", 1)
	seedAgent(t, st, "pricer", 1)
	now := time.Now().UTC()
	rfc := func(d time.Duration) string {
		return now.Add(-d).Format(time.RFC3339)
	}

	// Fixtures for persona "marketer" — should be returned:
	seedIssue(t, st, "marketer", "in_review", 11, rfc(2*time.Hour), "")     // open
	seedIssue(t, st, "marketer", "in_progress", 12, rfc(2*time.Hour), "")   // open
	seedIssue(t, st, "marketer", "done", 13, rfc(6*24*time.Hour), "")       // approved 6d ago — under 7d
	seedIssue(t, st, "marketer", "dismissed", 14, rfc(29*24*time.Hour), rfc(29*24*time.Hour)) // dismissed 29d ago — under 30d

	// Fixtures for persona "marketer" — should NOT be returned:
	seedIssue(t, st, "marketer", "done", 21, rfc(8*24*time.Hour), "")        // approved 8d ago — over 7d cooldown
	seedIssue(t, st, "marketer", "dismissed", 22, rfc(31*24*time.Hour), rfc(31*24*time.Hour)) // dismissed 31d ago — over 30d cooldown
	seedIssue(t, st, "marketer", "rejected", 23, rfc(1*time.Hour), "")       // rejected is not in scope
	seedIssue(t, st, "marketer", "in_review", 0, rfc(1*time.Hour), "")       // no product_id in target
	seedIssue(t, st, "pricer", "in_review", 99, rfc(1*time.Hour), "")        // different persona

	got, err := RecentlyTouchedProductIDs(ctx, st, "marketer")
	if err != nil {
		t.Fatalf("RecentlyTouchedProductIDs: %v", err)
	}

	want := map[int]struct{}{11: {}, 12: {}, 13: {}, 14: {}}
	if len(got) != len(want) {
		t.Errorf("got %d ids, want %d; got=%v want=%v", len(got), len(want), got, want)
	}
	for id := range want {
		if _, ok := got[id]; !ok {
			t.Errorf("expected product_id %d in cooldown set, missing", id)
		}
	}
	for id := range got {
		if _, ok := want[id]; !ok {
			t.Errorf("unexpected product_id %d in cooldown set", id)
		}
	}

	// Empty result when persona has no issues at all.
	empty, err := RecentlyTouchedProductIDs(ctx, st, "ghost")
	if err != nil {
		t.Fatalf("ghost lookup: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected empty set for persona with no issues, got %v", empty)
	}
}

func TestRunAndPersist_DraftSkipPropagated(t *testing.T) {
	st := newStore(t)
	seedAgent(t, st, "fake-skip", 1)
	p := fakePersona{slug: "fake-skip", drafted: Drafted{
		Skipped: true, SkipReason: "no work to do",
	}}
	res, err := RunAndPersist(context.Background(), p, Deps{Store: st})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Skipped {
		t.Errorf("expected skipped; got %+v", res)
	}
	if res.SkipReason != "no work to do" {
		t.Errorf("skip reason = %q, want %q", res.SkipReason, "no work to do")
	}
	// Verify NOTHING was inserted.
	var n int
	if err := st.DB.QueryRowContext(context.Background(),
		`SELECT count(*) FROM issues WHERE persona = ?`, "fake-skip",
	).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 issues after skipped Draft; got %d", n)
	}
}
