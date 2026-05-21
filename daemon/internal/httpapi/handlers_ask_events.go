package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/wooagent-os/wooagent-os/daemon/internal/ask"
)

// handleAskEvents serves Server-Sent Events for one chat thread
// (DSGWOO-1356). The drawer opens this stream immediately before
// posting /v1/ask, and the daemon publishes thinking events to it
// whenever a slow tool (currently web_search) fires inside the
// tool-use loop. The connection stays open until the client closes
// (ctx done) — typically when the /v1/ask reply lands and the UI tears
// down the EventSource.
//
// Wire format: standard text/event-stream with one JSON Event per
// "data:" frame. The UI's EventSource handler parses each frame as
// ask.Event and renders accordingly.
//
// Auth: same bearer-token middleware as the rest of /v1/*. The route
// also accepts a `token` query parameter as a fallback for the
// EventSource API, which doesn't expose a way to set arbitrary
// request headers in browsers — the bearer middleware honors it.
func (s *Server) handleAskEvents(w http.ResponseWriter, r *http.Request) {
	if s.askCfg == nil || s.askCfg.Events == nil {
		writeError(w, http.StatusServiceUnavailable, "events_unavailable", "ask events endpoint not configured")
		return
	}

	threadID := r.URL.Query().Get("thread_id")
	if threadID == "" {
		writeError(w, http.StatusBadRequest, "missing_thread_id", "thread_id query parameter is required")
		return
	}

	// SSE handshake. Disable proxy buffering so events flush
	// immediately; the X-Accel-Buffering header is honored by nginx
	// and some reverse proxies common in WP-hosted deployments.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "no_flusher", "streaming not supported")
		return
	}

	// Subscribe before the first flush so any thinking events fired
	// in the brief window between client EventSource open and server
	// Subscribe land in the ring buffer and replay on connect.
	ch, cancel := s.askCfg.Events.Subscribe(threadID)
	defer cancel()

	// Initial empty comment line — flushes headers immediately so the
	// client's EventSource onopen fires even if no events come for a
	// while. Without this, browsers buffer the first byte and the
	// drawer's "isConnected" signal lags.
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case e, open := <-ch:
			if !open {
				return
			}
			b, err := json.Marshal(e)
			if err != nil {
				// Skip malformed events; not worth crashing the
				// stream over a single bad serialize.
				continue
			}
			// SSE "event: <type>" line is optional — every payload is
			// a JSON Event with its own Kind field, so the UI
			// discriminates on the body. Single-line data is required.
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}

// Compile-time assertion: handler signature matches chi expectation.
var _ http.HandlerFunc = (*Server)(nil).handleAskEvents
var _ = ask.Event{} // keep the ask import honest if no other reference lands here
