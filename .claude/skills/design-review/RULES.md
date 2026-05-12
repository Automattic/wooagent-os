# WooAgent design-review rules

The rule set used by the `design-review` skill. Grounded in `DESIGN.md` (repo root) and `CLAUDE.md`. When `DESIGN.md` changes, update this file to match.

The WPDS MCP server (`@wordpress/design-system-mcp`) is the live source of truth for component status and token names — when in doubt, query it via `mcp__wordpress-design-system__get_component_details` and `…__get_design_tokens`.

## Severity reference

| Severity | Meaning |
|---|---|
| **Violation** | Breaks a clear rule below. Must fix before merge. |
| **Warning** | Likely wrong but context-dependent. Developer should verify. |
| **Flag for designer** | New pattern not covered by existing rules. Needs designer input. |

## `CUSTOM:` marker syntax

When a rule requires a `CUSTOM:` comment above a custom element, the comment style must match the surrounding language:

- **TS / JS code (outside JSX)** — `// CUSTOM: ...`
- **JSX child position (sibling of other JSX)** — `{/* CUSTOM: ... */}`
- **JS expression inside `return (...)`** — `// CUSTOM: ...` is valid (the parser is in JS-expression mode between `(` and the inner JSX)
- **CSS / SCSS** — `/* CUSTOM: ... */`

All three styles are equivalent for review purposes. The marker must appear within 2 lines above the custom element (an intervening `return (` line is allowed).

The comment body must cover:
- **(a)** why no WPDS component fits
- **(b)** what's custom about the element
- **(c)** where the pattern is documented (DESIGN.md / a P2 / an issue) or what the follow-up is

---

## Color rules

### Rule 1 — No hex literals (with explicit exceptions)

Every color in component code is a `--wpds-*` or `--wa-persona-*` CSS variable.

**Allowed hex:**
- Inside `var(--token, #fallback)` — standard fallback practice.
- Inside `:root` blocks in `ui/src/styles/app.css`, **only** for declaring `--wa-persona-*-bg` / `--wa-persona-*-ink` variables, and **only** when the hex matches the canonical values in `DESIGN.md` front matter. These declarations are the single source of truth for the persona palette — they have to live somewhere.
- Inside `.wa-sidebar` rules in `ui/src/styles/app.css` *only*: `#2C045D` (surface), `#1F0342` (hover/active), and `#ffffff` (foreground text on the brand-purple surface). The first two are the pre-approved brand-purple pair; white is the natural foreground on that surface. Anywhere outside `.wa-sidebar`, all three are violations.
- `DESIGN.md` front matter — documentation, not code (and `DESIGN.md` is in the skip list anyway).

**Violation:** any other hex value (`#[0-9a-fA-F]{3,8}\b`) in `.tsx`, `.ts`, `.jsx`, `.js`, `.css`, `.scss`.

### Rule 2 — Persona color usage is bounded

Persona-color variables (`--wa-persona-*-bg`, `--wa-persona-*-ink`) appear in **exactly two surfaces:**
1. The `PersonaAvatar` component (`ui/src/components/PersonaAvatar.tsx`).
2. The kind pill on queue cards (CSS rules in `ui/src/styles/app.css`).

**Violation:** persona-color variables referenced from any other component, button fill, hover state, focus ring, or surface.

### Rule 3 — Indigo is the only primary-action color

Primary CTAs (approve, save, continue, send, retry) use the default WPDS `Button` color (indigo). Persona color never becomes a button fill, hover, or focus ring.

**Violation:** `<Button>` with `style`/`className` overriding fill, hover, or focus to a persona-color or any non-WPDS color.

### Rule 4 — Status / intent via WPDS intent variants only

Status badges and notices use the WPDS intent system: `high`, `medium`, `low`, `none`, `stable`, `informational`, `draft`.

**Violation:** custom red / yellow / green hex for status indication. Use the intent variant.

---

## Typography rules

### Rule 5 — No monospace fonts

`--wpds-typography-font-family-mono`, `Menlo`, `SF Mono`, `Consolas`, `Roboto Mono`, and `font-family: monospace` are not used in the operator UI. Identifiers, model names, hostnames, and slugs render in the body font.

