# Local skills registry

Versioned local (non-MCP) skill definitions, loaded by `internal/registry/skills.go` on daemon startup.

Each file lives at `skills/<skill-name>/v<semver>.yaml`:

```yaml
name: pricing.margin.compute
version: 0.1.0
description: |
  Compute per-SKU gross margin from price, COGS, fees, and shipping.
schema:
  input:
    type: object
    properties:
      sku: {type: string}
      ...
examples:
  - "Compute margin for SKU ABC-123 at $49.99 with COGS $22"
```

Skill descriptions are first-class GEPA optimization targets — they drive the agent's skill-selection accuracy. Keeping them in file-backed YAML lets the v2 pipeline evolve them via signed manifest without a daemon rebuild.

No skills ship in v0.1 — landing them is scheduled for Phase 2 alongside the Pricing and Sales Support personas.
