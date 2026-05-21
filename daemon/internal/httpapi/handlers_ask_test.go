package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wooagent-os/wooagent-os/daemon/internal/ask"
	"github.com/wooagent-os/wooagent-os/daemon/internal/ask/tools"
	"github.com/wooagent-os/wooagent-os/daemon/internal/llm/anthropic"
	"github.com/wooagent-os/wooagent-os/daemon/internal/scheduler"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

// askTestHarness wires a minimal Server + scripted Anthropic backend
// for handler-level testing. Bypasses the bearer-auth middleware (we
// inject the operator name directly into request context) so the
// tests focus on /v1/ask semantics, not auth plumbing.
type askTestHarness struct {
	t       *testing.T
	server  *Server
	replies []string
	idx     int
	calls   int
}

func newAskHarness(t *testing.T, replies ...string) *askTestHarness {
	t.Helper()
	h := &askTestHarness{t: t, replies: replies}

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.calls++
		if h.idx >= len(h.replies) {
			t.Fatalf("ask harness: ran out of scripted replies at call %d", h.idx)
		}
		reply := h.replies[h.idx]
		h.idx++
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(mock.Close)

	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.DB.Close() })

	// Stub enqueuer so dispatch tool calls don't need a live scheduler.
	enq := func(_ context.Context, persona string) (scheduler.Run, error) {
		return scheduler.Run{ID: "rn_test_" + persona, Persona: persona, Trigger: scheduler.TriggerOperatorAsked}, nil
	}

	client := anthropic.New("test-key", "test-model").
		WithAPIURL(mock.URL + "/v1/messages")

	srv := &Server{store: st}
	srv.SetAsk(AskConfig{
		Client:  client,
		Threads: ask.NewThreadStore(0),
		Agents: map[ask.AgentSlug]AskAgent{
			ask.AgentChiefOfStaff: {
				Prompt: func(storeName string) string { return "you are CoS at " + storeName },
				Tools: []anthropic.ToolHandler{
					&tools.ListProposalsTool{DB: st.DB},
					&tools.GetProposalTool{DB: st.DB},
					&tools.DispatchTool{Enqueue: enq, Limiter: tools.DefaultRateLimiter()},
				},
			},
		},
		GetStoreName: func(_ context.Context) string { return "Linenly" },
	})

	h.server = srv
	return h
}

// post runs one handleAsk request through the harness. operatorName is
// set on the context so the handler sees an authenticated caller; the
// real bearer-auth middleware is skipped.
func (h *askTestHarness) post(body any) (*httptest.ResponseRecorder, ask.Response) {
	h.t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/v1/ask", bytes.NewReader(raw))
	req = req.WithContext(context.WithValue(req.Context(), operatorNameKey, "alice"))
	rec := httptest.NewRecorder()
	h.server.handleAsk(rec, req)

	var resp ask.Response
	if rec.Code == http.StatusOK {
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	}
	return rec, resp
}

// --------------------------------------------------------------- happy

