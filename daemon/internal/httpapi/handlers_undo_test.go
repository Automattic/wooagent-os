package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
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

// undoMCP records every CallTool invocation and lets the test script the
// product 'get' response (for the staleness check) and the 'update' response.
type undoMCP struct {
	t              *testing.T
	calls          []recordedMCPCall
	getResponse    string // JSON for the next wooagent-products/get
	updateResponse string // JSON for the next wooagent-products/update
	// respondByAbility lets a test script a JSON body per ability name.
	// Takes precedence over getResponse/updateResponse when the key matches.
	respondByAbility map[string]string
}

type recordedMCPCall struct {
	tool   string
	params map[string]any
}

func (m *undoMCP) Initialize(_ context.Context) (mcp.ServerInfo, error) {
	return mcp.ServerInfo{}, nil
}

func (m *undoMCP) CallTool(_ context.Context, tool string, params any) (mcp.ToolCallResult, error) {
	p, _ := params.(map[string]any)
	m.calls = append(m.calls, recordedMCPCall{tool: tool, params: p})
	ability, _ := p["ability_name"].(string)
	if m.respondByAbility != nil {
		if body, ok := m.respondByAbility[ability]; ok && body != "" {
			return mcp.ToolCallResult{Content: []mcp.ContentPart{{Type: "text", Text: body}}}, nil
		}
	}
	switch ability {
	case "wooagent-products/get":
		body := m.getResponse
		if body == "" {
			body = `{"success":true,"data":{}}`
		}
		return mcp.ToolCallResult{Content: []mcp.ContentPart{{Type: "text", Text: body}}}, nil
	default:
		body := m.updateResponse
		if body == "" {
			body = `{"success":true}`
		}
		return mcp.ToolCallResult{Content: []mcp.ContentPart{{Type: "text", Text: body}}}, nil
	}
}

func newUndoRig(t *testing.T) (*httptest.Server, *store.Store, *undoMCP) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	m := &manifest.Manifest{
		Version: 1,
		Entries: []manifest.Entry{
			{
				Ability:        "wooagent-products/update",
				NamespaceOwner: "test",
				SchemaHash:     "sha256:test",
				Scope:          manifest.ScopePropose,
				Reversibility:  0.6,
				Personas:       []manifest.Persona{manifest.PersonaPricing, manifest.PersonaMarketing},
			},
			{
				Ability:        "wooagent-products/get",
				NamespaceOwner: "test",
				SchemaHash:     "sha256:test",
				Scope:          manifest.ScopePropose,
				Reversibility:  1.0,
				Personas:       []manifest.Persona{manifest.PersonaPricing, manifest.PersonaMarketing},
			},
			{
				Ability:        "wooagent-products/variations-list",
				NamespaceOwner: "test",
				SchemaHash:     "sha256:test",
				Scope:          manifest.ScopeRead,
				Reversibility:  1.0,
				Personas:       []manifest.Persona{manifest.PersonaPricing, manifest.PersonaMarketing},
			},
			{
				Ability:        "wooagent-products/update-variations-bulk",
				NamespaceOwner: "test",
				SchemaHash:     "sha256:test",
				Scope:          manifest.ScopePropose,
				Reversibility:  0.6,
				Personas:       []manifest.Persona{manifest.PersonaPricing, manifest.PersonaMarketing},
			},
		},
	}
	lookup, err := manifest.NewLookup(m)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	rec := &undoMCP{t: t}
	p := pep.New(lookup, rec, st.DB, nil)
	s := &Server{store: st, pep: p}

	r := chi.NewRouter()
	r.Post("/v1/issues", s.handleCreateIssue)
	r.Post("/v1/issues/{id}/approve", s.handleApproveIssue)
	r.Post("/v1/issues/{id}/undo", s.handleUndoIssue)
	r.Get("/v1/issues/{id}", s.handleGetIssue)

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := st.DB.ExecContext(ctx,
		`INSERT INTO agents(persona, name, enabled, created_at, updated_at) VALUES('pricing','Pricing',1,?,?)`,
		now, now,
	); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	return ts, st, rec
}

