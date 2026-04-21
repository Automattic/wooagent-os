# Prompts registry

Versioned persona system prompts, loaded by `internal/registry/prompts.go` on daemon startup.

Each file lives at `prompts/<persona>/v<semver>.yaml`:

```yaml
name: Marketing & SEO v0.1.0
persona: marketing
version: 0.1.0
supersedes: ""
body: |
  You are the Marketing & SEO agent for a WooCommerce store...
```

The registry picks the highest-version file per persona. This is intentionally file-based, not Go string literals, so the v2 GEPA pipeline (see `wooagent-os-gepa-v2-plan.md`) can ship new prompt versions as signed manifest drops without a daemon rebuild.

No prompts ship in v0.1 — Phase 1–2 agents will land them as they come online.
