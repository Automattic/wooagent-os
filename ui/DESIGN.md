---
name: WooAgent
description: Calm, competent AI coworkers inside a WordPress-native shell. Layered on the WordPress Design System (WPDS).
colors:
  persona-mk-bg: "#FCE7F3"
  persona-mk-ink: "#BE185D"
  persona-pr-bg: "#DBEAFE"
  persona-pr-ink: "#1D4ED8"
  persona-in-bg: "#FEF3C7"
  persona-in-ink: "#B45309"
  persona-ac-bg: "#DCFCE7"
  persona-ac-ink: "#15803D"
  persona-rp-bg: "#EDE9FE"
  persona-rp-ink: "#6D28D9"
  persona-ss-bg: "#CCFBF1"
  persona-ss-ink: "#0F766E"
  persona-cs-bg-from: "#6366F1"
  persona-cs-bg-to: "#8B5CF6"
  persona-cs-ink: "#FFFFFF"
---

## Overview

WooAgent is a small team of AI coworkers running inside a WordPress shop. The UI's job is to make that team feel **calm, competent, and trustworthy** — not flashy, not magical, not narrated.

The visual model is intentionally restrained: a dark navigational sidebar where each agent has a small persona-colored avatar, a generous light workspace where the actual work happens, and a single indigo accent for primary action. Persona color is identity, never decoration. There are no AI-product flourishes — no shimmer gradients, no typewriter streaming, no glow effects. When agents are working, the UI says so plainly and gets out of the way.

This file extends the WordPress Design System (WPDS) with the decisions specific to WooAgent. WPDS is the source of truth for tokens; this file captures the layer above — the personality, the few WooAgent-owned tokens (persona colors), and the rules that distinguish a WooAgent screen from a generic high-quality WP plugin screen.

**Voice is warm and conversational, chrome stays calm.** Empty states address the user directly ("Nothing here yet — ready to draft your first campaign?"). Agent activity is plain ("Working on those rewrites for you…"). Errors apologize without grovelling ("Hmm, I can't reach your store right now. Want to retry?"). Calm chrome, warm words.

The reference designs are the 13.0 / 13.1 / 13.2 pages in the WooAgent Figma file.

## Colors

**The rule:** every color in component code must be a `--wpds-*` CSS variable or a `--wa-persona-*` variable. Hex literals are a smell — they indicate that a WPDS token wasn't found, which usually means a WPDS lookup wasn't attempted. Query the WPDS MCP server (`mcp__wordpress-design-system__get_components`, `…__get_design_tokens`) before reaching for hex.

### WPDS tokens (everything except persona)

- Surfaces — `--wpds-color-bg-surface-*`
- Foreground (text, icons) — `--wpds-color-fg-content-*`
- Strokes — `--wpds-color-stroke-*`
- Interactive — `--wpds-color-bg-interactive-*`
- Status / intent — WPDS intents (`high`, `medium`, `low`, `stable`, `informational`, `draft`, `none`). Never custom red / yellow / green hex values.

### Persona palette (WooAgent-owned)

The seven agent identities each have a `bg` / `ink` pair, exposed as CSS variables `--wa-persona-{xx}-bg` / `--wa-persona-{xx}-ink`. Keys: `mk` (Marketing), `pr` (Pricing), `in` (Inventory), `ac` (Accounting), `rp` (Reporting), `ss` (Sales Support), `cs` (Chief of Staff).

**Used in two places only:**

1. The `PersonaAvatar` component — sidebar header, queue card corners, page eyebrows. The persona's identity tile.
2. The kind pill on cards (CONTENT / CAMPAIGN / EMAIL) — colored by the owning persona.

**Don't expand this exception.** A future need for a third persona-colored surface should be redirected to a WPDS intent variant or a neutral surface. The exception is small on purpose — broadening it makes the UI feel costumed.

### Primary action

Primary CTAs are **WPDS indigo** — the same color across every screen, every persona context, every depth of navigation. Persona color never appears as a button fill, hover state, or focus ring. Approve, save, continue, send, retry: all indigo. If a CTA needs to be elevated, use scale or position, not color.

## Typography

WPDS provides the type system. Use only `--wpds-typography-font-family-{body, heading}` and the `--wpds-typography-font-size-*` / `--wpds-typography-line-height-*` / `--wpds-typography-font-weight-*` scales. No `Inter`. No `system-ui`. No bespoke type ramps.

