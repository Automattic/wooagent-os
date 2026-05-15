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

## Working on UI

A behavioral lens for customer-facing UI work in `ui/`. Apply when the change touches a screen, page, modal, component, copy string, or operator-visible flow. Skip it for daemon-side work, build tooling, or purely internal refactors.

**Execute it as a sequence**, not as background reference: state assumptions, walk the pattern-matching tier order below, ask the smallest-version question, run the verify-before-done checklist, and surface designer-review triggers by name. Other workflows (brainstorming clarifying questions, mockup tools, visual companions) come *after* the lens, not instead of it. If you're explicitly asked to skip the lens, say so out loud before proceeding.

### State your assumptions before building

Before writing UI code, surface what you're assuming and tag each as **confident**, **assuming**, or **unclear**:

- **Visual treatment** — placement, hierarchy, which WPDS component, which intent variant.
- **Scope** — what's in this change, what's deliberately not.
- **Edge cases** — empty, loading, error, over-full; narrow widths (≥320px).
- **Adjacent impact** — what else on the page or in the flow this touches.

Ask the user to confirm anything tagged **assuming** or **unclear** before writing code. This catches the implicit choices a designer would have flagged in review, at a much lower cost.

### Pattern matching: where to look first

When choosing a component, layout, or interaction for a customer-facing change, look in this order — don't skip a tier:

1. **`@wordpress/admin-ui`** — page-level shell. `Page` is the only component currently used; every screen wraps content in `<Page title subTitle actions hasPadding>`.
2. **`@wordpress/ui`** — primary surfaces, layout, typography, status. `Card.Root` / `Stack` / `Text` / `Badge` / `Notice.Root` (compound). The default first stop for new UI.
3. **`@wordpress/components`** — gap-fillers (`Button`, `Modal`, `Spinner`, `TextControl` / `SearchControl` / `SelectControl`, `FormToggle`). Always verify the component's **Status** in Storybook is `stable` — query the WPDS MCP server (`mcp__wordpress-design-system__get_component_details`) if uncertain. Pass `__next40pxDefaultSize` to every `Button`. **For outbound text links, use `Button variant="link" target="_blank" rel="noreferrer noopener"` with a 16px `arrowUpRight` icon in children** — not `ExternalLink` (superseded), and not the `external` box-with-arrow icon (wrong shape).
4. **`@wordpress/dataviews`** — when the shape is tabular. The agent roster and abilities screens are the references.
5. **WooAgent composites** in `ui/src/components/` — `PersonaAvatar`, `LeftNav`, `ActionBar`, `Kpi`, `StatusBadge`, `AskAgentDrawer`, `EditPersonaModal`, `PageGlobalActions`. Reach for these before re-implementing a similar shape.
6. **Bespoke with a `// CUSTOM:` comment** — last resort. The comment must explain (a) why no WPDS component fits, (b) what's custom about it, (c) where it's documented (DESIGN.md, a P2, an issue). Reviewers reject custom UI that isn't called out.

Query the WPDS MCP (`mcp__wordpress-design-system__get_components`, `…__get_design_tokens`) before guessing whether a tier 2 / 3 component or token exists. The principle: **don't shout against the WPDS** — a screen built from bespoke divs inside a WPDS app reads as the one wrong note on the page.

### Smallest version

Before adding a new field, option, tab, modal, or screen, ask: **what is the smallest change that solves the problem?** New surface area has a cost — every option dilutes the queue, every modal slows the flow, every tab forces a navigation decision. The agent personas earn their separation; other UI generally doesn't. A new filter is cheaper than a new queue. A reused empty state is cheaper than a new one. A WPDS intent variant is cheaper than a new `// CUSTOM:` badge.

### Verify before claiming done

When wrapping up a customer-facing change, verify each item below or confirm it with the user. Flag anything missing.

