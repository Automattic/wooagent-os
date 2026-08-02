// Package llm centralizes shared infrastructure for LLM-provider calls.
//
// Two responsibilities today:
//
//   - Cost: mapping a (provider, model, tokens) tuple into a USD figure so
//     the per-persona daily budget gate and the per-run budget gate can do
//     their jobs. See pricing.go.
//   - Typed errors: provider-neutral failure classes (ErrRateLimited,
//     ErrAuth, ErrInvalidRequest, ErrServer) that callers match with
//     errors.Is instead of pattern-matching message text. See errors.go.
//
// The Anthropic wire client lives in the llm/anthropic subpackage and
// returns these errors.
package llm

import (
	"log/slog"
	"strings"
	"sync"
)

// modelRate is per-million-token pricing in USD for one model. Input/Output
// are kept separate because every modern model bills them differently.
type modelRate struct {
	InputUSDPerM  float64
	OutputUSDPerM float64
}

// rates is the canonical price book. Numbers are public-list per
// anthropic.com/pricing as of late 2025 / early 2026. The keys are the
// model identifiers personas pass to the API and capture into
// telemetry.ModelCall.Model — match that string exactly.
//
// Adding a model: append a row. Do NOT remove rows for retired models;
// historical turn_events still reference them and the GEPA pipeline
// expects the cost back when it re-ingests.
var rates = map[string]modelRate{
	// Anthropic — Claude 4.x family.
	"claude-opus-4-7":     {InputUSDPerM: 15.00, OutputUSDPerM: 75.00},
	"claude-opus-4-6":     {InputUSDPerM: 15.00, OutputUSDPerM: 75.00},
	"claude-sonnet-4-6":   {InputUSDPerM: 3.00, OutputUSDPerM: 15.00},
	"claude-sonnet-4-5":   {InputUSDPerM: 3.00, OutputUSDPerM: 15.00},
	"claude-haiku-4-5":    {InputUSDPerM: 1.00, OutputUSDPerM: 5.00},
	"claude-haiku-4-5-20251001": {InputUSDPerM: 1.00, OutputUSDPerM: 5.00},
}

// unknownLogged remembers (provider, model) tuples we've already warned
// about — one Warn line per missing rate, not one per call.
var (
	unknownMu      sync.Mutex
	unknownLogged  = map[string]struct{}{}
)

// CostUSD returns the dollar cost of a single model call given the
// provider, model identifier, and token counts pulled from the provider's
// response. Unknown (provider, model) pairs return 0 and emit a single
// Warn log so the operator can spot the gap; the call still proceeds —
// the budget gates fail open rather than punishing operators because we
// forgot to seed a price.
//
// Local model providers (LM Studio etc.) are intentionally absent from
// the rate book: they cost $0 from the operator's perspective. The
// unknown-pair Warn fires for them too but that's correct — if a user
// configures Marketing to fall through to a non-LM-Studio OpenAI model,
// we'd want the operator to know we're under-counting.
func CostUSD(provider, model string, inputTokens, outputTokens int) float64 {
	if provider == "" || model == "" {
		return 0
	}
	rate, ok := rates[model]
	if !ok {
		logUnknownOnce(provider, model)
		return 0
	}
	in := float64(inputTokens) * rate.InputUSDPerM / 1_000_000.0
	out := float64(outputTokens) * rate.OutputUSDPerM / 1_000_000.0
	return in + out
}

func logUnknownOnce(provider, model string) {
	key := provider + ":" + model
	unknownMu.Lock()
	defer unknownMu.Unlock()
	if _, seen := unknownLogged[key]; seen {
		return
	}
	unknownLogged[key] = struct{}{}
	// Local providers shouldn't pollute the log; suppress LM Studio's
	// well-known model-name prefixes.
	if strings.HasPrefix(strings.ToLower(model), "lmstudio") ||
		strings.HasPrefix(strings.ToLower(model), "local") {
		return
	}
	slog.Default().Warn("llm: no pricing entry; cost reported as $0",
		"provider", provider,
		"model", model,
	)
}
