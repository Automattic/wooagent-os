package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestProduceReplyDraft_HappyPath asserts the tool writes an Issue + Run
// pair with the cadence-mode customer_reply_draft shape — title with
// order number + subject, proposal_type=customer_reply_draft, note_type
// preserved in target, body as proposal_content.
func TestProduceReplyDraft_HappyPath(t *testing.T) {
	db := newTestDB(t)
	ensureAgent(t, db, "sales-support")
	tool := &ProduceReplyDraftTool{DB: db}

	input, _ := json.Marshal(produceReplyDraftInput{
		OrderID:       4521,
		OrderNumber:   "4521",
		OrderStatus:   "completed",
		OrderTotal:    "82.00",
		OrderCurrency: "USD",
		CustomerEmail: "p@example.com",
		CustomerName:  "Priya Kumar",
		NoteType:      "refund_apology",
		SubjectHint:   "Re: your Linen Napkin order",
		Body:          "Hi Priya, sorry the order arrived damaged. I'd like to refund you for the napkins and send a replacement set out tomorrow. Let me know if that works. — Elizabeth",
	})
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var resp produceOutput
	json.Unmarshal([]byte(out), &resp)
	if !resp.OK || resp.ProposalID == "" {
		t.Fatalf("expected ok+id, got %+v", resp)
	}

	var title, ptype, pcontent, ptargetJSON, dedup string
	db.QueryRow(
		`SELECT title, proposal_type, proposal_content, proposal_target, dedup_key
		   FROM issues WHERE id = ?`, resp.ProposalID,
	).Scan(&title, &ptype, &pcontent, &ptargetJSON, &dedup)
	if !strings.Contains(title, "#4521") {
		t.Errorf("title %q missing order number", title)
	}
	if !strings.Contains(title, "Linen Napkin") {
		t.Errorf("title %q missing subject hint", title)
	}
	if ptype != "customer_reply_draft" {
		t.Errorf("proposal_type = %q, want customer_reply_draft", ptype)
	}
	if !strings.Contains(pcontent, "sorry the order arrived damaged") {
		t.Errorf("proposal_content should be the reply body, got %q", pcontent)
	}
	if dedup != "order:4521" {
		t.Errorf("dedup_key = %q, want order:4521", dedup)
	}
	var target map[string]any
	json.Unmarshal([]byte(ptargetJSON), &target)
	if target["note_type"] != "refund_apology" {
		t.Errorf("target.note_type = %v, want refund_apology", target["note_type"])
	}
}

// TestProduceReplyDraft_RejectsBadInput covers note_type enum, missing
// order_id, missing body, missing subject_hint.
func TestProduceReplyDraft_RejectsBadInput(t *testing.T) {
	db := newTestDB(t)
	ensureAgent(t, db, "sales-support")
	tool := &ProduceReplyDraftTool{DB: db}

	cases := []struct {
		name string
		in   produceReplyDraftInput
	}{
		{
			name: "invalid note_type",
			in: produceReplyDraftInput{
				OrderID:     1,
				NoteType:    "bogus",
				SubjectHint: "x",
				Body:        "y",
			},
		},
		{
			name: "missing order_id",
			in: produceReplyDraftInput{
				NoteType:    "shipping_update",
				SubjectHint: "x",
				Body:        "y",
			},
		},
		{
			name: "missing body",
			in: produceReplyDraftInput{
				OrderID:     1,
				NoteType:    "shipping_update",
				SubjectHint: "x",
			},
		},
		{
			name: "missing subject_hint",
			in: produceReplyDraftInput{
				OrderID:  1,
				NoteType: "shipping_update",
				Body:     "y",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input, _ := json.Marshal(tc.in)
			if _, err := tool.Execute(context.Background(), input); err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

// TestSalesSupportTools_Definitions sanity-checks tool names.
func TestSalesSupportTools_Definitions(t *testing.T) {
	cases := []struct{ want, got string }{
		{"list_orders", (&ListOrdersTool{}).Definition().Name},
		{"get_order", (&GetOrderTool{}).Definition().Name},
		{"produce_reply_draft", (&ProduceReplyDraftTool{}).Definition().Name},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("name = %q, want %q", tc.got, tc.want)
		}
	}
}
