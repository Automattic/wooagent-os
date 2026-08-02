package llm

import (
	"errors"
	"fmt"
	"net/http"
)

// Provider-neutral failure classes for LLM calls. Callers match on these
// with errors.Is rather than pattern-matching error strings —
// scheduler.classify used to do the latter, which broke silently whenever a
// persona reworded its error (DSGWOO-1292).
//
// The set is deliberately small: it covers the distinctions the scheduler
// actually acts on, which is "retry this" vs. "stop, an operator has to fix
// something".
var (
	// ErrRateLimited — the provider is throttling us. Transient.
	ErrRateLimited = errors.New("llm: rate limited")
	// ErrAuth — bad or missing credentials. Permanent until the operator
	// fixes the provider config.
	ErrAuth = errors.New("llm: auth failure")
	// ErrInvalidRequest — we sent something the provider rejected (bad
	// model name, malformed body, oversized prompt). Permanent: retrying
	// the same request produces the same rejection.
	ErrInvalidRequest = errors.New("llm: invalid request")
	// ErrServer — provider-side failure or overload. Transient.
	ErrServer = errors.New("llm: provider error")
)

// bodyExcerptMax caps how much of the provider's response body appears in
// an error message. The full body stays on the Body field. The cap exists
// because this string can land in runs.failure_reason, which the run-log UI
// renders in a table cell.
const bodyExcerptMax = 200

// APIStatusError is a failed LLM provider call. It carries enough context
// to debug (which provider, which status, what the body said) while
// unwrapping to one of the sentinels above so callers can classify it
// without touching the message text.
type APIStatusError struct {
	// Provider is the short identifier used elsewhere for cost accounting
	// and telemetry.ModelCall.Provider — "anthropic", "openai".
	Provider string
	// StatusCode is the HTTP status. Zero when the provider signalled the
	// failure inside an otherwise-200 response body, in which case Type
	// carries the discriminator.
	StatusCode int
	// Type is the provider's own error identifier when it supplies one
	// (Anthropic: "rate_limit_error", "authentication_error", ...).
	Type string
	// Body is the raw response body, untruncated.
	Body string
}

// NewAPIStatusError builds an APIStatusError for a non-2xx HTTP response.
func NewAPIStatusError(provider string, statusCode int, body []byte) *APIStatusError {
	return &APIStatusError{Provider: provider, StatusCode: statusCode, Body: string(body)}
}

// NewAPIError builds an APIStatusError for a failure the provider reported
// inside a successful HTTP response, identified by its own type string.
func NewAPIError(provider, errType, message string) *APIStatusError {
	return &APIStatusError{Provider: provider, Type: errType, Body: message}
}

// ErrorBody returns the provider's raw response body carried by err, or ""
// if err doesn't wrap an *APIStatusError.
//
// The personas surface this alongside their own return value: the run log
// persists the raw provider output so an operator triaging a failed draft
// can see what actually came back, not just the classification.
func ErrorBody(err error) string {
	var se *APIStatusError
	if errors.As(err, &se) {
		return se.Body
	}
	return ""
}

func (e *APIStatusError) Error() string {
	var head string
	switch {
	case e.StatusCode > 0 && e.Type != "":
		head = fmt.Sprintf("%s: http %d (%s)", e.Provider, e.StatusCode, e.Type)
	case e.StatusCode > 0:
		head = fmt.Sprintf("%s: http %d", e.Provider, e.StatusCode)
	case e.Type != "":
		head = fmt.Sprintf("%s: api error %s", e.Provider, e.Type)
	default:
		head = e.Provider + ": api error"
	}
	if e.Body == "" {
		return head
	}
	body := e.Body
	if len(body) > bodyExcerptMax {
		body = body[:bodyExcerptMax] + "…"
	}
	return head + ": " + body
}

// Unwrap maps the provider's response onto one of the package sentinels so
// errors.Is works through however many layers of fmt.Errorf("%w") a caller
// has wrapped this in.
//
// Returns nil for statuses with no meaningful class — an unrecognized
// failure should reach the caller's conservative default rather than be
// mislabelled.
func (e *APIStatusError) Unwrap() error {
	switch {
	case e.StatusCode == http.StatusTooManyRequests:
		return ErrRateLimited
	case e.StatusCode == http.StatusUnauthorized, e.StatusCode == http.StatusForbidden:
		return ErrAuth
	case e.StatusCode == http.StatusBadRequest, e.StatusCode == http.StatusUnprocessableEntity,
		e.StatusCode == http.StatusRequestEntityTooLarge, e.StatusCode == http.StatusNotFound:
		return ErrInvalidRequest
	case e.StatusCode >= 500:
		// Includes Anthropic's 529 "overloaded".
		return ErrServer
	case e.StatusCode > 0:
		return nil
	}

	// No usable status: the provider embedded the failure in a 200 body.
	// These type strings are Anthropic's; OpenAI-compatible servers use the
	// same shape for the two that matter.
	switch e.Type {
	case "rate_limit_error":
		return ErrRateLimited
	case "authentication_error", "permission_error":
		return ErrAuth
	case "invalid_request_error", "not_found_error", "request_too_large":
		return ErrInvalidRequest
	case "api_error", "overloaded_error":
		return ErrServer
	}
	return nil
}
