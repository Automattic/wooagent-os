// Package marketing is the Marketing-agent implementation.
//
// Side-effect-registers itself with personas.Register on import. Reads a
// product via MCP, drafts a brand-voice rewrite, lands a proposal of type
// product_description_rewrite.
//
// LLM routing:
//   - When ANTHROPIC_API_KEY is set, calls Claude directly via /v1/messages
//     (matches pricing and sales-support; this is the production path).
//   - Otherwise falls back to an OpenAI-compatible chat-completions
//     endpoint (LM Studio's gemma by default) — kept as a no-key escape
//     hatch for local spike work.
//
// Skipped outcomes (no daemon failure):
//   - MCP not configured
//   - LLM call errored (Claude rate-limit, LM Studio not running, etc.)
//   - The store has no published products
package marketing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
	"github.com/wooagent-os/wooagent-os/daemon/internal/telemetry"
)

// draftOpts threads per-call options through the LLM call stack. Zero
// value (Mode=="") defaults to the rewrite path for back-compat with
// callers that don't care.
type draftOpts struct {
	Mode     string   // "" or "rewrite" → rewrite path; "cold_draft" → cold-draft path
	Drafting []string // for cold_draft: which fields to fill ("short", "long")
}

func (o draftOpts) isColdDraft() bool { return o.Mode == "cold_draft" }

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

// llmVariant is what the LLM returns inside its JSON response. Translated
// to the persisted `variant` shape after parsing. Note: a JSON `null` on
// the Seo/Voice int fields unmarshals to 0, which the parseVariants
// validator clears via the <= 0 guard — so the prompt-instructed
// "emit null when corpus unavailable" path lands at the same end state
// as a missing field.
type llmVariant struct {
	Label string `json:"label"`
	Angle string `json:"angle"`
	Body  string `json:"body"`
	Seo   int    `json:"seo"`
	Voice int    `json:"voice"`
}

type llmVariantsResp struct {
	Variants []llmVariant `json:"variants"`
}

// jsonObjectRe extracts the first {...} block from a possibly-noisy LLM
// response. Permissive on whitespace and surrounding chatter so a stray
// "Here is the JSON:" prefix doesn't fail the parse.
var jsonObjectRe = regexp.MustCompile(`(?s)\{.*\}`)

// parseVariants pulls a structured 3-variant response out of LLM text. The
// caller falls back to single-variant on a non-nil error. Strict on count
// (must be 3) to keep the contract with the operator clear; permissive on
// the surrounding chatter via jsonObjectRe.
func parseVariants(raw string) ([]variant, error) {
	block := jsonObjectRe.FindString(raw)
	if block == "" {
		return nil, fmt.Errorf("no JSON object found in LLM output")
	}
	var parsed llmVariantsResp
	if err := json.Unmarshal([]byte(block), &parsed); err != nil {
		return nil, fmt.Errorf("decode variants JSON: %w", err)
	}
	if len(parsed.Variants) != 3 {
		return nil, fmt.Errorf("expected 3 variants, got %d", len(parsed.Variants))
	}
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
		// TODO(DSGWOO-1326 follow-up): emit a structured telemetry counter
		// (marketing.score_missing{kind="seo|voice"}) when scores are cleared.
		// Counter sink doesn't exist in internal/telemetry yet (turn-event-only
		// shape); deferred per the plan's "Out-of-band follow-ups" section.
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
}

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
// missing required field on any variant is an error so the caller falls
// back rather than persisting a partial proposal. Returns variants ready
// to persist (BodyShort/BodyLong set, Body left empty).
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

const (
	defaultAnthropicModel = "claude-sonnet-4-6"
	anthropicAPIURL       = "https://api.anthropic.com/v1/messages"
	anthropicVersion      = "2023-06-01"

	defaultOpenAIBase  = "http://localhost:1234/v1"
	defaultOpenAIModel = "google/gemma-4-e4b"
	defaultOpenAIKey   = "lm-studio"
)

func init() {
	personas.Register(&Marketing{})
}

type Marketing struct{}

func (Marketing) Slug() string        { return "marketing" }
func (Marketing) DisplayName() string { return "Marketing agent" }

