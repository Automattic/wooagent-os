package httpapi

import (
	"testing"
)

// Locks the Sales Support customer_reply_draft dispatch contract.
// End-to-end approve flow is covered by the existing handler-level tests
// (the dispatch table is the single point of variance between proposal
// types). Here we only assert (a) the right ability is named and (b) the
// buildParams projection is correct for the typical inputs the Sales
// Support persona produces.

func TestApproveDispatch_CustomerReplyDraft_BuildParamsShape(t *testing.T) {
	d, ok := approveDispatchByType["customer_reply_draft"]
	if !ok {
		t.Fatalf("customer_reply_draft dispatch not registered")
	}
	if d.ability != "wooagent-orders/add-note" {
		t.Errorf("ability = %q, want wooagent-orders/add-note", d.ability)
	}

	type want struct {
		oid              int
		note             string
		isCustomerNote   bool
	}
	cases := []struct {
		name     string
		content  string
		target   map[string]any
		wantErr  bool
		wantP    want
	}{
		{
			name:    "default note_type → customer-facing",
			content: "Hi Maria,\n\nThanks for the wool throw order. — Elizabeth",
			target:  map[string]any{"order_id": 4711},
			wantP:   want{oid: 4711, note: "Hi Maria,\n\nThanks for the wool throw order. — Elizabeth", isCustomerNote: true},
		},
		{
			name:    "explicit customer note_type",
			content: "Hi Maria,\n\nFollowing up. — Elizabeth",
			target:  map[string]any{"order_id": 4711, "note_type": "customer"},
			wantP:   want{oid: 4711, note: "Hi Maria,\n\nFollowing up. — Elizabeth", isCustomerNote: true},
		},
		{
			name:    "internal note_type strips customer flag",
			content: "Watch this account — large order, on-hold for fraud screen.",
			target:  map[string]any{"order_id": 4711, "note_type": "internal"},
			wantP:   want{oid: 4711, note: "Watch this account — large order, on-hold for fraud screen.", isCustomerNote: false},
		},
		{
			name:    "internal note_type case-insensitive",
			content: "fyi",
			target:  map[string]any{"order_id": 4711, "note_type": "Internal"},
			wantP:   want{oid: 4711, note: "fyi", isCustomerNote: false},
		},
		{
			name:    "trims surrounding whitespace from note",
			content: "  hello  \n",
			target:  map[string]any{"order_id": 4711},
			wantP:   want{oid: 4711, note: "hello", isCustomerNote: true},
		},
		{
			name:    "empty content rejected",
			content: "   \n  \n",
			target:  map[string]any{"order_id": 4711},
			wantErr: true,
		},
		{
			name:    "missing order_id rejected",
			content: "hello",
			target:  map[string]any{"note_type": "customer"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params, _, err := d.buildParams(tc.content, nil, tc.target)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got params=%v", params)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if params["id"] != tc.wantP.oid {
				t.Errorf("id = %v, want %d", params["id"], tc.wantP.oid)
			}
			if params["note"] != tc.wantP.note {
				t.Errorf("note = %q, want %q", params["note"], tc.wantP.note)
			}
			if params["is_customer_note"] != tc.wantP.isCustomerNote {
				t.Errorf("is_customer_note = %v, want %v", params["is_customer_note"], tc.wantP.isCustomerNote)
			}
		})
	}
}