**Violation:** any of the above font-family strings.

### Rule 6 — No bespoke type ramps

Use `--wpds-typography-font-size-*`, `--wpds-typography-line-height-*`, `--wpds-typography-font-weight-*` exclusively. No `Inter`, no `system-ui`, no hardcoded `font-size`.

**Violation:** hardcoded `font-size: 14px` (etc.) or `font-family: Inter, ...`.

### Rule 7 — Sentence case for user-facing strings

Persona names, page titles, button labels, menu items, modal titles, table headers, empty-state copy use sentence case. Proper nouns and acronyms (SEO, KPI, UI, AI, URL, JSON) are preserved.

**Warning:** Title Case in `title=`, `subTitle=`, `label=`, `placeholder=` JSX props (e.g. `title="Edit Persona"` → should be `"Edit persona"`).

Eyebrow labels are an exception — the visual uppercase comes from `text-transform`; the underlying string is still sentence case.

---

## Component rules

### Rule 8 — No raw HTML for interactive elements

| Don't | Use |
|---|---|
| `<button>` | `Button` (from `@wordpress/components`) |
| `<input type="search">` | `SearchControl` |
| `<input type="text">` | `InputControl` / `TextControl` |
| `<select>` | `SelectControl` |
| `<a href>` (external) | `ExternalLink` |
| `<a href>` (in-app) | `react-router-dom` `Link` |

**Violation:** any raw element above without a `CUSTOM:` marker comment within 2 lines above (see "`CUSTOM:` marker syntax" at the top of this file for accepted forms — `// CUSTOM:`, `{/* CUSTOM: */}`, or `/* CUSTOM: */`).

### Rule 9 — `__next40pxDefaultSize` on every Button

Every `<Button>` call site passes `__next40pxDefaultSize` so it aligns with other 40px-tall controls on the same row.

**Violation:** `<Button>` without `__next40pxDefaultSize`.

### Rule 10 — Icon-only Buttons need a `label`

`<Button icon={…}>` with no text children must pass `label="…"` for accessibility.

**Violation:** icon-only `<Button>` with no `label` prop.

### Rule 11 — Icons via Button's `icon` prop, not wrapped

For buttons with icons: `<Button icon={iconName} label="…" />`. Don't wrap with `<Icon icon={…} size={…}>` inside the Button — Button's `icon` prop handles sizing.

**Warning:** `<Button>` containing `<Icon …>` as a child.

### Rule 12 — Default-size form controls

`SearchControl`, `InputControl`, `SelectControl`, `TextControl` use the default 40px size. `size="compact"` is allowed only if the whole row is compact.

**Warning:** `size="compact"` on a form control sharing a row with a 40px `Button`.

### Rule 13 — `hasPadding` on every `<Page>`

`<Page>` passes `hasPadding` so body content inherits the same horizontal token (`--wpds-dimension-padding-2xl`) as the header.

**Violation:** `<Page>` without `hasPadding` *and* without a child that owns its own padding (`<DataViews>`, onboarding centered card).

### Rule 14 — Compound `Notice` only

Notices use `Notice.Root` + `Notice.Description` + `Notice.Actions` + `Notice.ActionButton` + `Notice.CloseIcon` from `@wordpress/ui`. The legacy single-component `Notice` from `@wordpress/components` is **not** used.

**Violation:** `import { Notice } from '@wordpress/components'`.

### Rule 15 — Component status check

When importing from `@wordpress/components`, the component must have `stable` Status in Storybook. Query `@wordpress/design-system-mcp` (`mcp__wordpress-design-system__get_component_details`) for live status.

**Warning:** import from `@wordpress/components` of a component flagged experimental / unstable. Use `__experimentalHStack` / `__experimentalVStack` aliases only with explicit justification.

### Rule 16 — `Stack` over raw flex

For flex-based layouts, use `Stack` from `@wordpress/ui` over hand-rolled `display: flex` + `gap`. Reach for plain CSS flex only where `Stack` doesn't fit (`position: sticky`, custom grid templates).