func TestHandleAsk_PlainTextRoundTrip(t *testing.T) {
	h := newAskHarness(t, `{
		"content": [{"type":"text","text":"queue is light right now."}],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 12, "output_tokens": 8}
	}`)

	rec, resp := h.post(ask.Request{
		Agent:    ask.AgentChiefOfStaff,
		ThreadID: "thr_1",
		Messages: []ask.Message{
			{Role: ask.RoleUser, Content: "what's pending?"},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if resp.Message.Role != ask.RoleAssistant {
		t.Errorf("expected assistant role, got %q", resp.Message.Role)
	}
	if !strings.Contains(resp.Message.Content, "queue is light") {
		t.Errorf("unexpected reply: %q", resp.Message.Content)
	}
	if len(resp.Message.References) != 0 {
		t.Errorf("expected no refs, got %+v", resp.Message.References)
	}
}

func TestHandleAsk_ExtractsReferencesFromToolResults(t *testing.T) {
	// Turn 1: model calls list_proposals.
	// Turn 2: model emits text citing one of the proposals.
	h := newAskHarness(t,
		`{"content":[
			{"type":"tool_use","id":"tu_1","name":"list_proposals","input":{}}
		],"stop_reason":"tool_use","usage":{"input_tokens":5,"output_tokens":3}}`,
		`{"content":[
			{"type":"text","text":"Look at [proposal #i1] first — oldest pending."}
		],"stop_reason":"end_turn","usage":{"input_tokens":15,"output_tokens":6}}`,
	)

	// Seed an issue so list_proposals has something to return.
	if _, err := h.server.store.DB.Exec(
		`INSERT OR IGNORE INTO agents (persona, name, enabled, created_at, updated_at, cadence_seconds, max_attempts)
		 VALUES ('marketing', 'Marketing', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 21600, 3)`,
	); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	if _, err := h.server.store.DB.Exec(
		`INSERT INTO issues (id, title, persona, status, priority, created_at, updated_at)
		 VALUES ('i1', 'Linen Napkin', 'marketing', 'in_review', 'normal',
		         '2026-05-20T08:00:00Z', '2026-05-20T08:00:00Z')`,
	); err != nil {
		t.Fatalf("seed issue: %v", err)
	}

	rec, resp := h.post(ask.Request{
		Agent:    ask.AgentChiefOfStaff,
		ThreadID: "thr_refs",
		Messages: []ask.Message{
			{Role: ask.RoleUser, Content: "what's first?"},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(resp.Message.References) != 1 {
		t.Fatalf("expected 1 reference, got %+v", resp.Message.References)
	}
	ref := resp.Message.References[0]
	if ref.Kind != "proposal" || ref.ID != "i1" || ref.Title != "Linen Napkin" || ref.State != "pending" {
		t.Errorf("unexpected reference: %+v", ref)
	}
}

func TestHandleAsk_DispatchedReceiptIncluded(t *testing.T) {
	h := newAskHarness(t,
		`{"content":[
			{"type":"tool_use","id":"tu_1","name":"dispatch_persona","input":{"persona":"pricing","target":"SKU-1234","brief":"competitor dropped"}}
		],"stop_reason":"tool_use","usage":{"input_tokens":7,"output_tokens":5}}`,
		`{"content":[
			{"type":"text","text":"Asked Pricing to look at SKU-1234 — [run rn_test_pricing] will land in about a minute."}
		],"stop_reason":"end_turn","usage":{"input_tokens":20,"output_tokens":12}}`,
	)
	rec, resp := h.post(ask.Request{
		Agent:    ask.AgentChiefOfStaff,
		ThreadID: "thr_dispatch",
		Messages: []ask.Message{
			{Role: ask.RoleUser, Content: "Pricing, look at SKU-1234"},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(resp.Message.Dispatched) != 1 {
		t.Fatalf("expected 1 dispatched, got %+v", resp.Message.Dispatched)
	}
	d := resp.Message.Dispatched[0]
	if d.Persona != "pricing" || d.RunID != "rn_test_pricing" || d.ETASeconds != 60 {
		t.Errorf("unexpected dispatched: %+v", d)
	}
	// The run reference should also resolve via the dispatch's title.
	if len(resp.Message.References) != 1 || resp.Message.References[0].ID != "rn_test_pricing" {
		t.Errorf("expected one run reference; got %+v", resp.Message.References)
	}
}

func TestHandleAsk_PageContextFlowsToModel(t *testing.T) {
	// Capture the request body the mock receives so we can assert
	// the page_context was inlined into the user message.
	var captured string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		captured = string(b)
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"got it"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`)
	}))
	t.Cleanup(mock.Close)

	st, _ := store.Open(context.Background(), ":memory:")
	t.Cleanup(func() { _ = st.DB.Close() })

	srv := &Server{store: st}
	srv.SetAsk(AskConfig{
		Client:  anthropic.New("k", "m").WithAPIURL(mock.URL + "/v1/messages"),
		Threads: ask.NewThreadStore(0),
		Agents: map[ask.AgentSlug]AskAgent{
			ask.AgentChiefOfStaff: {
				Prompt: func(string) string { return "system" },
				Tools:  nil,
			},
		},
	})

	req := httptest.NewRequest("POST", "/v1/ask", bytes.NewReader([]byte(`{
		"agent":"chief_of_staff",
		"thread_id":"t",
		"messages":[{
			"role":"user","content":"why is this stuck?",
			"page_context":{"page":"needs-review","visible_items":[{"id":"i1","title":"Linen Napkin","kind":"proposal"}]}
		}]
	}`)))
	req = req.WithContext(context.WithValue(req.Context(), operatorNameKey, "alice"))
	rec := httptest.NewRecorder()
	srv.handleAsk(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	// Decode the body the model would see (json.Encoder escapes < and >
	// in the raw wire form; the model receives the decoded text).
	var sent struct {
		Messages []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(captured), &sent); err != nil {
		t.Fatalf("decode captured request: %v · %s", err, captured)
	}
	if len(sent.Messages) != 1 || len(sent.Messages[0].Content) != 1 {
		t.Fatalf("unexpected message shape: %+v", sent)
	}
	text := sent.Messages[0].Content[0].Text
	if !strings.Contains(text, "<page_context>") {
		t.Errorf("expected <page_context> tag in user text; got: %q", text)
	}
	if !strings.Contains(text, "Linen Napkin") {
		t.Errorf("expected visible_item title in user text; got: %q", text)
	}
}

func TestHandleAsk_AppendsBothTurnsToThreadStore(t *testing.T) {
	h := newAskHarness(t, `{
		"content":[{"type":"text","text":"hi"}],
		"stop_reason":"end_turn",
		"usage":{"input_tokens":2,"output_tokens":1}
	}`)
	rec, _ := h.post(ask.Request{
		Agent:    ask.AgentChiefOfStaff,
		ThreadID: "thr_thread",
		Messages: []ask.Message{{Role: ask.RoleUser, Content: "hello"}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	stored := h.server.askCfg.Threads.Snapshot("alice", ask.AgentChiefOfStaff)
	if len(stored) != 2 {
		t.Fatalf("expected user + assistant in store, got %d", len(stored))
	}
	if stored[0].Role != ask.RoleUser || stored[0].Content != "hello" {
		t.Errorf("first stored turn wrong: %+v", stored[0])
	}
	if stored[1].Role != ask.RoleAssistant || stored[1].Content != "hi" {
		t.Errorf("second stored turn wrong: %+v", stored[1])
	}
}

// --------------------------------------------------------- error paths

func TestHandleAsk_503WhenNotConfigured(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest("POST", "/v1/ask", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	srv.handleAsk(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleAsk_400OnUnknownAgent(t *testing.T) {
	h := newAskHarness(t, `{"content":[{"type":"text","text":"ok"}]}`)
	rec, _ := h.post(ask.Request{
		Agent:    "marketing", // not registered in this harness (only CoS)
		ThreadID: "t",
		Messages: []ask.Message{{Role: ask.RoleUser, Content: "x"}},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unknown_agent") {
		t.Errorf("expected unknown_agent code, got: %s", rec.Body.String())
	}
}

func TestHandleAsk_400OnEmptyMessages(t *testing.T) {
	h := newAskHarness(t)
	rec, _ := h.post(ask.Request{Agent: ask.AgentChiefOfStaff, ThreadID: "t", Messages: nil})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "no_messages") {
		t.Errorf("expected no_messages code, got: %s", rec.Body.String())
	}
}

func TestHandleAsk_400OnAssistantLastTurn(t *testing.T) {
	h := newAskHarness(t)
	rec, _ := h.post(ask.Request{
		Agent:    ask.AgentChiefOfStaff,
		ThreadID: "t",
		Messages: []ask.Message{
			{Role: ask.RoleUser, Content: "hi"},
			{Role: ask.RoleAssistant, Content: "hello"},
		},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "no_user_message") {
		t.Errorf("expected no_user_message code, got: %s", rec.Body.String())
	}
}

func TestHandleAsk_502OnLLMHardError(t *testing.T) {
	// Mock returns 5xx; the handler should map to 502.
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
		_, _ = io.WriteString(w, "internal error")
	}))
	t.Cleanup(mock.Close)
	st, _ := store.Open(context.Background(), ":memory:")
	t.Cleanup(func() { _ = st.DB.Close() })
	srv := &Server{store: st}
	srv.SetAsk(AskConfig{
		Client:  anthropic.New("k", "m").WithAPIURL(mock.URL + "/v1/messages"),
		Threads: ask.NewThreadStore(0),
		Agents: map[ask.AgentSlug]AskAgent{
			ask.AgentChiefOfStaff: {Prompt: func(string) string { return "s" }},
		},
	})
	req := httptest.NewRequest("POST", "/v1/ask", bytes.NewReader([]byte(`{
		"agent":"chief_of_staff","thread_id":"t",
		"messages":[{"role":"user","content":"x"}]
	}`)))
	rec := httptest.NewRecorder()
	srv.handleAsk(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "llm_error") {
		t.Errorf("expected llm_error code, got: %s", rec.Body.String())
	}
}
