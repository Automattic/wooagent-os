// Package ask is the orchestration layer for the Ask Agent drawer
// (DSGWOO-1348). Sub-packages own the per-agent prompts (`ask/agents`)
// and the tool handlers (`ask/tools`); this package holds the wire
// types, in-memory thread store, and the reference / dispatched
// extraction logic the /v1/ask handler composes them with.
package ask

// Role is the author of one Message. Mirrors the Anthropic Messages
// role enum (user / assistant) — tool turns get folded into user turns
// by the LLM client, so callers don't construct them directly.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// AgentSlug is one of the chat-mode agents the drawer can address.
// `chief_of_staff` is the only one wired in Phase-1 work (A1+A5);
// the three specialists join under A2.
type AgentSlug string

const (
	AgentChiefOfStaff AgentSlug = "chief_of_staff"
	AgentMarketing    AgentSlug = "marketing"
	AgentPricing      AgentSlug = "pricing"
	AgentSalesSupport AgentSlug = "sales-support"
)

// VisibleItem mirrors the UI's VisibleItem (see ui/src/lib/askAgent.tsx).
// One of these per item on screen, with enough metadata that the agent
// can resolve "this one"-style references without a tool round-trip.
type VisibleItem struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Kind       string `json:"kind"` // proposal | run | agent | product | order
	Persona    string `json:"persona,omitempty"`
	State      string `json:"state,omitempty"`
	AgeSeconds int64  `json:"age_seconds,omitempty"`
}

// PageContext is the snapshot of the operator's current view at submit
// time. Attached to every user message.
type PageContext struct {
	Page         string        `json:"page"`
	VisibleItems []VisibleItem `json:"visible_items,omitempty"`
	StoreID      string        `json:"store_id,omitempty"`
}

// Reference is one structured chip the UI renders below an assistant
// message. Built by the handler from the model's text + the tool
// results that produced the referenced records.
type Reference struct {
	Kind  string `json:"kind"` // proposal | run
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
	State string `json:"state,omitempty"`
}

// Dispatched is the receipt for a successful dispatch_persona call.
// Surfaced on the assistant message so the UI can show a "Working"
// chip pointing at the freshly-enqueued run.
type Dispatched struct {
	Persona    string `json:"persona"`
	RunID      string `json:"run_id"`
	ETASeconds int    `json:"eta_seconds"`
}

// Message is one turn in a chat thread. The wire shape is identical
// for what the client sends (user) and what the server returns
// (assistant); the role determines which fields are meaningful.
type Message struct {
	Role        Role         `json:"role"`
	Content     string       `json:"content"`
	PageContext *PageContext `json:"page_context,omitempty"`
	References  []Reference  `json:"references,omitempty"`
	Dispatched  []Dispatched `json:"dispatched,omitempty"`
}

// Request is the POST /v1/ask body.
type Request struct {
	Agent    AgentSlug `json:"agent"`
	ThreadID string    `json:"thread_id"`
	Messages []Message `json:"messages"`
}

// Response is the POST /v1/ask reply.
type Response struct {
	Message Message `json:"message"`
}