- **Hierarchy** — primary action visually dominant; secondary looks secondary.
- **Alignment** — elements line up; `hasPadding` on `<Page>`; row controls share the 40px height.
- **Spacing** — `--wpds-dimension-*` tokens, not magic pixels; `Stack` gaps over hand-rolled margins.
- **Copy** — sentence case everywhere user-facing; plain language; warm body voice, calm chrome.
- **States** — hover, focus, disabled, loading, empty, error all considered. Steady spinner for activity, no streaming flourishes.
- **Tokens** — every color is `--wpds-*` or `--wa-persona-*`; no hex literals. Body font on identifiers; no mono.
- **A11y basics** — keyboard navigable, visible focus, sufficient contrast, labelled inputs, sensible heading order.
- **Responsive** — works at ≥320px and wide; nothing clips; the action bar stays pinned; content reflows.
- **Localisation** — strings survive ~1.5× expansion without breaking the layout (German is the canonical worst case).
- **WPDS-first** — components sourced from `@wordpress/ui` / `@wordpress/components`; any custom drawing carries a `// CUSTOM:` comment.

### When to loop in a designer

Don't block on review, but tell the user the change is worth a designer's eye before merging if it touches any of the following:

- A new screen, modal, or full page (not just an addition to an existing one).
- A new pattern or component WPDS doesn't have yet — i.e., something headed for a `// CUSTOM:` comment.
- More than a sentence or two of customer-facing copy (empty-state body, onboarding step description, error explanation).
- Information architecture changes — moving items between the sidebar, the page header, settings groups, or modal tabs.
- A primary CTA for a flow — the button that completes the operator's job (approve, send, publish, run).
- Anything touching onboarding, the persona system, or the approve / review surface — the highest-trust moments in the product.
- Persona-color usage beyond the two pre-approved sites (`PersonaAvatar` + kind pill).

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
2. The kind pill on cards (CONTENT / CAMPAIGN / EMAIL) — colored by the owning persona. The canonical wrapper is `KindBadge` in `StatusBadge.tsx`; the supporting CSS lives in `app.css`.

The brand-header "W" tile in `LeftNav` previously sat in this exception list (using the marketing persona color), but moved to WPDS brand indigo (`--wpds-color-bg-interactive-brand-strong`) per the i3.2 Figma. The persona-color exception is now scoped to identity surfaces only.

**Don't expand this exception further.** A third persona-colored surface should be redirected to a WPDS intent variant or a neutral surface. The exception is small on purpose — broadening it makes the UI feel costumed.

### Sidebar surface color (WooAgent-owned)

The left nav uses two values that have no direct WPDS equivalent: `#1e1e1e` for the sidebar surface (matches the i3.2 Figma and is the same value WPDS uses internally for `--wpds-color-bg-interactive-neutral-strong` / its primary-button background — borrowed here for the inverse surface), and `rgba(56, 88, 233, 0.12)` for nav-link hover and active states (12%-opacity indigo on the near-black bg, also from the Figma). Both values live **only** in `.wa-sidebar` rules in `app.css` — nowhere else in the app. If WPDS adds a true dark-brand surface token + an inverse-interactive token later, swap to them.

### Primary action

Primary CTAs are **WPDS indigo** — the same color across every screen, every persona context, every depth of navigation. Persona color never appears as a button fill, hover state, or focus ring. Approve, save, continue, send, retry: all indigo. If a CTA needs to be elevated, use scale or position, not color.

## Typography

WPDS provides the type system. Use only `--wpds-typography-font-family-{body, heading}` and the `--wpds-typography-font-size-*` / `--wpds-typography-line-height-*` / `--wpds-typography-font-weight-*` scales. No `Inter`. No `system-ui`. No bespoke type ramps.

**No monospace fonts anywhere — with one documented exception.** Don't use `--wpds-typography-font-family-mono`. Don't reach for `Menlo`, `SF Mono`, `Consolas`, or any other code-style face. Identifiers like model names (`anthropic/claude-sonnet-4-6`), hostnames (`localhost:7777`), and slugs (`marketing`, `pricing`) render in the body font. Mono creates a "developer console" feel that conflicts with the calm-coworker personality — and in practice, every place we tried mono (the model picker, the daemon hostname, the sidebar badge) became the *most visually jarring* element on its surface. If something is genuinely code, wrap it in `<code>` only when it's part of a documentation context; in the operator UI, plain body text is correct.

