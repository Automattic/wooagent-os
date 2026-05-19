# Marketing cold-draft batches — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When Marketing's tick finds ≥3 products with empty `short_description` or `long_description` out of cooldown, emit a batch of drafted descriptions rendered through `BatchReview`. Retire the Reporting `product_health_digest` Draft path.

**Architecture:** Smart `Draft()` in `marketing.go` decides per tick: cold-draft batch (≥3 candidates, cap 10) or existing single-rewrite. Cold-draft uses the existing `BatchSiblings` packaging pattern from Pricing. New skill mode `cold_draft` produces variants with structured short/long bodies. UI extends `BatchReview`'s Marketing rendering path to a two-cell Current column and structured variant columns. New approve dispatch entry handles the `wooagent-products/update` payload with only previously-empty fields.

**Tech Stack:** Go 1.22 + SQLite + Cobra (daemon), TypeScript + React + WPDS (`@wordpress/ui` + `@wordpress/components`) (UI), WC AI Companion Plugin via MCP for catalog reads/writes.

**Design spec:** `docs/specs/2026-05-18-marketing-cold-draft-batch-design.md`.

---

## File structure

**Go (daemon) — modify:**
- `daemon/internal/personas/marketing/marketing.go` — extend `variant` struct, add `parseColdDraftVariants`, `pickColdDraftCandidates`, `draftColdDraftForProduct`, `draftColdDraftBatch`, branch `Draft()`.
- `daemon/internal/personas/marketing/marketing_test.go` — new tests for the cold-draft path.
- `daemon/internal/personas/reporting/reporting.go` — strip `Draft()` body, delete `findProductsWithDataIssues`, `buildDataIssuesDigest`, `renderDigest`. Persona stays registered.
- `daemon/internal/personas/reporting/reporting_test.go` — remove tests for the deleted helpers; add a test confirming `Draft()` returns `Skipped:true`.
- `daemon/internal/httpapi/handlers_v1.go` — add `"product_cold_draft"` entry to `approveDispatchByType`; teach approve flow to read structured variant bodies.
- `daemon/internal/httpapi/handlers_batches_test.go` — add a cold-draft fixture variant for the approve test.

**Go (daemon) — modify (prompt template):**
- `prompts/skills/marketing-description-rewrite/v1.yaml` — accept a `mode` parameter (`rewrite` default for back-compat, `cold_draft` new). Add a cold-draft prompt block with a structured output schema; drop the "rewrite the existing copy" framing.

**TypeScript (UI) — modify:**
- `ui/src/api/client.ts` — extend `Variant` interface with optional `body_short` / `body_long` fields; update `variantsFromProposal` to pass them through.
- `ui/src/screens/BatchReview.tsx` — detect cold-draft proposals (`firstProposal?.type === 'product_cold_draft'`), render structured Current column + variant columns, swap KPI hint copy, narrow footer text.

**Specs (already written):**
- `docs/specs/2026-05-18-marketing-cold-draft-batch-design.md` — the design spec (committed at cb12f2a on this branch).

---

## Conventions used throughout this plan

- **Run all Go tests** with `cd /Users/elizabethpizzuti/claude/wooagent/daemon && go test ./...`.
- **Run focused Go tests** with `go test ./internal/personas/marketing/ -run TestName -v`.
- **Run UI typecheck** with `cd /Users/elizabethpizzuti/claude/wooagent/ui && npm run typecheck`.
- **Run UI tests** with `cd /Users/elizabethpizzuti/claude/wooagent/ui && npm test`.
- **Branch:** `feat/dsgwoo-marketing-cold-draft` (already created off `trunk`).
- **Commits:** small, one per task. Trailing `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`.
- **No `--no-verify`, no force-push, no commits to `trunk` directly.**

---

## Task 1: Extend `variant` struct with optional structured-body fields

**Files:**
- Modify: `daemon/internal/personas/marketing/marketing.go:42-51` (variant struct)
- Test: `daemon/internal/personas/marketing/marketing_test.go`

- [ ] **Step 1: Write a failing test for variant JSON round-trip with body_short / body_long.**

Append to `marketing_test.go`:

```go
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
	// Body is the legacy single-body field; should be empty for cold-draft.
	if back.Body != "" {
		t.Errorf("body = %q, want empty for cold-draft variant", back.Body)
	}
}
```

- [ ] **Step 2: Run the test, confirm compile failure.**

```
cd /Users/elizabethpizzuti/claude/wooagent/daemon
go test ./internal/personas/marketing/ -run TestVariant_StructuredBody_RoundTrip -v
```

Expected: compile error mentioning `BodyShort` / `BodyLong` undefined.

- [ ] **Step 3: Extend the struct.**

Edit `marketing.go:42-51`. The new struct:

```go
// variant is the persisted shape. label/seo/voice optional; Body is the
// single-body case (rewrite + fallback); BodyShort/BodyLong populated for
// cold-draft variants where the agent fills missing description fields.
// Exactly one of (Body) or (BodyShort | BodyLong) is set per variant.
type variant struct {
	ID          string `json:"id"`
	Label       string `json:"label,omitempty"`
	Body        string `json:"body,omitempty"`
	BodyShort   string `json:"body_short,omitempty"`
	BodyLong    string `json:"body_long,omitempty"`
	CharCount   int    `json:"charCount,omitempty"`
	Recommended bool   `json:"recommended,omitempty"`
	Angle       string `json:"angle,omitempty"`
	Seo         int    `json:"seo,omitempty"`
	Voice       int    `json:"voice,omitempty"`
}
```

Note the `body,omitempty` change — cold-draft variants leave `Body` empty.

- [ ] **Step 4: Run the test, confirm it passes.**

```
go test ./internal/personas/marketing/ -run TestVariant_StructuredBody_RoundTrip -v
```

Expected: PASS.

- [ ] **Step 5: Run all marketing tests to confirm no regression.**

```
go test ./internal/personas/marketing/ -v
```

Expected: all PASS. The existing rewrite-path tests still parse the legacy `body` field correctly because the JSON tag is unchanged on that field.

- [ ] **Step 6: Commit.**

```
git add daemon/internal/personas/marketing/marketing.go daemon/internal/personas/marketing/marketing_test.go
git commit -m "$(cat <<'EOF'
feat(marketing): add structured short/long body fields to variant struct

Additive: existing rewrite variants continue to use Body (single string).
Cold-draft variants will populate BodyShort and/or BodyLong instead.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Write `parseColdDraftVariants` parser

**Files:**
- Modify: `daemon/internal/personas/marketing/marketing.go` (add new function alongside `parseVariants`)
- Test: `daemon/internal/personas/marketing/marketing_test.go`

- [ ] **Step 1: Write failing tests for the cold-draft parser.**

Append to `marketing_test.go`:

```go
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
	// Drafting expects [short, long] but variant A only has body_long.
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
```

- [ ] **Step 2: Run the tests, confirm they fail with `parseColdDraftVariants undefined`.**

```
go test ./internal/personas/marketing/ -run TestParseColdDraftVariants -v
```

Expected: compile error.

- [ ] **Step 3: Implement `parseColdDraftVariants`.**

Add to `marketing.go` just below `parseVariants`:

```go
// llmColdDraftVariant mirrors llmVariant but carries structured body fields
// for the cold-draft case. Either or both of BodyShort / BodyLong may be
// populated; the parser validates against the drafting slice the caller
// provides (e.g. ["short"], ["long"], ["short", "long"]).
type llmColdDraftVariant struct {
	Label     string `json:"label"`
	Angle     string `json:"angle"`
	BodyShort string `json:"body_short"`
	BodyLong  string `json:"body_long"`
	Seo       int    `json:"seo"`
	Voice     int    `json:"voice"`
}

