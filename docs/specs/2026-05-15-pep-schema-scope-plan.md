# PEP Phase-2 (checkSchema + checkScopeSufficiency) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Light up two of the four stubbed PEP checks — JSON Schema argument validation and Intent-vs-Scope sufficiency — preserving today's operator-Approve flow.

**Architecture:** New `Source` field on `pep.Request` gates scope sufficiency (operator-mediated calls bypass; agent-mediated calls enforce strictly). New `schemaCache` (sync.Map) backs `checkSchema`, sourcing schemas from the existing `abilities.schema_json` cache. Conservative deny on compile failure via a new `ReasonSchemaCompileError`.

**Tech Stack:** Go, `github.com/santhosh-tekuri/jsonschema/v5` (new direct dep), SQLite (existing), `sync.Map` (stdlib).

**Spec:** [`docs/specs/2026-05-15-pep-schema-scope-design.md`](./2026-05-15-pep-schema-scope-design.md)

---

## File map

| File | Responsibility |
|---|---|
| `daemon/internal/pep/decisions.go` | Types only — add `Source` enum + `Request.Source`, add `ReasonSchemaCompileError`. |
| `daemon/internal/pep/schema.go` *(new)* | Compiled-schema cache. Pure, no DB. |
| `daemon/internal/pep/pep.go` | Wire `schemas *schemaCache` into `PEP`, implement `checkSchema(ctx, req)` and `checkScopeSufficiency(req)` + rank helpers. |
| `daemon/internal/pep/schema_test.go` *(new)* | Unit tests for `schemaCache.compileOrGet`. |
| `daemon/internal/pep/checkscopesufficiency_test.go` *(new)* | Table-driven Source×Intent×Scope tests. |
| `daemon/internal/pep/checkschema_test.go` *(new)* | DB-backed checkSchema integration tests. |
| `daemon/internal/pep/pep_test.go` | Extend `abilitiesDDL`; add `Source: pep.SourceOperator` to existing happy-path tests. |
| `daemon/internal/httpapi/handlers_v1.go` | Set `Source: pep.SourceOperator` on the Approve call site; add `ReasonSchemaCompileError` case to `pepDenialMessage` + `writePEPDenial` (HTTP 500). |
| `daemon/go.mod` / `daemon/go.sum` | Add `github.com/santhosh-tekuri/jsonschema/v5` as direct dep. |

---

## Task 1: Add `Source` enum, `Request.Source`, and `ReasonSchemaCompileError`

Pure type additions. No behavior change. Establishes vocabulary for later tasks.

**Files:**
- Modify: `daemon/internal/pep/decisions.go`

- [ ] **Step 1: Add `Source` enum and `Request.Source` field**

Open `daemon/internal/pep/decisions.go`. After the `Intent` constants block (currently ending around line 27), add a new `Source` block:

```go
// Source declares who originated the call. Operator-mediated calls (the
// Approve button) bypass strict scope sufficiency because the operator IS
// the apply step the persona proposed. Agent-mediated calls enforce
// Intent <= Scope strictly. The zero value Source("") is treated as
// SourceAgent in checks — deny-by-default for any caller that forgets to
// set Source.
type Source string

const (
	SourceOperator Source = "operator"
	SourceAgent    Source = "agent"
)
```

Then add the `Source` field to the `Request` struct. Locate the `Request` struct (currently ending around line 58 with `PromptHash string`) and add the field at the bottom of the struct, just before the closing `}`:

```go
	// Source declares whether this invocation originated from an operator
	// action (Approve button) or an autonomous agent path. checkScope
	// Sufficiency uses Source to allow operator-mediated calls to apply
	// against propose-scoped abilities while keeping the gate strict for
	// agent paths. Empty Source is treated as SourceAgent.
	Source Source
```

- [ ] **Step 2: Add `ReasonSchemaCompileError` constant**

In the same file, locate the `ReasonCode` constants block (ends with `ReasonScopeInsufficient` around line 105). Add a new constant at the end of the block:

```go
	// ReasonSchemaCompileError means the ability's cached input_schema
	// could not be compiled by the JSON Schema validator, or the underlying
	// lookup failed. Distinct from ReasonInvalidArguments so operators can
	// tell cache rot / infra failure from caller mistakes. Check 3.
	ReasonSchemaCompileError ReasonCode = "schema_compile_error"
```

- [ ] **Step 3: Verify it compiles**

