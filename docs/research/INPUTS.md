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

What WooAgent does TODAY (not roadmap):

- Generates **draft improvements to product descriptions** for merchant review (Marketing persona).
- Suggests **individual pricing updates** based on competitive context (Pricing persona).
- **Drafts customer-facing sales support messages** for the merchant to review, edit, and send (Sales Support persona).
- All operations that write to the store require **explicit merchant approval** before they execute.

Realistic first-week "aha" moment:

- A drafted message or product description the merchant ships, saving the hour they'd have spent writing it themselves.
- A pricing change recommendation they hadn't thought of and decide to apply.

What it doesn't do yet:

- **Marketing agent** today covers product descriptions only — campaigns and email are not shipped yet.
- **Pricing agent** today covers individual-product recommendations only — bulk pricing operations (e.g., discount sweeps) are not shipped yet.
- **Inventory** agent (stock, reorder, supplier) — under construction.
- **Reporting** agent (analytics, dashboards) — under construction.
- **Accounting** and **Chief of Staff** agents — under construction.
- Of the seven personas in the agent grid, only **Marketing / Pricing / Sales Support** ship working capabilities today, and all three are partial.

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
