package anthropic

import (
	"context"
	"encoding/json"
)

// ToolHandler executes one locally-registered tool. Client.RunToolLoop
// dispatches each tool_use block from the model to the handler whose
// Definition().Name matches, then feeds the handler's return value back
// to the model as a tool_result block on the next turn.
//
// Handler semantics:
//
//   - Return (result, nil) on success. result is the text that goes
//     back to the model in a tool_result block — usually JSON so the
//     model can parse it cleanly.
//
//   - Return ("", err) when the tool input is malformed or the
//     underlying operation fails. RunToolLoop wraps err.Error() into a
//     tool_result with is_error=true so the model can recover and
//     either retry or explain the failure to the user.
//
// Handlers must be safe for concurrent use — a single Client + handler
// map may be shared across in-flight /v1/ask requests.
type ToolHandler interface {
	Definition() ToolDef
	Execute(ctx context.Context, input json.RawMessage) (string, error)
}

// Handlers builds a name→handler lookup for fast dispatch inside the
// tool-use loop. Later registrations win if names collide; that's a
// caller error, not something Handlers tries to detect — keep names
// unique at registration time.
func Handlers(hs ...ToolHandler) map[string]ToolHandler {
	out := make(map[string]ToolHandler, len(hs))
	for _, h := range hs {
		out[h.Definition().Name] = h
	}
	return out
}

// Definitions returns each handler's ToolDef in registration order.
// Pass the result to Request.Tools so the model knows what's available.
func Definitions(hs ...ToolHandler) []ToolDef {
	out := make([]ToolDef, 0, len(hs))
	for _, h := range hs {
		out = append(out, h.Definition())
	}
	return out
}