type llmColdDraftResp struct {
	Variants []llmColdDraftVariant `json:"variants"`
}

// parseColdDraftVariants parses a cold-draft 3-variant LLM response. The
// drafting slice lists which body fields each variant MUST populate; any
// missing field on any variant is an error so the caller falls back rather
// than persisting a partial proposal. Returns variants ready to persist
// (BodyShort/BodyLong set, Body left empty).
func parseColdDraftVariants(raw string, drafting []string) ([]variant, error) {
	block := jsonObjectRe.FindString(raw)
	if block == "" {
		return nil, fmt.Errorf("no JSON object found in LLM output")
	}
	var parsed llmColdDraftResp
	if err := json.Unmarshal([]byte(block), &parsed); err != nil {
		return nil, fmt.Errorf("decode cold-draft variants JSON: %w", err)
	}
	if len(parsed.Variants) != 3 {
		return nil, fmt.Errorf("expected 3 variants, got %d", len(parsed.Variants))
	}
	needShort, needLong := false, false
	for _, f := range drafting {
		switch f {
		case "short":
			needShort = true
		case "long":
			needLong = true
		default:
			return nil, fmt.Errorf("unknown drafting field %q (expected short|long)", f)
		}
	}
	if !needShort && !needLong {
		return nil, fmt.Errorf("drafting list is empty")
	}
	out := make([]variant, 0, 3)
	for i, v := range parsed.Variants {
		short := strings.TrimSpace(v.BodyShort)
		long := strings.TrimSpace(v.BodyLong)
		if needShort && short == "" {
			return nil, fmt.Errorf("variant %d missing body_short", i)
		}
		if needLong && long == "" {
			return nil, fmt.Errorf("variant %d missing body_long", i)
		}
		label := strings.TrimSpace(v.Label)
		if label == "" {
			label = string(rune('A' + i))
		}
		seo := v.Seo
		if seo <= 0 || seo > 100 {
			seo = 0
		}
		voice := v.Voice
		if voice <= 0 || voice > 100 {
			voice = 0
		}
		charCount := len(short) + len(long)
		out = append(out, variant{
			ID:          fmt.Sprintf("var_%s", strings.ToLower(label)),
			Label:       label,
			BodyShort:   short,
			BodyLong:    long,
			CharCount:   charCount,
			Recommended: i == 0,
			Angle:       strings.TrimSpace(v.Angle),
			Seo:         seo,
			Voice:       voice,
		})
	}
	return out, nil
}
```

- [ ] **Step 4: Run the tests, confirm they pass.**

```
go test ./internal/personas/marketing/ -run TestParseColdDraftVariants -v
```

Expected: 3 tests PASS.

- [ ] **Step 5: Commit.**

```
git add daemon/internal/personas/marketing/marketing.go daemon/internal/personas/marketing/marketing_test.go
git commit -m "$(cat <<'EOF'
feat(marketing): add parseColdDraftVariants for structured short/long output

Mirrors parseVariants but expects body_short and/or body_long per the
drafting slice. Strict on count (3) and on field presence — any missing
required field on any variant fails the parse so the caller can fall back
rather than persisting a partial proposal.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Implement `pickColdDraftCandidates`

**Files:**
- Modify: `daemon/internal/personas/marketing/marketing.go` (add alongside `pickFirstPublished`)
- Test: `daemon/internal/personas/marketing/marketing_test.go`

- [ ] **Step 1: Write a failing test.**

This test uses a fake MCP responder. Look at the existing `TestPickFirstPublished_*` tests in `marketing_test.go` for the fake-MCP pattern in use; mirror that. Append:

```go
func TestPickColdDraftCandidates_ReturnsEmptyFieldsOnly(t *testing.T) {
	// Fake MCP returns 4 published products. P1 has both descriptions; P2 has
	// only long; P3 has only short; P4 has neither. P1 should be skipped (no
	// data issue); P2, P3, P4 should come back.
	fakeMCP := &fakeMCP{products: []productSummary{
		{ID: 1, Name: "P1", Status: "publish", DescriptionLength: 100, ShortDescriptionLength: 50},
		{ID: 2, Name: "P2", Status: "publish", DescriptionLength: 100, ShortDescriptionLength: 0},
		{ID: 3, Name: "P3", Status: "publish", DescriptionLength: 0, ShortDescriptionLength: 50},
		{ID: 4, Name: "P4", Status: "publish", DescriptionLength: 0, ShortDescriptionLength: 0},
	}}
	got, err := pickColdDraftCandidates(context.Background(), fakeMCP.client(), nil, 10)
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	wantIDs := []int{2, 3, 4}
	if !sameIDs(got, wantIDs) {
		t.Errorf("ids = %v, want %v", ids(got), wantIDs)
	}
}

func TestPickColdDraftCandidates_HonorsCooldown(t *testing.T) {
	fakeMCP := &fakeMCP{products: []productSummary{
		{ID: 2, Name: "P2", Status: "publish", ShortDescriptionLength: 0},
		{ID: 3, Name: "P3", Status: "publish", DescriptionLength: 0},
	}}
	skip := map[int]struct{}{2: {}}
	got, _ := pickColdDraftCandidates(context.Background(), fakeMCP.client(), skip, 10)
	if len(got) != 1 || got[0].ID != 3 {
		t.Errorf("expected only product 3 (P2 in cooldown), got %v", ids(got))
	}
}

func TestPickColdDraftCandidates_RespectsMax(t *testing.T) {
	products := []productSummary{}
	for i := 1; i <= 15; i++ {
		products = append(products, productSummary{
			ID: i, Status: "publish", DescriptionLength: 0, ShortDescriptionLength: 0,
		})
	}
	fakeMCP := &fakeMCP{products: products}
	got, _ := pickColdDraftCandidates(context.Background(), fakeMCP.client(), nil, 10)
	if len(got) != 10 {
		t.Errorf("len = %d, want 10 (capped)", len(got))
	}
}

// Helpers (add once near top of test file if not already present).
func ids(ps []productSummary) []int {
	out := make([]int, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.ID)
	}
	return out
}
func sameIDs(ps []productSummary, want []int) bool {
	if len(ps) != len(want) {
		return false
	}
	for i, p := range ps {
		if p.ID != want[i] {
			return false
		}
	}
	return true
}
```

If `fakeMCP` doesn't exist yet, look at existing pick tests in the file for the canonical fixture and reuse it. If no such fixture exists, fall back to integration-style test using the staging store; the function is small enough that an integration smoke is acceptable for v1.

- [ ] **Step 2: Run the tests, confirm they fail with `pickColdDraftCandidates undefined`.**

```
go test ./internal/personas/marketing/ -run TestPickColdDraftCandidates -v
```

Expected: compile error.

- [ ] **Step 3: Implement `pickColdDraftCandidates`.**

