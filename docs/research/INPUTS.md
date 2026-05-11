# WooAgent — target-user research inputs

Loaded by the `user-research` skill. Keep this short (~150 lines); load-bearing facts only.

## Stated target

Verbatim from Linear (pulled 2026-05-11):

> Solo operators and small teams running $250K–$1M+ GMV WooCommerce stores. Multi-channel (DTC + B2B + wholesale), mature operations, often agency-supported, high operational complexity. They are already using AI ad-hoc (ChatGPT, Claude) but without a system around it.

## Hard constraints

The product literally cannot function without these.

- Runs a WooCommerce store with admin access.
- Has a local machine they can install software on (Mac/Windows/Linux daemon).

## Friction (acquirable with motivation)

Motivated users will cross these; flag them as onboarding cost, not as filters.

- **LLM API key** (Anthropic / OpenAI). The UI provides a link to the acquisition flow. ~10 min, ~$0–20/mo for typical usage.
- **Or Ollama familiarity** — local model alternative for users who don't want a hosted-LLM relationship. ~1hr learning curve.
- **Terminal comfort** for daemon install today. (Tracked separately — a GUI installer would change this.)

## Value prop, current state

*To be filled in with Elizabeth in Task 5. What WooAgent does TODAY — not roadmap.*

## The seven personas (jobs WooAgent represents)

Each persona is a "hire" the merchant might be making.

- **Marketing** — content + campaigns + email
- **Pricing** — pricing strategy + promos
- **Inventory** — stock + reorder + supplier
- **Accounting** — books + reconciliation
- **Reporting** — analytics + dashboards
- **Sales Support** — pre/post-sale customer comms
- **Chief of Staff** — coordinator across the six

Persona definitions live in `daemon/prompts/` and `daemon/internal/personas/`.

## Adjacent docs

- `uxresearch.md` — usability testing plan; receives the screener block this skill produces.
- `CLAUDE.md` — project conventions, persona color tokens.
- `docs/superpowers/specs/2026-05-11-user-research-skill-design.md` — the design spec for the skill itself (gitignored locally).
