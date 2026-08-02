// Package anthropic is a shared client for Anthropic's Messages API with
// tool-use support. It centralizes the request/response wire types, the
// HTTP call, and the local tool-execution loop that the Ask Agent drawer
// (DSGWOO-1348) and — eventually — the cadence-mode personas all need.
//
// The persona-level call sites today inline their own anthropicReq /
// anthropicResp types and POST to the API directly. This package is the
// "shared HTTP / retry / typed-error scaffolding" the parent `llm`
// package's doc promises (DSGWOO-1296 follow-ups). Migration of the
// existing personas is out of scope here; they keep working unchanged.
package anthropic

import "encoding/json"

const (
	// APIURL is the canonical Anthropic Messages endpoint.
	APIURL = "https://api.anthropic.com/v1/messages"
	// Version is the Anthropic API version we target. Matches the value
	// already in use by the cadence-mode personas.
	Version = "2023-06-01"
	// Provider is the short identifier used for cost accounting
	// (llm.CostUSD), telemetry.ModelCall.Provider, and llm.APIStatusError.
	Provider = "anthropic"
)

// Message is one turn in a conversation.
type Message struct {
	Role    string         `json:"role"` // "user" | "assistant"
	Content []ContentBlock `json:"content"`
}

// ContentBlock is one block within a Message's content. The Type
// discriminator decides which fields are meaningful:
//
//   - "text"        → Text
//   - "tool_use"    → ID, Name, Input
//   - "tool_result" → ToolUseID, ToolResultContent, IsError
//
// Unused fields stay empty and are omitted on the wire via omitempty.
type ContentBlock struct {
	Type string `json:"type"`

	// text
	Text string `json:"text,omitempty"`

	// tool_use (assistant emits)
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result (user supplies in response to tool_use)
	ToolUseID         string `json:"tool_use_id,omitempty"`
	ToolResultContent string `json:"content,omitempty"`
	IsError           bool   `json:"is_error,omitempty"`
}

// TextBlock is a convenience constructor for a plain-text content block.
func TextBlock(text string) ContentBlock {
	return ContentBlock{Type: "text", Text: text}
}

// ToolResultBlock builds a tool_result content block. Pass isError=true
// to mark a failure — the model will see that flag and recover instead
// of treating the result as canonical output.
func ToolResultBlock(toolUseID, content string, isError bool) ContentBlock {
	return ContentBlock{
		Type:              "tool_result",
		ToolUseID:         toolUseID,
		ToolResultContent: content,
		IsError:           isError,
	}
}

// UserMessage is a convenience constructor for a plain-text user turn.
func UserMessage(text string) Message {
	return Message{Role: "user", Content: []ContentBlock{TextBlock(text)}}
}

// AssistantMessage is a convenience constructor for an assistant turn
// composed of arbitrary content blocks (text + tool_use + ...).
func AssistantMessage(blocks ...ContentBlock) Message {
	return Message{Role: "assistant", Content: blocks}
}

// Request is the POST body for /v1/messages.
type Request struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []Message `json:"messages"`
	Tools     []ToolDef `json:"tools,omitempty"`
}

// ToolDef is one tool definition exposed to the model. For local tools
// (handled by ToolHandler), Type is empty (standard Anthropic-managed
// shape) and InputSchema is required. For server-managed tools like
// web_search, set Type to the server-tool identifier (e.g.
// "web_search_20250305"), Name to its canonical name, and leave
// InputSchema empty.
type ToolDef struct {
	Type        string          `json:"type,omitempty"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
	MaxUses     int             `json:"max_uses,omitempty"`
}

// Response is the parsed JSON body from a successful /v1/messages call.
type Response struct {
	ID         string         `json:"id,omitempty"`
	Type       string         `json:"type,omitempty"`
	Role       string         `json:"role,omitempty"`
	Model      string         `json:"model,omitempty"`
	Content    []ContentBlock `json:"content"`
	StopReason string         `json:"stop_reason,omitempty"`
	Usage      Usage          `json:"usage,omitempty"`
	Error      APIError       `json:"error,omitempty"`
}

// Usage is token accounting from one model response. Accumulated across
// every Call inside a RunToolLoop and surfaced on Trace.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// APIError is the structured error Anthropic returns inside a 200
// response body when something went wrong server-side. (HTTP 4xx/5xx
// responses produce a Go error in Client.Call directly — APIError is
// for the rarer "200 OK with embedded error" case.)
type APIError struct {
	Type    string `json:"type,omitempty"`
	Message string `json:"message,omitempty"`
}