**No monospace fonts anywhere.** Don't use `--wpds-typography-font-family-mono`. Don't reach for `Menlo`, `SF Mono`, `Consolas`, or any other code-style face. Identifiers like model names (`anthropic/claude-sonnet-4-6`), hostnames (`localhost:7777`), and slugs (`marketing`, `pricing`) render in the body font. Mono creates a "developer console" feel that conflicts with the calm-coworker personality — and in practice, every place we tried mono (the model picker, the daemon hostname, the sidebar badge) became the *most visually jarring* element on its surface. If something is genuinely code, wrap it in `<code>` only when it's part of a documentation context; in the operator UI, plain body text is correct.

Two WooAgent-specific notes:

- **Tabular numerals on KPIs and metric strips** (`Kpi`). Numbers that change shouldn't jiggle horizontally. Use `font-variant-numeric: tabular-nums` on the metric value — this is *not* a mono font, it's a body-font feature.
- **Heading font for agent display names** even in body contexts (sidebar, page headers, persona pills). Agent names carry identity — they should stand out from running prose.
- **Sentence case for everything user-facing.** Persona names, page titles, button labels, menu items, modal titles, table headers, empty-state copy. Capitalize the first word and any proper nouns or acronyms; lowercase the rest. *Yes:* "Sales support", "Chief of staff", "Inventory manager", "Marketing & SEO" (SEO stays uppercase as an acronym), "Edit persona", "Approve all". *No:* "Sales Support", "Chief Of Staff", "Inventory Manager", "Edit Persona", "Approve All". The one deliberate exception is the eyebrow label style (uppercased via `text-transform`, e.g., "FLEET ROSTER") — that's a typographic treatment, not a casing convention; the underlying string should still be sentence case.

## Layout

`Stack` from `@wordpress/ui` is the default layout primitive. Reach for plain CSS (grid, `position: sticky`) only where Stack doesn't fit.

### The frame

Every WooAgent screen lives in the same frame:

- **`LeftNav` (dark surface)** — vertical sidebar, persistent across screens. WooAgent wordmark + nav. Uses WPDS dark surface tokens.
- **`TopBar` (light, thin)** — page-aware header. Breadcrumbs left, search center, "Ask Agent" trigger right.
- **Content area (light surface)** — generous padding, max width that respects WPDS dimension tokens. Whitespace > density.
- **`ActionBar` (sticky bottom, when applicable)** — for review / approve / batch actions. Indigo primary CTA right, secondary actions left.

### Queues, not dashboards

WooAgent's primary work surface is a **queue** — table or kanban — not a KPI dashboard. "Today's marketing queue" is the canonical pattern: page heading → optional 1–2 metric strip → list of work items. Operators scan; they don't stare at six tiles of mixed data.

When KPIs are needed (e.g., the review-and-approve summary in 13.1), they go in a flat horizontal strip above the work, not in card grids.

## Elevation

WPDS elevation tokens (`--wpds-elevation-{xs, sm, md, lg}`) only. **Default to flat surfaces.** Cards in WooAgent are bordered, not shadowed. Modals and drawers use `--wpds-elevation-md`. The sidebar is on its own layer — no shadow needed; the surface color separates it.

No glow effects. No backdrop blur. No glassmorphism.

## Shapes

WPDS border-radius tokens. Two WooAgent-specific notes:

- **Persona avatars are squares with a small radius** (`--wpds-border-radius-sm`). Not circles. The squareness is part of the identity — it's how the persona reads at 14–24px.
- **Kind pills are full-radius** (`--wpds-border-radius-full`). They are the *only* full-radius surface in the UI; they read as labels because of it.

Cards use the standard WPDS card radius. No hand-tuned per-component radii.

## Components

These are the canonical WooAgent components. When a screen needs one, use the existing component — don't fork.

### `PersonaAvatar` (`ui/src/components/PersonaAvatar.tsx`)

Square swatch with the persona's `bg` and `ink` colors and the persona's initials. Sizes: `xs` (14px), `sm` (18px, default), `md` (24px). Appears in three places: the sidebar header tile, queue card corners, and page eyebrows. Identity element. Never used decoratively.

### `LeftNav` (`ui/src/components/LeftNav.tsx`)

