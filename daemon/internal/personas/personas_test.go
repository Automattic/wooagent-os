package personas

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
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
func (f fakePersona) Addable() bool                                 { return false }
func (f fakePersona) Cooldown() CooldownPolicy {
	// fakePersona's Draft doesn't pick a target, so the policy is unused
	// by the registry/lifecycle tests. Return a non-zero policy so any
	// future caller that does consult it isn't surprised by zero values.
	return CooldownPolicy{TargetKey: "product_id", Approved: 7 * 24 * time.Hour, Dismissed: 30 * 24 * time.Hour}
}
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

func TestMigration_LLMSkipsTableExists(t *testing.T) {
	st := newStore(t) // store.Open applies all embedded migrations
	var name string
	err := st.DB.QueryRowContext(context.Background(),
		`SELECT name FROM sqlite_master WHERE type='table' AND name='llm_skips'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("llm_skips table not found after migrations: %v", err)
	}
	if name != "llm_skips" {
		t.Fatalf("got table %q, want llm_skips", name)
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

	// One open issue is below the threshold (OpenProposalSkipThreshold=2):
	// the guard should still let a second proposal through.
	p2 := fakePersona{slug: "fake-dup", drafted: Drafted{
		Title: "second", ProposalType: "x", ProposalContent: "2",
	}}
	res, err := RunAndPersist(context.Background(), p2, Deps{Store: st})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if res.Skipped {
		t.Errorf("expected second run to proceed (1 open < threshold); got skip: %s", res.SkipReason)
	}
	if res.IssueID == "" {
		t.Errorf("expected IssueID on second proposal; got empty")
	}

	// Now there are 2 open issues — guard should skip.
	p3 := fakePersona{slug: "fake-dup", drafted: Drafted{
		Title: "third", ProposalType: "x", ProposalContent: "3",
	}}
	res2, err := RunAndPersist(context.Background(), p3, Deps{Store: st})
	if err != nil {
		t.Fatalf("third run: %v", err)
	}
	if !res2.Skipped {
		t.Errorf("expected duplicate guard to skip at 2 open; got %+v", res2)
	}
	if res2.IssueID != "" {
		t.Errorf("expected no IssueID on skip; got %q", res2.IssueID)
	}

	// Move both open issues to done — guard should now allow another seed.
	if _, err := st.DB.ExecContext(context.Background(),
		`UPDATE issues SET status='done' WHERE persona='fake-dup'`,
	); err != nil {
		t.Fatalf("flip status: %v", err)
	}
	res3, err := RunAndPersist(context.Background(), p3, Deps{Store: st})
	if err != nil {
		t.Fatalf("fourth run: %v", err)
	}
	if res3.Skipped {
		t.Errorf("expected new issue after open ones resolved; got skipped: %s", res3.SkipReason)
	}
	if res3.IssueID == "" {
		t.Errorf("expected new IssueID; got empty")
	}
}

// seedIssue inserts a fixture row directly so the test controls the
// status, dismissed_at, updated_at, and proposal_target shape — bypassing
// RunAndPersist's auto-set values. targetID == 0 means "omit the key from
// proposal_target"; dismissedAt == "" means leave NULL. targetKey is the
// JSON key under which targetID lands (e.g. "product_id" or "order_id").
func seedIssue(
	t *testing.T,
	st *store.Store,
	persona, status string,
	targetKey string,
	targetID int,
	updatedAt, dismissedAt string,
) {
	t.Helper()
	var target string
	if targetID > 0 && targetKey != "" {
		target = `{"` + targetKey + `":` + strconv.Itoa(targetID) + `}`
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

func TestRecentlyTouchedTargets_ProductCentric(t *testing.T) {
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
	policy := CooldownPolicy{
		TargetKey: "product_id",
		Approved:  7 * 24 * time.Hour,
		Dismissed: 30 * 24 * time.Hour,
	}

	// Fixtures for persona "marketer" — should be returned:
	seedIssue(t, st, "marketer", "in_review", "product_id", 11, rfc(2*time.Hour), "")     // open
	seedIssue(t, st, "marketer", "in_progress", "product_id", 12, rfc(2*time.Hour), "")   // open
	seedIssue(t, st, "marketer", "done", "product_id", 13, rfc(6*24*time.Hour), "")       // approved 6d ago — under 7d
	seedIssue(t, st, "marketer", "dismissed", "product_id", 14, rfc(29*24*time.Hour), rfc(29*24*time.Hour)) // dismissed 29d ago — under 30d

	// Fixtures for persona "marketer" — should NOT be returned:
	seedIssue(t, st, "marketer", "done", "product_id", 21, rfc(8*24*time.Hour), "")                          // approved 8d ago — over 7d cooldown
	seedIssue(t, st, "marketer", "dismissed", "product_id", 22, rfc(31*24*time.Hour), rfc(31*24*time.Hour))  // dismissed 31d ago — over 30d cooldown
	seedIssue(t, st, "marketer", "rejected", "product_id", 23, rfc(1*time.Hour), "")                          // rejected is not in scope
	seedIssue(t, st, "marketer", "in_review", "product_id", 0, rfc(1*time.Hour), "")                         // no product_id in target
	seedIssue(t, st, "marketer", "in_review", "order_id", 77, rfc(1*time.Hour), "")                          // target under a different key — ignored
	seedIssue(t, st, "pricer", "in_review", "product_id", 99, rfc(1*time.Hour), "")                          // different persona

	got, err := RecentlyTouchedTargets(ctx, st, "marketer", policy)
	if err != nil {
		t.Fatalf("RecentlyTouchedTargets: %v", err)
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
	empty, err := RecentlyTouchedTargets(ctx, st, "ghost", policy)
	if err != nil {
		t.Fatalf("ghost lookup: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected empty set for persona with no issues, got %v", empty)
	}
}

// Sales-support-style policy: order_id key, longer windows (30d/90d).
// Verifies (a) different target keys are honored, (b) different cooldown
// windows are honored, and (c) cross-key contamination doesn't happen.
func TestRecentlyTouchedTargets_OrderCentricStricterWindows(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	seedAgent(t, st, "ss", 1)
	now := time.Now().UTC()
	rfc := func(d time.Duration) string {
		return now.Add(-d).Format(time.RFC3339)
	}
	policy := CooldownPolicy{
		TargetKey: "order_id",
		Approved:  30 * 24 * time.Hour,
		Dismissed: 90 * 24 * time.Hour,
	}

	// Should be returned:
	seedIssue(t, st, "ss", "in_review", "order_id", 100, rfc(1*time.Hour), "")                            // open
	seedIssue(t, st, "ss", "done", "order_id", 101, rfc(29*24*time.Hour), "")                              // approved 29d ago — under 30d
	seedIssue(t, st, "ss", "dismissed", "order_id", 102, rfc(89*24*time.Hour), rfc(89*24*time.Hour))       // dismissed 89d ago — under 90d

	// Should NOT be returned:
	seedIssue(t, st, "ss", "done", "order_id", 201, rfc(31*24*time.Hour), "")                              // approved 31d ago — over 30d
	seedIssue(t, st, "ss", "dismissed", "order_id", 202, rfc(91*24*time.Hour), rfc(91*24*time.Hour))       // dismissed 91d ago — over 90d
	seedIssue(t, st, "ss", "in_review", "product_id", 203, rfc(1*time.Hour), "")                           // wrong key (would match if SQL ignored TargetKey)

	got, err := RecentlyTouchedTargets(ctx, st, "ss", policy)
	if err != nil {
		t.Fatalf("RecentlyTouchedTargets: %v", err)
	}

	want := map[int]struct{}{100: {}, 101: {}, 102: {}}
	if len(got) != len(want) {
		t.Errorf("got %d ids, want %d; got=%v want=%v", len(got), len(want), got, want)
	}
	for id := range want {
		if _, ok := got[id]; !ok {
			t.Errorf("expected order_id %d in cooldown set, missing", id)
		}
	}
	for id := range got {
		if _, ok := want[id]; !ok {
			t.Errorf("unexpected order_id %d in cooldown set (cross-key contamination?)", id)
		}
	}
}

func TestRecentlyTouchedTargets_EmptyTargetKeyErrors(t *testing.T) {
	st := newStore(t)
	_, err := RecentlyTouchedTargets(context.Background(), st, "ghost", CooldownPolicy{})
	if err == nil {
		t.Fatal("expected error for empty TargetKey, got nil")
	}
}

// ---- RunAndPersist dedup-key guard ----

func TestRunAndPersist_DedupKey_BlocksWhileInReview(t *testing.T) {
	st := newStore(t)
	seedAgent(t, st, "reporter", 1)
	p := fakePersona{slug: "reporter", drafted: Drafted{
		Title:           "Product health digest",
		ProposalType:    "product_health_digest",
		ProposalContent: "body",
		DedupKey:        "digest:data_issues",
	}}
	first, err := RunAndPersist(context.Background(), p, Deps{Store: st})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first.Skipped || first.IssueID == "" {
		t.Fatalf("first run: expected insert, got %+v", first)
	}

	// Second run, identical DedupKey, first issue still in_review.
	// Count-based guard would let this through (1 open < threshold),
	// so a block here proves the dedup-key guard fired.
	second, err := RunAndPersist(context.Background(), p, Deps{Store: st})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if !second.Skipped {
		t.Errorf("expected second run blocked by dedup-key guard; got %+v", second)
	}
	if second.IssueID != "" {
		t.Errorf("expected no IssueID on dup skip; got %q", second.IssueID)
	}
	if !strings.Contains(second.SkipReason, first.IssueID) {
		t.Errorf("SkipReason should reference duplicate id %q; got %q", first.IssueID, second.SkipReason)
	}
}

func TestRunAndPersist_DedupKey_EmptyKeyIsNoOp(t *testing.T) {
	st := newStore(t)
	seedAgent(t, st, "fake-empty", 1)
	p := fakePersona{slug: "fake-empty", drafted: Drafted{
		Title: "first", ProposalType: "x", ProposalContent: "1",
		// DedupKey deliberately empty — back-compat path.
	}}
	if _, err := RunAndPersist(context.Background(), p, Deps{Store: st}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	// Second emit with empty DedupKey must NOT be blocked by the
	// dedup-key guard. (Count threshold still applies; with one open,
	// we're under it.)
	res, err := RunAndPersist(context.Background(), p, Deps{Store: st})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if res.Skipped {
		t.Errorf("empty DedupKey should not block; got skip: %s", res.SkipReason)
	}
	if res.IssueID == "" {
		t.Errorf("expected second IssueID; got empty")
	}
}

func TestRunAndPersist_DedupKey_DoneRowDoesNotBlock(t *testing.T) {
	st := newStore(t)
	seedAgent(t, st, "reporter", 1)
	p := fakePersona{slug: "reporter", drafted: Drafted{
		Title:           "x",
		ProposalType:    "y",
		ProposalContent: "z",
		DedupKey:        "digest:data_issues",
	}}
	first, err := RunAndPersist(context.Background(), p, Deps{Store: st})
	if err != nil || first.IssueID == "" {
		t.Fatalf("first run: %v / %+v", err, first)
	}
	// Resolve it.
	if _, err := st.DB.ExecContext(context.Background(),
		`UPDATE issues SET status='done' WHERE id=?`, first.IssueID,
	); err != nil {
		t.Fatalf("flip to done: %v", err)
	}
	second, err := RunAndPersist(context.Background(), p, Deps{Store: st})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.Skipped {
		t.Errorf("done row should not block same DedupKey; got skip: %s", second.SkipReason)
	}
	if second.IssueID == "" {
		t.Errorf("expected new IssueID; got empty")
	}
}

// ---- findOpenIssueWithDedupKey ----

// seedIssueWithDedupKey is a sibling of seedIssue for the dedup-key tests.
// dedupKey "" lands as SQL NULL; non-empty lands as a literal string.
func seedIssueWithDedupKey(
	t *testing.T,
	st *store.Store,
	persona, status, dedupKey string,
) string {
	t.Helper()
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	var keyArg any
	if dedupKey != "" {
		keyArg = dedupKey
	}
	_, err := st.DB.ExecContext(context.Background(),
		`INSERT INTO issues(id, title, description, persona, status, priority, created_at, updated_at, proposal_type, proposal_content, dedup_key)
		 VALUES(?, 'fixture', '', ?, ?, 'medium', ?, ?, 'x', '', ?)`,
		id, persona, status, now, now, keyArg,
	)
	if err != nil {
		t.Fatalf("seed issue: %v", err)
	}
	return id
}

func TestFindOpenIssueWithDedupKey_HitsInReview(t *testing.T) {
	st := newStore(t)
	seedAgent(t, st, "reporter", 1)
	wantID := seedIssueWithDedupKey(t, st, "reporter", "in_review", "digest:data_issues")

	got, err := findOpenIssueWithDedupKey(context.Background(), st, "reporter", "digest:data_issues")
	if err != nil {
		t.Fatalf("findOpenIssueWithDedupKey: %v", err)
	}
	if got != wantID {
		t.Errorf("got id %q, want %q", got, wantID)
	}
}

func TestFindOpenIssueWithDedupKey_IgnoresNonInReview(t *testing.T) {
	st := newStore(t)
	seedAgent(t, st, "reporter", 1)
	// Three matching-key issues in non-in_review states; the guard must
	// not block on any of them — Cooldown owns approved/dismissed gating.
	seedIssueWithDedupKey(t, st, "reporter", "done", "digest:data_issues")
	seedIssueWithDedupKey(t, st, "reporter", "dismissed", "digest:data_issues")
	seedIssueWithDedupKey(t, st, "reporter", "rejected", "digest:data_issues")

	got, err := findOpenIssueWithDedupKey(context.Background(), st, "reporter", "digest:data_issues")
	if err != nil {
		t.Fatalf("findOpenIssueWithDedupKey: %v", err)
	}
	if got != "" {
		t.Errorf("got id %q, want empty (no in_review row)", got)
	}
}

func TestFindOpenIssueWithDedupKey_IgnoresOtherPersona(t *testing.T) {
	st := newStore(t)
	seedAgent(t, st, "reporter", 1)
	seedAgent(t, st, "marketer", 1)
	// Same key, different persona — must not collide. Cross-persona
	// dedup is out of scope (Marketing copy + Reporting digest can both
	// reference the same underlying data).
	seedIssueWithDedupKey(t, st, "marketer", "in_review", "digest:data_issues")

	got, err := findOpenIssueWithDedupKey(context.Background(), st, "reporter", "digest:data_issues")
	if err != nil {
		t.Fatalf("findOpenIssueWithDedupKey: %v", err)
	}
	if got != "" {
		t.Errorf("got id %q, want empty (cross-persona)", got)
	}
}

func TestFindOpenIssueWithDedupKey_NullStoredKey(t *testing.T) {
	st := newStore(t)
	seedAgent(t, st, "reporter", 1)
	// A NULL-keyed in_review row must never match a non-empty query —
	// otherwise existing personas that don't set DedupKey would
	// accidentally block each other.
	seedIssueWithDedupKey(t, st, "reporter", "in_review", "")

	got, err := findOpenIssueWithDedupKey(context.Background(), st, "reporter", "digest:data_issues")
	if err != nil {
		t.Fatalf("findOpenIssueWithDedupKey: %v", err)
	}
	if got != "" {
		t.Errorf("got id %q, want empty (NULL stored key)", got)
	}
}

// ---- insertIssue + Drafted.DedupKey ----

// dedupKeyFor reads the stored dedup_key for an issue id, returning the
// SQL value as (string, isNull) so tests can distinguish empty-string
// from NULL — the partial index hinges on the difference.
func dedupKeyFor(t *testing.T, st *store.Store, issueID string) (string, bool) {
	t.Helper()
	var key sql.NullString
	err := st.DB.QueryRowContext(context.Background(),
		`SELECT dedup_key FROM issues WHERE id = ?`, issueID,
	).Scan(&key)
	if err != nil {
		t.Fatalf("read dedup_key: %v", err)
	}
	return key.String, key.Valid
}

func TestInsertIssue_PersistsDedupKey(t *testing.T) {
	st := newStore(t)
	seedAgent(t, st, "reporter", 1)
	d := Drafted{
		Title:           "x",
		ProposalType:    "y",
		ProposalContent: "z",
		DedupKey:        "digest:data_issues",
	}
	id, err := insertIssue(context.Background(), st, "reporter", d)
	if err != nil {
		t.Fatalf("insertIssue: %v", err)
	}
	got, valid := dedupKeyFor(t, st, id)
	if !valid || got != "digest:data_issues" {
		t.Errorf("dedup_key: got (%q, valid=%v), want (\"digest:data_issues\", true)", got, valid)
	}
}

func TestInsertIssue_EmptyDedupKeyStoresNull(t *testing.T) {
	st := newStore(t)
	seedAgent(t, st, "reporter", 1)
	d := Drafted{
		Title:           "x",
		ProposalType:    "y",
		ProposalContent: "z",
		// DedupKey deliberately empty — back-compat for personas that
		// don't opt in must store SQL NULL so the partial index excludes
		// the row.
	}
	id, err := insertIssue(context.Background(), st, "reporter", d)
	if err != nil {
		t.Fatalf("insertIssue: %v", err)
	}
	_, valid := dedupKeyFor(t, st, id)
	if valid {
		t.Errorf("expected dedup_key NULL for empty DedupKey, got valid string")
	}
}

// ---- RecentlySkippedTargets ----

func seedSkip(t *testing.T, st *store.Store, persona string, targetID int, reason, attemptedAt string) {
	t.Helper()
	if _, err := st.DB.ExecContext(context.Background(),
		`INSERT INTO llm_skips(persona, target_id, skip_reason, attempted_at) VALUES(?, ?, ?, ?)`,
		persona, targetID, reason, attemptedAt,
	); err != nil {
		t.Fatalf("seed skip: %v", err)
	}
}

func TestRecentlySkippedTargets(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	seedAgent(t, st, "pricing", 1)
	seedAgent(t, st, "other", 1)
	now := time.Now().UTC()
	rfc := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }

	policy := CooldownPolicy{TargetKey: "product_id", Skipped: 7 * 24 * time.Hour}

	seedSkip(t, st, "pricing", 11, "price optimal", rfc(1*time.Hour)) // inside window
	seedSkip(t, st, "pricing", 12, "no comps", rfc(6*24*time.Hour))   // inside window
	seedSkip(t, st, "pricing", 21, "stale", rfc(8*24*time.Hour))      // outside window
	seedSkip(t, st, "other", 31, "price optimal", rfc(1*time.Hour))   // other persona

	got, err := RecentlySkippedTargets(ctx, st, "pricing", policy)
	if err != nil {
		t.Fatalf("RecentlySkippedTargets: %v", err)
	}
	want := map[int]struct{}{11: {}, 12: {}}
	if len(got) != len(want) {
		t.Errorf("got %d ids, want %d; got=%v", len(got), len(want), got)
	}
	for id := range want {
		if _, ok := got[id]; !ok {
			t.Errorf("expected product_id %d in skip set, missing", id)
		}
	}
	for id := range got {
		if _, ok := want[id]; !ok {
			t.Errorf("unexpected product_id %d in skip set", id)
		}
	}
}

func TestRecentlySkippedTargets_ZeroDurationReturnsEmpty(t *testing.T) {
	st := newStore(t)
	seedAgent(t, st, "pricing", 1)
	seedSkip(t, st, "pricing", 11, "price optimal", time.Now().UTC().Format(time.RFC3339))

	got, err := RecentlySkippedTargets(context.Background(), st, "pricing",
		CooldownPolicy{TargetKey: "product_id", Skipped: 0})
	if err != nil {
		t.Fatalf("RecentlySkippedTargets: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty set when Skipped==0, got %v", got)
	}
}

func TestRecordLLMSkip_InsertsAndUpserts(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	seedAgent(t, st, "pricing", 1)
	policy := CooldownPolicy{TargetKey: "product_id", Skipped: 7 * 24 * time.Hour}

	t0 := time.Now().UTC().Add(-time.Hour)
	if err := RecordLLMSkip(ctx, st, "pricing", 42, "price optimal", t0); err != nil {
		t.Fatalf("first RecordLLMSkip: %v", err)
	}
	got, err := RecentlySkippedTargets(ctx, st, "pricing", policy)
	if err != nil {
		t.Fatalf("RecentlySkippedTargets: %v", err)
	}
	if _, ok := got[42]; !ok || len(got) != 1 {
		t.Fatalf("after insert want {42}, got %v", got)
	}

	t1 := time.Now().UTC()
	if err := RecordLLMSkip(ctx, st, "pricing", 42, "no comps now", t1); err != nil {
		t.Fatalf("second RecordLLMSkip (upsert): %v", err)
	}
	var count int
	if err := st.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM llm_skips WHERE persona='pricing' AND target_id=42`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("want exactly 1 row after upsert, got %d", count)
	}
	var reason, attemptedAt string
	if err := st.DB.QueryRowContext(ctx,
		`SELECT skip_reason, attempted_at FROM llm_skips WHERE persona='pricing' AND target_id=42`,
	).Scan(&reason, &attemptedAt); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if reason != "no comps now" {
		t.Errorf("upsert did not refresh skip_reason: got %q", reason)
	}
	if attemptedAt != t1.Format(time.RFC3339) {
		t.Errorf("upsert did not refresh attempted_at: got %q want %q", attemptedAt, t1.Format(time.RFC3339))
	}
}