Run: `cd daemon && go build ./...`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add daemon/internal/pep/decisions.go
git commit -m "feat(pep): add Source enum + ReasonSchemaCompileError"
```

---

## Task 2: Add `santhosh-tekuri/jsonschema/v5` as a direct dep

**Files:**
- Modify: `daemon/go.mod`, `daemon/go.sum`

- [ ] **Step 1: Add the dependency**

Run from the repo root:
```bash
cd daemon && go get github.com/santhosh-tekuri/jsonschema/v5@latest
```
Expected: `go: added github.com/santhosh-tekuri/jsonschema/v5 vX.Y.Z` (no errors).

- [ ] **Step 2: Verify go.mod has it as a direct dep (not indirect)**

Run: `grep santhosh-tekuri daemon/go.mod`
Expected: a line `github.com/santhosh-tekuri/jsonschema/v5 vX.Y.Z` WITHOUT the `// indirect` comment.

If the line has `// indirect`, that means nothing imports it yet. Leave it as indirect — Task 6 adds the import, after which `go mod tidy` will promote it.

- [ ] **Step 3: Commit**

```bash
git add daemon/go.mod daemon/go.sum
git commit -m "chore(daemon): add santhosh-tekuri/jsonschema/v5 dep"
```

---

## Task 3: Extend test fixture DDL + set Source on existing tests

Prepares `pep_test.go` for the new checks WITHOUT changing production behavior yet (Source is set but unenforced because `checkScopeSufficiency` is still stubbed). Doing this first keeps the test suite green throughout subsequent tasks.

**Files:**
- Modify: `daemon/internal/pep/pep_test.go`

- [ ] **Step 1: Extend the inline `abilitiesDDL` constant with `schema_json` + `schema_hash` columns**

Locate the `abilitiesDDL` constant (around line 99). Replace it with:

```go
const abilitiesDDL = `
CREATE TABLE abilities (
    name        TEXT PRIMARY KEY,
    trust_state TEXT NOT NULL DEFAULT 'new',
    revoked_at  TEXT,
    schema_json TEXT,
    schema_hash TEXT
);`
```

- [ ] **Step 2: Set `Source: pep.SourceOperator` on existing happy-path tests**