Add to `marketing.go` near `pickFirstPublished`:

```go
// pickColdDraftCandidates returns up to max published products where
// short_description OR long_description is empty, excluding products in
// skip. Order matches the underlying list call (date_modified asc) so the
// stalest products surface first — same intuition as pickFirstPublished's
// data_issues bias, but returning a slice instead of one ID.
func pickColdDraftCandidates(ctx context.Context, c *mcp.Client, skip map[int]struct{}, max int) ([]productSummary, error) {
	var listOut struct {
		Products []productSummary `json:"products"`
	}
	if err := callAbility(ctx, c, "wooagent-products/list",
		map[string]any{"per_page": 100, "orderby": "date_modified", "order": "asc"}, &listOut); err != nil {
		return nil, err
	}
	out := make([]productSummary, 0, max)
	for _, p := range listOut.Products {
		if len(out) >= max {
			break
		}
		if p.Status != "publish" && p.Status != "" {
			continue
		}
		if _, inCooldown := skip[p.ID]; inCooldown {
			continue
		}
		if p.DescriptionLength == 0 || p.ShortDescriptionLength == 0 {
			out = append(out, p)
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run the tests, confirm they pass.**

```
go test ./internal/personas/marketing/ -run TestPickColdDraftCandidates -v
```

Expected: 3 PASS.

- [ ] **Step 5: Commit.**

```
git add daemon/internal/personas/marketing/marketing.go daemon/internal/personas/marketing/marketing_test.go
git commit -m "$(cat <<'EOF'
feat(marketing): add pickColdDraftCandidates returning N empty-copy products

Returns up to max published products with at least one empty description
field, excluding cooldown set. Used by the cold-draft batch path; the
single-rewrite path keeps using pickFirstPublished.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Implement `draftColdDraftForProduct` (per-product cold-draft call)

**Files:**
- Modify: `daemon/internal/personas/marketing/marketing.go`
- Test: `daemon/internal/personas/marketing/marketing_test.go`

This function does the single-product half of the cold-draft work: fetch product, determine which fields are empty (`drafting`), call the skill with `mode=cold_draft`, parse with `parseColdDraftVariants`, build a `Drafted` carrying the structured variants and `target.drafting`. The batch wrapper (Task 5) calls this N times.

- [ ] **Step 1: Write a failing test using a fake LLM responder.**

Append (and reuse any existing fake-LLM scaffolding from the file):

```go
func TestDraftColdDraftForProduct_PartialEmpty_DraftsLongOnly(t *testing.T) {
	p := productSummary{
		ID: 42, Name: "Wool Slippers", SKU: "wool-slippers",
		DescriptionLength: 0, ShortDescriptionLength: 50,
	}
	// Fake the LLM returning a valid 3-variant cold-draft response with body_long only.
	fakeLLM := &fakeLLM{response: `{"variants":[
		{"label":"A","body_long":"Long A","seo":80,"voice":85},
		{"label":"B","body_long":"Long B","seo":75,"voice":80},
		{"label":"C","body_long":"Long C","seo":70,"voice":78}
	]}`}
	deps := personas.Deps{ /* wire fakes per existing test pattern */ }
	got, err := draftColdDraftForProduct(context.Background(), deps, p, "skill desc")
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	drafting, _ := got.Target["drafting"].([]string)
	if !reflect.DeepEqual(drafting, []string{"long"}) {
		t.Errorf("drafting = %v, want [long]", drafting)
	}
	if got.ProposalType != "product_cold_draft" {
		t.Errorf("proposal_type = %q, want product_cold_draft", got.ProposalType)
	}
	vars, _ := got.Target["variants"].([]variant)
	if vars[0].BodyShort != "" || vars[0].BodyLong == "" {
		t.Errorf("variant A should have only long; got short=%q long=%q",
			vars[0].BodyShort, vars[0].BodyLong)
	}
	if got.DedupKey != "product:42" {
		t.Errorf("dedup_key = %q, want product:42", got.DedupKey)
	}
}

func TestDraftColdDraftForProduct_BothEmpty_DraftsBoth(t *testing.T) {
	p := productSummary{
		ID: 99, DescriptionLength: 0, ShortDescriptionLength: 0,
	}
	fakeLLM := &fakeLLM{response: `{"variants":[
		{"label":"A","body_short":"Short A","body_long":"Long A","seo":80,"voice":85},
		{"label":"B","body_short":"Short B","body_long":"Long B","seo":75,"voice":80},
		{"label":"C","body_short":"Short C","body_long":"Long C","seo":70,"voice":78}
	]}`}
	deps := personas.Deps{ /* wire fakes */ }
	got, _ := draftColdDraftForProduct(context.Background(), deps, p, "skill desc")
	drafting, _ := got.Target["drafting"].([]string)
	wantDrafting := []string{"short", "long"}
	if !reflect.DeepEqual(drafting, wantDrafting) {
		t.Errorf("drafting = %v, want %v", drafting, wantDrafting)
	}
}
```

