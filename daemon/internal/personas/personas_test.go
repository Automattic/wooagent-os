package personas

import (
	"context"
	"testing"
	"time"

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