Dark vertical sidebar. WooAgent wordmark at top, nav items below. Persistent across screens. The only place `PersonaAvatar` appears in the chrome.

### `ActionBar` (`ui/src/components/ActionBar.tsx`)

Sticky bottom bar on review / approve / batch surfaces. Indigo primary CTA right, secondary actions left. Replaces inline per-row action buttons in batch contexts. The 13.1 review-and-approve view is the reference.

### `Kpi` (`ui/src/components/Kpi.tsx`)

Single metric block: large tabular numeral, label below. Used in horizontal strips of 2–4 above queues and review surfaces. Never the *only* content on a screen.

### `StatusBadge` (`ui/src/components/StatusBadge.tsx`)

Uses WPDS intent variants only (`high`, `medium`, `low`, `stable`, `informational`, `draft`, `none`). No persona color. No custom hex.

### `AskAgentDrawer` (`ui/src/components/AskAgentDrawer.tsx`)

Right-side drawer triggered from `TopBar`. Conversational chat with the relevant persona. Streaming is a single steady spinner — no typewriter, no shimmer.

### `EditPersonaModal` (`ui/src/components/EditPersonaModal.tsx`)

WPDS `Modal`. Form for editing an agent's name, role description, and persona-specific settings. The `PersonaAvatar` (`md`) anchors the top.

### Kind pill

Small full-radius pill on queue cards: `CONTENT` / `CAMPAIGN` / `EMAIL`, colored by the owning persona's `bg` + `ink`. The second of the two persona-color exception sites.

### Onboarding card (`ui/src/onboarding/`)

Centered card on a neutral background, WooAgent wordmark above, sequential `Stepper` below the heading. Used for first-run setup (the 13.2 flow). Light surface, indigo primary CTA, no persona color (no agent identity yet at this stage). The stepper is conditional — three steps in the embedded UI, four in Vite dev.

### `PageGlobalActions` (`ui/src/components/PageGlobalActions.tsx`)

Right-side actions slot shared across every WPDS `<Page>` in WooAgent: the global search input (stub for V1) and the "Ask agent" button. Each screen passes it via Page's `actions` prop so heading + search + Ask agent always sit on a single horizontal band. Replaces the prior standalone `TopBar` component, which has been retired.

## Component inventory

The canonical WPDS + library components in use across `ui/`. **Reach for one of these before writing custom UI.** If something here doesn't fit, escalate in #design-systems before forking — and add a `// CUSTOM:` comment per the rule below.

### `@wordpress/admin-ui` — page-level layout

- **`Page`** — every screen wraps its content in `<Page title subTitle actions>`. Provides the heading + actions row on a single horizontal band. The `actions` slot always receives `<PageGlobalActions onAskAgent={…} />` for consistency. Optional slots: `breadcrumbs`, `badges`, `visual`, `headingLevel`.

### `@wordpress/ui` — primary surfaces, layout, type, status

- **`Badge`** — status/identity pill. Allowed intents: `high`, `medium`, `low`, `none`, `stable`, `informational`, `draft`. (Used: agent online indicator, status column on the roster, kind pill via `StatusBadge`.)
- **`Card.Root`** / **`Card.Header`** / **`Card.Content`** — bordered surface for grouped content. Wraps form sections (Settings) and proposal panels (IssueDetail).
- **`Notice.Root`** + **`Notice.Description`** + **`Notice.Actions`** + **`Notice.ActionButton`** + **`Notice.CloseIcon`** — compound notice component (intents: `neutral`, `info`, `warning`, `success`, `error`). The legacy `Notice` from `@wordpress/components` is **not** used; all notices are the compound form.
- **`Stack`** — default layout primitive (flex with token-based gaps). Reach for this before plain CSS flex.
- **`Text`** — typographic primitive. Variants: `heading-2xl` … `heading-xs`, `body-md`, `body-sm`. Always pair with a real heading element via `render={<h1 />}` when it's a page heading.

### `@wordpress/components` — gap-fillers (forms, controls, utilities)