// Cooldown: product-centric (proposal_target.product_id). 7d after an
// approve so we don't rewrite the same description we just wrote; 30d
// after a dismiss so the operator's "no" sticks.
func (Marketing) Cooldown() personas.CooldownPolicy {
	return personas.CooldownPolicy{
		TargetKey: "product_id",
		Approved:  7 * 24 * time.Hour,
		Dismissed: 30 * 24 * time.Hour,
	}
}

const skillName = "marketing.description-rewrite"

// maxDraftAttempts caps how many products a single Draft run will try
// before giving up. Each attempt costs one LLM call (~5-15s) plus one
// MCP list call for the voice corpus (~1s on staging). 3 is a pragmatic
// balance: most catalogs have only a few "undraftable" products at any
// given time, and bounding latency keeps a run from hogging the worker.
// The cooldown set is appended to in-memory after each LLM no_proposal
// so the loop doesn't re-pick the same product.
const maxDraftAttempts = 3

const (
	coldDraftMin = 3  // require >= this many empty-copy candidates to emit a batch
	coldDraftMax = 10 // cap per-tick LLM cost and operator review surface
)

func (Marketing) Draft(ctx context.Context, deps personas.Deps) (personas.Drafted, error) {
	if deps.MCP == nil {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: "MCP client not configured (set WOOAGENT_MCP_URL/USER/APP_PASSWORD on the daemon)",
		}, nil
	}

	if _, err := deps.MCP.Initialize(ctx); err != nil {
		return personas.Drafted{}, fmt.Errorf("mcp initialize: %w", err)
	}

	skill, ok := deps.Skills[skillName]
	if !ok {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("skill %q not found in skills registry", skillName),
		}, nil
	}

	// Debug override: always draft on the operator-supplied product,
	// bypassing both cooldown and the within-run loop.
	if deps.Env.ProductIDOverride != 0 {
		return draftForProduct(ctx, deps, deps.Env.ProductIDOverride, skill.Description)
	}

	// Skip products that already have an open issue, an approved
	// proposal within the last 7d, or a dismissed proposal within the
	// last 30d for this persona (see Marketing.Cooldown).
	m := Marketing{}
	skip, err := personas.RecentlyTouchedTargets(ctx, deps.Store, m.Slug(), m.Cooldown())
	if err != nil {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("look up recently-touched products: %v", err),
		}, nil
	}

	// Cold-draft batch path: if enough products have empty short/long
	// descriptions out of cooldown, draft them in one batch this tick
	// instead of the single-rewrite loop. Operator-targeted runs
	// (ProductIDOverride) already bypassed this above.
	// See docs/specs/2026-05-18-marketing-cold-draft-batch-design.md.
	candidates, err := pickColdDraftCandidates(ctx, deps.MCP, skip, coldDraftMax)
	if err != nil {
		fmt.Printf("marketing: cold-draft candidate scan errored (%v); falling back to single-rewrite\n", err)
	} else if len(candidates) >= coldDraftMin {
		batch, err := draftColdDraftBatch(ctx, deps, candidates, skill.Description, draftColdDraftForProduct)
		if err != nil {
			return personas.Drafted{}, err
		}
		if !batch.Skipped {
			return batch, nil
		}
		fmt.Printf("marketing: cold-draft batch skipped (%s); falling back to single-rewrite\n", batch.SkipReason)
	}

	// Within-run iteration: if the LLM can't draft for a product (returns
	// no_proposal / empty rewrite), add it to the run-local skip set and
	// try the next eligible product. Up to maxDraftAttempts.
	return personas.IterateDraft(
		maxDraftAttempts,
		"product",
		skip,
		func(s map[int]struct{}) (int, error) { return pickFirstPublished(ctx, deps.MCP, s) },
		func(id int) (personas.Drafted, error) { return draftForProduct(ctx, deps, id, skill.Description) },
	)
}