func approveAndGetID(t *testing.T, ts *httptest.Server, body map[string]any) string {
	t.Helper()
	res := httpPostJSON(t, ts.URL+"/v1/issues", body)
	if res.StatusCode != http.StatusCreated {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(res.Body)
		res.Body.Close()
		t.Fatalf("create: status=%d body=%s", res.StatusCode, buf.String())
	}
	var created struct {
		Issue struct {
			ID string `json:"id"`
		} `json:"issue"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	res.Body.Close()
	approveRes := httpPostJSON(t, ts.URL+"/v1/issues/"+created.Issue.ID+"/approve", map[string]any{})
	if approveRes.StatusCode != http.StatusOK {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(approveRes.Body)
		approveRes.Body.Close()
		t.Fatalf("approve: status=%d body=%s", approveRes.StatusCode, buf.String())
	}
	approveRes.Body.Close()
	return created.Issue.ID
}

func TestUndoIssue_PriceChange_HappyPath(t *testing.T) {
	ts, st, mock := newUndoRig(t)

	id := approveAndGetID(t, ts, map[string]any{
		"title":   "p",
		"persona": "pricing",
		"status":  "in_review",
		"proposal": map[string]any{
			"type":    "product_price_change",
			"content": "rationale",
			"target": map[string]any{
				"product_id":     821,
				"currency":       "USD",
				"previous_price": 39.00,
				"proposed_price": 44.99,
				"regular_price":  "44.99",
				"percent_change": 15.4,
				"direction":      "increase",
				"sources":        []map[string]any{{"url": "u", "comparable_product": "c", "observed_price": 45.0}},
			},
		},
	})

	// Staleness check expects current regular_price == "44.99" (what we wrote).
	mock.getResponse = `{"success":true,"data":{"id":821,"regular_price":"44.99"}}`

	undoRes := httpPostJSON(t, ts.URL+"/v1/issues/"+id+"/undo", map[string]any{})
	if undoRes.StatusCode != http.StatusOK {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(undoRes.Body)
		undoRes.Body.Close()
		t.Fatalf("undo: status=%d body=%s", undoRes.StatusCode, buf.String())
	}
	undoRes.Body.Close()

	// We expect (approve→update) then (undo→get, undo→update).
	if len(mock.calls) < 3 {
		t.Fatalf("expected >=3 MCP calls (approve update + undo get + undo update); got %d", len(mock.calls))
	}
	last := mock.calls[len(mock.calls)-1]
	if last.params["ability_name"] != "wooagent-products/update" {
		t.Errorf("final MCP call should be update; got %q", last.params["ability_name"])
	}
	inner, _ := last.params["parameters"].(map[string]any)
	if inner["regular_price"] != "39.00" {
		t.Errorf("undo MCP regular_price = %v, want \"39.00\" (the previous_price)", inner["regular_price"])
	}

	var undoneAt sql.NullString
	var status string
	if err := st.DB.QueryRow(
		`SELECT status, undone_at FROM issues WHERE id = ?`, id,
	).Scan(&status, &undoneAt); err != nil {
		t.Fatalf("read undo state: %v", err)
	}
	if status != "done" {
		t.Errorf("status = %q, want done (status doesn't change on undo)", status)
	}
	if !undoneAt.Valid {
		t.Errorf("undone_at should be populated")
	}
}

func TestUndoIssue_NotUndoableProposalType(t *testing.T) {
	ts, st, _ := newUndoRig(t)
	// customer_reply_draft isn't in undoableProposalTypes.
	// Seed an issue directly in 'done' state to skip the customer-note
	// approve path (which would require wooagent-orders/add-note in the
	// manifest, not relevant to this test).
	now := time.Now().UTC().Format(time.RFC3339)
	// Seed sales_support agent for this test.
	if _, err := st.DB.ExecContext(context.Background(),
		`INSERT INTO agents(persona, name, enabled, created_at, updated_at) VALUES('sales_support','Sales Support',1,?,?)`,
		now, now,
	); err != nil {
		t.Fatalf("seed sales_support agent: %v", err)
	}
	body := map[string]any{
		"title":   "msg",
		"persona": "sales_support",
		"status":  "in_review",
		"proposal": map[string]any{
			"type":    "customer_reply_draft",
			"content": "hello",
			"target":  map[string]any{"order_id": 99, "note_type": "internal"},
		},
	}
	res := httpPostJSON(t, ts.URL+"/v1/issues", body)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create: status=%d", res.StatusCode)
	}
	var created struct {
		Issue struct {
			ID string `json:"id"`
		} `json:"issue"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	res.Body.Close()

	// Force the row to done without going through approve.
	if _, err := st.DB.ExecContext(context.Background(),
		`UPDATE issues SET status = 'done', updated_at = ? WHERE id = ?`,
		now, created.Issue.ID,
	); err != nil {
		t.Fatalf("force done: %v", err)
	}

	undoRes := httpPostJSON(t, ts.URL+"/v1/issues/"+created.Issue.ID+"/undo", map[string]any{})
	if undoRes.StatusCode != http.StatusUnprocessableEntity {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(undoRes.Body)
		undoRes.Body.Close()
		t.Fatalf("status = %d, want 422; body=%s", undoRes.StatusCode, buf.String())
	}
	var body2 map[string]any
	if err := json.NewDecoder(undoRes.Body).Decode(&body2); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	undoRes.Body.Close()
	errObj, _ := body2["error"].(map[string]any)
	if errObj["code"] != "not_undoable" {
		t.Errorf("code = %v, want not_undoable", errObj["code"])
	}
}

func TestUndoIssue_StaleProductReturns409(t *testing.T) {
	ts, _, mock := newUndoRig(t)

	id := approveAndGetID(t, ts, map[string]any{
		"title":   "p",
		"persona": "pricing",
		"status":  "in_review",
		"proposal": map[string]any{
			"type":    "product_price_change",
			"content": "rationale",
			"target": map[string]any{
				"product_id":     821,
				"currency":       "USD",
				"previous_price": 39.00,
				"proposed_price": 44.99,
				"regular_price":  "44.99",
				"target_field":   "regular_price",
				"percent_change": 15.4,
				"direction":      "increase",
				"sources":        []map[string]any{{"url": "u", "comparable_product": "c", "observed_price": 45.0}},
			},
		},
	})

	// Live product price is $52 — someone changed it after we approved.
	mock.getResponse = `{"success":true,"data":{"id":821,"regular_price":"52.00"}}`

	undoRes := httpPostJSON(t, ts.URL+"/v1/issues/"+id+"/undo", map[string]any{})
	if undoRes.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", undoRes.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(undoRes.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	undoRes.Body.Close()
	errObj, _ := body["error"].(map[string]any)
	if errObj == nil {
		t.Fatalf("body has no error envelope: %v", body)
	}
	if errObj["code"] != "undo_stale" {
		t.Errorf("error.code = %v, want undo_stale", errObj["code"])
	}
	if errObj["current"] != "52.00" {
		t.Errorf("error.current = %v, want \"52.00\"", errObj["current"])
	}
}

func TestUndoIssue_AlreadyUndone(t *testing.T) {
	ts, _, mock := newUndoRig(t)

	id := approveAndGetID(t, ts, map[string]any{
		"title":   "p",
		"persona": "pricing",
		"status":  "in_review",
		"proposal": map[string]any{
			"type":    "product_price_change",
			"content": "rationale",
			"target": map[string]any{
				"product_id":     821,
				"currency":       "USD",
				"previous_price": 39.00,
				"proposed_price": 44.99,
				"regular_price":  "44.99",
				"target_field":   "regular_price",
				"percent_change": 15.4,
				"direction":      "increase",
				"sources":        []map[string]any{{"url": "u", "comparable_product": "c", "observed_price": 45.0}},
			},
		},
	})
	mock.getResponse = `{"success":true,"data":{"id":821,"regular_price":"44.99"}}`

	first := httpPostJSON(t, ts.URL+"/v1/issues/"+id+"/undo", map[string]any{})
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first undo: status=%d", first.StatusCode)
	}
	first.Body.Close()

	// Second undo must fail with already_undone, not attempt another write.
	second := httpPostJSON(t, ts.URL+"/v1/issues/"+id+"/undo", map[string]any{})
	if second.StatusCode != http.StatusConflict {
		t.Fatalf("second undo: status=%d, want 409", second.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(second.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	second.Body.Close()
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "already_undone" {
		t.Errorf("code = %v, want already_undone", errObj["code"])
	}
}

func TestUndoIssue_DescriptionRewrite_HappyPath(t *testing.T) {
	ts, st, mock := newUndoRig(t)

	// Seed marketing agent (issues.persona FK).
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := st.DB.ExecContext(context.Background(),
		`INSERT INTO agents(persona, name, enabled, created_at, updated_at) VALUES('marketing','Marketing',1,?,?)`,
		now, now,
	); err != nil {
		t.Fatalf("seed marketing agent: %v", err)
	}

	id := approveAndGetID(t, ts, map[string]any{
		"title":   "rewrite",
		"persona": "marketing",
		"status":  "in_review",
		"proposal": map[string]any{
			"type":    "product_description_rewrite",
			"content": "Handcrafted from 100% merino wool, made to last.",
			"target": map[string]any{
				"product_id": 821,
				"previous":   "Old description that needs rewriting.",
			},
		},
	})

	// Staleness check expects the description we wrote.
	mock.getResponse = `{"success":true,"data":{"id":821,"description":"Handcrafted from 100% merino wool, made to last."}}`

	undoRes := httpPostJSON(t, ts.URL+"/v1/issues/"+id+"/undo", map[string]any{})
	if undoRes.StatusCode != http.StatusOK {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(undoRes.Body)
		undoRes.Body.Close()
		t.Fatalf("undo: status=%d body=%s", undoRes.StatusCode, buf.String())
	}
	undoRes.Body.Close()

	last := mock.calls[len(mock.calls)-1]
	if last.params["ability_name"] != "wooagent-products/update" {
		t.Errorf("final MCP call should be update; got %q", last.params["ability_name"])
	}
	inner, _ := last.params["parameters"].(map[string]any)
	if inner["description"] != "Old description that needs rewriting." {
		t.Errorf("undo MCP description = %v, want \"Old description that needs rewriting.\"", inner["description"])
	}

	var undoneAt sql.NullString
	if err := st.DB.QueryRow(
		`SELECT undone_at FROM issues WHERE id = ?`, id,
	).Scan(&undoneAt); err != nil {
		t.Fatalf("read undo state: %v", err)
	}
	if !undoneAt.Valid {
		t.Errorf("undone_at should be populated")
	}
}

func TestGetIssue_ExposesUndoneAt(t *testing.T) {
	ts, _, mock := newUndoRig(t)

	id := approveAndGetID(t, ts, map[string]any{
		"title":   "p",
		"persona": "pricing",
		"status":  "in_review",
		"proposal": map[string]any{
			"type":    "product_price_change",
			"content": "rationale",
			"target": map[string]any{
				"product_id":     821,
				"currency":       "USD",
				"previous_price": 39.00,
				"proposed_price": 44.99,
				"regular_price":  "44.99",
				"target_field":   "regular_price",
				"percent_change": 15.4,
				"direction":      "increase",
				"sources":        []map[string]any{{"url": "u", "comparable_product": "c", "observed_price": 45.0}},
			},
		},
	})
	mock.getResponse = `{"success":true,"data":{"id":821,"regular_price":"44.99"}}`
	undoRes := httpPostJSON(t, ts.URL+"/v1/issues/"+id+"/undo", map[string]any{})
	if undoRes.StatusCode != http.StatusOK {
		t.Fatalf("undo: status=%d", undoRes.StatusCode)
	}
	undoRes.Body.Close()

	getRes, err := http.Get(ts.URL + "/v1/issues/" + id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer getRes.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(getRes.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	issue, _ := payload["issue"].(map[string]any)
	if issue == nil {
		t.Fatalf("payload missing issue: %v", payload)
	}
	if _, present := issue["undone_at"]; !present {
		t.Errorf("issue payload missing undone_at: %v", issue)
	}
}

func TestUndoIssue_VariablePriceChange_HappyPath(t *testing.T) {
	ts, _, mcpHandler := newUndoRig(t)

	// Pre-script the approve-time MCP call: the dispatcher will invoke
	// wooagent-products/update-variations-bulk; success is enough.
	// At undo time the mock needs to answer variations-list with the
	// values we wrote (no drift) and then accept the reverse bulk write.
	mcpHandler.respondByAbility = map[string]string{
		"wooagent-products/update-variations-bulk": `{"success":true,"data":{"parent_id":4012,"updated":[{"variation_id":4013,"updated_fields":["regular_price"]},{"variation_id":4014,"updated_fields":["sale_price"]}]}}`,
		"wooagent-products/variations-list":        `{"success":true,"data":{"parent_id":4012,"variations":[{"id":4013,"attributes_label":"S/Blue","regular_price":"21.59","sale_price":"","stock_status":"instock"},{"id":4014,"attributes_label":"M/Blue","regular_price":"30.00","sale_price":"27.00","stock_status":"instock"}]}}`,
	}

	id := approveAndGetID(t, ts, map[string]any{
		"title":   "Price change · V-Neck",
		"persona": "pricing",
		"status":  "in_review",
		"proposal": map[string]any{
			"type":    "product_price_change_variable",
			"content": "rationale",
			"target": map[string]any{
				"product_id":     4012,
				"percent_change": 8.0,
				"direction":      "increase",
				"variations": []any{
					map[string]any{
						"variation_id":     4013,
						"attributes_label": "Small / Blue",
						"target_field":     "regular_price",
						"regular_price":    "21.59",
						"previous_price":   19.99,
						"proposed_price":   21.59,
					},
					map[string]any{
						"variation_id":     4014,
						"attributes_label": "Medium / Blue",
						"target_field":     "sale_price",
						"regular_price":    "27.00",
						"previous_price":   25.00,
						"proposed_price":   27.00,
					},
				},
			},
		},
	})

	// Reset captured calls so we only see undo-time MCP traffic.
	mcpHandler.calls = nil

	undoRes := httpPostJSON(t, ts.URL+"/v1/issues/"+id+"/undo", map[string]any{})
	if undoRes.StatusCode != http.StatusOK {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(undoRes.Body)
		undoRes.Body.Close()
		t.Fatalf("expected 200; got %d body=%s", undoRes.StatusCode, buf.String())
	}
	undoRes.Body.Close()

	// Find the bulk-update call carrying the reverse write.
	var bulkArgs map[string]any
	for _, c := range mcpHandler.calls {
		abilityName, _ := c.params["ability_name"].(string)
		if abilityName == "wooagent-products/update-variations-bulk" {
			bulkArgs, _ = c.params["parameters"].(map[string]any)
		}
	}
	if bulkArgs == nil {
		t.Fatalf("undo did not call update-variations-bulk; calls=%+v", mcpHandler.calls)
	}
	// updates is []map[string]any in-process (no JSON round-trip), so we
	// try both shapes for resilience to a future params marshaler change.
	var updatesList []map[string]any
	switch v := bulkArgs["updates"].(type) {
	case []map[string]any:
		updatesList = v
	case []any:
		for _, e := range v {
			if m, ok := e.(map[string]any); ok {
				updatesList = append(updatesList, m)
			}
		}
	}
	if len(updatesList) != 2 {
		t.Fatalf("expected 2 updates; got %d (bulkArgs=%+v)", len(updatesList), bulkArgs)
	}
	if updatesList[0]["regular_price"] != "19.99" {
		t.Fatalf("variation 4013 undo regular_price = %v; want 19.99", updatesList[0]["regular_price"])
	}
	if updatesList[1]["sale_price"] != "25.00" {
		t.Fatalf("variation 4014 undo sale_price = %v; want 25.00", updatesList[1]["sale_price"])
	}
}

func TestUndoIssue_VariablePriceChange_StaleReturns409(t *testing.T) {
	ts, _, mcpHandler := newUndoRig(t)

	// Approve-time: bulk update succeeds. Undo-time: variations-list
	// returns a DIFFERENT regular_price for 4013 (someone changed it),
	// so undo should bail with 409.
	mcpHandler.respondByAbility = map[string]string{
		"wooagent-products/update-variations-bulk": `{"success":true,"data":{"parent_id":4012,"updated":[]}}`,
		"wooagent-products/variations-list":        `{"success":true,"data":{"parent_id":4012,"variations":[{"id":4013,"attributes_label":"S/Blue","regular_price":"99.99","sale_price":"","stock_status":"instock"}]}}`,
	}

	id := approveAndGetID(t, ts, map[string]any{
		"title":   "Price change · X",
		"persona": "pricing",
		"status":  "in_review",
		"proposal": map[string]any{
			"type":    "product_price_change_variable",
			"content": "rationale",
			"target": map[string]any{
				"product_id":     4012,
				"percent_change": 8.0,
				"direction":      "increase",
				"variations": []any{
					map[string]any{
						"variation_id":     4013,
						"attributes_label": "Small / Blue",
						"target_field":     "regular_price",
						"regular_price":    "21.59",
						"previous_price":   19.99,
						"proposed_price":   21.59,
					},
				},
			},
		},
	})

	mcpHandler.calls = nil

	undoRes := httpPostJSON(t, ts.URL+"/v1/issues/"+id+"/undo", map[string]any{})
	if undoRes.StatusCode != http.StatusConflict {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(undoRes.Body)
		undoRes.Body.Close()
		t.Fatalf("expected 409 stale; got %d body=%s", undoRes.StatusCode, buf.String())
	}
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(undoRes.Body)
	undoRes.Body.Close()
	if !strings.Contains(buf.String(), "undo_stale") {
		t.Fatalf("expected error code undo_stale; got %s", buf.String())
	}
}
