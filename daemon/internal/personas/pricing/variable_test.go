package pricing

import (
	"math"
	"testing"
)

func TestMedianAnchor_OddCount(t *testing.T) {
	vs := []variation{
		{ID: 1, RegularPrice: "19.99"},
		{ID: 2, RegularPrice: "24.99"},
		{ID: 3, RegularPrice: "29.99"},
	}
	got, ok := medianAnchor(vs)
	if !ok {
		t.Fatal("expected ok=true; got false")
	}
	if math.Abs(got-24.99) > 0.001 {
		t.Fatalf("expected 24.99; got %v", got)
	}
}

func TestMedianAnchor_EvenCount(t *testing.T) {
	vs := []variation{
		{ID: 1, RegularPrice: "10.00"},
		{ID: 2, RegularPrice: "20.00"},
		{ID: 3, RegularPrice: "30.00"},
		{ID: 4, RegularPrice: "40.00"},
	}
	got, ok := medianAnchor(vs)
	if !ok {
		t.Fatal("expected ok=true; got false")
	}
	if math.Abs(got-25.00) > 0.001 {
		t.Fatalf("expected 25.00; got %v", got)
	}
}

func TestMedianAnchor_PrefersSalePrice(t *testing.T) {
	vs := []variation{
		{ID: 1, RegularPrice: "30.00", SalePrice: "25.00"},
		{ID: 2, RegularPrice: "30.00", SalePrice: ""},
		{ID: 3, RegularPrice: "30.00", SalePrice: "25.00"},
	}
	got, ok := medianAnchor(vs)
	if !ok {
		t.Fatal("expected ok=true; got false")
	}
	if math.Abs(got-25.00) > 0.001 {
		t.Fatalf("expected 25.00; got %v", got)
	}
}

func TestMedianAnchor_NoPricedVariations(t *testing.T) {
	vs := []variation{
		{ID: 1, RegularPrice: "", SalePrice: ""},
		{ID: 2, RegularPrice: "0.00", SalePrice: ""},
	}
	_, ok := medianAnchor(vs)
	if ok {
		t.Fatal("expected ok=false when no variation has a parseable positive price")
	}
}

func TestBuildPricingTargetVariable_PerVariationTargetField(t *testing.T) {
	p := product{ID: 4012, Name: "V-Neck T-Shirt", SKU: "VN-001", ImageURL: "https://example.com/v.jpg", Type: "variable"}
	vs := []variation{
		{ID: 4013, AttributesLabel: "Small / Blue", RegularPrice: "19.99", SalePrice: ""},
		{ID: 4014, AttributesLabel: "Medium / Blue", RegularPrice: "30.00", SalePrice: "25.00"},
	}
	out := proposalOut{
		PreviousPrice:  22.49,
		ProposedPrice:  24.29,
		PercentChange:  8.0,
		Direction:      "increase",
		Rationale:      "Comparables suggest +8% headroom.",
		Sources:        []proposalSource{{URL: "https://example.com/a", ComparableProduct: "Linen Tee", ObservedPrice: 24.0}},
		ObservedMedian: 24.0,
	}
	target := buildPricingTargetVariable(p, vs, out, "USD")

	if target["product_id"].(int) != 4012 {
		t.Fatalf("product_id = %v; want 4012", target["product_id"])
	}
	if target["percent_change"].(float64) != 8.0 {
		t.Fatalf("percent_change = %v; want 8.0", target["percent_change"])
	}
	if target["variation_count"].(int) != 2 {
		t.Fatalf("variation_count = %v; want 2", target["variation_count"])
	}

	variations := target["variations"].([]map[string]any)
	if len(variations) != 2 {
		t.Fatalf("variations len = %d; want 2", len(variations))
	}

	v0 := variations[0]
	if v0["target_field"].(string) != "regular_price" {
		t.Fatalf("v0 target_field = %q; want regular_price", v0["target_field"])
	}
	if v0["regular_price"].(string) != "21.59" {
		t.Fatalf("v0 regular_price = %q; want \"21.59\"", v0["regular_price"])
	}
	if math.Abs(v0["previous_price"].(float64)-19.99) > 0.001 {
		t.Fatalf("v0 previous_price = %v; want 19.99", v0["previous_price"])
	}

	v1 := variations[1]
	if v1["target_field"].(string) != "sale_price" {
		t.Fatalf("v1 target_field = %q; want sale_price", v1["target_field"])
	}
	if v1["regular_price"].(string) != "27.00" {
		t.Fatalf("v1 proposed-price string (keyed regular_price for back-compat with dispatcher) = %q; want \"27.00\"", v1["regular_price"])
	}
	if math.Abs(v1["previous_price"].(float64)-25.00) > 0.001 {
		t.Fatalf("v1 previous_price = %v; want 25.00 (sale)", v1["previous_price"])
	}
}

func TestBuildPricingTargetVariable_RollupTotals(t *testing.T) {
	p := product{ID: 1, Name: "X"}
	vs := []variation{
		{ID: 10, AttributesLabel: "S", RegularPrice: "10.00"},
		{ID: 11, AttributesLabel: "M", RegularPrice: "20.00"},
		{ID: 12, AttributesLabel: "L", RegularPrice: "30.00"},
	}
	out := proposalOut{PercentChange: 10.0, ProposedPrice: 22.0, PreviousPrice: 20.0}
	target := buildPricingTargetVariable(p, vs, out, "USD")

	if math.Abs(target["previous_price_min"].(float64)-10.00) > 0.001 {
		t.Fatalf("previous_price_min = %v", target["previous_price_min"])
	}
	if math.Abs(target["previous_price_max"].(float64)-30.00) > 0.001 {
		t.Fatalf("previous_price_max = %v", target["previous_price_max"])
	}
	if math.Abs(target["proposed_price_min"].(float64)-11.00) > 0.001 {
		t.Fatalf("proposed_price_min = %v", target["proposed_price_min"])
	}
	if math.Abs(target["proposed_price_max"].(float64)-33.00) > 0.001 {
		t.Fatalf("proposed_price_max = %v", target["proposed_price_max"])
	}
}