// draftForProduct does the per-product work: fetch via MCP, call the
// LLM, parse variants, assemble Drafted. Returns Drafted{Skipped:true}
// when the LLM yields no_proposal or an empty rewrite — the outer Draft
// loop treats that as "try the next product" rather than ending the run.
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

	rawOutput, skipReason, err := draftWithFallback(ctx, deps.Env, p, skillDescription, corpus, draftOpts{})
	if err != nil {
		return personas.Drafted{}, err
	}
	if skipReason != "" {
		return personas.Drafted{
			Skipped:    true,
			SkipReason: skipReason,
		}, nil
	}

	// Try to parse the structured 3-variant response. On parse failure,
	// fall back to single-variant with the raw text as content so the
	// operator still gets something actionable instead of a skipped run.
	variants, parseErr := parseVariants(rawOutput)
	target := map[string]any{
		"product_id":   p.ID,
		"product_name": p.Name,
		"product_sku":  p.SKU,
		"previous":     p.Description,
		"image_url":    p.ImageURL,
		"image_alt":    p.ImageAlt,
	}
	var content string
	if parseErr == nil {
		target["variants"] = variants
		// proposal.content is the recommended variant's body. The UI's
		// multi-variant view reads target.variants; surfaces that handle
		// single-content (run log, archived row, dismiss dialog body) get
		// a coherent string instead of raw JSON.
		content = variants[0].Body
	} else {
		// Single-variant fallback — surface the parse failure in the
		// daemon log so it's visible during the testing-call pass.
		fmt.Printf("marketing: variants parse failed (%v); falling back to single-variant\n", parseErr)
		content = strings.TrimSpace(rawOutput)
	}

	return personas.Drafted{
		Title: fmt.Sprintf("Product description rewrite · %s", p.Name),
		Description: fmt.Sprintf(
			"Drafted by Marketing agent for product #%d (%s).",
			p.ID, p.SKU,
		),
		Priority:        "medium",
		ProposalType:    "product_description_rewrite",
		ProposalContent: content,
		Target:          target,
		// Belt over the existing product_id Cooldown — the picker
		// already skips products with open issues, but RunAndPersist's
		// insert-time guard catches races that bypass the picker.
		// See docs/specs/2026-05-18-agent-proposal-dedup-design.md.
		DedupKey: fmt.Sprintf("product:%d", p.ID),
	}, nil
}

// ---- MCP helpers (private to this package) ----

type abilityEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error,omitempty"`
}

// callAbility wraps mcp.CallTool with TurnEvent skill-call recording. The
// inner func does the actual work; the wrapper measures latency + reports
// status to the tracker on ctx (no-op if no tracker is attached).
func callAbility(ctx context.Context, c *mcp.Client, ability string, params map[string]any, out any) error {
	start := time.Now()
	err := callAbilityInner(ctx, c, ability, params, out)
	telemetry.RecordSkillCallFromError(ctx, ability, time.Since(start), err)
	return err
}

func callAbilityInner(ctx context.Context, c *mcp.Client, ability string, params map[string]any, out any) error {
	res, err := c.CallTool(ctx, "mcp-adapter-execute-ability", map[string]any{
		"ability_name": ability,
		"parameters":   params,
	})
	if err != nil {
		return fmt.Errorf("mcp call %s: %w", ability, err)
	}
	if len(res.Content) == 0 {
		return fmt.Errorf("mcp %s: empty content", ability)
	}
	var env abilityEnvelope
	if err := json.Unmarshal([]byte(res.Content[0].Text), &env); err != nil {
		return fmt.Errorf("decode envelope (%s): %w body=%s", ability, err, res.Content[0].Text)
	}
	if !env.Success {
		return fmt.Errorf("ability %s failed: %s", ability, env.Error)
	}
	if out != nil {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("decode data (%s): %w body=%s", ability, err, string(env.Data))
		}
	}
	return nil
}

type productSummary struct {
	ID                     int    `json:"id"`
	Name                   string `json:"name"`
	SKU                    string `json:"sku"`
	Status                 string `json:"status"`
	DescriptionLength      int    `json:"description_length"`
	ShortDescriptionLength int    `json:"short_description_length"`
}

