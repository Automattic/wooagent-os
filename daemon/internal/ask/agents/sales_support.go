package agents

import (
	"strings"

	_ "embed"
)

//go:embed sales_support_prompt.md
var salesSupportPromptTemplate string

// SalesSupportPrompt renders the Sales Support chat-mode system prompt
// for one store. Voice paragraph is duplicated verbatim from the
// cadence-mode `daemon/internal/personas/sales-support/sales_support.go`
// system prompt — if that voice is ever tweaked, this prompt has to be
// tweaked in lockstep (long-term follow-up: factor into a shared
// resource).
func SalesSupportPrompt(storeName string) string {
	name := strings.TrimSpace(storeName)
	if name == "" {
		name = "the store"
	}
	return strings.ReplaceAll(salesSupportPromptTemplate, "{{store_name}}", name)
}