(Use the existing fake-LLM scaffolding in `marketing_test.go`; if the parameter names differ, match that file's conventions.)

- [ ] **Step 2: Run the tests, confirm they fail.**

```
go test ./internal/personas/marketing/ -run TestDraftColdDraftForProduct -v
```

Expected: compile error.

- [ ] **Step 3: Implement `draftColdDraftForProduct`.**

```go
// draftColdDraftForProduct does the per-product cold-draft work: fetch the
// product, determine which fields are empty, call the skill with mode
// cold_draft, parse the structured response, return a Drafted. Like
// draftForProduct but the LLM is invoked in cold-draft mode and the variant
// shape is structured (short / long) instead of a single body.
func draftColdDraftForProduct(ctx context.Context, deps personas.Deps, p productSummary, skillDescription string) (personas.Drafted, error) {
	full, err := getProduct(ctx, deps.MCP, p.ID)
	if err != nil {
		return personas.Drafted{}, fmt.Errorf("get product %d: %w", p.ID, err)
	}

	drafting := []string{}
	if strings.TrimSpace(full.ShortDescription) == "" {
		drafting = append(drafting, "short")
	}
	if strings.TrimSpace(full.Description) == "" {
		drafting = append(drafting, "long")
	}
	if len(drafting) == 0 {
		// Race: by the time we fetched the full product, it had non-empty
		// fields. Skip — single-rewrite path is the right home for it.
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("product %d no longer has empty fields", p.ID),
		}, nil
	}

	corpus, corpusErr := fetchVoiceCorpus(ctx, deps.MCP, p.ID)
	if corpusErr != nil {
		fmt.Printf("marketing(cold_draft): voice corpus fetch errored (%v); proceeding with empty corpus\n", corpusErr)
		corpus = nil
	}

	rawOutput, skipReason, err := draftWithFallback(ctx, deps.Env, full, skillDescription, corpus, draftOpts{Mode: "cold_draft", Drafting: drafting})
	if err != nil {
		return personas.Drafted{}, err
	}
	if skipReason != "" {
		return personas.Drafted{Skipped: true, SkipReason: skipReason}, nil
	}

	variants, parseErr := parseColdDraftVariants(rawOutput, drafting)
	if parseErr != nil {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("parse cold-draft variants: %v", parseErr),
		}, nil
	}

	target := map[string]any{
		"product_id":   full.ID,
		"product_name": full.Name,
		"product_sku":  full.SKU,
		"previous_short": full.ShortDescription,
		"previous_long":  full.Description,
		"image_url":    full.ImageURL,
		"image_alt":    full.ImageAlt,
		"variants":     variants,
		"drafting":     drafting,
	}

	return personas.Drafted{
		Title:           fmt.Sprintf("Product description rewrite · %s", full.Name),
		Description:     fmt.Sprintf("Cold-drafted by Marketing agent for product #%d (%s).", full.ID, full.SKU),
		Priority:        "medium",
		ProposalType:    "product_cold_draft",
		ProposalContent: variants[0].BodyLong, // surfaces in log / archive views; long body is the verbose one
		Target:          target,
		DedupKey:        fmt.Sprintf("product:%d", full.ID),
	}, nil
}
```

**Note on `draftWithFallback`:** the existing signature takes a fixed set of args. You'll need to extend it to accept a `draftOpts` struct with `Mode` and `Drafting` fields so the cold-draft path can be threaded through. The rewrite path passes the zero value (default mode `"rewrite"`). Match the file's existing conventions for option-struct shape.

- [ ] **Step 4: Update `draftWithFallback` to accept `draftOpts`.**

Find the existing `draftWithFallback` signature and extend it; pass-through to the LLM prompt builder so the skill template receives `mode` and `drafting` parameters. The rewrite-path callsite (`draftForProduct` line ~247) passes `draftOpts{}` (zero value).

- [ ] **Step 5: Run the tests, confirm they pass.**

```
go test ./internal/personas/marketing/ -run TestDraftColdDraftForProduct -v
go test ./internal/personas/marketing/ -v  # confirm rewrite path still passes
```

Expected: all PASS.

- [ ] **Step 6: Commit.**

```
git add daemon/internal/personas/marketing/marketing.go daemon/internal/personas/marketing/marketing_test.go
git commit -m "$(cat <<'EOF'
feat(marketing): add draftColdDraftForProduct (single-product cold draft)

Determines empty fields per product, calls the skill in cold_draft mode,
parses the structured 3-variant response, returns a Drafted with
ProposalType "product_cold_draft" and target.drafting indicating which
fields the variants will write to. Used by the batch wrapper.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Implement `draftColdDraftBatch` (the BatchSiblings packer)

**Files:**
- Modify: `daemon/internal/personas/marketing/marketing.go`
- Test: `daemon/internal/personas/marketing/marketing_test.go`

- [ ] **Step 1: Write a failing test.**

```go
func TestDraftColdDraftBatch_PacksAsSiblings(t *testing.T) {
	// 4 candidate products, all with empty long_description.
	products := []productSummary{
		{ID: 11, Name: "P11", DescriptionLength: 0, ShortDescriptionLength: 50},
		{ID: 12, Name: "P12", DescriptionLength: 0, ShortDescriptionLength: 50},
		{ID: 13, Name: "P13", DescriptionLength: 0, ShortDescriptionLength: 50},
		{ID: 14, Name: "P14", DescriptionLength: 0, ShortDescriptionLength: 50},
	}
	deps := personas.Deps{ /* wire fakes returning a valid 3-variant response per call */ }
	got, err := draftColdDraftBatch(context.Background(), deps, products, "skill desc")
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
		t.Errorf("siblings = %d, want 3 (4 drafts: 1 primary + 3 siblings)", len(got.BatchSiblings))
	}
}

func TestDraftColdDraftBatch_DropsParseFailures(t *testing.T) {
	// 3 candidates; the LLM returns garbage for the second.
	// Expected: batch has 2 children (the 2 that parsed).
	// If <2 children survive, the function returns Skipped:true rather
	// than emitting a degenerate batch.
	// (assertions per the pattern above)
}
```

- [ ] **Step 2: Run tests, confirm failure.**

```
go test ./internal/personas/marketing/ -run TestDraftColdDraftBatch -v
```

- [ ] **Step 3: Implement `draftColdDraftBatch`.**

```go
// draftColdDraftBatch drafts cold-copy proposals for N candidates and packs
// them into a single batch. The first successful draft carries BatchTitle /
// BatchIntent / BatchSiblings; subsequent successes become siblings.
// Per-product failures are logged and dropped; if fewer than coldDraftMin
// drafts survive parsing, the whole batch is skipped (caller falls through
// to single-rewrite).
func draftColdDraftBatch(ctx context.Context, deps personas.Deps, candidates []productSummary, skillDescription string) (personas.Drafted, error) {
	drafts := make([]personas.Drafted, 0, len(candidates))
	for _, p := range candidates {
		d, err := draftColdDraftForProduct(ctx, deps, p, skillDescription)
		if err != nil {
			fmt.Printf("marketing(cold_draft): product %d errored (%v); dropping\n", p.ID, err)
			continue
		}
		if d.Skipped {
			fmt.Printf("marketing(cold_draft): product %d skipped (%s); dropping\n", p.ID, d.SkipReason)
			continue
		}
		drafts = append(drafts, d)
	}
	if len(drafts) < coldDraftMin {
		return personas.Drafted{
			Skipped: true,
			SkipReason: fmt.Sprintf(
				"only %d cold-draft candidates survived parsing (need %d)", len(drafts), coldDraftMin),
		}, nil
	}
	primary := drafts[0]
	primary.BatchSiblings = drafts[1:]
	primary.BatchTitle = fmt.Sprintf("Review & approve · %d product descriptions", len(drafts))
	primary.BatchIntent = "fill_missing_copy"
	return primary, nil
}
```

Add the constants `coldDraftMin = 3` and `coldDraftMax = 10` near `maxDraftAttempts`:

```go
const (
	coldDraftMin = 3  // require >= this many empty-copy candidates to emit a batch
	coldDraftMax = 10 // cap per-tick LLM cost and operator review surface
)
```

- [ ] **Step 4: Run tests, confirm pass.**

```
go test ./internal/personas/marketing/ -run TestDraftColdDraftBatch -v
```

- [ ] **Step 5: Commit.**

```
git add daemon/internal/personas/marketing/marketing.go daemon/internal/personas/marketing/marketing_test.go
git commit -m "$(cat <<'EOF'
feat(marketing): add draftColdDraftBatch packing N drafts as BatchSiblings

Per-product errors and parse failures are logged and dropped; if fewer
than coldDraftMin survive, the function returns Skipped:true so Draft()
can fall through to the single-rewrite path. Sets BatchTitle, BatchIntent,
BatchSiblings on the primary Drafted.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Branch `Draft()` to call the cold-draft path when threshold met

**Files:**
- Modify: `daemon/internal/personas/marketing/marketing.go` (the `Draft` method, currently lines ~179-228)
- Test: `daemon/internal/personas/marketing/marketing_test.go`

- [ ] **Step 1: Write a failing test asserting batch dispatch at threshold.**

