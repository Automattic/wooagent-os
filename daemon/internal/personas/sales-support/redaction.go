// Package salessupport — see sales_support.go for the persona overview.
//
// This file holds the prompt-construction redaction layer. The Sales
// Support persona is the only persona in the daemon that sends real
// customer PII (name + email) to whichever LLM provider the operator
// has configured. redactOrderForPrompt is the single point at which
// that PII is dropped before it reaches the model.
//
// Drop-not-substitute: send the first name only; never include the
// customer's email or surname in the prompt. The model writes "Hi
// Maria,…" naturally because it's been handed a first name; nothing
// downstream needs to re-hydrate anything because nothing downstream
// ever saw a placeholder.
package salessupport

import "strings"

// promptContext is the redacted subset of an order that draftMessage
// passes to the LLM. It carries no email and no surname — only the
// first name and non-PII order fields the model needs to compose a
// reply. Adding a PII field here breaks the redaction guarantee and
// the rule-lock test (sales_support_redaction_test.go) will fail.
type promptContext struct {
	OrderNumber string
	OrderID     int // numeric fallback when Number is empty
	Status      string
	Total       string
	Currency    string
	DateCreated string
	FirstName   string
	LineItems   []lineItem
}

// redactOrderForPrompt builds a promptContext from a full order. It
// does not mutate o. The returned struct is the only data shape that
// crosses the prompt boundary on its way to the LLM provider.
func redactOrderForPrompt(o order) promptContext {
	return promptContext{
		OrderNumber: o.Number,
		OrderID:     o.ID,
		Status:      o.Status,
		Total:       o.Total,
		Currency:    o.Currency,
		DateCreated: o.DateCreated,
		FirstName:   firstName(o),
		LineItems:   o.LineItems,
	}
}

// firstName extracts the customer's first name from billing or
// shipping name, falling back to "the customer" so the prompt always
// has something to greet. The first space-separated token wins:
// "Maria del Carmen Rodriguez" → "Maria"; "Anne-Marie Smith" →
// "Anne-Marie"; "Madonna" → "Madonna".
func firstName(o order) string {
	name := strings.TrimSpace(firstNonEmpty(o.BillingName, o.ShippingName, ""))
	if name == "" {
		return "the customer"
	}
	if i := strings.Index(name, " "); i > 0 {
		return name[:i]
	}
	return name
}