In `daemon/internal/pep/pep_test.go`, add `Source: SourceOperator,` to each of these `pep.Request` literals (they're inside test functions, package is `pep`, so the unqualified name works):

- `TestInvoke_AllowedSuccess` (around line 123)
- `TestInvoke_OperatorTrustedBypassesTrustCheck` (around line 211)
- `TestInvoke_MCPCallError` (around line 227)
- `TestInvoke_NoMCPClient` (around line 254)

Example for `TestInvoke_AllowedSuccess`:

```go
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 819, "description": "x"},
		Intent:  IntentApply,
		Source:  SourceOperator,
		IssueID: "abc",
	})
```

Do NOT add Source to:
- `TestInvoke_DeniedAbilityUnapproved` — trust check denies first, scope never runs.
- `TestInvoke_DeniedPersonaForbidden` — persona check denies first.

- [ ] **Step 3: Run tests, confirm still green**

Run: `cd daemon && go test ./internal/pep/...`
Expected: all tests PASS (Source field exists but no check enforces it yet).

- [ ] **Step 4: Commit**

```bash
git add daemon/internal/pep/pep_test.go
git commit -m "test(pep): extend abilitiesDDL and set Source on happy-path tests"
```

---

## Task 4: Set `Source: pep.SourceOperator` on the Approve call site

Production wiring. One-line change. Has no behavioral effect until Task 5 enforces the check.

**Files:**
- Modify: `daemon/internal/httpapi/handlers_v1.go:494`

- [ ] **Step 1: Add the Source field on the Approve `pep.Request` literal**

Open `daemon/internal/httpapi/handlers_v1.go`. Locate the `pep.Request` literal at line 494. Add `Source: pep.SourceOperator,` after `Intent`:

```go
	decision, mcpRes, invokeErr := s.pep.Invoke(ctx, pep.Request{
		Persona: persona,
		Ability: dispatch.ability,
		Args:    params,
		Intent:  pep.IntentApply,
		Source:  pep.SourceOperator,
		IssueID: issueID,
		BatchID: batchIDStr,
	})
```

- [ ] **Step 2: Verify it compiles + httpapi tests pass**

Run: `cd daemon && go test ./internal/httpapi/...`
Expected: all tests PASS.

- [ ] **Step 3: Commit**

```bash
git add daemon/internal/httpapi/handlers_v1.go
git commit -m "feat(httpapi): tag Approve as SourceOperator for PEP scope checks"
```

---

## Task 5: TDD `checkScopeSufficiency`

Write the failing tests first, watch them fail (current stub returns `""` for every input), then implement.

**Files:**
- Create: `daemon/internal/pep/checkscopesufficiency_test.go`
- Modify: `daemon/internal/pep/pep.go`

- [ ] **Step 1: Write the failing table-driven test**

Create `daemon/internal/pep/checkscopesufficiency_test.go`:

```go
package pep

import (
	"testing"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
)

// pepWithManifest builds a PEP that only has its manifest field populated —
// checkScopeSufficiency doesn't touch the DB, MCP, or audit writer, so the
// minimum useful fixture is just a Lookup.
func pepWithManifest(t *testing.T, entries []manifest.Entry) *PEP {
	t.Helper()
	lookup, err := manifest.NewLookup(&manifest.Manifest{Version: 1, Entries: entries})
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	return &PEP{manifest: lookup}
}

func TestCheckScopeSufficiency_Table(t *testing.T) {
	entry := manifest.Entry{
		Ability:        "test/ability",
		NamespaceOwner: "test",
		SchemaHash:     "sha256:test",
		Personas:       []manifest.Persona{manifest.PersonaMarketing},
	}

	cases := []struct {
		name      string
		source    Source
		intent    Intent
		scope     manifest.Scope
		inMani    bool
		wantReason ReasonCode
	}{
		{"operator-apply-against-propose", SourceOperator, IntentApply, manifest.ScopePropose, true, ""},
		{"operator-apply-against-read", SourceOperator, IntentApply, manifest.ScopeRead, true, ""},
		{"agent-apply-against-apply", SourceAgent, IntentApply, manifest.ScopeApply, true, ""},
		{"agent-apply-against-propose", SourceAgent, IntentApply, manifest.ScopePropose, true, ReasonScopeInsufficient},
		{"agent-propose-against-propose", SourceAgent, IntentPropose, manifest.ScopePropose, true, ""},
		{"agent-propose-against-read", SourceAgent, IntentPropose, manifest.ScopeRead, true, ReasonScopeInsufficient},
		{"agent-read-against-read", SourceAgent, IntentRead, manifest.ScopeRead, true, ""},
		{"agent-read-against-propose", SourceAgent, IntentRead, manifest.ScopePropose, true, ""},
		{"unset-source-apply-propose", Source(""), IntentApply, manifest.ScopePropose, true, ReasonScopeInsufficient},
		{"unset-source-read-read", Source(""), IntentRead, manifest.ScopeRead, true, ReasonScopeInsufficient},
		{"agent-unset-intent-apply-scope", SourceAgent, Intent(""), manifest.ScopeApply, true, ReasonScopeInsufficient},
		{"no-manifest-entry-passes", SourceAgent, IntentApply, manifest.ScopePropose, false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := entry
			e.Scope = tc.scope
			entries := []manifest.Entry{e}
			req := Request{Ability: "test/ability", Persona: manifest.PersonaMarketing, Intent: tc.intent, Source: tc.source}
			if !tc.inMani {
				entries = nil
				req.Ability = "some/other-ability"
			}
			p := pepWithManifest(t, entries)
			got := p.checkScopeSufficiency(req)
			if got != tc.wantReason {
				t.Errorf("got reason %q, want %q", got, tc.wantReason)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test, confirm it fails**

Run: `cd daemon && go test ./internal/pep/ -run TestCheckScopeSufficiency_Table -v`

Expected: multiple subtests FAIL — every case that expects `ReasonScopeInsufficient` will see `""` (the stub passes through everything).

- [ ] **Step 3: Implement rank helpers and `checkScopeSufficiency`**

Open `daemon/internal/pep/pep.go`. Replace the stubbed `checkScopeSufficiency` (currently at lines 227-234) with:

```go
// checkScopeSufficiency — Check 6. For agent-originated calls, the Intent
// must not exceed the manifest entry's Scope (read < propose < apply).
// Operator-originated calls bypass this gate: the operator IS the apply step
// the persona proposed. Abilities without a manifest entry (operator-
// approved at runtime) pass through; their scoping is the Phase-2 admin
// surface's job.
func (p *PEP) checkScopeSufficiency(req Request) ReasonCode {
	entry := p.manifest.Get(req.Ability)
	if entry == nil {
		return ""
	}
	if req.Source == SourceOperator {
		return ""
	}
	ir := intentRank(req.Intent)
	sr := scopeRank(entry.Scope)
	if ir == 0 || ir > sr {
		return ReasonScopeInsufficient
	}
	return ""
}

// intentRank returns the authority level of an Intent. Higher number = more
// privileged. Zero means "unknown" — treated as too privileged to pass any
// check so an unset Intent never quietly bypasses scope sufficiency.
func intentRank(i Intent) int {
	switch i {
	case IntentRead:
		return 1
	case IntentPropose:
		return 2
	case IntentApply:
		return 3
	}
	return 0
}

// scopeRank mirrors intentRank for manifest.Scope.
func scopeRank(s manifest.Scope) int {
	switch s {
	case manifest.ScopeRead:
		return 1
	case manifest.ScopePropose:
		return 2
	case manifest.ScopeApply:
		return 3
	}
	return 0
}
```

- [ ] **Step 4: Run the test, confirm it passes**

Run: `cd daemon && go test ./internal/pep/ -run TestCheckScopeSufficiency_Table -v`
Expected: all subtests PASS.

- [ ] **Step 5: Run the full PEP test suite to confirm no regressions**

Run: `cd daemon && go test ./internal/pep/...`
Expected: all tests PASS. (The existing `TestInvoke_AllowedSuccess` etc. now exercise the strict check via `Source: SourceOperator`, which bypasses cleanly.)

- [ ] **Step 6: Commit**

```bash
git add daemon/internal/pep/pep.go daemon/internal/pep/checkscopesufficiency_test.go
git commit -m "feat(pep): implement checkScopeSufficiency (Source-gated)"
```

---

## Task 6: Create `schemaCache` with unit tests

Pure, in-memory compilation cache. TDD style — write a few focused tests, then implement.

**Files:**
- Create: `daemon/internal/pep/schema.go`
- Create: `daemon/internal/pep/schema_test.go`

- [ ] **Step 1: Write the failing tests**

Create `daemon/internal/pep/schema_test.go`:

```go
package pep

import (
	"testing"
)

const validEnvelope = `{
  "name": "test/ability",
  "input_schema": {
    "type": "object",
    "properties": {"id": {"type": "integer"}},
    "required": ["id"]
  }
}`

const envelopeWithoutInputSchema = `{"name": "test/ability"}`

const malformedSchemaEnvelope = `{
  "name": "test/ability",
  "input_schema": {"type": "not-a-real-type"}
}`

func TestSchemaCache_CompilesValidSchema(t *testing.T) {
	c := &schemaCache{}
	s, err := c.compileOrGet("test/ability", "h1", validEnvelope)
	if err != nil {
		t.Fatalf("compileOrGet: %v", err)
	}
	if s == nil {
		t.Fatal("expected compiled schema, got nil")
	}
	if err := s.Validate(map[string]any{"id": 1}); err != nil {
		t.Errorf("expected valid args to pass, got %v", err)
	}
	if err := s.Validate(map[string]any{}); err == nil {
		t.Error("expected missing required to fail")
	}
}

func TestSchemaCache_ReturnsNilWhenNoInputSchema(t *testing.T) {
	c := &schemaCache{}
	s, err := c.compileOrGet("test/ability", "h1", envelopeWithoutInputSchema)
	if err != nil {
		t.Fatalf("compileOrGet: %v", err)
	}
	if s != nil {
		t.Errorf("expected nil schema for envelope without input_schema, got %#v", s)
	}
}

func TestSchemaCache_ErrorsOnMalformedSchema(t *testing.T) {
	c := &schemaCache{}
	_, err := c.compileOrGet("test/ability", "h1", malformedSchemaEnvelope)
	if err == nil {
		t.Fatal("expected compile error for invalid schema type")
	}
}

func TestSchemaCache_CachesByNameAndHash(t *testing.T) {
	c := &schemaCache{}
	s1, err := c.compileOrGet("test/ability", "h1", validEnvelope)
	if err != nil {
		t.Fatalf("first compileOrGet: %v", err)
	}
	// Same key → same compiled instance.
	s2, err := c.compileOrGet("test/ability", "h1", validEnvelope)
	if err != nil {
		t.Fatalf("second compileOrGet: %v", err)
	}
	if s1 != s2 {
		t.Error("expected same *jsonschema.Schema pointer for same (name, hash) key")
	}
	// Different hash → new compilation, different pointer.
	s3, err := c.compileOrGet("test/ability", "h2", validEnvelope)
	if err != nil {
		t.Fatalf("third compileOrGet: %v", err)
	}
	if s1 == s3 {
		t.Error("expected different *jsonschema.Schema pointer for different hash")
	}
}
```

- [ ] **Step 2: Run the test, confirm it fails (file doesn't exist yet)**

Run: `cd daemon && go test ./internal/pep/ -run TestSchemaCache -v`
Expected: build error — `undefined: schemaCache`.

- [ ] **Step 3: Implement `schemaCache`**

Create `daemon/internal/pep/schema.go`:

```go
package pep

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// schemaCache compiles JSON Schemas once per (ability_name, schema_hash) pair
// and reuses the compiled form on subsequent Invokes. Compile is O(schema
// size); validation is O(args size). Moving compile out of the hot path is
// the whole point of the cache.
//
// Backed by sync.Map for concurrent Invoke safety. No eviction in V1:
// manifest is ~20 entries, operator-approved abilities scale slowly,
// compiled schemas are kilobytes.
type schemaCache struct {
	entries sync.Map // map[string]*jsonschema.Schema, key = name + ":" + hash
}

// compileOrGet returns the compiled input_schema for (name, hash). schemaJSON
// is the full canonicalized envelope from abilities.schema_json — the
// function extracts the "input_schema" sub-document before compiling.
// Returns (nil, nil) when the envelope has no input_schema (caller should
// pass through validation).
func (c *schemaCache) compileOrGet(name, hash, schemaJSON string) (*jsonschema.Schema, error) {
	key := name + ":" + hash
	if v, ok := c.entries.Load(key); ok {
		return v.(*jsonschema.Schema), nil
	}
	var envelope struct {
		InputSchema json.RawMessage `json:"input_schema"`
	}
	if err := json.Unmarshal([]byte(schemaJSON), &envelope); err != nil {
		return nil, fmt.Errorf("decode schema envelope: %w", err)
	}
	if len(envelope.InputSchema) == 0 {
		return nil, nil
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(key, bytes.NewReader(envelope.InputSchema)); err != nil {
		return nil, fmt.Errorf("add resource: %w", err)
	}
	s, err := compiler.Compile(key)
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}
	c.entries.Store(key, s)
	return s, nil
}
```

- [ ] **Step 4: Run go mod tidy to promote the dep to direct**

Run: `cd daemon && go mod tidy`
Expected: `go.mod` now lists `github.com/santhosh-tekuri/jsonschema/v5` WITHOUT `// indirect`.

- [ ] **Step 5: Run the tests, confirm they pass**

Run: `cd daemon && go test ./internal/pep/ -run TestSchemaCache -v`
Expected: 4 subtests PASS.

- [ ] **Step 6: Commit**

```bash
git add daemon/internal/pep/schema.go daemon/internal/pep/schema_test.go daemon/go.mod daemon/go.sum
git commit -m "feat(pep): schemaCache for compiled JSON Schemas"
```

---

## Task 7: TDD `checkSchema` — DB integration + Invoke wiring

End-to-end check that pulls the schema from `abilities.schema_json`, validates `req.Args`, and uses the cache.

**Files:**
- Create: `daemon/internal/pep/checkschema_test.go`
- Modify: `daemon/internal/pep/pep.go`

- [ ] **Step 1: Add the schema cache to the `PEP` struct + `New()`**

Open `daemon/internal/pep/pep.go`. Edit the `PEP` struct definition (currently around lines 22-28):

```go
type PEP struct {
	manifest *manifest.Lookup
	mcp      MCPClient
	audit    *auditWriter
	db       *sql.DB
	schemas  *schemaCache
}
```

Then edit `New()` (currently lines 33-40) to initialize it:

```go
func New(m *manifest.Lookup, mcpClient MCPClient, db *sql.DB) *PEP {
	return &PEP{
		manifest: m,
		mcp:      mcpClient,
		audit:    newAuditWriter(db),
		db:       db,
		schemas:  &schemaCache{},
	}
}
```

- [ ] **Step 2: Update `pepWithManifest` test helper to initialize the cache**

Open `daemon/internal/pep/checkscopesufficiency_test.go` (created in Task 5). Edit the `pepWithManifest` helper so it also sets `schemas`:

```go
func pepWithManifest(t *testing.T, entries []manifest.Entry) *PEP {
	t.Helper()
	lookup, err := manifest.NewLookup(&manifest.Manifest{Version: 1, Entries: entries})
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	return &PEP{manifest: lookup, schemas: &schemaCache{}}
}
```

- [ ] **Step 3: Write the failing checkSchema tests**

Create `daemon/internal/pep/checkschema_test.go`:

```go
package pep

import (
	"context"
	"database/sql"
	"testing"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
)

// schemaEnvelopeRequiringID — schema_json shape stored by the abilities
// canonicalizer: a top-level object with an "input_schema" sub-document.
const schemaEnvelopeRequiringID = `{
  "name": "wooagent-products/update",
  "input_schema": {
    "type": "object",
    "properties": {"id": {"type": "integer"}, "description": {"type": "string"}},
    "required": ["id"]
  }
}`

const schemaEnvelopeMalformed = `{
  "name": "wooagent-products/update",
  "input_schema": {"type": "not-a-real-type"}
}`

func insertAbility(t *testing.T, db *sql.DB, name, trustState, schemaJSON, schemaHash string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO abilities(name, trust_state, schema_json, schema_hash) VALUES(?, ?, ?, ?)`,
		name, trustState, schemaJSON, schemaHash,
	)
	if err != nil {
		t.Fatalf("insert ability: %v", err)
	}
}

