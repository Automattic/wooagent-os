# WooAgent OS — Conventions

This file is loaded into every Claude Code session in this repo. It captures conventions that aren't obvious from the code, especially around the UI stack. Keep it short. If a section grows long, move it into a focused doc and link to it.

## Repo layout

- `daemon/`, `cmd/`, `internal/` — Go (ADK Go runtime). Pure-Go, no Python.
- `ui/` — daemon-served React UI. Connects to the local daemon over `/v1/*`. Real auth, real fetches.
- `marketing-prototype/` — standalone React app deployed to GHES Pages. Mock data; serves as the design reference for teammates.
- `prompts/`, `skills/` — agent prompts and ability templates.
- `spike-adk/` — exploratory Python (kept for now; new work goes in Go).

## UI stack — WordPress Design System (WPDS)

Both `ui/` and `marketing-prototype/` use the WordPress Design System. **No Tailwind, no bespoke token systems, no Inter / Roboto / system-font defaults.** When building or reviewing UI in this repo, invoke these skills:

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
- Typography: `--wpds-typography-font-family-{body,heading,mono}`, `--wpds-typography-font-size-*`, `--wpds-typography-line-height-*`, `--wpds-typography-font-weight-*`.
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

Used in **two places only**: the agent avatar squares in the sidebar, and the kind pill on cards (CONTENT / CAMPAIGN / EMAIL — colored by the owning persona). Everything else (status badges, KPI cards, action buttons, notices) uses native WPDS intents (`high`, `medium`, `low`, `stable`, `informational`, `draft`, `none`). **Don't expand this exception.** Any new color need is a WPDS need.

### Layout

`Stack` from `@wordpress/ui` is the default layout primitive (it's CSS Flexbox with design-token gaps). Reach for plain flexbox CSS only where `Stack` doesn't fit (`position: sticky`, custom grid templates). No utility-class systems.

## Build & dev

- `cd ui && npm run dev` — port 5173. Connects to a local daemon via stored bearer token.
- `cd marketing-prototype && npm run dev` — port 5174. Mock data; uses Vite proxy `/api/woo/*` to the staging WooCommerce store.
- `cd marketing-prototype && npm run deploy` — bash script to publish to GHES Pages.

Both apps share the WPDS foundation but maintain independent Vite setups, dependencies, and bundle output.

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
