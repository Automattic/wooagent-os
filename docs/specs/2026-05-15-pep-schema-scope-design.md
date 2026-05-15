# PEP Phase-2: checkSchema + checkScopeSufficiency — Design

**Date:** 2026-05-15
**Status:** Design (pre-implementation plan)
**Scope:** Two of the four stubbed PEP checks. Policy + budget remain stubbed.

## Motivation

WooAgent is launching to the public, not to a demo audience. The PEP currently passes through four security checks (schema validation, policy, budget, scope sufficiency) with TODOs. Per the launch-readiness frame, these are launch-blocking, not deferred. This spec lights up the two checks that don't require new infra surfaces — schema validation and scope sufficiency. Policy + budget each pull in additional dependencies (operator admin surface for policies; per-persona counter store for budgets) and warrant their own design rounds.

## Goals

1. Light up `checkSchema` so the daemon catches malformed args before they reach the WP boundary.
2. Light up `checkScopeSufficiency` so autonomous agent calls cannot exceed the manifest-declared authority of an ability.
3. Preserve current operator-Approve behavior (the only PEP call site today, always `IntentApply` against propose-scoped abilities).
4. Match the existing PEP code style: typed reason codes, audit row per decision, no free-form strings into the audit log.

## Non-goals

- Policy predicate evaluation (`checkPolicy`).
- Per-persona budget enforcement (`checkBudget`).
- Operator-configurable scope for non-manifest (operator-approved) abilities.
- Manifest format changes; no `input_schema` baked into the manifest.
- DB schema migrations. The existing `abilities.schema_json` / `schema_hash` columns are sufficient.
- New HTTP endpoints, metrics, retries, or cache eviction.

## Design decisions

### 1. Scope sufficiency under operator vs. agent origin

The only PEP call site today (`handlers_v1.go:494`, the Approve handler) always sends `Intent: pep.IntentApply` against abilities at various manifest scopes — including `wooagent-products/update` at `scope: propose`. A literal "deny Intent > Scope" rule would break the operator-Approve flow.

**Decision:** `pep.Request` gains a `Source` field. Operator-mediated calls (operator clicked Approve) bypass strict scope sufficiency; agent-mediated calls enforce strictly. The conceptual model: "scope: propose" means agents can only propose; operators applying via review is the legitimate apply step.

```go
type Source string

const (
    SourceOperator Source = "operator"
    SourceAgent    Source = "agent"
)
```

Default zero `Source("")` is treated as `SourceAgent` in the check — deny-by-default for any future caller that forgets to set Source. Only the explicit Approve call site opts into operator mode.

### 2. JSON Schema validator

