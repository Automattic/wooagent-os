---
description: Review WooAgent ui/ code for WPDS + WooAgent compliance — hex literals, persona-color misuse, mono fonts, raw HTML interactive elements, missing `__next40pxDefaultSize` / `hasPadding`, legacy Notice imports, streaming flourishes, search redundancy, unmarked custom UI. Scope: branch diff, PR, or explicit file paths. Read-only.
argument-hint: "[file-paths | PR# | empty]"
allowed-tools:
  - Read
  - Grep
  - Glob
  - Bash(git diff *)
  - Bash(git status)
  - Bash(git branch --show-current)
  - Bash(git ls-files *)
  - Bash(gh pr view *)
  - Bash(gh pr diff *)
  - Bash(wc -l *)
  - Agent
---

# Design review — WPDS + WooAgent

Mechanical, rule-based review of `ui/` code against `DESIGN.md` and `CLAUDE.md` before merging. The complement to the behavioural lens in `DESIGN.md` (the "Working on UI" section) — that asks judgment questions; this scans for rule violations.

## How to use

- `/design-review` — review the current branch diff against `trunk`.
- `/design-review ui/src/screens/Settings.tsx` — review a specific file.
- `/design-review ui/src/screens/**/*.tsx` — review a glob.
- `/design-review 42` — review PR #42 on the current repo.

## Scope

