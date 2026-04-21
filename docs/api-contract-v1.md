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

### `GET /v1/issues[?status=&persona=]`

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
      "created_at": "2026-04-21T16:53:56Z",
      "updated_at": "2026-04-21T16:53:56Z"
    }
  ]
}
```

Query parameters:
- `status` — one of `backlog | todo | in_progress | in_review | done | rejected`.
- `persona` — persona ID (e.g. `marketing`).

### `POST /v1/issues`

Request:
```json
{
  "title":       "Rewrite product descriptions for clarity",
  "description": "Top 20 SKUs, brand-voice pass.",
  "persona":     "marketing",
  "priority":    "medium",
  "status":      "backlog"
}
```
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

### `POST /v1/issues/{id}/approve`  *(Phase 2)*

Moves the issue to `done` and triggers the staged MCP ability invocation. Returns the updated `Issue`. Currently returns `501 not_implemented`.

### `POST /v1/issues/{id}/reject`  *(Phase 2)*

Request body:
```json
{ "reason_tag": "off-brand | wrong-numbers | irrelevant | unsafe | incomplete | other", "reason_text": "free-form reason" }
```
Moves the issue to `rejected`. Currently returns `501 not_implemented`.

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
  created_at:  string;              // RFC3339
  updated_at:  string;              // RFC3339
}

type IssueStatus   = "backlog" | "todo" | "in_progress" | "in_review" | "done" | "rejected";
type IssuePriority = "urgent" | "high" | "medium" | "low" | "none";
```

Status machine: `backlog → todo → in_progress → in_review → done`. `rejected` is a terminal state reachable from `in_review`.

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

| HTTP | `code`             | when                                         |
|------|--------------------|----------------------------------------------|
| 400  | `bad_json`         | request body failed to parse                 |
| 400  | `missing_title`    | POST /issues without a title                 |
| 401  | `auth_missing`     | no bearer header                             |
| 401  | `auth_invalid`     | token not recognized                         |
| 404  | `not_found`        | no route / no such issue                     |
| 500  | `db_error`         | SQLite failure during a handler              |
| 500  | `db_scan`          | row scan failed                              |
| 501  | `not_implemented`  | Phase 2/3 surface not wired yet              |

## What's deliberately not here yet

- Store connection endpoints (MCP pairing) — lands Phase 3.
- Model-provider configuration — lands Phase 3.
- Ability discovery payloads — lands once MCP client does.
- Run log streaming — lands Phase 2.
- Webhook ingest — Phase 2+.

The types above are intentionally stable from day one so Nevena's design work and the daemon-side implementation can land in parallel.
