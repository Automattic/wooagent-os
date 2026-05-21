// Package agents holds the per-agent system prompts and tool surfaces
// that the /v1/ask endpoint composes into Anthropic Messages requests.
// Chief of Staff (DSGWOO-1348 task A1) is the first agent landed here;
// the three live specialists follow under task A2.
package agents

import (
	"strings"

	_ "embed"
)

//go:embed cos_prompt.md
var cosPromptTemplate string

// CoSPrompt renders the Chief of Staff system prompt for one store.
// The prompt template ships with a literal `{{store_name}}` token that
// gets replaced at call time; if storeName is empty the substitution
// drops to the literal "the store" so the prompt still parses cleanly.
func CoSPrompt(storeName string) string {
	name := strings.TrimSpace(storeName)
	if name == "" {
		name = "the store"
	}
	return strings.ReplaceAll(cosPromptTemplate, "{{store_name}}", name)
}
