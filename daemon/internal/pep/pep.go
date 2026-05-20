package pep

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
)

// MCPClient is the subset of mcp.Client that pep needs. Defined as an
// interface so tests can supply a fake without spinning up a real server.
type MCPClient interface {
	Initialize(ctx context.Context) (mcp.ServerInfo, error)
	CallTool(ctx context.Context, name string, args any) (mcp.ToolCallResult, error)
}

// PEP is the runtime gate. One per daemon process — checks are deterministic
// and the audit writer is the only state, so PEP itself is safe for
// concurrent Invoke calls (SQLite handles the writer serialization).
type PEP struct {
	manifest *manifest.Lookup
	mcp      MCPClient
	audit    *auditWriter
	db       *sql.DB
	schemas  *schemaCache
	budget   *BudgetGate
	// logger captures schema-validation diagnostics that we deliberately
	// do not propagate to audit rows or HTTP responses (the rejected values
	// may contain PII). Defaults to slog.Default(); tests overwrite directly.
	logger   *slog.Logger
	policies []Policy
}

// New wires the PEP to its dependencies. The manifest Lookup is required;
// the MCP client may be nil for ability fetch testing or UI-only daemon
// runs (Invoke will refuse to dispatch in that case). budget may be nil
// to disable budget enforcement (useful in tests that don't care about it).
func New(m *manifest.Lookup, mcpClient MCPClient, db *sql.DB, budget *BudgetGate) *PEP {
	return &PEP{
		manifest: m,
		mcp:      mcpClient,
		audit:    newAuditWriter(db, budget),
		db:       db,
		schemas:  &schemaCache{},
		budget:   budget,
		logger:   slog.Default(),
		policies: []Policy{
			&defensiveArgsPolicy{},
			&reversibilityBeltPolicy{},
			&operatorHoursPolicy{},
		},
	}
}

// Manifest returns the bundled pre-signed manifest lookup so callers (the
// abilities HTTP handler in particular) can derive the "this ability is
// built-in" UI signal without re-loading the manifest themselves. Read-only
// — callers must not mutate the returned Lookup.
func (p *PEP) Manifest() *manifest.Lookup {
	if p == nil {
		return nil
	}
	return p.manifest
}

// ErrMCPNotConfigured is returned by Invoke when the daemon is running
// without an MCP client. Callers map it to 503.
var ErrMCPNotConfigured = errors.New("pep: mcp client not configured")

// Invoke runs the V1 check pipeline (trust state, persona scope) plus the
// stubbed phase-2/3 checks (which currently pass through), records an audit
// row regardless of outcome, and — when allowed — calls the MCP ability and
// finalizes the audit row with the result.
//
// Returns:
//   - decision (whether allowed; on deny, Reason is set and AuditID points
//     at the audit row already finalized as denied)
//   - mcp result (zero-value when the call wasn't dispatched)
//   - error (any infra failure: audit write, mcp init, mcp call, etc.)
//
// A denial is *not* an error — error is reserved for things that broke. The
// caller branches on decision.Allowed first.
func (p *PEP) Invoke(ctx context.Context, req Request) (Decision, mcp.ToolCallResult, error) {
	argsHash := hashArgs(req.Args)
	auditID, err := p.audit.insert(ctx, req, argsHash)
	if err != nil {
		return Decision{}, mcp.ToolCallResult{}, fmt.Errorf("pep audit insert: %w", err)
	}

	// Run the six checks in PRD §8.4.2 order. First denial wins; subsequent
	// checks are skipped.
	if reason := p.checkTrustState(ctx, req); reason != "" {
		return p.deny(ctx, auditID, req.Persona, reason)
	}
	if reason := p.checkPersonaScope(req); reason != "" {
		return p.deny(ctx, auditID, req.Persona, reason)
	}
	if reason := p.checkSchema(ctx, req); reason != "" {
		return p.deny(ctx, auditID, req.Persona, reason)
	}
	if reason, ruleName := p.checkPolicy(ctx, req); reason != "" {
		if err := p.audit.finalizeWithRule(ctx, auditID, req.Persona, OutcomeDenied, reason, ruleName); err != nil {
			return Decision{}, mcp.ToolCallResult{}, err
		}
		return Decision{Allowed: false, Reason: reason, AuditID: auditID}, mcp.ToolCallResult{}, nil
	}
	if reason := p.checkBudget(ctx, req); reason != "" {
		return p.deny(ctx, auditID, req.Persona, reason)
	}
	if reason := p.checkScopeSufficiency(req); reason != "" {
		return p.deny(ctx, auditID, req.Persona, reason)
	}

	// All checks passed. Dispatch via MCP and finalize the row.
	if p.mcp == nil {
		// Audit row stays pending — finalize it as mcp_error so the operator
		// can tell allowed-but-undispatched from genuine MCP failures.
		if err := p.audit.finalize(ctx, auditID, req.Persona, OutcomeMCPError, ""); err != nil {
			return Decision{}, mcp.ToolCallResult{}, err
		}
		return Decision{}, mcp.ToolCallResult{}, ErrMCPNotConfigured
	}

	// Initialize is idempotent; persona-marketing and approve both call it.
	if _, err := p.mcp.Initialize(ctx); err != nil {
		_ = p.audit.finalize(ctx, auditID, req.Persona, OutcomeMCPError, "")
		return Decision{}, mcp.ToolCallResult{}, fmt.Errorf("mcp init: %w", err)
	}

	res, err := p.mcp.CallTool(ctx, "mcp-adapter-execute-ability", map[string]any{
		"ability_name": req.Ability,
		"parameters":   req.Args,
	})
	if err != nil {
		_ = p.audit.finalize(ctx, auditID, req.Persona, OutcomeMCPError, "")
		return Decision{}, mcp.ToolCallResult{}, fmt.Errorf("mcp call %s: %w", req.Ability, err)
	}

	if err := p.audit.finalize(ctx, auditID, req.Persona, OutcomeSuccess, ""); err != nil {
		return Decision{}, res, err
	}
	return Decision{Allowed: true, AuditID: auditID}, res, nil
}

