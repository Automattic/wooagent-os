package salessupport

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDraftMessageDoesNotLeakPII is the rule-lock test for
// DSGWOO-1320. It exercises the full draftMessage code path against a
// fake Anthropic endpoint, captures the request body, and asserts
// that the customer's email and surname never appear in it. Mirrors
// the WooCommerce-for-Claude test-pii-toggle.php regression lock.
//
// Add canaries here when extending coverage to address lines, prior
// notes, or any other future PII surface. The promise is: real
// customer PII never crosses the daemon → LLM-provider boundary.
func TestDraftMessageDoesNotLeakPII(t *testing.T) {
	const (
		canaryEmail   = "maria.rodriguez+canary@example.test"
		canarySurname = "RodriguezCanarySurname"
	)
	var capturedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"{\"no_proposal\":false,\"note_type\":\"customer\",\"subject_hint\":\"thanks\",\"message\":\"Hi Maria,\\n\\nThanks!\\n\\n— Elizabeth\"}"}],"usage":{"input_tokens":10,"output_tokens":10}}`))
	}))
	defer server.Close()

	prevURL := anthropicAPIURL
	anthropicAPIURL = server.URL
	defer func() { anthropicAPIURL = prevURL }()

	o := order{
		ID:            42,
		Number:        "1042",
		Status:        "processing",
		Total:         "59.95",
		Currency:      "USD",
		DateCreated:   "2026-05-14T10:00:00",
		BillingName:   "Maria " + canarySurname,
		CustomerEmail: canaryEmail,
	}
	pc := redactOrderForPrompt(o)

	if _, _, err := draftMessage(context.Background(), "test-key", "test-model", pc); err != nil {
		t.Fatalf("draftMessage: %v", err)
	}

	for _, forbidden := range []string{canaryEmail, canarySurname} {
		if strings.Contains(capturedBody, forbidden) {
			t.Errorf("Anthropic request body contains forbidden PII %q.\n--- body ---\n%s\n--- end ---", forbidden, capturedBody)
		}
	}
	if !strings.Contains(capturedBody, "Maria") {
		t.Errorf("Anthropic request body missing first name 'Maria'.\n--- body ---\n%s\n--- end ---", capturedBody)
	}
}

// TestDraftMessageGuestOrderUsesFallback covers the no-name path —
// "the customer" should appear in the prompt and the canary surname
// (still set on the order via a different field, to make sure no
// other field is leaking) must not.
func TestDraftMessageGuestOrderUsesFallback(t *testing.T) {
	const canaryEmail = "guest+canary@example.test"
	var capturedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"{\"no_proposal\":false,\"note_type\":\"customer\",\"subject_hint\":\"thanks\",\"message\":\"Hi,\\n\\nThanks!\\n\\n— Elizabeth\"}"}],"usage":{"input_tokens":10,"output_tokens":10}}`))
	}))
	defer server.Close()

	prevURL := anthropicAPIURL
	anthropicAPIURL = server.URL
	defer func() { anthropicAPIURL = prevURL }()

	o := order{
		ID:            43,
		Number:        "1043",
		Status:        "processing",
		Total:         "29.00",
		Currency:      "USD",
		DateCreated:   "2026-05-14T11:00:00",
		// No BillingName, no ShippingName.
		CustomerEmail: canaryEmail,
	}
	pc := redactOrderForPrompt(o)

	if _, _, err := draftMessage(context.Background(), "test-key", "test-model", pc); err != nil {
		t.Fatalf("draftMessage: %v", err)
	}

	if strings.Contains(capturedBody, canaryEmail) {
		t.Errorf("Anthropic request body contains forbidden email %q.\n--- body ---\n%s\n--- end ---", canaryEmail, capturedBody)
	}
	if !strings.Contains(capturedBody, "the customer") {
		t.Errorf("Anthropic request body missing guest fallback 'the customer'.\n--- body ---\n%s\n--- end ---", capturedBody)
	}
}
