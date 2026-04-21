# WooAgent OS — GEPA-Powered Prompt & Skill Improvement Plan

## Context

WooAgent OS is greenfield (Phase 1 build starts 2026-05-22). Its core UX is a **propose-approve** loop: three personas (Marketing, Pricing, Sales Support) emit proposals; the operator approves, rejects, or edits each one. That verdict stream is a labelled dataset nobody is exploiting yet — Phase 4 in the current plan is "manually tune prompts by eyeballing outputs."

This plan replaces that manual loop with a data-driven one built on **GEPA** (Genetic-Pareto Evolutionary Prompt Adaptation, arXiv 2507.19457, https://github.com/gepa-ai/gepa). GEPA uses an LLM to read execution traces, reflect in natural language on what went wrong, and mutate prompts — maintaining a Pareto frontier of candidates across multiple objectives. It reaches better-than-RL quality with ~35× fewer evaluations, and it ships a pluggable `GEPAAdapter` interface plus a Google ADK integration.

**Scope decision (confirmed with user):**
- **v2 add-on.** v1 (shipping 2026-05-22) is untouched in feature set, but gains the minimum scaffolding — a prompt registry, structured telemetry, redaction + opt-in export — so v2 can plug in without painful migrations.
- **Offline batch** optimization, not online/continuous.
- **Federated export** across opted-in daemons → a central GEPA pipeline → signed prompt updates pushed back to daemons. The Go daemon never runs GEPA itself.

Intended outcome: within ~3 months of v1 launch, prompt updates for each persona are produced by a reproducible pipeline, reviewed by the team, and rolled out via signed manifests. "Which persona are we bad at?" becomes a dashboard, not a vibe check.

---

## Architecture at a glance

```
   ┌────────── user laptop (per daemon) ──────────┐        ┌──── central services (Automattic-hosted) ────┐
   │                                              │        │                                              │
   │   wooagent daemon                            │        │   ingest API  →  data lake  →  GEPA runner   │
   │   ├─ prompt registry (versioned YAML)        │        │                                     │        │
   │   ├─ skill registry                          │        │                                     ▼        │
   │   ├─ turn telemetry → local SQLite           │        │                              Pareto candidates│
   │   ├─ redactor                                │        │                                     │        │
   │   └─ export + upload (opt-in, signed)   ─────┼────────┼──►   review UI (diffs, samples)  ◄──┘        │
   │                                              │        │                │                             │
   │   prompt update client  ◄───────── signed manifest ◄──┼────────────────┘                             │
   │                                              │        │                                              │
   └──────────────────────────────────────────────┘        └──────────────────────────────────────────────┘
```

Two surfaces: (1) **daemon-side telemetry + registry** (ships in v1), (2) **central GEPA pipeline + review + distribution** (v2).

---

## The metrics set

The core deliverable the user asked for. These are what GEPA's metric function returns per candidate; Pareto selection picks the non-dominated set across them.

### Outcome (reward) metrics — the primary signal

| # | Metric | Type | Source |
|---|--------|------|--------|
| 1 | **Approval rate** — % of proposals approved without edits | ratio | verdict events |
| 2 | **Edit-distance-on-approval** — token-level diff between proposal and published text, lower is better | normalized [0,1] | approve-with-edit events |
| 3 | **Rejection rate by tagged reason** — {off-brand, wrong-numbers, irrelevant, unsafe, incomplete, other} | multi-class distribution | reject events + tag picker in UI |
| 4 | **Time-to-decision** — median seconds from proposal to verdict (proxy for "obviously right") | seconds | timestamps |
| 5 | **Recovery rate** — after a reject, did the *next* proposal for the same issue get approved? | ratio | issue state machine |

### Skill / tool-use metrics

| # | Metric | Type | Source |
|---|--------|------|--------|
| 6 | **Skill-selection accuracy** — did the agent pick the right MCP ability / local skill for the intent? | ratio (labelled or LLM-judge) | trace + labels |
| 7 | **Skill-argument validity** — did the MCP / ability call succeed vs. 4xx/5xx | ratio | skill response status |
| 8 | **Skill success rate per (persona, skill) pair** | ratio | aggregated from #7 |
| 9 | **Grounding / hallucination flag** — references to non-existent products, customers, or figures | boolean per proposal | automated check against store data, later cross-checked with operator rejection reason |

### Efficiency metrics (secondary Pareto axes)

| # | Metric | Type | Source |
|---|--------|------|--------|
| 10 | **Cost per proposal** — input + output tokens × model rate | USD | model provider |
| 11 | **Latency per turn** | seconds | spans |
| 12 | **Turns-to-done** — how many back-and-forths before an issue closed | integer | issue history |

### Consistency metrics (LLM-as-judge, offline)

| # | Metric | Type | Source |
|---|--------|------|--------|
| 13 | **Brand-voice score** — LLM-as-judge against a persona reference style guide | 1–5 | offline evaluator |
| 14 | **Numeric sanity** (Pricing only) — proposed margin within configured bounds; PnL math passes | boolean | rule-based check |

**Why this set.** 1–5 are the cheap ground truth already available in the approve/reject flow. 6–9 catch failures that *look* fine but misuse tools. 10–12 keep GEPA from producing 3000-token rambles that win on quality but blow out costs — Pareto frontier forces the trade-off to be explicit. 13–14 are cheap offline checks that don't need operator attention.

**Starter set for first GEPA run:** metrics 1, 2, 3, 7, 10 — they're collectable on day one with no extra labelling. Metrics 6, 9, 13 require a small labelled set or judge prompts; land them in the second iteration.

---

## v1 scaffolding — what ships by 2026-05-22

Four concrete additions to Elizabeth's current Phase 1–3 plan. None change the v1 feature surface; all are enablers.

### 1. Prompt & skill registry (not Go string literals)

- Location: `internal/registry/` + on-disk `prompts/` and `skills/` directories loaded at daemon start.
- Each prompt file: YAML with frontmatter `name`, `persona`, `version` (semver), `content_sha`, `supersedes`, body in Markdown. Example path: `prompts/marketing/v0.1.0.yaml`.
- Runtime resolves persona → active prompt version via a `manifest.yaml` that can be hot-swapped.
- Skills similarly: `skills/pricing.margin.compute/v0.1.0.yaml` with schema, description, examples — these are the textual artefacts GEPA will optimize alongside prompts.
- Rationale: **GEPA mutates text**. If prompts live in compiled Go, every optimization iteration is a rebuild. A registry makes prompt updates a file drop + daemon SIGHUP.

### 2. Structured turn telemetry

Emit one `TurnEvent` per agent turn with this schema (SQLite + JSON blob for nested fields):

```
turn_id (uuid), issue_id, persona, prompt_version, skill_versions[],
started_at, completed_at, latency_ms,
context: { issue_summary, store_snapshot_hash, operator_intent },
model_calls[]: { provider, model, input_tokens, output_tokens, cost_usd, request_sha, response_sha },
skill_calls[]: { name, version, args_sha, status, latency_ms, error_code },
proposal_text, proposal_sha,
verdict: { kind: approve|approve_with_edits|reject, reason_tag, reason_text, published_text, published_sha, decided_at }
```

Full raw request/response bodies land in a separate blob table keyed by `*_sha` (content-addressed, so dedup across turns is free). Schema versioned with `event_schema_version`.

Critical: **nail this schema in Phase 1.** It's the dataset row. Changing it after six months of operator data means painful migrations.

### 3. Redaction + opt-in consent

- `internal/redact/` runs over every `TurnEvent` before export. Default ruleset strips emails, phone numbers, customer names, product SKUs, URLs, UUIDs. Store operators edit the ruleset in settings.
- Opt-in toggle in the React UI onboarding + settings: "Share redacted proposal data to improve WooAgent." Off by default. Per-persona opt-in granularity (an operator might share Marketing but not Pricing).
- Every export bundle ships with a manifest listing the redaction ruleset hash that was applied, so the central pipeline can filter by compliance tier.

### 4. Export command + signed prompt update client

- `wooagent export --since <ts> --out <path>` produces a `.jsonl.zst` bundle of redacted `TurnEvent`s + a manifest. Cron-friendly.
- Upload: `wooagent sync` POSTs to a configurable endpoint with operator's opt-in token.
- Inverse direction: `internal/updates/` polls `https://updates.wooagent.<host>/manifest.json` (signed with an offline key) for new prompt versions. Operator gets a UI banner — "Marketing prompt v0.2.1 available (diff preview) [Accept] [Decline]". Never auto-applied.

---

## v2 — the central GEPA pipeline (post-v1)

### Ingest & dataset

- Thin HTTP service receives opted-in bundles, validates schema + signatures, writes to a data lake (parquet, partitioned by `persona` × `week`).
- PII scanning pass on ingest as a safety net over daemon-side redaction.

### GEPA adapter

Implement `GEPAAdapter` (Python, upstream repo's interface):

- `evaluate(candidate, batch)` — for each held-out `TurnEvent`, replay the proposal context against the candidate prompt using the same model + tool offering captured in the trace. Score with the metric set above. Returns per-example metric vectors.
- `make_reflective_dataset(candidate, batch, feedback)` — formats (proposal, verdict, reason_tag, reason_text, operator_edits, skill_errors) tuples as a natural-language reflection corpus, grouped by failure mode.

Two models:
- **Task LM**: the same model the daemon will run in production (e.g. Claude Sonnet 4.6). Keeps evaluation distribution-matched.
- **Reflection LM**: a stronger model, probably Claude Opus 4.7, for diagnosis + mutation. GEPA's paper shows this asymmetry works well.

### Cheap evaluator for large candidate pools

GEPA explores many candidates. We can't re-ask operators per candidate, and running the real task LM on every held-out context × every candidate is expensive. Train a lightweight **approval classifier** from historical `(proposal_sha → verdict)` pairs and use it as a cheap screen; reserve full LM evaluation for the top-k candidates on the Pareto front. Paper-and-pencil approach; refine if signal is noisy.

### Review & publish workflow

- Every GEPA run produces a Pareto set of candidate prompts per persona + a reflection report (which failure modes improved, which regressed, sample proposals before/after).
- Internal review UI: reviewer (Nevena for voice, Elizabeth for engineering, a pricing SME for numbers) sees the diff + 20 random before/after sample proposals drawn from the held-out set.
- On approval: prompt is signed with the offline key, published to the update manifest, versioned in Git.

### Joint prompt + skill optimization

GEPA natively handles multi-component systems — pass a `seed_candidate` dict with keys like `{"persona.marketing.system_prompt": "...", "skill.pricing.margin.compute.description": "..."}`. The adapter replays against the full agent graph. Start with prompts only in the first cycle, fold skills in once the telemetry shows which skill descriptions drive selection errors (metric #6).

---

## Files / paths (v1 scaffolding)

Elizabeth doesn't need to build anything new for v2 during v1 — she just needs these in the May-22 daemon:

- `internal/registry/prompts.go`, `internal/registry/skills.go` — load + version resolution
- `prompts/<persona>/v*.yaml`, `skills/<name>/v*.yaml`, `prompts/manifest.yaml`
- `internal/telemetry/turn_event.go` — the schema above; **review carefully, this is load-bearing**
- `internal/telemetry/store.go` — SQLite persistence
- `internal/redact/` — default rules + config
- `internal/export/` — bundle + upload commands
- `internal/updates/` — signed manifest poller
- UI: settings page with per-persona opt-in toggle + redaction rule editor; banner component for pending prompt updates

---

## Verification

**v1 scaffolding (by 2026-05-22 demo):**
- Run 10 operator-driven turns through Marketing, approve/reject mix.
- Inspect SQLite: all 10 turns present with complete `TurnEvent` fields, no null `verdict` on closed issues.
- Toggle opt-in on for Marketing only. Run `wooagent export` → inspect bundle: Marketing turns present, Pricing/Sales Support absent, emails/SKUs redacted per ruleset.
- Drop a new `prompts/marketing/v0.1.1.yaml` + update manifest → daemon picks up new version on SIGHUP without restart; UI shows the diff.

**v2 GEPA pipeline (first real run, target ~3 months post-v1):**
- Accumulate ≥500 verdicted turns per persona across opted-in daemons.
- Run GEPA with `max_metric_calls=200`, starter metric set (1, 2, 3, 7, 10), seed = current prompt.
- Expect: Pareto front of 3–8 candidates, reflection report naming at least one concrete failure mode per persona.
- Reviewer approves one candidate per persona → signed → pushed to daemon manifest → operator sees banner → accepts → next day's approval rate on held-out turns moves in the right direction.
- Success criterion for the first cycle: approval rate up on at least 2 of 3 personas on a held-out week, with cost/proposal not regressing by >15%.

---

## Open questions to resolve during v1 (not blocking this plan)

- **Skill description storage.** Are skill descriptions authoritative in the MCP server (for WP abilities) or in the daemon registry? GEPA can only optimize what lives on our side — needs a local override layer for MCP abilities.
- **Approval classifier** — built on what? Simple logistic regression over embeddings of proposal text + persona + context, trained once per month, is a reasonable starting point. Revisit after first 500 verdicts.
- **Central infra** — where does the ingest service live? A8c-internal service vs. a small hosted stack is a separate decision; the daemon-side design doesn't care which.

---

## One note on file location

Per your saved preference, team-shareable plans belong in the project folder. This plan was written to `/Users/elizabethpizzuti/.claude/plans/merry-forging-horizon.md` because plan mode required it, but once you approve I can copy it into `/Users/elizabethpizzuti/claude/wooagent/` (suggested name: `wooagent-os-gepa-v2-plan.md`) so it lives alongside the PRD and the build plan.
