package httpapi

import (
	"bytes"
	"context"
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

// Tests for the pricing-persona side of the approve dispatch table. The
// marketing path is exercised in handlers_batches_test.go; here we lock in
// (1) buildParams shape for product_price_change, (2) the helper that
// canonicalises decimal prices, and (3) that an end-to-end approve actually
// invokes the MCP `wooagent-products/update` ability with the right
// regular_price string.

// recordingMCP captures the params passed to CallTool so we can assert the
// dispatch built the right payload. The fakeMCP in handlers_batches_test.go
// only counts calls; this records the wire-level args.
type recordingMCP struct {
	tool   string
	params any
	calls  int
}

func (r *recordingMCP) Initialize(_ context.Context) (mcp.ServerInfo, error) {
	return mcp.ServerInfo{}, nil
}

func (r *recordingMCP) CallTool(_ context.Context, tool string, params any) (mcp.ToolCallResult, error) {
	r.calls++
	r.tool = tool
	r.params = params
	return mcp.ToolCallResult{Content: []mcp.ContentPart{{Type: "text", Text: `{"success":true}`}}}, nil
}

func TestApproveDispatch_ProductPriceChange_BuildParamsShape(t *testing.T) {
	d, ok := approveDispatchByType["product_price_change"]
	if !ok {
		t.Fatalf("product_price_change dispatch not registered")
	}
	if d.ability != "wooagent-products/update" {
		t.Errorf("ability = %q, want wooagent-products/update", d.ability)
	}

	cases := []struct {
		name      string
		target    map[string]any
		wantPrice string
		wantErr   bool
	}{
		{
			name:      "string price preserved",
			target:    map[string]any{"product_id": 821, "regular_price": "29.99"},
			wantPrice: "29.99",
		},
		{
			name:      "float price canonicalised to 2dp",
			target:    map[string]any{"product_id": 821, "regular_price": 29.9},
			wantPrice: "29.90",
		},
		{
			name:      "int price canonicalised",
			target:    map[string]any{"product_id": 821, "regular_price": 30},
			wantPrice: "30.00",
		},
		{
			name:    "zero price rejected",
			target:  map[string]any{"product_id": 821, "regular_price": 0},
			wantErr: true,
		},
		{
			name:    "negative price rejected",
			target:  map[string]any{"product_id": 821, "regular_price": -5.0},
			wantErr: true,
		},
		{
			name:    "missing regular_price",
			target:  map[string]any{"product_id": 821},
			wantErr: true,
		},
		{
			name:    "missing product_id",
			target:  map[string]any{"regular_price": "29.99"},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params, err := d.buildParams("rationale ignored for price changes", tc.target)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got params=%v", params)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if params["id"] != 821 {
				t.Errorf("id = %v, want 821", params["id"])
			}
			if got := params["regular_price"]; got != tc.wantPrice {
				t.Errorf("regular_price = %q, want %q", got, tc.wantPrice)
			}
			if _, hasDesc := params["description"]; hasDesc {
				t.Errorf("price-change params should NOT carry description; got %v", params)
			}
		})
	}
}

