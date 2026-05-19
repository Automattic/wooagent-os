package marketing

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
)

func TestParseVariants_Happy(t *testing.T) {
	raw := `{"variants":[
		{"label":"A","angle":"material","body":"Cotton canvas, hand-dyed in small batches with natural indigo."},
		{"label":"B","angle":"use","body":"Drapes well across a sofa or bed. Made to last through years of use."},
		{"label":"C","angle":"story","body":"Hand-dyed by a small studio in Oaxaca. Each piece carries its own slight variation."}
	]}`
	vs, err := parseVariants(raw)
	if err != nil {
		t.Fatalf("parseVariants: %v", err)
	}
	if len(vs) != 3 {
		t.Fatalf("want 3 variants, got %d", len(vs))
	}
	if vs[0].ID != "var_a" || vs[1].ID != "var_b" || vs[2].ID != "var_c" {
		t.Errorf("ids = %q,%q,%q; want var_a,var_b,var_c", vs[0].ID, vs[1].ID, vs[2].ID)
	}
	if !vs[0].Recommended || vs[1].Recommended || vs[2].Recommended {
		t.Errorf("only the first variant should be recommended")
	}
	if vs[0].CharCount != len(vs[0].Body) {
		t.Errorf("charCount %d != body len %d", vs[0].CharCount, len(vs[0].Body))
	}
}

func TestParseVariants_TolerantOfChatter(t *testing.T) {
	raw := "Here is the JSON:\n```\n{\"variants\":[{\"label\":\"A\",\"body\":\"one\"},{\"label\":\"B\",\"body\":\"two\"},{\"label\":\"C\",\"body\":\"three\"}]}\n```\nLet me know if you want adjustments."
	vs, err := parseVariants(raw)
	if err != nil {
		t.Fatalf("parseVariants: %v", err)
	}
	if len(vs) != 3 {
		t.Fatalf("want 3 variants, got %d", len(vs))
	}
}

func TestParseVariants_RejectsWrongCount(t *testing.T) {
	raw := `{"variants":[{"label":"A","body":"only one"}]}`
	if _, err := parseVariants(raw); err == nil {
		t.Errorf("expected error on 1-variant payload, got nil")
	}
	raw = `{"variants":[]}`
	if _, err := parseVariants(raw); err == nil {
		t.Errorf("expected error on empty variants, got nil")
	}
}

func TestParseVariants_RejectsEmptyBody(t *testing.T) {
	raw := `{"variants":[{"label":"A","body":""},{"label":"B","body":"two"},{"label":"C","body":"three"}]}`
	if _, err := parseVariants(raw); err == nil || !strings.Contains(err.Error(), "empty body") {
		t.Errorf("expected empty-body error, got %v", err)
	}
}

func TestParseVariants_RejectsNonJSON(t *testing.T) {
	raw := "Sorry, I cannot complete this task."
	if _, err := parseVariants(raw); err == nil {
		t.Errorf("expected error on non-JSON output, got nil")
	}
}

