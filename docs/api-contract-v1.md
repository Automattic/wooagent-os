# WooAgent OS — HTTP API contract v1

**Status:** draft v1 (Phase 1, Apr 21 2026). This contract is the shape Nevena's designs key off of, and the shape the daemon's stubs already return. Real write paths (approve/reject) and MCP ability discovery land in later phases — their request/response shapes are documented here up front so the UI does not need to be reshaped when they come online.

## Connection model

- Daemon listens on `localhost:7777` by default (overridable via `wooagent run --bind`).
- All non-health endpoints require a bearer token: `Authorization: Bearer wo_pat_...`.
- Tokens are minted by `wooagent init` (first one, auto) and `wooagent auth token create` (additional).
- CORS is permissive: any origin, `Authorization` and `Content-Type` headers allowed. The bearer token is the real security boundary; CORS only loosens browser gating so the UI can run from any static host.
- Error envelope: `{ "error": { "code": string, "message": string } }`. HTTP status carries the category.

## Versioning

- Every response from `GET /v1/health` includes `schema_version: "v1"`. UIs compare this against their supported range on connect.
- Breaking changes go to a new prefix (`/v2/...`); additive changes stay on `/v1/`.

## Endpoints

### `GET /v1/health` (unauthenticated)

```json
{
  "status": "ok",
  "version": "0.1.0-dev",
  "schema_version": "v1"
}
```

### `GET /v1/agents`

```json
{
  "agents": [
    { "persona": "marketing",     "name": "Marketing & SEO", "model_preference": "anthropic/claude-sonnet-4-6", "enabled": true },
    { "persona": "pricing",       "name": "Pricing",         "model_preference": "anthropic/claude-sonnet-4-6", "enabled": true },
    { "persona": "sales_support", "name": "Sales Support",   "model_preference": "anthropic/claude-sonnet-4-6", "enabled": true }
  ]
}
```

### `GET /v1/issues[?status=&persona=&batch_id=]`

```json
{
  "issues": [
    {
      "id": "71fefac0-2d11-435b-a0b9-2eb2836a8916",
      "title": "Rewrite product descriptions for clarity",
      "description": "",
      "persona": "marketing",
      "status": "backlog",
      "priority": "medium",
      "batch_id": "bddb2d07-...",
      "created_at": "2026-04-21T16:53:56Z",
      "updated_at": "2026-04-21T16:53:56Z"
    }
  ]
}
```

Query parameters (AND-composed):
- `status` — one of `backlog | todo | in_progress | in_review | done | rejected`.
- `persona` — persona ID (e.g. `marketing`).
- `batch_id` — return only children of the named batch.

`batch_id` is omitted from the response (`omitempty`) when the issue is unbatched.

### `POST /v1/issues`

Request:
```json
{
  "title":       "Rewrite product descriptions for clarity",
  "description": "Top 20 SKUs, brand-voice pass.",
  "persona":     "marketing",
  "priority":    "medium",
  "status":      "backlog",
  "batch_id":    "bddb2d07-..."
}
```

`batch_id` is optional. When present, the daemon validates that the named
batch exists (404 `batch_not_found` otherwise) and the new issue is filed
as a child of that batch.

Response: the created `Issue` (201).

### `GET /v1/issues/{id}`

```json
{
  "issue":    { /* Issue */ },
  "runs":     [ /* Run (see below), most recent first — empty in v0.1 */ ],
  "proposal": null
}
```

When a persona is running in propose-mode, `proposal` becomes a `DiffProposal` (below).

### `POST /v1/issues/{id}/approve`

Optional request body:
```json
{ "variant_id": "A" }
```

When the issue's proposal carries multiple variants in `proposal.target.variants[]`, `variant_id` selects which body ships to MCP. Empty body keeps the legacy single-proposal path (ships `proposal_content` as-is).

Routes through the Policy Enforcement Point (§8.4 of the PRD): trust state, persona scope, schema, policy, budget, and scope-sufficiency checks before dispatching. On allow, the issue moves to `done` and a chain-of-identity audit row is written.

Race-hardened: claims the issue by flipping `status='in_review' → 'in_progress'` with a `RowsAffected==1` gate before invoking PEP; concurrent approvers that lose the race come back with `409 wrong_status`. The kanban briefly shows the card under Drafting during the in-flight window — by design.

Response (200):
```json
{
  "id":         "71fefac0-...",
  "status":     "done",
  "ability":    "wooagent-products/update",
  "audit_id":   42,
  "updated_at": "2026-05-04T13:42:00Z"
}
```

PEP denials map to typed HTTP responses: `403 ability_unapproved | persona_forbidden | scope_insufficient`, `422 invalid_arguments | policy_violation`, `429 budget_exceeded`. MCP failures return `502 mcp_call_failed | mcp_empty | mcp_decode | ability_failed`.

### `POST /v1/issues/{id}/reject`

