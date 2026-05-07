# Building WooAgent's design language with DESIGN.md

Most design systems answer the question *"what does our default Card look like?"* Few of them answer *"what would feel out of place in our product?"* That second question is where personality lives — and personality is exactly what an AI coding agent needs in order to generate UI that feels like it belongs.

This week we wrote a `DESIGN.md` for WooAgent OS — a small file that captures the *layer above* WPDS: the voice, the few WooAgent-owned tokens (persona colors), and the rules that distinguish a WooAgent screen from a generic high-quality WP plugin. Then we used it to audit and tighten one screen — the Agent Roster — as the test case.

This post walks through:

- **Why** a DESIGN.md (vs leaning entirely on WPDS)
- **What** is actually in the file (the file itself is included below in full)
- **How** we linted it — and why two warnings were deliberate, not bugs
- **What changed** in the Agent Roster, with code and Figma before/after

End-to-end: about 90 minutes.

---

## Why a DESIGN.md

WPDS already answers most of the design questions any Automattic product faces. We don't need to redefine surfaces, intents, dimensions, type ramps, or radii — those exist, are tokenized, and we use them. CLAUDE.md has codified "use WPDS" as the rule for the codebase already.

But WPDS doesn't know:

- Whether our voice is **plainspoken-warm** or **enterprise-neutral**
- Whether our agents should feel like **trustworthy coworkers** or **a chat-window AI demo**
- That **persona color** (the seven agent identities) appears in exactly **two** places — the avatar squares and the kind pill on cards — and nowhere else
- That the canonical work surface is a **queue**, not a dashboard
- That **streaming is a steady spinner**, not a typewriter or shimmer

A coding agent picking up the codebase has WPDS docs and CLAUDE.md, but neither tells it any of the above. So while it produces clean WPDS code, the *aesthetic decisions* drift toward whatever the agent's training emphasized — usually "AI demo" patterns. The Agent Roster gets a pulsing avatar; the empty state gets a shimmery gradient; the streaming text gets a typewriter effect.

A DESIGN.md catches that drift — it's the file the agent reads before generating anything UI-related.

## The format