func TestProductJSON_ImageFields(t *testing.T) {
	payload := []byte(`{
		"id": 42,
		"name": "Test Product",
		"sku": "SKU-42",
		"status": "publish",
		"description": "",
		"short_description": "",
		"permalink": "",
		"image_url": "https://store.example.com/wp-content/uploads/2024/01/test.jpg",
		"image_alt": "Test product alt text"
	}`)
	var p product
	if err := json.Unmarshal(payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.ImageURL != "https://store.example.com/wp-content/uploads/2024/01/test.jpg" {
		t.Errorf("ImageURL = %q, want full URL", p.ImageURL)
	}
	if p.ImageAlt != "Test product alt text" {
		t.Errorf("ImageAlt = %q, want full alt", p.ImageAlt)
	}
}

func TestProductJSON_ImageFieldsAbsent_DefaultsEmpty(t *testing.T) {
	payload := []byte(`{"id":1,"name":"x","sku":"","status":"publish","description":"","short_description":"","permalink":""}`)
	var p product
	if err := json.Unmarshal(payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.ImageURL != "" {
		t.Errorf("ImageURL = %q, want empty string", p.ImageURL)
	}
	if p.ImageAlt != "" {
		t.Errorf("ImageAlt = %q, want empty string", p.ImageAlt)
	}
}

func TestParseVariants_ValidScores(t *testing.T) {
	in := `{"variants":[
		{"label":"A","angle":"material","body":"hello","seo":80,"voice":75},
		{"label":"B","angle":"use","body":"world","seo":65,"voice":90},
		{"label":"C","angle":"story","body":"again","seo":100,"voice":50}
	]}`
	got, err := parseVariants(in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got[0].Seo != 80 || got[0].Voice != 75 {
		t.Errorf("variant 0: got seo=%d voice=%d, want 80/75", got[0].Seo, got[0].Voice)
	}
	if got[1].Seo != 65 || got[1].Voice != 90 {
		t.Errorf("variant 1: got seo=%d voice=%d, want 65/90", got[1].Seo, got[1].Voice)
	}
	if got[2].Seo != 100 || got[2].Voice != 50 {
		t.Errorf("variant 2: got seo=%d voice=%d, want 100/50", got[2].Seo, got[2].Voice)
	}
}

func TestParseVariants_OutOfRangeScoresClearedToZero(t *testing.T) {
	// Out-of-range scores → cleared to zero so omitempty drops them from
	// the persisted JSON. UI then renders `—` instead of a misleading number.
	in := `{"variants":[
		{"label":"A","angle":"material","body":"hello","seo":101,"voice":-5},
		{"label":"B","angle":"use","body":"world","seo":50,"voice":50},
		{"label":"C","angle":"story","body":"again","seo":50,"voice":50}
	]}`
	got, err := parseVariants(in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got[0].Seo != 0 {
		t.Errorf("variant 0 out-of-range seo: got %d, want 0", got[0].Seo)
	}
	if got[0].Voice != 0 {
		t.Errorf("variant 0 out-of-range voice: got %d, want 0", got[0].Voice)
	}
}

func TestParseVariants_MissingScoresAreZero(t *testing.T) {
	// Missing seo/voice fields → zero-value, which omitempty drops.
	in := `{"variants":[
		{"label":"A","angle":"material","body":"hello"},
		{"label":"B","angle":"use","body":"world"},
		{"label":"C","angle":"story","body":"again"}
	]}`
	got, err := parseVariants(in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got[0].Seo != 0 || got[0].Voice != 0 {
		t.Errorf("missing scores: got seo=%d voice=%d, want 0/0", got[0].Seo, got[0].Voice)
	}
}

func TestParseVariants_ZeroScoreIsClearedIndependently(t *testing.T) {
	// Each score is validated independently — an explicit 0 on one field
	// is cleared regardless of the other field's value. This is distinct
	// from TestParseVariants_MissingScoresAreZero, which covers absent
	// fields; here the LLM explicitly emitted 0 and the validator treats
	// that as the legacy "no scoring" default per DSGWOO-1326.
	in := `{"variants":[
		{"label":"A","angle":"material","body":"hello","seo":0,"voice":0},
		{"label":"B","angle":"use","body":"world","seo":80,"voice":80},
		{"label":"C","angle":"story","body":"again","seo":0,"voice":75}
	]}`
	got, err := parseVariants(in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got[0].Seo != 0 || got[0].Voice != 0 {
		t.Errorf("variant 0 both-zero: got seo=%d voice=%d, want 0/0", got[0].Seo, got[0].Voice)
	}
	if got[1].Seo != 80 || got[1].Voice != 80 {
		t.Errorf("variant 1 should keep its scores: got seo=%d voice=%d, want 80/80", got[1].Seo, got[1].Voice)
	}
	if got[2].Seo != 0 {
		t.Errorf("variant 2 asymmetric seo=0: got seo=%d, want 0", got[2].Seo)
	}
	if got[2].Voice != 75 {
		t.Errorf("variant 2 asymmetric voice=75: got voice=%d, want 75 (should NOT be cleared)", got[2].Voice)
	}
}

func TestFetchVoiceCorpus_FiltersExcludesAndSorts(t *testing.T) {
	// Stub MCP returning a mix of products. Asserts: filters out the
	// excluded product, sorts by description length descending, returns
	// the top 5.
	fake := &fakeMCP{
		// 6 published products + 1 draft + the excluded one
		listProductsResp: []byte(`{"products":[
			{"id":10,"name":"P10","status":"publish","description":"short"},
			{"id":11,"name":"P11","status":"publish","description":"aaaaaaaaaa bbbbbbbbbb cccccccccc"},
			{"id":12,"name":"P12","status":"publish","description":"medium length descrip"},
			{"id":13,"name":"P13","status":"publish","description":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"},
			{"id":14,"name":"P14","status":"publish","description":"yyy"},
			{"id":15,"name":"P15","status":"publish","description":"zzzzzzzzzzzzzzzz zzzzzzzzzzzz"},
			{"id":99,"name":"Excluded","status":"publish","description":"this product is the one being rewritten"},
			{"id":20,"name":"Draft","status":"draft","description":"should be filtered out by status"}
		]}`),
	}
	got, err := fetchVoiceCorpus(context.Background(), fake, 99)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("got %d corpus samples, want 5 (top-5 longest after exclusion)", len(got))
	}
	if got[0].Name != "P13" {
		t.Errorf("longest first: got %q, want P13", got[0].Name)
	}
	for _, s := range got {
		if s.Name == "Excluded" {
			t.Errorf("excluded product (id=99) leaked into corpus")
		}
		if s.Name == "Draft" {
			t.Errorf("non-publish status leaked into corpus")
		}
	}
}

func TestBuildPromptUserMessage_IncludesProductAndCorpus(t *testing.T) {
	p := product{
		Name:        "Indigo Throw Pillow",
		SKU:         "PIL-IND-22",
		Description: "Existing thin description.",
	}
	corpus := []corpusSample{
		{Name: "Stoneware Mug", Body: "Body fired in our wood kiln. Holds 12oz. Hand-thrown."},
		{Name: "Cashmere Scarf", Body: "Plate-loomed in the Loire valley. 200g of two-ply yarn."},
	}
	msg := buildPromptUserMessage(p, corpus, draftOpts{})

	// Product fields appear.
	for _, want := range []string{"Indigo Throw Pillow", "PIL-IND-22", "Existing thin description."} {
		if !strings.Contains(msg, want) {
			t.Errorf("user message missing %q\n---\n%s", want, msg)
		}
	}
	// Corpus samples appear.
	for _, want := range []string{
		"Voice corpus",
		"Stoneware Mug",
		"Body fired in our wood kiln",
		"Cashmere Scarf",
		"Plate-loomed in the Loire valley",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("user message missing corpus marker %q\n---\n%s", want, msg)
		}
	}
	// Negative: with a real corpus, the "Emit null" instruction must NOT
	// appear — that path is reserved for the empty-corpus case.
	if strings.Contains(msg, "Emit null") {
		t.Errorf("non-empty-corpus message should NOT contain 'Emit null'\n---\n%s", msg)
	}
}

func TestBuildPromptUserMessage_EmptyCorpus(t *testing.T) {
	p := product{Name: "New Store Product", SKU: "NEW-1", Description: "hi"}
	msg := buildPromptUserMessage(p, nil, draftOpts{})
	// Empty corpus → prompt instructs the LLM to emit null for voice.
	if !strings.Contains(msg, "Voice corpus: (none available") {
		t.Errorf("empty-corpus message should mark the gap explicitly\n---\n%s", msg)
	}
	if !strings.Contains(msg, "Emit null for") {
		t.Errorf("empty-corpus message should explicitly instruct emitting null for voice\n---\n%s", msg)
	}
}

func TestBuildPromptUserMessage_EmptyDescription(t *testing.T) {
	// Empty description still produces a well-formed message — the
	// "Current description:" label stays, even with an empty value
	// (signals "no current copy" to the LLM, which is exactly when
	// marketing's job kicks in).
	p := product{Name: "Blank Product", SKU: "BLANK-1", Description: ""}
	corpus := []corpusSample{{Name: "Example", Body: "An existing description."}}
	msg := buildPromptUserMessage(p, corpus, draftOpts{})

	if !strings.Contains(msg, "Current description: \n") {
		t.Errorf("empty description should still surface the label with an empty value\n---\n%s", msg)
	}
	if !strings.Contains(msg, "Blank Product") {
		t.Errorf("product name still present\n---\n%s", msg)
	}
}

// fakeMCP implements just enough of *mcp.Client for fetchVoiceCorpus.
// CallTool returns the canned bytes; everything else panics so a wrong
// invocation surfaces immediately.
type fakeMCP struct {
	listProductsResp []byte
}

func (f *fakeMCP) CallTool(ctx context.Context, name string, args any) (mcp.ToolCallResult, error) {
	// Mirrors callAbility's envelope shape.
	envelope := fmt.Sprintf(`{"success":true,"data":%s}`, string(f.listProductsResp))
	return mcp.ToolCallResult{Content: []mcp.ContentPart{{Text: envelope}}}, nil
}

func TestParseColdDraftVariants_BothFields(t *testing.T) {
	raw := `{"variants":[
		{"label":"A","angle":"warm","body_short":"Cozy wool slippers.","body_long":"Handcrafted from 100% merino wool...","seo":85,"voice":92},
		{"label":"B","angle":"informational","body_short":"100% merino wool slippers.","body_long":"Pure merino wool, 100% indoor wear...","seo":80,"voice":85},
		{"label":"C","angle":"minimal","body_short":"Wool slippers.","body_long":"Merino wool. Indoor.","seo":70,"voice":78}
	]}`
	got, err := parseColdDraftVariants(raw, []string{"short", "long"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].BodyShort == "" || got[0].BodyLong == "" {
		t.Errorf("variant A missing structured body fields: %+v", got[0])
	}
	if got[0].Body != "" {
		t.Errorf("variant A should have empty Body, got %q", got[0].Body)
	}
	if !got[0].Recommended {
		t.Errorf("first variant should have Recommended=true")
	}
}

func TestParseColdDraftVariants_LongOnly(t *testing.T) {
	raw := `{"variants":[
		{"label":"A","body_long":"Handcrafted...","seo":85,"voice":92},
		{"label":"B","body_long":"Pure merino...","seo":80,"voice":85},
		{"label":"C","body_long":"Merino. Indoor.","seo":70,"voice":78}
	]}`
	got, err := parseColdDraftVariants(raw, []string{"long"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got[0].BodyShort != "" {
		t.Errorf("BodyShort should be empty when drafting=[long]; got %q", got[0].BodyShort)
	}
	if got[0].BodyLong == "" {
		t.Errorf("BodyLong should be populated")
	}
}

func TestParseColdDraftVariants_MissingDraftedField_Errors(t *testing.T) {
	raw := `{"variants":[
		{"label":"A","body_long":"..."},
		{"label":"B","body_short":"s","body_long":"l"},
		{"label":"C","body_short":"s","body_long":"l"}
	]}`
	_, err := parseColdDraftVariants(raw, []string{"short", "long"})
	if err == nil {
		t.Fatalf("expected error for missing body_short on variant A")
	}
}

func TestVariant_StructuredBody_RoundTrip(t *testing.T) {
	v := variant{
		ID:        "var_a",
		Label:     "A",
		BodyShort: "Cozy wool slippers for cold floors.",
		BodyLong:  "Handcrafted from 100% merino wool ...",
		CharCount: 100,
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back variant
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.BodyShort != v.BodyShort {
		t.Errorf("body_short = %q, want %q", back.BodyShort, v.BodyShort)
	}
	if back.BodyLong != v.BodyLong {
		t.Errorf("body_long = %q, want %q", back.BodyLong, v.BodyLong)
	}
	if back.Body != "" {
		t.Errorf("body = %q, want empty for cold-draft variant", back.Body)
	}
}

func TestPickColdDraftCandidates_ReturnsEmptyFieldsOnly(t *testing.T) {
	resp := []byte(`{"products":[
		{"id":1,"name":"P1","status":"publish","description_length":100,"short_description_length":50},
		{"id":2,"name":"P2","status":"publish","description_length":100,"short_description_length":0},
		{"id":3,"name":"P3","status":"publish","description_length":0,"short_description_length":50},
		{"id":4,"name":"P4","status":"publish","description_length":0,"short_description_length":0}
	]}`)
	fake := &fakeMCP{listProductsResp: resp}
	got, err := pickColdDraftCandidates(context.Background(), fake, nil, 10)
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	wantIDs := []int{2, 3, 4}
	if len(got) != len(wantIDs) {
		t.Fatalf("len = %d, want %d (ids %v)", len(got), len(wantIDs), got)
	}
	for i, p := range got {
		if p.ID != wantIDs[i] {
			t.Errorf("[%d] id = %d, want %d", i, p.ID, wantIDs[i])
		}
	}
}

func TestPickColdDraftCandidates_HonorsCooldown(t *testing.T) {
	resp := []byte(`{"products":[
		{"id":2,"name":"P2","status":"publish","description_length":100,"short_description_length":0},
		{"id":3,"name":"P3","status":"publish","description_length":0,"short_description_length":50}
	]}`)
	fake := &fakeMCP{listProductsResp: resp}
	skip := map[int]struct{}{2: {}}
	got, err := pickColdDraftCandidates(context.Background(), fake, skip, 10)
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if len(got) != 1 || got[0].ID != 3 {
		t.Errorf("expected only product 3 (P2 in cooldown); got %+v", got)
	}
}

func TestPickColdDraftCandidates_RespectsMax(t *testing.T) {
	// 15 candidates all with empty descriptions; max=10 caps the slice.
	var items []string
	for i := 1; i <= 15; i++ {
		items = append(items, fmt.Sprintf(
			`{"id":%d,"name":"P%d","status":"publish","description_length":0,"short_description_length":0}`,
			i, i,
		))
	}
	resp := []byte("{\"products\":[" + strings.Join(items, ",") + "]}")
	fake := &fakeMCP{listProductsResp: resp}
	got, err := pickColdDraftCandidates(context.Background(), fake, nil, 10)
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if len(got) != 10 {
		t.Errorf("len = %d, want 10 (capped)", len(got))
	}
}

func TestComputeDrafting(t *testing.T) {
	cases := []struct {
		name     string
		p        product
		expected []string
	}{
		{"both empty", product{ShortDesc: "", Description: ""}, []string{"short", "long"}},
		{"short only", product{ShortDesc: "", Description: "filled"}, []string{"short"}},
		{"long only", product{ShortDesc: "filled", Description: ""}, []string{"long"}},
		{"neither", product{ShortDesc: "s", Description: "l"}, []string{}},
		{"whitespace counts as empty", product{ShortDesc: "  ", Description: "\n\t"}, []string{"short", "long"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := computeDrafting(tc.p)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("got %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestBuildPromptUserMessage_ColdDraftMode(t *testing.T) {
	p := product{
		Name: "Wool Slippers", SKU: "wool-slippers",
		ShortDesc: "", Description: "",
	}
	msg := buildPromptUserMessage(p, nil, draftOpts{Mode: "cold_draft", Drafting: []string{"short", "long"}})
	if !strings.Contains(msg, "Current short description:") {
		t.Errorf("cold-draft message should label short/long current separately:\n%s", msg)
	}
	if !strings.Contains(msg, "Drafting fields: short, long") {
		t.Errorf("cold-draft message should declare which fields to draft:\n%s", msg)
	}
	if strings.Contains(msg, "Write the THREE rewrite variants") {
		t.Errorf("cold-draft message should not use rewrite phrasing:\n%s", msg)
	}
}

func TestBuildPromptUserMessage_RewriteMode_Unchanged(t *testing.T) {
	p := product{
		Name: "Wool Slippers", SKU: "wool-slippers",
		Description: "Existing description.",
	}
	msg := buildPromptUserMessage(p, nil, draftOpts{})
	if !strings.Contains(msg, "Current description: Existing description.") {
		t.Errorf("rewrite mode should use the single-current-description format:\n%s", msg)
	}
	if !strings.Contains(msg, "Write the THREE rewrite variants") {
		t.Errorf("rewrite mode should keep its existing phrasing:\n%s", msg)
	}
}

func TestDraftColdDraftBatch_PacksAsSiblings(t *testing.T) {
	// 4 successful drafts → 1 primary + 3 siblings, title "Review & approve · 4 ..."
	cands := []productSummary{
		{ID: 11, Name: "P11"}, {ID: 12, Name: "P12"},
		{ID: 13, Name: "P13"}, {ID: 14, Name: "P14"},
	}
	drafterFn := func(_ context.Context, _ personas.Deps, p productSummary, _ string) (personas.Drafted, error) {
		return personas.Drafted{
			Title:        fmt.Sprintf("draft %d", p.ID),
			ProposalType: "product_cold_draft",
			DedupKey:     fmt.Sprintf("product:%d", p.ID),
			Target:       map[string]any{"product_id": p.ID},
		}, nil
	}
	got, err := draftColdDraftBatch(context.Background(), personas.Deps{}, cands, "skill desc", drafterFn)
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if got.BatchTitle != "Review & approve · 4 product descriptions" {
		t.Errorf("batch_title = %q", got.BatchTitle)
	}
	if got.BatchIntent != "fill_missing_copy" {
		t.Errorf("batch_intent = %q", got.BatchIntent)
	}
	if len(got.BatchSiblings) != 3 {
		t.Errorf("siblings = %d, want 3", len(got.BatchSiblings))
	}
}

func TestDraftColdDraftBatch_DropsErrors(t *testing.T) {
	// 4 candidates; second errors. Expect batch with 3 children.
	cands := []productSummary{
		{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4},
	}
	drafterFn := func(_ context.Context, _ personas.Deps, p productSummary, _ string) (personas.Drafted, error) {
		if p.ID == 2 {
			return personas.Drafted{}, fmt.Errorf("network glitch")
		}
		return personas.Drafted{Title: fmt.Sprintf("d%d", p.ID), ProposalType: "product_cold_draft"}, nil
	}
	got, err := draftColdDraftBatch(context.Background(), personas.Deps{}, cands, "skill", drafterFn)
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if got.Skipped {
		t.Fatalf("expected non-skipped result, got Skipped=%q", got.SkipReason)
	}
	total := 1 + len(got.BatchSiblings)
	if total != 3 {
		t.Errorf("expected 3 children (1 primary + 2 siblings), got %d total", total)
	}
}

func TestDraftColdDraftBatch_DropsSkippedDrafts(t *testing.T) {
	// 4 candidates; second returns Skipped. Same expected outcome as above.
	cands := []productSummary{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}}
	drafterFn := func(_ context.Context, _ personas.Deps, p productSummary, _ string) (personas.Drafted, error) {
		if p.ID == 2 {
			return personas.Drafted{Skipped: true, SkipReason: "race"}, nil
		}
		return personas.Drafted{Title: fmt.Sprintf("d%d", p.ID), ProposalType: "product_cold_draft"}, nil
	}
	got, _ := draftColdDraftBatch(context.Background(), personas.Deps{}, cands, "skill", drafterFn)
	total := 1 + len(got.BatchSiblings)
	if total != 3 {
		t.Errorf("expected 3 children, got %d", total)
	}
}

func TestDraftColdDraftBatch_BelowThresholdReturnsSkipped(t *testing.T) {
	// 3 candidates, 2 of which error. Only 1 survives → below coldDraftMin=3 → Skipped.
	cands := []productSummary{{ID: 1}, {ID: 2}, {ID: 3}}
	drafterFn := func(_ context.Context, _ personas.Deps, p productSummary, _ string) (personas.Drafted, error) {
		if p.ID != 1 {
			return personas.Drafted{}, fmt.Errorf("nope")
		}
		return personas.Drafted{Title: "d1", ProposalType: "product_cold_draft"}, nil
	}
	got, err := draftColdDraftBatch(context.Background(), personas.Deps{}, cands, "skill", drafterFn)
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if !got.Skipped {
		t.Fatalf("expected Skipped=true, got %+v", got)
	}
	if !strings.Contains(got.SkipReason, "only 1") || !strings.Contains(got.SkipReason, "need 3") {
		t.Errorf("SkipReason should explain threshold: %q", got.SkipReason)
	}
}
