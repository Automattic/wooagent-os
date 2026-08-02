package scheduler

import (
	"context"
	"errors"
	"strings"

	"github.com/wooagent-os/wooagent-os/daemon/internal/llm"
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/telemetry"
)

// classify maps an error from personas.RunAndPersist to a failure class
// and a one-sentence operator-readable reason. The scheduler uses the class
// to decide whether to enqueue a retry; the reason lands in
// runs.failure_reason for the run-log UI.
//
// Order matters: sentinel checks (errors.Is) come first, then the legacy
// string matching, then the default. Every check is against a wrapped error
// chain rather than a formatted message, so a persona adding context with
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
	// Typed LLM failures. Every persona now calls through
	// internal/llm{,/anthropic}, which returns *llm.APIStatusError wrapping
	// one of these sentinels, so classification survives however the
	// message is worded or wrapped (DSGWOO-1292).
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

	// String fallback for LLM-shaped errors that don't come from our
	// clients — a third-party SDK, or a path not yet migrated. Retained on
	// purpose: it costs one strings.Contains on the failure path and it is
	// what keeps a reworded upstream error from silently degrading to
	// "unknown". Safe to delete once nothing can produce a bare-string LLM
	// error.
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "http 429") || strings.Contains(msg, "rate limit") || strings.Contains(msg, "rate_limit_error") {
		return FailureTransient, "LLM rate limited"
	}
	if strings.Contains(msg, "http 401") || strings.Contains(msg, "http 403") || strings.Contains(msg, "invalid api key") {
		return FailurePermanent, "LLM auth failure; check provider API key in Settings"
	}
	// Conservative default: retry. Worst case we waste 3 retries before
	// marking failed_permanent — which is still better than silently
	// ignoring a flake.
	return FailureTransient, "unknown error: " + err.Error()
}
