package llm

import (
	"math"
	"testing"
)

func TestCostUSD_Anthropic(t *testing.T) {
	tests := []struct {
		name   string
		model  string
		in     int
		out    int
		wantUSD float64
	}{
		{
			name:    "opus 4.7 standard call",
			model:   "claude-opus-4-7",
			in:      1_000_000, out: 1_000_000,
			wantUSD: 15.00 + 75.00,
		},
		{
			name:    "sonnet 4.6 small call",
			model:   "claude-sonnet-4-6",
			in:      10_000, out: 1_000,
			wantUSD: (10_000 * 3.00 / 1_000_000) + (1_000 * 15.00 / 1_000_000),
		},
		{
			name:    "haiku 4.5 dated model id resolves",
			model:   "claude-haiku-4-5-20251001",
			in:      0, out: 0,
			wantUSD: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CostUSD("anthropic", tt.model, tt.in, tt.out)
			if math.Abs(got-tt.wantUSD) > 1e-9 {
				t.Errorf("CostUSD = %v, want %v", got, tt.wantUSD)
			}
		})
	}
}

func TestCostUSD_UnknownModelReturnsZero(t *testing.T) {
	got := CostUSD("anthropic", "claude-not-a-model", 1_000_000, 1_000_000)
	if got != 0 {
		t.Errorf("expected unknown model to bill $0, got %v", got)
	}
}

func TestCostUSD_EmptyProviderOrModelReturnsZero(t *testing.T) {
	if got := CostUSD("", "claude-opus-4-7", 100, 100); got != 0 {
		t.Errorf("empty provider should cost 0, got %v", got)
	}
	if got := CostUSD("anthropic", "", 100, 100); got != 0 {
		t.Errorf("empty model should cost 0, got %v", got)
	}
}

// Sanity: a single Marketing-sized call (~6k input / ~400 output on Sonnet)
// costs cents-not-dollars. Guards against future rate-table edits accidentally
// landing in cents-per-token instead of dollars-per-million-tokens.
func TestCostUSD_MarketingCallStaysCheap(t *testing.T) {
	got := CostUSD("anthropic", "claude-sonnet-4-6", 6_000, 400)
	if got > 0.05 {
		t.Errorf("a typical Marketing call should cost < $0.05; got $%v", got)
	}
	if got <= 0 {
		t.Errorf("a typical Marketing call should cost > $0; got $%v", got)
	}
}