func TestCheckSchema_PassThroughWhenNoDBRow(t *testing.T) {
	mcpc := &fakeMCP{result: mcp.ToolCallResult{Content: []mcp.ContentPart{{Type: "text", Text: `{"ok":true}`}}}}
	p, _ := newTestPEP(t, mcpc)
	// No row inserted for this ability; manifest entry exists.
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1},
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !dec.Allowed {
		t.Errorf("expected allowed (pass-through), got denied: %s", dec.Reason)
	}
}

func TestCheckSchema_PassThroughWhenSchemaJSONEmpty(t *testing.T) {
	mcpc := &fakeMCP{result: mcp.ToolCallResult{Content: []mcp.ContentPart{{Type: "text", Text: `{"ok":true}`}}}}
	p, db := newTestPEP(t, mcpc)
	insertAbility(t, db, "wooagent-products/update", "trusted", "", "")
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1},
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !dec.Allowed {
		t.Errorf("expected allowed (empty schema_json passes through), got: %s", dec.Reason)
	}
}

func TestCheckSchema_AllowsValidArgs(t *testing.T) {
	mcpc := &fakeMCP{result: mcp.ToolCallResult{Content: []mcp.ContentPart{{Type: "text", Text: `{"ok":true}`}}}}
	p, db := newTestPEP(t, mcpc)
	insertAbility(t, db, "wooagent-products/update", "trusted", schemaEnvelopeRequiringID, "h1")
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1, "description": "x"},
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !dec.Allowed {
		t.Errorf("expected allowed for valid args, got: %s", dec.Reason)
	}
}