**The pairing-code exception.** The onboarding pairing-code display (`.wa-onboarding-pairing-code` in `Step2Store.tsx`) renders in `--wpds-typography-font-family-mono`. Pairing codes are verbatim short tokens (e.g. `W00A-3HSS-7YHC`) where character disambiguation (`0` vs `O`, `1` vs `l` vs `I`) matters more than visual harmony with body copy — the operator is reading the code aloud or retyping it. This is the only mono usage in the app; do not expand it.

Two WooAgent-specific notes:

- **Tabular numerals on KPIs and metric strips** (`Kpi`). Numbers that change shouldn't jiggle horizontally. Use `font-variant-numeric: tabular-nums` on the metric value — this is *not* a mono font, it's a body-font feature.
- **Heading font for agent display names** even in body contexts (sidebar, page headers, persona pills). Agent names carry identity — they should stand out from running prose.
- **Sentence case for everything user-facing.** Persona names, page titles, button labels, menu items, modal titles, table headers, empty-state copy. Capitalize the first word and any proper nouns or acronyms; lowercase the rest. *Yes:* "Sales support", "Chief of staff", "Inventory manager", "Marketing & SEO" (SEO stays uppercase as an acronym), "Edit persona", "Approve all". *No:* "Sales Support", "Chief Of Staff", "Inventory Manager", "Edit Persona", "Approve All". The one deliberate exception is the eyebrow label style (uppercased via `text-transform`, e.g., "FLEET ROSTER") — that's a typographic treatment, not a casing convention; the underlying string should still be sentence case.

## Layout

`Stack` from `@wordpress/ui` is the default layout primitive. Reach for plain CSS (grid, `position: sticky`) only where Stack doesn't fit.

### The frame

Every WooAgent screen lives in the same frame:

- **`LeftNav` (dark surface)** — vertical sidebar, persistent across screens. WooAgent wordmark + nav. Uses the near-black + translucent-indigo hex pair documented under "Sidebar brand color"; everything inside (persona avatar, badges, eyebrows) still uses WPDS / persona tokens.
- **`Page` (light, thin)** — page-aware header from `@wordpress/admin-ui`. Title + subtitle on the left, `actions` slot on the right. Every screen wraps its content in `<Page>` and passes the shared `<PageGlobalActions>` (global search + Ask agent) into its `actions` slot, so the heading + search + Ask agent always sit on a single horizontal band. **Always pass `hasPadding`** so the content area inherits the same horizontal token (`--wpds-dimension-padding-2xl`) as the header — without it, body content sits flush left and the leftmost element no longer aligns with the title.
- **Content area (light surface)** — rendered as `<Page>` children. Generous padding, max width that respects WPDS dimension tokens. Whitespace > density.
- **`ActionBar` (pinned to viewport bottom, when applicable)** — for review / approve / batch actions. Indigo primary CTA right, secondary actions left. Lives inside the **detail-shell layout** (`.wa-detail-shell` flex column + `<Page className="wa-detail-shell-page">`) — the shell is `height: 100vh`, the Page grows via `flex: 1; min-height: 0;`, and the ActionBar is the last sibling so it sits at the bottom of the column = bottom of the viewport regardless of content length. Horizontal padding on the bar uses the same `--wpds-dimension-padding-2xl` token as the Page content, so the bar's badge and buttons line up with the title. The 13.1 review-and-approve view is the reference.

### Queues, not dashboards

WooAgent's primary work surface is a **queue** — table or kanban — not a KPI dashboard. "Today's queue" is the canonical pattern: page heading → optional 1–2 metric strip → list of work items. Operators scan; they don't stare at six tiles of mixed data.

The board is **cross-agent** — it holds work staged by every agent in the fleet, not just marketing. Persona-filtered views (`?persona=…`) get per-agent copy; the default board copy stays plural ("Everything your agents have staged for you…"). Don't slip back into marketing-specific phrasing on the default view.

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

Bottom bar on review / approve / batch surfaces, pinned to the viewport via the detail-shell layout (see Layout > The frame). Indigo primary CTA right, secondary actions left. Replaces inline per-row action buttons in batch contexts. The 13.1 review-and-approve view is the reference.