// End-to-end: create a price-change issue, approve it, assert MCP got
// invoked with the right payload and the issue moved to status=done.
func TestApproveIssue_ProductPriceChange_EndToEnd(t *testing.T) {
	rig := newPricingTestRig(t)

	// Seed pricing agent (issues.persona FK).
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := rig.store.DB.ExecContext(context.Background(),
		`INSERT INTO agents(persona, name, enabled, created_at, updated_at) VALUES('pricing', 'Pricing Agent', 1, ?, ?)`,
		now, now,
	); err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	createBody := map[string]any{
		"title":    "Price change · Cashmere Scarf · $39.00 → $44.99 (+15.4%)",
		"persona":  "pricing",
		"status":   "in_review",
		"priority": "medium",
		"proposal": map[string]any{
			"type":    "product_price_change",
			"content": "Median of 4 cashmere scarves at independent retailers is $46.50; current $39 trails the band.",
			"target": map[string]any{
				"product_id":     821,
				"product_name":   "Cashmere Scarf",
				"product_sku":    "SCARF-001",
				"currency":       "USD",
				"previous_price": 39.00,
				"proposed_price": 44.99,
				"regular_price":  "44.99",
				"percent_change": 15.4,
				"direction":      "increase",
				"sources": []map[string]any{
					{"url": "https://example.com/a", "comparable_product": "Brand X cashmere scarf", "observed_price": 49.0},
					{"url": "https://example.com/b", "comparable_product": "Brand Y cashmere scarf", "observed_price": 45.0},
					{"url": "https://example.com/c", "comparable_product": "Brand Z cashmere scarf", "observed_price": 46.0},
				},
			},
		},
	}
	res := httpPostJSON(t, rig.ts.URL+"/v1/issues", createBody)
	if res.StatusCode != http.StatusCreated {
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(res.Body)
		res.Body.Close()
		t.Fatalf("create issue: status=%d body=%s", res.StatusCode, body.String())
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
	if created.Issue.ID == "" {
		t.Fatalf("created issue has no id")
	}

	approveRes := httpPostJSON(t, rig.ts.URL+"/v1/issues/"+created.Issue.ID+"/approve", map[string]any{})
	if approveRes.StatusCode != http.StatusOK {
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(approveRes.Body)
		approveRes.Body.Close()
		t.Fatalf("approve: status=%d body=%s", approveRes.StatusCode, body.String())
	}
	approveRes.Body.Close()

	if rig.rec.tool != "mcp-adapter-execute-ability" {
		t.Errorf("MCP tool = %q, want mcp-adapter-execute-ability", rig.rec.tool)
	}
	pmap, ok := rig.rec.params.(map[string]any)
	if !ok {
		t.Fatalf("MCP params not a map: %T", rig.rec.params)
	}
	if pmap["ability_name"] != "wooagent-products/update" {
		t.Errorf("MCP ability_name = %q, want wooagent-products/update", pmap["ability_name"])
	}
	inner, _ := pmap["parameters"].(map[string]any)
	if inner == nil {
		t.Fatalf("MCP parameters missing or wrong type: %#v", pmap)
	}
	if inner["id"] != 821 {
		t.Errorf("MCP parameters.id = %v, want 821", inner["id"])
	}
	if inner["regular_price"] != "44.99" {
		t.Errorf("MCP parameters.regular_price = %v, want %q", inner["regular_price"], "44.99")
	}
	if _, hasDesc := inner["description"]; hasDesc {
		t.Errorf("MCP parameters should not carry description for a price change")
	}

	var status string
	if err := rig.store.DB.QueryRowContext(context.Background(),
		`SELECT status FROM issues WHERE id = ?`, created.Issue.ID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "done" {
		t.Errorf("issue status = %q, want done", status)
	}
}

type pricingRig struct {
	ts    *httptest.Server
	store *store.Store
	rec   *recordingMCP
}

func newPricingTestRig(t *testing.T) *pricingRig {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	m := &manifest.Manifest{
		Version: 1,
		Entries: []manifest.Entry{{
			Ability:        "wooagent-products/update",
			NamespaceOwner: "test",
			SchemaHash:     "sha256:test",
			Scope:          manifest.ScopePropose,
			Reversibility:  0.6,
			Personas:       []manifest.Persona{manifest.PersonaPricing},
		}},
	}
	lookup, err := manifest.NewLookup(m)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	rec := &recordingMCP{}
	p := pep.New(lookup, rec, st.DB)
	s := &Server{store: st, pep: p}

	r := chi.NewRouter()
	r.Post("/v1/issues", s.handleCreateIssue)
	r.Post("/v1/issues/{id}/approve", s.handleApproveIssue)

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return &pricingRig{ts: ts, store: st, rec: rec}
}
