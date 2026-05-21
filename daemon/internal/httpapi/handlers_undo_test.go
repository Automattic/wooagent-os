package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