### `Kpi` (`ui/src/components/Kpi.tsx`)

Single metric block: large tabular numeral, label below. Used in horizontal strips of 2–4 above queues and review surfaces. Never the *only* content on a screen.

### `SectionHeader` (`ui/src/components/SectionHeader.tsx`)

Shared eyebrow + optional title / inline badge / right-aligned meta or action row. Used inside `<Card.Header>` across the three issue-detail views (`IssueDetail` prose path, `PriceIssueView`, `MessageIssueView`) and standalone above the variant list. Replaces the recurring `Stack direction="row" justify="space-between" align="center"` shape that those headers previously hand-rolled. Carries a `// CUSTOM:` comment — composes three WPDS primitives (`Stack`, eyebrow span, `Text`); no WPDS section-header component exists.

### `StatusBadge` (`ui/src/components/StatusBadge.tsx`)

Uses WPDS intent variants only (`high`, `medium`, `low`, `stable`, `informational`, `draft`, `none`). No persona color. No custom hex.

### `AskAgentDrawer` (`ui/src/components/AskAgentDrawer.tsx`)

Right-side drawer triggered from the "Ask agent" button in `PageGlobalActions` (or globally via ⌘K / Ctrl+K). Conversational chat with the relevant persona. Streaming is a single steady spinner — no typewriter, no shimmer.

### `EditPersonaModal` (`ui/src/components/EditPersonaModal.tsx`)

WPDS `Modal`. Form for editing an agent's name, role description, and persona-specific settings. The `PersonaAvatar` (`md`) anchors the top.

### Kind pill

Small full-radius pill on queue cards: `CONTENT` / `CAMPAIGN` / `EMAIL`, colored by the owning persona's `bg` + `ink`. The second of the two persona-color exception sites.

### Onboarding card (`ui/src/onboarding/`)

Centered card on a neutral background, WooAgent wordmark above, sequential `Stepper` below the heading. Used for first-run setup (the 13.2 flow). Light surface, indigo primary CTA, no persona color (no agent identity yet at this stage). The stepper is conditional — three steps in the embedded UI, four in Vite dev.

### `PageGlobalActions` (`ui/src/components/PageGlobalActions.tsx`)

Right-side actions slot shared across every WPDS `<Page>` in WooAgent: an optional global search input (stub for V1) and the "Ask agent" button. Each screen passes it via Page's `actions` prop so heading + search + Ask agent always sit on a single horizontal band. Replaces the prior standalone `TopBar` component, which has been retired.

**Pages that render their own search via DataViews (Agents, Abilities) pass `showSearch={false}`** to drop the redundant top-bar search. Don't carry two search inputs on the same screen — DataViews owns the in-table filter when it's there.

## Component inventory

The canonical WPDS + library components in use across `ui/`. **Reach for one of these before writing custom UI.** If something here doesn't fit, escalate in #design-systems before forking — and add a `// CUSTOM:` comment per the rule below.

### `@wordpress/admin-ui` — page-level layout

- **`Page`** — every screen wraps its content in `<Page title subTitle actions>`. Provides the heading + actions row on a single horizontal band. The `actions` slot always receives `<PageGlobalActions onAskAgent={…} />` for consistency. Optional slots: `breadcrumbs`, `badges`, `visual`, `headingLevel`.

### `@wordpress/ui` — primary surfaces, layout, type, status

