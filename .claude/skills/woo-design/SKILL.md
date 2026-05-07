---
name: woo-design
description: Use when building, reviewing, or modifying UI in WooAgent's /ui/ directory, or when implementing screens / components for the WooAgent product. Captures WooAgent's specific design language (visual identity, voice, component patterns) — extends the WPDS guidelines with WooAgent-specific decisions. Pair with the wpds skill.
---

# WooAgent design language

Read **`ui/DESIGN.md`** before generating, modifying, or reviewing any UI in `ui/`. The full spec lives there — front-matter tokens (the WooAgent-owned persona palette) plus prose covering Overview / Colors / Typography / Layout / Elevation / Shapes / Components / Do's and Don'ts.

## Critical rules (the things people get wrong)

1. **Persona colors live in two places only**: `PersonaAvatar` and the kind pill on cards. Never as button fills, hover states, or focus rings.
2. **Indigo is the only primary-action color.** Approve, save, continue, send, retry — always WPDS indigo, never persona-tinted.
3. **No hex literals** in component code. Every color is a `--wpds-*` or `--wa-persona-*` variable. If you can't find a WPDS token, query the WPDS MCP server before reaching for hex.
4. **Voice is warm & conversational, chrome stays calm.** Empty states address the user. Errors apologize without grovelling. Calm chrome, warm words.
5. **No streaming flourishes.** Steady spinners only. No typewriter, shimmer, pulse, or glow.
6. **Stack is the default layout primitive.** Plain CSS only when Stack doesn't fit.
7. **WPDS components first.** Don't fork `Card`, `Modal`, or `Button` — escalate first.

When in doubt, ask: would this look at home in a high-quality calm productivity tool (Linear, Notion), or does it look like an "AI demo"? Aim for the former.

## Pair with

- `wpds` — for canonical WPDS token / component lookups via MCP.
- `frontend-design` — for distinctive frontend principles, applied within WPDS, not against it.
