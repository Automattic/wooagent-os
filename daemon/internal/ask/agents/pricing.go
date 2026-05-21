package agents

import (
	"strings"

	_ "embed"
)

//go:embed pricing_prompt.md
var pricingPromptTemplate string

// PricingPrompt renders the Pricing chat-mode system prompt for one
// store. Voice paragraph is duplicated verbatim from the cadence-mode
// `pricing-benchmark/v1.yaml` skill prompt — if that voice is ever
// tweaked, this prompt has to be tweaked in lockstep (long-term
// follow-up: factor into a shared resource).
func PricingPrompt(storeName string) string {
	name := strings.TrimSpace(storeName)
	if name == "" {
		name = "the store"
	}
	return strings.ReplaceAll(pricingPromptTemplate, "{{store_name}}", name)
}
