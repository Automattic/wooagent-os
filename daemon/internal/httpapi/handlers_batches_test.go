package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/pep"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

// ---------- fixture ----------

type fakeMCP struct {
	callErr error
	result  mcp.ToolCallResult
	// failOnce sequences responses: when non-nil, the first CallTool returns
	// failOnceErr, subsequent calls return result. Used for the
	// approve-all-mixed-failures test.
	failOnceErr error
	failed      bool
	calls       int
}

func (f *fakeMCP) Initialize(_ context.Context) (mcp.ServerInfo, error) {
	return mcp.ServerInfo{}, nil
}

func (f *fakeMCP) CallTool(_ context.Context, _ string, _ any) (mcp.ToolCallResult, error) {
	f.calls++
	if f.failOnceErr != nil && !f.failed {
		f.failed = true
		return mcp.ToolCallResult{}, f.failOnceErr
	}
	if f.callErr != nil {
		return mcp.ToolCallResult{}, f.callErr
	}
	return f.result, nil
}

func mcpSuccessResult() mcp.ToolCallResult {
	return mcp.ToolCallResult{Content: []mcp.ContentPart{{Type: "text", Text: `{"success":true}`}}}
}

// newTestRig wires a Server backed by an in-memory SQLite (with all
// embedded migrations applied) plus a PEP whose manifest grants the
// marketing persona access to the rewrite ability. Returns the server, a
// chi-based test http server, and the store. The test server bypasses
// bearer auth — these tests exercise the handler logic, not auth.
func newTestRig(t *testing.T, mcpc pep.MCPClient) (*Server, *httptest.Server, *store.Store) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	// Seed the marketing agent row so children with persona=marketing
	// satisfy the issues.persona FK constraint.
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := st.DB.ExecContext(ctx,
		`INSERT INTO agents(persona, name, enabled, created_at, updated_at) VALUES('marketing', 'Marketing Agent', 1, ?, ?)`,
		now, now,
	); err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	m := &manifest.Manifest{
		Version: 1,
		Entries: []manifest.Entry{{
			Ability:        "wooagent-products/update",
			NamespaceOwner: "test",
			SchemaHash:     "sha256:test",
			Scope:          manifest.ScopePropose,
			Reversibility:  0.6,
			Personas:       []manifest.Persona{manifest.PersonaMarketing},
		}},
	}
	lookup, err := manifest.NewLookup(m)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	p := pep.New(lookup, mcpc, st.DB)
	s := &Server{store: st, pep: p}

	r := chi.NewRouter()
	r.Get("/v1/issues", s.handleListIssues)
	r.Post("/v1/issues", s.handleCreateIssue)
	r.Get("/v1/issues/{id}", s.handleGetIssue)
	r.Post("/v1/issues/{id}/approve", s.handleApproveIssue)
	r.Post("/v1/issues/{id}/reject", s.handleRejectIssue)
	r.Get("/v1/batches", s.handleListBatches)
	r.Post("/v1/batches", s.handleCreateBatch)
	r.Get("/v1/batches/{id}", s.handleGetBatch)
	r.Post("/v1/batches/{id}/approve-all", s.handleApproveBatch)
	r.Post("/v1/batches/{id}/reject-all", s.handleRejectBatch)

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return s, ts, st
}

// productRewriteBody returns a child issue payload suitable for the
// "product_description_rewrite" approve dispatcher. ProductID drives the
// FK-free target.product_id field that the dispatcher reads.
func productRewriteBody(title string, productID int, body string) map[string]any {
	return map[string]any{
		"title": title,
		"proposal": map[string]any{
			"type":    "product_description_rewrite",
			"content": body,
			"target": map[string]any{
				"product_id": productID,
				"variants": []map[string]any{
					{"id": "A", "label": "Warm", "body": body, "seo": 88, "voice": 96, "charCount": len(body), "recommended": true},
					{"id": "B", "label": "Spec", "body": body + " (spec)", "seo": 93, "voice": 84, "charCount": len(body) + 7},
				},
			},
		},
	}
}

func httpPostJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http: %v", err)
	}
	return res
}

func decode[T any](t *testing.T, res *http.Response) T {
	t.Helper()
	defer res.Body.Close()
	var out T
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

// ---------- tests ----------

// CreateBatch round-trip: POST /v1/batches with two children, GET /v1/batches/:id
// returns derived counts that match the inserted children's statuses.
func TestCreateBatch_RoundTrip(t *testing.T) {
	_, ts, _ := newTestRig(t, &fakeMCP{result: mcpSuccessResult()})

	res := httpPostJSON(t, ts.URL+"/v1/batches", map[string]any{
		"title":   "Test batch",
		"persona": "marketing",
		"intent":  "meta_rewrite",
		"issues": []map[string]any{
			productRewriteBody("Child 1", 1001, "Body 1"),
			productRewriteBody("Child 2", 1002, "Body 2"),
		},
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d", res.StatusCode)
	}
	created := decode[struct {
		Batch  Batch               `json:"batch"`
		Issues []IssueWithProposal `json:"issues"`
	}](t, res)
	if created.Batch.Total != 2 || created.Batch.Pending != 2 {
		t.Errorf("counts wrong: total=%d pending=%d", created.Batch.Total, created.Batch.Pending)
	}
	if len(created.Issues) != 2 {
		t.Errorf("issues=%d, want 2", len(created.Issues))
	}
	for _, c := range created.Issues {
		if c.Issue.BatchID != created.Batch.ID {
			t.Errorf("child batch_id=%q, want %q", c.Issue.BatchID, created.Batch.ID)
		}
		if c.Issue.Status != "in_review" {
			t.Errorf("child status=%q, want in_review", c.Issue.Status)
		}
	}

	// GET /v1/batches/:id should return the same shape with derived counts.
	getRes, err := http.Get(ts.URL + "/v1/batches/" + created.Batch.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if getRes.StatusCode != http.StatusOK {
		t.Fatalf("get status=%d", getRes.StatusCode)
	}
	got := decode[struct {
		Batch  Batch               `json:"batch"`
		Issues []IssueWithProposal `json:"issues"`
	}](t, getRes)
	if got.Batch.Total != 2 || got.Batch.Pending != 2 || got.Batch.Approved != 0 || got.Batch.Rejected != 0 {
		t.Errorf("derived counts wrong: %+v", got.Batch)
	}
	if len(got.Issues) != 2 {
		t.Errorf("get issues=%d, want 2", len(got.Issues))
	}
}

// Empty batches still appear in the list with zero counts.
func TestListBatches_EmptyAndNonEmpty(t *testing.T) {
	s, ts, st := newTestRig(t, &fakeMCP{result: mcpSuccessResult()})
	_ = s

	// Insert an empty batch directly so we don't need a no-children create
	// endpoint for the test (handleCreateBatch requires len(issues) > 0).
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := st.DB.Exec(
		`INSERT INTO batches(id, title, persona, intent, source_run_id, created_at, updated_at) VALUES('empty-1', 'Empty', 'marketing', '', '', ?, ?)`,
		now, now,
	); err != nil {
		t.Fatalf("seed empty batch: %v", err)
	}

	// Plus one populated batch.
	httpPostJSON(t, ts.URL+"/v1/batches", map[string]any{
		"title": "Populated", "persona": "marketing",
		"issues": []map[string]any{productRewriteBody("Child A", 1, "x")},
	}).Body.Close()

	listRes, err := http.Get(ts.URL + "/v1/batches")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	listed := decode[struct {
		Batches []Batch `json:"batches"`
	}](t, listRes)
	if len(listed.Batches) != 2 {
		t.Fatalf("got %d batches, want 2", len(listed.Batches))
	}
	for _, b := range listed.Batches {
		if b.ID == "empty-1" {
			if b.Total != 0 || b.Pending != 0 {
				t.Errorf("empty batch counts wrong: %+v", b)
			}
		}
	}
}

// approve-all best-effort: when one child's MCP call fails, other children
// still commit. Per-child results report ok flags. Always 200.
func TestApproveAll_MixedFailures(t *testing.T) {
	mcpc := &fakeMCP{
		result:      mcpSuccessResult(),
		failOnceErr: errors.New("simulated MCP outage"),
	}
	_, ts, _ := newTestRig(t, mcpc)

	created := decode[struct {
		Batch  Batch               `json:"batch"`
		Issues []IssueWithProposal `json:"issues"`
	}](t, httpPostJSON(t, ts.URL+"/v1/batches", map[string]any{
		"title": "Mixed", "persona": "marketing",
		"issues": []map[string]any{
			productRewriteBody("c1", 1, "b1"),
			productRewriteBody("c2", 2, "b2"),
			productRewriteBody("c3", 3, "b3"),
		},
	}))
	if len(created.Issues) != 3 {
		t.Fatalf("seed: got %d children", len(created.Issues))
	}

	// Approve all three; the fakeMCP will fail the first call only.
	children := []map[string]any{}
	for _, c := range created.Issues {
		children = append(children, map[string]any{"issue_id": c.Issue.ID, "variant_id": "A"})
	}
	res := httpPostJSON(t, ts.URL+"/v1/batches/"+created.Batch.ID+"/approve-all", map[string]any{
		"children": children,
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", res.StatusCode)
	}
	out := decode[struct {
		Results []map[string]any `json:"results"`
	}](t, res)
	if len(out.Results) != 3 {
		t.Fatalf("results=%d", len(out.Results))
	}
	okCount := 0
	for _, r := range out.Results {
		if r["ok"] == true {
			okCount++
		}
	}
	if okCount != 2 {
		t.Errorf("ok count=%d, want 2 (one mid-batch failure)", okCount)
	}

	// Confirm the failure didn't permanently leave its child in in_progress —
	// the rollbackClaim path should have flipped it back to in_review.
	getRes, _ := http.Get(ts.URL + "/v1/batches/" + created.Batch.ID)
	got := decode[struct {
		Batch Batch `json:"batch"`
	}](t, getRes)
	if got.Batch.Approved != 2 {
		t.Errorf("approved count=%d, want 2", got.Batch.Approved)
	}
	if got.Batch.Pending != 1 {
		t.Errorf("pending count=%d, want 1 (failed child rolled back)", got.Batch.Pending)
	}
}

// reject-all: a child that's already in done state comes back with
// code=wrong_status while the in_review siblings get flipped to rejected.
func TestRejectAll_WrongStatusOnDone(t *testing.T) {
	_, ts, st := newTestRig(t, &fakeMCP{result: mcpSuccessResult()})

	created := decode[struct {
		Batch  Batch               `json:"batch"`
		Issues []IssueWithProposal `json:"issues"`
	}](t, httpPostJSON(t, ts.URL+"/v1/batches", map[string]any{
		"title": "Reject mix", "persona": "marketing",
		"issues": []map[string]any{
			productRewriteBody("c1", 1, "b1"),
			productRewriteBody("c2", 2, "b2"),
		},
	}))

	// Manually flip the first child to done so the reject-all loop should
	// surface code=wrong_status for it.
	if _, err := st.DB.Exec(
		`UPDATE issues SET status = 'done' WHERE id = ?`, created.Issues[0].Issue.ID,
	); err != nil {
		t.Fatalf("force done: %v", err)
	}

	res := httpPostJSON(t, ts.URL+"/v1/batches/"+created.Batch.ID+"/reject-all", map[string]any{})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", res.StatusCode)
	}
	out := decode[struct {
		Results []map[string]any `json:"results"`
	}](t, res)
	if len(out.Results) != 2 {
		t.Fatalf("results=%d", len(out.Results))
	}
	wrongStatusFound := false
	rejectedFound := false
	for _, r := range out.Results {
		if r["ok"] == false {
			if errMap, ok := r["error"].(map[string]any); ok {
				if errMap["code"] == "wrong_status" {
					wrongStatusFound = true
				}
			}
		}
		if r["ok"] == true && r["status"] == "rejected" {
			rejectedFound = true
		}
	}
	if !wrongStatusFound {
		t.Errorf("expected wrong_status result for the done child")
	}
	if !rejectedFound {
		t.Errorf("expected one in_review child to flip to rejected")
	}
}

// Single-issue approve still works — regression check after the
// approveOne refactor.
func TestApproveIssue_SingleStillWorks(t *testing.T) {
	_, ts, _ := newTestRig(t, &fakeMCP{result: mcpSuccessResult()})

	// Create an in_review single issue (no batch).
	issueRes := httpPostJSON(t, ts.URL+"/v1/issues", map[string]any{
		"title":    "Single",
		"persona":  "marketing",
		"status":   "in_review",
		"priority": "medium",
		"proposal": map[string]any{
			"type":    "product_description_rewrite",
			"content": "body",
			"target":  map[string]any{"product_id": 9001},
		},
	})
	if issueRes.StatusCode != http.StatusCreated {
		t.Fatalf("create issue status=%d", issueRes.StatusCode)
	}
	created := decode[struct {
		Issue Issue `json:"issue"`
	}](t, issueRes)

	approveRes := httpPostJSON(t, ts.URL+"/v1/issues/"+created.Issue.ID+"/approve", map[string]any{})
	if approveRes.StatusCode != http.StatusOK {
		body, _ := decode[map[string]any](t, approveRes), error(nil)
		_ = body
		t.Fatalf("approve status=%d", approveRes.StatusCode)
	}
	out := decode[map[string]any](t, approveRes)
	if out["status"] != "done" {
		t.Errorf("status=%v, want done", out["status"])
	}
}

// approveOne race-loss: when the issue is no longer in_review, approveOne
// returns wrong_status without touching PEP/MCP. Simulated by pre-flipping
// the issue to done before calling approveOne.
func TestApproveOne_RaceLoss(t *testing.T) {
	mcpc := &fakeMCP{result: mcpSuccessResult()}
	s, ts, st := newTestRig(t, mcpc)
	_ = ts

	// Create an in_review issue, then race-flip its status to done before
	// calling approveOne.
	issueRes := httpPostJSON(t, ts.URL+"/v1/issues", map[string]any{
		"title": "Racey", "persona": "marketing", "status": "in_review", "priority": "medium",
		"proposal": map[string]any{
			"type":    "product_description_rewrite",
			"content": "x",
			"target":  map[string]any{"product_id": 7},
		},
	})
	created := decode[struct {
		Issue Issue `json:"issue"`
	}](t, issueRes)

	if _, err := st.DB.Exec(`UPDATE issues SET status = 'done' WHERE id = ?`, created.Issue.ID); err != nil {
		t.Fatalf("preflip: %v", err)
	}

	_, perr := s.approveOne(context.Background(), created.Issue.ID, "")
	if perr == nil {
		t.Fatalf("expected race-loss error, got nil")
	}
	if perr.Code != "wrong_status" {
		t.Errorf("code=%q, want wrong_status", perr.Code)
	}
	if mcpc.calls != 0 {
		t.Errorf("MCP was called %d times; race-loss must short-circuit before MCP", mcpc.calls)
	}
}

// Filter ?batch_id= on /v1/issues returns only the batch's children.
func TestListIssues_FilterByBatchID(t *testing.T) {
	_, ts, _ := newTestRig(t, &fakeMCP{result: mcpSuccessResult()})

	// Two batches, one orphan issue.
	created := decode[struct {
		Batch  Batch               `json:"batch"`
		Issues []IssueWithProposal `json:"issues"`
	}](t, httpPostJSON(t, ts.URL+"/v1/batches", map[string]any{
		"title": "B1", "persona": "marketing",
		"issues": []map[string]any{
			productRewriteBody("c1", 1, "x"),
			productRewriteBody("c2", 2, "y"),
		},
	}))
	httpPostJSON(t, ts.URL+"/v1/issues", map[string]any{
		"title": "orphan", "persona": "marketing", "priority": "medium",
		"proposal": map[string]any{"type": "product_description_rewrite", "content": "z", "target": map[string]any{"product_id": 99}},
	}).Body.Close()

	listRes, err := http.Get(ts.URL + "/v1/issues?batch_id=" + created.Batch.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := decode[struct {
		Issues []Issue `json:"issues"`
	}](t, listRes)
	if len(got.Issues) != 2 {
		t.Errorf("got %d filtered issues, want 2", len(got.Issues))
	}
	for _, i := range got.Issues {
		if i.BatchID != created.Batch.ID {
			t.Errorf("filtered issue has wrong batch_id: %q vs %q", i.BatchID, created.Batch.ID)
		}
	}
}

// Sanity: chi route placeholders work for /:id paths in the test rig.
func TestGetBatch_NotFound(t *testing.T) {
	_, ts, _ := newTestRig(t, &fakeMCP{result: mcpSuccessResult()})
	res, err := http.Get(ts.URL + "/v1/batches/nonexistent")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", res.StatusCode)
	}
}

// approveBatch confused-deputy: approving a child that lives in a different
// batch (or no batch at all) returns code=not_in_batch.
func TestApproveBatch_NotInBatch(t *testing.T) {
	_, ts, _ := newTestRig(t, &fakeMCP{result: mcpSuccessResult()})

	created := decode[struct {
		Batch Batch `json:"batch"`
	}](t, httpPostJSON(t, ts.URL+"/v1/batches", map[string]any{
		"title": "B1", "persona": "marketing",
		"issues": []map[string]any{productRewriteBody("c1", 1, "x")},
	}))

	// An orphan issue created via /v1/issues — not part of created.Batch.
	orphanRes := httpPostJSON(t, ts.URL+"/v1/issues", map[string]any{
		"title": "orphan", "persona": "marketing", "status": "in_review", "priority": "medium",
		"proposal": map[string]any{"type": "product_description_rewrite", "content": "z", "target": map[string]any{"product_id": 99}},
	})
	orphan := decode[struct {
		Issue Issue `json:"issue"`
	}](t, orphanRes)

	res := httpPostJSON(t, ts.URL+"/v1/batches/"+created.Batch.ID+"/approve-all", map[string]any{
		"children": []map[string]any{
			{"issue_id": orphan.Issue.ID, "variant_id": "A"},
		},
	})
	out := decode[struct {
		Results []map[string]any `json:"results"`
	}](t, res)
	if len(out.Results) != 1 {
		t.Fatalf("results=%d", len(out.Results))
	}
	if out.Results[0]["ok"] != false {
		t.Errorf("expected ok=false")
	}
	if errMap, ok := out.Results[0]["error"].(map[string]any); ok {
		if errMap["code"] != "not_in_batch" {
			t.Errorf("code=%v, want not_in_batch", errMap["code"])
		}
	} else {
		t.Errorf("expected error map in result, got %+v", out.Results[0])
	}
}

func TestServer_BatchRoutesRegistered(t *testing.T) {
	// Sanity: confirm the production buildRouter wires the new routes (not
	// just our test rig). Without auth.Manager New panics, so we construct
	// the Server directly and call buildRouter manually.
	st, _ := store.Open(context.Background(), ":memory:")
	t.Cleanup(func() { _ = st.Close() })
	s := &Server{store: st}
	r := s.buildRouter()
	for _, route := range []struct {
		method string
		path   string
	}{
		{"POST", "/v1/batches"},
		{"GET", "/v1/batches"},
		{"GET", "/v1/batches/{id}"},
		{"POST", "/v1/batches/{id}/approve-all"},
		{"POST", "/v1/batches/{id}/reject-all"},
	} {
		req := httptest.NewRequest(route.method, strings.Replace(route.path, "{id}", "x", 1), nil)
		rctx := chi.NewRouteContext()
		if !r.Match(rctx, req.Method, req.URL.Path) {
			t.Errorf("route not registered: %s %s", route.method, route.path)
		}
	}
}