// pickFirstPublished returns the next published product whose ID is not
// in skip. Surfaces stale + content-thin products first (orderby=date_modified
// asc, then a client-side bias toward products with empty descriptions —
// "data_issues" in the WC AI plugin's behavioral vocabulary). Marketing's
// job is to provide descriptions for products that need them most; sorting
// by date_modified ascending puts the products no one's touched in a while
// at the top, and the data_issues bias prefers ones missing the content
// Marketing can supply.
//
// orderby is passed through to the Companion Plugin (v0.2+ accepts it). On
// stores still running v0.1 of the plugin the arg is rejected by the
// ability's additionalProperties:false schema — surfaces as an explicit
// error rather than a silent mis-order.
func pickFirstPublished(ctx context.Context, c *mcp.Client, skip map[int]struct{}) (int, error) {
	var listOut struct {
		Products []productSummary `json:"products"`
	}
	if err := callAbility(ctx, c, "wooagent-products/list",
		map[string]any{"per_page": 100, "orderby": "date_modified", "order": "asc"}, &listOut); err != nil {
		return 0, err
	}
	if len(listOut.Products) == 0 {
		return 0, fmt.Errorf("no products in store")
	}
	// Two-pass: first prefer products with an obvious data_issue (empty
	// description); fall back to first-eligible if no data_issues remain.
	skipped := 0
	for _, p := range listOut.Products {
		if p.Status != "publish" && p.Status != "" {
			continue
		}
		if _, inCooldown := skip[p.ID]; inCooldown {
			skipped++
			continue
		}
		if p.DescriptionLength == 0 || p.ShortDescriptionLength == 0 {
			return p.ID, nil
		}
	}
	for _, p := range listOut.Products {
		if p.Status != "publish" && p.Status != "" {
			continue
		}
		if _, inCooldown := skip[p.ID]; inCooldown {
			continue
		}
		return p.ID, nil
	}
	if skipped > 0 {
		return 0, fmt.Errorf(
			"every published product in the first %d is in cooldown (%d skipped); "+
				"approved proposals cool down for 7d, dismissed for 30d",
			len(listOut.Products), skipped,
		)
	}
	return 0, fmt.Errorf("no published products in store")
}

// pickColdDraftCandidates returns up to max published products where
// short_description OR long_description is empty, excluding products in
// skip. Order matches the underlying list call (date_modified asc) so
// the stalest products surface first — same intuition as
// pickFirstPublished's data_issues bias, but returning a slice. Takes
// mcpLister (not *mcp.Client) so tests can pass fakeMCP; see the NOTE
// near fetchVoiceCorpus on why callAbilityInner can't be shared yet.
func pickColdDraftCandidates(ctx context.Context, c mcpLister, skip map[int]struct{}, max int) ([]productSummary, error) {
	res, err := c.CallTool(ctx, "mcp-adapter-execute-ability", map[string]any{
		"ability_name": "wooagent-products/list",
		"parameters": map[string]any{
			"per_page": 100,
			"orderby":  "date_modified",
			"order":    "asc",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	if len(res.Content) == 0 {
		return nil, fmt.Errorf("list products: empty content")
	}
	var env abilityEnvelope
	if err := json.Unmarshal([]byte(res.Content[0].Text), &env); err != nil {
		return nil, fmt.Errorf("decode list envelope: %w", err)
	}
	if !env.Success {
		return nil, fmt.Errorf("list ability failed: %s", env.Error)
	}
	var out struct {
		Products []productSummary `json:"products"`
	}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		return nil, fmt.Errorf("decode list data: %w", err)
	}

	candidates := make([]productSummary, 0, max)
	for _, p := range out.Products {
		if len(candidates) >= max {
			break
		}
		if p.Status != "publish" && p.Status != "" {
			continue
		}
		if _, inCooldown := skip[p.ID]; inCooldown {
			continue
		}
		if p.DescriptionLength == 0 || p.ShortDescriptionLength == 0 {
			candidates = append(candidates, p)
		}
	}
	return candidates, nil
}

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
	CallTool(ctx context.Context, name string, args any) (mcp.ToolCallResult, error)
}

// Compile-time check that *mcp.Client satisfies mcpLister.
var _ mcpLister = (*mcp.Client)(nil)

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
		"ability_name": "wooagent-products/list",
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
	// NOTE: mirrors callAbilityInner's envelope decode. The two cannot share
	// a helper today because callAbilityInner takes *mcp.Client and we take
	// mcpLister; if a third caller appears, extract a helper that takes
	// mcp.ToolCallResult instead.
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

	// Belt-and-suspenders: also filter status=publish locally. The server
	// is asked to filter via the `status` param, but a misconfigured
	// adapter or older Companion Plugin could return mixed statuses; the
	// local guard makes the corpus shape robust to that. Then apply the
	// per-product exclusion.
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

type product struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	SKU         string `json:"sku"`
	Status      string `json:"status"`
	Description string `json:"description"`
	ShortDesc   string `json:"short_description"`
	Permalink   string `json:"permalink"`
	ImageURL    string `json:"image_url"`
	ImageAlt    string `json:"image_alt"`
}