- **`Badge`** — status/identity pill. Allowed intents: `high`, `medium`, `low`, `none`, `stable`, `informational`, `draft`. (Used: status column on the agent roster, kind pill via `StatusBadge`, the in-review counter on the sidebar's "Board" item.)
- **`Card.Root`** / **`Card.Header`** / **`Card.Content`** — bordered surface for grouped content. Wraps form sections (Settings) and proposal panels (IssueDetail).
- **`Notice.Root`** + **`Notice.Description`** + **`Notice.Actions`** + **`Notice.ActionButton`** + **`Notice.CloseIcon`** — compound notice component (intents: `neutral`, `info`, `warning`, `success`, `error`). The legacy `Notice` from `@wordpress/components` is **not** used; all notices are the compound form.
- **`Stack`** — default layout primitive (flex with token-based gaps). Reach for this before plain CSS flex.
- **`Text`** — typographic primitive. Variants: `heading-2xl` (32px), `heading-xl` (20px), `heading-lg` (15px), `heading-md` (13px), `heading-sm` (11px); body: `body-xl`, `body-lg`, `body-md` (13px), `body-sm` (12px). **Watch out for `heading-sm`** — it's the **uppercase 11px eyebrow** style with `text-transform: uppercase` applied, *not* a small heading. The smallest non-uppercase heading is `heading-md`; use that for in-card titles and other small headings. There is no `heading-xs`. Always pair with a real heading element via `render={<h1 />}` when it's a page heading.

### `@wordpress/components` — gap-fillers (forms, controls, utilities)

- **`Button`** — primary/secondary/tertiary actions, icon-only buttons (`Button icon={…} label="…"`), **and text links** (`Button variant="link" href="…" target="_blank" rel="noreferrer noopener"` with a 16px `arrowUpRight` icon from `@wordpress/icons` rendered inline in children for outbound links). Use in place of any `<button>` or `<a href>` for both interactive controls and inline text links. **Always pass `__next40pxDefaultSize` for non-link variants** — this opts into the canonical 40px height the rest of the WPDS form controls use; without it Button renders at the legacy ~32px size and looks short next to a default `SearchControl` or `InputControl`. **Pass icons via the `icon` prop directly** (e.g., `icon={comment}`) — don't wrap them in `<Icon icon={comment} size={…} />`. Button's `icon` prop takes the icon definition and handles sizing itself; manually wrapping bypasses Button's icon-size handling. **Outbound-link icon:** use `arrowUpRight` from `@wordpress/icons` (the clean arrow shape that matches the Figma). Do NOT use the `external` icon — that's the box-with-arrow style which doesn't match. **No underline on text links** — WPDS Button `is-link` ships with `text-decoration: underline`; the project overrides it to `none` globally in `app.css`. Brand color + the arrow icon are enough to read as a link without underline noise.
- **`Spinner`** — async-loading indicator.
- **`TextControl`** / **`SearchControl`** / **`SelectControl`** — text input, search input, and select dropdown. Use in place of any `<input>` / `<select>`. **Use the default size** (40px) so they align with `Button` + `__next40pxDefaultSize` on the same row. Don't pass `size="compact"` unless you genuinely want a smaller control — and if you do, the *whole* row needs to be compact, not just one element.
- **`FormToggle`** — on/off boolean toggle (used inline in the agent roster's "Enabled" column).
- **`Modal`** — modal dialog (used by `EditPersonaModal`).
- **Internal-router `Link`** — `react-router-dom` `<Link>` is the canonical WooAgent in-app link. (No `@wordpress/components` `Link` is currently in use; `react-router-dom` ownership of routing makes it a better fit.)

### `@wordpress/dataviews` — tabular UIs

- **`DataViews`** + **`filterSortAndPaginate`** + types `Action`, `Field`, `View` — used by the agent roster (Agents) and is the canonical building block for any future data-table screen. Bulk-select is suppressed app-wide by omitting `supportsBulk` on actions; layouts default to table-only via `defaultLayouts={{ table: {} }}` unless a specific screen needs grid/list.

### `@wordpress/icons`

- Use the named icon exports (e.g., `comment`, `chevronDown`, `chevronUp`, `close`, `plus`, `funnel`, `inbox`, `columns`, `people`, `category`, `box`, `store`, `shield`, `key`, `external`, `rotateRight`, `check`, `moreVertical`).
- For **standalone icons**: render via `<Icon icon={iconName} size={…} />`.
- For **Button's `icon` prop**: pass the icon definition directly (`<Button icon={comment}>…</Button>`) — Button handles sizing internally.

### WooAgent components (`ui/src/components/`)

Project-specific composites that wrap or extend the above. See the **Components** section above for descriptions: `PersonaAvatar`, `LeftNav`, `ActionBar`, `Kpi`, `SectionHeader`, `StatusBadge`, `AskAgentDrawer`, `EditPersonaModal`, `PageGlobalActions`. Reach for these before re-implementing similar shapes.

### Out of scope

- `@wordpress/components` `Notice` (legacy single-component form) — use the `@wordpress/ui` compound `Notice.Root` instead.
- `@wordpress/components` `ExternalLink` — superseded; use `Button variant="link" target="_blank" rel="noreferrer noopener"` with an inline `arrowUpRight` icon for outbound text links so they pick up brand color and WPDS link styling automatically.
- `TopBar` — removed; replaced by `Page` + `PageGlobalActions`.
- Raw `<button>` / `<input>` / `<select>` / `<a href>` / mono-style spans — every one needs a `// CUSTOM:` comment immediately above explaining why no WPDS component fits.

## Do's and Don'ts

### Don't expand the persona-color exception

Persona color appears in `PersonaAvatar` and the kind pill on cards (`KindBadge` + supporting `app.css` rules). **That's it.** The `LeftNav` brand "W" tile used to sit here but moved to WPDS brand indigo (`--wpds-color-bg-interactive-brand-strong`) per the i3.2 Figma. The `.wa-eyebrow--persona` modifier (pink eyebrows on "Proposed", "Rationale", "Price change", "Customer-facing message") used to be a third de-facto site; it was removed when aligning the Marketing detail to the 2.0/Single Product Figma frame (2026-05-15). A future need for a third persona-colored surface should be redirected to a WPDS intent variant or a neutral surface. Broadening this makes the UI feel costumed.

### No monospace fonts (one exception)

Never use `--wpds-typography-font-family-mono` or any code-style font in the operator UI. This includes model names, hostnames, persona slugs, IDs, and any other identifier-shaped strings. Body font for everything. See Typography for the rationale; the short version is "mono made every surface we tried it on look like a developer console."

**The single allowed exception** is the onboarding pairing-code display (`.wa-onboarding-pairing-code` in `Step2Store.tsx`) — a verbatim short token where character disambiguation outweighs visual harmony. Don't expand this exception to other identifiers.

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

**If something must be custom, the code MUST include a `// CUSTOM:` comment immediately above it** explaining: (a) why no WPDS component fits, (b) what's custom about it, (c) where it's documented (DESIGN.md, a P2, an issue). Reviewers should reject custom UI that isn't called out this way. The same `CUSTOM:` marker pattern is the escape hatch for any rule with a genuine WPDS gap — including raw HTML interactive elements (no WPDS wrapper fits) and bespoke font-sizes (no WPDS token matches). The persona avatars and the kind pill are the two pre-approved persona-color customs (see Colors); anything else is new territory and needs a flag.

### Sticky action bar for batch actions

Multi-item approval, rejection, or any batch operation goes in `ActionBar` at the bottom of the screen — not in inline per-row buttons. The 13.1 review-and-approve view is the reference. Use the **detail-shell layout** (`.wa-detail-shell` + `<Page className="wa-detail-shell-page">`) so the bar pins to viewport bottom, not page-content bottom.

### Match control heights on the same row

Anything that sits next to a `Button` on a row must share its height. The default WPDS `Button` is the legacy ~32px size; opt every Button into 40px via `__next40pxDefaultSize`, and use the *default* size on `SearchControl`/`InputControl`/`SelectControl` (40px) — not `size="compact"`. If you find yourself overriding control heights with CSS, that's the cue to revisit the row's component choices instead.

### Don't double up on search

If a screen renders DataViews (which carries its own filter input), pass `showSearch={false}` to `<PageGlobalActions>` — two search inputs on the same page is always wrong. The same rule applies to any future in-content search affordance: only one search per screen.

### Page content alignment

Always pass `hasPadding` to `<Page>`. Without it, body content sits flush left while the title sits at `--wpds-dimension-padding-2xl` — your leftmost card / table / form will visually misalign with the heading. The exception is when content is already padded by another container (DataViews, Onboarding's centered card, etc.) — and even there, `hasPadding` is rarely wrong.