- **`Button`** — primary/secondary/tertiary actions and icon-only buttons (`Button icon={…} label="…"`). Use in place of any `<button>`.
- **`Spinner`** — async-loading indicator.
- **`TextControl`** / **`SearchControl`** / **`SelectControl`** — text input, search input, and select dropdown. Use in place of any `<input>` / `<select>`.
- **`FormToggle`** — on/off boolean toggle (used inline in the agent roster's "Enabled" column).
- **`Modal`** — modal dialog (used by `EditPersonaModal`).
- **`ExternalLink`** — outbound URL with built-in icon and `rel="noopener"`. Use in place of any `<a href>` for external destinations.
- **Internal-router `Link`** — `react-router-dom` `<Link>` is the canonical WooAgent in-app link. (No `@wordpress/components` `Link` is currently in use; `react-router-dom` ownership of routing makes it a better fit.)

### `@wordpress/dataviews` — tabular UIs

- **`DataViews`** + **`filterSortAndPaginate`** + types `Action`, `Field`, `View` — used by the agent roster (Agents) and is the canonical building block for any future data-table screen. Bulk-select is suppressed app-wide by omitting `supportsBulk` on actions; layouts default to table-only via `defaultLayouts={{ table: {} }}` unless a specific screen needs grid/list.

### `@wordpress/icons`

- Use the named icon exports (e.g., `comment`, `chevronDown`, `chevronUp`, `close`, `plus`, `funnel`, `inbox`, `columns`, `people`, `category`, `box`, `store`, `shield`, `key`, `external`, `rotateRight`, `check`, `moreVertical`). Always render via `<Icon icon={iconName} size={…} />`.

### WooAgent components (`ui/src/components/`)

Project-specific composites that wrap or extend the above. See the **Components** section above for descriptions: `PersonaAvatar`, `LeftNav`, `ActionBar`, `Kpi`, `StatusBadge`, `AskAgentDrawer`, `EditPersonaModal`, `PageGlobalActions`. Reach for these before re-implementing similar shapes.

### Out of scope

- `@wordpress/components` `Notice` (legacy single-component form) — use the `@wordpress/ui` compound `Notice.Root` instead.
- `TopBar` — removed; replaced by `Page` + `PageGlobalActions`.
- Raw `<button>` / `<input>` / `<select>` / `<a href>` / mono-style spans — every one needs a `// CUSTOM:` comment immediately above explaining why no WPDS component fits.

## Do's and Don'ts

### Don't expand the persona-color exception

Persona color appears in `PersonaAvatar` and the kind pill on cards. **That's it.** A future need for a third persona-colored surface should be redirected to a WPDS intent variant or a neutral surface. Broadening this makes the UI feel costumed.

### No monospace fonts

Never use `--wpds-typography-font-family-mono` or any code-style font in the operator UI. This includes model names, hostnames, persona slugs, IDs, and any other identifier-shaped strings. Body font for everything. See Typography for the rationale; the short version is "mono made every surface we tried it on look like a developer console."

### Don't add streaming flourishes

No typewriter effects. No shimmer gradients. No pulsing avatars. No glow rings around active agents. When an agent is working, use a single steady spinner with a plain label ("Working on those rewrites for you…"). Streaming is matter-of-fact.

### Indigo is the only primary-action color

Every primary CTA — approve, save, continue, send, retry — is WPDS indigo. Persona colors never become buttons. If a CTA needs to be elevated, use scale or position, not color.

### Only WPDS components — call out anything custom

Every shell, control, surface, and form element MUST be a WPDS component (`@wordpress/ui` or `@wordpress/components`). **No exceptions without a flag.** Concretely:

- No bare `<button>`, `<input>`, `<select>`, or `<a>` with custom CSS.
- No hand-rolled badges, pills, dropdowns, or mono-text spans.
- No custom-styled clones of components that already exist in WPDS.

Before drawing anything custom: check WPDS via the MCP server (`mcp__wordpress-design-system__get_components`), then check `@wordpress/ui` and `@wordpress/components` Storybook. If nothing fits, raise it in #design-systems before forking.

**If something must be custom, the code MUST include a `// CUSTOM:` comment immediately above it** explaining: (a) why no WPDS component fits, (b) what's custom about it, (c) where it's documented (DESIGN.md, a P2, an issue). Reviewers should reject custom UI that isn't called out this way. The persona avatars and the kind pill are the two pre-approved customs (see Colors); anything else is new territory and needs a flag.

### Sticky action bar for batch actions

Multi-item approval, rejection, or any batch operation goes in `ActionBar` at the bottom of the screen — not in inline per-row buttons. The 13.1 review-and-approve view is the reference.