// deny finalizes the audit row with the given reason and returns the decision.
// Errors from finalize are surfaced — losing the audit row would compromise
// the chain-of-identity story even on a denial.
func (p *PEP) deny(ctx context.Context, auditID int64, persona manifest.Persona, reason ReasonCode) (Decision, mcp.ToolCallResult, error) {
	if err := p.audit.finalize(ctx, auditID, persona, OutcomeDenied, reason); err != nil {
		return Decision{}, mcp.ToolCallResult{}, err
	}
	return Decision{Allowed: false, Reason: reason, AuditID: auditID}, mcp.ToolCallResult{}, nil
}

// ---------- the six checks ----------

// checkTrustState — Check 1. Decides whether the ability is admissible
// from the daemon's trust perspective: revoked rows are always denied,
// manifest-pre-signed rows are always allowed, operator-trusted rows
// at the current schema are allowed, everything else is denied.
//
// Per-call DB read by design — the truth lives in the abilities table
// and the operator's UI mutations must take effect immediately. SQLite
// local reads are sub-millisecond; no in-memory cache.
func (p *PEP) checkTrustState(ctx context.Context, req Request) ReasonCode {
	var trustState string
	var revokedAt sql.NullString
	err := p.db.QueryRowContext(ctx,
		`SELECT trust_state, revoked_at FROM abilities WHERE name = ?`,
		req.Ability,
	).Scan(&trustState, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		// No row at all means we've never discovered this ability.
		// Manifest-only entries that aren't yet in the DB (e.g. before
		// the first discovery sweep) still flow through the manifest
		// branch below.
		if p.manifest.Get(req.Ability) != nil {
			return ""
		}
		return ReasonAbilityUnapproved
	}
	if err != nil {
		// Fall back to the conservative "deny on lookup failure" stance.
		// The error will surface via the audit row's denial_reason. The
		// alternative (allow on error) is unacceptable for a launch
		// product.
		return ReasonAbilityUnapproved
	}
	if revokedAt.Valid && revokedAt.String != "" {
		return ReasonAbilityRevoked
	}
	if p.manifest.Get(req.Ability) != nil {
		return ""
	}
	if trustState == "trusted" {
		return ""
	}
	return ReasonAbilityUnapproved
}