[Google Labs published a draft format for DESIGN.md](https://github.com/google-labs-code/design.md): YAML front-matter for machine-readable tokens, prose body for human-readable rationale.

```yaml
---
name: System name
colors: { ... }
typography: { ... }
spacing: { ... }
---

## Overview
## Colors
## Typography
## Layout
## Elevation & Depth
## Shapes
## Components
## Do's and Don'ts
```

The tokens are normative; the prose tells agents *why* and *when*.

For a self-contained design system, this is straightforward — define every color, every spacing unit, every shadow. But WooAgent isn't self-contained: WPDS already owns those tokens. So we faced a real choice.

## The layered-overlay decision

Two options:

1. **Resolved snapshot.** Copy the current WPDS values into the YAML — full token list, ~50 colors, full type ramp, etc. Lints cleanly, exports cleanly, but duplicates upstream state and rots when WPDS ships changes.
2. **Layered overlay.** YAML contains only WooAgent-owned tokens (the seven persona color pairs). Prose says "use WPDS for everything else." Honest about the architecture; goes stale slowly.

We chose **layered overlay**. It matches reality, and if WPDS evolves, our DESIGN.md doesn't lie about anything.

The cost is that the lint will flag the missing `primary` color and missing `typography` tokens. That's actually OK — we'll see why below.

## The DESIGN.md file (full)

Saved at `ui/DESIGN.md` in the WooAgent OS repo:

````markdown
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

**The rule:** every color in component code must be a `--wpds-*` CSS variable or a `--wa-persona-*` variable. Hex literals are a smell — they indicate that a WPDS token wasn't found, which usually means a WPDS lookup wasn't attempted.

### WPDS tokens (everything except persona)

- Surfaces — `--wpds-color-bg-surface-*`
- Foreground (text, icons) — `--wpds-color-fg-content-*`
- Strokes — `--wpds-color-stroke-*`
- Interactive — `--wpds-color-bg-interactive-*`
- Status / intent — WPDS intents (`high`, `medium`, `low`, `stable`, `informational`, `draft`, `none`). Never custom red / yellow / green hex values.

### Persona palette (WooAgent-owned)

The seven agent identities each have a `bg` / `ink` pair. **Used in two places only:**

1. The `PersonaAvatar` component — sidebar header, queue card corners, page eyebrows.
2. The kind pill on cards (CONTENT / CAMPAIGN / EMAIL) — colored by the owning persona.

**Don't expand this exception.** A future need for a third persona-colored surface should be redirected to a WPDS intent variant or a neutral surface.

### Primary action

Primary CTAs are **WPDS indigo** — the same color across every screen, every persona context, every depth of navigation. Persona color never appears as a button fill, hover state, or focus ring. Approve, save, continue, send, retry: all indigo.

## Typography

WPDS provides the type system. No `Inter`. No `system-ui`. No bespoke type ramps.

Two WooAgent-specific notes:

- **Tabular numerals on KPIs and metric strips.** Numbers that change shouldn't jiggle horizontally.
- **Heading font for agent display names** even in body contexts. Agent names carry identity — they should stand out from running prose.

## Layout

`Stack` from `@wordpress/ui` is the default layout primitive. Reach for plain CSS only where Stack doesn't fit.

### The frame

Every WooAgent screen lives in the same frame: dark `LeftNav`, thin light `TopBar`, generous content area, and a sticky `ActionBar` at the bottom for review / approve / batch surfaces.

### Queues, not dashboards

The primary work surface is a queue — table or kanban — not a KPI dashboard. Page heading → optional 1–2 metric strip → list of work items. Operators scan; they don't stare at six tiles of mixed data.

## Elevation

WPDS elevation tokens only. Default to flat. Cards are bordered, not shadowed. No glow, no backdrop blur, no glassmorphism.

## Shapes

WPDS border-radius tokens. **Persona avatars are squares** (`--wpds-border-radius-sm`); the squareness is part of the identity. **Kind pills are full-radius** — the *only* full-radius surface in the UI.

## Components

`PersonaAvatar`, `LeftNav`, `TopBar`, `ActionBar`, `Kpi`, `StatusBadge`, `AskAgentDrawer`, `EditPersonaModal`, kind pill, onboarding card. (Each documented in `ui/DESIGN.md` with file path, sizes, and rules.)

## Do's and Don'ts

- **Don't expand the persona-color exception.** Two sites only.
- **Don't add streaming flourishes.** No typewriter, shimmer, pulse, or glow. Steady spinner with a plain label.
- **Indigo is the only primary-action color.** Persona colors never become buttons.
- **WPDS components first, every time.** Don't fork `Card`, `Modal`, `Button`. Escalate first.
- **Sticky action bar for batch actions.** Not inline per-row buttons.
````

The full file is ~170 lines; what's above is condensed. The YAML is tiny — just the 15 persona color values. Everything else is delegated to WPDS.

## Linting it

Plan A was to use the agent in [Antigravity](https://antigravity.google/) to run the lint. That kept erroring out. Plan B was simpler: run the CLI directly.

```bash
npx @google/design.md lint ui/DESIGN.md
```

Result:

```json
{
  "summary": { "errors": 0, "warnings": 2, "infos": 3 },
  "findings": [
    {
      "severity": "warning",
      "path": "colors",
      "message": "No 'primary' color defined. The agent will auto-generate key colors..."
    },
    { "severity": "info", "message": "Design system defines 15 colors." },
    {
      "severity": "info",
      "path": "spacing",
      "message": "No 'spacing' section defined. Layout spacing will fall back to agent defaults."
    },
    {
      "severity": "info",
      "path": "rounded",
      "message": "No 'rounded' section defined."
    },
    {
      "severity": "warning",
      "path": "typography",
      "message": "No typography tokens defined. Agents will use default font choices..."
    }
  ]
}
```

**0 errors, 2 warnings, 3 infos.** All five findings are expected friction from the layered-overlay choice. The lint assumes DESIGN.md is self-contained; ours deliberately delegates spacing, typography, radii, and primary color to WPDS.

We could silence the warnings by adding `typography:` and a `primary:` color to the front-matter — but that means duplicating WPDS state, which is exactly what we said we wouldn't do. **The warnings are signals, not bugs.** Worth treating that distinction explicitly in any DESIGN.md adoption.

## What changed in the Agent Roster

The first surface we audited was the Agent Roster (`ui/src/screens/Agents.tsx`). Five findings:

1. **Voice was clinical.** The error and loading copy didn't match DESIGN.md's "warm and conversational" rule.
2. **Custom shell components.** Five places used bare HTML with custom CSS classes instead of WPDS components: a `<button className="wa-icon-btn">` ×3, a bare `<input>` in `<label className="wa-topbar__search">`, and a `<button className="wa-roster__model">` for the model picker.
3. **`StatusBadge` parallel system.** Running/idle status used custom `wa-status*` CSS instead of WPDS `Badge`.
4. **DataViews opportunity.** The whole `<table>` + manual toolbar shape is what `DataViews` is built for.
5. **Optional KPI strip.** DESIGN.md prescribes "1–2 metric strip → list of work items." The Roster could use a `Kpi` row.

We landed the surgical fixes (#1 + #2) plus the *structural* part of #4 — restructuring the Roster's code to mirror `DataViews`'s shape without yet adopting the package. The DataViews discussion that drove that decision is below. #3 (`StatusBadge` parallel system) and #5 (KPI strip) are deferred.

### Voice fixes

```diff
-Loading agents…
+Getting your agents ready…
```

```diff
-<Notice status="error" isDismissible={false}>
-  Failed to load agents: {error}
-</Notice>
+<Notice
+  status="error"
+  isDismissible={false}
+  actions={[{ label: 'Retry', onClick: handleRetry, variant: 'primary' }]}
+>
+  Hmm, couldn't load your agents right now. ({error})
+</Notice>
```

The error case got a structural improvement too — no retry affordance before, indigo Retry button now.

### Component swaps

**Search input** — bare label + input → `<SearchControl>`:

```diff
-<label className="wa-topbar__search" style={{ width: 280 }}>
-  <Icon icon={search} size={18} />
-  <input type="search" placeholder="Search agents" />
-</label>
+<div style={{ width: 280 }}>
+  <SearchControl
+    __nextHasNoMarginBottom
+    value={search}
+    onChange={setSearch}
+    label="Search agents"
+    placeholder="Search agents"
+    hideLabelFromVision
+  />
+</div>
```

**Three icon buttons** (filter, change view, row actions) — custom `<button className="wa-icon-btn">` → WPDS `<Button icon={...} label="..." />` with proper keyboard, focus, and ARIA behavior.

**Model picker** — custom button with chevron → `<Button variant="tertiary" icon={chevronDown} iconPosition="right">` with a `// TODO: SelectControl when daemon exposes available models`.

Net diff: small, surgical, and entirely guided by DESIGN.md rules.

## The DataViews question

Audit finding #4 flagged a bigger architectural opportunity: the whole `<Card.Root>` + manual `<table>` + custom toolbar shape is what [`DataViews`](https://wordpress.github.io/gutenberg/?path=/docs/dataviews-dataviews--best-practices) is built for. Filter, search, view-switcher (table ↔ grid), row actions — `@wordpress/dataviews` handles all of those upstream. Adopting it would replace the surgical component swaps from #2 with a single upstream component, and would future-proof the grid-toggle the toolbar already implies.

We considered three paths:

- **Big bang.** Replace the whole table block with `<DataViews>` in one PR. ~200 lines deleted, ~150 added. Roster gets sort / filter / grid for free.
- **Two-step.** Adopt `view` / `fields` / `actions` props but keep the inline `FormToggle` and model picker via render escape hatches; follow up by moving them to row actions / modal. (Footgun: leaves the codebase fighting the API for the duration of the split.)
- **Defer + align.** Ship the surgical fixes now, but restructure the code to *mirror* DataViews's shape so a future migration is mostly mechanical.

We picked the third. DataViews is a real refactor that pays off best with two adopters lined up — Roster alone doesn't earn the abstraction. Restructuring without depending on the package gets us migration-readiness without first-adopter rough edges.

### What "alignment" means concretely

**Fields-as-config.** The manually-written `<th>` and `<td>` markup gets restructured into a fields config that mirrors DataViews's shape:

```ts
const fields: Field<Persona>[] = [
  { id: 'persona',  label: 'Persona',  render: (item) => <PersonaCell item={item} /> },
  { id: 'mandate',  label: 'Mandate',  render: (item) => metaFor(item.persona).mandate },
  { id: 'model',    label: 'Model',    render: (item) => <ModelCell item={item} /> },
  { id: 'status',   label: 'Status',   render: (item) => <StatusCell item={item} /> },
  { id: 'last_run', label: 'Last run', render: (item) => metaFor(item.persona).lastRun },
  { id: 'enabled',  label: 'Enabled',  render: (item) => <EnabledCell item={item} /> },
];
```

The body becomes `personas.map(p => <tr>{fields.map(f => <td>{f.render(p)}</td>)}</tr>)`. Future migration: swap `<table>` for `<DataViews fields={fields} ...>`. Mechanical.

**Actions-as-config.** The row click + ⋮ button get unified under an `actions` array that mirrors DataViews's API. The row click stays primary; future entries (duplicate, view history, reset) drop into the ⋮ menu without code restructure.

**Empty state.** A friendly notice when the persona list is empty — covers a gap DataViews would handle automatically.

### Customizations we explicitly preserved

The user-visible behaviors that needed to stay stay inline, inside `render` functions:

- **Inline `FormToggle` per row** — the daemon's enable/disable affordance.
- **Inline model picker** — placeholder until the daemon exposes available models.
- **Custom persona avatar + display name + slug cell.**
- **Custom running / idle status indicator with dot.**

Any of these survive a future DataViews migration as-is — `render` functions are the API's escape hatch and they're in the right place.

### What we explicitly skipped

- Bulk-select checkbox column (no use case yet).
- Pagination footer (7 personas, no need).
- The actual `@wordpress/dataviews` import — that's the future migration, not this PR.

## Second pass: lessons from the build

Once the surgical fixes landed and we ran the build, more issues surfaced that the *first version* of DESIGN.md hadn't explicitly forbidden. Each finding drove a more concrete rule.

**1. Monospace text was creeping in everywhere.**

The model name in the picker (`anthropic/claude-sonnet-4-6`), the daemon hostname in the sidebar (`localhost:7777`), the persona slug under each name (`marketing`), the nav-counter badge — all rendered in a code-style mono face. The effect was "developer console," not "calm coworker." We added a **"No monospace fonts anywhere"** rule to DESIGN.md and stripped 17 mono `font-family` declarations from `ui/src/` (12 in `app.css`, 5 inline in `IssueDetail.tsx`). Body font for everything: identifiers, hostnames, model names, slugs.

**2. The nav badge was a custom-drawn `<span>` AND used a persona color as a button-fill.**

Two violations in one element: (a) a custom shell when WPDS `Badge` exists, and (b) an expansion of the persona-color exception (the `<span>` was using `--wa-persona-mk-ink` as background). Swapped to `<Badge intent="high">{count}</Badge>` from `@wordpress/ui` — fixes both at once.

**3. The two search bars rendered differently.**

The TopBar global search was a bare `<input>` in a `<label className="wa-topbar__search">`. The Roster's "Search agents" used WPDS `SearchControl` (from the first round of fixes). Same component role, two implementations. Standardized both on `SearchControl`.

**4. The "model picker" was a `Button` with a chevron, not a form control.**

A placeholder I'd added in the first round, with a `// TODO: SelectControl when daemon exposes available models`. But it broadcast the wrong affordance — looked clickable, had no menu. Swapped to a real WPDS `SelectControl` with a canonical 5-model list (Claude Sonnet 4.6 / Opus 4.7 / Haiku 4.5, Gemini 2.5 Pro, GPT-5). The persona's current `model_preference` auto-prepends if it isn't in the canonical list, so existing values are never lost.

**5. The Roster only showed 3 of 7 personas.**

The daemon returns whichever personas are configured for the connected store; the table was rendering only those. Changed the Roster to always show the canonical 7-agent fleet, merging the daemon's data onto a static `ALL_PERSONA_KEYS` list — placeholders for unconfigured personas show with the toggle off. Operators always see the full team, even when only some agents are live.

**6. Persona display names were Title Case.**

`Sales Support`, `Chief of Staff`, `Inventory Manager`. Updated to sentence case (`Sales support`, `Chief of staff`, `Inventory manager`; `Marketing & SEO` keeps `SEO` as an acronym) across five files: `Agents.tsx`, `EditPersonaModal.tsx`, `IssueDetail.tsx`, `App.tsx`, `Step5Done.tsx`. Both `displayName()` functions also got a sentence-case fallback for unknown daemon-supplied names. Sentence-case rule added to DESIGN.md's Typography section.

**7. Table row borders were too dark.**

WPDS only exposes one stroke weight at the light end (`--wpds-color-stroke-surface-neutral-weak`); there's no "weakest." Rather than reach for a custom color, we dropped the per-row `border-bottom` on `.wa-roster tbody td` entirely and let padding + hover state separate rows. Header underline + toolbar underline stay (those are structural). Linear/Notion-style flat tables.

**8. Toolbar icons didn't match the reference design.**

WPDS's default `filter` icon renders as a small downward triangle (a funnel at small sizes); the design called for the three-decreasing-lines shape. Same icon name, wrong glyph. Swapped `filter` → `funnel` and `grid` → `blockTable`.

### What this round added to DESIGN.md

The biggest lesson: **DESIGN.md is only useful when the rules are concrete enough to police a diff.** The first version said "WPDS components first" — true but vague. After this round:

- An **anti-rolls table in CLAUDE.md** (which auto-loads every session) mapping `<button>→Button`, `<input>→SearchControl`, `<select>→SelectControl`, custom badge→`Badge`, custom dropdown→`Dropdown`, mono span→body text. A reach-for-this lookup. Future Claude sessions don't have to re-derive WPDS-first from principles.
- A **`// CUSTOM:` comment requirement** for any element that genuinely must be custom — explaining (a) why no WPDS component fits, (b) what's custom, (c) where it's documented. Reviewers reject custom UI without that flag. The persona avatars and the kind pill are the only two pre-approved customs.
- The explicit **no-mono rule**, with rationale and the list of common offenders (model names, hostnames, slugs, identifiers).
- The explicit **sentence-case rule** for all user-facing strings.

In short: the first DESIGN.md was a manifesto. The second version is a checklist. That's the version that actually changes diffs.

### Process note

The "build review" loop was the most valuable part of the day. Reading the live UI revealed mismatches between the design intent (i3.1 in Figma) and the build that the first audit hadn't flagged because it was reading code, not running it. **A DESIGN.md sweep should always include running the actual UI and comparing it to the design** — code-only audits miss visual regressions, and visual-only reviews miss structural ones.

## Before & after

We built a [`DESIGN REVIEW`](https://www.figma.com/design/0kiSHVNytQ7QjfglyxvBxF/WooAgent?node-id=330-433) page in the WooAgent Figma file showing each change side-by-side. Each row has a labeled BEFORE column (the old code's behavior) and AFTER column (the new code's behavior, using WPDS components):

- [Diff: Loading copy](https://www.figma.com/design/0kiSHVNytQ7QjfglyxvBxF/WooAgent?node-id=342-2) — clinical phrasing → warm phrasing.
- [Diff: Error message + Retry](https://www.figma.com/design/0kiSHVNytQ7QjfglyxvBxF/WooAgent?node-id=342-10) — `Notice` without retry → `Notice` with warm copy and indigo Retry CTA.
- [Diff: Search input](https://www.figma.com/design/0kiSHVNytQ7QjfglyxvBxF/WooAgent?node-id=343-10) — bare `<input>` → `<SearchControl>`.
- [Diff: Icon buttons (toolbar + row actions)](https://www.figma.com/design/0kiSHVNytQ7QjfglyxvBxF/WooAgent?node-id=343-64) — `<button class="wa-icon-btn">` → WPDS `<Button icon={...} label="...">`.
- *Diff: Model picker* — custom inline button → WPDS `<Button variant="tertiary">` with chevron-right (frame coming).

> **Note for upload:** export each Figma diff row as PNG (right-click frame → Copy/Paste as → Copy as PNG, or use the Export panel) and upload to the WordPress media library; replace the links above with `<img>` embeds in the published post.

The first time we tried this comparison, we cloned the existing i3.1 "Agents list" frame and called it "After." That was wrong — i3.1 already showed the WPDS-component version of the design (`SearchControl`, WPDS `Button`s) as the *intent*; the code had drifted from it. The honest comparison shows the **old code's actual rendering** as BEFORE and the **new code's rendering** as AFTER. That's what's on the DESIGN REVIEW page now.

The visual delta isn't huge — the WPDS components and the bare-HTML stand-ins look superficially similar. The real change is under the hood: accessible keyboard / focus behavior comes for free, the toggle/picker are real form controls, the error state has a recovery affordance, and the copy speaks to the user instead of at them.

## What's next

- **Five more surfaces to audit**: Kanban, Onboarding, and three persona approve/reject flows (Marketing, Pricing, Sales Support — all sharing `IssueDetail.tsx` + `BatchReview.tsx`).
- **Actual `@wordpress/dataviews` adoption.** The Roster is now shaped for it; the migration becomes interesting once a second tabular surface (Marketing queue?) is being built and the abstraction earns its keep.
- **Other deferred follow-ups**: a `Kpi` strip on the Roster, and the CSS cleanup of `.wa-icon-btn` / `.wa-topbar__search` / `.wa-roster__model` (still used by four other files).
- **Skill wrapper is live.** A `.claude/skills/woo-design/` skill is registered — Claude Code now auto-loads `DESIGN.md` whenever it's working on `/ui/` code, alongside the existing `wpds` skill.

If you're working on a similar surface in another a8c product, the format is portable: the WPDS "everything-else" convention works wherever WPDS does, and the persona-overlay structure adapts to any product with its own brand layer.

---

*Discussion welcome on #wooagent.*