func TestCheckSchema_DeniesMissingRequired(t *testing.T) {
	mcpc := &fakeMCP{}
	p, db := newTestPEP(t, mcpc)
	insertAbility(t, db, "wooagent-products/update", "trusted", schemaEnvelopeRequiringID, "h1")
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"description": "no id here"},
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if dec.Allowed {
		t.Fatal("expected denied for missing required field")
	}
	if dec.Reason != ReasonInvalidArguments {
		t.Errorf("reason = %q, want %q", dec.Reason, ReasonInvalidArguments)
	}
	if mcpc.calls != 0 {
		t.Errorf("denied call should not reach mcp, got %d calls", mcpc.calls)
	}
}

func TestCheckSchema_DeniesWrongType(t *testing.T) {
	mcpc := &fakeMCP{}
	p, db := newTestPEP(t, mcpc)
	insertAbility(t, db, "wooagent-products/update", "trusted", schemaEnvelopeRequiringID, "h1")
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": "not-an-integer"},
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if dec.Allowed {
		t.Fatal("expected denied for wrong type")
	}
	if dec.Reason != ReasonInvalidArguments {
		t.Errorf("reason = %q, want %q", dec.Reason, ReasonInvalidArguments)
	}
}

func TestCheckSchema_DeniesOnCompileFailure(t *testing.T) {
	mcpc := &fakeMCP{}
	p, db := newTestPEP(t, mcpc)
	insertAbility(t, db, "wooagent-products/update", "trusted", schemaEnvelopeMalformed, "h1")
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1},
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if dec.Allowed {
		t.Fatal("expected denied for malformed schema")
	}
	if dec.Reason != ReasonSchemaCompileError {
		t.Errorf("reason = %q, want %q", dec.Reason, ReasonSchemaCompileError)
	}
}