// checkPersonaScope — Check 2. The ability's manifest entry must list this
// persona. Operator-approved abilities have no Personas list yet (V1 always
// allows for them; Phase 2 wires the operator-config admin surface).
func (p *PEP) checkPersonaScope(req Request) ReasonCode {
	entry := p.manifest.Get(req.Ability)
	if entry == nil {
		// Operator-approved abilities currently have no per-persona
		// restriction. Phase 2 turns this on.
		return ""
	}
	for _, p := range entry.Personas {
		if p == req.Persona {
			return ""
		}
	}
	return ReasonPersonaForbidden
}

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
		p.logSchemaValidationFailure(req.Ability, err)
		return ReasonInvalidArguments
	}
	return ""
}

// logSchemaValidationFailure emits a Warn log with the failing JSON Pointer
// paths so operators have something actionable when triaging a denied call.
// Per the schema-scope design, rejected *values* never reach the log (PII);
// only the count + paths are logged, since paths come from the schema, not
// the args.
func (p *PEP) logSchemaValidationFailure(ability string, err error) {
	if p.logger == nil {
		return
	}
	paths := schemaErrorPaths(err)
	p.logger.Warn("pep schema validation rejected call",
		"ability", ability,
		"reason", string(ReasonInvalidArguments),
		"failures", len(paths),
		"paths", paths,
	)
}

// schemaErrorPaths flattens a *jsonschema.ValidationError tree into the leaf
// InstanceLocation strings. Returns nil for non-validation errors so callers
// can still log a coarse "failures: 0" entry.
func schemaErrorPaths(err error) []string {
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return nil
	}
	var paths []string
	var walk func(*jsonschema.ValidationError)
	walk = func(v *jsonschema.ValidationError) {
		if len(v.Causes) == 0 {
			paths = append(paths, v.InstanceLocation)
			return
		}
		for _, c := range v.Causes {
			walk(c)
		}
	}
	walk(ve)
	return paths
}

// checkPolicy — Check 4. Iterates the registered policies in order;
// first deny wins. On deny, returns (ReasonPolicyViolation, ruleName).
// The caller (Invoke) plumbs ruleName into the audit row via
// audit.finalizeWithRule.
//
// Fail-closed: a load failure on agent settings or a panic in a policy
// implementation both deny. Tests cover both.
func (p *PEP) checkPolicy(ctx context.Context, req Request) (ReasonCode, string) {
	settings, err := loadAgentSettings(ctx, p.db, req.Persona)
	if err != nil {
		return ReasonPolicyViolation, "agent_settings_unavailable"
	}
	evalCtx := EvalContext{
		Now:      time.Now(),
		Manifest: p.manifest,
		Agent:    settings,
	}
	for _, pol := range p.policies {
		deny, reason, panicked := evalWithRecover(pol, req, evalCtx)
		if panicked {
			p.logger.Error("pep policy evaluator panicked",
				"ability", req.Ability,
				"persona", string(req.Persona),
				"rule_name", pol.Name(),
			)
			return ReasonPolicyViolation, pol.Name()
		}
		if deny {
			p.logPolicyDenial(req, pol.Name(), reason)
			return ReasonPolicyViolation, pol.Name()
		}
	}
	return "", ""
}

// logPolicyDenial emits a Warn line on every policy deny. Matches the
// PII-redaction posture from checkSchema (DSGWOO-1307): the operator
// gets ability + persona + rule + a short structured reason; rejected
// argument *values* never reach the log.
func (p *PEP) logPolicyDenial(req Request, ruleName, _ string) {
	if p.logger == nil {
		return
	}
	p.logger.Warn("pep policy denied call",
		"ability", req.Ability,
		"persona", string(req.Persona),
		"reason", string(ReasonPolicyViolation),
		"rule_name", ruleName,
	)
}

// checkBudget — Check 5. Refuses MCP dispatch when the persona is at or
// over its daily cost or call limit. The scheduler also pre-checks budget
// before starting a tick; this PEP-side gate is the backstop for any
// path that bypasses the scheduler (e.g. operator-driven approve flows).
func (p *PEP) checkBudget(ctx context.Context, req Request) ReasonCode {
	if p.budget == nil {
		return ""
	}
	reason, err := p.budget.Check(ctx, req.Persona)
	if err != nil {
		return ReasonBudgetExceeded
	}
	return reason
}

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
	// Unknown Source (empty string) is treated as deny-by-default — callers
	// that forget to set Source should not quietly pass through the gate.
	if req.Source != SourceAgent {
		return ReasonScopeInsufficient
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