// ---- IterateDraft ----

// pickFromSequence returns a PickerFunc that yields ids from `seq` in
// order, skipping any id already present in the skip map (mirrors the
// real picker's "first eligible" semantics). When the sequence is
// exhausted, it returns the supplied exhaustErr.
func pickFromSequence(seq []int, exhaustErr error) PickerFunc {
	return func(skip map[int]struct{}) (int, error) {
		for _, id := range seq {
			if _, blocked := skip[id]; !blocked {
				return id, nil
			}
		}
		return 0, exhaustErr
	}
}

func TestIterateDraft_SuccessFirstAttempt(t *testing.T) {
	skip := map[int]struct{}{}
	draftCalls := 0
	drafted, err := IterateDraft(
		3, "product", skip,
		pickFromSequence([]int{42, 43, 44}, errors.New("exhausted")),
		func(id int) (Drafted, error) {
			draftCalls++
			return Drafted{Title: fmt.Sprintf("ok %d", id)}, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if drafted.Skipped {
		t.Errorf("expected non-skipped drafted, got Skipped with reason=%q", drafted.SkipReason)
	}
	if drafted.Title != "ok 42" {
		t.Errorf("expected first id used, got Title=%q", drafted.Title)
	}
	if draftCalls != 1 {
		t.Errorf("expected exactly 1 draft attempt, got %d", draftCalls)
	}
	if len(skip) != 0 {
		t.Errorf("expected skip set untouched on first-attempt success, got %v", skip)
	}
}

func TestIterateDraft_SkipThenSucceed(t *testing.T) {
	skip := map[int]struct{}{}
	draftCalls := 0
	drafted, err := IterateDraft(
		3, "product", skip,
		pickFromSequence([]int{42, 43, 44}, errors.New("exhausted")),
		func(id int) (Drafted, error) {
			draftCalls++
			// First two return skipped, third succeeds.
			if id == 42 || id == 43 {
				return Drafted{Skipped: true, SkipReason: fmt.Sprintf("no_proposal for %d", id)}, nil
			}
			return Drafted{Title: fmt.Sprintf("ok %d", id)}, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if drafted.Skipped {
		t.Errorf("expected non-skipped drafted after iteration, got skipped reason=%q", drafted.SkipReason)
	}
	if drafted.Title != "ok 44" {
		t.Errorf("expected id 44 used (after skipping 42, 43), got Title=%q", drafted.Title)
	}
	if draftCalls != 3 {
		t.Errorf("expected 3 draft attempts, got %d", draftCalls)
	}
	if _, ok := skip[42]; !ok {
		t.Errorf("expected id 42 added to skip after its skip, got %v", skip)
	}
	if _, ok := skip[43]; !ok {
		t.Errorf("expected id 43 added to skip after its skip, got %v", skip)
	}
	if _, ok := skip[44]; ok {
		t.Errorf("did not expect successful id 44 in skip set, got %v", skip)
	}
}

func TestIterateDraft_AllAttemptsSkipped(t *testing.T) {
	skip := map[int]struct{}{}
	draftCalls := 0
	drafted, err := IterateDraft(
		3, "product", skip,
		pickFromSequence([]int{1, 2, 3, 4, 5}, errors.New("exhausted")),
		func(id int) (Drafted, error) {
			draftCalls++
			return Drafted{Skipped: true, SkipReason: fmt.Sprintf("no_proposal for %d", id)}, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !drafted.Skipped {
		t.Errorf("expected Skipped after exhausting maxAttempts, got non-skipped Title=%q", drafted.Title)
	}
	if draftCalls != 3 {
		t.Errorf("expected exactly maxAttempts (3) draft calls, got %d", draftCalls)
	}
	// SkipReason should mention each tried id and the persona target name.
	for _, want := range []string{"tried 3 products", "product 1", "product 2", "product 3", "no_proposal for 1"} {
		if !strings.Contains(drafted.SkipReason, want) {
			t.Errorf("SkipReason missing %q; got %q", want, drafted.SkipReason)
		}
	}
}

func TestIterateDraft_PickerExhaustsAfterSomeSkips(t *testing.T) {
	// Only 2 ids available; both skip; third pick returns exhaustErr.
	skip := map[int]struct{}{}
	exhaustErr := errors.New("no more eligible products in window")
	drafted, err := IterateDraft(
		5, "product", skip,
		pickFromSequence([]int{10, 20}, exhaustErr),
		func(id int) (Drafted, error) {
			return Drafted{Skipped: true, SkipReason: "no_proposal"}, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !drafted.Skipped {
		t.Errorf("expected Skipped, got non-skipped Title=%q", drafted.Title)
	}
	// Reason should report what was tried before the picker ran out.
	for _, want := range []string{"tried 2 products", "no more eligible products"} {
		if !strings.Contains(drafted.SkipReason, want) {
			t.Errorf("SkipReason missing %q; got %q", want, drafted.SkipReason)
		}
	}
}

func TestIterateDraft_PickerEmptyOnFirstCall(t *testing.T) {
	// Cooldown already excludes everything — picker errs immediately.
	skip := map[int]struct{}{}
	drafted, err := IterateDraft(
		3, "product", skip,
		pickFromSequence(nil, errors.New("every published product is in cooldown")),
		func(id int) (Drafted, error) {
			t.Fatalf("draftFn should not be called when picker errs on first attempt; got id=%d", id)
			return Drafted{}, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !drafted.Skipped {
		t.Errorf("expected Skipped on first-attempt picker error, got non-skipped")
	}
	if drafted.SkipReason != "every published product is in cooldown" {
		t.Errorf("expected raw picker error as SkipReason, got %q", drafted.SkipReason)
	}
}

func TestIterateDraft_HardErrorFromDraftFnPropagates(t *testing.T) {
	skip := map[int]struct{}{}
	hardErr := errors.New("mcp unreachable")
	_, err := IterateDraft(
		3, "product", skip,
		pickFromSequence([]int{1}, errors.New("exhausted")),
		func(id int) (Drafted, error) { return Drafted{}, hardErr },
	)
	if !errors.Is(err, hardErr) {
		t.Errorf("expected hardErr propagated, got %v", err)
	}
}

func TestRunAndPersist_EmitsBatchWhenSiblingsSet(t *testing.T) {
	st := newStore(t)
	seedAgent(t, st, "pricer-batch", 1)
	sibling1 := Drafted{
		Title:           "Price change · SKU-002 · $52.00 → $48.00 (-7.7%)",
		Priority:        "medium",
		ProposalType:    "product_price_change",
		ProposalContent: "Comparable retailers list $46-$50.",
		Target: map[string]any{
			"product_id":     202,
			"product_sku":    "SKU-002",
			"previous_price": 52.00,
			"proposed_price": 48.00,
			"percent_change": -7.7,
			"direction":      "decrease",
			"currency":       "USD",
		},
	}
	sibling2 := Drafted{
		Title:           "Price change · SKU-003 · $39.00 → $36.00 (-7.7%)",
		Priority:        "medium",
		ProposalType:    "product_price_change",
		ProposalContent: "Comparable retailers list $35-$38.",
		Target: map[string]any{
			"product_id":     203,
			"product_sku":    "SKU-003",
			"previous_price": 39.00,
			"proposed_price": 36.00,
			"percent_change": -7.7,
			"direction":      "decrease",
			"currency":       "USD",
		},
	}
	p := fakePersona{
		slug: "pricer-batch",
		drafted: Drafted{
			Title:           "Price change · SKU-001 · $42.00 → $38.00 (-9.5%)",
			Priority:        "medium",
			ProposalType:    "product_price_change",
			ProposalContent: "Comparable retailers list $36-$40.",
			Target: map[string]any{
				"product_id":     201,
				"product_sku":    "SKU-001",
				"previous_price": 42.00,
				"proposed_price": 38.00,
				"percent_change": -9.5,
				"direction":      "decrease",
				"currency":       "USD",
			},
			BatchSiblings: []Drafted{sibling1, sibling2},
			BatchTitle:    "Pricing · Home & Textiles seasonal parity run (3 products)",
			BatchIntent:   "pricing_bulk",
		},
	}
	res, err := RunAndPersist(context.Background(), p, Deps{Store: st})
	if err != nil {
		t.Fatalf("RunAndPersist returned error: %v", err)
	}
	if res.BatchID == "" {
		t.Fatalf("expected non-empty BatchID on Result; got empty string")
	}

	// Verify the batch row exists.
	var batchTitle string
	if err := st.DB.QueryRow(`SELECT title FROM batches WHERE id = ?`, res.BatchID).Scan(&batchTitle); err != nil {
		t.Fatalf("batches row lookup failed: %v", err)
	}
	if batchTitle != "Pricing · Home & Textiles seasonal parity run (3 products)" {
		t.Errorf("batch title=%q, want the spec'd title", batchTitle)
	}

	// Verify 3 issues all carry the batch_id.
	var count int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM issues WHERE batch_id = ?`, res.BatchID).Scan(&count); err != nil {
		t.Fatalf("issues count query failed: %v", err)
	}
	if count != 3 {
		t.Errorf("issues with batch_id %s: got %d, want 3", res.BatchID, count)
	}
}

// queuePersona returns a queued sequence of Drafted/error pairs across
// successive Draft calls. Used by the multi-emit tests below to simulate
// a persona that picks a different target on each iteration (in reality
// driven by RecentlyTouchedTargets, but the fake doesn't need to consult
// it — RunAndPersist itself is what we're testing).
type queuePersona struct {
	slug     string
	drafted  []Drafted
	errs     []error
	calls    int
}

func (q *queuePersona) Slug() string        { return q.slug }
func (q *queuePersona) DisplayName() string { return "queue " + q.slug }
func (q *queuePersona) Addable() bool       { return false }
func (q *queuePersona) Cooldown() CooldownPolicy {
	return CooldownPolicy{
		TargetKey: "product_id",
		Approved:  7 * 24 * time.Hour,
		Dismissed: 30 * 24 * time.Hour,
	}
}
func (q *queuePersona) Draft(_ context.Context, _ Deps) (Drafted, error) {
	i := q.calls
	q.calls++
	var d Drafted
	if i < len(q.drafted) {
		d = q.drafted[i]
	}
	var err error
	if i < len(q.errs) {
		err = q.errs[i]
	}
	return d, err
}

func TestRunAndPersist_MultiEmit_InsertsN(t *testing.T) {
	st := newStore(t)
	seedAgent(t, st, "fake-multi", 1)
	p := &queuePersona{
		slug: "fake-multi",
		drafted: []Drafted{
			{
				Title: "first proposal", Priority: "medium",
				ProposalType: "x", ProposalContent: "a",
				Target: map[string]any{"product_id": 1},
			},
			{
				Title: "second proposal", Priority: "medium",
				ProposalType: "x", ProposalContent: "b",
				Target: map[string]any{"product_id": 2},
			},
		},
	}
	res, err := RunAndPersist(context.Background(), p, Deps{Store: st, MaxEmits: 2})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Skipped {
		t.Errorf("expected not skipped; got %+v", res)
	}
	if got := len(res.IssueIDs); got != 2 {
		t.Errorf("len(IssueIDs) = %d, want 2", got)
	}
	if res.IssueID == "" || res.IssueID != res.IssueIDs[0] {
		t.Errorf("IssueID (%q) should match IssueIDs[0] (%v)", res.IssueID, res.IssueIDs)
	}
	if p.calls != 2 {
		t.Errorf("Draft call count = %d, want 2", p.calls)
	}
	var n int
	if err := st.DB.QueryRowContext(context.Background(),
		`SELECT count(*) FROM issues WHERE persona = ?`, "fake-multi",
	).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 issues inserted, got %d", n)
	}
}

func TestRunAndPersist_MultiEmit_BestEffortOnSecondError(t *testing.T) {
	st := newStore(t)
	seedAgent(t, st, "fake-multi-err", 1)
	p := &queuePersona{
		slug: "fake-multi-err",
		drafted: []Drafted{
			{
				Title: "first", Priority: "medium",
				ProposalType: "x", ProposalContent: "a",
				Target: map[string]any{"product_id": 1},
			},
			{}, // second drafted ignored — err is non-nil
		},
		errs: []error{nil, errors.New("LLM timeout")},
	}
	res, err := RunAndPersist(context.Background(), p, Deps{Store: st, MaxEmits: 2})
	if err != nil {
		t.Fatalf("expected nil err (best-effort), got: %v", err)
	}
	if res.Skipped {
		t.Errorf("expected not Skipped (first emit succeeded); got %+v", res)
	}
	if len(res.IssueIDs) != 1 {
		t.Errorf("len(IssueIDs) = %d, want 1", len(res.IssueIDs))
	}
	if !strings.Contains(res.SkipReason, "LLM timeout") {
		t.Errorf("SkipReason should mention the second-emit failure; got %q", res.SkipReason)
	}
}

func TestRunAndPersist_MultiEmit_BestEffortOnSecondSkip(t *testing.T) {
	st := newStore(t)
	seedAgent(t, st, "fake-multi-skip", 1)
	p := &queuePersona{
		slug: "fake-multi-skip",
		drafted: []Drafted{
			{
				Title: "first", Priority: "medium",
				ProposalType: "x", ProposalContent: "a",
				Target: map[string]any{"product_id": 1},
			},
			{Skipped: true, SkipReason: "no more eligible targets"},
		},
	}
	res, err := RunAndPersist(context.Background(), p, Deps{Store: st, MaxEmits: 2})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Skipped {
		t.Errorf("Result.Skipped should be false (first emit succeeded); got %+v", res)
	}
	if len(res.IssueIDs) != 1 {
		t.Errorf("len(IssueIDs) = %d, want 1", len(res.IssueIDs))
	}
	if !strings.Contains(res.SkipReason, "no more eligible targets") {
		t.Errorf("SkipReason should mention the second-emit skip; got %q", res.SkipReason)
	}
}

func TestRunAndPersist_MaxEmitsZeroBehavesAsOne(t *testing.T) {
	st := newStore(t)
	seedAgent(t, st, "fake-default", 1)
	p := &queuePersona{
		slug: "fake-default",
		drafted: []Drafted{
			{
				Title: "single", Priority: "medium",
				ProposalType: "x", ProposalContent: "a",
				Target: map[string]any{"product_id": 1},
			},
			{
				Title: "should not be emitted", Priority: "medium",
				ProposalType: "x", ProposalContent: "b",
			},
		},
	}
	// MaxEmits left at 0 — should be treated as 1.
	res, err := RunAndPersist(context.Background(), p, Deps{Store: st})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if p.calls != 1 {
		t.Errorf("Draft call count = %d, want 1 (default MaxEmits)", p.calls)
	}
	if len(res.IssueIDs) != 1 {
		t.Errorf("len(IssueIDs) = %d, want 1", len(res.IssueIDs))
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
