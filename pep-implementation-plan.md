# Policy Enforcement Point — Implementation Plan

The deterministic non-LLM gate between the orchestrator and MCP. Anchored to PRD §8.4.2; complementary to the manifest layer that already shipped (`daemon/internal/manifest/`).

## Goal

A single chokepoint — `pep.Invoke(...)` — through which **every** MCP ability call must flow. No orchestrator → MCP shortcut. Six deterministic checks, a per-invocation capability token, and a tamper-evident audit row per call. No LLM in the path.

## Scope for V1 — thin slice, not the full vision

In V1 the operator clicks Approve on every store-mutating change. The LLM never invokes an ability autonomously. That means most of the PEP's runtime value (preventing prompt-injected calls, enforcing budgets, blocking out-of-policy writes) is **redundant with the human-in-the-loop gate that already exists**. Building the full §8.4.2 surface now is optimizing for a world we won't enter until Phase 2 personas land.

V1 ships the parts of the PEP that have value *today*:

- **Chain-of-identity audit log** — answers "why did the agent do that?" in every demo and every customer call. Without this, "auditable autonomy" is aspirational.
- **Trust-state check** — denies invocations of unapproved abilities. Cheap, catches real bugs, makes the demo claim true instead of aspirational.
- **Persona-scope check** — denies cross-persona ability calls (e.g., a Marketing-persona issue trying to hit a Pricing-only ability). Cheap, catches real seeder/code bugs.

V1 explicitly defers schema validation, policy predicates, budgets, capability tokens, and tamper-evidence. Each of those is on the plan in a later phase, and they're all designed against the same `Invoke()` interface so the architecture doesn't change when we light them up.

## What we already have

- **Manifest** (`daemon/internal/manifest/`). Defines `Entry`, `Scope` (read/propose/apply), `TrustState` (pre-signed/operator-approved/unapproved), `Reversibility`, persona mappings, and `Lookup` for O(1) ability resolution. 25-entry default manifest is embedded.
- **MCP client** (`daemon/internal/mcp/`). Used today directly from `handlers_v1.go` and `cmd/persona-marketing/`.
- **Issues store** (`daemon/internal/store/`). SQLite + migrations, the natural home for the audit table.
- **Approve flow.** Single MCP call site in `handleApproveIssue` (`handlers_v1.go:305`) — small target for the first refactor.

## What we're building

A new package `daemon/internal/pep/` plus an audit-log migration. Everything else is rewiring existing call sites to route through it.

### Public surface

```go
type Decision struct {
    Allowed   bool
    Reason    string             // typed reason code on deny
    CapToken  string             // populated only when Allowed
    AuditID   int64              // row id of the audit log entry
    Outcome   string             // success | mcp_error | denied
}

type Request struct {
    Persona     manifest.Persona
    Ability     string             // fully-qualified, e.g. wooagent-products/update
    Args        map[string]any     // already JSON-decoded
    Intent      Intent             // Read | Propose | Apply
    PlanID      string             // chain-of-identity context
    TaskID      string
    StepID      string
    IssueID     string             // when the call originates from an issue (e.g. approve)
    PromptHash  string             // hash of the prompt that produced this call (or "" if not LLM-driven)
    Model       string             // model id, "" if non-LLM
}

func (p *PEP) Invoke(ctx context.Context, req Request) (Decision, *mcp.CallToolResult, error)
```

`Invoke` either returns a typed `permission_denied` error class with one of the reason codes below, or it executes the call and returns the MCP result. There is no path that returns success without producing both a cap token and an audit row.

### The six checks (in order, per PRD §8.4.2)

| # | Check | Backed by | Deny code |
|---|---|---|---|
| 1 | **Trust state** | `manifest.Lookup` + operator-approval table | `ability_unapproved` |
| 2 | **Persona scope** | `Entry.Personas` + operator overrides | `persona_forbidden` |
| 3 | **Schema validation** | structural validation against the registered `input_schema` | `invalid_arguments` |
| 4 | **Policy predicates** | operator-configured rules (see "Phase 2" below) | `policy_violation` |
| 5 | **Budgets** | persona daily token / cost / call counters | `budget_exceeded` |
| 6 | **Scope sufficiency** | `Intent` vs `Entry.Scope`; write to a `propose`-scoped ability requires staging, not applying | `scope_insufficient` |

