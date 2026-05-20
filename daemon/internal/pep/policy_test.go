package pep

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
)

// agentsDDL — minimal shape mirroring the merged migrations 001 + 011 +
// 014 + 016. Kept inline in tests so the pep package doesn't pull in
// store/migrations.
const agentsDDL = `
CREATE TABLE agents (
    persona            TEXT PRIMARY KEY,
    name               TEXT,
    model_preference   TEXT,
    enabled            INTEGER NOT NULL DEFAULT 0,
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL,
    cadence_seconds    INTEGER NOT NULL DEFAULT 21600,
    max_attempts       INTEGER NOT NULL DEFAULT 3,
    last_run_at        TEXT,
    run_budget_cents   INTEGER NOT NULL DEFAULT 1000,
    apply_hours_start  TEXT,
    apply_hours_end    TEXT
);`

func newAgentsDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(agentsDDL); err != nil {
		t.Fatalf("ddl: %v", err)
	}
	return db
}

func TestLoadAgentSettings_BothNullReturnsEmpty(t *testing.T) {
	db := newAgentsDB(t)
	if _, err := db.Exec(
		`INSERT INTO agents(persona, enabled, created_at, updated_at) VALUES('marketing', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
	); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := loadAgentSettings(context.Background(), db, manifest.PersonaMarketing)
	if err != nil {
		t.Fatalf("loadAgentSettings: %v", err)
	}
	if got.ApplyHoursStart != "" || got.ApplyHoursEnd != "" {
		t.Errorf("got %+v, want zero AgentSettings", got)
	}
}

func TestLoadAgentSettings_BothSetReturnsValues(t *testing.T) {
	db := newAgentsDB(t)
	if _, err := db.Exec(
		`INSERT INTO agents(persona, enabled, created_at, updated_at, apply_hours_start, apply_hours_end) VALUES('marketing', 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', '09:00', '17:00')`,
	); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := loadAgentSettings(context.Background(), db, manifest.PersonaMarketing)
	if err != nil {
		t.Fatalf("loadAgentSettings: %v", err)
	}
	if got.ApplyHoursStart != "09:00" || got.ApplyHoursEnd != "17:00" {
		t.Errorf("got %+v, want {09:00, 17:00}", got)
	}
}

func TestLoadAgentSettings_MissingPersonaReturnsEmpty(t *testing.T) {
	db := newAgentsDB(t)
	got, err := loadAgentSettings(context.Background(), db, manifest.PersonaMarketing)
	if err != nil {
		t.Fatalf("loadAgentSettings: %v", err)
	}
	if got.ApplyHoursStart != "" || got.ApplyHoursEnd != "" {
		t.Errorf("got %+v, want zero AgentSettings", got)
	}
}

// ---------- orchestration test helpers ----------

// defaultApplyRequest returns a Request that the test rig's fixture
// manifest + trusted abilities row admits through checks 1, 2, 3.
// Tests that want a different shape build their own.
func defaultApplyRequest() Request {
	return Request{
		Persona: manifest.PersonaMarketing,
		Ability: "wooagent-products/update",
		Args:    map[string]any{"id": 1, "description": "x"},
		Intent:  IntentApply,
		Source:  SourceOperator,
	}
}

// insertManifestAndTrust seeds the abilities + agents rows the
// orchestration tests need. Without this, checkTrustState denies
// before checkPolicy runs.
func insertManifestAndTrust(t *testing.T, p *PEP) {
	t.Helper()
	if _, err := p.db.Exec(
		`INSERT INTO abilities(name, trust_state, schema_json, schema_hash) VALUES(?, 'trusted', '', '')`,
		"wooagent-products/update",
	); err != nil {
		t.Fatalf("seed abilities row: %v", err)
	}
	if _, err := p.db.Exec(
		`INSERT INTO agents(persona, enabled, created_at, updated_at) VALUES(?, 1, ?, ?)`,
		string(manifest.PersonaMarketing), "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z",
	); err != nil {
		t.Fatalf("seed agents row: %v", err)
	}
}

// readPolicyRuleName fetches the column for a given audit_invocations
// row. Empty string when NULL.
func readPolicyRuleName(t *testing.T, db *sql.DB, auditID int64) string {
	t.Helper()
	var v sql.NullString
	if err := db.QueryRow(
		`SELECT policy_rule_name FROM audit_invocations WHERE id = ?`, auditID,
	).Scan(&v); err != nil {
		t.Fatalf("read policy_rule_name: %v", err)
	}
	if v.Valid {
		return v.String
	}
	return ""
}

// succeedResult returns a non-empty MCP result so Invoke's happy path
// doesn't bomb on mcp_empty.
func succeedResult() mcp.ToolCallResult {
	return mcp.ToolCallResult{Content: []mcp.ContentPart{{Type: "text", Text: `{"success":true}`}}}
}

// stubPolicy is a test-only Policy that always denies with the given
// rule name and reason. Used by the orchestration tests below.
type stubPolicy struct {
	name   string
	deny   bool
	reason string
	panic  bool
}

func (s *stubPolicy) Name() string { return s.name }
func (s *stubPolicy) Evaluate(_ Request, _ EvalContext) (bool, string) {
	if s.panic {
		panic("stubPolicy: deliberate test panic")
	}
	return s.deny, s.reason
}

// ---------- orchestration tests ----------

func TestCheckPolicy_EmptyPolicySetAllows(t *testing.T) {
	mcpc := &fakeMCP{result: succeedResult()}
	p, _ := newTestPEP(t, mcpc)
	p.policies = nil
	insertManifestAndTrust(t, p)

	dec, _, err := p.Invoke(context.Background(), defaultApplyRequest())
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !dec.Allowed {
		t.Errorf("empty policy set should allow; denied with %q", dec.Reason)
	}
}

func TestCheckPolicy_FirstDenyWinsOrdering(t *testing.T) {
	mcpc := &fakeMCP{}
	p, _ := newTestPEP(t, mcpc)
	p.policies = []Policy{
		&stubPolicy{name: "first_denier", deny: true, reason: "first"},
		&stubPolicy{name: "second_denier", deny: true, reason: "second"},
	}
	insertManifestAndTrust(t, p)

	dec, _, err := p.Invoke(context.Background(), defaultApplyRequest())
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if dec.Allowed {
		t.Fatal("expected deny from first policy")
	}
	if dec.Reason != ReasonPolicyViolation {
		t.Errorf("reason = %q, want policy_violation", dec.Reason)
	}
	got := readPolicyRuleName(t, p.db, dec.AuditID)
	if got != "first_denier" {
		t.Errorf("policy_rule_name = %q, want first_denier", got)
	}
}

func TestCheckPolicy_PanicTreatedAsDeny(t *testing.T) {
	mcpc := &fakeMCP{}
	p, _ := newTestPEP(t, mcpc)
	p.policies = []Policy{&stubPolicy{name: "panic_pol", panic: true}}
	p.logger = nullLogger
	insertManifestAndTrust(t, p)

	dec, _, err := p.Invoke(context.Background(), defaultApplyRequest())
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if dec.Allowed {
		t.Fatal("panic should fail-closed")
	}
	if got := readPolicyRuleName(t, p.db, dec.AuditID); got != "panic_pol" {
		t.Errorf("policy_rule_name = %q, want panic_pol", got)
	}
}

func TestCheckPolicy_AgentSettingsLoadFailureDenies(t *testing.T) {
	mcpc := &fakeMCP{}
	p, _ := newTestPEP(t, mcpc)
	insertManifestAndTrust(t, p)
	// Close the DB underfoot so loadAgentSettings errors.
	_ = p.db.Close()

	dec, _, err := p.Invoke(context.Background(), defaultApplyRequest())
	// Two acceptable outcomes given timing:
	// (a) audit.insert errors first (DB closed before insert succeeds) → err != nil, dec zero-valued
	// (b) audit.insert succeeds before close, checkPolicy denies → err == nil, dec.Allowed == false
	// Both are fail-closed. The test asserts NOT-allowed regardless of which path fires.
	if err == nil && dec.Allowed {
		t.Fatal("closed DB should never allow")
	}
}
