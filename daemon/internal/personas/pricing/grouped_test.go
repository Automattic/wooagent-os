package pricing

import (
	"math"
	"testing"
)

func TestMedianAnchorProducts_OddCount(t *testing.T) {
	ps := []product{
		{ID: 1, RegularPrice: "19.99"},
		{ID: 2, RegularPrice: "24.99"},
		{ID: 3, RegularPrice: "29.99"},
	}
	got, ok := medianAnchorProducts(ps)
	if !ok {
		t.Fatal("expected ok=true; got false")
	}
	if math.Abs(got-24.99) > 0.001 {
		t.Fatalf("expected 24.99; got %v", got)
	}
}

func TestMedianAnchorProducts_EvenCount(t *testing.T) {
	ps := []product{
		{ID: 1, RegularPrice: "10.00"},
		{ID: 2, RegularPrice: "20.00"},
		{ID: 3, RegularPrice: "30.00"},
		{ID: 4, RegularPrice: "40.00"},
	}
	got, ok := medianAnchorProducts(ps)
	if !ok {
		t.Fatal("expected ok=true; got false")
	}
	if math.Abs(got-25.00) > 0.001 {
		t.Fatalf("expected 25.00; got %v", got)
	}
}

func TestMedianAnchorProducts_PrefersSalePrice(t *testing.T) {
	// pickAnchorPrice prefers sale when set; falls back to regular otherwise.
	// Anchors: [25 (sale), 30 (regular fallback), 25 (sale)] → sorted [25,25,30]
	// → median 25.
	ps := []product{
		{ID: 1, RegularPrice: "30.00", SalePrice: "25.00"},
		{ID: 2, RegularPrice: "30.00", SalePrice: ""},
		{ID: 3, RegularPrice: "30.00", SalePrice: "25.00"},
	}
	got, ok := medianAnchorProducts(ps)
	if !ok {
		t.Fatal("expected ok=true; got false")
	}
	if math.Abs(got-25.00) > 0.001 {
		t.Fatalf("expected 25.00; got %v", got)
	}
}

func TestMedianAnchorProducts_NoPricedChildren(t *testing.T) {
	ps := []product{
		{ID: 1, RegularPrice: "", SalePrice: ""},
		{ID: 2, RegularPrice: "0.00", SalePrice: ""},
	}
	_, ok := medianAnchorProducts(ps)
	if ok {
		t.Fatal("expected ok=false when no child has a parseable positive price")
	}
}

func TestBuildGroupedChildTarget_RegularPricePath(t *testing.T) {
	// Grouped children are full products with their own SKU, image, and
	// name. The proposal target should carry the CHILD's own identity —
	// the batch header is what carries the parent's "Logo Collection"
	// context.
	parent := product{ID: 4000, Name: "Logo Collection", SKU: "LC-001"}
	child := product{
		ID: 3902, Name: "Logo Mug", SKU: "LM-001",
		RegularPrice: "12.00", SalePrice: "",
		ImageURL: "https://example.com/mug.jpg", ImageAlt: "Logo mug",
	}
	out := proposalOut{
		PercentChange:  8.33,
		Direction:      "increase",
		ObservedMedian: 14.0,
		Sources: []proposalSource{
			{URL: "https://example.com/a", ComparableProduct: "Brand Mug", ObservedPrice: 14.0},
		},
	}
	target := buildGroupedChildTarget(parent, child, out, "USD", 12.00, "regular_price", 13.00)

	if target["product_id"].(int) != 3902 {
		t.Fatalf("product_id should be the child id (3902); got %v", target["product_id"])
	}
	if target["product_name"].(string) != "Logo Mug" {
		t.Fatalf("product_name = %q; want \"Logo Mug\" (child's own name)", target["product_name"])
	}
	if target["product_sku"].(string) != "LM-001" {
		t.Fatalf("product_sku should be child's own SKU; got %q", target["product_sku"])
	}
	if target["image_url"].(string) != "https://example.com/mug.jpg" {
		t.Fatalf("image_url should be child's own image; got %q", target["image_url"])
	}
	if target["target_field"].(string) != "regular_price" {
		t.Fatalf("target_field = %q; want regular_price", target["target_field"])
	}
	if target["regular_price"].(string) != "13.00" {
		t.Fatalf("regular_price = %q; want \"13.00\"", target["regular_price"])
	}
	if math.Abs(target["previous_price"].(float64)-12.00) > 0.001 {
		t.Fatalf("previous_price = %v; want 12.00", target["previous_price"])
	}
	if math.Abs(target["proposed_price"].(float64)-13.00) > 0.001 {
		t.Fatalf("proposed_price = %v; want 13.00", target["proposed_price"])
	}
	if _, present := target["sale_price_observed"]; present {
		t.Fatalf("sale_price_observed should be omitted when child has no sale price; got %v", target["sale_price_observed"])
	}
}

func TestBuildGroupedChildTarget_SalePricePath(t *testing.T) {
	parent := product{ID: 4000, Name: "Logo Collection"}
	child := product{
		ID: 3903, Name: "Logo T-Shirt", SKU: "LT-001",
		RegularPrice: "25.00", SalePrice: "20.00",
	}
	out := proposalOut{PercentChange: 10.0, Direction: "increase"}
	// Anchor is the sale price (20.00); proposed = 22.00.
	target := buildGroupedChildTarget(parent, child, out, "USD", 20.00, "sale_price", 22.00)

	if target["target_field"].(string) != "sale_price" {
		t.Fatalf("target_field = %q; want sale_price", target["target_field"])
	}
	if target["regular_price"].(string) != "22.00" {
		t.Fatalf("regular_price (decimal string the dispatcher writes — keyed regular_price for back-compat with the simple shape, even when target_field=sale_price) = %q; want \"22.00\"", target["regular_price"])
	}
	if math.Abs(target["previous_price"].(float64)-20.00) > 0.001 {
		t.Fatalf("previous_price = %v; want 20.00 (sale anchor)", target["previous_price"])
	}
	if target["sale_price_observed"].(string) != "20.00" {
		t.Fatalf("sale_price_observed = %q; want \"20.00\"", target["sale_price_observed"])
	}
	if target["regular_price_observed"].(string) != "25.00" {
		t.Fatalf("regular_price_observed = %q; want \"25.00\"", target["regular_price_observed"])
	}
}
