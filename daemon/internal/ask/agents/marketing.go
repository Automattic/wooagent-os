package agents

import (
	"strings"

	_ "embed"
)

//go:embed marketing_prompt.md
var marketingPromptTemplate string

// MarketingPrompt renders the Marketing chat-mode system prompt for one
// store. Voice paragraph is duplicated verbatim from the cadence-mode
// `marketing-description-rewrite/v1.yaml` skill prompt — if that voice
// is ever tweaked, this prompt has to be tweaked in lockstep (long-term
// follow-up: factor into a shared resource).
func MarketingPrompt(storeName string) string {
	name := strings.TrimSpace(storeName)
	if name == "" {
		name = "the store"
	}
	return strings.ReplaceAll(marketingPromptTemplate, "{{store_name}}", name)
}
