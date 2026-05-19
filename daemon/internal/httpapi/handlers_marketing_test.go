package httpapi

import (
	"testing"
)

// Locks the Marketing cold-draft dispatch contract. The dispatcher reads
// body_short / body_long from the selected variant and writes only the fields
// named in target.drafting via wooagent-products/update. Fields not in
// drafting are intentionally omitted so existing product copy is preserved.

func TestApproveDispatch_ProductColdDraft_BothFields(t *testing.T) {
	dispatch, ok := approveDispatchByType["product_cold_draft"]
	if !ok {
		t.Fatalf("product_cold_draft dispatch not registered")
	}
	if dispatch.ability != "wooagent-products/update" {
		t.Errorf("ability = %q, want wooagent-products/update", dispatch.ability)
	}

	target := map[string]any{
		"product_id": float64(42),
		"drafting":   []any{"short", "long"},
	}
	variant := map[string]any{
		"id":         "var_a",
		"body_short": "Cozy wool slippers.",
		"body_long":  "Handcrafted from 100% merino wool, made to last.",
	}
	params, err := dispatch.buildParams("", variant, target)
	if err != nil {
		t.Fatalf("buildParams: %v", err)
	}
	if params["id"] != 42 {
		t.Errorf("id = %v, want 42", params["id"])
	}
	if params["short_description"] != "Cozy wool slippers." {
		t.Errorf("short_description = %q, want %q", params["short_description"], "Cozy wool slippers.")
	}
	if params["description"] != "Handcrafted from 100% merino wool, made to last." {
		t.Errorf("description = %q, want %q", params["description"], "Handcrafted from 100% merino wool, made to last.")
	}
}

func TestApproveDispatch_ProductColdDraft_LongOnly_OmitsShort(t *testing.T) {
	dispatch := approveDispatchByType["product_cold_draft"]
	target := map[string]any{
		"product_id": float64(42),
		"drafting":   []any{"long"},
	}
	variant := map[string]any{
		"id":        "var_a",
		"body_long": "Handcrafted from 100% merino wool, made to last.",
	}
	params, err := dispatch.buildParams("", variant, target)
	if err != nil {
		t.Fatalf("buildParams: %v", err)
	}
	if _, has := params["short_description"]; has {
		t.Errorf("short_description should be omitted when drafting=[long]; got %v", params["short_description"])
	}
	if params["description"] != "Handcrafted from 100% merino wool, made to last." {
		t.Errorf("description = %q, want %q", params["description"], "Handcrafted from 100% merino wool, made to last.")
	}
}

func TestApproveDispatch_ProductColdDraft_MissingDrafting_Errors(t *testing.T) {
	dispatch := approveDispatchByType["product_cold_draft"]
	target := map[string]any{
		"product_id": float64(42),
		// no drafting
	}
	variant := map[string]any{"id": "var_a", "body_long": "..."}
	_, err := dispatch.buildParams("", variant, target)
	if err == nil {
		t.Fatalf("expected error when target has no drafting")
	}
}

func TestApproveDispatch_ProductColdDraft_NilVariant_Errors(t *testing.T) {
	dispatch := approveDispatchByType["product_cold_draft"]
	target := map[string]any{"product_id": float64(42), "drafting": []any{"long"}}
	_, err := dispatch.buildParams("", nil, target)
	if err == nil {
		t.Fatalf("expected error when variant is nil")
	}
}
