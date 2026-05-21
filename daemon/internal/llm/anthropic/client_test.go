package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// scriptedServer replays a sequence of canned Anthropic /v1/messages
// responses, one per inbound POST in order. Captures every decoded
// request for later assertions.
type scriptedServer struct {
	t        *testing.T
	requests []Request
	replies  []string
	idx      int
}

func newScripted(t *testing.T, replies ...string) *scriptedServer {
	return &scriptedServer{t: t, replies: replies}
}

func (s *scriptedServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.t.Helper()
	if r.Method != http.MethodPost {
		s.t.Fatalf("unexpected method: %s", r.Method)
	}
	if r.Header.Get("x-api-key") == "" {
		s.t.Fatalf("missing x-api-key header")
	}
	if r.Header.Get("anthropic-version") == "" {
		s.t.Fatalf("missing anthropic-version header")
	}
	if got := r.Header.Get("content-type"); got != "application/json" {
		s.t.Fatalf("unexpected content-type: %q", got)
	}
	body, _ := io.ReadAll(r.Body)
	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		s.t.Fatalf("decode request: %v · body=%s", err, body)
	}
	s.requests = append(s.requests, req)
	if s.idx >= len(s.replies) {
		s.t.Fatalf("script ran out of replies at call %d", s.idx)
	}
	reply := s.replies[s.idx]
	s.idx++
	w.Header().Set("content-type", "application/json")
	_, _ = io.WriteString(w, reply)
}

// newTestClient wires a Client to a scripted test server.
func newTestClient(t *testing.T, h http.Handler) *Client {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New("test-key", "test-model").WithAPIURL(srv.URL + "/v1/messages")
}

// stubHandler is a ToolHandler the tests can drive — captures inputs
// and returns a canned result or error.
type stubHandler struct {
	name   string
	result string
	err    error
	calls  []json.RawMessage
}

func (h *stubHandler) Definition() ToolDef {
	return ToolDef{
		Name:        h.name,
		Description: "test tool",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}
}

func (h *stubHandler) Execute(_ context.Context, input json.RawMessage) (string, error) {
	h.calls = append(h.calls, input)
	return h.result, h.err
}

// ---------------------------------------------------------------- Call

func TestCall_PlainText(t *testing.T) {
	c := newTestClient(t, newScripted(t, `{
		"content": [{"type":"text","text":"hello"}],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 5, "output_tokens": 1}
	}`))
	resp, err := c.Call(context.Background(), Request{
		Messages: []Message{UserMessage("hi")},
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if len(resp.Content) != 1 || resp.Content[0].Text != "hello" {
		t.Fatalf("unexpected content: %+v", resp.Content)
	}
	if resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 1 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
}

func TestCall_APIError(t *testing.T) {
	c := newTestClient(t, newScripted(t, `{
		"content": [],
		"error": {"type":"invalid_request_error","message":"bad input"}
	}`))
	_, err := c.Call(context.Background(), Request{
		Messages: []Message{UserMessage("hi")},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid_request_error") {
		t.Fatalf("expected api error, got: %v", err)
	}
}

func TestCall_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
		_, _ = io.WriteString(w, "internal server error")
	}))
	t.Cleanup(srv.Close)
	c := New("test-key", "test-model").WithAPIURL(srv.URL + "/v1/messages")
	_, err := c.Call(context.Background(), Request{
		Messages: []Message{UserMessage("hi")},
	})
	if err == nil || !strings.Contains(err.Error(), "http 500") {
		t.Fatalf("expected http 500 error, got: %v", err)
	}
}