No request body. Moves the issue from `in_review → rejected`.

Response (200):
```json
{ "id": "71fefac0-...", "status": "rejected", "updated_at": "2026-05-04T13:42:00Z" }
```

### `POST /v1/batches`

Creates a batch + N child issues atomically in a single transaction.

Request:
```json
{
  "title":         "Review & approve · 7 meta rewrites",
  "persona":       "marketing",
  "intent":        "meta_rewrite",
  "source_run_id": "...",
  "issues": [
    {
      "title":       "Meta rewrite · Brass Candlestick",
      "description": "...",
      "proposal": {
        "type":    "product_description_rewrite",
        "content": "...",
        "target":  { "product_id": 1001, "sku": "BC-007", "variants": [/* ... */] }
      }
    }
  ]
}
```

Children inherit `persona` from the parent unless they override. Default child status is `in_review`. Returns 201:
```json
{
  "batch":  { /* Batch */ },
  "issues": [ { "issue": /* Issue with batch_id */, "proposal": /* Proposal */ } ]
}
```

### `GET /v1/batches[?persona=]`

```json
{
  "batches": [
    {
      "id": "bddb2d07-...",
      "title": "Review & approve · 7 meta rewrites",
      "persona": "marketing",
      "intent": "meta_rewrite",
      "source_run_id": "",
      "total": 7,
      "pending": 7,
      "approved": 0,
      "rejected": 0,
      "created_at": "2026-05-04T13:00:00Z",
      "updated_at": "2026-05-04T13:00:00Z"
    }
  ]
}
```

Counts (`total`, `pending`, `approved`, `rejected`) are derived at read time via `LEFT JOIN issues GROUP BY batches.id` — they always reflect the children's current statuses.

### `GET /v1/batches/{id}`

Returns the batch with embedded children + their proposals so the UI can render the batch review screen in one fetch:
```json
{
  "batch":  { /* Batch */ },
  "issues": [
    { "issue": /* Issue */, "proposal": /* Proposal | null */ }
  ]
}
```

### `POST /v1/batches/{id}/approve-all`

Best-effort batch approval. Loops `approveOne` per child sequentially (SQLite writer-lock + audit attribution makes parallelism a net loss). Each child runs through PEP independently and gets its own audit row.

Request:
```json
{
  "children": [
    { "issue_id": "...", "variant_id": "A" },
    { "issue_id": "...", "variant_id": "B" }
  ]
}
```

`variant_id` is optional per child (single-proposal children ship `proposal_content` as-is).

Always returns 200, never aborts mid-loop on per-child failure. Response (`BatchOperationResult`):
```json
{
  "results": [
    { "issue_id": "...", "ok": true,  "status": "done",  "ability": "wooagent-products/update", "audit_id": 42, "updated_at": "..." },
    { "issue_id": "...", "ok": false, "error": { "code": "persona_forbidden", "message": "..." } }
  ]
}
```

Per-child `error.code` reuses the existing vocabulary (`mcp_call_failed`, `wrong_status`, `bad_variant_id`, all `pep.ReasonCode` values, etc.) — no batch-specific codes.

### `POST /v1/batches/{id}/reject-all`

No request body. Bulk update of every `in_review` child in the batch to `rejected`. Children that aren't `in_review` (already done/rejected) come back as `ok: false` with `code: wrong_status` rather than failing the whole batch.

Returns 200 with the same per-child shape as approve-all (success entries omit `ability` / `audit_id`).

### `GET /v1/abilities`  *(populated after MCP client lands)*

```json
{ "abilities": [ /* AbilityRef */ ] }
```

## Core types

### `Issue`

```ts
{
  id:          string;              // uuid
  title:       string;
  description: string;              // empty string if unset
  persona:     string | "";         // empty if unassigned
  status:      IssueStatus;
  priority:    IssuePriority;
  batch_id?:   string;              // present iff this issue is a child of a batch
  created_at:  string;              // RFC3339
  updated_at:  string;              // RFC3339
}

type IssueStatus   = "backlog" | "todo" | "in_progress" | "in_review" | "done" | "rejected";
type IssuePriority = "urgent" | "high" | "medium" | "low" | "none";
```

Status machine: `backlog → todo → in_progress → in_review → done`. `rejected` is a terminal state reachable from `in_review`.

### `Batch`

```ts
{
  id:             string;
  title:          string;
  persona?:       string;
  intent?:        string;       // free-form, e.g. "meta_rewrite"
  source_run_id?: string;       // chain-of-identity to the run that created the batch
  total:          number;       // derived: count of children
  pending:        number;       // derived: count where status='in_review'
  approved:       number;       // derived: count where status='done'
  rejected:       number;       // derived: count where status='rejected'
  created_at:     string;       // RFC3339
  updated_at:     string;       // RFC3339
}
```

A batch is a lightweight grouping — counts are computed from children at read time (never stored). Issues retain their own status; the batch has no separate lifecycle.

