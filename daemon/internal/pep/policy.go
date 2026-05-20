package pep

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
)

// Policy is the PEP's Check 4 plug-point. Each Policy is a small, focused
// gate: it inspects a Request + EvalContext and either allows or denies
// with a short reason. Policies are evaluated in registration order in
// PEP.checkPolicy; first deny wins. The reason string is operator-facing
// (the PEP doesn't propagate it to HTTP) — keep it short and concrete.
//
// Name() is the audit signal: it lands in audit_invocations.policy_rule_name
// so an operator triaging a "policy_violation" denial can trace exactly
// which gate fired. Stable; treat as part of the daemon's external contract.
type Policy interface {
	Name() string
	Evaluate(req Request, ctx EvalContext) (deny bool, reason string)
}

// EvalContext is everything a Policy gets beyond the Request. Kept narrow
// on purpose: policies can't reach into arbitrary state.
type EvalContext struct {
	// Now is the wall-clock time for time-of-day comparisons. Injected so
	// policy_hours_test.go can freeze the clock; defaults to time.Now() in
	// production (PEP.checkPolicy sets this).
	Now time.Time
	// Manifest exposes ability metadata (Reversibility, Personas, Scope)
	// without re-loading. Read-only — policies must not mutate.
	Manifest *manifest.Lookup
	// Agent carries per-persona configuration loaded from the agents row.
	Agent AgentSettings
}

// AgentSettings is the subset of the agents-table row that policies read.
// Adding fields here is purely additive; new policies pick up new fields
// without other policies needing to know.
type AgentSettings struct {
	// ApplyHoursStart / ApplyHoursEnd describe the operator-hours window
	// in "HH:MM" 24-hour daemon-local format. Both empty = no window
	// configured (policy_hours is a no-op). XOR (only one set) is rejected
	// at write time by the agents PATCH handler, but the policy parser
	// re-validates as defense-in-depth.
	ApplyHoursStart string
	ApplyHoursEnd   string
}

// loadAgentSettings reads the agents row for the given persona. Returns
// the zero-value AgentSettings on a missing row (so a not-yet-seeded
// persona doesn't accidentally fail-closed); any other DB error is
// returned for the caller to translate into a conservative deny.
func loadAgentSettings(ctx context.Context, db *sql.DB, persona manifest.Persona) (AgentSettings, error) {
	var startNull, endNull sql.NullString
	err := db.QueryRowContext(ctx,
		`SELECT apply_hours_start, apply_hours_end FROM agents WHERE persona = ?`,
		string(persona),
	).Scan(&startNull, &endNull)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentSettings{}, nil
	}
	if err != nil {
		return AgentSettings{}, err
	}
	s := AgentSettings{}
	if startNull.Valid {
		s.ApplyHoursStart = startNull.String
	}
	if endNull.Valid {
		s.ApplyHoursEnd = endNull.String
	}
	return s, nil
}

// evalWithRecover wraps a Policy.Evaluate call so a panic in a built-in
// (or future user-supplied) policy can't crash the daemon. Recovered
// panics are surfaced as a deny with rule_name=<offending policy>; the
// caller in checkPolicy logs them at slog.Error so operators see them.
func evalWithRecover(p Policy, req Request, ctx EvalContext) (deny bool, reason string, panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			deny = true
			panicked = true
			reason = "policy evaluator panicked"
		}
	}()
	deny, reason = p.Evaluate(req, ctx)
	return deny, reason, false
}