```go
func TestDraft_AboveColdDraftThreshold_EmitsBatch(t *testing.T) {
	// Catalog: 5 products with empty descriptions, none in cooldown.
	deps := personas.Deps{ /* wire fakes: products list returns 5 cold candidates */ }
	got, err := (Marketing{}).Draft(context.Background(), deps)
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	if len(got.BatchSiblings) == 0 {
		t.Errorf("expected batch dispatch (siblings > 0), got single proposal")
	}
	if got.ProposalType != "product_cold_draft" {
		t.Errorf("proposal_type = %q, want product_cold_draft", got.ProposalType)
	}
}

func TestDraft_BelowColdDraftThreshold_FallsThrough(t *testing.T) {
	// Catalog: 2 products with empty descriptions, 10 healthy products.
	// Expected: single-rewrite emit (no batch siblings).
	deps := personas.Deps{ /* wire fakes */ }
	got, _ := (Marketing{}).Draft(context.Background(), deps)
	if len(got.BatchSiblings) != 0 {
		t.Errorf("expected fall-through to single-rewrite, got batch with %d siblings", len(got.BatchSiblings))
	}
	if got.ProposalType != "product_description_rewrite" {
		t.Errorf("proposal_type = %q, want product_description_rewrite", got.ProposalType)
	}
}
```

- [ ] **Step 2: Run tests, confirm failure.**

```
go test ./internal/personas/marketing/ -run TestDraft_ -v
```

- [ ] **Step 3: Add the cold-draft branch in `Draft()`.**

Find the existing `Draft()` body (around line 180-228). After the skill lookup and cooldown skip-map build (`personas.RecentlyTouchedTargets`), but *before* the existing single-rewrite loop, insert:

```go
	// Cold-draft batch path: if enough products have empty short/long
	// descriptions, draft them in one batch this tick instead of the
	// single-rewrite loop. See docs/specs/2026-05-18-marketing-cold-draft-batch-design.md.
	if deps.Env.ProductIDOverride == 0 { // operator-targeted runs skip cold-draft
		candidates, err := pickColdDraftCandidates(ctx, deps.MCP, skip, coldDraftMax)
		if err != nil {
			fmt.Printf("marketing: cold-draft candidate scan errored (%v); falling back to single-rewrite\n", err)
		} else if len(candidates) >= coldDraftMin {
			batch, err := draftColdDraftBatch(ctx, deps, candidates, skill.Description)
			if err != nil {
				return personas.Drafted{}, err
			}
			if !batch.Skipped {
				return batch, nil
			}
			fmt.Printf("marketing: cold-draft batch skipped (%s); falling back to single-rewrite\n", batch.SkipReason)
		}
	}
```

- [ ] **Step 4: Run tests, confirm pass.**

```
go test ./internal/personas/marketing/ -run TestDraft_ -v
go test ./internal/personas/marketing/ -v  # full suite still green
```

- [ ] **Step 5: Commit.**

```
git add daemon/internal/personas/marketing/marketing.go daemon/internal/personas/marketing/marketing_test.go
git commit -m "$(cat <<'EOF'
feat(marketing): branch Draft() to cold-draft batch at >=3 candidates

Smart Draft: when enough products with empty short/long descriptions
exist out of cooldown, emit a batch this tick; otherwise fall through
to single-rewrite. Operator-targeted runs (ProductIDOverride) bypass
the cold-draft path entirely.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Update the skill YAML to accept a `mode` parameter

**Files:**
- Modify: `prompts/skills/marketing-description-rewrite/v1.yaml`

- [ ] **Step 1: Read the current skill YAML to understand its parameter shape.**

```
cat /Users/elizabethpizzuti/claude/wooagent/prompts/skills/marketing-description-rewrite/v1.yaml
```

Note the existing parameters and the prompt structure. The skill probably has a single `description` template that the daemon's `draftWithFallback` interpolates the product into.

- [ ] **Step 2: Add a `mode` parameter (default `rewrite`) and branch the prompt.**

The skill description should include conditional sections:

- **When `mode == rewrite`** (default): existing prompt unchanged. Output schema: `{variants: [{label, angle, body, seo, voice}, ...]}` (single `body` field).
- **When `mode == cold_draft`**: new prompt that drops the "rewrite the existing copy" framing, instructs the LLM to draft from scratch for the fields listed in `drafting`, and specifies the output schema with `body_short` and/or `body_long` (depending on which fields are in `drafting`).

Replace or extend the YAML accordingly. Example structure for the cold-draft block (Go-template-style; match whatever templating the skill loader uses):

```yaml
description: |
  ...existing rewrite prompt...

  {{if eq .mode "cold_draft"}}
  ## Cold-draft mode

  This product has empty {{.drafting | join ", "}} description field(s).
  Draft fresh copy from scratch. Do NOT reference existing copy — there
  is none for the field(s) listed in `drafting`.

  For each variant emit ONLY the fields listed in `drafting`:
  - "short" → emit `body_short` (60-200 chars, scannable)
  - "long"  → emit `body_long`  (200-800 chars, persuasive PDP body)

  Output schema (3 variants):
  {"variants": [
    {"label": "A", "angle": "...", {{range .drafting}}"body_{{.}}": "...",{{end}} "seo": ..., "voice": ...},
    ...
  ]}
  {{else}}
  ## Rewrite mode (default)
  ...existing rewrite instructions...
  {{end}}
```

Adjust to the file's actual templating syntax.

- [ ] **Step 3: Manually invoke the skill on staging with `mode=cold_draft` to verify the LLM emits the structured shape.**

The simplest verification is a unit-style end-to-end run via the Marketing persona's existing test harness against staging. If a manifest refresh is needed (per CLAUDE.md "Manifest refresh"), document that in the PR but don't run it as part of this task — refreshes are operator-gated.

- [ ] **Step 4: Commit.**

```
git add prompts/skills/marketing-description-rewrite/v1.yaml
git commit -m "$(cat <<'EOF'
feat(skill): add cold_draft mode to marketing description rewrite skill

When mode=cold_draft, the skill drops the rewrite framing and instructs
the LLM to draft from scratch for the fields listed in drafting.
Output schema branches: body_short / body_long fields replace the single
body field. Default mode (rewrite) is unchanged for back-compat.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Add `product_cold_draft` approve dispatch entry

**Files:**
- Modify: `daemon/internal/httpapi/handlers_v1.go` (the `approveDispatchByType` map at line 413, the `approveOne` flow that selects variants)
- Test: `daemon/internal/httpapi/handlers_batches_test.go`

The existing `product_description_rewrite` entry writes only `description`. The new entry writes whichever of `short_description` / `long_description` were in `target.drafting`, reading body fields from the selected variant.

- [ ] **Step 1: Locate `approveOne` and find where it picks the variant body.**

```
grep -n "approveOne\|variant_id\|VariantID\|variant.Body" daemon/internal/httpapi/handlers_v1.go | head -20
```

The flow today: `approveOne` reads `VariantID` from the request, locates the variant in `target.variants[]`, passes that variant's `Body` string into `dispatch.buildParams(body, target)`. For cold-draft we need to pass both `body_short` and `body_long`. Either:

  (a) Extend `buildParams` signature to take the full variant struct (touches all dispatchers, additive but wider blast radius); or
  (b) Pack short+long into a JSON string in the existing `body` arg before calling buildParams for cold-draft (narrower change, slightly hacky).

Pick (a): cleaner, costs ~10 lines of signature change.

- [ ] **Step 2: Write a failing test for the cold-draft approve path.**

In `handlers_batches_test.go`, add a fixture variant where the proposal is `product_cold_draft` with `target.drafting=["long"]` and the selected variant has `body_long="New long copy."`. Assert the `wooagent-products/update` payload is exactly `{"id": <pid>, "description": "New long copy."}` — no `short_description` field.

