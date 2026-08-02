package scheduler

import (
	"context"
	"errors"

	"github.com/wooagent-os/wooagent-os/daemon/internal/llm"
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/telemetry"
)

// classify maps an error from personas.RunAndPersist to a failure class
// and a one-sentence operator-readable reason. The scheduler uses the class
// to decide whether to enqueue a retry; the reason lands in
// runs.failure_reason for the run-log UI.
//
// Every check matches against the wrapped error chain via errors.Is, never
// against a formatted message, so a persona adding context with
// fmt.Errorf("...: %w", err) cannot break classification.
//
// The conservative default for unknown errors is FailureTransient — better
// to waste a retry than to give up on a flaky LLM call.
func classify(err error) (FailureClass, string) {
	if err == nil {
		return FailureUnknown, ""
	}
	if errors.Is(err, context.Canceled) {
		return FailureTransient, "cancelled before completion"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return FailureTransient, "timed out"
	}
	if errors.Is(err, mcp.ErrSessionLost) {
		return FailureTransient, "MCP session lost; re-initialize on next attempt"
	}
	if errors.Is(err, mcp.ErrTransport) {
		return FailureTransient, "MCP transport error; transient network issue"
	}
	// Per-run cost cap. Permanent: a looping persona would re-trip the gate
	// on every retry. The wrapped error message carries the actual dollar
	// amount the run consumed, which is exactly what an operator triaging
	// the failure_reason cell wants to see. DSGWOO-1296.
	if errors.Is(err, telemetry.ErrRunBudgetExceeded) {
		return FailurePermanent, err.Error()
	}
	// Typed LLM failures. Every LLM call in the daemon goes through
	// internal/llm/anthropic.Client (or, on Marketing's OpenAI-compatible
	// fallback, wraps the same type), so these arrive as
	// *llm.APIStatusError and classify correctly however the message is
	// worded or however many layers of fmt.Errorf("%w") wrap it.
	// DSGWOO-1292 / DSGWOO-1467.
	if errors.Is(err, llm.ErrRateLimited) {
		return FailureTransient, "LLM rate limited"
	}
	if errors.Is(err, llm.ErrAuth) {
		return FailurePermanent, "LLM auth failure; check provider API key in Settings"
	}
	if errors.Is(err, llm.ErrServer) {
		return FailureTransient, "LLM provider error; retrying"
	}
	if errors.Is(err, llm.ErrInvalidRequest) {
		return FailurePermanent, "LLM rejected the request; check model name and prompt size"
	}
	// Conservative default: retry. Worst case we waste 3 retries before
	// marking failed_permanent — which is still better than silently
	// ignoring a flake.
	//
	// This is also where a genuinely untyped failure lands: an LLM
	// transport error, an MCP error that isn't one of the sentinels above,
	// a decode failure. Transient is right for all of them.
	return FailureTransient, "unknown error: " + err.Error()
}
