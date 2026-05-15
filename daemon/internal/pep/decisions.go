// Package pep implements the Policy Enforcement Point from PRD §8.4.2 — the
// deterministic non-LLM gate between the orchestrator and the MCP client.
// Every store-mutating MCP call routes through pep.Invoke; the rule "no
// orchestrator → MCP shortcut" is enforced by making this the only path.
//
// V1 ships two of the six checks plus the chain-of-identity audit log. The
// other four checks (schema validation, policy predicates, budgets, scope
// sufficiency) are stubbed pass-through with TODO links to the post-V1 phase
// that lights them up. Defining the full surface in V1 means each later phase
// is additive, not refactor-y.
package pep

import (
	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
)

// Intent declares whether the caller is reading from the store, staging a
// proposal for review, or applying a change immediately. The PEP uses Intent
// in the scope-sufficiency check (Phase 2): an apply against a propose-scoped
// ability is denied unless the call is staging a proposal, not applying it.
type Intent string

const (
	IntentRead    Intent = "read"
	IntentPropose Intent = "propose"
	IntentApply   Intent = "apply"
)

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

// Request is the input to Invoke. Fields cluster into three groups:
//   - what to do: Persona, Ability, Args, Intent
//   - where it came from (chain-of-identity): PlanID, TaskID, StepID, IssueID
//   - how it was produced (LLM provenance): Model, PromptHash
//
// Empty chain-of-identity fields are fine in V1 — the operator-driven approve
// path doesn't have a plan/task/step yet. The audit log records whatever's
// present so we have history when the orchestrator lands.
type Request struct {
	Persona manifest.Persona
	Ability string
	Args    map[string]any
	Intent  Intent

	// Chain-of-identity context. Empty when the call originates from an
	// operator action (e.g. clicking Approve) rather than an agent plan.
	PlanID  string
	TaskID  string
	StepID  string
	IssueID string
	// BatchID links the call back to the parent batch when the issue is
	// part of one. Forward-compat for analytics ("which batches got
	// partially denied"); audit_invocations doesn't carry it as a column —
	// joins go through issues.batch_id.
	BatchID string

	// LLM provenance. Empty when the call is operator-driven.
	Model      string
	PromptHash string

	// Source declares whether this invocation originated from an operator
	// action (Approve button) or an autonomous agent path. checkScope
	// Sufficiency uses Source to allow operator-mediated calls to apply
	// against propose-scoped abilities while keeping the gate strict for
	// agent paths. Empty Source is treated as SourceAgent.
	Source Source
}

// Decision is the result of a check pipeline. When Allowed is true, AuditID
// points at the audit row and the caller can proceed with the MCP call.
// When Allowed is false, Reason is one of the typed deny codes below and the
// audit row is already written with outcome=denied.
type Decision struct {
	Allowed  bool
	Reason   ReasonCode // populated when Allowed is false
	AuditID  int64      // row id of the audit_invocations entry
	CapToken string     // empty until Phase 3
}

// ReasonCode is the typed denial code returned to the caller and recorded in
// the audit log. Strings are stable and become part of the daemon's external
// contract — adding new codes is fine, renaming existing ones is not.
type ReasonCode string

const (
	// ReasonAbilityUnapproved means the ability isn't in the manifest and
	// hasn't been operator-approved at runtime. Check 1.
	ReasonAbilityUnapproved ReasonCode = "ability_unapproved"

	// ReasonAbilityRevoked means the operator explicitly revoked this
	// ability via the Skills UI. Revocation wins over manifest pre-
	// signing and over trust_state='trusted'. Check 1.
	ReasonAbilityRevoked ReasonCode = "ability_revoked"

	// ReasonPersonaForbidden means the manifest entry exists but this
	// persona isn't on its Personas list. Check 2.
	ReasonPersonaForbidden ReasonCode = "persona_forbidden"

	// ReasonInvalidArguments means the arguments don't validate against the
	// ability's input_schema. Check 3 — Phase 2.
	ReasonInvalidArguments ReasonCode = "invalid_arguments"

	// ReasonPolicyViolation means the arguments tripped an operator-defined
	// policy predicate. Check 4 — Phase 2.
	ReasonPolicyViolation ReasonCode = "policy_violation"

	// ReasonBudgetExceeded means this persona is over its daily token, cost,
	// or call budget. Check 5 — Phase 2.
	ReasonBudgetExceeded ReasonCode = "budget_exceeded"

	// ReasonScopeInsufficient means the call's Intent (e.g. apply) is more
	// privileged than the ability's manifest Scope (e.g. propose) allows.
	// Check 6 — Phase 2.
	ReasonScopeInsufficient ReasonCode = "scope_insufficient"

	// ReasonSchemaCompileError means the ability's cached input_schema
	// could not be compiled by the JSON Schema validator, or the underlying
	// lookup failed. Distinct from ReasonInvalidArguments so operators can
	// tell cache rot / infra failure from caller mistakes. Check 3.
	ReasonSchemaCompileError ReasonCode = "schema_compile_error"
)

// Outcome is recorded on every audit row. Final state when Invoke returns.
type Outcome string

const (
	OutcomePending  Outcome = "pending"   // row written, MCP call in flight
	OutcomeSuccess  Outcome = "success"   // checks passed, MCP returned ok
	OutcomeMCPError Outcome = "mcp_error" // checks passed, MCP returned an error
	OutcomeDenied   Outcome = "denied"    // a check denied the call
)
