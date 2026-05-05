package pricing

import (
	"encoding/json"
	"strings"
	"testing"
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