Order matters: cheapest checks first, expensive ones (schema, policy) last. Failing any check short-circuits the rest.

### Capability token

After all checks pass, mint a short-lived token bound to `(invocation_id, ability, args_hash, expires_at)`. Pass it to the MCP client, which includes it in the request to the companion plugin. Companion verifies it against an `/v1/cap/verify` endpoint on the daemon (or a shared-secret HMAC — TBD; both shapes exist in security-token-exchange literature). Token TTL: 30s. Single-use.

### Audit log — chain of identity

New table `audit_invocations`:

```
plan_id, task_id, step_id, persona, model, prompt_hash, ability, args_hash,
cap_token_id, outcome, denial_reason, created_at, completed_at
```

One row per `Invoke` call regardless of outcome. Append-only; no UPDATE except setting `completed_at` and `outcome` on the row by id. Tamper-evidence is deferred (Phase 3) — Phase 1 just gets the rows in place.

Surfaced in the IssueDetail "Run log" pane (PRD §9.2) as the source of truth for *what the agent did and why it was allowed to*.

## Phasing

### V1 — thin slice (~1 day, one PR)

**Scope:** Two of the six checks (trust state, persona scope), one append-only audit table, one production call site rewired. Nothing else.

- `daemon/internal/pep/pep.go` — `New(deps) *PEP`, `Invoke(...)`. Two check functions only; the other four return pass-through stubs with TODO comments referencing the post-V1 phase that lights them up.
- `daemon/internal/pep/decisions.go` — full `Decision`, `Request`, `Intent`, all reason-code constants. Defining the surface in full now means post-V1 work is additive instead of refactor-y.
- `daemon/internal/pep/audit.go` — append-only writer for the audit table.
- `daemon/internal/store/migrations/00X_audit_invocations.sql` — the audit table.
- **Refactor `handleApproveIssue`** (`handlers_v1.go:305`) to call `pep.Invoke` instead of `s.mcp.CallTool`. Build a `Request` from the issue + selected variant.
- **Refactor `cmd/persona-marketing/main.go`** to route its single MCP call site through PEP too. Otherwise the rule "no orchestrator → MCP shortcut" is already broken on day one.
- **Tests.** Trust-state check + persona-scope check + audit writer get table-driven tests. One integration test for the approve path: pass + each deny mode + audit row written. ~80% line coverage on what we actually shipped.

V1 deliberately defers (each below has a post-V1 phase that turns it on):
- Schema validation. The companion plugin validates at the WP boundary; doubling up earns nothing until agents are autonomous.
- Policy predicates. No operator-configured policies exist to enforce.
- Per-persona budgets. No autonomous agent is burning budget.
- Capability tokens. The daemon is the only path to MCP today.
- Tamper-evident audit. Append-only is enough until we're worried about insider risk.

V1 still moves the needle: trust state and persona scope are real on day one, the audit log shows the chain of identity behind every store mutation, and the architecture is in place so Phase 2 personas land already-gated.

### Post-V1 / Phase 2 — schema validation + policy predicates + budgets (~3-4 days)

Triggered by: a second persona shipping, or an operator asking for "never auto-discount > 20%".

- `daemon/internal/pep/schema.go` — JSON Schema validator wrapper. `github.com/santhosh-tekuri/jsonschema/v5`.
- **Policy DSL.** Small expression language over `args`/`target`/`previous` — enough to express "never invoke `products-update` with `price < cost*1.1`" and "never invoke `orders-refund` for amount > $500 without explicit per-issue approval". Likely Cel-Go.
- **Budget counters** in SQLite with daily window math. Per-persona limits configured in `~/.wooagent/config.yaml`. `wooagent budget show` surfaces remaining.
- **Operator policy admin** — minimal CLI (`wooagent policy add/list/remove`) before any UI.

### Post-V1 / Phase 3 — capability tokens + tamper evidence + drift detection (~2-3 days, multi-PR)

Triggered by: agents going autonomous (Phase 2 personas applying changes without per-issue review), or onboarding a third-party MCP plugin not authored by us.

- **Companion plugin** verifies cap tokens via shared-secret HMAC; daemon mints with the same secret. Refuses ability calls without a valid, unexpired, single-use token.
- **Audit log tamper evidence.** Add `prev_hash` column; each row hashes prior row's id+contents. Detects retroactive edits.
- **Drift detection wiring.** Periodic re-sync recomputes schema hashes for all discovered abilities; mismatches auto-demote to `unapproved` and emit a notice into the daemon's notifications channel.

