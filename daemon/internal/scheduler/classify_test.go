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
		{"http 429 in message is transient", errors.New("anthropic http 429: rate limited"), FailureTransient, "rate limit"},
		{"standalone rate limit message is transient", errors.New("anthropic rate limit exceeded"), FailureTransient, "rate limit"},
		{"http 401 is permanent", errors.New("anthropic http 401: bad key"), FailurePermanent, "auth"},
		{"http 403 is permanent", errors.New("openai http 403: forbidden"), FailurePermanent, "auth"},
		{"invalid api key message is permanent", errors.New("anthropic invalid api key"), FailurePermanent, "auth"},
		{"unknown error defaults to transient", errors.New("some weird thing"), FailureTransient, "unknown error"},

		// Typed LLM errors (DSGWOO-1292). These are what the personas
		// actually return now; the bare-string cases above are the retained
		// fallback for anything that isn't one of our clients.
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
