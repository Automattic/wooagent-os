# Marketing Detail KPI Scoring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire real SEO and brand-voice scoring for the Marketing detail KPI tiles so live agent runs produce meaningful 0–100 scores instead of the always-0 placeholder.

**Architecture:** Single LLM round-trip emits `seo` + `voice` per variant alongside the existing `label`/`angle`/`body`. Voice corpus is sampled live per product from the store's longest published descriptions (excluding the product being rewritten). SEO rubric is 6 explicit product-copy checks embedded in the skill prompt. Scoring failures (missing, out-of-range, or suspect-zero) render `—` + "Not yet scored" in the UI instead of the misleading `0%`/`0`.

**Tech Stack:** Go (daemon — ADK Go runtime), TypeScript / React (UI — `@wordpress/ui`, `@wordpress/components`), YAML (skill registry).

**Spec:** `docs/specs/2026-05-18-marketing-kpi-scoring-design.md`
**Linear:** [DSGWOO-1326](https://linear.app/a8c/issue/DSGWOO-1326/wire-real-seo-brand-voice-scoring-for-marketing-detail-kpis)

---

## File Structure

**Created:** none — all changes touch existing files.

**Modified:**

- `daemon/internal/personas/marketing/marketing.go` — extend `llmVariant` + `variant` structs, harden `parseVariants` with score validation, add `fetchVoiceCorpus` + `buildPromptUserMessage` helpers, thread the corpus through `draftForProduct`, remove the stale line-39 comment, log when scores are missing.
- `daemon/internal/personas/marketing/marketing_test.go` — extend `TestParseVariants_*` with score validation cases; add `TestFetchVoiceCorpus`; add `TestBuildPromptUserMessage`.
- `daemon/internal/registry/skills/marketing-description-rewrite/v1.yaml` — extend the system prompt with the SEO rubric + voice corpus instructions, update the JSON output schema to require `seo` and `voice`.
- `ui/src/api/client.ts` — `Variant.seo` / `Variant.voice` become optional; `variantsFromProposal` returns `undefined` (not `0`) for absent fields.
- `ui/src/components/Kpi.tsx` — render `—` (em-dash) when `value` is `undefined`/`null`; suppress the score bar.
- `ui/src/screens/IssueDetail.tsx` — Brand voice + SEO score tile call sites guard on `typeof === 'number'`; relax `voiceToneBand` thresholds (90/75 → 80/65); update hint copy.

**Out of scope (per spec):** no new MCP abilities, no new daemon storage, no proposal-schema migration, no settings UI.

---

## Task 1: Extend `parseVariants` to validate scores

Sets the data-shape contract before anything else uses it.

**Files:**
- Modify: `daemon/internal/personas/marketing/marketing.go:36-102`
- Test: `daemon/internal/personas/marketing/marketing_test.go`

- [ ] **Step 1: Write the failing tests for new score validation cases**

Append these tests to `marketing_test.go`:

```go
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

func TestParseVariants_BothZeroIsSuspect(t *testing.T) {
	// Both scores exactly 0 → treated as suspect (the legacy default that
	// prompted this ticket). Cleared to zero so omitempty drops both fields.
	in := `{"variants":[
		{"label":"A","angle":"material","body":"hello","seo":0,"voice":0},
		{"label":"B","angle":"use","body":"world","seo":80,"voice":80},
		{"label":"C","angle":"story","body":"again","seo":80,"voice":80}
	]}`
	got, err := parseVariants(in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got[0].Seo != 0 || got[0].Voice != 0 {
		t.Errorf("both-zero variant 0: got seo=%d voice=%d, want 0/0", got[0].Seo, got[0].Voice)
	}
	if got[1].Seo != 80 || got[1].Voice != 80 {
		t.Errorf("variant 1 should keep its scores: got seo=%d voice=%d", got[1].Seo, got[1].Voice)
	}
}
```

- [ ] **Step 2: Run the tests and verify they fail to compile**

Run: `cd /Users/elizabethpizzuti/claude/wooagent && go test ./daemon/internal/personas/marketing/ -run TestParseVariants -v`
Expected: compile error — `got[0].Seo` and `.Voice` are undefined on the `variant` struct.

- [ ] **Step 3: Add `Seo` and `Voice` fields to `variant` and `llmVariant`**

In `marketing.go`, modify the structs (around lines 40-55):

```go
// variant is the shape the UI's variantsFromProposal expects to find under
// proposal.target.variants. Keep field names in sync with
// ui/src/api/client.ts:282 — id, body required; label/charCount/recommended
// optional; seo/voice optional (omitempty → UI distinguishes "absent" from
// "real 0" and renders '—' on absent).
type variant struct {
	ID          string `json:"id"`
	Label       string `json:"label,omitempty"`
	Body        string `json:"body"`
	CharCount   int    `json:"charCount,omitempty"`
	Recommended bool   `json:"recommended,omitempty"`
	Angle       string `json:"angle,omitempty"`
	Seo         int    `json:"seo,omitempty"`
	Voice       int    `json:"voice,omitempty"`
}

// llmVariant is what the LLM returns inside its JSON response. Translated
// to the persisted `variant` shape after parsing.
type llmVariant struct {
	Label string `json:"label"`
	Angle string `json:"angle"`
	Body  string `json:"body"`
	Seo   int    `json:"seo"`
	Voice int    `json:"voice"`
}
```

- [ ] **Step 4: Add the score validator inside `parseVariants`**

Replace the loop body inside `parseVariants` (around lines 82-101) with:

```go
out := make([]variant, 0, 3)
for i, v := range parsed.Variants {
	body := strings.TrimSpace(v.Body)
	if body == "" {
		return nil, fmt.Errorf("variant %d has empty body", i)
	}
	label := strings.TrimSpace(v.Label)
	if label == "" {
		label = string(rune('A' + i))
	}
	// Validate scores. Anything out of [1, 100] (note: 0 is also suspect —
	// it's the legacy default that prompted DSGWOO-1326) → clear to 0 so
	// the omitempty JSON tag drops the field; UI then renders '—' for
	// the missing dimension.
	seo := v.Seo
	if seo <= 0 || seo > 100 {
		if seo != 0 {
			fmt.Printf("marketing: variant %d seo out of range (%d), clearing\n", i, seo)
		}
		seo = 0
	}
	voice := v.Voice
	if voice <= 0 || voice > 100 {
		if voice != 0 {
			fmt.Printf("marketing: variant %d voice out of range (%d), clearing\n", i, voice)
		}
		voice = 0
	}
	out = append(out, variant{
		ID:          fmt.Sprintf("var_%s", strings.ToLower(label)),
		Label:       label,
		Body:        body,
		CharCount:   len(body),
		Recommended: i == 0,
		Angle:       strings.TrimSpace(v.Angle),
		Seo:         seo,
		Voice:       voice,
	})
}
return out, nil
```

- [ ] **Step 5: Remove the stale placeholder comment at line 39**

Delete this line from the `variant` struct comment (it's been replaced by the new comment in Step 3):

```go
// optional; seo/voice default to 0 in the UI until real scoring lands.
```

Already removed by Step 3.

- [ ] **Step 6: Run the tests and verify they pass**

Run: `cd /Users/elizabethpizzuti/claude/wooagent && go test ./daemon/internal/personas/marketing/ -run TestParseVariants -v`
Expected: all `TestParseVariants_*` cases PASS (the existing 5 happy/tolerance/rejection tests + the 4 new score tests).

- [ ] **Step 7: Commit**

```bash
cd /Users/elizabethpizzuti/claude/wooagent
git add daemon/internal/personas/marketing/marketing.go daemon/internal/personas/marketing/marketing_test.go
git commit -m "$(cat <<'EOF'
feat(marketing): validate seo + voice scores in parseVariants

Extend the variant + llmVariant structs with Seo/Voice int fields
(omitempty so absent/invalid scores serialize away). parseVariants
clamps anything outside [1, 100] to zero — including a real zero,
which was the legacy "no scoring" default and is therefore treated
as suspect. UI distinguishes absent from real-zero via the
omitempty JSON tag and renders '—' on absent.

Part of DSGWOO-1326.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `fetchVoiceCorpus` helper

Live-samples 3–5 of the store's longest published product descriptions for the voice-match prompt context, excluding the product currently being rewritten.

**Files:**
- Modify: `daemon/internal/personas/marketing/marketing.go` (add helper near the existing MCP helpers, around line 326)
- Test: `daemon/internal/personas/marketing/marketing_test.go`

- [ ] **Step 1: Write the failing test**

Append to `marketing_test.go`:

```go
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

// fakeMCP implements just enough of *mcp.Client for fetchVoiceCorpus.
// CallTool returns the canned bytes; everything else panics so a wrong
// invocation surfaces immediately.
type fakeMCP struct {
	listProductsResp []byte
}

func (f *fakeMCP) CallTool(ctx context.Context, name string, params map[string]any) (*mcp.CallToolResult, error) {
	// Mirrors callAbility's envelope shape.
	envelope := fmt.Sprintf(`{"success":true,"data":%s}`, string(f.listProductsResp))
	return &mcp.CallToolResult{Content: []mcp.Content{{Text: envelope}}}, nil
}

func (f *fakeMCP) Initialize(ctx context.Context) (*mcp.InitializeResult, error) {
	panic("not used by fetchVoiceCorpus")
}
```

Add the imports if not already present at the top of `marketing_test.go`:

```go
import (
	"context"
	"fmt"
	"testing"

	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
)
```

- [ ] **Step 2: Run the test and verify it fails to compile**

Run: `cd /Users/elizabethpizzuti/claude/wooagent && go test ./daemon/internal/personas/marketing/ -run TestFetchVoiceCorpus -v`
Expected: compile errors — `fetchVoiceCorpus` undefined, `corpusSample.Name` undefined. (If `fakeMCP` cannot satisfy the actual `*mcp.Client` type because the test calls a struct-method directly, refactor `fetchVoiceCorpus` to take an interface that both `*mcp.Client` and `fakeMCP` can satisfy — see Step 3.)

- [ ] **Step 3: Implement `fetchVoiceCorpus` with an interface seam**

Add to `marketing.go` near the existing MCP helpers (after the `pickFirstPublished` function, around line 380):

```go
// corpusSample is one product-description sample passed into the marketing
// prompt as a voice reference. Per the design spec, the corpus is the
// store's existing longest published descriptions (excluding the product
// currently being rewritten) — those are the closest available proxy for
// the operator's "good" voice.
type corpusSample struct {
	Name string
	Body string
}

// mcpLister is the subset of *mcp.Client that fetchVoiceCorpus needs.
// Existing code stays on *mcp.Client; the interface exists so tests can
// pass a fake without touching the rest of the package.
type mcpLister interface {
	CallTool(ctx context.Context, name string, params map[string]any) (*mcp.CallToolResult, error)
}

// fetchVoiceCorpus pulls 3–5 of the store's longest published product
// descriptions for use as voice-match context in the marketing prompt.
// Excludes excludeProductID so the LLM isn't grading variants against the
// description it's about to replace. Returns at most 5 samples; fewer is
// fine (new stores, all-thin descriptions). Soft-fails: a non-nil error
// is logged but the caller proceeds with whatever was assembled.
func fetchVoiceCorpus(ctx context.Context, c mcpLister, excludeProductID int) ([]corpusSample, error) {
	const want = 5
	type productListItem struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Status      string `json:"status"`
		Description string `json:"description"`
	}
	var out struct {
		Products []productListItem `json:"products"`
	}
	res, err := c.CallTool(ctx, "mcp-adapter-execute-ability", map[string]any{
		"ability_name": "wc/products",
		"parameters": map[string]any{
			"per_page": 20,
			"orderby":  "date_modified",
			"order":    "desc",
			"status":   "publish",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("fetch voice corpus: %w", err)
	}
	if len(res.Content) == 0 {
		return nil, fmt.Errorf("fetch voice corpus: empty content")
	}
	var env abilityEnvelope
	if err := json.Unmarshal([]byte(res.Content[0].Text), &env); err != nil {
		return nil, fmt.Errorf("decode corpus envelope: %w", err)
	}
	if !env.Success {
		return nil, fmt.Errorf("corpus ability failed: %s", env.Error)
	}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		return nil, fmt.Errorf("decode corpus data: %w", err)
	}

	// Filter (status=publish redundant — server already filtered, but the
	// fake-MCP test data mixes statuses, and defensive filtering costs
	// nothing) and exclude.
	filtered := make([]productListItem, 0, len(out.Products))
	for _, p := range out.Products {
		if p.Status != "publish" || p.ID == excludeProductID {
			continue
		}
		body := strings.TrimSpace(p.Description)
		if body == "" {
			continue
		}
		p.Description = body
		filtered = append(filtered, p)
	}

	// Sort by description length descending, take top N.
	sort.Slice(filtered, func(i, j int) bool {
		return len(filtered[i].Description) > len(filtered[j].Description)
	})
	if len(filtered) > want {
		filtered = filtered[:want]
	}

	samples := make([]corpusSample, 0, len(filtered))
	for _, p := range filtered {
		samples = append(samples, corpusSample{Name: p.Name, Body: p.Description})
	}
	return samples, nil
}
```

Add the `sort` import to the existing import block at the top of `marketing.go`:

```go
import (
	// existing imports …
	"sort"
	// existing imports …
)
```

- [ ] **Step 4: Run the test and verify it passes**

Run: `cd /Users/elizabethpizzuti/claude/wooagent && go test ./daemon/internal/personas/marketing/ -run TestFetchVoiceCorpus -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add daemon/internal/personas/marketing/marketing.go daemon/internal/personas/marketing/marketing_test.go
git commit -m "$(cat <<'EOF'
feat(marketing): fetchVoiceCorpus helper

Pulls 3–5 of the store's longest published product descriptions to
seed the marketing prompt as a voice-match corpus. Excludes the
product currently being rewritten so the LLM doesn't grade variants
against the description they're about to replace.

Soft-fails: returns the assembled samples plus an error on partial
fetch issues; caller proceeds with whatever was returned.

Part of DSGWOO-1326.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `buildPromptUserMessage` helper

Extract and extend the per-product user-message construction. The system prompt (from the YAML skill description) already carries the voice/SEO instructions after Task 4; this helper assembles the dynamic per-product user message that includes the corpus.

**Files:**
- Modify: `daemon/internal/personas/marketing/marketing.go` (near `draftRewriteAnthropic`, around line 470; also touch `draftRewriteOpenAI` symmetrically)
- Test: `daemon/internal/personas/marketing/marketing_test.go`

- [ ] **Step 1: Write the failing golden test**

Append to `marketing_test.go`:

```go
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
	msg := buildPromptUserMessage(p, corpus)

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
}

func TestBuildPromptUserMessage_EmptyCorpus(t *testing.T) {
	p := product{Name: "New Store Product", SKU: "NEW-1", Description: "hi"}
	msg := buildPromptUserMessage(p, nil)
	// Empty corpus → prompt instructs the LLM to emit null for voice.
	if !strings.Contains(msg, "Voice corpus: (none available") {
		t.Errorf("empty-corpus message should mark the gap explicitly\n---\n%s", msg)
	}
	if !strings.Contains(msg, "null") {
		t.Errorf("empty-corpus message should instruct emitting null for voice\n---\n%s", msg)
	}
}
```

- [ ] **Step 2: Run the tests and verify they fail to compile**

Run: `cd /Users/elizabethpizzuti/claude/wooagent && go test ./daemon/internal/personas/marketing/ -run TestBuildPromptUserMessage -v`
Expected: compile error — `buildPromptUserMessage` undefined.

- [ ] **Step 3: Implement `buildPromptUserMessage`**

Add to `marketing.go` near the LLM helpers (around line 440, just before `draftRewriteAnthropic`):

```go
// buildPromptUserMessage assembles the per-product user message for the
// marketing draft call. The system prompt (carried in the skill YAML
// description) holds the voice + SEO rubric instructions; this helper
// supplies the dynamic per-product context: the product to rewrite plus
// the corpus samples the LLM compares the variants' voice to.
//
// Empty corpus is supported (new stores, all-thin descriptions). In that
// case the prompt tells the LLM to emit null for the voice field — the
// Go validator clears anything ≤ 0 anyway, so this is belt-and-suspenders.
func buildPromptUserMessage(p product, corpus []corpusSample) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Product: %s\nSKU: %s\nCurrent description: %s\n\n",
		p.Name, p.SKU, strings.TrimSpace(p.Description))
	if len(corpus) == 0 {
		b.WriteString("Voice corpus: (none available — this store has no other long-form published descriptions to compare against. Emit null for `voice` on each variant; score SEO as normal.)\n\n")
	} else {
		b.WriteString("Voice corpus (3–5 of this store's existing published descriptions — use these as the reference for the store's voice; do not copy):\n")
		for _, s := range corpus {
			fmt.Fprintf(&b, "\n— %s —\n%s\n", s.Name, s.Body)
		}
		b.WriteString("\n")
	}
	b.WriteString("Write the THREE rewrite variants per the system instructions. Score each variant 0–100 for `seo` and `voice` using the rubrics in the system prompt. Return JSON only.")
	return b.String()
}
```

- [ ] **Step 4: Run the tests and verify they pass**

Run: `cd /Users/elizabethpizzuti/claude/wooagent && go test ./daemon/internal/personas/marketing/ -run TestBuildPromptUserMessage -v`
Expected: both cases PASS.

- [ ] **Step 5: Commit**

```bash
git add daemon/internal/personas/marketing/marketing.go daemon/internal/personas/marketing/marketing_test.go
git commit -m "$(cat <<'EOF'
feat(marketing): buildPromptUserMessage assembles per-product context

Replaces the previously-inline user-message construction in
draftRewriteAnthropic / draftRewriteOpenAI. The helper takes the
product + the voice corpus and emits the dynamic per-product context:
product fields, corpus samples, and the score-emission directive that
matches the (forthcoming) updated system prompt.

Empty-corpus path is explicit — the message tells the LLM to emit
null for voice on each variant. The Go validator clears anything
≤ 0 anyway, so this is belt-and-suspenders.

Part of DSGWOO-1326.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Update the skill YAML system prompt + output schema

Updates the system prompt (the `description:` field of the skill YAML) with voice + SEO rubric instructions and extends the output schema to require the two new score fields.

**Files:**
- Modify: `daemon/internal/registry/skills/marketing-description-rewrite/v1.yaml`

- [ ] **Step 1: Update the `description:` (system prompt)**

Replace the entire `description:` block in `v1.yaml` with:

```yaml
description: |
  You are a copywriter for a small-batch home-goods store.
  Voice: warm, sincere, concrete. Avoid the words "luxe", "premium", "elevate",
  "curated". Prefer "small-batch", "handcrafted", "made to last". Lead with the
  material or the use, not adjectives. Two to four short sentences, 140-220
  characters total per variant.

  Draft THREE distinct rewrite variants — each takes a different angle (e.g.
  material-first, use-first, story-first). For each variant, also emit two
  integer scores in [1, 100]:

  - `seo`: how well the variant satisfies the product-copy SEO rubric below.
  - `voice`: how well the variant matches the store's voice corpus (when
    provided in the user message). Reward natural alignment in phrasing,
    register, and lexical choice — do not reward copying. When the user
    message says the corpus is unavailable, emit `null` for `voice`.

  SEO rubric (six checks, each contributes ~16 points; round the total):
  1. Focus keyphrase = the product name. The body uses it naturally (not
     necessarily verbatim — common-noun paraphrases are fine).
  2. Keyphrase coverage: the product's noun phrase or a close paraphrase
     appears at least once across the body (in 4 sentences or fewer).
  3. Length adequacy: 140–220 characters (matches the variant cap above).
     A variant that is too short or too long fails this check.
  4. Benefit-led opener: first sentence leads with what the product does
     for the customer or its standout property, not the product's name or
     category.
  5. Scannability: short sentences (median ≤ 20 words), 2–4 sentences total
     (matches the variant cap above).
  6. Specificity: the body names at least two concrete features, materials,
     or measurements (e.g., "wood-fired", "12oz", "two-ply", "stoneware").

  Return JSON ONLY, no preamble:

  {"variants":[{"label":"A","angle":"material","body":"...","seo":85,"voice":78},
               {"label":"B","angle":"use","body":"...","seo":72,"voice":90},
               {"label":"C","angle":"story","body":"...","seo":80,"voice":65}]}

  Constraints:
  - Exactly three variants.
  - Distinct bodies (don't paraphrase the same sentence three times).
  - Each body 140-220 characters of plain prose, no markdown, no labels in
    the body.
  - `seo` and `voice` are integers in [1, 100]. Emit `null` for `voice` when
    the user message says the corpus is unavailable.
```

- [ ] **Step 2: Extend the output schema**

In the same file, replace the `output:` block with:

```yaml
  output:
    type: object
    required: [variants]
    properties:
      variants:
        type: array
        minItems: 3
        maxItems: 3
        items:
          type: object
          required: [label, angle, body, seo, voice]
          properties:
            label:
              type: string
              enum: [A, B, C]
            angle:
              type: string
              description: |
                The angle this variant takes (e.g. "material", "use",
                "story"). Free-form so the model can extend the angle set
                when warranted — but A/B/C labels are fixed.
            body:
              type: string
              minLength: 140
              maxLength: 220
              description: Plain prose, two to four short sentences. No markdown, no labels.
            seo:
              type: integer
              minimum: 1
              maximum: 100
              description: |
                Product-copy SEO score per the rubric in the system prompt.
                Out-of-range or missing → daemon clears to 0 → UI renders '—'.
            voice:
              type:
                - integer
                - "null"
              description: |
                Voice-corpus match score in [1, 100]. `null` is valid when
                no corpus was provided in the user message (new-store path).
```

- [ ] **Step 3: Verify the YAML still parses**

Run: `cd /Users/elizabethpizzuti/claude/wooagent && go build ./...`
Expected: build succeeds. If the skill registry's loader chokes on YAML, the build will fail; the error message will pinpoint the line.

- [ ] **Step 4: Run the full daemon test suite to make sure nothing regressed**

Run: `cd /Users/elizabethpizzuti/claude/wooagent && go test ./daemon/...`
Expected: all PASS. If the skill-registry loader has a YAML-validation test that fails because the new schema is stricter than existing seed data, that's a separate fix — surface the failing test name and pause.

- [ ] **Step 5: Commit**

```bash
git add daemon/internal/registry/skills/marketing-description-rewrite/v1.yaml
git commit -m "$(cat <<'EOF'
feat(marketing): extend skill yaml with seo/voice rubric + output schema

System prompt now carries the 6-check product-copy SEO rubric and
the voice-match instructions (corpus-aware, null-when-absent). Output
schema requires `seo` (integer, 1-100) and `voice` (integer 1-100 or
null) on every variant.

The actual corpus samples are passed in the dynamic user message
(buildPromptUserMessage); only the rubric + scoring directives live
in the static system prompt.

Part of DSGWOO-1326.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Wire `fetchVoiceCorpus` + `buildPromptUserMessage` into `draftForProduct`

Replaces the inline user-message construction and threads the corpus through the LLM call paths.

**Files:**
- Modify: `daemon/internal/personas/marketing/marketing.go` — `draftForProduct` (lines 198-258), `draftWithFallback` (line 401), `draftRewriteAnthropic` (line 470), `draftRewriteOpenAI` (find it lower in the file).

- [ ] **Step 1: Extend `draftWithFallback`'s signature to accept the corpus**

Find `draftWithFallback` (line 401) and change its signature to take `corpus []corpusSample`. Update both call sites within the function to thread the corpus into `draftRewriteAnthropic` and `draftRewriteOpenAI`.

```go
func draftWithFallback(ctx context.Context, env personas.Env, p product, skillDescription string, corpus []corpusSample) (string, string, error) {
	if strings.TrimSpace(env.AnthropicAPIKey) != "" {
		model := env.AnthropicModel
		if model == "" {
			model = defaultAnthropicModel
		}
		rewrite, err := draftRewriteAnthropic(ctx, env.AnthropicAPIKey, model, p, skillDescription, corpus)
		// … rest unchanged
	}
	// … OpenAI branch:
	rewrite, err := draftRewriteOpenAI(ctx, base, apiKey, model, p, skillDescription, corpus)
	// … rest unchanged
}
```

- [ ] **Step 2: Update `draftRewriteAnthropic` to use the corpus**

Modify the signature + replace the inline `user :=` construction (around line 470-474) with a call to `buildPromptUserMessage`:

```go
func draftRewriteAnthropic(ctx context.Context, apiKey, model string, p product, skillDescription string, corpus []corpusSample) (string, error) {
	user := buildPromptUserMessage(p, corpus)
	// … rest of the function unchanged (the user variable is sent
	// as the message content to Anthropic).
```

- [ ] **Step 3: Update `draftRewriteOpenAI` the same way**

Find `draftRewriteOpenAI` (likely shaped identically to `draftRewriteAnthropic` further down in the file) and apply the same signature + body change:

```go
func draftRewriteOpenAI(ctx context.Context, base, apiKey, model string, p product, skillDescription string, corpus []corpusSample) (string, error) {
	user := buildPromptUserMessage(p, corpus)
	// … rest unchanged
```

- [ ] **Step 4: Update `draftForProduct` to fetch the corpus and pass it along**

Modify `draftForProduct` (line 198) to call `fetchVoiceCorpus` after `getProduct` and pass the corpus into `draftWithFallback`:

```go
func draftForProduct(ctx context.Context, deps personas.Deps, productID int, skillDescription string) (personas.Drafted, error) {
	p, err := getProduct(ctx, deps.MCP, productID)
	if err != nil {
		return personas.Drafted{}, fmt.Errorf("get product %d: %w", productID, err)
	}

	// Voice corpus: live-sampled per attempt. Soft-fails (logs + empties)
	// — the prompt's empty-corpus path handles that case explicitly.
	corpus, corpusErr := fetchVoiceCorpus(ctx, deps.MCP, productID)
	if corpusErr != nil {
		fmt.Printf("marketing: voice corpus fetch errored (%v); proceeding with empty corpus\n", corpusErr)
		corpus = nil
	}

	rawOutput, skipReason, err := draftWithFallback(ctx, deps.Env, p, skillDescription, corpus)
	// … rest of draftForProduct unchanged
```

- [ ] **Step 5: Verify everything builds and existing tests still pass**

Run: `cd /Users/elizabethpizzuti/claude/wooagent && go build ./... && go test ./daemon/internal/personas/marketing/ -v`
Expected: build succeeds; all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add daemon/internal/personas/marketing/marketing.go
git commit -m "$(cat <<'EOF'
feat(marketing): thread voice corpus through draftForProduct

draftForProduct now fetches the voice corpus per attempt and threads
it through draftWithFallback → draftRewriteAnthropic/OpenAI →
buildPromptUserMessage. Corpus fetch is soft: an error logs and
proceeds with nil; the prompt's empty-corpus branch handles new
stores and other coverage gaps.

Worst-case MCP cost per Marketing.Draft run is 3 list calls (bounded
by maxDraftAttempts), each ~1s on staging — within the run's
existing latency budget.

Part of DSGWOO-1326.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: UI — make `Variant.seo` / `Variant.voice` optional in `client.ts`

The wire still carries integers when scoring succeeds; absent fields (after the daemon's omitempty) become `undefined` in TS, driving the UI's "Not yet scored" branch.

**Files:**
- Modify: `ui/src/api/client.ts:282-336`

- [ ] **Step 1: Modify the `Variant` interface**

Change lines 286–287 of `client.ts`:

```ts
export interface Variant {
  id: string;
  label: string;
  body: string;
  seo?: number;
  voice?: number;
  charCount: number;
  recommended?: boolean;
  note?: string;
  angle?: string;
}
```

- [ ] **Step 2: Modify `variantsFromProposal` to preserve absence**

Change the relevant lines in `variantsFromProposal` (around lines 322–333):

```ts
out.push({
  id: r.id,
  label: typeof r.label === 'string' ? r.label : r.id,
  body: r.body,
  seo: typeof r.seo === 'number' ? r.seo : undefined,
  voice: typeof r.voice === 'number' ? r.voice : undefined,
  charCount:
    typeof r.charCount === 'number' ? r.charCount : r.body.length,
  recommended: r.recommended === true,
  note: typeof r.note === 'string' ? r.note : undefined,
  angle: typeof r.angle === 'string' ? r.angle : undefined,
});
```

- [ ] **Step 3: Type-check (this will surface every call-site that needs updating)**

Run: `cd /Users/elizabethpizzuti/claude/wooagent/ui && npx tsc --noEmit`
Expected: errors at the IssueDetail KPI call sites and any other readers that pass `seo` / `voice` as required numbers — those are exactly the spots Task 8 patches. Note the file/line of each error and confirm they're all in `IssueDetail.tsx` and `BatchReview.tsx`/related. If errors appear in unexpected files, surface them.

- [ ] **Step 4: Commit (type errors in IssueDetail are expected at this step)**

```bash
cd /Users/elizabethpizzuti/claude/wooagent
git add ui/src/api/client.ts
git commit -m "$(cat <<'EOF'
refactor(ui): make Variant.seo / .voice optional

Daemon now emits both fields with omitempty when scoring fails or is
unavailable. UI treats absent as the "not yet scored" signal — distinct
from a real 0. Subsequent commits update the KPI call sites and the
Kpi component to render '—' when absent.

Part of DSGWOO-1326.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: UI — `Kpi` component renders `—` when `value` is absent

The Kpi component already accepts `value: ReactNode`. We extend its rendering so `undefined` / `null` produces an em-dash and suppresses the score bar.

**Files:**
- Modify: `ui/src/components/Kpi.tsx`

- [ ] **Step 1: Add the missing-value branch to `Kpi`**

Replace the `<Text … >{value}</Text>` block (lines 49–58) with:

```tsx
<Text
  style={{
    color: TONE_FG[tone],
    fontSize: 'var(--wpds-typography-font-size-lg)',
    fontWeight: 700,
    lineHeight: 'var(--wpds-typography-line-height-lg)',
  }}
>
  {value ?? '—'}
</Text>
```

And update the score-bar conditional (lines 59–63) so the bar is also suppressed when value is missing:

```tsx
{typeof score === 'number' && value != null && (
  <div className="wa-score-bar" style={barStyle} aria-hidden="true">
    <div style={{ width: `${Math.max(0, Math.min(100, score))}%` }} />
  </div>
)}
```

(Note: `value != null` is intentional `==` semantics — catches both `undefined` and `null`. The Kpi caller in Task 8 passes `undefined`.)

- [ ] **Step 2: Type-check**

Run: `cd /Users/elizabethpizzuti/claude/wooagent/ui && npx tsc --noEmit`
Expected: still the same set of errors at the IssueDetail call sites (Task 6 surfaced them). Nothing new in `Kpi.tsx` itself.

- [ ] **Step 3: Commit**

```bash
git add ui/src/components/Kpi.tsx
git commit -m "$(cat <<'EOF'
feat(ui): Kpi renders '—' when value is absent

When a caller passes value={undefined}, the tile shows an em-dash
in place of the metric and suppresses the score bar. Tone bands,
label, and hint remain caller-controlled — typical use is to pair
{undefined} with hint='Not yet scored'.

Part of DSGWOO-1326.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: UI — update IssueDetail KPI call sites + tone band thresholds + hint copy

Wire the new "Not yet scored" branch on the Marketing detail's KPI strip. Update the voice-score tone-band threshold per the spec.

**Files:**
- Modify: `ui/src/screens/IssueDetail.tsx:53-62` (tone bands), `:350-370` (KPI tile call sites — exact lines may have shifted from earlier edits; locate the two `<Kpi label="Brand voice match" …/>` and `<Kpi label="SEO score" …/>` calls in the prose-path KPI row).

- [ ] **Step 1: Relax `voiceToneBand` thresholds**

In `IssueDetail.tsx`, change `voiceToneBand` (around line 58):

```ts
// Score → Kpi tone. SEO uses success/caution/warning. Voice uses brand
// (matches the Figma frame's blue for high-match voice) / caution / warning.
// Voice thresholds relaxed (was 90/75 → 80/65) per the corpus-based scoring
// design in docs/specs/2026-05-18-marketing-kpi-scoring-design.md — 90%
// against a small sample is unrealistic.
function voiceToneBand(score: number): KpiTone {
  if (score >= 80) return 'brand';
  if (score >= 65) return 'caution';
  return 'warning';
}
```

Leave `seoToneBand` unchanged (80/70 thresholds still apply).

- [ ] **Step 2: Update the Brand voice match tile**

Locate the existing `<Kpi label="Brand voice match" …` (around line 354) and replace with:

```tsx
<Kpi
  label="Brand voice match"
  value={
    typeof activeVariant?.voice === 'number'
      ? `${activeVariant.voice}%`
      : undefined
  }
  score={
    typeof activeVariant?.voice === 'number'
      ? activeVariant.voice
      : undefined
  }
  tone={
    typeof activeVariant?.voice === 'number'
      ? voiceToneBand(activeVariant.voice)
      : 'neutral'
  }
  hint={
    typeof activeVariant?.voice === 'number'
      ? 'vs. your existing copy'
      : 'Not yet scored'
  }
/>
```

- [ ] **Step 3: Update the SEO score tile**

Locate the existing `<Kpi label="SEO score" …` (around line 361) and replace with:

```tsx
<Kpi
  label="SEO score"
  value={
    typeof activeVariant?.seo === 'number'
      ? String(activeVariant.seo)
      : undefined
  }
  score={
    typeof activeVariant?.seo === 'number'
      ? activeVariant.seo
      : undefined
  }
  tone={
    typeof activeVariant?.seo === 'number'
      ? seoToneBand(activeVariant.seo)
      : 'neutral'
  }
  hint={
    typeof activeVariant?.seo === 'number'
      ? 'Product-copy rubric · out of 100'
      : 'Not yet scored'
  }
/>
```

- [ ] **Step 4: Type-check — all errors from Task 6 should now resolve**

Run: `cd /Users/elizabethpizzuti/claude/wooagent/ui && npx tsc --noEmit`
Expected: clean (no errors). If anything remains, the most likely culprit is another caller of `Variant.seo` / `.voice` — check `BatchReview.tsx`, the prose-path variant scores in IssueDetail, and any other detail components.

- [ ] **Step 5: Commit**

```bash
git add ui/src/screens/IssueDetail.tsx
git commit -m "$(cat <<'EOF'
feat(ui): render real SEO + voice scores on the Marketing detail KPIs

Brand voice match + SEO score tiles now guard on typeof === 'number'.
When the daemon omits the field (failed scoring), the tile renders
'—' + 'Not yet scored' instead of a misleading 0%/0.

Voice-band threshold drops from 90/75 → 80/65 per the corpus-based
scoring design — 90% against a small sample was unrealistic.

SEO hint updates from 'Yoast · out of 100' to 'Product-copy rubric
· out of 100' since we're not running Yoast — the rubric is the
6-check product-copy heuristic now embedded in the marketing skill
system prompt.

Part of DSGWOO-1326.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Manual end-to-end smoke + verify-before-done

No code changes — verification step that the spec explicitly requires.

**Files:** none (read-only verification).

- [ ] **Step 1: Build + run the full test suite**

Run: `cd /Users/elizabethpizzuti/claude/wooagent && go build ./... && go test ./daemon/... && cd ui && npx tsc --noEmit`
Expected: build green, all Go tests PASS, type-check clean.

- [ ] **Step 2: Start the daemon + UI**

```bash
# Terminal 1
cd /Users/elizabethpizzuti/claude/wooagent && \
  ANTHROPIC_API_KEY=sk-... \
  WOOAGENT_MCP_URL='https://woo-demo-store-99cc5c.mystagingwebsite.com/wp-json/...' \
  WOOAGENT_MCP_USER='...' \
  WOOAGENT_MCP_APP_PASSWORD='...' \
  go run ./cmd/wooagent-daemon

# Terminal 2
cd /Users/elizabethpizzuti/claude/wooagent/ui && npm run dev
```

- [ ] **Step 3: Trigger a fresh marketing run**

From the UI's Agents screen, click "Run now" on Marketing (or wait for the scheduler). Watch the daemon log for the new `marketing.score_missing` lines — they should NOT appear on a healthy run.

- [ ] **Step 4: Inspect the resulting issue on the Marketing detail**

Confirm visually:
- Brand voice match tile reads a real percentage (probably 60–90%) with a tone band cue (brand/caution/warning).
- SEO score tile reads a real integer (probably 50–95) with a tone band cue.
- Hints read "vs. your existing copy" and "Product-copy rubric · out of 100".

- [ ] **Step 5: Force the "Not yet scored" branch (one variant pass)**

Temporarily edit the skill YAML to drop the `seo` property from the output schema and remove the `seo` field from the example JSON in the system prompt. Restart the daemon, trigger another marketing run, confirm the SEO tile renders `—` + "Not yet scored". Revert the YAML change.

- [ ] **Step 6: Quick visual pass on the seeded showcase data**

Navigate to a board card seeded by `daemon/scripts/seed-demo-merino-turtleneck.sh` (these still hardcode scores). Confirm the tiles render the seeded scores correctly — the optional-field migration should be a no-op for the seeded shape.

- [ ] **Step 7: No commit (verification-only). Confirm working tree is clean.**

```bash
git status --short
```
Expected: clean working tree (only optionally the temporary YAML edit from Step 5, which should already be reverted).

---

## Out-of-band follow-ups (not in this plan)

- **Telemetry counter.** The spec's `marketing.score_missing` counter currently surfaces only via `fmt.Printf` logs. The daemon's telemetry system (`internal/telemetry`) is event-based (per-turn `ModelCall` / `SkillCall` records), not Prometheus-style counters. Adding a structured score-missing event needs a small extension to `turn_event.go` + recorder shape — file a Linear follow-up.
- **Seeded score backwards compatibility.** The two seed scripts emit hardcoded scores that the optional-field migration silently accepts. If the seeded scores stop reflecting realistic distribution, refresh both scripts; this plan does not touch them.
- **Voice corpus sampling refinement.** "Top N longest published" is a heuristic. If real runs show "longest" diverges from "most on-brand", consider operator-curated picks (a settings UI). Out of scope for this PR.