**Decision:** `github.com/santhosh-tekuri/jsonschema/v5`. Battle-tested Go validator with broad draft support (07/2019-09/2020-12), already named in the existing TODO comment. Add as a direct dep. (`google/jsonschema-go` is already in `go.sum` indirectly, but it's newer and primarily oriented around structured-output use cases, not arbitrary validation.)

### 3. Schema source

**Decision:** Read `input_schema` from the cached `abilities.schema_json` envelope (per-call DB read, matching `checkTrustState`'s pattern). Single source of truth for both pre-signed and operator-approved abilities. Manifest entries do not carry input_schema.

If `schema_json` is absent (NULL or empty — e.g., a pre-signed ability invoked before the first discovery sweep), `checkSchema` passes through. The companion plugin still validates at the WP boundary, so a missed local validation degrades to "no early-fail," not a security hole.

### 4. Failure mode for malformed cached schemas

**Decision:** Conservative deny with a distinct typed reason. The launch posture is "everything needs to work"; silent degradation under cache rot is exactly what the launch memory warns against. A new `ReasonSchemaCompileError` distinguishes infra-side failures (cache rot, draft mismatch) from caller-side mistakes (`ReasonInvalidArguments`) in the audit log — operators triage faster.

## Data model changes

### `daemon/internal/pep/decisions.go`

Add `Source` enum + field on `Request`:

```go
type Source string

const (
    SourceOperator Source = "operator"
    SourceAgent    Source = "agent"
)

type Request struct {
    // ... existing fields
    Source Source
}
```

Add typed reason:

```go
const (
    ReasonSchemaCompileError ReasonCode = "schema_compile_error"
)
```

### Approve call site

`daemon/internal/httpapi/handlers_v1.go:494` — set `Source: pep.SourceOperator` on the existing `pep.Request` literal. One-line change.

## Implementation

### Files touched

| File | Change |
|---|---|
| `daemon/internal/pep/pep.go` | Implement `checkSchema` (DB read + cache + validate); implement `checkScopeSufficiency` (manifest lookup + rank compare); `Invoke` passes `ctx` to `checkSchema`; add `schemas *schemaCache` to `PEP`. |
| `daemon/internal/pep/decisions.go` | Add `Source` enum + `Request.Source`; add `ReasonSchemaCompileError`. |
| `daemon/internal/pep/schema.go` *(new)* | `schemaCache` (sync.Map keyed by `name:hash`); `compileOrGet` helper that extracts `input_schema` from the envelope and compiles. |
| `daemon/internal/pep/checkschema_test.go` *(new)* | Table-driven tests (pass-through cases, valid, invalid, compile-error, cache-hit sentinel). |
| `daemon/internal/pep/checkscopesufficiency_test.go` *(new)* | Table-driven tests across Source × Intent × Scope × manifest-entry presence. |
| `daemon/internal/pep/pep_test.go` | Extend inline `abilitiesDDL` with `schema_json TEXT, schema_hash TEXT`. Add `Source: pep.SourceOperator` to the existing Allowed/Trusted/MCPError/NoMCP tests. |
| `daemon/internal/httpapi/handlers_v1.go` | Add `Source: pep.SourceOperator` to the Approve `pep.Request` literal. Add `ReasonSchemaCompileError` case to `pepDenialMessage` + `writePEPDenial`. |
| `daemon/go.mod` / `daemon/go.sum` | Add `github.com/santhosh-tekuri/jsonschema/v5` as a direct dep. |

### `checkSchema` flow

1. `SELECT schema_json, schema_hash FROM abilities WHERE name = ?`.
2. If `sql.ErrNoRows`, or `schema_json` is NULL, or `schema_json` is the empty string → return `""` (pass-through; companion plugin validates at WP boundary).
3. Other DB error → `ReasonSchemaCompileError`. This deliberately lumps "lookup infra failed" under the schema-compile reason: same posture as `checkTrustState`'s "deny on lookup failure" stance, and an unreachable abilities table during a Phase-2 check is closer to a cache-rot situation than a caller mistake.
4. `schemas.compileOrGet(name, hash, schemaJSON)`:
   - Cache hit → use compiled `*jsonschema.Schema`.
   - Cache miss: decode envelope, extract `input_schema` (RawMessage). If absent → cache `nil` semantics, return pass-through. Else compile via `jsonschema.NewCompiler` + `AddResource(key, schema) + Compile(key)`. On compile failure → `ReasonSchemaCompileError`. On success → cache and return.
5. `s.Validate(req.Args)`:
   - On error → `ReasonInvalidArguments`.
   - On success → `""`.

### `schemaCache` shape

```go
type schemaCache struct {
    entries sync.Map // key: name + ":" + schema_hash; value: *jsonschema.Schema
}
```

No eviction in V1. Manifest entries are ~20; operator-approved abilities scale slowly; compiled schemas are kilobytes. If cardinality becomes a real concern later, swap to a fixed-size LRU.

### `checkScopeSufficiency` flow

1. `entry := p.manifest.Get(req.Ability)`. If `nil` → return `""` (operator-approved abilities; admin surface for scoping them is Phase 2's separate problem, same as in `checkPersonaScope`).
2. If `req.Source == SourceOperator` → return `""`.
3. Otherwise: compute `intentRank(req.Intent)` and `scopeRank(entry.Scope)`. Unknown intent (`Intent("")`) → rank 0 → deny. If `intentRank > scopeRank` → `ReasonScopeInsufficient`. Else `""`.

Authority rank: `read=1, propose=2, apply=3`. Allowed when `intentRank ≤ scopeRank`.

### Invoke pipeline

Pipeline order stays the same (PRD §8.4.2): trust → persona → schema → policy → budget → scope-sufficiency. Only `checkSchema` gains a `ctx` parameter.

## Error handling

| Failure mode | Reason code | HTTP status |
|---|---|---|
| Args fail JSON Schema validation | `ReasonInvalidArguments` | 422 (already mapped) |
| Cached schema fails to compile / DB error | `ReasonSchemaCompileError` | 500 (new mapping) |
| Agent call exceeds manifest scope | `ReasonScopeInsufficient` | 403 (already mapped) |

`ReasonSchemaCompileError` is operator-actionable but not caller-actionable, so 500 ("daemon can't process this until the schema cache is repaired") reads more honestly than 422 ("your request is wrong"). `pepDenialMessage` returns a string hinting at recovery (re-discovery via Skills UI).

**Audit log:** denials write the typed `ReasonCode` to `audit_invocations.denial_reason`. The validation error message from santhosh-tekuri is NOT propagated to audit or HTTP (potential PII in args); it's emitted at daemon `Warn` log level only.

## Testing

### `checkschema_test.go`

| Case | Setup | Expected |
|---|---|---|
| Pass-through, no DB row | Ability in manifest, no abilities row | Allowed, MCP dispatched |
| Pass-through, empty schema_json | DB row with `schema_json = NULL` | Allowed |
| Pass-through, envelope lacks input_schema | DB row with `schema_json='{"name":"x"}'` | Allowed |
| Valid args | Schema requires `id:integer`; args = `{"id":1}` | Allowed |
| Invalid args, missing required | Schema requires `id`; args = `{}` | Denied, `ReasonInvalidArguments` |
| Invalid args, wrong type | Schema requires `id:integer`; args = `{"id":"oops"}` | Denied, `ReasonInvalidArguments` |
| Schema-compile failure | `schema_json` contains an invalid JSON Schema | Denied, `ReasonSchemaCompileError` |
| Cache hit | Same ability invoked twice; mutate `schema_json` in DB between calls; second call uses cached compiled schema | Both calls validate against the original schema (proves cache reuse) |

### `checkscopesufficiency_test.go`

Table-driven across (Source, Intent, Scope, manifest-entry presence):

| Source | Intent | Manifest Scope | Manifest entry? | Expected |
|---|---|---|---|---|
| `SourceOperator` | `apply` | `propose` | yes | allowed |
| `SourceOperator` | `apply` | `read` | yes | allowed |
| `SourceAgent` | `apply` | `apply` | yes | allowed |
| `SourceAgent` | `apply` | `propose` | yes | `ReasonScopeInsufficient` |
| `SourceAgent` | `propose` | `propose` | yes | allowed |
| `SourceAgent` | `propose` | `read` | yes | `ReasonScopeInsufficient` |
| `SourceAgent` | `read` | `read` | yes | allowed |
| `SourceAgent` | `read` | `propose` | yes | allowed |
| `Source("")` (zero) | `apply` | `propose` | yes | `ReasonScopeInsufficient` (unset Source → strict) |
| `Source("")` (zero) | `read` | `read` | yes | `ReasonScopeInsufficient` (unset Source → strict) |
| `SourceAgent` | `Intent("")` (zero) | `apply` | yes | `ReasonScopeInsufficient` (unset Intent → deny) |
| `SourceAgent` | `apply` | `propose` | no | allowed (no manifest entry → bypass) |

### Existing `pep_test.go`

- Extend `abilitiesDDL` constant with `schema_json TEXT, schema_hash TEXT` columns.
- Add `Source: pep.SourceOperator` to: `TestInvoke_AllowedSuccess`, `TestInvoke_OperatorTrustedBypassesTrustCheck`, `TestInvoke_MCPCallError`, `TestInvoke_NoMCPClient`. The two existing Denied tests (Unapproved, PersonaForbidden) don't need it — earlier checks deny first.

## Out of scope

- `checkPolicy` and `checkBudget` remain stubbed; each needs its own design round (operator admin surface for policies; counter store for budgets).
- No manifest format changes.
- No DB migrations.
- No new HTTP endpoints, metrics, retries, or cache eviction.
- No call-site updates beyond Approve. Future orchestrator/agent call sites will set `Source: SourceAgent` and emit appropriate `Intent` when they land.
- No operator-configurable scope for non-manifest abilities.
- No JSON Schema draft pinning; validator auto-detects from `$schema`.

## Open questions

None at design time. All four decision points were resolved in the brainstorming round:
1. Source-gated scope sufficiency (vs. strict / vs. defer).
2. santhosh-tekuri/jsonschema/v5 (vs. google/jsonschema-go).
3. DB-cached schema source (vs. baking into manifest).
4. Conservative deny on schema compile failure (vs. pass-through warn).
