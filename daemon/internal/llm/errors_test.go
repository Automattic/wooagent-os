package llm

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestAPIStatusError_UnwrapsByStatus(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   error
	}{
		{429, ErrRateLimited},
		{401, ErrAuth},
		{403, ErrAuth},
		{400, ErrInvalidRequest},
		{404, ErrInvalidRequest},
		{413, ErrInvalidRequest},
		{422, ErrInvalidRequest},
		{500, ErrServer},
		{502, ErrServer},
		{529, ErrServer}, // Anthropic "overloaded"
		{418, nil},       // no meaningful class → caller's default
	} {
		err := NewAPIStatusError("anthropic", tc.status, []byte("body"))
		if tc.want == nil {
			for _, sentinel := range []error{ErrRateLimited, ErrAuth, ErrInvalidRequest, ErrServer} {
				if errors.Is(err, sentinel) {
					t.Errorf("status %d should match no sentinel, matched %v", tc.status, sentinel)
				}
			}
			continue
		}
		if !errors.Is(err, tc.want) {
			t.Errorf("status %d: want errors.Is(_, %v), got %v", tc.status, tc.want, err)
		}
	}
}

func TestAPIStatusError_UnwrapsByProviderType(t *testing.T) {
	// The embedded-in-200 case: no status, provider supplies a type string.
	for _, tc := range []struct {
		errType string
		want    error
	}{
		{"rate_limit_error", ErrRateLimited},
		{"authentication_error", ErrAuth},
		{"permission_error", ErrAuth},
		{"invalid_request_error", ErrInvalidRequest},
		{"request_too_large", ErrInvalidRequest},
		{"overloaded_error", ErrServer},
		{"api_error", ErrServer},
	} {
		err := NewAPIError("anthropic", tc.errType, "something went wrong")
		if !errors.Is(err, tc.want) {
			t.Errorf("type %q: want errors.Is(_, %v), got %v", tc.errType, tc.want, err)
		}
	}
}

// The whole point of Unwrap: classification has to survive being wrapped in
// layers of context on the way up through a persona.
func TestAPIStatusError_SurvivesWrapping(t *testing.T) {
	base := NewAPIStatusError("anthropic", 429, []byte("slow down"))
	wrapped := fmt.Errorf("draft rewrite: %w", fmt.Errorf("call llm: %w", base))

	if !errors.Is(wrapped, ErrRateLimited) {
		t.Errorf("wrapped error lost its class: %v", wrapped)
	}

	var target *APIStatusError
	if !errors.As(wrapped, &target) {
		t.Fatalf("errors.As should recover the concrete type")
	}
	if target.StatusCode != 429 || target.Provider != "anthropic" {
		t.Errorf("recovered %+v, want provider=anthropic status=429", target)
	}
	if target.Body != "slow down" {
		t.Errorf("Body = %q, want the untruncated original", target.Body)
	}
}

func TestAPIStatusError_Message(t *testing.T) {
	if got := NewAPIStatusError("anthropic", 429, []byte("slow down")).Error(); got != "anthropic: http 429: slow down" {
		t.Errorf("Error() = %q", got)
	}
	if got := NewAPIStatusError("openai", 500, nil).Error(); got != "openai: http 500" {
		t.Errorf("Error() with empty body = %q", got)
	}
	if got := NewAPIError("anthropic", "rate_limit_error", "too fast").Error(); got != "anthropic: api error rate_limit_error: too fast" {
		t.Errorf("Error() for embedded type = %q", got)
	}
}

// failure_reason lands in a run-log table cell, so the message must not
// carry a multi-kilobyte body even though the field retains it.
func TestAPIStatusError_TruncatesBodyInMessage(t *testing.T) {
	huge := strings.Repeat("x", 5000)
	err := NewAPIStatusError("anthropic", 400, []byte(huge))

	msg := err.Error()
	if len(msg) > bodyExcerptMax+64 {
		t.Errorf("message length %d, want it truncated near %d", len(msg), bodyExcerptMax)
	}
	if !strings.HasSuffix(msg, "…") {
		t.Errorf("expected an ellipsis to mark truncation, got %q", msg[len(msg)-20:])
	}
	if len(err.Body) != 5000 {
		t.Errorf("Body should stay untruncated, got %d chars", len(err.Body))
	}
}
