package pep

import "github.com/wooagent-os/wooagent-os/daemon/internal/manifest"

// reversibilityBeltPolicy gates direct agent applies against low-
// reversibility abilities. The PEP's existing checkScopeSufficiency
// covers Scope (whether the ability allows apply at all); this is a
// belt-and-braces against a future autonomous-apply path quietly
// applying something hard to undo. Dead-on-arrival in V1 since no
// agent code path produces Source=Agent + Intent=Apply.
type reversibilityBeltPolicy struct{}

func (*reversibilityBeltPolicy) Name() string { return "reversibility_belt" }

func (*reversibilityBeltPolicy) Evaluate(req Request, ctx EvalContext) (bool, string) {
	if req.Source != SourceAgent || req.Intent != IntentApply {
		return false, ""
	}
	if ctx.Manifest == nil {
		return false, ""
	}
	entry := ctx.Manifest.Get(req.Ability)
	if entry == nil {
		// Operator-approved-at-runtime ability — no manifest scope to read.
		return false, ""
	}
	if entry.Reversibility >= manifest.LowReversibilityThreshold {
		return false, ""
	}
	return true, "agent apply blocked: low-reversibility ability (operator must mediate)"
}