### `BatchOperationResult`

```ts
{
  results: Array<{
    issue_id:    string;
    ok:          boolean;
    status?:     string;        // present when ok=true (e.g. "done", "rejected")
    ability?:    string;        // present on approve-all success
    audit_id?:   number;        // present on approve-all success
    updated_at?: string;        // present when ok=true
    error?: { code: string; message: string };  // present when ok=false
  }>;
}
```

Returned by `POST /v1/batches/{id}/approve-all` and `…/reject-all`. Always 200 — per-child failure surfaces in the `error` object, not the HTTP status.

### `Persona`

```ts
{
  persona:           string;   // id, e.g. "marketing"
  name:              string;   // human label
  model_preference?: string;   // e.g. "anthropic/claude-sonnet-4-6"
  enabled:           boolean;
}
```

### `Run`

```ts
{
  id:           string;
  issue_id:     string;
  started_at:   string;
  completed_at: string | null;
  status:       "running" | "succeeded" | "failed" | "cancelled";
}
```

### `AbilityRef`

```ts
{
  namespace:   string;   // e.g. "woocommerce", "yoast-seo"
  name:        string;   // e.g. "products/update"
  schema:      object;   // JSON schema of inputs
  provided_by: string;   // plugin id
}
```

### `DiffProposal`

A diff is one of three shapes matching the three Phase 1 personas. The UI switches on `kind`.

```ts
type DiffProposal =
  | { kind: "prose";   before: string;       after: string; }
  | { kind: "numeric"; fields: { path: string; before: number; after: number; unit?: string }[] }
  | { kind: "message"; recipient: string;    subject?: string; body: string };
```

- `prose` — Marketing product-description rewrite. Render a two-column before/after with word-level diff highlighting.
- `numeric` — Pricing change. Render each field with before/after values, color-coded delta.
- `message` — Sales Support customer-reply draft. Render the draft with recipient + subject context.

## Error codes

| HTTP | `code`                  | when                                                                  |
|------|-------------------------|-----------------------------------------------------------------------|
| 400  | `bad_json`              | request body failed to parse                                          |
| 400  | `missing_title`         | POST /issues or POST /batches without a title                         |
| 400  | `missing_proposal_type` | proposal supplied without `proposal.type`                             |
| 400  | `bad_proposal_target`   | `proposal.target` not JSON-serialisable                               |
| 400  | `missing_issues`        | POST /batches without an issues array                                 |
| 400  | `missing_children`      | POST /batches/:id/approve-all without a children array                |
| 401  | `auth_missing`          | no bearer header                                                      |
| 401  | `auth_invalid`          | token not recognized                                                  |
| 403  | `ability_unapproved`    | PEP: ability not in trusted manifest (also `permission_denied`)       |
| 403  | `persona_forbidden`     | PEP: persona not on the ability's allowed list                        |
| 403  | `scope_insufficient`    | PEP: intent exceeds ability's authorised scope                        |
| 404  | `not_found`             | no route, no such issue, or no such batch                             |
| 404  | `batch_not_found`       | POST /issues with a `batch_id` that doesn't exist                     |
| 409  | `wrong_status`          | approve/reject on an issue not in `in_review` (or claim race lost)    |
| 422  | `no_proposal`           | approve called on an issue with no proposal                           |
| 422  | `unknown_proposal_type` | approve handler not registered for this proposal type                 |
| 422  | `bad_variant_id`        | `variant_id` not found in `proposal.target.variants`                  |
| 422  | `bad_target`            | proposal target unmarshallable                                        |
| 422  | `invalid_arguments`     | PEP: arguments fail ability input schema (Phase 2)                    |
| 422  | `policy_violation`      | PEP: arguments tripped an operator policy predicate (Phase 2)         |
| 429  | `budget_exceeded`       | PEP: persona over its daily budget (Phase 2)                          |
| 500  | `db_error`              | SQLite failure during a handler                                       |
| 500  | `db_scan`               | row scan failed                                                       |
| 502  | `mcp_call_failed`       | MCP transport / handshake / tool-call error                           |
| 502  | `mcp_empty`             | MCP returned no content                                               |
| 502  | `mcp_decode`            | MCP envelope failed to parse                                          |
| 502  | `ability_failed`        | MCP envelope reported `success: false`                                |
| 503  | `mcp_not_configured`    | daemon started without MCP credentials                                |

Inside a `BatchOperationResult.results[].error`, the `code` field reuses the same vocabulary above — there are no batch-specific error codes.

## What's deliberately not here yet

- Store connection endpoints (MCP pairing) — lands Phase 3.
- Model-provider configuration — lands Phase 3.
- Ability discovery payloads — lands once MCP client does.
- Run log streaming — lands Phase 2.
- Webhook ingest — Phase 2+.

The types above are intentionally stable from day one so Nevena's design work and the daemon-side implementation can land in parallel.