func getProduct(ctx context.Context, c *mcp.Client, id int) (product, error) {
	var p product
	if err := callAbility(ctx, c, "wooagent-products/get",
		map[string]any{"id": id}, &p); err != nil {
		return p, err
	}
	if p.ID == 0 {
		p.ID = id
	}
	return p, nil
}

// computeDrafting returns which description fields are empty on this
// product — the cold-draft path passes this slice to the LLM.
func computeDrafting(p product) []string {
	out := []string{}
	if strings.TrimSpace(p.ShortDesc) == "" {
		out = append(out, "short")
	}
	if strings.TrimSpace(p.Description) == "" {
		out = append(out, "long")
	}
	return out
}

// ---- LLM ----

// buildPromptUserMessage assembles the per-product user message for the
// marketing draft call. The system prompt (carried in the skill YAML
// description) holds the voice + SEO rubric instructions; this helper
// supplies the dynamic per-product context: the product to rewrite plus
// the corpus samples the LLM compares the variants' voice to.
//
// Empty corpus is supported (new stores, all-thin descriptions). In that
// case the prompt tells the LLM to emit null for the voice field — the
// Go validator clears anything ≤ 0 anyway, so this is belt-and-suspenders.
//
// opts.isColdDraft() switches to a separate preamble that surfaces both
// description fields individually and instructs the LLM to fill only the
// empty ones (declared in opts.Drafting).
func buildPromptUserMessage(p product, corpus []corpusSample, opts draftOpts) string {
	var b strings.Builder
	if opts.isColdDraft() {
		fmt.Fprintf(&b, "Product: %s\nSKU: %s\n", p.Name, p.SKU)
		fmt.Fprintf(&b, "Current short description: %s\n", strings.TrimSpace(p.ShortDesc))
		fmt.Fprintf(&b, "Current long description: %s\n\n", strings.TrimSpace(p.Description))
		fmt.Fprintf(&b, "Drafting fields: %s. Emit body_%s on each variant. Leave Body empty.\n\n",
			strings.Join(opts.Drafting, ", "),
			strings.Join(opts.Drafting, " and body_"))
	} else {
		fmt.Fprintf(&b, "Product: %s\nSKU: %s\nCurrent description: %s\n\n",
			p.Name, p.SKU, strings.TrimSpace(p.Description))
	}
	if len(corpus) == 0 {
		b.WriteString("Voice corpus: (none available — this store has no other long-form published descriptions to compare against. Emit null for `voice` on each variant; score SEO as normal.)\n\n")
	} else {
		b.WriteString("Voice corpus (3–5 of this store's existing published descriptions — use these as the reference for the store's voice; do not copy):\n")
		for i, s := range corpus {
			fmt.Fprintf(&b, "\n[%d] %s\n%s\n", i+1, s.Name, s.Body)
		}
		b.WriteString("\n")
	}
	if opts.isColdDraft() {
		b.WriteString("Draft THREE variants. Return JSON only with the body_short/body_long fields you were asked to fill plus seo and voice scores.")
	} else {
		b.WriteString("Write the THREE rewrite variants per the system instructions. Score each variant 1–100 for `seo` and `voice` using the rubrics in the system prompt. Return JSON only.")
	}
	return b.String()
}

