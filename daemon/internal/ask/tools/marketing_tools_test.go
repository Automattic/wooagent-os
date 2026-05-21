package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestProduceDescriptionRewrite_HappyPath asserts that calling the
// produce_description_rewrite tool with a valid 3-variant input writes
// both an Issue row (status=in_review, proposal_type=product_description_rewrite,
// the right target JSON) AND a Run row (trigger=operator-asked,
// status=succeeded, linked to the issue id).
func TestProduceDescriptionRewrite_HappyPath(t *testing.T) {
	db := newTestDB(t)
	ensureAgent(t, db, "marketing")

	tool := &ProduceDescriptionRewriteTool{DB: db}
	input, _ := json.Marshal(produceDescriptionRewriteInput{
		ProductID:   42,
		ProductName: "Linen Napkin",
		ProductSKU:  "LN-001",
		Previous:    "Old description.",
		Variants: []rewriteVariant{
			{Body: "Variant A body, woven from European linen for everyday use."},
			{Body: "Variant B body, soft to the touch and made to last."},
			{Body: "Variant C body, simple and durable for the dinner table."},
		},
	})

	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var resp produceOutput
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if !resp.OK || resp.ProposalID == "" {
		t.Fatalf("expected ok=true with proposal_id, got %+v", resp)
	}
	if resp.ProposalType != "product_description_rewrite" {
		t.Errorf("proposal_type = %q, want product_description_rewrite", resp.ProposalType)
	}

	// Issue row was written with the right shape.
	var (
		title       string
		persona     string
		status      string
		ptype       string
		pcontent    string
		ptargetJSON string
		dedup       string
	)
	err = db.QueryRow(
		`SELECT title, persona, status, proposal_type, proposal_content, proposal_target, dedup_key
		   FROM issues WHERE id = ?`, resp.ProposalID,
	).Scan(&title, &persona, &status, &ptype, &pcontent, &ptargetJSON, &dedup)
	if err != nil {
		t.Fatalf("read issue: %v", err)
	}
	if !strings.Contains(title, "Linen Napkin") {
		t.Errorf("title %q missing product name", title)
	}
	if persona != "marketing" {
		t.Errorf("persona = %q, want marketing", persona)
	}
	if status != "in_review" {
		t.Errorf("status = %q, want in_review", status)
	}
	if ptype != "product_description_rewrite" {
		t.Errorf("proposal_type = %q, want product_description_rewrite", ptype)
	}
	if !strings.Contains(pcontent, "Variant A body") {
		t.Errorf("proposal_content missing variant A, got %q", pcontent)
	}
	if dedup != "product:42" {
		t.Errorf("dedup_key = %q, want product:42", dedup)
	}
	var target map[string]any
	if err := json.Unmarshal([]byte(ptargetJSON), &target); err != nil {
		t.Fatalf("parse target: %v", err)
	}
	if id, _ := target["product_id"].(float64); int(id) != 42 {
		t.Errorf("target.product_id = %v, want 42", target["product_id"])
	}
	variants, ok := target["variants"].([]any)
	if !ok || len(variants) != 3 {
		t.Errorf("target.variants should be a 3-entry array, got %v", target["variants"])
	}

	// Run row was written with trigger=operator-asked, linked to issue.
	var (
		runPersona string
		runTrigger string
		runStatus  string
		runIssueID string
	)
	err = db.QueryRow(
		`SELECT persona, trigger, status, COALESCE(issue_id, '')
		   FROM runs WHERE issue_id = ?`, resp.ProposalID,
	).Scan(&runPersona, &runTrigger, &runStatus, &runIssueID)
	if err != nil {
		t.Fatalf("read run: %v", err)
	}
	if runTrigger != "operator-asked" {
		t.Errorf("run trigger = %q, want operator-asked", runTrigger)
	}
	if runStatus != "succeeded" {
		t.Errorf("run status = %q, want succeeded", runStatus)
	}
	if runIssueID != resp.ProposalID {
		t.Errorf("run.issue_id = %q, want %q", runIssueID, resp.ProposalID)
	}
}

