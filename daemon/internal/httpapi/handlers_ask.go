package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/ask"
	"github.com/wooagent-os/wooagent-os/daemon/internal/llm/anthropic"
)

// AskAgent groups the per-agent configuration used by handleAsk. Each
// entry in AskConfig.Agents pairs the system-prompt renderer with the
// concrete tool handlers the LLM is allowed to call for that agent.
// CoS lands first (DSGWOO-1348 task A1); specialists follow under A2.
type AskAgent struct {
	// Prompt renders the system prompt for one chat turn. Receives
	// the connected store's name (may be empty); see CoSPrompt for
	// the canonical signature.
	Prompt func(storeName string) string
	// Tools is the toolbelt the model gets for this agent. Order
	// doesn't affect routing — the model picks by name — but matching
	// Definitions() / Handlers() output keeps registration honest.
	Tools []anthropic.ToolHandler
	// ServerTools is the slot for Anthropic server-managed tools
	// (currently just web_search_20250305 for Pricing). These are
	// merged into the Tools array on the request but never dispatched
	// to a local handler — Anthropic executes them server-side and
	// returns results inline. Keep this empty for agents that don't
	// use server tools.
	ServerTools []anthropic.ToolDef
}

// AskConfig is the bundle of dependencies the /v1/ask handler needs.
// Pass via Server.SetAsk(); when not configured, /v1/ask returns 503.
type AskConfig struct {
	// Client is the shared Anthropic client (one per daemon).
	Client *anthropic.Client
	// Threads is the in-memory per-(operator, agent) message log.
	Threads *ask.ThreadStore
	// Agents enumerates the live chat-mode agents (CoS at minimum;
	// Marketing / Pricing / Sales Support join under A2).
	Agents map[ask.AgentSlug]AskAgent
	// Events is the per-thread pub-sub for mid-flight progress events
	// (DSGWOO-1356). The /v1/ask handler publishes thinking events
	// via LoopOpts.OnToolStart; the SSE subscriber on
	// GET /v1/ask/events?thread_id=<id> reads them. When nil, the
	// handler runs uninstrumented — fine for tests, no SSE endpoint.
	Events *ask.Broker
	// GetStoreName returns the connected store's name (or empty). The
	// handler invokes it on every request so multi-store futures are
	// easy. May be nil — the prompt falls back to "the store".
	GetStoreName func(ctx context.Context) string
	// MaxIterations overrides the anthropic loop cap. 0 means use the
	// client's default (6).
	MaxIterations int
}

// SetAsk wires the /v1/ask handler dependencies. Call after the
// scheduler is set so the dispatch tool's enqueuer points at a live
// queue.
func (s *Server) SetAsk(cfg AskConfig) { s.askCfg = &cfg }

