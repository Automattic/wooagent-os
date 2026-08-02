package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/wooagent-os/wooagent-os/daemon/internal/llm"
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/telemetry"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		wantCls FailureClass
		wantSub string // substring expected in the reason
	}{
		{"nil error returns unknown", nil, FailureUnknown, ""},
		{"context canceled is transient", context.Canceled, FailureTransient, "cancel"},
		{"deadline exceeded is transient", context.DeadlineExceeded, FailureTransient, "timed out"},
		{"mcp session lost is transient", fmt.Errorf("call: %w", mcp.ErrSessionLost), FailureTransient, "MCP session"},
		{"mcp transport is transient", fmt.Errorf("call: %w", mcp.ErrTransport), FailureTransient, "MCP transport"},
		{"unknown error defaults to transient", errors.New("some weird thing"), FailureTransient, "unknown error"},

		// Typed LLM errors. Every LLM call in the daemon routes through
		// llm/anthropic.Client, so this is the only shape classify sees for
		// a provider failure (DSGWOO-1292).
		{
			"typed rate limit is transient",
			llm.NewAPIStatusError("anthropic", 429, []byte("slow down")),
			FailureTransient, "rate limit",
		},
		{
			"typed auth is permanent",
			llm.NewAPIStatusError("anthropic", 401, []byte("bad key")),
			FailurePermanent, "auth",
		},
		{
			"typed 5xx is transient",
			llm.NewAPIStatusError("anthropic", 503, nil),
			FailureTransient, "provider error",
		},
		{
			"typed 400 is permanent",
			llm.NewAPIStatusError("anthropic", 400, []byte("max_tokens too large")),
			FailurePermanent, "rejected the request",
		},
		{
			"typed openai fallback classifies the same way",
			llm.NewAPIStatusError("openai", 429, nil),
			FailureTransient, "rate limit",
		},
		{
			// The real shape from pricing.go: the persona wraps the typed
			// error with context before the scheduler sees it.
			"typed error survives persona wrapping",
			fmt.Errorf("draft proposal: %w (raw=%s)", llm.NewAPIStatusError("anthropic", 429, []byte("x")), "x"),
			FailureTransient, "rate limit",
		},
		{
			"embedded api error type classifies",
			llm.NewAPIError("anthropic", "authentication_error", "invalid x-api-key"),
			FailurePermanent, "auth",
		},
		{
			"per-run budget exceeded is permanent",
			fmt.Errorf("draft: %w", fmt.Errorf("%w: total $0.1234 exceeds cap $0.10", telemetry.ErrRunBudgetExceeded)),
			FailurePermanent,
			"exceeds cap",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cls, reason := classify(tc.err)
			if cls != tc.wantCls {
				t.Errorf("class = %q, want %q", cls, tc.wantCls)
			}
			if tc.wantSub != "" && !strings.Contains(reason, tc.wantSub) {
				t.Errorf("reason = %q, want substring %q", reason, tc.wantSub)
			}
		})
	}
}

// TestClassify_BareStringLLMErrorsFallThrough pins the deliberate behavior
// change from DSGWOO-1467, which removed the string-matching fallback.
//
// classify no longer reads error text at all. An LLM-shaped error that
// arrives as a bare string is therefore NOT recognized — it lands on the
// transient default. That is safe because no such path exists: every LLM
// call routes through llm/anthropic.Client, Marketing's OpenAI-compatible
// fallback wraps *llm.APIStatusError, the Reporting persona makes no LLM
// calls, and lessons/digest.go already used the shared client.
//
// If this test ever starts failing because someone expects a bare string to
// classify, the fix is to make that call site return a typed error — not to
// reinstate string matching.
func TestClassify_BareStringLLMErrorsFallThrough(t *testing.T) {
	for _, raw := range []string{
		"anthropic http 429: rate limited",
		"anthropic http 401: bad key",
		"openai http 403: forbidden",
		"anthropic invalid api key",
	} {
		cls, reason := classify(errors.New(raw))
		if cls != FailureTransient {
			t.Errorf("classify(%q) = %q, want %q (the conservative default)", raw, cls, FailureTransient)
		}
		if !strings.HasPrefix(reason, "unknown error: ") {
			t.Errorf("classify(%q) reason = %q, want the unknown-error default", raw, reason)
		}
	}
}
