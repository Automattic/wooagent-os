# Daemon: batch-issues concept

## Status

✅ **Shipped 2026-05-04.** Daemon migration `004_add_batches.sql`, `approveOne`
extracted from the single-issue approve handler (with race-hardening as a
free side-benefit), `handlers_batches.go` with five endpoints (`POST
/v1/batches`, `GET /v1/batches`, `GET /v1/batches/:id`,
`POST /v1/batches/:id/approve-all`, `POST /v1/batches/:id/reject-all`), PEP
`Request` gains `BatchID`, 10 unit tests in `handlers_batches_test.go` all
green. Seed script `seed-demo-batch-meta-rewrites.sh` ships a 7-product
demo batch in one POST. UI follow-on (`BatchReview.tsx` + kanban routes
batched cards to `/batches/:id`) shipped same session. Verified end-to-end
against the staging store. See `progress.md` Mon May 4 session for the
build log and findings.

## Context

Task 3 of the recent UI work shipped a refreshed single-product approval flow but **deferred** the Figma "Review & approve · 7 meta rewrites" screen because the daemon has no way to model a batch — every issue today is independent, with no parent / group / batch field on the table. To unlock that screen we need the daemon to grow a "batch" concept: a parent that groups multiple sibling issues sharing a generation prompt, so the operator can review them as a set with per-row Approve/Reject and a top-level counter.

Variants (multiple body alternatives within a single issue, A/B/C) and batches (multiple sibling issues sharing a parent) are **orthogonal**: a batch can have N children, each child can have M variants. We're adding the batch axis without disturbing the variant axis.

## Locked-in design decisions

1. **Lightweight grouping, derived status.** New `batches` metadata table; `issues.batch_id` FK; batch counters (Pending/Approved/Rejected) computed at read time from joined children — never stored, never out of sync.
2. **Best-effort `approve-all`.** Loop PEP per child; return 200 with a per-child `results` array even if some children deny. Reuses the existing audit-per-Invoke pattern.
3. **Per-child variant pick.** Approve-all body carries `{children: [{issue_id, variant_id?}, ...]}`. Each child dispatches with its own variant via the existing `resolveVariantBody` path.
4. **No autonomous agent loop in V1.** Batch creation is operator-driven via a new `POST /v1/batches` endpoint (and seed scripts).

## Implementation steps

### 1. Schema — `daemon/internal/store/migrations/004_add_batches.sql`

```
CREATE TABLE batches (
  id            TEXT PRIMARY KEY,
  title         TEXT NOT NULL,
  persona       TEXT REFERENCES agents(persona) ON DELETE SET NULL,
  intent        TEXT,
  source_run_id TEXT,
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);
ALTER TABLE issues ADD COLUMN batch_id TEXT REFERENCES batches(id) ON DELETE SET NULL;
CREATE INDEX idx_issues_batch_id ON issues(batch_id);
CREATE INDEX idx_batches_created_at ON batches(created_at DESC);
```

Convention notes (caught by validation):
- `created_at` / `updated_at` are `TEXT NOT NULL` — match `001_init.sql:25–26`, not `TIMESTAMP` (SQLite affinity hint, not a real type; the rest of the schema stores RFC3339 strings).
- `persona` FK uses `ON DELETE SET NULL` to match the existing `issues.persona` FK (`001_init.sql:27`).
- Adding a nullable column to populated `issues` rows is safe in SQLite — existing rows default to `NULL`.
- The migrate runner (`store.go:65–128`) already wraps each `00N_*.sql` in a `BeginTx`, so the multi-statement DDL is atomic for free.

### 2. Storage layer — `daemon/internal/store/batches.go` (new)

First cross-table app-level transaction in the codebase. Pattern: `s.DB.BeginTx(ctx, nil)` → insert `batches` row → loop child `INSERT INTO issues(...)` with `batch_id = ?` → commit. Defer `tx.Rollback()` (no-op post-commit).

Functions to expose:
- `CreateBatch(ctx, batch, children) (Batch, []Issue, error)` — atomic batch + children. Mirror `nullIfEmpty(persona)` from `handlers_v1.go:168–172` so issues created via this path are byte-for-byte interchangeable with `POST /v1/issues`.
- `GetBatch(ctx, id) (Batch, error)` — single row with derived counts.
- `ListBatches(ctx, filter) ([]Batch, error)` — list with derived counts. **One round-trip with `LEFT JOIN`** (avoid N+1):

  ```sql
  SELECT b.id, b.title, b.persona, b.intent, b.source_run_id, b.created_at, b.updated_at,
         COUNT(i.id) AS total,
         COUNT(CASE WHEN i.status = 'in_review' THEN 1 END) AS pending,
         COUNT(CASE WHEN i.status = 'done'      THEN 1 END) AS approved,
         COUNT(CASE WHEN i.status = 'rejected'  THEN 1 END) AS rejected
  FROM batches b LEFT JOIN issues i ON i.batch_id = b.id
  GROUP BY b.id
  ORDER BY b.created_at DESC;
  ```

  Use `COUNT(CASE WHEN x THEN 1 END)` (no `ELSE`) — `COUNT` ignores NULLs. `LEFT JOIN` keeps empty batches in the list with zero counts.