## File layout

```
daemon/
  internal/
    pep/
      pep.go             // Invoke(); V1 wires 2 checks + audit, others stub
      decisions.go       // full surface (types and reason codes)
      audit.go           // append-only audit row writer  ← V1
      schema.go          // input_schema validator        ← Phase 2
      policy.go          // predicate DSL                 ← Phase 2
      budget.go          // daily counters                ← Phase 2
      captoken.go        // mint + verify                 ← Phase 3
      pep_test.go        // table-driven check tests
      integration_test.go// full Invoke() paths
    store/
      migrations/
        00X_audit_invocations.sql       ← V1
        00Y_persona_budgets.sql         ← Phase 2
        00Z_audit_invocations_chain.sql ← Phase 3 (adds prev_hash)
```

## Migration path for existing call sites

1. **`handleApproveIssue`** — current direct call to `s.mcp.CallTool` becomes:
   ```go
   decision, mcpResult, err := s.pep.Invoke(ctx, pep.Request{
       Persona: manifest.PersonaMarketing,
       Ability: dispatch.ability,
       Args:    params,
       Intent:  pep.IntentApply,
       IssueID: id,
       // PlanID/TaskID/StepID empty for now — approve is operator-driven
   })
   ```
   On `decision.Allowed == false`, map the reason code to an HTTP status (`403 forbidden` for persona/scope, `422` for schema/policy, `429` for budget).

2. **`cmd/persona-marketing/main.go`** — same refactor. The CLI builds the Request itself; no daemon round-trip needed since it lives in the same process tree (or, if we want to keep the persona-marketing CLI as a true client, route via a new `/v1/invoke` endpoint that wraps `pep.Invoke`).

3. **No other production MCP call sites today.** New personas land already-PEP'd by construction.

## Test strategy

- Each check function has unit tests covering pass + every fail mode.
- A handful of golden integration tests for `Invoke`: known input → expected `Decision` + audit row + (when allowed) MCP call shape.
- `mcp.Client` is interface-based already; tests use a fake that records calls.
- Audit assertions read the audit row out of an in-memory SQLite and assert `outcome`/`denial_reason`/`cap_token_id`.

The PRD line "covered by unit tests, not by vibes" is the bar. Phase 1 ships with ~80% line coverage on the `pep` package; anything less should fail review.

## Open questions

1. **Cap token shape.** HMAC-signed JWT-ish blob, or opaque random + DB lookup on verify? JWT is operator-readable (good for debugging) but adds dependencies; opaque is one fewer thing to fight. Lean: opaque, with a tiny `/v1/cap/verify` endpoint. (Phase 3.)
2. **Where does the `Intent` come from?** Today, the approve handler sets it itself. For agent-driven calls we need the orchestrator to commit to `Read`/`Propose`/`Apply` per step. PRD §10 doesn't quite spell this out — worth a short clarification.
3. **What happens when an issue's `proposal_target.product_id` doesn't match the ability's schema?** Today the daemon would 422 from MCP. Under PEP, schema validation catches this earlier — better DX, but we need to make sure the message lands at the operator (not just stays in logs).
4. **Persona origin in non-agent contexts.** When the operator clicks Approve in the UI, "the persona" is the issue's owning persona. When a script seeds an issue and approves it, who's the persona? Probably the issue's, but worth nailing down.

## Out of scope for this plan

- Discovery / re-sync flow that *populates* the trust table (separate concern, partly already in `internal/manifest/loader.go`).
- The Ability explorer UI surface (PRD §11).
- Plan/task/step model (PRD §10) — PEP just consumes plan/task/step ids; the orchestrator wires them.
- Multi-tenant / hosted-daemon concerns. PEP is local-first; the same package will run unchanged when the hosted daemon ships.

## Estimate

- **V1 thin slice: ~1 day.** Two check functions, one migration, one writer, two refactors, table-driven tests for what shipped. Reviewable in a single PR.
- Phase 2 (post-V1): ~3-4 days, mostly on the policy-predicate DSL.
- Phase 3 (post-V1): ~2-3 days plus cross-PR coordination with the companion plugin.

V1 lands the architecture and the demo claim ("every store mutation is gated and audited"). The post-V1 phases turn on enforcement against a use case that doesn't exist yet — light each up when the use case shows up, not before.
