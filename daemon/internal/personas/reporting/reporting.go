// Package reporting is the Reporting agent persona.
//
// The persona is registered at init() so it appears in the seven-persona
// sidebar model. Draft() is currently a dormant stub — the original
// product_health_digest skill was retired 2026-05-18 when Marketing's
// cold-draft batch workflow took over the empty-copy workflow.
//
// Draft will return Skipped:true until a real reporting skill lands
// (sales summaries, KPI digests, or equivalent). The registration and
// all identity methods are intentionally preserved so the persona shows
// in the sidebar without code changes when a new skill is wired in.
package reporting

import (
	"context"

	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
)

func init() {
	personas.Register(&Reporting{})
}

type Reporting struct{}

func (Reporting) Slug() string        { return "reporting" }
func (Reporting) DisplayName() string { return "Reporting" }

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