- `IssuesByBatch(ctx, batchID) ([]IssueWithProposal, error)` — used by the detail handler.

### 3. Refactor: extract `approveOne` from `handleApproveIssue`

Required before the batch loop can land. Current `handleApproveIssue` (`handlers_v1.go:253–390`) mixes HTTP, DB, PEP, dispatch, and response writing. Cleanest cleavage:

- Keep `handleApproveIssue` as the HTTP entry point; it does request-decode + status mapping only.
- New helper `approveOne(ctx, store, pep, issueID, variantID string) (approveResult, *approveError)` covers everything from the issue load (line 276) through the status update (line 378). Pure: no HTTP coupling, no response writing.
- `approveResult`: `{IssueID, Status, Ability, AuditID, UpdatedAt}` (mirrors current JSON shape, lines 383–388).
- `approveError`: `{HTTPStatus, Code, Message, PEPReason ReasonCode}` so the HTTP handler maps to `writePEPDenial` / `writeError` paths it uses today, and the batch handler converts to `{code, message}` per-child JSON.
- **Don't move `writePEPDenial` into the helper.** `pepDenialMessage` mapping (`handlers_v1.go:424–441`) is reusable from both call sites.
- Merge `loadIssuePersona` (`handlers_v1.go:395–406`) into the single `SELECT` at lines 276–278 — halves DB roundtrips per child (matters for 7-product batches).
- Move the `s.pep == nil` guard (line 254) into `approveOne`, surfaced as a 503-mapped `approveError`.

### 4. Race hardening on the status update (cheap, do it now)

Today's approve handler does `SELECT status` then later `UPDATE status='done'` non-transactionally. Single-issue cadence makes the race theoretical; batch loops + concurrent single-issue clicks make it real. Cheapest fix:

```sql
UPDATE issues SET status = 'in_progress', updated_at = ? WHERE id = ? AND status = 'in_review';
```

Gate on `RowsAffected == 1`. If 0, another approve already grabbed it — return a `wrong_status` error before invoking PEP. Goes inside `approveOne` so single-issue and batch paths both benefit.

### 5. HTTP handlers — `daemon/internal/httpapi/handlers_batches.go` (new)

Wire into `buildRouter` (`server.go:66–75`) **inside** the bearer-auth group.

- `POST /v1/batches` — body `{title, persona, intent, issues: [{title, description, proposal}, ...]}`; calls `store.CreateBatch`; returns `201` with `{batch, issues}`.
- `GET /v1/batches?status=…&persona=…` — calls `store.ListBatches`; AND-composes filters like `handleListIssues:83–93`.
- `GET /v1/batches/:id` — returns `{batch, issues: [{...issue, proposal}]}` so the UI gets everything in one request.
- `POST /v1/batches/:id/approve-all` — body `{children: [{issue_id, variant_id?}, ...]}`. **Sequential** loop calling `approveOne` per child (SQLite writer-lock + audit attribution makes parallelism worse). Always returns `200` with `{results: [{issue_id, ok, status?, audit_id?, error?}, ...]}`. Per-child `error.code` reuses the existing vocabulary (`mcp_call_failed`, `permission_denied`, `wrong_status`, the `pep.ReasonCode` values) — no new error codes.
- `POST /v1/batches/:id/reject-all` — wrap children in a `tx` and update `status = 'rejected'` `WHERE batch_id = ? AND status = 'in_review'`; return per-child results. Children not in `in_review` come back as `{ok: false, error: {code: 'wrong_status'}}` rather than failing the whole batch.

### 6. Extend existing endpoints

- `Issue` wire struct (`handlers_v1.go:30–39`): add `BatchID *string \`json:"batch_id,omitempty"\``. Update `SELECT` projections + scan order in `handleListIssues:80`, `handleGetIssue:192`, `handleCreateIssue:168`.
- `handleListIssues`: accept `?batch_id=` filter, AND-composed with `status` / `persona`.
- `handleCreateIssue`: optionally accept `batch_id` in the body, validated against `batches.id` (404 if missing).

### 7. PEP one-line forward-compat — `daemon/internal/pep/decisions.go:37–53`

Add `BatchID string` next to `IssueID`. PEP itself ignores it; `pep.Invoke` already records `IssueID` in the audit row, and joining audit_invocations → issues.batch_id retroactively works fine. The Request field is purely for chain-of-identity readability and future analytics ("which batches got partially denied"). **No** `batch_id` column on `audit_invocations` in V1 — avoids a second migration.

### 8. Seed script — `daemon/scripts/seed-demo-batch-meta-rewrites.sh` (new)

Mirror `seed-demo-merino-turtleneck.sh:36–93` (jq-built payload, bearer token from env, single `curl`). One `POST /v1/batches` payload with 7 product-rewrite proposals, each carrying its own `target.variants[]` with 3 variants. Used for visual testing of the batch screen (UI follow-on).

### 9. Tests

No `handlers_v1_test.go` exists today — set the pattern. Mirror `pep_test.go:40–67` for the in-memory SQLite fixture, plus:

- **Critical**: add `PRAGMA foreign_keys=ON` to the test fixture. The pep_test fixture omits it; FK-crossing tests (batch + issue) will silently let invalid FKs through without it.
- Coverage:
  - `CreateBatch` round-trip (commit + tx-rollback paths).
  - `ListBatches` derived counts: empty batch (zero counts), all-pending, mixed states.
  - `approveOne` extracted helper: happy path, PEP deny, race-loss (`RowsAffected == 0`).
  - `POST /v1/batches/:id/approve-all`: 3-child batch where the middle child's PEP denies — assert 200 with per-child `ok` flags, audit rows for each Invoke that ran.
  - `POST /v1/batches/:id/reject-all`: mixed children (in_review + done) → done child comes back `ok: false, code: wrong_status`; in_review child rejected.

## UI follow-on (out of scope here — separate task)

After daemon ships:

- `ui/src/api/client.ts` — add `Batch` interface + `api.batches.{list,get,approveAll,rejectAll}`; add `batch_id?: string` to `Issue`.
- `ui/src/screens/BatchReview.tsx` — match Figma `210:28015` (counter + per-child accordion + per-row variant pick + top-level Approve-all).
- `ui/src/App.tsx` — new route `/batches/:id`.
- Kanban: when an `Issue.batch_id` is set, click navigates to `/batches/:id`; meta line shows "{N} child tasks" (already drafted in `Kanban.tsx`'s placeholder logic).

## Watch out for

- **FK + WAL + memory tests** — `PRAGMA foreign_keys = ON` is per-connection (`store.go:42`). Tests that use `:memory:` directly must opt in.
- **Don't `cascade` on batch delete** — V1 has no batch-delete surface, but `ON DELETE SET NULL` is the right choice anyway: orphaning children to the unbatched pool is the product behavior we want.
- **Count semantics** — use `COUNT(CASE WHEN ... THEN 1 END)`, not `THEN 1 ELSE 0` (the latter counts zeros).
- **Nesting `approve-all`** — the per-child errors live inside a `200` response; don't reuse the top-level `{error: {code, message}}` envelope — embed `{code, message}` per child only.
- **Sequential, not parallel** — for N=7 children, SQLite writer contention + audit row attribution makes goroutines a net loss. Document the call-out so future maintainers don't "optimize" it.

## Critical files

- `daemon/internal/store/migrations/004_add_batches.sql` *(new)*
- `daemon/internal/store/migrations/001_init.sql:18–28` (schema convention reference)
- `daemon/internal/store/store.go:65–128` (migrate runner; pattern for `BeginTx`)
- `daemon/internal/store/batches.go` *(new)*
- `daemon/internal/httpapi/handlers_v1.go:30–39` (Issue wire struct — add `BatchID`)
- `daemon/internal/httpapi/handlers_v1.go:75–116` (ListIssues — add `?batch_id=`)
- `daemon/internal/httpapi/handlers_v1.go:118–184` (CreateIssue — optionally accept `batch_id`)
- `daemon/internal/httpapi/handlers_v1.go:253–390` (ApproveIssue — extract `approveOne`)
- `daemon/internal/httpapi/handlers_v1.go:395–406` (loadIssuePersona — merge into single SELECT)
- `daemon/internal/httpapi/handlers_v1.go:424–441` (`pepDenialMessage` — keep; reuse from batch path)
- `daemon/internal/httpapi/handlers_v1.go:483–508` (`resolveVariantBody` — reused per child unchanged)
- `daemon/internal/httpapi/handlers_batches.go` *(new)*
- `daemon/internal/httpapi/server.go:42–82` (router — register new routes inside auth group)
- `daemon/internal/pep/decisions.go:37–53` (Request — add `BatchID`)
- `daemon/internal/pep/pep.go:63–120` (Invoke — unchanged; audit per child)
- `daemon/scripts/seed-demo-merino-turtleneck.sh` (template for new seed script)

## Verification

1. `go test ./...` — new unit tests pass; existing tests untouched.
2. `go run ./daemon/cmd/wooagent run` — daemon starts; `004_add_batches.sql` applies cleanly against a fresh DB and against an existing populated DB.
3. `bash daemon/scripts/seed-demo-batch-meta-rewrites.sh` — creates a batch + 7 children; check via `curl GET /v1/batches` that derived counts read correctly.
4. `curl GET /v1/batches/:id` — returns batch + children with proposals; counts match issue states.
5. Approve flow:
   - Pick variant per child via UI (or build a manual JSON body).
   - `curl POST /v1/batches/:id/approve-all` with `{children: [...]}`.
   - Verify per-child `ok` flags; verify `audit_invocations` rows exist per dispatched child; verify children in DB moved to `done`.
6. Mixed-failure flow: stage one child to fail PEP (e.g., revoke ability or use a persona without scope) → `approve-all` returns 200 with per-child error; other children still committed.
7. Reject flow: `curl POST /v1/batches/:id/reject-all` after one child is already `done` → done child reports `wrong_status`, in_review children move to `rejected`.
8. Single-issue path still works (regression check): `POST /v1/issues/:id/approve` on a non-batched issue runs through the refactored `approveOne` unchanged.
