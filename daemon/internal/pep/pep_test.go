package pep

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
)

// fakeMCP implements MCPClient with scriptable responses. Use for happy-path
// and error tests without spinning up a real WP server.
type fakeMCP struct {
	initErr error
	callErr error
	result  mcp.ToolCallResult
	calls   int
}

func (f *fakeMCP) Initialize(_ context.Context) (mcp.ServerInfo, error) {
	return mcp.ServerInfo{}, f.initErr
}

func (f *fakeMCP) CallTool(_ context.Context, _ string, _ any) (mcp.ToolCallResult, error) {
	f.calls++
	if f.callErr != nil {
		return mcp.ToolCallResult{}, f.callErr
	}
	return f.result, nil
}

// newTestPEP wires a PEP against an in-memory SQLite with the audit table
// already applied. The fixture manifest has one pre-signed ability that the
// marketing persona is allowed to invoke; everything else trips one of the
// two V1 checks.
func newTestPEP(t *testing.T, mcpc MCPClient) (*PEP, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(auditDDL); err != nil {
		t.Fatalf("apply audit ddl: %v", err)
	}
	if _, err := db.Exec(abilitiesDDL); err != nil {
		t.Fatalf("apply abilities ddl: %v", err)
	}
	if _, err := db.Exec(budgetUsageDDL); err != nil {
		t.Fatalf("apply budget ddl: %v", err)
	}

	m := &manifest.Manifest{
		Version: 1,
		Entries: []manifest.Entry{{
			Ability:        "wooagent-products/update",
			NamespaceOwner: "test",
			SchemaHash:     "sha256:test",
			Scope:          manifest.ScopePropose,
			Reversibility:  0.6,
			Personas:       []manifest.Persona{manifest.PersonaMarketing},
		}},
	}
	lookup, err := manifest.NewLookup(m)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	p := New(lookup, mcpc, db, NewBudgetGate(db, DefaultThresholds()))
	// Keep test stderr clean; specific tests overwrite p.logger to capture
	// warn-log output for assertions.
	p.logger = nullLogger
	return p, db
}

// auditDDL is the minimal DDL for the audit table — same shape as the
// 003_audit_invocations.sql migration, inlined here so tests don't depend on
// the store package's migration runner.
const auditDDL = `
CREATE TABLE audit_invocations (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id         TEXT,
    task_id         TEXT,
    step_id         TEXT,
    issue_id        TEXT,
    persona         TEXT NOT NULL,
    model           TEXT,
    prompt_hash     TEXT,
    ability         TEXT NOT NULL,
    args_hash       TEXT NOT NULL,
    cap_token_id    TEXT,
    intent          TEXT NOT NULL,
    outcome         TEXT NOT NULL,
    denial_reason   TEXT,
    created_at      TEXT NOT NULL,
    completed_at    TEXT
);`

// abilitiesDDL is the minimal DDL for the abilities table — same shape as the
// 004_abilities.sql migration (Tasks 1–2), inlined here so PEP tests don't
// depend on the full migration runner.
const abilitiesDDL = `
CREATE TABLE abilities (
    name        TEXT PRIMARY KEY,
    trust_state TEXT NOT NULL DEFAULT 'new',
    revoked_at  TEXT,
    schema_json TEXT,
    schema_hash TEXT
);`

// auditRow is a thin read helper so tests can assert on the row that was
// written. Returns outcome + denial_reason for the row at id.
func auditRow(t *testing.T, db *sql.DB, id int64) (string, string) {
	t.Helper()
	var outcome string
	var denial sql.NullString
	if err := db.QueryRow(
		`SELECT outcome, denial_reason FROM audit_invocations WHERE id = ?`, id,
	).Scan(&outcome, &denial); err != nil {
		t.Fatalf("read audit row: %v", err)
	}
	return outcome, denial.String
}

func TestInvoke_AllowedSuccess(t *testing.T) {
	mcpc := &fakeMCP{result: mcp.ToolCallResult{Content: []mcp.ContentPart{{Type: "text", Text: `{"success":true}`}}}}
	p, db := newTestPEP(t, mcpc)
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 819, "description": "x"},
		Intent:  IntentApply,
		Source:  SourceOperator,
		IssueID: "abc",
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !dec.Allowed {
		t.Fatalf("expected allowed, got denied: %s", dec.Reason)
	}
	if mcpc.calls != 1 {
		t.Errorf("expected 1 mcp call, got %d", mcpc.calls)
	}
	outcome, _ := auditRow(t, db, dec.AuditID)
	if outcome != string(OutcomeSuccess) {
		t.Errorf("audit outcome = %q, want success", outcome)
	}
}

