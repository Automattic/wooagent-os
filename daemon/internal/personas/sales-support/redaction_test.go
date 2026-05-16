package salessupport

import "testing"

func TestFirstName(t *testing.T) {
	cases := []struct {
		name string
		in   order
		want string
	}{
		{"standard billing name", order{BillingName: "Maria Rodriguez"}, "Maria"},
		{"empty billing, has shipping", order{ShippingName: "Tom Lee"}, "Tom"},
		{"guest order, no name anywhere", order{}, "the customer"},
		{"single name, no surname", order{BillingName: "Madonna"}, "Madonna"},
		{"hyphenated first name", order{BillingName: "Anne-Marie Smith"}, "Anne-Marie"},
		{"multi-word given name", order{BillingName: "Maria del Carmen Rodriguez"}, "Maria"},
		{"all-caps preserved", order{BillingName: "JOHN SMITH"}, "JOHN"},
		{"surrounding whitespace stripped", order{BillingName: "  Bob Jones  "}, "Bob"},
		{"whitespace-only billing falls back", order{BillingName: "   "}, "the customer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := firstName(tc.in)
			if got != tc.want {
				t.Errorf("firstName(%+v) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRedactOrderForPromptCarriesNonPIIFields(t *testing.T) {
	o := order{
		ID:            42,
		Number:        "1042",
		Status:        "processing",
		Total:         "59.95",
		Currency:      "USD",
		DateCreated:   "2026-05-14T10:00:00",
		BillingName:   "Maria Rodriguez",
		CustomerEmail: "maria@example.com",
		LineItems: []lineItem{
			{ProductID: 7, Name: "Indigo Throw Pillow", Quantity: 1, Total: "59.95", SKU: "ITP-001"},
		},
	}
	pc := redactOrderForPrompt(o)

	// PII-redaction guarantee — structural enforcement.
	//
	// Rule: promptContext must NEVER carry the customer's email address or surname.
	//
	// Today this rule is enforced by the type system: promptContext has no
	// CustomerEmail field, so the absence is compile-time-guaranteed rather than
	// assertion-enforced. A future developer adding a CustomerEmail (or Surname)
	// field to promptContext MUST also add assertions here, e.g.:
	//
	//   if pc.CustomerEmail != "" {
	//       t.Errorf("CustomerEmail leaked into promptContext: %q", pc.CustomerEmail)
	//   }
	//
	// Defensive check: FirstName must hold only the given name, never the surname.
	// (The existing FirstName == "Maria" check below would pass even if this
	// field accidentally contained "Rodriguez".)
	if pc.FirstName == "Rodriguez" {
		t.Errorf("surname leaked into FirstName: %q", pc.FirstName)
	}

	if pc.OrderNumber != "1042" {
		t.Errorf("OrderNumber = %q; want %q", pc.OrderNumber, "1042")
	}
	if pc.Status != "processing" {
		t.Errorf("Status = %q; want %q", pc.Status, "processing")
	}
	if pc.Total != "59.95" || pc.Currency != "USD" {
		t.Errorf("Total/Currency = %q/%q; want 59.95/USD", pc.Total, pc.Currency)
	}
	if pc.DateCreated != "2026-05-14T10:00:00" {
		t.Errorf("DateCreated = %q; want %q", pc.DateCreated, "2026-05-14T10:00:00")
	}
	if pc.FirstName != "Maria" {
		t.Errorf("FirstName = %q; want %q", pc.FirstName, "Maria")
	}
	if len(pc.LineItems) != 1 || pc.LineItems[0].SKU != "ITP-001" {
		t.Errorf("LineItems not carried through: %+v", pc.LineItems)
	}
}
