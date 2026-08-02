import { readFileSync, readdirSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

// Guards against the failure mode that took out the Ask Agent drawer's
// background, the selected-radio fill, and a handful of hover states in the
// @wordpress/ui 0.11 → 0.17 bump: WPDS renamed the `bg` / `fg` token segments
// to `background` / `foreground`, and every `var(--wpds-color-bg-*)` in the app
// silently became `unset`.
//
// Nothing catches this on its own. A dead `var()` isn't a CSS parse error, so
// the build stays green; `tsc` never sees inside a style string; and the
// symptom is invisible in tests because jsdom doesn't resolve custom
// properties. It only surfaces as "why is this transparent" in a screenshot.
//
// So: collect every --wpds-* reference in the source tree and assert the
// installed @wordpress/theme actually defines it. When a package bump breaks
// this, the token was renamed — find its replacement in design-tokens.css
// rather than pinning the old package.

const THIS_FILE = fileURLToPath(import.meta.url);
const SRC = resolve(dirname(THIS_FILE), '..');
const SCANNED_EXTENSIONS = ['.css', '.ts', '.tsx'];

/** Matches a token *reference* — `--wpds-…` as it appears inside `var()`. */
const TOKEN_REFERENCE = /--wpds-[a-z0-9-]+/g;

/** Matches a token *declaration* — `--wpds-…:` in the theme stylesheet. */
const TOKEN_DECLARATION = /(--wpds-[a-z0-9-]+)\s*:/g;

/** Resolves the theme package the way Vite does — through @wordpress/ui,
 *  which carries its own nested copy. Reading the installed file rather than
 *  a checked-in list means the test tracks whatever version is actually
 *  resolved at build time. */
function readThemeTokens(): Set<string> {
  const require = createRequire(import.meta.url);
  const uiPackage = require.resolve('@wordpress/ui/package.json');
  const tokensPath = require.resolve('@wordpress/theme/design-tokens.css', {
    paths: [dirname(uiPackage)],
  });
  const css = readFileSync(tokensPath, 'utf8');
  const defined = new Set<string>();
  for (const [, name] of css.matchAll(TOKEN_DECLARATION)) defined.add(name);
  return defined;
}

/** Every scannable file under src/, minus this test itself — it names dead
 *  tokens in its own comments. */
function sourceFiles(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      out.push(...sourceFiles(path));
    } else if (
      SCANNED_EXTENSIONS.some((ext) => entry.name.endsWith(ext)) &&
      path !== THIS_FILE
    ) {
      out.push(path);
    }
  }
  return out;
}

describe('WPDS design tokens', () => {
  const defined = readThemeTokens();

  it('resolves the installed theme stylesheet', () => {
    // Sanity check: a silently-empty token set would make the assertion below
    // pass for the wrong reason.
    expect(defined.size).toBeGreaterThan(100);
  });

  it('every --wpds-* token referenced in src is defined by @wordpress/theme', () => {
    const dead = new Map<string, string[]>();

    for (const file of sourceFiles(SRC)) {
      const contents = readFileSync(file, 'utf8');
      for (const [token] of contents.matchAll(TOKEN_REFERENCE)) {
        if (defined.has(token)) continue;
        const where = file.slice(SRC.length + 1);
        const files = dead.get(token) ?? [];
        if (!files.includes(where)) files.push(where);
        dead.set(token, files);
      }
    }

    const report = [...dead.entries()]
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([token, files]) => `  ${token}\n    ${files.join('\n    ')}`)
      .join('\n');

    expect(
      report,
      `Undefined WPDS tokens referenced in src/. These resolve to \`unset\`, ` +
        `which silently turns backgrounds transparent. Look each one up in ` +
        `@wordpress/theme's design-tokens.css — it was probably renamed.\n${report}`,
    ).toBe('');
  });
});