func TestInvoke_DeniedAbilityUnapproved(t *testing.T) {
	mcpc := &fakeMCP{}
	p, db := newTestPEP(t, mcpc)
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "evil-plugin/drop-database",
		Intent:  IntentApply,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if dec.Allowed {
		t.Fatal("expected denied")
	}
	if dec.Reason != ReasonAbilityUnapproved {
		t.Errorf("reason = %q, want %q", dec.Reason, ReasonAbilityUnapproved)
	}
	if mcpc.calls != 0 {
		t.Errorf("denied call should not reach mcp, got %d calls", mcpc.calls)
	}
	outcome, denial := auditRow(t, db, dec.AuditID)
	if outcome != string(OutcomeDenied) {
		t.Errorf("audit outcome = %q, want denied", outcome)
	}
	if denial != string(ReasonAbilityUnapproved) {
		t.Errorf("audit denial = %q, want %q", denial, ReasonAbilityUnapproved)
	}
}

func TestInvoke_DeniedPersonaForbidden(t *testing.T) {
	mcpc := &fakeMCP{}
	p, db := newTestPEP(t, mcpc)
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaPricing, // not on the manifest entry's list
		Ability: "wooagent-products/update",
		Intent:  IntentApply,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if dec.Allowed {
		t.Fatal("expected denied")
	}
	if dec.Reason != ReasonPersonaForbidden {
		t.Errorf("reason = %q, want %q", dec.Reason, ReasonPersonaForbidden)
	}
	if mcpc.calls != 0 {
		t.Errorf("denied call should not reach mcp, got %d calls", mcpc.calls)
	}
	outcome, _ := auditRow(t, db, dec.AuditID)
	if outcome != string(OutcomeDenied) {
		t.Errorf("audit outcome = %q, want denied", outcome)
	}
}

func TestInvoke_OperatorTrustedBypassesTrustCheck(t *testing.T) {
	mcpc := &fakeMCP{result: mcp.ToolCallResult{Content: []mcp.ContentPart{{Type: "text", Text: `{"success":true}`}}}}
	p, db := newTestPEP(t, mcpc)
	// Insert a row with trust_state='trusted' to simulate the operator
	// approving this ability via the Skills UI. No manifest entry.
	if _, err := db.ExecContext(context.Background(),
		`INSERT INTO abilities(name, trust_state) VALUES(?, ?)`,
		"custom-plugin/weird-ability", "trusted",
	); err != nil {
		t.Fatalf("insert ability row: %v", err)
	}
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "custom-plugin/weird-ability",
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !dec.Allowed {
		t.Fatalf("operator-trusted ability should pass trust check, got %s", dec.Reason)
	}
}

func TestInvoke_MCPCallError(t *testing.T) {
	mcpc := &fakeMCP{callErr: errors.New("boom")}
	p, db := newTestPEP(t, mcpc)
	dec, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if err == nil {
		t.Fatal("expected error from mcp call")
	}
	// On infra failure the decision struct is zero — caller branches on err.
	if dec.Allowed {
		t.Error("decision should not be allowed when mcp call fails")
	}
	// But the audit row should exist and be marked mcp_error. Find it by
	// scanning since dec.AuditID is zero in this path.
	var outcome string
	if err := db.QueryRow(
		`SELECT outcome FROM audit_invocations ORDER BY id DESC LIMIT 1`,
	).Scan(&outcome); err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if outcome != string(OutcomeMCPError) {
		t.Errorf("audit outcome = %q, want mcp_error", outcome)
	}
}

func TestInvoke_NoMCPClient(t *testing.T) {
	p, db := newTestPEP(t, nil)
	_, _, err := p.Invoke(context.Background(), Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Intent:  IntentApply,
		Source:  SourceOperator,
	})
	if !errors.Is(err, ErrMCPNotConfigured) {
		t.Fatalf("expected ErrMCPNotConfigured, got %v", err)
	}
	var outcome string
	if err := db.QueryRow(
		`SELECT outcome FROM audit_invocations ORDER BY id DESC LIMIT 1`,
	).Scan(&outcome); err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if outcome != string(OutcomeMCPError) {
		t.Errorf("audit outcome = %q, want mcp_error", outcome)
	}
}

func TestHashArgs_Canonical(t *testing.T) {
	a := hashArgs(map[string]any{"a": 1, "b": "two"})
	b := hashArgs(map[string]any{"b": "two", "a": 1})
	if a != b {
		t.Errorf("canonical hash should ignore key order: %s != %s", a, b)
	}
	c := hashArgs(map[string]any{"a": 1, "b": "three"})
	if a == c {
		t.Error("hash should differ when values differ")
	}
}

func TestHashArgs_EmptyArgs(t *testing.T) {
	a := hashArgs(nil)
	b := hashArgs(map[string]any{})
	if a != b {
		t.Errorf("nil and empty map should hash identically: %s != %s", a, b)
	}
}