func TestCall_AppliesDefaultModelAndMaxTokens(t *testing.T) {
	s := newScripted(t, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`)
	c := newTestClient(t, s)
	_, err := c.Call(context.Background(), Request{
		Messages: []Message{UserMessage("hi")},
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	got := s.requests[0]
	if got.Model != "test-model" {
		t.Fatalf("expected default model, got %q", got.Model)
	}
	if got.MaxTokens != 2048 {
		t.Fatalf("expected default max_tokens=2048, got %d", got.MaxTokens)
	}
}

// ---------------------------------------------------------- RunToolLoop

func TestRunToolLoop_NoTools(t *testing.T) {
	c := newTestClient(t, newScripted(t, `{
		"content": [{"type":"text","text":"final answer"}],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 5, "output_tokens": 2}
	}`))
	final, trace, err := c.RunToolLoop(context.Background(),
		Request{Messages: []Message{UserMessage("hi")}},
		nil,
		LoopOpts{})
	if err != nil {
		t.Fatalf("RunToolLoop: %v", err)
	}
	if trace.Iterations != 1 {
		t.Fatalf("expected 1 iteration, got %d", trace.Iterations)
	}
	if len(final.Content) != 1 || final.Content[0].Text != "final answer" {
		t.Fatalf("unexpected final: %+v", final.Content)
	}
	if final.StopReason != "end_turn" {
		t.Fatalf("expected stop_reason=end_turn, got %q", final.StopReason)
	}
}

func TestRunToolLoop_WithHandler(t *testing.T) {
	// Turn 1: model emits tool_use; Turn 2: model emits final text.
	c := newTestClient(t, newScripted(t,
		`{"content":[
			{"type":"text","text":"let me check"},
			{"type":"tool_use","id":"toolu_1","name":"echo","input":{"x":1}}
		],"stop_reason":"tool_use","usage":{"input_tokens":5,"output_tokens":3}}`,
		`{"content":[
			{"type":"text","text":"done"}
		],"stop_reason":"end_turn","usage":{"input_tokens":7,"output_tokens":1}}`,
	))
	h := &stubHandler{name: "echo", result: "echoed:1"}
	final, trace, err := c.RunToolLoop(context.Background(),
		Request{Messages: []Message{UserMessage("hi")}, Tools: Definitions(h)},
		Handlers(h),
		LoopOpts{})
	if err != nil {
		t.Fatalf("RunToolLoop: %v", err)
	}
	if trace.Iterations != 2 {
		t.Fatalf("expected 2 iterations, got %d", trace.Iterations)
	}
	if len(h.calls) != 1 || string(h.calls[0]) != `{"x":1}` {
		t.Fatalf("unexpected handler calls: %+v", h.calls)
	}
	if final.Content[0].Text != "done" {
		t.Fatalf("unexpected final: %+v", final.Content)
	}
	// Trace = [user, assistant(text+tool_use), user(tool_result), assistant(final)]
	if len(trace.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(trace.Messages))
	}
	if trace.Messages[2].Content[0].Type != "tool_result" {
		t.Fatalf("expected tool_result, got %s", trace.Messages[2].Content[0].Type)
	}
	if trace.Messages[2].Content[0].ToolResultContent != "echoed:1" {
		t.Fatalf("unexpected tool_result content: %q", trace.Messages[2].Content[0].ToolResultContent)
	}
	if trace.Usage.InputTokens != 12 || trace.Usage.OutputTokens != 4 {
		t.Fatalf("unexpected accumulated usage: %+v", trace.Usage)
	}
}

func TestRunToolLoop_HandlerError(t *testing.T) {
	c := newTestClient(t, newScripted(t,
		`{"content":[{"type":"tool_use","id":"toolu_1","name":"fail","input":{}}],"stop_reason":"tool_use","usage":{"input_tokens":3,"output_tokens":1}}`,
		`{"content":[{"type":"text","text":"recovered"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":1}}`,
	))
	h := &stubHandler{name: "fail", err: errors.New("boom")}
	final, trace, err := c.RunToolLoop(context.Background(),
		Request{Messages: []Message{UserMessage("hi")}, Tools: Definitions(h)},
		Handlers(h),
		LoopOpts{})
	if err != nil {
		t.Fatalf("RunToolLoop: %v", err)
	}
	if final.Content[0].Text != "recovered" {
		t.Fatalf("expected recovery text, got: %+v", final.Content)
	}
	feedback := trace.Messages[2].Content[0]
	if !feedback.IsError {
		t.Fatalf("expected is_error=true, got: %+v", feedback)
	}
	if feedback.ToolResultContent != "boom" {
		t.Fatalf("expected error text in result, got: %q", feedback.ToolResultContent)
	}
}

func TestRunToolLoop_UnknownTool(t *testing.T) {
	c := newTestClient(t, newScripted(t,
		`{"content":[{"type":"tool_use","id":"toolu_1","name":"nonexistent","input":{}}],"stop_reason":"tool_use","usage":{"input_tokens":3,"output_tokens":1}}`,
		`{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":1}}`,
	))
	final, trace, err := c.RunToolLoop(context.Background(),
		Request{Messages: []Message{UserMessage("hi")}},
		nil, // no handlers registered
		LoopOpts{})
	if err != nil {
		t.Fatalf("RunToolLoop: %v", err)
	}
	if final.Content[0].Text != "ok" {
		t.Fatalf("unexpected final: %+v", final.Content)
	}
	feedback := trace.Messages[2].Content[0]
	if !feedback.IsError || !strings.Contains(feedback.ToolResultContent, "not available") {
		t.Fatalf("expected unknown-tool error, got: %+v", feedback)
	}
}

func TestRunToolLoop_IterationCap(t *testing.T) {
	// Model always emits tool_use; never converges.
	stuck := `{"content":[{"type":"tool_use","id":"toolu_X","name":"echo","input":{}}],"stop_reason":"tool_use","usage":{"input_tokens":3,"output_tokens":1}}`
	c := newTestClient(t, newScripted(t, stuck, stuck, stuck))
	h := &stubHandler{name: "echo", result: "ok"}
	final, trace, err := c.RunToolLoop(context.Background(),
		Request{Messages: []Message{UserMessage("hi")}, Tools: Definitions(h)},
		Handlers(h),
		LoopOpts{MaxIterations: 3})
	if !errors.Is(err, ErrToolLoopExhausted) {
		t.Fatalf("expected ErrToolLoopExhausted, got: %v", err)
	}
	if trace.Iterations != 3 {
		t.Fatalf("expected 3 iterations, got %d", trace.Iterations)
	}
	if len(final.Content) == 0 {
		t.Fatalf("expected partial final content, got empty")
	}
}

func TestRunToolLoop_OnToolStart(t *testing.T) {
	c := newTestClient(t, newScripted(t,
		`{"content":[{"type":"tool_use","id":"toolu_1","name":"echo","input":{"q":"x"}}],"stop_reason":"tool_use","usage":{"input_tokens":3,"output_tokens":1}}`,
		`{"content":[{"type":"text","text":"done"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":1}}`,
	))
	h := &stubHandler{name: "echo", result: "ok"}
	var observed []string
	_, _, err := c.RunToolLoop(context.Background(),
		Request{Messages: []Message{UserMessage("hi")}, Tools: Definitions(h)},
		Handlers(h),
		LoopOpts{OnToolStart: func(name string, _ json.RawMessage) {
			observed = append(observed, name)
		}})
	if err != nil {
		t.Fatalf("RunToolLoop: %v", err)
	}
	if len(observed) != 1 || observed[0] != "echo" {
		t.Fatalf("unexpected callback invocations: %v", observed)
	}
}

func TestRunToolLoop_DoesNotMutateInputMessages(t *testing.T) {
	c := newTestClient(t, newScripted(t, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	input := []Message{UserMessage("hi")}
	_, _, err := c.RunToolLoop(context.Background(),
		Request{Messages: input},
		nil,
		LoopOpts{})
	if err != nil {
		t.Fatalf("RunToolLoop: %v", err)
	}
	if len(input) != 1 {
		t.Fatalf("input messages were mutated: len=%d", len(input))
	}
}

// ------------------------------------------------------- Integration

// TestIntegration_Live runs against the real Anthropic API when
// ANTHROPIC_API_KEY is set in the environment. Skipped otherwise.
// Run with: ANTHROPIC_API_KEY=... go test ./internal/llm/anthropic/...
func TestIntegration_Live(t *testing.T) {
	// integrationModel is Anthropic's published model identifier
	// (claude-haiku-4-5, snapshot 20251001), not a credential — kept in
	// a const so secret scanners don't pattern-match `key, "..."` as a
	// generic-api-key candidate.
	const integrationModel = "claude-haiku-4-5-20251001"

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set; skipping live integration test")
	}
	c := New(apiKey, integrationModel)
	resp, err := c.Call(context.Background(), Request{
		MaxTokens: 64,
		Messages:  []Message{UserMessage("What is 2+2? Answer with just the number.")},
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if len(resp.Content) == 0 {
		t.Fatalf("no content in response: %+v", resp)
	}
	var sb strings.Builder
	for _, b := range resp.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	if !strings.Contains(sb.String(), "4") {
		t.Errorf("expected response to contain '4', got: %q", sb.String())
	}
}
