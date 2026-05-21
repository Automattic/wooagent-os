package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/ask"
)

// TestHandleAskEvents_PublishedEventReachesSubscriber asserts the SSE
// stream delivers events the broker receives. Subscribe → publish →
// receive a "data:" frame containing the event JSON.
func TestHandleAskEvents_PublishedEventReachesSubscriber(t *testing.T) {
	broker := ask.NewBroker()
	srv := &Server{}
	srv.SetAsk(AskConfig{Events: broker})

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/ask/events", srv.handleAskEvents)
	httpsrv := httptest.NewServer(mux)
	defer httpsrv.Close()

	// Start a goroutine that reads SSE frames from the streaming
	// response. Has to read concurrently with the publish call below.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET",
		httpsrv.URL+"/v1/ask/events?thread_id=thr_1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect SSE: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}

	// Drain the initial ": connected" handshake comment, then read
	// the first data frame.
	frame := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		// Read until we see at least one "data:" payload.
		var acc []byte
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				acc = append(acc, buf[:n]...)
				if i := bytes.Index(acc, []byte("data:")); i >= 0 {
					rest := acc[i:]
					if end := bytes.Index(rest, []byte("\n\n")); end >= 0 {
						frame <- string(rest[:end])
						return
					}
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Brief sleep so the SSE handler is parked in the for-select
	// before we publish. The broker's ring buffer would replay even
	// if we published first, but ordering this way exercises the
	// live-delivery path.
	time.Sleep(50 * time.Millisecond)

	broker.Publish("thr_1", ask.Event{
		Kind: "thinking", Agent: "pricing", Tool: "web_search",
		Message: "Pricing is researching — about a minute.",
	})

	select {
	case f := <-frame:
		if !strings.Contains(f, `"thinking"`) {
			t.Errorf("frame missing kind=thinking: %q", f)
		}
		if !strings.Contains(f, `"pricing"`) {
			t.Errorf("frame missing agent=pricing: %q", f)
		}
		if !strings.Contains(f, `"web_search"`) {
			t.Errorf("frame missing tool=web_search: %q", f)
		}
	case <-ctx.Done():
		t.Fatal("did not receive event before timeout")
	}
}

// TestHandleAskEvents_MissingThreadIDReturns400 asserts the required
// query parameter check.
func TestHandleAskEvents_MissingThreadIDReturns400(t *testing.T) {
	srv := &Server{}
	srv.SetAsk(AskConfig{Events: ask.NewBroker()})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/ask/events", nil)
	srv.handleAskEvents(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// TestHandleAskEvents_UnavailableReturns503 asserts that without an
// Events broker configured, the endpoint returns 503 with a clear
// error code.
func TestHandleAskEvents_UnavailableReturns503(t *testing.T) {
	srv := &Server{}
	// No SetAsk call.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/ask/events?thread_id=thr_1", nil)
	srv.handleAskEvents(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

// TestHandleAsk_PublishesThinkingForSlowTool wires through the
// integration: a scripted Anthropic backend whose first turn emits a
// web_search tool_use should cause the handler to publish a thinking
// event to the broker. Verifies OnToolStart is hooked up correctly and
// the slow-tool gate works.
func TestHandleAsk_PublishesThinkingForSlowTool(t *testing.T) {
	h := newAskHarness(t,
		// Turn 1: model emits a web_search tool_use. Anthropic-managed
		// in reality, but for this test we just need the loop to see
		// the block and fire OnToolStart. RunToolLoop will then look
		// for a handler named "web_search" and return tool-not-found,
		// which the next turn handles.
		`{"content":[{"type":"tool_use","id":"tu_1","name":"web_search","input":{"query":"linen napkins"}}],
		  "stop_reason":"tool_use","usage":{"input_tokens":5,"output_tokens":3}}`,
		// Turn 2: model recovers + emits final text.
		`{"content":[{"type":"text","text":"Searched — no comparables found."}],
		  "stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`,
	)

	// Attach a broker + capture published events.
	broker := ask.NewBroker()
	cfg := h.server.askCfg
	cfg.Events = broker
	h.server.askCfg = cfg

	// Subscribe to the broker BEFORE the post so we don't race the
	// ring-buffer replay path.
	ch, cancel := broker.Subscribe("thr_1")
	defer cancel()

	var received []ask.Event
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		timeout := time.After(2 * time.Second)
		for {
			select {
			case e, ok := <-ch:
				if !ok {
					return
				}
				received = append(received, e)
				// Single event is enough for this assertion.
				return
			case <-timeout:
				return
			}
		}
	}()

	rec, _ := h.post(ask.Request{
		Agent:    ask.AgentChiefOfStaff,
		ThreadID: "thr_1",
		Messages: []ask.Message{{Role: ask.RoleUser, Content: "look at SKU-1234"}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("handler status = %d: %s", rec.Code, rec.Body.String())
	}
	wg.Wait()

	if len(received) == 0 {
		t.Fatal("expected at least one thinking event")
	}
	got := received[0]
	if got.Kind != "thinking" {
		t.Errorf("kind = %q, want thinking", got.Kind)
	}
	if got.Tool != "web_search" {
		t.Errorf("tool = %q, want web_search", got.Tool)
	}
	if got.Agent != string(ask.AgentChiefOfStaff) {
		t.Errorf("agent = %q, want %q", got.Agent, ask.AgentChiefOfStaff)
	}
}

// TestHandleAsk_DoesNotPublishForFastTool asserts the slow-tool gate
// works: list_proposals should never trigger a thinking event.
func TestHandleAsk_DoesNotPublishForFastTool(t *testing.T) {
	h := newAskHarness(t,
		`{"content":[{"type":"tool_use","id":"tu_1","name":"list_proposals","input":{}}],
		  "stop_reason":"tool_use","usage":{"input_tokens":5,"output_tokens":3}}`,
		`{"content":[{"type":"text","text":"none pending."}],
		  "stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`,
	)
	broker := ask.NewBroker()
	cfg := h.server.askCfg
	cfg.Events = broker
	h.server.askCfg = cfg

	ch, cancel := broker.Subscribe("thr_1")
	defer cancel()

	rec, _ := h.post(ask.Request{
		Agent:    ask.AgentChiefOfStaff,
		ThreadID: "thr_1",
		Messages: []ask.Message{{Role: ask.RoleUser, Content: "what's pending?"}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	select {
	case e := <-ch:
		t.Errorf("expected no event for list_proposals, got %+v", e)
	case <-time.After(150 * time.Millisecond):
		// OK — slow-tool gate held.
	}
}

// silence unused-import warnings when the test file is read but not
// every helper is called.
var _ = io.Discard
var _ = json.NewDecoder