- **Files in scope:** `.tsx`, `.ts`, `.jsx`, `.js`, `.css`, `.scss` under `ui/`.
- **Skip:** `daemon/`, `cmd/`, `internal/`, `prompts/`, `skills/`, `spike-adk/`, `marketing-prototype/`, `node_modules`, `dist/`, `build/`, tests (`*.test.*`, `*.spec.*`, `__tests__/`, `__mocks__/`, `fixtures/`), Storybook stories, `DESIGN.md` itself.
- **Not in scope:** rendered visuals or screenshots — that's a designer's job (see DESIGN.md's "When to loop in a designer").

## Dynamic context

- Branch: !`git branch --show-current`
- Status: !`git status --short`

## Procedure

### 1. Load the rule set

Read `RULES.md` in this skill's directory. It is the source of truth for review decisions. If `@wordpress/design-system-mcp` is available in this session, use it to verify component status and token names live — when `RULES.md` and the MCP disagree, the MCP is authoritative; note the discrepancy in the report and flag that `RULES.md` needs an update.

### 2. Determine files to review

Based on `$ARGUMENTS`:

- **PR number** (e.g. `42`): fetch the diff with `gh pr diff <n>` (prefix with `HTTPS_PROXY=socks5://127.0.0.1:8080 GH_HOST=github.a8c.com` for the a8c GHES). Extract changed files, filter to extensions in scope.
- **File paths / globs**: resolve with Glob, read each.
- **Empty**: run `git diff trunk...HEAD --name-only` (fall back to `main` if `trunk` doesn't resolve). If that's also empty, scan the working tree changes (`git diff HEAD --name-only`). Filter to in-scope extensions.

Apply the skip list before scanning.

### 3. Scan

Walk each file once, top to bottom. For each in-scope file, run the rules in this order:

1. **Hex literals** — grep `#[0-9a-fA-F]{3,8}\b`. Filter out comment lines before flagging: lines containing `//`, `/*`, or `{/*` are documentation, not enforceable code. Apply Rule 1's allowed-hex exceptions: `var(--token, #fallback)`; `:root` persona-var declarations in `app.css` (lines defining `--wa-persona-*-{bg,ink}`); and `.wa-sidebar` rules in `app.css` (`#2C045D`, `#1F0342`, `#ffffff`). Front matter in `DESIGN.md` is documentation, not code (and `DESIGN.md` is in the skip list).
2. **Persona color usage** — grep `--wa-persona-`. Allow only inside `PersonaAvatar.tsx`, `StatusBadge.tsx` (the canonical `KindBadge` wrapper), the kind-pill rules in `app.css`, and the brand-header "W" tile rules in `LeftNav.tsx`.
3. **Mono fonts** — grep `font-family-mono\|Menlo\|SF Mono\|Consolas\|Roboto Mono\|font-family:\s*monospace`.
4. **Bespoke type ramps** — grep `font-family:\s*(Inter|system-ui|-apple-system)\|font-size:\s*[0-9]+(px|rem|em|%)` in `.css` / `.scss` and `fontSize:\s*[0-9]+\b` in `.tsx` / `.jsx`. Skip matches where a `CUSTOM:` marker appears within 2 lines above — accept any of the three comment styles per RULES.md. Recommend the closest WPDS body-type token: `xs=11`, `sm=12`, `md=13`, `lg=15`, `xl=20`, `2xl=32` px.
5. **Raw HTML interactive elements** — grep with line-start anchor: `^\s*<button\b\|^\s*<input\b\|^\s*<select\b\|^\s*<a\s+href`. Anchoring to start-of-line avoids false positives where these tags appear as text inside `CUSTOM:` comments. Allow when a `CUSTOM:` marker appears within 2 lines above — accept `// CUSTOM:`, `{/* CUSTOM:`, or `/* CUSTOM:` (see RULES.md "`CUSTOM:` marker syntax"). Two lines of slack covers the common `return (` line sitting between the marker and the element.
6. **Button props** — grep `<Button` in `.tsx` / `.jsx`. Verify each call site has `__next40pxDefaultSize`. Verify icon-only buttons (no text children) pass a `label` prop.
7. **Page props** — grep `<Page` occurrences in `.tsx` / `.jsx`. Verify `hasPadding` is set, **unless** the same file imports from `@wordpress/dataviews` (grep `from\s+['"]@wordpress/dataviews['"]`) or the page is the onboarding centered-card flow (under `ui/src/onboarding/`) — both provide their own padding.
8. **Legacy Notice import** — grep `import\s+{[^}]*\bNotice\b[^}]*}\s+from\s+['"]@wordpress/components['"]`. Always a violation — use compound `Notice.Root` from `@wordpress/ui`.
9. **Streaming flourishes** — grep `@keyframes\s+\w*\(shimmer\|pulse\|typewriter\|glow\)\w*`. Verify it's not a flourish on agent-status surfaces.
10. **Search redundancy** — for files that import `DataViews`, grep the same file for `<PageGlobalActions` and check that `showSearch={false}` is passed.
11. **Inline SVG** — grep `^\s*<svg\b` (line-start anchored) in `.tsx` / `.jsx`. Flag if no `CUSTOM:` marker within 2 lines above — accept any of the three comment styles per RULES.md.
12. **Sentence case** — grep `title=|subTitle=|label=|placeholder=` and inspect string values. Flag obvious Title Case (e.g. `"Edit Persona"`, `"Inventory Manager"`) — but tolerate proper nouns and known acronyms (SEO, KPI, UI, AI, URL).

For each violation, capture `file:line` + rule + severity + suggested fix.

### 4. Classify

| Severity | Meaning |
|---|---|
| **Violation** | Breaks a clear rule. Must fix before merge. |
| **Warning** | Likely wrong but context-dependent. Developer should verify. |
| **Flag for designer** | New pattern not covered by existing rules. Needs designer input. |

### 5. Present the report

```
## Design Review: <scope>

### Violations (N)
- **file:line** — <what's wrong> → <fix>

### Warnings (N)
- **file:line** — <what's wrong> → <suggested fix>

### Flag for Designer (N)
- **file:line** — <description of new pattern>

### Clean
<count of files in scope with no findings>
```

If zero findings: `No design system violations found in <scope>.`

### 6. Offer next steps

After presenting the report, ask the user:

- **Walk through violations** with exact code suggestions (do not modify files without explicit confirmation).
- **Post to the PR** as a `gh pr review --comment` (PR scope only, never without confirmation).
- **Dismiss** — acknowledge and close.

## Rules of the road

- **Read-only.** This skill never edits files, creates commits, or pushes.
- **Never post review comments to GitHub without explicit user confirmation.**
- **No false positives on fallbacks.** Hex inside `var(--token, #...)` is standard practice — don't flag.
- **The skip list is non-negotiable.** Don't scan daemon/, marketing-prototype/, test files, build output. Definition vs consumption — files inside design system source packages are canonical; never flag them.
- **MCP for live truth.** Component status and token names can drift. When `RULES.md` and `@wordpress/design-system-mcp` disagree, follow the MCP and note that `RULES.md` needs updating in the report.

## Sources

- Procedure shape adapted from [`Automattic/devkit` design-review skill](https://github.a8c.com/Automattic/devkit/blob/trunk/skills/design-review/SKILL.md) (DesignEx team). Devkit's structural template — load rules → resolve scope → measure → scan → classify → report → offer next steps — is theirs; the rule set has been re-grounded against WooAgent's `DESIGN.md` and `CLAUDE.md`.
- Rule set: `RULES.md` in this skill's directory.
- Live component / token verification: `@wordpress/design-system-mcp` and [system.automattic.design](https://system.automattic.design/).