```go
func TestApproveOne_ColdDraft_WritesOnlyDraftedFields(t *testing.T) {
	// Seed a cold-draft proposal with drafting=[long].
	// Approve with variant_id=var_a.
	// Assert: wooagent-products/update called with exactly id + description,
	// NO short_description in the payload.
	// (match the existing handlers_batches_test.go fixture style)
}

func TestApproveOne_ColdDraft_BothFields_WritesBoth(t *testing.T) {
	// drafting=[short, long]; variant has both body_short and body_long.
	// Assert payload has id + short_description + description.
}
```

- [ ] **Step 3: Run tests, confirm failure.**

```
cd /Users/elizabethpizzuti/claude/wooagent/daemon
go test ./internal/httpapi/ -run TestApproveOne_ColdDraft -v
```

- [ ] **Step 4: Extend `approveDispatch` signature and add the cold-draft entry.**

Change `buildParams` from `func(content string, target map[string]any)` to `func(variantBody map[string]string, target map[string]any)` — `variantBody` is a map with the keys the dispatch needs (`"long"`, `"short"`, `""` for legacy single-body). Update all existing dispatchers to read `variantBody[""]` for the legacy single-body case.

Add the new entry:

```go
"product_cold_draft": {
	ability: "wooagent-products/update",
	buildParams: func(body map[string]string, target map[string]any) (map[string]any, error) {
		pid, err := requireIntFromTarget(target, "product_id")
		if err != nil {
			return nil, err
		}
		drafting, err := stringSliceFromTarget(target, "drafting")
		if err != nil {
			return nil, err
		}
		params := map[string]any{"id": pid}
		for _, field := range drafting {
			switch field {
			case "short":
				params["short_description"] = body["short"]
			case "long":
				params["description"] = body["long"]
			default:
				return nil, fmt.Errorf("unknown drafting field %q", field)
			}
		}
		return params, nil
	},
},
```

(Helper `stringSliceFromTarget` exists already, or follow the pattern of `requireIntFromTarget`.)

Update `approveOne` where it constructs the `variantBody` map from the selected variant. For legacy variants, set `variantBody[""] = variant.Body`. For cold-draft variants, set `variantBody["short"] = variant.BodyShort; variantBody["long"] = variant.BodyLong`.

- [ ] **Step 5: Run tests, confirm pass.**

```
go test ./internal/httpapi/ -run TestApproveOne -v
go test ./internal/httpapi/ -v  # full handler suite
```

- [ ] **Step 6: Commit.**

```
git add daemon/internal/httpapi/handlers_v1.go daemon/internal/httpapi/handlers_batches_test.go
git commit -m "$(cat <<'EOF'
feat(httpapi): add product_cold_draft approve dispatch

Writes only the fields listed in target.drafting via
wooagent-products/update — short_description and/or description.
Existing fields on the product are preserved (the payload omits them).
buildParams signature widened to accept a map of body fields so the
cold-draft path can pass {short, long}; legacy dispatchers read the
empty-string key for back-compat.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Undo / reversibility — snapshot empty state and restore

**Files:**
- Modify: the apply handler's snapshot path (same area as Task 8; likely a sibling `snapshot...` function near `approveDispatchByType`)
- Test: `daemon/internal/httpapi/handlers_batches_test.go` or sibling

- [ ] **Step 1: Locate the snapshot path.**

```
grep -n "snapshot\|Snapshot\|previous\|Undo\|undo\|undo_state" daemon/internal/httpapi/*.go | head -20
```

The existing approve handler records a snapshot of the product fields *before* writing so Undo can restore them. For `product_description_rewrite` it snapshots the long description. We need to snapshot whichever fields are in `target.drafting`, including empty values.

- [ ] **Step 2: Write a failing test.**

```go
func TestApproveOne_ColdDraft_SnapshotsEmptyForUndo(t *testing.T) {
	// Seed a cold-draft proposal where the product currently has
	// short_description="" and description="". Approve.
	// Read the snapshot row that was written; assert it contains
	// short_description="" and description="" (literal empty strings, not absent).
	// Trigger Undo; assert wooagent-products/update is called with the same
	// id + empty short_description + empty description.
}
```

- [ ] **Step 3: Implement the snapshot to handle the cold-draft proposal type.**

Add a parallel snapshot dispatcher (or extend the existing one) for `product_cold_draft` that reads `target.previous_short` and `target.previous_long` (already populated at draft time in Task 4's `draftColdDraftForProduct`) and persists them to the snapshot row. Undo replays those values.

- [ ] **Step 4: Run tests, confirm pass.**

```
go test ./internal/httpapi/ -run TestApproveOne_ColdDraft_Snapshot -v
```

- [ ] **Step 5: Commit.**

```
git add daemon/internal/httpapi/...
git commit -m "$(cat <<'EOF'
feat(httpapi): snapshot empty values for cold-draft undo

product_cold_draft snapshots target.previous_short and previous_long,
which were captured by Marketing at draft time. Undo restores literal
empty strings if that's what the field was before. "Reversible · always"
means literally previous state.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Extend TypeScript `Variant` interface with structured body fields

**Files:**
- Modify: `ui/src/api/client.ts` (around lines 282-308 per spec)

- [ ] **Step 1: Read the current `Variant` definition.**

```
sed -n '280,310p' /Users/elizabethpizzuti/claude/wooagent/ui/src/api/client.ts
```

- [ ] **Step 2: Add optional fields.**

```typescript
export interface Variant {
  id: string;
  label: string;
  body: string;          // legacy single-body (rewrite + fallback)
  body_short?: string;   // cold-draft: short_description
  body_long?: string;    // cold-draft: long_description
  seo?: number;
  voice?: number;
  charCount: number;
  recommended?: boolean;
  angle?: string;
  note?: string;
}
```

- [ ] **Step 3: Update `variantsFromProposal` to pass the new fields through.**

```typescript
// inside variantsFromProposal — wherever it constructs a Variant from
// the raw target.variants[] entry:
return {
  id: v.id,
  label: v.label,
  body: v.body ?? '',
  body_short: typeof v.body_short === 'string' ? v.body_short : undefined,
  body_long:  typeof v.body_long  === 'string' ? v.body_long  : undefined,
  charCount: typeof v.charCount === 'number' ? v.charCount : (v.body ?? '').length,
  recommended: v.recommended === true,
  // ... existing fields
};
```

- [ ] **Step 4: Run typecheck.**

```
cd /Users/elizabethpizzuti/claude/wooagent/ui
npm run typecheck
```

Expected: PASS.

- [ ] **Step 5: Commit.**

```
git add ui/src/api/client.ts
git commit -m "$(cat <<'EOF'
feat(ui): extend Variant interface with optional body_short/body_long

Cold-draft variants from Marketing carry structured body fields instead
of (or alongside) the legacy single body string. The rewrite path
continues to read body; the cold-draft path reads body_short/body_long.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: BatchReview — render structured Current column for cold-draft

**Files:**
- Modify: `ui/src/screens/BatchReview.tsx` (around lines 277-423 — the `renderMarketingBody` Current column)

- [ ] **Step 1: Detect cold-draft batches.**

At BatchReview.tsx:277 there's already `const isPricingBatch = firstProposal?.type === 'product_price_change';`. Add a sibling:

```typescript
const isColdDraftBatch = firstProposal?.type === 'product_cold_draft';
```

- [ ] **Step 2: Read the existing Current column structure.**

Lines ~398-423 render a single Description cell. Change this to render two cells when `isColdDraftBatch` is true: one for short, one for long. The non-cold-draft (rewrite) path keeps the existing single-cell layout.

- [ ] **Step 3: Update the rendering — Current column.**

For each row, replace the single Description cell with conditional rendering:

```tsx
{isColdDraftBatch ? (
  <>
    <span className="wa-eyebrow">Short description</span>
    <Text
      variant="body-sm"
      style={{
        color: previousShort
          ? 'var(--wpds-color-fg-content-neutral)'
          : 'var(--wpds-color-fg-content-neutral-weak)',
        whiteSpace: 'pre-wrap',
        lineHeight: 1.5,
      }}
    >
      {previousShort || '— empty —'}
    </Text>
    <span className="wa-eyebrow" style={{ marginTop: 'var(--wpds-dimension-gap-md)' }}>
      Long description
    </span>
    <Text
      variant="body-sm"
      style={{
        color: previousLong
          ? 'var(--wpds-color-fg-content-neutral)'
          : 'var(--wpds-color-fg-content-neutral-weak)',
        whiteSpace: 'pre-wrap',
        lineHeight: 1.5,
      }}
    >
      {previousLong || '— empty —'}
    </Text>
  </>
) : (
  <>
    {/* existing single-Description cell, unchanged */}
  </>
)}
```

Read `previousShort` / `previousLong` from `target.previous_short` / `target.previous_long` (the daemon sets these at draft time). For the rewrite path keep `previousCopy` reading from `target.previous`.

- [ ] **Step 4: Typecheck.**

```
cd /Users/elizabethpizzuti/claude/wooagent/ui
npm run typecheck
```

- [ ] **Step 5: Visual verification — run the dev server.**

```
cd /Users/elizabethpizzuti/claude/wooagent/ui
npm run dev
```

Manually trigger a cold-draft batch via the daemon (or seed a fixture proposal). Verify the BatchReview detail view renders the two-cell Current column.

- [ ] **Step 6: Commit.**

```
git add ui/src/screens/BatchReview.tsx
git commit -m "$(cat <<'EOF'
feat(ui): BatchReview renders two-cell Current column for cold-draft

