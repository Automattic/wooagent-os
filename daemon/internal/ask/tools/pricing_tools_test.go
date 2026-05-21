package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestProduceRecommendation_HappyPath asserts the tool writes an Issue
// + Run pair with the cadence-mode pricing proposal shape — title with
// before→after, proposal_type=product_price_change, sources array
// preserved in proposal_target.
func TestProduceRecommendation_HappyPath(t *testing.T) {
	db := newTestDB(t)
	ensureAgent(t, db, "pricing")
	tool := &ProduceRecommendationTool{DB: db}

	input, _ := json.Marshal(produceRecommendationInput{
		ProductID:     7,
		ProductName:   "Wool Throw",
		ProductSKU:    "WT-001",
		Currency:      "USD",
		CurrentPrice:  28.00,
		ProposedPrice: 24.00,
		Rationale:     "Mid-tier retailers are pricing comparable throws in the $22-$26 range.",
		Sources: []pricingSource{
			{Retailer: "Brooklinen", Price: 22.00},
			{Retailer: "Parachute", Price: 25.00},
			{Retailer: "Boll & Branch", Price: 26.00},
		},
	})
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var resp produceOutput
	json.Unmarshal([]byte(out), &resp)
	if !resp.OK || resp.ProposalID == "" {
		t.Fatalf("expected ok+id, got %+v", resp)
	}

	var title, ptype, pcontent, ptargetJSON, dedup string
	db.QueryRow(
		`SELECT title, proposal_type, proposal_content, proposal_target, dedup_key
		   FROM issues WHERE id = ?`, resp.ProposalID,
	).Scan(&title, &ptype, &pcontent, &ptargetJSON, &dedup)

	if !strings.Contains(title, "$28.00") || !strings.Contains(title, "$24.00") {
		t.Errorf("title %q missing before/after prices", title)
	}
	if !strings.Contains(title, "Wool Throw") {
		t.Errorf("title %q missing product name", title)
	}
	if ptype != "product_price_change" {
		t.Errorf("proposal_type = %q, want product_price_change", ptype)
	}
	if pcontent == "" || !strings.Contains(strings.ToLower(pcontent), "mid-tier") {
		t.Errorf("proposal_content should be the rationale, got %q", pcontent)
	}
	if dedup != "product:7" {
		t.Errorf("dedup_key = %q, want product:7", dedup)
	}
	var target map[string]any
	json.Unmarshal([]byte(ptargetJSON), &target)
	if got, _ := target["proposed_price"].(float64); got != 24.0 {
		t.Errorf("target.proposed_price = %v, want 24.0", target["proposed_price"])
	}
	if srcs, _ := target["sources"].([]any); len(srcs) != 3 {
		t.Errorf("target.sources should be 3 entries, got %v", target["sources"])
	}
}

// TestProduceRecommendation_TooThinGrounding refuses to write when the
// model offers fewer than 3 sources. Same threshold the cadence-mode
// skill enforces (out.Sources < 3 is a no_proposal skip there).
func TestProduceRecommendation_TooThinGrounding(t *testing.T) {
	db := newTestDB(t)
	ensureAgent(t, db, "pricing")
	tool := &ProduceRecommendationTool{DB: db}
	input, _ := json.Marshal(produceRecommendationInput{
		ProductID: 1, ProductName: "X",
		CurrentPrice: 10, ProposedPrice: 9, Rationale: "ok",
		Sources: []pricingSource{
			{Retailer: "A", Price: 9}, {Retailer: "B", Price: 9},
		},
	})
	if _, err := tool.Execute(context.Background(), input); err == nil {
		t.Errorf("expected error for thin grounding, got nil")
	}
}

// TestProduceRecommendation_StepCap mirrors the cadence-mode ±25% guard.
func TestProduceRecommendation_StepCap(t *testing.T) {
	db := newTestDB(t)
	ensureAgent(t, db, "pricing")
	tool := &ProduceRecommendationTool{DB: db}
	input, _ := json.Marshal(produceRecommendationInput{
		ProductID: 1, ProductName: "X",
		CurrentPrice: 10, ProposedPrice: 20, // +100% change
		Rationale: "out of range",
		Sources: []pricingSource{
			{Retailer: "A", Price: 20}, {Retailer: "B", Price: 20}, {Retailer: "C", Price: 20},
		},
	})
	if _, err := tool.Execute(context.Background(), input); err == nil {
		t.Errorf("expected step-cap error, got nil")
	}
}

// TestWebSearchToolDef asserts the server-tool spec the Pricing agent
// advertises matches the Anthropic shape (type + name + max_uses).
// Pinned here so a future model-version bump or accidental rename
// surfaces in this package, not at run-time when an operator chats.
func TestWebSearchToolDef(t *testing.T) {
	def := WebSearchToolDef
	if def.Type != "web_search_20250305" {
		t.Errorf("type = %q, want web_search_20250305", def.Type)
	}
	if def.Name != "web_search" {
		t.Errorf("name = %q, want web_search", def.Name)
	}
	if def.MaxUses != 4 {
		t.Errorf("max_uses = %d, want 4", def.MaxUses)
	}
}

// TestCurrencySymbol covers the common map + unknown-code passthrough.
func TestCurrencySymbol(t *testing.T) {
	cases := []struct{ in, want string }{
		{"USD", "$"}, {"", "$"}, {"EUR", "€"}, {"GBP", "£"},
		{"JPY", "¥"}, {"CAD", "CA$"}, {"AUD", "A$"},
		{"NOK", "NOK "}, // unknown passthrough
	}
	for _, tc := range cases {
		if got := currencySymbol(tc.in); got != tc.want {
			t.Errorf("currencySymbol(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