// draftWithFallback prefers Anthropic when AnthropicAPIKey is set, and
// falls back to the OpenAI-compatible endpoint (LM Studio by default)
// otherwise. Returns (rewrite, skipReason, err): a non-empty skipReason
// means the caller should mark the run Skipped. opts is threaded through
// to buildPromptUserMessage to switch between rewrite and cold-draft mode.
func draftWithFallback(ctx context.Context, env personas.Env, p product, skillDescription string, corpus []corpusSample, opts draftOpts) (string, string, error) {
	if strings.TrimSpace(env.AnthropicAPIKey) != "" {
		model := env.AnthropicModel
		if model == "" {
			model = defaultAnthropicModel
		}
		rewrite, err := draftRewriteAnthropic(ctx, env.AnthropicAPIKey, model, p, skillDescription, corpus, opts)
		if err != nil {
			return "", fmt.Sprintf("Claude API errored: %v", err), nil
		}
		if rewrite = strings.TrimSpace(rewrite); rewrite == "" {
			return "", "Claude returned empty rewrite", nil
		}
		return rewrite, "", nil
	}

	base := env.OpenAIAPIBase
	if base == "" {
		base = defaultOpenAIBase
	}
	model := env.OpenAIModel
	if model == "" {
		model = defaultOpenAIModel
	}
	apiKey := env.OpenAIAPIKey
	if apiKey == "" {
		apiKey = defaultOpenAIKey
	}
	rewrite, err := draftRewriteOpenAI(ctx, base, apiKey, model, p, skillDescription, corpus, opts)
	if err != nil {
		return "", fmt.Sprintf("LLM endpoint at %s unreachable or errored: %v", base, err), nil
	}
	if rewrite = strings.TrimSpace(rewrite); rewrite == "" {
		return "", "LLM returned empty rewrite", nil
	}
	return rewrite, "", nil
}

// ---- Anthropic ----

type anthropicMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicReq struct {
	Model     string         `json:"model"`
	MaxTokens int            `json:"max_tokens"`
	System    string         `json:"system"`
	Messages  []anthropicMsg `json:"messages"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type anthropicResp struct {
	Content []anthropicContentBlock `json:"content"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage,omitempty"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func draftRewriteAnthropic(ctx context.Context, apiKey, model string, p product, skillDescription string, corpus []corpusSample, opts draftOpts) (string, error) {
	user := buildPromptUserMessage(p, corpus, opts)

	body, _ := json.Marshal(anthropicReq{
		Model:     model,
		MaxTokens: 2048, // headroom for 3 variants × ~220 chars + JSON overhead
		System:    skillDescription,
		Messages:  []anthropicMsg{{Role: "user", Content: user}},
	})

	cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(cctx, "POST", anthropicAPIURL, bytes.NewReader(body))
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("content-type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic http: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		return "", fmt.Errorf("anthropic http %d: %s", res.StatusCode, raw)
	}
	var parsed anthropicResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("anthropic decode: %w", err)
	}
	if parsed.Error.Type != "" {
		return "", fmt.Errorf("anthropic error: %s · %s", parsed.Error.Type, parsed.Error.Message)
	}

	if t := telemetry.TrackerFromContext(ctx); t != nil {
		t.RecordModelCall(telemetry.ModelCall{
			Provider:     "anthropic",
			Model:        model,
			InputTokens:  parsed.Usage.InputTokens,
			OutputTokens: parsed.Usage.OutputTokens,
		})
	}

	var sb strings.Builder
	for _, b := range parsed.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

// ---- OpenAI-compatible (LM Studio fallback) ----

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatReq struct {
	Model       string    `json:"model"`
	Messages    []chatMsg `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
}

type chatResp struct {
	Choices []struct {
		Message chatMsg `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage,omitempty"`
}

