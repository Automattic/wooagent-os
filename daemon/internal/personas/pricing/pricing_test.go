package pricing

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
)

// The lenient UnmarshalJSON handles three real drifts we've seen from the
// model: `recommendation` instead of `direction`, `price_change_percent`
// instead of `percent_change`, and `rationale` as an array of strings.
// Anything beyond these three is intentionally not in scope.

func TestProposalOut_LenientParse_ModelDriftFromIndigoPillowRun(t *testing.T) {
	// Reproduces the exact drift caught on the May 4 staging run: model
	// emitted recommendation/price_change_percent/rationale-as-array.
	raw := `{
		"product_id": "3664",
		"product_name": "Indigo-Dyed Throw Pillow",
		"current_price": 48.00,
		"recommendation": "increase",
		"proposed_price": 56.00,
		"price_change_amount": 8.00,
		"price_change_percent": 16.67,
		"rationale": [
			"Mid-tier indigo throw pillows at Crate & Barrel, Anthropologie, and Parachute cluster at $59.95-$72.00.",
			"Current $48 trails the band. Recommend a +16.67% step toward the median, capped at the skill's ±25% rule."
		],
		"observed_median": 65.00,
		"observed_low": 59.95,
		"observed_high": 72.00,
		"sources": [
			{"url": "https://crateandbarrel.com/x", "retailer": "Crate & Barrel", "comparable_product": "Indigo Pillow", "observed_price": 59.95},
			{"url": "https://anthropologie.com/x", "retailer": "Anthropologie", "comparable_product": "Indigo Pillow", "observed_price": 68.00},
			{"url": "https://parachutehome.com/x", "retailer": "Parachute", "comparable_product": "Linen Pillow", "observed_price": 72.00}
		]
	}`
	var out proposalOut
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Direction != "increase" {
		t.Errorf("Direction = %q, want %q (recommendation→direction synonym)", out.Direction, "increase")
	}
	if out.PercentChange != 16.67 {
		t.Errorf("PercentChange = %v, want 16.67 (price_change_percent→percent_change synonym)", out.PercentChange)
	}
	if !strings.Contains(out.Rationale, "Mid-tier indigo") || !strings.Contains(out.Rationale, "trails the band") {
		t.Errorf("Rationale array not joined into string; got %q", out.Rationale)
	}
	if !strings.Contains(out.Rationale, "\n\n") {
		t.Errorf("Rationale paragraphs not separated; got %q", out.Rationale)
	}
	if out.ProposedPrice != 56.00 {
		t.Errorf("ProposedPrice = %v, want 56.00", out.ProposedPrice)
	}
	if len(out.Sources) != 3 {
		t.Errorf("Sources len = %d, want 3", len(out.Sources))
	}
	if out.Sources[0].Retailer != "Crate & Barrel" {
		t.Errorf("Sources[0].Retailer = %q, want %q", out.Sources[0].Retailer, "Crate & Barrel")
	}
}

func TestProposalOut_StrictShapeAlsoWorks(t *testing.T) {
	// The shape the prompt actually asks for must still parse cleanly —
	// the lenient layer must never break the canonical path.
	raw := `{
		"no_proposal": false,
		"previous_price": 48.00,
		"proposed_price": 56.00,
		"percent_change": 16.67,
		"direction": "increase",
		"observed_median": 65.00,
		"observed_low": 59.95,
		"observed_high": 72.00,
		"rationale": "Mid-tier comps cluster at $65; current $48 trails the band.",
		"sources": [
			{"url": "https://x", "retailer": "X", "comparable_product": "y", "observed_price": 60.0}
		]
	}`
	var out proposalOut
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal canonical shape: %v", err)
	}
	if out.NoProposal {
		t.Errorf("NoProposal = true, want false")
	}
	if out.Direction != "increase" {
		t.Errorf("Direction = %q, want increase", out.Direction)
	}
	if out.PercentChange != 16.67 {
		t.Errorf("PercentChange = %v, want 16.67", out.PercentChange)
	}
	if out.Rationale != "Mid-tier comps cluster at $65; current $48 trails the band." {
		t.Errorf("Rationale roundtrip wrong: %q", out.Rationale)
	}
}