**Warning:** `style={{ display: 'flex', ... }}` in JSX, or new `.wa-*` CSS rules with `display: flex` + `gap` where `Stack` would serve.

### Rule 17 — Cards bordered, not shadowed

WooAgent cards use the standard WPDS card surface (bordered). Default to flat surfaces. Elevation tokens (`--wpds-elevation-md`) apply to modals and drawers only.

**Warning:** `box-shadow` on a `Card.Root` or a `.wa-*` card-like surface.

### Rule 18 — No streaming flourishes

No typewriter effects. No shimmer gradients. No pulsing avatars. No glow rings around active agents. When an agent is working, use a `Spinner` with a plain label.

**Violation:** `@keyframes` named `*shimmer*`, `*pulse*`, `*typewriter*`, `*glow*` applied to agent-status surfaces.

**Warning:** other `@keyframes` rules in component-scoped CSS — verify they're not flourishes.

### Rule 19 — No double search

If a screen renders `<DataViews>` (which carries its own filter input), the same screen passes `showSearch={false}` to `<PageGlobalActions>` so there's only one search input.

**Violation:** screen imports `DataViews` *and* renders `<PageGlobalActions>` without `showSearch={false}`.

---

## Icon rules

### Rule 20 — Named exports from `@wordpress/icons`

Icons are imported as named exports from `@wordpress/icons` (e.g. `comment`, `chevronDown`, `close`, `plus`, `funnel`, `inbox`, `columns`, `people`, `box`, `store`, `shield`, `key`, `external`, `rotateRight`, `check`, `moreVertical`).

**Violation:** inline `<svg>` element in JSX without a `CUSTOM:` marker within 2 lines above.

---

## Accessibility rules

### Rule 21 — Inputs must be labelled

Every form control needs a visible label (the default) or `aria-label` when the label is conveyed visually elsewhere.

**Violation:** `InputControl` / `TextControl` / `SearchControl` / `SelectControl` with no `label` prop *and* no `aria-label`.

### Rule 22 — Visible focus

WPDS components handle focus by default. Custom interactive surfaces (anything with a `CUSTOM:` marker) need `&:focus-visible` styling.

**Warning:** `CUSTOM:` interactive element with no `:focus-visible` rule nearby.

---

## Voice and tone

### Rule 23 — Warm body, calm chrome; address the user

Empty states, errors, and helper text address the user directly ("you"). Active voice. Errors apologize without grovelling.

**Warning:** passive voice in user-facing strings (`"can be edited"`, `"is being processed"`).

### Rule 24 — Long customer-facing copy

More than a sentence or two of customer-facing copy (empty-state body, onboarding step description, error explanation) is a designer-review trigger per DESIGN.md.

**Flag for designer:** any user-facing string longer than ~140 characters.

---

## Out of scope (skip without flagging)

- Files in: `daemon/`, `cmd/`, `internal/`, `prompts/`, `skills/`, `spike-adk/`, `marketing-prototype/`, `node_modules/`, `dist/`, `build/`.
- Test files: `*.test.*`, `*.spec.*`, `__tests__/`, `__mocks__/`, `fixtures/`.
- Storybook stories: `*.stories.*`.
- Design system definition packages (`@wordpress/ui`, `@wordpress/components`, `@wordpress/admin-ui` source) — never flag.
- `DESIGN.md` and `CLAUDE.md` — documentation, not code.

---

## Confirmed-from-source

All rules above are grounded in:
- `DESIGN.md` (repo root) — visual identity, component inventory, anti-rolls, Do's and Don'ts.
- `CLAUDE.md` — UI-stack rules, anti-rolls table, `// CUSTOM:` requirement.
- ["Resources for the WordPress Design System"](https://designsystemsp2.wordpress.com/2026/04/20/resources-for-the-wordpress-design-system/) (Andrew Duthie, April 2026) — package preference (`@wordpress/ui` first, `@wordpress/components` to fill gaps), Storybook Status check, MCP server.
- WCAG 2.1 AA — accessibility minimums.

When `DESIGN.md` is amended, this file must be updated to match.
