// Package reporting is the Reporting agent persona.
//
// Draft is a dormant stub today — the original product_health_digest skill
// was retired 2026-05-18 when Marketing's cold-draft batch workflow took
// over the empty-copy workflow. Until a real reporting skill lands
// (sales summaries, KPI digests, or equivalent), the persona is
// intentionally NOT registered in the personas registry, so:
//
//   - GET /v1/agents reports Implemented=false for any historical row
//     (operators who previously enabled Reporting see "Coming soon" rather
//     than dead controls).
//   - PATCH /v1/agents/{slug=reporting} returns 404 (no opt-in path).
//   - The UI's Add Agent modal does not offer Reporting.
//
// The Go-side persona type and methods are preserved so re-enabling once
// skills are wired is a one-line change (un-comment the Register call).
package reporting

import (
	"context"

	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
)

func init() {
	// Disabled until a real Reporting skill lands. See package comment.
	// personas.Register(&Reporting{})
}

type Reporting struct{}

func (Reporting) Slug() string        { return "reporting" }
func (Reporting) DisplayName() string { return "Reporting" }

// Addable: Reporting ships dormant. Draft is a stub today (product_health_digest
// retired); operator opts in via Add Agent when a real Reporting skill lands.
func (Reporting) Addable() bool { return true }

// Cooldown is zero-value because Reporting doesn't dedup by target.
// A digest summarizes catalog state rather than acting on a specific
// product, so per-target Cooldown (Marketing's product_id pattern)
// doesn't apply.
func (Reporting) Cooldown() personas.CooldownPolicy {
	return personas.CooldownPolicy{}
}

// Draft is a dormant stub. product_health_digest was retired 2026-05-18;
// see docs/specs/2026-05-18-marketing-cold-draft-batch-design.md.
// Returns Skipped:true until a real reporting skill is registered.
func (Reporting) Draft(ctx context.Context, deps personas.Deps) (personas.Drafted, error) {
	return personas.Drafted{
		Skipped:    true,
		SkipReason: "no skills registered yet — product_health_digest retired 2026-05-18 (see docs/specs/2026-05-18-marketing-cold-draft-batch-design.md)",
	}, nil
}
