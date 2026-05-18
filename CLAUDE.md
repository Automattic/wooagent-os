# WooAgent OS — Conventions

This file is loaded into every Claude Code session in this repo. It captures conventions that aren't obvious from the code, especially around the UI stack. Keep it short. If a section grows long, move it into a focused doc and link to it.

## Repo layout

- `daemon/`, `cmd/`, `internal/` — Go (ADK Go runtime). Pure-Go, no Python.
- `ui/` — daemon-served React UI. Connects to the local daemon over `/v1/*`. Real auth, real fetches. **The active UI surface — all new UI work happens here.**
- `prompts/`, `skills/` — agent prompts and ability templates.
- `spike-adk/` — exploratory Python (kept for now; new work goes in Go).

## UI stack — WordPress Design System (WPDS)

**Before doing any UI work in `ui/`, read [`DESIGN.md`](./DESIGN.md) first.** It captures the canonical components in use across the app (the **Component inventory** section), the layout frame (sidebar + `Page` header + content), the persona-color exception, and the Do's/Don'ts that go beyond CLAUDE.md. Reach for one of the listed components before writing custom UI. If you find yourself adding a new dependency or a new bespoke component, that's the cue to call it out as a decision to be made and update DESIGN.md alongside the change.

**For customer-facing UI changes, also apply DESIGN.md's [Working on UI](./DESIGN.md#working-on-ui) lens:** state assumptions as *confident / assuming / unclear* before writing code; ask "what's the smallest version that solves this?"; run the verify-before-done checklist before claiming done; flag designer-review triggers (new screen, new pattern, primary CTA, onboarding / persona / approve-review touch, IA change, >1 sentence of customer copy) so the user can decide whether to loop in design.

`ui/` uses the WordPress Design System **exclusively**. **Only WPDS components.** No Tailwind, no bespoke token systems, no Inter / Roboto / system-font defaults, no custom-styled HTML elements (`<button>`, `<input>`, `<select>`, badges, dropdowns) without a WPDS wrapper. If WPDS doesn't have what you need, call it out as a decision to be made first and update DESIGN.md alongside the change.

**If you must draw something custom, you MUST add a `// CUSTOM:` code comment immediately above it explaining (a) why no WPDS component fits, (b) what's custom about it, (c) where it's documented (DESIGN.md / a P2 / an issue).** Reviewers should reject custom UI that isn't called out.

### Anti-rolls — reach for WPDS, not raw HTML

| Don't | Use |
|---|---|
| `<button>` with custom CSS | `Button` (icon-only: `Button icon={...} label="..."`) |
| `<input type="search">` | `SearchControl` |
| `<input type="text">` | `InputControl` / `TextControl` |
| `<select>` | `SelectControl` |
| `<a>` | `Link` |
| Custom badge span (counts, pills) | `Badge` from `@wordpress/ui` (use `intent="high"` for attention, `"informational"` for neutral counts) |
| Custom dropdown menu | `Dropdown` with `MenuItem` / `MenuGroup` |
| Mono-styled span for code-like text | `<code>` element with `font-family: var(--wpds-typography-font-family-mono)` only when semantically code; for everything else use the body font |

When building or reviewing UI in this repo, invoke these skills:

- `woo-design` — WooAgent's design language (visual identity, voice, component patterns); wraps `DESIGN.md` and extends WPDS with project-specific decisions. Pair with `wpds`.
- `wpds` — design system rules, MCP-server-backed component & token lookup.
- `frontend-design` — distinctive, polished frontend principles (apply within WPDS, not against it).
- `wordpress-mockups` — when prototyping WordPress admin / Site Editor concepts.

### Packages (per [the WPDS resources P2 post](https://designsystemsp2.wordpress.com/2026/04/20/resources-for-the-wordpress-design-system/) — private, read via the `context-a8c` `wpcom` MCP)

- `@wordpress/ui` — primary. `Card`, `CollapsibleCard`, `Stack`, `Text`, `Badge`, etc.
- `@wordpress/components` — fill the gaps (`Button`, `Notice`, `Spinner`, `Modal`, `Snackbar`, form controls). Always check the component's "Status" in Storybook — only adopt `stable`.
- `@wordpress/icons`, `@wordpress/element`.
- Higher-level abstractions when the shape fits (`DataViews` for tabular UIs).

### Tokens

Use `--wpds-*` CSS variables only:
- Color: `--wpds-color-bg-surface-*`, `--wpds-color-fg-content-*`, `--wpds-color-stroke-*`, `--wpds-color-bg-interactive-*`.
- Dimension: `--wpds-dimension-padding-*`, `--wpds-dimension-gap-*`, `--wpds-dimension-surface-width-*`.
- Typography: `--wpds-typography-font-family-{body,heading}`, `--wpds-typography-font-size-*`, `--wpds-typography-line-height-*`, `--wpds-typography-font-weight-*`. **No `mono`** — body font for identifiers, hostnames, slugs, model names, everything. See `DESIGN.md` Typography section for rationale.
- Elevation: `--wpds-elevation-{xs,sm,md,lg}`.
- Border: `--wpds-border-radius-*`, `--wpds-border-width-*`.