func TestProposalOut_DeclineShape(t *testing.T) {
	raw := `{
		"no_proposal": true,
		"reason_no_proposal": "Searched 4 retailers; only 1 returned comparable; below the skill's 3-source minimum."
	}`
	var out proposalOut
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal decline: %v", err)
	}
	if !out.NoProposal {
		t.Errorf("NoProposal = false, want true")
	}
	if !strings.Contains(out.ReasonNoProposal, "below the skill's 3-source") {
		t.Errorf("reason_no_proposal lost: %q", out.ReasonNoProposal)
	}
}

func TestExtractJSONObject_PrefixSuffixTolerated(t *testing.T) {
	// The model occasionally wraps JSON in commentary despite being told
	// not to. extractJSONObject pulls the balanced object out.
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "bare object",
			in:   `{"no_proposal": true}`,
			want: `{"no_proposal": true}`,
		},
		{
			name: "prose preamble",
			in:   "Here is the proposal:\n{\"no_proposal\": true}",
			want: `{"no_proposal": true}`,
		},
		{
			name: "trailing commentary",
			in:   `{"no_proposal": true}\n\nLet me know if you have questions.`,
			want: `{"no_proposal": true}`,
		},
		{
			name: "string with brace inside",
			in:   `{"reason": "the {curly} thing"}`,
			want: `{"reason": "the {curly} thing"}`,
		},
		{
			name:  "no object",
			in:    "no JSON here",
			want:  "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractJSONObject(tc.in)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------- bucketing

// bucketTestProduct is a lightweight fixture row for findLargestEligibleBucket
// tests. The adapter below converts it into the real `product` struct,
// matching whatever category shape the helper consumes.
type bucketTestProduct struct {
	id       int
	sku      string
	category string
}

func makeBucketProducts(in []bucketTestProduct) []product {
	out := make([]product, len(in))
	for i, p := range in {
		out[i] = product{ID: p.id, SKU: p.sku}
		if p.category != "" {
			out[i].Categories = []struct {
				Name string `json:"name"`
			}{{Name: p.category}}
		}
	}
	return out
}

func TestFindLargestEligibleBucket_ReturnsBucketWhenAtOrAboveThreshold(t *testing.T) {
	products := makeBucketProducts([]bucketTestProduct{
		{1, "SKU-001", "Home & Textiles"},
		{2, "SKU-002", "Home & Textiles"},
		{3, "SKU-003", "Home & Textiles"},
		{100, "SKU-100", "Apparel"},
		{101, "SKU-101", "Apparel"},
	})
	got := findLargestEligibleBucket(products, 3)
	if got == nil {
		t.Fatalf("expected bucket; got nil")
	}
	if got.Category != "Home & Textiles" {
		t.Errorf("Category=%q, want Home & Textiles", got.Category)
	}
	if len(got.Products) != 3 {
		t.Errorf("len(Products)=%d, want 3", len(got.Products))
	}
}

func TestFindLargestEligibleBucket_NilWhenNoneAtThreshold(t *testing.T) {
	products := makeBucketProducts([]bucketTestProduct{
		{1, "SKU-001", "Home & Textiles"},
		{2, "SKU-002", "Home & Textiles"},
		{100, "SKU-100", "Apparel"},
	})
	if got := findLargestEligibleBucket(products, 3); got != nil {
		t.Errorf("expected nil (no bucket >= 3); got Category=%q size=%d", got.Category, len(got.Products))
	}
}

func TestFindLargestEligibleBucket_PicksLargestOnTie_AlphabeticalBreak(t *testing.T) {
	products := makeBucketProducts([]bucketTestProduct{
		{1, "SKU-001", "Home & Textiles"},
		{2, "SKU-002", "Home & Textiles"},
		{3, "SKU-003", "Home & Textiles"},
		{100, "SKU-100", "Apparel"},
		{101, "SKU-101", "Apparel"},
		{102, "SKU-102", "Apparel"},
	})
	got := findLargestEligibleBucket(products, 3)
	if got == nil {
		t.Fatalf("expected bucket; got nil")
	}
	// Both buckets have 3; deterministic tie-break = alphabetical.
	if got.Category != "Apparel" {
		t.Errorf("Category=%q, want Apparel (alphabetical first)", got.Category)
	}
}

func TestFindLargestEligibleBucket_IgnoresEmptyCategory(t *testing.T) {
	products := makeBucketProducts([]bucketTestProduct{
		{1, "SKU-001", ""},
		{2, "SKU-002", ""},
		{3, "SKU-003", ""},
		{100, "SKU-100", "Apparel"},
	})
	if got := findLargestEligibleBucket(products, 3); got != nil {
		t.Errorf("uncategorized products should never form a batch; got bucket=%q", got.Category)
	}
}

// ---------------------------------------------------------------- packAsBatch

// makeDraft is a helper for test drafts.
func makeDraft(sku string, productID int, category string) personas.Drafted {
	return personas.Drafted{
		Title:           "Price change · " + sku,
		ProposalType:    "product_price_change",
		Priority:        "medium",
		ProposalContent: "rationale",
		Target: map[string]any{
			"product_id":       productID,
			"product_sku":      sku,
			"product_category": category,
			"previous_price":   50.0,
			"proposed_price":   45.0,
			"percent_change":   -10.0,
			"direction":        "decrease",
			"currency":         "USD",
		},
	}
}

func TestPackAsBatch_FillsBatchFields(t *testing.T) {
	drafts := []personas.Drafted{
		makeDraft("SKU-001", 1, "Home & Textiles"),
		makeDraft("SKU-002", 2, "Home & Textiles"),
		makeDraft("SKU-003", 3, "Home & Textiles"),
	}
	packed := packAsBatch(drafts, "Home & Textiles")
	if packed.BatchTitle == "" {
		t.Error("BatchTitle should be set")
	}
	if !strings.Contains(packed.BatchTitle, "Home & Textiles") {
		t.Errorf("BatchTitle should name the category; got %q", packed.BatchTitle)
	}
	if !strings.Contains(packed.BatchTitle, "3 products") {
		t.Errorf("BatchTitle should show product count; got %q", packed.BatchTitle)
	}
	if packed.BatchIntent != "pricing_bulk" {
		t.Errorf("BatchIntent=%q, want pricing_bulk", packed.BatchIntent)
	}
	if len(packed.BatchSiblings) != 2 {
		t.Errorf("BatchSiblings length=%d, want 2 (primary + 2 siblings = 3 total)", len(packed.BatchSiblings))
	}
	if got := packed.Target["product_sku"]; got != "SKU-001" {
		t.Errorf("primary should be the first draft; got product_sku=%v", got)
	}
}

func TestProductJSON_ImageFields(t *testing.T) {
	payload := []byte(`{
		"id": 7,
		"name": "Indigo Pillow",
		"sku": "IND-7",
		"status": "publish",
		"type": "simple",
		"regular_price": "48.00",
		"image_url": "https://store.example.com/wp-content/uploads/2024/01/pillow.jpg",
		"image_alt": "Indigo throw pillow"
	}`)
	var p product
	if err := json.Unmarshal(payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.ImageURL != "https://store.example.com/wp-content/uploads/2024/01/pillow.jpg" {
		t.Errorf("ImageURL = %q", p.ImageURL)
	}
	if p.ImageAlt != "Indigo throw pillow" {
		t.Errorf("ImageAlt = %q", p.ImageAlt)
	}
}

func TestPickAnchorPrice_PrefersSaleWhenPresent(t *testing.T) {
	cases := []struct {
		name      string
		regular   string
		sale      string
		wantValue float64
		wantField string
		wantOK    bool
	}{
		{"only regular", "39.00", "", 39.00, "regular_price", true},
		{"sale active", "39.00", "29.99", 29.99, "sale_price", true},
		{"sale empty string", "39.00", "  ", 39.00, "regular_price", true},
		{"sale zero", "39.00", "0.00", 39.00, "regular_price", true},
		{"sale negative", "39.00", "-1.00", 39.00, "regular_price", true},
		{"no regular, no sale", "", "", 0, "", false},
		{"no regular but sale set — skip", "", "29.99", 0, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := product{RegularPrice: tc.regular, SalePrice: tc.sale}
			value, field, ok := pickAnchorPrice(p)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if value != tc.wantValue {
				t.Errorf("value=%v, want %v", value, tc.wantValue)
			}
			if field != tc.wantField {
				t.Errorf("field=%q, want %q", field, tc.wantField)
			}
		})
	}
}
