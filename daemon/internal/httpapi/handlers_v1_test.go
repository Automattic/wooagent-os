package httpapi

import (
	"encoding/json"
	"testing"
)

func TestApproveDispatch_VariablePriceChange_BuildParams(t *testing.T) {
	d, ok := approveDispatchByType["product_price_change_variable"]
	if !ok {
		t.Fatal("dispatcher missing entry for product_price_change_variable")
	}
	target := map[string]any{
		"product_id": float64(4012),
		"variations": []any{
			map[string]any{
				"variation_id":   float64(4013),
				"target_field":   "regular_price",
				"regular_price":  "21.59",
				"previous_price": float64(19.99),
			},
			map[string]any{
				"variation_id":   float64(4014),
				"target_field":   "sale_price",
				"regular_price":  "27.00",
				"previous_price": float64(25.00),
			},
		},
	}
	params, applied, err := d.buildParams("", nil, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params["parent_id"] != 4012 {
		t.Fatalf("parent_id = %v; want 4012", params["parent_id"])
	}
	updates, ok := params["updates"].([]map[string]any)
	if !ok || len(updates) != 2 {
		t.Fatalf("updates malformed: %+v", params["updates"])
	}
	if updates[0]["variation_id"] != 4013 || updates[0]["regular_price"] != "21.59" {
		t.Fatalf("updates[0] = %+v", updates[0])
	}
	if updates[1]["variation_id"] != 4014 || updates[1]["sale_price"] != "27.00" {
		t.Fatalf("updates[1] = %+v", updates[1])
	}
	if applied == "" {
		t.Fatal("appliedValue must be non-empty for undoable proposal types")
	}
	// Snapshot is JSON; sanity-check it parses to the expected length.
	var snap []map[string]any
	if err := json.Unmarshal([]byte(applied), &snap); err != nil {
		t.Fatalf("applied snapshot not valid JSON: %v", err)
	}
	if len(snap) != 2 {
		t.Fatalf("snapshot len = %d; want 2", len(snap))
	}
}