The reference site is https://system.automattic.design/. The WPDS MCP server (`@wordpress/design-system-mcp`) is wired — query `mcp__wordpress-design-system__get_components`, `…__get_design_tokens`, `…__get_component_details` for canonical docs before guessing.

### Documented exception: persona colors

The seven agent identities (Marketing / Pricing / Inventory / Accounting / Reporting / Sales Support / Chief of Staff) keep their brand colors as a small set of CSS variables:

- `--wa-persona-mk-bg` / `--wa-persona-mk-ink` — Marketing (pink: `#FCE7F3` / `#BE185D`)
- `--wa-persona-pr-bg` / `--wa-persona-pr-ink` — Pricing (blue: `#DBEAFE` / `#1D4ED8`)
- `--wa-persona-in-bg` / `--wa-persona-in-ink` — Inventory (amber: `#FEF3C7` / `#B45309`)
- `--wa-persona-ac-bg` / `--wa-persona-ac-ink` — Accounting (green: `#DCFCE7` / `#15803D`)
- `--wa-persona-rp-bg` / `--wa-persona-rp-ink` — Reporting (violet: `#EDE9FE` / `#6D28D9`)
- `--wa-persona-ss-bg` / `--wa-persona-ss-ink` — Sales Support (teal: `#CCFBF1` / `#0F766E`)
- `--wa-persona-cs-bg` / `--wa-persona-cs-ink` — Chief of Staff (indigo gradient: `#6366F1` → `#8B5CF6` / `#FFFFFF`)

Used in **one place only**: the agent avatar squares in the sidebar (`PersonaAvatar`). Two previous sites have been retired — the brand-header "W" tile in `LeftNav` moved to WPDS brand indigo (`--wpds-color-bg-interactive-brand-strong`) per the i3.2 Figma, and the persona-colored `KindBadge` "kind pill" on board cards was removed entirely when the queue moved to DataViews (agent identity via `PersonaAvatar` is the canonical visual signal). Everything else (status badges, KPI cards, action buttons, notices) uses native WPDS intents (`high`, `medium`, `low`, `stable`, `informational`, `draft`, `none`). **Don't expand this exception.** Any new color need is a WPDS need.

### Layout

`Stack` from `@wordpress/ui` is the default layout primitive (it's CSS Flexbox with design-token gaps). Reach for plain flexbox CSS only where `Stack` doesn't fit (`position: sticky`, custom grid templates). No utility-class systems.

## Build & dev

- `cd ui && npm run dev` — port 5173. Connects to a local daemon via stored bearer token.
- `scripts/build-companion-plugin-zip.sh` — packages the Companion Plugin for upload to a Woo store. Output at `build/wooagent-companion.zip`.

## Manifest refresh

The pre-signed ability manifest at `daemon/internal/manifest/default.json` is a snapshot of the WP Abilities API on the connected staging store. Drift detection compares the snapshot's schema hashes against what the live store registers — schema changes auto-demote the affected ability to `unapproved` until an operator re-reviews.

Refresh cadence: **monthly, or whenever a plugin update on the staging store changes the ability surface** (e.g., a new WC AI plugin release). Run:

```bash
WOOAGENT_MCP_USER='you@example.com' \
WOOAGENT_MCP_APP_PASSWORD='app password from wp-admin' \
scripts/refresh-manifest.sh
```

The script surgically merges so pre-signed canonical entries (placeholder `schema_hash`) survive a refresh against a store that doesn't yet register them — see DSGWOO-1279 for context on the WC 10.9 canonical-name pre-signs.

After the refresh, review the diff and commit if it looks right.

## Git

- Default branch is `trunk`.
- Push freely on feature branches; confirm before pushing to `main`/`trunk`.
- Never `--force-push`, never `--no-verify`.
- Committer must use the `@a8c.com` email when pushing to `github.a8c.com` (set in repo-local `.git/config`).

## External resources

- WPDS reference site: https://system.automattic.design/
- WPDS MCP package: `@wordpress/design-system-mcp` (npm)
- Design Systems P2 post (private): https://designsystemsp2.wordpress.com/2026/04/20/resources-for-the-wordpress-design-system/
- `@wordpress/components` Storybook: https://wordpress.github.io/gutenberg/?path=/docs/components-introduction--docs
- `@wordpress/ui` Storybook: https://wordpress.github.io/gutenberg/?path=/docs/design-system-components-introduction--docs
- DataViews docs: https://wordpress.github.io/gutenberg/?path=/docs/dataviews-dataviews--best-practices
- #design-systems on a8c.slack.com