func draftRewriteOpenAI(
	ctx context.Context,
	base, apiKey, model string,
	p product,
	skillDescription string,
	corpus []corpusSample,
	opts draftOpts,
) (string, error) {
	user := buildPromptUserMessage(p, corpus, opts)
	body, _ := json.Marshal(chatReq{
		Model: model,
		Messages: []chatMsg{
			{Role: "system", Content: skillDescription},
			{Role: "user", Content: user},
		},
		Temperature: 0.7,
		MaxTokens:   2048, // headroom for 3 variants × ~220 chars + JSON overhead
	})

	cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(cctx, "POST",
		strings.TrimRight(base, "/")+"/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm http: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		raw, _ := io.ReadAll(res.Body)
		return "", fmt.Errorf("llm http %d: %s", res.StatusCode, raw)
	}
	var parsed chatResp
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("llm decode: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices")
	}
	if t := telemetry.TrackerFromContext(ctx); t != nil {
		t.RecordModelCall(telemetry.ModelCall{
			Provider:     "openai",
			Model:        model,
			InputTokens:  parsed.Usage.PromptTokens,
			OutputTokens: parsed.Usage.CompletionTokens,
		})
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}

// draftColdDraftForProduct does the per-product cold-draft work: fetch
// the product, determine which fields are empty, call the LLM in
// cold_draft mode, parse the structured response, and return a Drafted
// with ProposalType "product_cold_draft" and target.drafting indicating
// which fields the variants will write to. Sibling of draftForProduct;
// the batch wrapper in T5 calls this N times.
func draftColdDraftForProduct(ctx context.Context, deps personas.Deps, candidate productSummary, skillDescription string) (personas.Drafted, error) {
	full, err := getProduct(ctx, deps.MCP, candidate.ID)
	if err != nil {
		return personas.Drafted{}, fmt.Errorf("get product %d: %w", candidate.ID, err)
	}

	drafting := computeDrafting(full)
	if len(drafting) == 0 {
		// Race: by the time we fetched the full product, it had non-empty
		// fields. Skip — the single-rewrite path will handle it later.
		return personas.Drafted{
			Skipped:    true,
			SkipReason: fmt.Sprintf("product %d no longer has empty fields", full.ID),
		}, nil
	}

	corpus, corpusErr := fetchVoiceCorpus(ctx, deps.MCP, full.ID)
	if corpusErr != nil {
		fmt.Printf("marketing(cold_draft): voice corpus fetch errored (%v); proceeding with empty corpus\n", corpusErr)
		corpus = nil
	}

	opts := draftOpts{Mode: "cold_draft", Drafting: drafting}
	rawOutput, skipReason, err := draftWithFallback(ctx, deps.Env, full, skillDescription, corpus, opts)
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
		"product_id":     full.ID,
		"product_name":   full.Name,
		"product_sku":    full.SKU,
		"previous_short": full.ShortDesc,
		"previous_long":  full.Description,
		"image_url":      full.ImageURL,
		"image_alt":      full.ImageAlt,
		"variants":       variants,
		"drafting":       drafting,
	}

	// ProposalContent surfaces in run-log / archive views. Prefer long
	// body, fall back to short, fall back to empty.
	content := variants[0].BodyLong
	if content == "" {
		content = variants[0].BodyShort
	}

	return personas.Drafted{
		Title:           fmt.Sprintf("Product description rewrite · %s", full.Name),
		Description:     fmt.Sprintf("Cold-drafted by Marketing agent for product #%d (%s).", full.ID, full.SKU),
		Priority:        "medium",
		ProposalType:    "product_cold_draft",
		ProposalContent: content,
		Target:          target,
		DedupKey:        fmt.Sprintf("product:%d", full.ID),
	}, nil
}

// draftColdDraftBatch packs successful drafts produced by drafterFn into
// a single batch. drafterFn is a seam for testing; the production call
// site passes draftColdDraftForProduct.
//
// Per-product errors and skips are logged and dropped; if fewer than
// coldDraftMin drafts survive, the function returns Skipped:true so the
// caller falls through to single-rewrite. The first successful draft
// carries BatchSiblings + BatchTitle + BatchIntent; subsequent successes
// become siblings.
func draftColdDraftBatch(ctx context.Context, deps personas.Deps, candidates []productSummary, skillDescription string, drafterFn func(context.Context, personas.Deps, productSummary, string) (personas.Drafted, error)) (personas.Drafted, error) {
	drafts := make([]personas.Drafted, 0, len(candidates))
	for _, p := range candidates {
		d, err := drafterFn(ctx, deps, p, skillDescription)
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
