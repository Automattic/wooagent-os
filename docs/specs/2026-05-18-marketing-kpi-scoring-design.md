# Marketing detail KPIs: real SEO + brand-voice scoring

**Linear:** [DSGWOO-1326](https://linear.app/a8c/issue/DSGWOO-1326/wire-real-seo-brand-voice-scoring-for-marketing-detail-kpis)
**Status:** Design — ready for implementation
**Author:** Elizabeth Pizzuti
**Date:** 2026-05-18

## Problem

The Marketing detail page (`IssueDetail.tsx` prose path) renders four KPI tiles: Scope, Brand voice match, SEO score, Est. impact. For every live agent run, **Brand voice match** and **SEO score** show `0%` / `0`.

Root cause is three things stacked:

1. The `marketing.description-rewrite` skill's LLM contract emits only `{label, angle, body}` per variant. There is no `seo` or `voice` field.
2. `variantsFromProposal` in `ui/src/api/client.ts:326-327` defaults both scores to `0` when absent.
3. There is no scorer in the daemon, no brand-voice profile anywhere, and no Yoast-equivalent SEO heuristic. The placeholder comment at `daemon/internal/personas/marketing/marketing.go:39` ("seo/voice default to 0 in the UI until real scoring lands") has been load-bearing for the gap.

The only way the tiles read non-zero today is via two seed scripts (`daemon/scripts/seed-demo-merino-turtleneck.sh`, `seed-demo-batch-meta-rewrites.sh`) that hardcode the scores for showcase data.

This is launch-blocking per the public-launch posture: the tiles look like real signal but aren't.

## Goals

- Each marketing variant on a live run carries a real `seo` integer (0–100) and a real `voice` integer (0–100).
- The scorer for both is real — no further hardcoded showcase logic. The product-copy rubric is explicit and testable; the voice corpus is sourced from the store's own existing copy.
- When scoring fails (LLM omits a score, parses invalid, or returns `0`), the tile renders an em-dash plus a "not yet scored" hint instead of `0%` / `0`. Misleading zero is the bug we are explicitly removing.
- One end-to-end design — no follow-up tickets required for the scorer itself.

## Non-goals

- Re-scoring on approve or edit. Scores are snapshotted on the variant at draft time and persist with the proposal target forever, the same way variant bodies do.
- A persisted brand-voice profile or settings UI. The corpus is sampled live per run from existing product descriptions — zero operator setup.
- Cross-store voice transfer. The corpus is per-store.
- Migrating existing seeded proposal data. The two seed scripts already emit `seo` and `voice` in the persisted shape; they continue to work unchanged.

## Architecture (one round-trip, inline scoring)

```
Marketing agent run (existing)                Marketing agent run (new)
──────────────────────────                    ──────────────────────────

  pick product (MCP list)                       pick product (MCP list)
  fetch product (MCP get)                       fetch product (MCP get)
                                                fetch voice corpus (MCP list)  ← NEW
                                                  └─ top 3–5 longest published
                                                     descriptions, ≠ this product
  build prompt                                  build prompt
    └─ skill desc + product             →         └─ skill desc + product
                                                     + corpus samples
                                                     + SEO rubric (6–7 checks)
                                                     + scoring instructions
  call Claude                                   call Claude
    └─ JSON {variants: [{label,                   └─ JSON {variants: [{label,
       angle, body}]}                  →             angle, body, seo, voice}]}

  parseVariants → variants[]                    parseVariants → variants[]
    (seo/voice absent, defaults 0)                + score validation (0–100, ints)
                                                  + telemetry on missing/invalid

  persist proposal                              persist proposal
  UI reads target.variants                      UI reads target.variants
                                                  (Variant.seo / .voice optional →
                                                   undefined drives '—' rendering)
```

**Key shape changes:**

- `llmVariant` (Go) gains `Seo int` + `Voice int`. The LLM emits both.
- `variant` (Go, persisted into `proposal.target.variants[]`) gains the same two fields.
- `Variant` (TS, `client.ts`) changes `seo: number; voice: number;` → `seo?: number; voice?: number;` so the UI can distinguish "absent" (render `—`) from "real zero" (render `0` — though the validator below treats real zero as suspect and omits the field too).
- `Kpi` component (`ui/src/components/Kpi.tsx`) gains a missing-value branch: when `value` is `undefined`/`null`, render `—`, suppress the score bar, and let the caller override the hint string.

**No new daemon storage. No new MCP abilities. No proposal-schema versioning.** The voice corpus is fetched live from the existing `wc/products` list call. The Marketing skill prompt template gains two sections (voice corpus, SEO rubric) and a wider output schema.

### Cost / latency impact

- One extra MCP `list_products` call per `draftForProduct` attempt. Bounded by `maxDraftAttempts=3`, so worst-case 3 list calls per run. Roughly ~1s each on staging.
- Prompt grows by ~3–5 product descriptions (few hundred chars each) plus ~150 words of rubric = ~1–3k extra input tokens per run.
- Generation time roughly the same; the LLM has to compute scores but stays in one round-trip.
- Net: marketing run goes from ~5–15s → ~7–20s.

## Components

### Go daemon

**`daemon/internal/personas/marketing/marketing.go`**

- `variant` struct gains `Seo int json:"seo,omitempty"` and `Voice int json:"voice,omitempty"`. `omitempty` lets the validator omit the field entirely when scoring fails — UI distinguishes `undefined` from real zero.
- `llmVariant` struct gains the same two fields.
- `parseVariants` validates `0 ≤ seo ≤ 100` and `0 ≤ voice ≤ 100`, both integer. Out-of-range, missing, non-integer, or exactly `0` → omit the field on the persisted variant and increment a telemetry counter `marketing.score_missing{kind="seo|voice"}`. Variant body, label, angle remain valid; only the missing dimension goes empty.
- The stale comment at `marketing.go:39` ("seo/voice default to 0 in the UI until real scoring lands") is removed.
- New helper `fetchVoiceCorpus(ctx context.Context, c *mcp.Client, excludeProductID int) ([]corpusSample, error)`. Calls `wc/products?per_page=20&orderby=date_modified&order=desc&status=publish`, filters out `excludeProductID`, sorts by description length descending, returns the top 5 as `[]corpusSample{Name, Body}`. ~25 lines. Failure returns an empty corpus and a non-nil error logged at WARN — the caller proceeds with an empty corpus rather than skipping the run.
- `draftForProduct` calls `fetchVoiceCorpus(ctx, deps.MCP, productID)` after `getProduct`, then `buildPrompt(p, corpus, skill.Description)` (new) instead of inlining the prompt assembly. Corpus is fetched per product (not once per run) so within-run retries get a clean exclusion — total worst case is 3 MCP list calls per `Marketing.Draft` (bounded by `maxDraftAttempts`), well within latency budget.

**Marketing skill prompt** (lives in the abilities manifest at `daemon/internal/manifest/default.json`'s `marketing.description-rewrite.description`, with overrides loaded from `prompts/` if present — implementation should match the existing pattern)

Adds two sections to the existing instruction block:

- **Voice corpus**: "Here are 3–5 existing product descriptions from this store. Use them as the reference for the store's voice. Each variant's `voice` score is its match against this corpus on a 0–100 scale — phrasing, register, sentence rhythm, lexical choices. Reward natural alignment; do not reward copying."
- **SEO rubric** (6 checks, each contributes ~16 points, LLM rounds the total):
  - Focus keyphrase = the product name; appears in the body naturally
  - Keyphrase density in the 0.5–3% band (no stuffing, no absence)
  - Length adequacy: 80–300 words
  - Benefit-led opener: first sentence leads with what the product does for the customer, not the product's name
  - Scannability: short sentences (median ≤ 20 words), short paragraphs (≤ 3 sentences)
  - Specificity: at least 2 concrete features, materials, or measurements

Output schema in the prompt updates from `{variants: [{label, angle, body}]}` to `{variants: [{label, angle, body, seo, voice}]}`. When the corpus is empty (new store, all thin descriptions), the prompt instructs the LLM to emit `null` for `voice` (not 0) — the Go validator treats `null` as "field absent" and omits it from the persisted variant.

### UI

**`ui/src/api/client.ts`**

- `Variant.seo` and `Variant.voice` become `seo?: number` / `voice?: number`.
- `variantsFromProposal` parser changes: `seo: typeof r.seo === 'number' ? r.seo : undefined` (was `: 0`). Same for `voice`. Absent on the wire → absent in TS.

**`ui/src/components/Kpi.tsx`**

- When the caller passes `value={undefined}` or `value={null}` directly, the value slot renders an em-dash (`—`) using the existing `value` slot's text styling, the score bar is suppressed, and the hint string is whatever the caller passes (typically "not yet scored"). ~5 lines added inside the value-rendering branch. No prop-type change — `value: ReactNode` already accepts `undefined`.

**`ui/src/screens/IssueDetail.tsx`**

The Brand voice match and SEO score tiles in the Marketing detail KPI row (around lines 354–367) change from always-passing-a-number to conditionally passing `undefined`:

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

Same shape for SEO score; the live-score hint changes from `Yoast · out of 100` to `Product-copy rubric · out of 100`.

### Seed scripts

The two seed scripts (`daemon/scripts/seed-demo-merino-turtleneck.sh`, `seed-demo-batch-meta-rewrites.sh`) already emit hardcoded `seo` and `voice` integers. They continue to work without change. Sweep the hint copy update (`Yoast · out of 100` → `Product-copy rubric · out of 100`) wherever the scripts mention it.

## Data flow

```
                ┌─────────────────────────────────────────────────────────┐
                │  Marketing.Draft → draftForProduct(ctx, deps, productID)│
                └─────────────────────────────────────────────────────────┘
                                          │
            ┌─────────────────────────────┼─────────────────────────────┐
            │                             │                             │
            ▼                             ▼                             ▼
   getProduct(MCP)             fetchVoiceCorpus(MCP) [NEW]      skill.Description
   ────────────────            ─────────────────────             ────────────────
   p {ID, Name, SKU,           wc/products?per_page=20&         existing instruction
      Description,             orderby=date_modified&            text from the
      ImageURL, ImageAlt}      order=desc&status=publish         abilities manifest
                                       │
                                       ▼
                               filter: status=publish
                                       │
                                       ▼
                               filter: ID ≠ p.ID
                                       │
                                       ▼
                               sort: len(description) desc
                                       │
                                       ▼
                               take top 5 → []corpusSample{Name, Body}

                                          │
                                          ▼
                         ┌───────────────────────────────┐
                         │  buildPrompt(p, corpus, skill)│
                         └───────────────────────────────┘
                                          │
                            prompt = [
                              skill.Description,
                              "Product to rewrite: …",   ← p
                              "Voice corpus:\n…",        ← corpus
                              "SEO rubric:\n• …",        ← 6 checks
                              "Emit JSON {variants:[{
                                label, angle, body,
                                seo, voice}]}"
                            ]
                                          │
                                          ▼
                              callClaude(prompt) → rawOutput
                                          │
                                          ▼
                              parseVariants(rawOutput) → []variant
                                          │
                                          ▼
                       per variant: validate scores
                         • integer, 0–100 (and ≠ 0) → keep
                         • out of range / missing / null → omit field
                         • record telemetry counter:
                             marketing.score_missing{kind="seo|voice"}
                                          │
                                          ▼
                              target = { product_id, product_name,
                                         product_sku, previous,
                                         image_url, image_alt,
                                         variants: [{…, seo?, voice?}] }
                                          │
                                          ▼
                              personas.Drafted{ ProposalContent: variants[0].Body,
                                                ProposalType: product_description_rewrite,
                                                Target: target, … }
                                          │
                                          ▼
                                     store insert
                                          │
                                          ▼
                              GET /v1/issues/:id   (UI loads)
                                          │
                                          ▼
                         variantsFromProposal(p)
                           • seo: typeof r.seo === 'number' ? r.seo : undefined
                           • voice: typeof r.voice === 'number' ? r.voice : undefined
                                          │
                                          ▼
                              activeVariant in IssueDetail
                                          │
                                          ▼
                         Kpi tiles for Brand voice + SEO
                           • number  → `${n}%` + score bar + tone band + real hint
                           • undefined → `—`   + no bar       + 'neutral' tone + 'Not yet scored'
```

### Side notes on the data flow

- **Voice corpus exclusion**: always exclude the product currently being rewritten so the LLM isn't grading against the thing it's about to replace.
- **Corpus shortfall**: <3 long-enough published descriptions → fall back to whatever's available. Empty corpus → prompt still works (rubric still applies); LLM is instructed to emit `null` for `voice`.
- **Within-run iteration**: existing `IterateDraft` retry loop (up to 3 products per run) re-fetches the corpus per attempt. Acceptable cost (~1s × ≤3) for the simpler signature; corpus exclusion naturally tracks the per-attempt product.
- **Persisted shape stays append-only**: existing seeded data continues to read correctly. New runs produce the same JSON shape with the same field names.
- **No cross-run cache**: `fetchVoiceCorpus` runs once per `Marketing.Draft` invocation. Each scheduler tick re-fetches — corpus stays fresh as the operator approves new descriptions.

## Error handling

| Failure | Where | Behavior |
|---|---|---|
| MCP `list_products` errors during corpus fetch | `fetchVoiceCorpus` | Log at WARN, return empty corpus. Prompt assembles with an empty corpus section; LLM is instructed to emit `null` for voice. Run continues. |
| Corpus is empty (new store, all thin descriptions) | `fetchVoiceCorpus` returns `[]` | Same as above — prompt instructs LLM to emit `null` for voice. SEO continues. |
| LLM returns malformed JSON | `parseVariants` | Existing fallback path: single-variant with raw text as content. Both scores absent → UI renders `—` on both tiles. No regression vs. today. |
| LLM emits `seo` / `voice` as non-integer, out of range, or missing | per-variant validator | Field omitted from persisted variant. Telemetry counter `marketing.score_missing{kind=...}` incremented. UI renders `—` for the missing dimension only. |
| LLM emits both scores as exactly `0` | per-variant validator | Treated as suspicious — same path as out-of-range. Rationale: `0` was the legacy default that prompted this ticket. The product-copy rubric should never legitimately score `0` for a non-empty body. |
| Approved variant's score retroactively changes (re-run) | n/a | Scores are snapshotted on the variant at draft time. We do not re-score. Consistent with how variant bodies work today. |

**Telemetry**: one new counter `marketing.score_missing` with a `kind` label. Surfaces in the existing telemetry pipe alongside `skill_call` metrics. Not user-visible; lets us detect prompt drift after deployment.

## Score bands

Kept from today with one adjustment:

- **SEO**: `≥80` success / `≥70` caution / `<70` warning — unchanged. Product-copy rubric with 6 checks lines up reasonably: 5 of 6 checks clear ≈ 83.
- **Brand voice**: today `≥90` brand / `≥75` caution / `<75` warning. The 90% bar is unrealistic for corpus-based matching against a small sample. Drops to **`≥80` brand / `≥65` caution / `<65` warning**.

Banding thresholds live in two ~3-line functions (`seoToneBand` / `voiceToneBand` in `IssueDetail.tsx:53-62`). Adjustable in a follow-up if real distribution shifts them.

## Hint copy

| Tile | Before | After |
|---|---|---|
| SEO score (live) | `Yoast · out of 100` | `Product-copy rubric · out of 100` |
| SEO score (no score) | n/a | `Not yet scored` |
| Brand voice match (live) | `vs. your voice model` | `vs. your existing copy` |
| Brand voice match (no score) | n/a | `Not yet scored` |

Sentence case throughout (per DESIGN.md).

## Testing

### Go (`marketing_test.go`)

- **Extend `TestParseVariants`** with new cases: valid `seo` and `voice` integers, out-of-range (101, -1), missing fields, non-integer (`"95"` as string), both-zero special case.
- **New `TestFetchVoiceCorpus`** with a mocked `mcp.Client` returning fake `list_products` output. Asserts: filters `status=publish`, excludes `excludeProductID`, sorts by description length descending, returns top 5. ~50 lines.
- **New `TestBuildPrompt`** — golden test. Feeds a known product + known corpus through the prompt builder; asserts the substring "Voice corpus:" appears, the SEO rubric bullets appear, and the output schema in the prompt includes both `seo` and `voice`. Catches prompt-template regressions without coupling to exact wording.

### UI

- `tsc --noEmit` is the type gate for the optional-field propagation.
- Manual verify in `npm run dev`: navigate to a marketing detail with seeded scores (existing behavior preserved) + one with the new live shape (em-dash path).
- No new Jest suites — the `Kpi` missing-value branch is shallow and visual.

### Manual end-to-end smoke

After landing, one live agent run against the staging store with `ANTHROPIC_API_KEY` set:

- Confirm both scores appear as real percentages on at least one variant.
- Confirm hint reads "Product-copy rubric · out of 100" + "vs. your existing copy".
- Manually trigger the LLM-omits-scores branch (temporarily tweak the prompt to drop `seo` from the schema, restart daemon, run once, revert) — confirm em-dash + "Not yet scored" rendering.

## Rollout

Single PR. No feature flag — the change is additive (Variant.seo/voice optional), the UI renders the safe `—` state when scores are absent, and the prompt change is backwards-compatible with the persisted shape that seed scripts already use.

After merge:

1. Daemon redeploy on staging.
2. Manual smoke per above.
3. Watch `marketing.score_missing` counter for a week. If it ticks materially above zero, the prompt or validator needs tuning.

## Open questions

None at design time. Follow-ups likely after the first week of real data:

- Score banding may need adjustment if the real distribution clusters differently from estimates.
- The 6-check SEO rubric may need to grow or shrink as we see what the LLM actually scores on.
- Voice corpus sampling (top 5 longest published descriptions) may need refinement if "longest" turns out not to correlate with "most on-brand" — operator-curated picks remain a future option.

## References

- [DSGWOO-1326](https://linear.app/a8c/issue/DSGWOO-1326/wire-real-seo-brand-voice-scoring-for-marketing-detail-kpis) — Linear ticket
- `daemon/internal/personas/marketing/marketing.go:39` — placeholder note (to be removed)
- `daemon/scripts/seed-demo-merino-turtleneck.sh` — seeded-score example
- `ui/src/api/client.ts:313-336` — `variantsFromProposal`
- `ui/src/components/Kpi.tsx` — KPI tile component with tone bands
- `ui/src/screens/IssueDetail.tsx:53-62` — score band thresholds; lines 354–367 — KPI row
