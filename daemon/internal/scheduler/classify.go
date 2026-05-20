package scheduler

import (
	"context"
	"errors"
	"strings"

	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/telemetry"
)

// classify maps an error from personas.RunAndPersist to a failure class
// and a one-sentence operator-readable reason. The scheduler uses the class
// to decide whether to enqueue a retry; the reason lands in
// runs.failure_reason for the run-log UI.
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
	msg := strings.ToLower(err.Error())
	// LLM rate limits — until a centralized LLM package exists with typed
	// errors, pattern-match on the HTTP status code that the persona's
	// ad-hoc client returns.
	if strings.Contains(msg, "http 429") || strings.Contains(msg, "rate limit") || strings.Contains(msg, "rate_limit_error") {
		return FailureTransient, "LLM rate limited"
	}
	// Auth failures — permanent until operator fixes config.
	if strings.Contains(msg, "http 401") || strings.Contains(msg, "http 403") || strings.Contains(msg, "invalid api key") {
		return FailurePermanent, "LLM auth failure; check provider API key in Settings"
	}
	// Conservative default: retry. Worst case we waste 3 retries before
	// marking failed_permanent — which is still better than silently
	// ignoring a flake.
	return FailureTransient, "unknown error: " + err.Error()
}
