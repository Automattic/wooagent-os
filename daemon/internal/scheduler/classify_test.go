package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
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
		{"http 401 is permanent", errors.New("anthropic http 401: bad key"), FailurePermanent, "auth"},
		{"http 403 is permanent", errors.New("openai http 403: forbidden"), FailurePermanent, "auth"},
		{"unknown error defaults to transient", errors.New("some weird thing"), FailureTransient, "unknown error"},
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