Cold-draft batches show Short / Long description cells stacked in the
Current column, each rendering existing copy or '— empty —'. Rewrite
batches keep the existing single-Description cell.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: BatchReview — render structured variant columns for cold-draft

**Files:**
- Modify: `ui/src/screens/BatchReview.tsx` (the variant-columns map around lines 425-580)

- [ ] **Step 1: For cold-draft batches, render each variant column with Short / Long cells.**

The variant column maps inside the existing `(variants ?? []).map(...)`. For `isColdDraftBatch`, replace the single body Text with two stacked Texts (Short description / Long description). A variant whose `body_short` is empty (because drafting=[long] only) shows "no change" greyed in that cell:

```tsx
{isColdDraftBatch ? (
  <>
    <span className="wa-eyebrow">Short description</span>
    <Text
      variant="body-sm"
      style={{
        color: v.body_short
          ? 'var(--wpds-color-fg-content-neutral)'
          : 'var(--wpds-color-fg-content-neutral-weak)',
        whiteSpace: 'pre-wrap',
        lineHeight: 1.5,
      }}
    >
      {v.body_short || 'no change'}
    </Text>
    <span className="wa-eyebrow" style={{ marginTop: 'var(--wpds-dimension-gap-md)' }}>
      Long description
    </span>
    <Text
      variant="body-sm"
      style={{
        color: v.body_long
          ? 'var(--wpds-color-fg-content-neutral)'
          : 'var(--wpds-color-fg-content-neutral-weak)',
        whiteSpace: 'pre-wrap',
        lineHeight: 1.5,
      }}
    >
      {v.body_long || 'no change'}
    </Text>
  </>
) : (
  <>
    {/* existing single body Text, unchanged */}
  </>
)}
```

- [ ] **Step 2: Typecheck + visual smoke.**

```
cd /Users/elizabethpizzuti/claude/wooagent/ui
npm run typecheck && npm run dev
```

Trigger a cold-draft batch with mixed cases (one product short-only, one long-only, one both-empty) and confirm:
- Short-only product's variant columns show drafted short copy + "no change" greyed for long.
- Long-only product's variant columns show "no change" greyed for short + drafted long copy.
- Both-empty product's variant columns show drafted short + drafted long.

- [ ] **Step 3: Commit.**

```
git add ui/src/screens/BatchReview.tsx
git commit -m "$(cat <<'EOF'
feat(ui): BatchReview renders Short/Long cells in each variant column

Cold-draft variant columns stack Short and Long description cells.
Cells for fields not being drafted by that variant show 'no change'
greyed using fg-content-neutral-weak.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: BatchReview — KPI hint copy + footer text for cold-draft

**Files:**
- Modify: `ui/src/screens/BatchReview.tsx` (KPI strip lines ~285-303; footer sticky bar)

- [ ] **Step 1: Swap the brand-voice hint when `isColdDraftBatch`.**

The KPI strip already exists. For cold-draft, change only the brand-voice hint:

```tsx
<Kpi
  label="Brand voice match"
  value="96%"
  score={96}
  hint={isColdDraftBatch ? 'vs. your voice model' : 'vs. your existing copy'}
/>
```

Leave SEO and Est. impact hints unchanged.

- [ ] **Step 2: Update the footer sticky-bar text for cold-draft.**

Find the footer area (around BatchReview.tsx:699-769). The current message reads `Variant {ID} selected — will write yoast-seo/meta.update on approve` (or similar). For cold-draft, compose it from the selected variant's `target.drafting`:

```tsx
const draftingForSelected = (...): string[] => {
  // pull target.drafting from the proposal of the currently-selected row,
  // or from the first row if no selection yet
};

const applyText = (() => {
  if (!isColdDraftBatch) return /* existing rewrite text */;
  const fields = draftingForSelected();
  const parts: string[] = [];
  if (fields.includes('short')) parts.push('product.short_description');
  if (fields.includes('long'))  parts.push('product.description');
  return `will write ${parts.join(' + ')} on approve`;
})();
```

Display per the existing footer pattern.

- [ ] **Step 3: Typecheck + visual smoke.**

```
cd /Users/elizabethpizzuti/claude/wooagent/ui
npm run typecheck && npm run dev
```

Confirm the brand-voice hint and footer text update correctly for cold-draft batches.

- [ ] **Step 4: Commit.**

```
git add ui/src/screens/BatchReview.tsx
git commit -m "$(cat <<'EOF'
feat(ui): BatchReview adapts KPI hint + footer text for cold-draft

Brand voice hint becomes 'vs. your voice model' (vs. nothing comparable
in existing copy). Footer reads 'will write product.short_description +
product.description on approve', narrowing to the actual drafted fields
when only one is being filled.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 14: Retire Reporting `product_health_digest` Draft path

**Files:**
- Modify: `daemon/internal/personas/reporting/reporting.go`
- Modify: `daemon/internal/personas/reporting/reporting_test.go`

