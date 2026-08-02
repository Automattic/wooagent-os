package mcp

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// Failure classes for an HTTP-level rejection from the store. The scheduler
// matches these with errors.Is to decide retry-vs-stop, and to tell the
// operator something they can act on.
//
// Before these existed, any HTTP status from the store fell through
// scheduler.classify to "unknown error" and dragged the whole response body
// into runs.failure_reason. A store returning a 410 "This site is disabled"
// page produced ~1KB of CSS in a run-log table cell and the word "unknown"
// in the UI (DSGWOO-1469).
var (
	// ErrStoreAuth — the store rejected our credentials. Permanent: the
	// operator has to re-pair before anything will work.
	ErrStoreAuth = errors.New("mcp: store rejected our credentials")
	// ErrStoreGone — nothing is listening at the MCP endpoint. Permanent:
	// the site is disabled, or the endpoint moved, or the plugin is off.
	ErrStoreGone = errors.New("mcp: store endpoint is gone")
	// ErrStoreUnavailable — the store is up but can't serve us right now
	// (5xx, throttling). Transient.
	ErrStoreUnavailable = errors.New("mcp: store unavailable")
	// ErrStoreBadRequest — the store understood us and refused. Permanent:
	// retrying an identical request gets an identical refusal.
	ErrStoreBadRequest = errors.New("mcp: store rejected the request")
)

// bodyExcerptMax caps how much of the store's response appears in an error
// message. The full body stays on StatusError.Body. The cap exists because
// this string reaches runs.failure_reason, which the run-log UI renders in
// a table cell.
const bodyExcerptMax = 160

// ReasonBudget is the length an operator-facing failure reason must stay
// within. It matches LONG_REASON_THRESHOLD in ui/src/lib/runText.ts: past
// this, the agent-roster notice condenses a reason to its leading clause,
// which for an error string means showing the operation and hiding the
// cause (DSGWOO-1472). Callers composing a reason around Detail should use
// it to size the excerpt.
const ReasonBudget = 140

// StatusError is a non-2xx HTTP response from the store's MCP endpoint.
// Mirrors llm.APIStatusError: enough context to debug on the struct, a
// bounded message, and an Unwrap that lets callers classify without reading
// the text.
type StatusError struct {
	// Endpoint is the MCP URL that returned this, so a run log can show
	// which store was actually called.
	Endpoint string
	// StatusCode is the HTTP status.
	StatusCode int
	// Body is the raw response body, untruncated.
	Body string
}

// NewStatusError builds a StatusError for a non-2xx response.
func NewStatusError(endpoint string, statusCode int, body []byte) *StatusError {
	return &StatusError{Endpoint: endpoint, StatusCode: statusCode, Body: string(body)}
}

func (e *StatusError) Error() string {
	head := fmt.Sprintf("store http %d", e.StatusCode)
	if d := readableBody(e.Body, bodyExcerptMax); d != "" {
		return head + ": " + d
	}
	return head
}

// Detail returns up to max runes of readable prose from the store's
// response body, or "" if it carried none. Callers building a bounded
// operator message size max against their own surrounding text rather than
// trusting a fixed cap to fit.
func (e *StatusError) Detail(max int) string {
	return readableBody(e.Body, max)
}

// Unwrap maps the status onto a failure class so errors.Is works through
// however many layers of fmt.Errorf("%w") the call stack added.
//
// Returns nil for statuses with no clear class; the caller's conservative
// default is better than a confident wrong answer.
func (e *StatusError) Unwrap() error {
	switch {
	case e.StatusCode == http.StatusUnauthorized, e.StatusCode == http.StatusForbidden:
		return ErrStoreAuth
	case e.StatusCode == http.StatusGone, e.StatusCode == http.StatusNotFound:
		// 404 and 410 mean the same thing to an operator: there's no MCP
		// endpoint at this URL any more. Same fix, so same class.
		return ErrStoreGone
	case e.StatusCode == http.StatusTooManyRequests, e.StatusCode >= 500:
		return ErrStoreUnavailable
	case e.StatusCode >= 400:
		return ErrStoreBadRequest
	}
	return nil
}

var (
	// Script and style bodies are markup, not prose — dropped wholesale.
	// Without this, an HTML error page contributes its stylesheet to the
	// excerpt, which is exactly what made the original 410 unreadable.
	scriptOrStyleRe = regexp.MustCompile(`(?is)<(script|style)\b[^>]*>.*?</(script|style)>`)
	tagRe           = regexp.MustCompile(`(?s)<[^>]*>`)
	wsRe            = regexp.MustCompile(`\s+`)
)

// readableBody reduces a response body to a single line of prose, capped at
// max runes.
//
// Hosting-level failures (Pressable, Cloudflare, a WAF) answer with an HTML
// page whose useful content is one sentence buried in markup — the 410 that
// prompted this carried "This site is disabled." inside 1.1KB of CSS. JSON
// and plain-text bodies pass through with whitespace collapsed.
//
// Error pages routinely state their heading twice — <title>410 Gone</title>
// followed by <h1>410 Gone</h1> — so the flattened text opens with a
// duplicated phrase. dropLeadingRepeat removes it, which matters because
// the excerpt is length-capped: 18 characters of restated heading is 18
// characters the sentence that explains the failure doesn't get.
func readableBody(body string, max int) string {
	s := strings.TrimSpace(body)
	if s == "" {
		return ""
	}
	if strings.Contains(s, "<") {
		s = scriptOrStyleRe.ReplaceAllString(s, " ")
		s = tagRe.ReplaceAllString(s, " ")
	}
	s = strings.TrimSpace(wsRe.ReplaceAllString(s, " "))
	if s == "" {
		return ""
	}
	s = dropLeadingRepeat(s)
	r := []rune(s)
	if len(r) > max {
		return strings.TrimSpace(string(r[:max])) + "…"
	}
	return s
}

// dropLeadingRepeat collapses an immediately-repeated opening phrase:
// "410 Gone 410 Gone This site is disabled." → "410 Gone This site is
// disabled."
//
// Longest repeat wins, so "a b a b" collapses to "a b" rather than looping
// on the single-word case. Only the opening is considered — a phrase
// recurring later in the text is likely meaningful rather than markup
// duplication.
func dropLeadingRepeat(s string) string {
	words := strings.Fields(s)
	for k := len(words) / 2; k >= 1; k-- {
		if slicesEqual(words[:k], words[k:2*k]) {
			return strings.Join(words[k:], " ")
		}
	}
	return s
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