// handleAsk processes POST /v1/ask. Body shape (matches
// daemon/internal/ask types):
//
//	{ agent, thread_id, messages: [{role, content, page_context?}] }
//
// Response:
//
//	{ message: { role, content, references, dispatched? } }
//
// Validation: agent must be one of the configured agents; the latest
// message must be a non-empty user turn. Anything below an HTTP-error
// floor (rate limit, unknown tool, etc.) surfaces inline through the
// model's reply, not as a 4xx.
func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	if s.askCfg == nil {
		writeError(w, http.StatusServiceUnavailable, "ask_unavailable", "ask endpoint not configured")
		return
	}

	var req ask.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json", "decode body: "+err.Error())
		return
	}

	agentCfg, ok := s.askCfg.Agents[req.Agent]
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown_agent", fmt.Sprintf("agent %q is not available", req.Agent))
		return
	}

	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "no_messages", "messages array is empty")
		return
	}
	latest := req.Messages[len(req.Messages)-1]
	if latest.Role != ask.RoleUser || latest.Content == "" {
		writeError(w, http.StatusBadRequest, "no_user_message", "latest message must be a non-empty user turn")
		return
	}

	operator := operatorName(r)

	// Journal the new user message to the thread store. The LLM call
	// uses what the client sent (client is authoritative for model
	// context in v1); the store gives us a server-side log that
	// future persistence work can swap for SQLite without changing
	// callers.
	s.askCfg.Threads.Append(operator, req.Agent, latest)

	// Render the system prompt with the connected store's name.
	var storeName string
	if s.askCfg.GetStoreName != nil {
		storeName = s.askCfg.GetStoreName(r.Context())
	}
	system := agentCfg.Prompt(storeName)

	tools := anthropic.Definitions(agentCfg.Tools...)
	if len(agentCfg.ServerTools) > 0 {
		tools = append(tools, agentCfg.ServerTools...)
	}
	handlers := anthropic.Handlers(agentCfg.Tools...)

	loopOpts := anthropic.LoopOpts{MaxIterations: s.askCfg.MaxIterations}
	if s.askCfg.Events != nil && req.ThreadID != "" {
		// Mid-flight progress (DSGWOO-1356). Emit a thinking event the
		// first moment a slow tool fires so the UI can render the
		// transient block well before the model returns its final
		// text. Only slow tools qualify — fast reads would just churn
		// the SSE stream with noise.
		threadID := req.ThreadID
		agent := string(req.Agent)
		broker := s.askCfg.Events
		loopOpts.OnToolStart = func(name string, _ json.RawMessage) {
			if !ask.SlowTools[name] {
				return
			}
			broker.Publish(threadID, ask.Event{
				Kind:    "thinking",
				Agent:   agent,
				Tool:    name,
				Message: ask.ThinkingMessageFor(agent, name),
			})
		}
	}

	start := time.Now()
	final, trace, loopErr := s.askCfg.Client.RunToolLoop(r.Context(),
		anthropic.Request{
			System:   system,
			Messages: buildAnthropicMessages(req.Messages),
			Tools:    tools,
		},
		handlers,
		loopOpts,
	)
	latency := time.Since(start)

	// Iteration-exhaustion is a soft failure — we still have a
	// partial final message to surface. Any other error is hard.
	exhausted := errors.Is(loopErr, anthropic.ErrToolLoopExhausted)
	if loopErr != nil && !exhausted {
		slog.Error("ask: llm call failed",
			"operator", operator, "agent", req.Agent,
			"thread_id", req.ThreadID, "latency_ms", latency.Milliseconds(),
			"err", loopErr)
		writeError(w, http.StatusBadGateway, "llm_error", loopErr.Error())
		return
	}

	finalText := assistantText(final.Content)
	refs, dispatched := ask.ExtractReferencesAndDispatched(finalText, trace)

	assistant := ask.Message{
		Role:       ask.RoleAssistant,
		Content:    finalText,
		References: refs,
		Dispatched: dispatched,
	}
	s.askCfg.Threads.Append(operator, req.Agent, assistant)

	slog.Info("ask: handled",
		"operator", operator, "agent", req.Agent,
		"thread_id", req.ThreadID,
		"iterations", trace.Iterations,
		"input_tokens", trace.Usage.InputTokens,
		"output_tokens", trace.Usage.OutputTokens,
		"refs", len(refs), "dispatched", len(dispatched),
		"latency_ms", latency.Milliseconds(),
		"exhausted", exhausted,
	)

	writeJSON(w, http.StatusOK, ask.Response{Message: assistant})
}

// buildAnthropicMessages translates ask.Messages to anthropic.Messages.
// User turns that carry a non-empty page_context get it appended as a
// fenced JSON block at the end of the text — same turn, separable by
// the model. Assistant references/dispatched are UI metadata and
// don't go back to the model.
func buildAnthropicMessages(msgs []ask.Message) []anthropic.Message {
	out := make([]anthropic.Message, 0, len(msgs))
	for _, m := range msgs {
		text := m.Content
		if m.Role == ask.RoleUser && m.PageContext != nil &&
			(m.PageContext.Page != "" || len(m.PageContext.VisibleItems) > 0) {
			ctx, _ := json.Marshal(m.PageContext)
			text = m.Content + "\n\n<page_context>\n" + string(ctx) + "\n</page_context>"
		}
		role := "user"
		if m.Role == ask.RoleAssistant {
			role = "assistant"
		}
		out = append(out, anthropic.Message{
			Role:    role,
			Content: []anthropic.ContentBlock{anthropic.TextBlock(text)},
		})
	}
	return out
}

// assistantText extracts the model's prose from the final assistant
// turn. The loop runs until the model produces no tool_use blocks, so
// in practice only text blocks land here — but be defensive in case
// future loop changes leave something else through.
func assistantText(content []anthropic.ContentBlock) string {
	var s string
	for _, b := range content {
		if b.Type == "text" {
			s += b.Text
		}
	}
	return s
}
