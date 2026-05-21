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

func TestBuildVariationChildTarget_RegularPricePath(t *testing.T) {
	parent := product{ID: 4012, Name: "V-Neck T-Shirt", SKU: "VN-001", ImageURL: "https://example.com/v.jpg"}
	v := variation{ID: 4013, AttributesLabel: "Small / Blue", RegularPrice: "19.99", SalePrice: ""}
	out := proposalOut{
		PercentChange:  8.0,
		Direction:      "increase",
		ObservedMedian: 24.0,
		Sources:        []proposalSource{{URL: "https://example.com/a", ComparableProduct: "Linen Tee", ObservedPrice: 24.0}},
	}
	target := buildVariationChildTarget(parent, v, out, "USD", 19.99, "regular_price", 21.59)

	if target["product_id"].(int) != 4013 {
		t.Fatalf("product_id should be the variation_id (4013); got %v", target["product_id"])
	}
	if target["product_name"].(string) != "V-Neck T-Shirt — Small / Blue" {
		t.Fatalf("product_name = %q; want \"V-Neck T-Shirt — Small / Blue\"", target["product_name"])
	}
	if target["target_field"].(string) != "regular_price" {
		t.Fatalf("target_field = %q; want regular_price", target["target_field"])
	}
	if target["regular_price"].(string) != "21.59" {
		t.Fatalf("regular_price (decimal string) = %q; want \"21.59\"", target["regular_price"])
	}
	if math.Abs(target["previous_price"].(float64)-19.99) > 0.001 {
		t.Fatalf("previous_price = %v; want 19.99", target["previous_price"])
	}
	if math.Abs(target["proposed_price"].(float64)-21.59) > 0.001 {
		t.Fatalf("proposed_price = %v; want 21.59", target["proposed_price"])
	}
	if target["product_sku"].(string) != "VN-001" {
		t.Fatalf("product_sku should inherit parent's sku; got %q", target["product_sku"])
	}
	// sale_price_observed should NOT be in the target when SalePrice is empty.
	if _, present := target["sale_price_observed"]; present {
		t.Fatalf("sale_price_observed should be omitted when variation has no sale price; got %v", target["sale_price_observed"])
	}
}

func TestBuildVariationChildTarget_SalePricePath(t *testing.T) {
	parent := product{ID: 4012, Name: "V-Neck T-Shirt", SKU: "VN-001"}
	v := variation{ID: 4014, AttributesLabel: "Medium / Blue", RegularPrice: "30.00", SalePrice: "25.00"}
	out := proposalOut{PercentChange: 8.0, Direction: "increase"}
	// Anchor is the sale price (25.00); proposed = 25.00 * 1.08 = 27.00
	target := buildVariationChildTarget(parent, v, out, "USD", 25.00, "sale_price", 27.00)

	if target["target_field"].(string) != "sale_price" {
		t.Fatalf("target_field = %q; want sale_price", target["target_field"])
	}
	if target["regular_price"].(string) != "27.00" {
		t.Fatalf("regular_price (decimal string the dispatcher writes — keyed regular_price for back-compat with the simple shape, even when target_field=sale_price) = %q; want \"27.00\"", target["regular_price"])
	}
	if math.Abs(target["previous_price"].(float64)-25.00) > 0.001 {
		t.Fatalf("previous_price = %v; want 25.00 (sale anchor)", target["previous_price"])
	}
	if target["sale_price_observed"].(string) != "25.00" {
		t.Fatalf("sale_price_observed = %q; want \"25.00\"", target["sale_price_observed"])
	}
	if target["regular_price_observed"].(string) != "30.00" {
		t.Fatalf("regular_price_observed = %q; want \"30.00\"", target["regular_price_observed"])
	}
}
