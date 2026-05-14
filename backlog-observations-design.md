# Backlog Observations — Design

Give the Kanban board a populated Backlog and a meaningful Drafting column by letting each persona produce a small ranked list of observations at bootstrap, auto-promoting the top one through Drafting to In Review, and parking the rest in Backlog as cards the operator can promote manually.

**Status:** design, ready for implementation planning. Scoped for v0.1.0 internal testing call (2 weeks out from 2026-05-14).

## Goal

Today, each persona produces exactly one proposal on first daemon start, then the scheduler skips subsequent runs with `persona already has open issues`. The Kanban board ends up with three cards — all in `In Review` — and `Backlog` / `Drafting` permanently empty. That misframes what the agents are doing:

- **Backlog** is labeled `Queued · agent will draft` but has never been populated by anything.
- **Drafting** is labeled `Drafting now` but Draft is synchronous, so no card ever lives in it.
- Internal testers landing on this board read it as "the agent did one thing" rather than "the agent has a queue of things it's noticed."

The fix: make the agent's first pass a **survey** that produces multiple observations, persist them as `status='backlog'` issues, and have the scheduler auto-promote the top-scored one through `in_progress` (Drafting) to `in_review`. Subsequent ticks promote the next-best observation when Review is empty; re-survey when both columns are empty.

## What we already have

- **`issues` table with a `backlog` status** in the migration schema (visible at `daemon/internal/personas/personas.go:170`). Currently unused — `Kanban.tsx:27-35` already routes `status='backlog'` to the Backlog column.
- **Scheduler with worker + retry classification** (`daemon/internal/scheduler/`) — the dispatch already routes runs to `personas.RunAndPersist`. We add new triggers (`promotion`) without changing the worker.
- **`turn_events` telemetry** — every LLM call already records into `turn_events`. Survey calls will write here the same way Draft calls do.
- **Kanban polling loop** in the UI — the kanban already refreshes via the API client; we tune the poll interval based on whether any issue is `in_progress`.

## Scope for v0.1.0

**In:**

