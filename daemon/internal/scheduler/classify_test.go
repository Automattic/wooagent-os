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
		// Store HTTP failures (DSGWOO-1469). Previously all of these fell
		// through to "unknown error".
		{
			"store 401 is permanent and says to re-pair",
			mcp.NewStatusError("https://s.example.com/mcp", 401, []byte(`{"message":"Sorry, you are not allowed to do that."}`)),
			FailurePermanent, "not allowed to do that",
		},
		{
			"store 410 is permanent and names the site as possibly disabled",
			mcp.NewStatusError("https://s.example.com/mcp", 410, []byte("<h1>410 Gone</h1><p>This site is disabled.</p>")),
			FailurePermanent, "This site is disabled",
		},
		{
			"store 503 is transient",
			mcp.NewStatusError("https://s.example.com/mcp", 503, nil),
			FailureTransient, "retrying",
		},
		{
			"store 400 is permanent",
			mcp.NewStatusError("https://s.example.com/mcp", 400, []byte("bad params")),
			FailurePermanent, "bad params",
		},
		{
			// The real shape: the persona wraps it before the scheduler sees it.
			"store error survives persona wrapping",
			fmt.Errorf("mcp initialize: %w", mcp.NewStatusError("https://s.example.com/mcp", 410, []byte("<p>This site is disabled.</p>"))),
			FailurePermanent, "This site is disabled",
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

// The end-to-end check for DSGWOO-1469: replay the exact failure an operator
// hit and assert the reason is now something they can act on, and short
// enough that the UI shows it whole.
//
// Before: "unknown error: mcp initialize: initialize: http 410: <!DOCTYPE
// html>…" — 1138 characters, mostly CSS, which the roster notice condensed
// all the way down to "unknown error."
func TestClassify_RealStoreDisabledFailure(t *testing.T) {
	body := `<!DOCTYPE html><html lang="en"><head><title>410 Gone</title>
<style>body { background-color: #f8f8f8; font-family: "Helvetica Neue", Helvetica, Arial, sans-serif; color: #333; }
.error-msg h1 { font-size: 52px; display: block; margin: 0px; }</style></head>
<body><div class="error-msg"><h1>410 Gone</h1><p>This site is disabled.</p></div>
<div class="support-msg"><p>Website owner? If you think you have reached this message in error, please contact support.</p></div></body></html>`

	err := fmt.Errorf("mcp initialize: %w",
		mcp.NewStatusError("https://woo-demo-store-99cc5c.example.com/mcp", 410, []byte(body)))

	cls, reason := classify(err)

	if cls != FailurePermanent {
		t.Errorf("class = %q, want %q — a disabled site will not recover on retry", cls, FailurePermanent)
	}
	if !strings.Contains(reason, "This site is disabled") {
		t.Errorf("reason should carry the store's own words, got: %q", reason)
	}
	if strings.Contains(reason, "unknown error") {
		t.Errorf("reason should no longer be 'unknown error', got: %q", reason)
	}
	for _, css := range []string{"font-family", "background-color", "<", "{"} {
		if strings.Contains(reason, css) {
			t.Errorf("markup/CSS leaked into failure_reason (%q): %q", css, reason)
		}
	}
	// Must stay under the UI's show-in-full threshold, or summarizeReason
	// condenses it and we are back to a useless notice (DSGWOO-1472).
	const uiShowInFullThreshold = 140
	if len([]rune(reason)) > uiShowInFullThreshold {
		t.Errorf("reason is %d runes, over the UI's %d-char threshold; it would be truncated again: %q",
			len([]rune(reason)), uiShowInFullThreshold, reason)
	}
	t.Logf("operator sees: %q", reason)
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