- [ ] **Step 1: Add a failing test for the new stub behavior.**

```go
func TestDraft_ReturnsSkipped(t *testing.T) {
	got, err := (Reporting{}).Draft(context.Background(), personas.Deps{})
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	if !got.Skipped {
		t.Errorf("expected Skipped=true, got %+v", got)
	}
	if got.SkipReason == "" {
		t.Errorf("SkipReason should explain dormant state")
	}
}
```

- [ ] **Step 2: Run the test to make sure it does compile but fails the assertion (today's Draft probably emits a digest).**

```
go test ./internal/personas/reporting/ -run TestDraft_ReturnsSkipped -v
```

- [ ] **Step 3: Strip the Draft body, delete the helpers.**

Replace the body of `Draft()` (currently lines 73-128) with:

```go
func (Reporting) Draft(ctx context.Context, deps personas.Deps) (personas.Drafted, error) {
	return personas.Drafted{
		Skipped:    true,
		SkipReason: "no skills registered yet — product_health_digest retired 2026-05-18",
	}, nil
}
```

Delete `findProductsWithDataIssues`, `buildDataIssuesDigest`, `renderDigest`, and any test helpers that referenced them.

- [ ] **Step 4: Update the existing reporting tests.**

Delete tests for the removed helpers. The new `TestDraft_ReturnsSkipped` is the only Draft-level assertion.

- [ ] **Step 5: Run the full reporting test suite and the daemon test suite.**

```
go test ./internal/personas/reporting/ -v
go test ./...
```

Expected: all PASS.

- [ ] **Step 6: Verify Reporting still registers (sidebar + scheduler).**

```
grep -n "reporting" daemon/internal/cli/run.go
```

Expected: the import line `_ "github.com/wooagent-os/wooagent-os/daemon/internal/personas/reporting"` is still present. Persona registration in `personas.Register` happens via `init()` in `reporting.go`; keep that intact.

- [ ] **Step 7: Commit.**

```
git add daemon/internal/personas/reporting/reporting.go daemon/internal/personas/reporting/reporting_test.go
git commit -m "$(cat <<'EOF'
feat(reporting): retire product_health_digest; Draft is dormant stub

Marketing's new cold-draft batches own the copy-gaps workflow
(docs/specs/2026-05-18-marketing-cold-draft-batch-design.md). Reporting
stays registered in the seven-persona sidebar; Draft returns Skipped:true
until a real reporting skill lands (sales summaries / KPI digests).

Existing in-flight product_health_digest proposals on operator stores
are left alone — they dismiss naturally.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 15: End-to-end smoke against the staging store

**Files:** none modified; verification only.

- [ ] **Step 1: Confirm `WOOAGENT_MCP_*` env vars point at the test store.**

```
cat .env.local | grep WOOAGENT_MCP  # or wherever the staging creds live
```

Expected: vars point at `woo-demo-store-99cc5c.mystagingwebsite.com` per `project_test_store.md`.

- [ ] **Step 2: Seed the staging store with ≥3 products lacking descriptions.**

Use the WP-admin on the staging store, or a small seed script, to ensure the store has at least 3 products with empty short or long descriptions. (If this is already the case from prior testing, skip.)

- [ ] **Step 3: Run the Marketing persona once and observe the batch.**

```
cd /Users/elizabethpizzuti/claude/wooagent/daemon
go run ./cmd/persona-marketing
```

Expected log lines: cold-draft candidate scan, N candidates found, N skill calls, batch packed with N siblings. No errors.

- [ ] **Step 4: Open the UI and verify the batch renders.**

```
cd /Users/elizabethpizzuti/claude/wooagent/ui
npm run dev
```

Open `http://localhost:5173`, log in, go to Needs Review. Click into the new "Review & approve · N product descriptions" batch. Verify:
- Two-cell Current column renders with `— empty —` placeholders.
- Three variant columns render with drafted Short / Long copy (or "no change" greyed).
- KPI strip header reads as expected with the swapped brand-voice hint.
- Footer message names the right fields.

- [ ] **Step 5: Approve one row and verify the write on the staging store.**

Click a variant in one row, then "Approve selected" or the per-row approve. In WP-admin on the staging store, confirm the product's short_description / long_description now contain the variant copy.

- [ ] **Step 6: Undo the approval and verify rollback.**

Click Undo in the WooAgent UI on that row. In WP-admin, confirm the fields are back to empty.

- [ ] **Step 7: Document any deviations.**

If anything diverged from the spec, note it in `docs/specs/2026-05-18-marketing-cold-draft-batch-design.md` under a new "Implementation notes" section before merging. Then commit.

---

## Task 16: Open PR

**Files:** none modified; PR creation.

- [ ] **Step 1: Push the branch.**

```
git push -u origin feat/dsgwoo-marketing-cold-draft
```

- [ ] **Step 2: Open the PR.**

```
gh pr create --title "feat: Marketing cold-draft batches for empty product copy" --body "$(cat <<'EOF'
## Summary
- Marketing now scans for products with empty short/long descriptions each tick. When ≥3 candidates are out of cooldown, it drafts a batch of descriptions (capped at 10) using a new cold_draft mode of the marketing-description-rewrite skill. Operators review and approve through the existing BatchReview UI extended with a two-cell Current column and structured variant columns.
- Retires the Reporting product_health_digest Draft path — Reporting stays registered with a dormant Draft until a real reporting skill lands.
- Spec: docs/specs/2026-05-18-marketing-cold-draft-batch-design.md (committed at cb12f2a).

## Test plan
- [ ] Go unit tests pass: `cd daemon && go test ./...`
- [ ] UI typecheck passes: `cd ui && npm run typecheck`
- [ ] Manual smoke on staging: a batch is produced, two-cell Current column renders, approve writes the right fields, undo restores empty state.
- [ ] Spot-check: a store with <3 empty-copy products still gets single-rewrite proposals (no batch dispatch).

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Link the PR in the Linear ticket (file DSGWOO-XXXX first if not done).**

---

## Self-review (run before claiming the plan is done)

1. **Spec coverage:** Each section of the spec maps to tasks:
   - Architecture (smart Draft, additive) → Tasks 3, 6
   - Batch packaging (BatchSiblings, title, intent, dedup) → Tasks 4, 5
   - UI rendering → Tasks 10-13
   - Apply path → Tasks 8, 9
   - Reporting retirement → Task 14
   - Testing → Tasks 1, 2, 3, 5, 6, 8, 9, 14
   - Migration / rollout → Task 14 covers in-flight handling; Task 15 covers e2e
   - Follow-ups → noted in spec; not implemented in this plan

2. **Placeholder scan:** No "TBD" / "TODO" / "implement later" steps. The skill-template task (7) leaves the exact YAML branching to the engineer because the template syntax must match the loader's expectations — but it specifies the inputs (mode, drafting), the output schema (body_short / body_long), and the verification step.

3. **Type consistency:** `variant` struct has `BodyShort` / `BodyLong` (Go) and `body_short` / `body_long` (JSON / TS). `ProposalType` is `product_cold_draft` consistently. `target.drafting` is `[]string` consistently across daemon, apply dispatch, and UI. `BatchIntent` is `fill_missing_copy` consistently.
