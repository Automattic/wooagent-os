package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/llm/anthropic"
	"github.com/wooagent-os/wooagent-os/daemon/internal/scheduler"
)

// allowedDispatchPersonas is the set of specialists Chief of Staff can
// dispatch to today. Mirrors the live persona registrations checked at
// build time: Marketing, Pricing, and Sales Support are registered in
// `daemon/internal/personas/{marketing,pricing,sales-support}`;
// Reporting / Inventory / Accounting are not.
var allowedDispatchPersonas = map[string]bool{
	"marketing":     true,
	"pricing":       true,
	"sales-support": true,
}

// dispatchETAs is the canonical wall-clock estimate per live persona,
// matching the values baked into the CoS prompt. If runtimes drift,
// update both this map and the prompt together so they stay aligned.
var dispatchETAs = map[string]int{
	"marketing":     10,
	"pricing":       60,
	"sales-support": 15,
}

// EnqueuerFunc is the scheduler entry point DispatchTool uses to kick
// off a persona run. Wrapping it in a func type instead of *Scheduler
// keeps the tool decoupled from the concrete scheduler for testing —
// fakes can drop in without spinning up the queue.
type EnqueuerFunc func(ctx context.Context, persona string) (scheduler.Run, error)

// DispatchTool implements the `dispatch_persona` tool: hand off a
// scoped piece of work to one of the live specialists and return the
// resulting Run id + ETA. The Run rolls forward into a real proposal
// on the board when the specialist completes — no immediate Issue
// placeholder is created (the chat surfaces the run id as a chip; the
// real proposal appears on the board when it lands).
//
// Soft rate limit guards against operator dispatch spam: by default,
// no more than 2 dispatches per persona per 60-second sliding window.
// Exceeded → tool returns a clear text result the model surfaces
// inline ("Pricing is already running…") instead of an HTTP error.
type DispatchTool struct {
	// Enqueue is the scheduler dispatch hook. Production code passes
	// `scheduler.EnqueueOperatorAsked`; tests pass a stub.
	Enqueue EnqueuerFunc
	// Limiter throttles operator-asked dispatches per persona. If nil,
	// DispatchTool installs a default 2-per-60s window on first use.
	Limiter *RateLimiter
}

type dispatchInput struct {
	Persona string `json:"persona"`
	Target  string `json:"target"`
	Brief   string `json:"brief"`
}

type dispatchOutput struct {
	OK         bool   `json:"ok"`
	Persona    string `json:"persona,omitempty"`
	RunID      string `json:"run_id,omitempty"`
	ETASeconds int    `json:"eta_seconds,omitempty"`
	Target     string `json:"target,omitempty"`
	Brief      string `json:"brief,omitempty"`
	// Set when ok=false to give the model a clear, short reason it
	// surfaces in chat (rate-limited, unknown persona, scheduler refused).
	Reason string `json:"reason,omitempty"`
}

func (h *DispatchTool) Definition() anthropic.ToolDef {
	return anthropic.ToolDef{
		Name: "dispatch_persona",
		Description: "Hand off concrete work to one of the live specialists. " +
			"`persona` must be marketing | pricing | sales-support. `target` " +
			"identifies what to work on (SKU, product name, ticket id) — pass " +
			"through what the operator gave you; the specialist resolves it. " +
			"`brief` is a one-sentence summary of what the operator asked for. " +
			"Returns a run id + ETA; the resulting proposal appears on the board " +
			"when the run completes.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"persona": { "type": "string", "enum": ["marketing", "pricing", "sales-support"] },
				"target":  { "type": "string", "description": "SKU, product name, ticket id, or other operator-supplied identifier" },
				"brief":   { "type": "string", "description": "One-sentence summary of the request" }
			},
			"required": ["persona", "target", "brief"]
		}`),
	}
}

func (h *DispatchTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	if h.Enqueue == nil {
		return "", errors.New("dispatch_persona: not configured (enqueuer missing)")
	}

	var in dispatchInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	in.Persona = strings.ToLower(strings.TrimSpace(in.Persona))
	in.Target = strings.TrimSpace(in.Target)
	in.Brief = strings.TrimSpace(in.Brief)

	if !allowedDispatchPersonas[in.Persona] {
		// Soft response so CoS can pivot inline instead of crashing.
		return marshalJSON(dispatchOutput{
			OK:     false,
			Reason: fmt.Sprintf("%q is not a dispatchable specialist (only marketing | pricing | sales-support are live in this version)", in.Persona),
		})
	}
	if in.Target == "" {
		return marshalJSON(dispatchOutput{
			OK:     false,
			Reason: "target is required — ask the operator which product, SKU, or ticket",
		})
	}
	if in.Brief == "" {
		return marshalJSON(dispatchOutput{
			OK:     false,
			Reason: "brief is required — summarize what the operator asked for in one sentence",
		})
	}

	limiter := h.Limiter
	if limiter == nil {
		limiter = DefaultRateLimiter()
		h.Limiter = limiter
	}
	if !limiter.Try(in.Persona, time.Now()) {
		return marshalJSON(dispatchOutput{
			OK:      false,
			Persona: in.Persona,
			Reason:  fmt.Sprintf("%s is already running %d operator-triggered jobs in the last %s — wait for one to land before re-dispatching", in.Persona, limiter.MaxInWindow, limiter.Window),
		})
	}

	run, err := h.Enqueue(ctx, in.Persona)
	if err != nil {
		// Surface scheduler errors as soft tool failures (e.g. persona
		// disabled, scheduler not started) so CoS can explain inline.
		return marshalJSON(dispatchOutput{
			OK:      false,
			Persona: in.Persona,
			Reason:  fmt.Sprintf("scheduler refused dispatch: %s", err.Error()),
		})
	}

	return marshalJSON(dispatchOutput{
		OK:         true,
		Persona:    in.Persona,
		RunID:      run.ID,
		ETASeconds: dispatchETAs[in.Persona],
		Target:     in.Target,
		Brief:      in.Brief,
	})
}

// -------------------------------------------------------------- rate limit

// RateLimiter is an in-memory per-key sliding window. Used to throttle
// operator-asked dispatches per persona so the queue doesn't get
// flooded if the operator (or a confused model) fires repeated tool
// calls. Single-process; resets on daemon restart.
type RateLimiter struct {
	mu          sync.Mutex
	recent      map[string][]time.Time
	Window      time.Duration
	MaxInWindow int
}

// NewRateLimiter builds a limiter with explicit window + cap. Used by
// tests that want a tighter window than the production default.
func NewRateLimiter(window time.Duration, maxInWindow int) *RateLimiter {
	return &RateLimiter{
		recent:      map[string][]time.Time{},
		Window:      window,
		MaxInWindow: maxInWindow,
	}
}

// DefaultRateLimiter is the 2-per-60s production setting.
func DefaultRateLimiter() *RateLimiter {
	return NewRateLimiter(60*time.Second, 2)
}

// Try returns true if the key has room within the window; false if it's
// at or over the limit. The call is recorded on success.
func (r *RateLimiter) Try(key string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	cutoff := now.Add(-r.Window)
	kept := r.recent[key][:0]
	for _, t := range r.recent[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	r.recent[key] = kept
	if len(kept) >= r.MaxInWindow {
		return false
	}
	r.recent[key] = append(r.recent[key], now)
	return true
}

var _ anthropic.ToolHandler = (*DispatchTool)(nil)