// TestProduceDescriptionRewrite_RejectsBadInput covers the input
// validation paths — empty variants, wrong variant count, missing
// product fields. None of these should write a row.
func TestProduceDescriptionRewrite_RejectsBadInput(t *testing.T) {
	db := newTestDB(t)
	ensureAgent(t, db, "marketing")
	tool := &ProduceDescriptionRewriteTool{DB: db}

	cases := []struct {
		name string
		in   produceDescriptionRewriteInput
	}{
		{
			name: "missing product id",
			in: produceDescriptionRewriteInput{
				ProductName: "X",
				Variants: []rewriteVariant{
					{Body: "a"}, {Body: "b"}, {Body: "c"},
				},
			},
		},
		{
			name: "missing product name",
			in: produceDescriptionRewriteInput{
				ProductID: 1,
				Variants: []rewriteVariant{
					{Body: "a"}, {Body: "b"}, {Body: "c"},
				},
			},
		},
		{
			name: "two variants",
			in: produceDescriptionRewriteInput{
				ProductID:   1,
				ProductName: "X",
				Variants: []rewriteVariant{
					{Body: "a"}, {Body: "b"},
				},
			},
		},
		{
			name: "empty body",
			in: produceDescriptionRewriteInput{
				ProductID:   1,
				ProductName: "X",
				Variants: []rewriteVariant{
					{Body: "a"}, {Body: ""}, {Body: "c"},
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input, _ := json.Marshal(tc.in)
			if _, err := tool.Execute(context.Background(), input); err == nil {
				t.Errorf("expected error, got nil")
			}
			var n int
			db.QueryRow(`SELECT count(*) FROM issues WHERE persona = 'marketing'`).Scan(&n)
			if n != 0 {
				t.Errorf("issue table should be empty after failed validation, got %d rows", n)
			}
		})
	}
}

// TestProduceSocialPost_HappyPath covers the social_post variant.
// Same writer underneath as launch_copy, so one test exercises the
// happy-path shape; launch_copy gets a smoke test below.
func TestProduceSocialPost_HappyPath(t *testing.T) {
	db := newTestDB(t)
	ensureAgent(t, db, "marketing")
	tool := &ProduceSocialPostTool{DB: db}

	input, _ := json.Marshal(produceSocialPostInput{
		TargetProducts: []productRef{
			{ID: 1, Name: "Linen Napkin", SKU: "LN-001"},
			{ID: 2, Name: "Wool Throw", SKU: "WT-001"},
		},
		Body: "A set for cozy mornings.",
	})
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var resp produceOutput
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if resp.ProposalType != "social_post" {
		t.Errorf("proposal_type = %q, want social_post", resp.ProposalType)
	}
	var title, dedup string
	db.QueryRow(`SELECT title, dedup_key FROM issues WHERE id = ?`, resp.ProposalID).Scan(&title, &dedup)
	if !strings.Contains(title, "Linen Napkin") || !strings.Contains(title, "Wool Throw") {
		t.Errorf("title %q missing product names", title)
	}
	if dedup != "social_post:products:1,2" {
		t.Errorf("dedup_key = %q, want social_post:products:1,2", dedup)
	}
}

// TestProduceLaunchCopy_Smoke confirms the launch_copy tool writes with
// the correct proposal_type / dedup namespace. Coverage of the shared
// validation path is in the social_post / description_rewrite tests.
func TestProduceLaunchCopy_Smoke(t *testing.T) {
	db := newTestDB(t)
	ensureAgent(t, db, "marketing")
	tool := &ProduceLaunchCopyTool{DB: db}
	input, _ := json.Marshal(produceSocialPostInput{
		TargetProducts: []productRef{{ID: 7, Name: "Holiday Towel"}},
		Body:           "Soft and warm for cold mornings.",
	})
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var resp produceOutput
	json.Unmarshal([]byte(out), &resp)
	var ptype, dedup string
	db.QueryRow(`SELECT proposal_type, dedup_key FROM issues WHERE id = ?`, resp.ProposalID).Scan(&ptype, &dedup)
	if ptype != "launch_copy" {
		t.Errorf("proposal_type = %q, want launch_copy", ptype)
	}
	if dedup != "launch_copy:products:7" {
		t.Errorf("dedup_key = %q, want launch_copy:products:7", dedup)
	}
}

// TestMarketingTools_Definitions sanity-checks that each tool advertises
// the expected name. Cheap insurance against accidental renames.
func TestMarketingTools_Definitions(t *testing.T) {
	cases := []struct {
		want string
		got  string
	}{
		{"list_products", (&ListProductsTool{}).Definition().Name},
		{"get_product", (&GetProductTool{}).Definition().Name},
		{"produce_description_rewrite", (&ProduceDescriptionRewriteTool{}).Definition().Name},
		{"produce_social_post", (&ProduceSocialPostTool{}).Definition().Name},
		{"produce_launch_copy", (&ProduceLaunchCopyTool{}).Definition().Name},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("tool name = %q, want %q", tc.got, tc.want)
		}
	}
}