- Additive `issues` schema (4 new columns + 1 index, no rewrites of existing rows).
- New `Persona.Survey` method on the persona interface; existing `Draft` parameterized by an explicit `DraftTarget` (today it's implicit).
- `RunAndPersist(ctx, p, deps, mode)` becomes a small dispatcher with two modes: `survey` and `draft`.
- `HasOpenWork` narrows to `in_progress | in_review` (excludes `backlog`).
- New scheduler trigger `promotion`; scheduler loop enqueues promotion runs when Review is empty AND backlog has items.
- Backlog observation card UI (lighter shape, "Draft this" CTA).
- Live Drafting column via poll-interval bump when any issue is `in_progress`.
- Persona-name badges on all card types (per finding F5 from the internal-testing smoke).
- Failure handling: backlog re-entry on draft fail, 3-strike auto-dismiss, skip-reason surfacing on survey failure.

**Out (filed as follow-ups, mostly tied to the queued insights-planning plan):**

- Cross-persona observation scoring / a "Planner" that consumes them. (Insights-planning workstream.)
- Observation staleness or re-validation. Backlog cards don't expire; operator dismisses manually.
- Top-up-when-low logic. We re-survey only when both Review AND Backlog are empty.
- The `insights` table itself. Observations live in `issues` with new columns; when insights-planning's migration 012 lands, backlog issues graduate to insights rows. The columns we add here are the migration path, not throwaway state.
- SSE / WebSocket for live updates. Poll-interval bump is sufficient for the visible delay.

## State machine

```
                       ┌─ operator dismisses ─▶ rejected
                       │
(survey) ──▶ backlog ──┼─ promotion (scheduler or operator) ──▶ in_progress (Drafting now)
                       │                                                  │
                       │                                                  ├─ draft succeeds ─▶ in_review ──▶ done | rejected
                       │                                                  │
                       └─ draft fails (attempts < 3) ◀───────────────────┘
                                                                          │
                                                                          └─ draft fails (attempts == 3) ──▶ rejected (auto)
```

Existing transitions (`in_review → done | rejected`) are unchanged.

## Schema

```sql
-- Migration: 013_backlog_observations.sql
ALTER TABLE issues ADD COLUMN observation_rationale TEXT;     -- 1–2 sentences "why this matters"
ALTER TABLE issues ADD COLUMN observation_score      REAL;    -- 0..1 ranking, used for auto-promote order
ALTER TABLE issues ADD COLUMN target_id              TEXT;    -- product_id / order_id; dedupe key for re-survey
ALTER TABLE issues ADD COLUMN draft_attempts         INTEGER NOT NULL DEFAULT 0;

CREATE INDEX idx_issues_persona_status_score
    ON issues(persona, status, observation_score DESC);
```

Backfill: existing `in_review` rows get `target_id=NULL`, `observation_score=NULL`. The dedupe logic in Survey treats `target_id IS NULL` as "no dedupe key, never matches" — i.e., legacy rows don't interfere with new surveys.

`HasOpenWork` (`daemon/internal/personas/personas.go:167`) changes:

```sql
-- before:
SELECT count(*) FROM issues WHERE persona = ? AND status IN ('todo','in_progress','in_review')
-- after:
SELECT count(*) FROM issues WHERE persona = ? AND status IN ('in_progress','in_review')
```

`backlog` items don't block surveys (they're the survey's output). `todo` is currently unused in the persona flow — kept out of the count so it doesn't accidentally block anything.

## Persona interface

```go
// daemon/internal/personas/types.go

type Persona interface {
    Slug() string
    Survey(ctx context.Context, deps Deps) (SurveyResult, error)
    Draft(ctx context.Context, deps Deps, target DraftTarget) (DraftResult, error)
}

type SurveyResult struct {
    Observations []Observation
    Skipped      bool
    SkipReason   string
}

type Observation struct {
    Title       string  // shown on the backlog card
    Rationale   string  // 1–2 sentences, "why this matters"
    TargetID    string  // product_id / order_id; required, non-empty
    Score       float64 // 0..1; ranking input for auto-promote order
}

type DraftTarget struct {
    TargetID string  // the thing the persona is drafting a proposal for
    Hint     string  // the observation rationale, passed forward
}
```

`Survey` implementation per persona (sketch):

- **Pricing**: MCP-fetch products with margin variance outside a healthy band; LLM ranks/structures into 3–5 observations.
- **Marketing**: MCP-fetch products with short descriptions or weak conversion words; LLM ranks/structures.
- **Sales Support**: MCP-fetch completed-but-not-followed-up orders in the last 14 days; LLM ranks/structures.

Each survey is one LLM call returning a JSON array of `Observation`, **capped at 5 entries** server-side (excess truncated, lowest-scored dropped). The cap is a tunable per-persona constant (`SurveyMaxObservations`, default 5) so we can dial down to 3 in test environments with thin data. Schema-validated server-side; malformed responses → survey returns `Skipped=true, SkipReason="malformed survey response"`. Errors during MCP fetch → existing `ErrTransport` / `ErrSessionLost` classification applies.

`Draft` is the existing per-persona Draft, with the addition that `DraftTarget` is now explicit instead of implicit. Each persona's existing Draft already does most of this work; the refactor is mostly threading `target_id` and `hint` through.

## RunAndPersist dispatch

```go
type RunMode struct {
    Kind        string  // "survey" | "draft"
    PromoteFrom string  // issue_id of the backlog row to draft against, when Kind="draft"
}

func RunAndPersist(ctx context.Context, p Persona, deps Deps, mode RunMode) (Result, error)
```

**Survey path:**

1. Check `IsEnabled(persona)`; skip if disabled.
2. Check `HasOpenWork(persona)` (narrowed: `in_progress | in_review`); skip with existing reason if true.
3. Call `p.Survey(ctx, deps)`.
4. For each observation in result: upsert by `(persona, target_id)` — if a backlog row exists, update rationale + score + updated_at; if not, insert as `status='backlog'` with `proposal_content=NULL`.
5. Return immediately. Auto-promotion is the scheduler's job, not the survey's.

**Draft path:**

1. Load the issue at `mode.PromoteFrom`. If not `status='backlog'`, return `Skipped=true, SkipReason="not in backlog"`.
2. Flip status to `in_progress`.
3. Build `DraftTarget{TargetID: issue.target_id, Hint: issue.observation_rationale}`.
4. Call `p.Draft(ctx, deps, target)`.
5. On success: write `proposal_content`, flip status to `in_review`, return.
6. On error: increment `draft_attempts`; if `draft_attempts >= 3`, flip status to `rejected` with `dismiss_reason='draft_attempts_exhausted'`; otherwise flip status back to `backlog`. Surface error in the `runs` row via the existing classification.

The promotion run's status follows the existing scheduler conventions — `succeeded` if Draft completed, `failed_transient` / `failed_permanent` per the error classifier.

## Scheduler dispatch

Inside `scheduler/loop.go`, the existing per-persona decision tree becomes:

```
for each enabled persona:
    if HasOpenWork(persona):                       // in_progress | in_review present
        continue
    if exists backlog row for persona:
        // top-scored backlog row by observation_score DESC, then created_at ASC
        enqueue(promotion, target_issue=top.id)
    else if never ran:
        enqueue(bootstrap, mode=survey)
    else:
        enqueue(tick, mode=survey)
```

A new trigger value `promotion` joins `tick | manual | bootstrap | retry` on `runs.trigger`. The worker's existing dispatch already routes to `RunAndPersist`; we add a `mode` argument resolved from the trigger and `target_issue_id`.

`POST /v1/runs` gains an optional `target_issue_id` field. When set, the manual trigger enqueues a promotion run for that issue. When unset, manual triggers fire a survey run (matches today's "force a fresh pass" mental model).

## UI

**Backlog card** — lighter shape than a proposal card:

```
┌─────────────────────────────────────┐
│ [Marketing]  D7802634               │   persona pill (per F5: name, not artifact), id slug
│ Indigo-Dyed Throw Pillow            │   title
│ Description thin · low CVR words    │   observation_rationale, 1 line, ellipsized
│ [Draft this →]                      │   button, POSTs /v1/runs with target_issue_id
└─────────────────────────────────────┘
```

**Drafting card** — same shape as today's in-progress visual, with a spinner + "Drafting now…" hint. The card moves from Backlog to Drafting automatically when status flips.

**Polling.** The kanban hook (existing) bumps its interval to ~2s while any visible issue is `in_progress`, back to ~30s otherwise. No SSE in v0.1.0.

**Persona pill (F5 fix).** `StatusBadge.tsx` / `KindBadge` reads the persona name (`Marketing / Pricing / Sales`) in the persona color, not the artifact type. The artifact type stays in the secondary line of the card. CLAUDE.md's persona-color exception documentation updates alongside (currently says "CONTENT / CAMPAIGN / EMAIL").

**Survey-failure surface.** When a persona's last run is `failed`, the kanban header renders a small notice scoped to that persona's swimlane: "Marketing's last survey failed. Retry?" The Retry CTA posts a manual run.

## Failure handling

| Failure | Handling |
|---|---|
| Survey LLM call fails (transient) | Existing retry chain. Run row records `failed_transient`. Header notice on retry exhaustion. |
| Survey LLM returns malformed JSON | Survey returns `Skipped=true, SkipReason="malformed survey response"`. No issues written. |
| MCP fetch inside Survey fails | Existing `ErrTransport`/`ErrSessionLost` classification. Same skip surface as today. |
| Draft LLM fails (transient, attempts < 3) | Issue flips back to `backlog`, `draft_attempts++`. Card shows "Last draft failed · retry?" badge. |
| Draft LLM fails (attempts == 3) | Issue flips to `rejected` with `dismiss_reason='draft_attempts_exhausted'`. |
| Operator deletes a backlog issue mid-survey-upsert | Upsert is a single SQL statement; race surfaces as a normal not-found, run logs and continues. |

## Test plan

| Layer | Test shape |
|---|---|
| Migration `013_backlog_observations.sql` | Applies fresh and against a populated v0.2 DB. Existing `in_review` rows untouched. `HasOpenWork` narrowing doesn't drop rows that should still count. |
| `Persona.Survey` per persona | Stub MCP + stub LLM. Returns N observations with required fields populated; malformed LLM response → `Skipped`. |
| `RunAndPersist` survey path | Upsert dedupe: same `(persona, target_id)` updates instead of inserts. Insert when new. Skip when `HasOpenWork` true. |
| `RunAndPersist` draft path | Flips status `backlog → in_progress → in_review` on success. Backlog re-entry + `draft_attempts++` on transient fail. Auto-`rejected` at attempts==3. |
| Scheduler loop | Matrix: (no work, backlog exists, in_progress exists, in_review exists, never-ran) × (enabled, disabled). Top-scored backlog row is enqueued. |
| API | `POST /v1/runs` with `target_issue_id` enqueues a promotion run. Without it, enqueues a survey run. `GET /v1/runs/:id` includes the target issue id when set. |
| UI smoke | Against the test store: bootstrap fills board with 3 personas × (1 in_review + up to 4 backlog) cards. Click "Draft this" on a backlog card → it moves to Drafting (spinner) → to In Review with proposal content. |

## Risks and mitigations

- **Survey LLM cost roughly doubles per-persona bootstrap.** Today: one Draft call. After: one Survey + one Draft. Mitigation: ranked, structured Survey prompt is cheaper than a full Draft (no proposal content generation, just title + rationale + score for N items). Net cost manageable for the 3-persona fleet. Worth re-evaluating when the fleet expands to 7.
- **Survey quality is the new load-bearing thing.** If the survey returns three weak observations, all three cards look weak — a worse impression than one solid in_review card. Mitigation: prompts pinned to per-persona heuristics (margin variance, weak descriptions, completed-but-unfollowed orders) so the LLM is ranking known signals, not free-associating.
- **`HasOpenWork` narrowing is a behavioral change.** Today, a persona with `in_progress` issues gets skipped (probably never observed in practice since drafts are synchronous). The narrowing keeps that property. The visible effect is only that `backlog` no longer blocks new surveys, which is the intended change.
- **Test store data shape may not produce 3 observations per persona.** If the staging store has only one product with a weak description, Marketing's survey returns one observation. Board ends up sparse anyway. Mitigation: pre-seed the test store with sample data before the internal testing call (already on the agenda for the testing call prep).
- **Draft path race.** If the operator clicks "Draft this" while the scheduler is also auto-promoting the same issue, two promotion runs could race. Mitigation: the `backlog → in_progress` flip uses `UPDATE issues SET status='in_progress' WHERE id=? AND status='backlog'`; only the first wins, the second's worker sees `status != 'backlog'` and skips.

## Follow-ups

- **Top-up-when-low logic + observation staleness** — option C from the brainstorm. Lives naturally in insights-planning's Planner persona; deferred to that workstream.
- **Cross-persona ranking** — same workstream.
- **SSE for live Drafting updates** — replace the poll-interval bump with server-sent events when the daemon's HTTP server grows a streaming surface.
- **Migration of backlog issues to `insights` rows** when insights-planning's migration 012 lands. The `observation_score / observation_rationale / target_id` columns map directly onto `insights.score / body / issue_id`.
- **F5 persona-name badges** ship with this change. CLAUDE.md persona-color exception section needs an update alongside (currently documents kind pills as "CONTENT / CAMPAIGN / EMAIL").

## Design review triggers (per `DESIGN.md`)

- New card variant (Backlog observation card) — designer review before UI step lands.
- New CTA copy ("Draft this →", "Drafting now…", "Last draft failed · retry?") — short but it's the trust surface, worth a pass.
- Kind-pill label change is a customer-visible copy + IA change — confirm with design.