// Sentinel: after the first validating call, mutate schema_json in the DB
// without changing schema_hash. The second call should still validate
// against the original schema — proving the compiled form was cached.
func TestCheckSchema_UsesCachedCompiledSchema(t *testing.T) {
	mcpc := &fakeMCP{result: mcp.ToolCallResult{Content: []mcp.ContentPart{{Type: "text", Text: `{"ok":true}`}}}}
	p, db := newTestPEP(t, mcpc)
	insertAbility(t, db, "wooagent-products/update", "trusted", schemaEnvelopeRequiringID, "h1")

	// First call: warm the cache.
	if _, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1},
		Intent:  IntentApply,
		Source:  SourceOperator,
	}); err != nil {
		t.Fatalf("warm call: %v", err)
	}

	// Mutate schema_json to require a different field; keep hash the same.
	mutated := `{"name":"x","input_schema":{"type":"object","required":["different"]}}`
	if _, err := db.ExecContext(context.Background(),
		`UPDATE abilities SET schema_json = ? WHERE name = ?`,
		mutated, "wooagent-products/update",
	); err != nil {
		t.Fatalf("mutate: %v", err)
	}

	// Second call with original args; should still pass because the cache
	// holds the compiled (id-required) schema.
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1},
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if !dec.Allowed {
		t.Errorf("expected allowed (cached schema), got: %s", dec.Reason)
	}
}
```

- [ ] **Step 4: Run the test, confirm it fails**

Run: `cd daemon && go test ./internal/pep/ -run TestCheckSchema -v`
Expected: tests run but most subtests FAIL — the stub `checkSchema` returns `""` so denial-expected cases see allowed.

- [ ] **Step 5: Implement `checkSchema`**

Open `daemon/internal/pep/pep.go`. Replace the stubbed `checkSchema` (currently lines 203-210) with:

```go
// checkSchema — Check 3. Validate req.Args against the ability's cached
// input_schema. Pass-through when no cached schema is available (companion
// plugin validates at the WP boundary one hop later). Conservative deny on
// any compile failure or DB infra error.
func (p *PEP) checkSchema(ctx context.Context, req Request) ReasonCode {
	var schemaJSON, schemaHash sql.NullString
	err := p.db.QueryRowContext(ctx,
		`SELECT schema_json, schema_hash FROM abilities WHERE name = ?`,
		req.Ability,
	).Scan(&schemaJSON, &schemaHash)
	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}
	if err != nil {
		return ReasonSchemaCompileError
	}
	if !schemaJSON.Valid || schemaJSON.String == "" {
		return ""
	}
	s, err := p.schemas.compileOrGet(req.Ability, schemaHash.String, schemaJSON.String)
	if err != nil {
		return ReasonSchemaCompileError
	}
	if s == nil {
		return ""
	}
	if err := s.Validate(req.Args); err != nil {
		return ReasonInvalidArguments
	}
	return ""
}
```

- [ ] **Step 6: Update the `Invoke` pipeline to pass `ctx` to `checkSchema`**

Open `daemon/internal/pep/pep.go`. Locate the `Invoke` method (around line 70). Change line 85 from:

```go
	if reason := p.checkSchema(req); reason != "" {
```

to:

```go
	if reason := p.checkSchema(ctx, req); reason != "" {
```

- [ ] **Step 7: Run the test, confirm it passes**

Run: `cd daemon && go test ./internal/pep/ -run TestCheckSchema -v`
Expected: all subtests PASS.

- [ ] **Step 8: Run the entire pep package test suite**

Run: `cd daemon && go test ./internal/pep/...`
Expected: all tests PASS.

- [ ] **Step 9: Commit**

```bash
git add daemon/internal/pep/pep.go daemon/internal/pep/checkschema_test.go daemon/internal/pep/checkscopesufficiency_test.go
git commit -m "feat(pep): implement checkSchema with DB-cached input_schema"
```

---

## Task 8: Wire `ReasonSchemaCompileError` into HTTP boundary

Adds the case to `pepDenialMessage` (user-facing string) and `writePEPDenial` (HTTP status).

**Files:**
- Modify: `daemon/internal/httpapi/handlers_v1.go`

- [ ] **Step 1: Add the case to `writePEPDenial`**

Open `daemon/internal/httpapi/handlers_v1.go`. Locate `writePEPDenial` (around line 588). Add a `ReasonSchemaCompileError` case to the switch — distinct because it's an infra failure (500), not a caller-input failure (422):

```go
func writePEPDenial(w http.ResponseWriter, reason pep.ReasonCode) {
	switch reason {
	case pep.ReasonAbilityUnapproved, pep.ReasonPersonaForbidden, pep.ReasonScopeInsufficient:
		writeError(w, http.StatusForbidden, string(reason), pepDenialMessage(reason))
	case pep.ReasonInvalidArguments, pep.ReasonPolicyViolation:
		writeError(w, http.StatusUnprocessableEntity, string(reason), pepDenialMessage(reason))
	case pep.ReasonBudgetExceeded:
		writeError(w, http.StatusTooManyRequests, string(reason), pepDenialMessage(reason))
	case pep.ReasonSchemaCompileError:
		writeError(w, http.StatusInternalServerError, string(reason), pepDenialMessage(reason))
	default:
		writeError(w, http.StatusForbidden, "permission_denied", "PEP denied the call")
	}
}
```

- [ ] **Step 2: Add the case to `pepDenialMessage`**

Same file, locate `pepDenialMessage` (around line 601). Add the case:

```go
	case pep.ReasonSchemaCompileError:
		return "the local schema cache for this ability is invalid — try re-discovering abilities for this store"
```

Place it before the `default` case in the switch. The complete switch after editing:

```go
func pepDenialMessage(reason pep.ReasonCode) string {
	switch reason {
	case pep.ReasonAbilityUnapproved:
		return "ability is not in the trusted manifest"
	case pep.ReasonPersonaForbidden:
		return "this persona is not permitted to invoke this ability"
	case pep.ReasonScopeInsufficient:
		return "intended action exceeds the ability's authorized scope"
	case pep.ReasonInvalidArguments:
		return "arguments did not validate against the ability's input schema"
	case pep.ReasonPolicyViolation:
		return "arguments tripped an operator-configured policy"
	case pep.ReasonBudgetExceeded:
		return "persona is over its daily budget"
	case pep.ReasonSchemaCompileError:
		return "the local schema cache for this ability is invalid — try re-discovering abilities for this store"
	default:
		return "PEP denied the call"
	}
}
```

- [ ] **Step 3: Run httpapi tests**

Run: `cd daemon && go test ./internal/httpapi/...`
Expected: all tests PASS (no existing tests assert on the new code paths; we're just confirming nothing else broke).

- [ ] **Step 4: Commit**

```bash
git add daemon/internal/httpapi/handlers_v1.go
git commit -m "feat(httpapi): map ReasonSchemaCompileError to 500 with recovery hint"
```

---

## Task 9: Full verification

Final pass: full daemon test suite, vet, and a quick visual scan of the new PEP comments to make sure the four TODO markers we set out to remove are gone.

**Files:** none

- [ ] **Step 1: Run the full daemon test suite**

Run: `cd daemon && go test ./...`
Expected: ALL tests PASS across all daemon packages.

- [ ] **Step 2: Run go vet**

Run: `cd daemon && go vet ./...`
Expected: no output (no vet warnings).

- [ ] **Step 3: Confirm the two `TODO(phase-2)` markers we addressed are gone**

Run: `grep -n "TODO(phase-2)" daemon/internal/pep/pep.go`
Expected: TWO matches remaining (`checkPolicy` and `checkBudget`), not four. The schema + scope-sufficiency TODOs are gone.

- [ ] **Step 4: Confirm `santhosh-tekuri/jsonschema/v5` is a direct dep**

Run: `grep santhosh-tekuri daemon/go.mod`
Expected: a line without `// indirect`.

- [ ] **Step 5: Final commit (only if anything was tidied up)**

If `go mod tidy` etc. produced any deltas in this task, commit them:

```bash
git add -A
git status   # confirm what's staged
git commit -m "chore(daemon): tidy after PEP Phase-2 schema + scope work"
```

If `git status` shows nothing staged, skip this step.

---

## Spec coverage check

Every requirement from the spec maps to a task:

| Spec section | Task(s) |
|---|---|
| Data model: `Source` enum + `Request.Source` | Task 1 |
| Data model: `ReasonSchemaCompileError` | Task 1 |
| Approve call site sets `Source: SourceOperator` | Task 4 |
| `checkScopeSufficiency` flow | Task 5 |
| `intentRank` / `scopeRank` helpers | Task 5 |
| `schemaCache` shape | Task 6 |
| `checkSchema` flow (DB read, pass-through cases) | Task 7 |
| `Invoke` passes `ctx` to `checkSchema` | Task 7 step 6 |
| `schemaCache` wired into `PEP` struct | Task 7 step 1 |
| New test files for both checks | Tasks 5, 6, 7 |
| Existing test updates (`abilitiesDDL` + Source) | Task 3 |
| HTTP boundary mappings (500 + denial message) | Task 8 |
| `santhosh-tekuri/jsonschema/v5` direct dep | Task 2 (+ promoted by `go mod tidy` in Task 6) |
| Verification (full test suite, vet, TODO scan) | Task 9 |

Non-goals from the spec (`checkPolicy`, `checkBudget`, manifest changes, DB migrations, retries, cache eviction, JSON Schema draft pinning, operator-scope admin surface) are explicitly excluded from this plan.
