// Condensing helpers for the verbose skip/failure reason strings the daemon
// emits on runs. Agents that exhaust a search budget (Pricing especially)
// produce long, colon/semicolon-delimited reasons — a per-product trace that's
// useful in full on the Runs page / run detail, but overwhelming as a roster
// notice. `summarizeReason` keeps the leading human-readable clause for summary
// surfaces; the full string stays available wherever it's rendered in full.

// A reason longer than this (~one and a half lines at the body size used in run
// rows) gets progressive disclosure on the Runs page rather than being printed
// inline in full. Proxy for "more than one line" — we can't measure wrapped
// line count without layout, so length stands in for it.
export const LONG_REASON_THRESHOLD = 140;

export function isLongReason(reason: string | null | undefined): boolean {
  return !!reason && reason.trim().length > LONG_REASON_THRESHOLD;
}

/**
 * Reduces a verbose daemon skip/failure reason to a single readable clause for
 * summary surfaces (the agent-roster outcome notice). Returns the text up to
 * the earlier of the first sentence terminator ('. ', '! ', '? ') or the first
 * colon that introduces structured detail (': '), whichever comes first, with a
 * trailing period added when the clause ends on a word.
 *
 * Reasons at or under LONG_REASON_THRESHOLD are returned whole. Condensing
 * exists to stop a multi-line trace from swamping a notice; a reason that
 * already fits doesn't need it, and trimming one loses information for free.
 *
 * That mattered in practice. The clause-first heuristic assumes the leading
 * clause is the human summary, which holds for Pricing's per-product traces
 * but inverts for error-shaped reasons, where the lead is a machine-y
 * operation label and the cause is at the tail:
 *
 *   "mcp call wooagent-products/list: tool "…" returned error: An error
 *    occurred while executing the tool."   (127 chars)
 *
 * That used to render as "mcp call wooagent-products/list." — the operation
 * name and nothing about what went wrong — even though it was comfortably
 * under the threshold. An operator had to query the database to find out that
 * their store's products ability was broken.
 *
 * Examples:
 *   "tried 3 products, none drafted: product 3908: no_proposal: Ran 5 …"
 *     → "tried 3 products, none drafted."          (long; condensed)
 *   "persona already has 2 or more open proposals; not enqueuing"
 *     → "persona already has 2 or more open proposals; not enqueuing."
 *   'no implementation registered for persona "reporting"'
 *     → 'no implementation registered for persona "reporting"'
 *
 * Known gap: a long error-shaped reason still condenses to its operation
 * label. Fixing that needs a way to tell "leading clause is a summary" from
 * "leading clause is a label", which is a guess we don't have to make yet —
 * the reasons that hit this are short.
 *
 * The full reason stays available on the Runs page / run detail — this only
 * trims the summary shown in the transient notice.
 */
export function summarizeReason(reason: string): string {
  const text = reason.trim();
  if (!text) return text;
  // Short enough to read at a glance — show all of it. Uses the same
  // threshold as isLongReason so the two helpers can't disagree about what
  // "too long to show inline" means.
  if (!isLongReason(text)) return ensureTerminal(text);
  // Earliest sentence end OR structured-detail colon. Scanning left to right,
  // `.match` returns the first index where either alternative hits.
  const match = text.match(/[.!?](?=\s|$)|:\s/);
  if (!match || match.index === undefined) return ensureTerminal(text);
  const clause = text.slice(0, match.index).trim();
  // Guard against a degenerate one-word label (e.g. "Error: …") collapsing to
  // just the label — keep the whole string when the leading clause is too
  // short to stand on its own.
  if (clause.split(/\s+/).length < 2) return ensureTerminal(text);
  return ensureTerminal(clause);
}

// Append a period only when the clause ends on a word or number; leave existing
// terminal punctuation, quotes, and brackets untouched.
function ensureTerminal(s: string): string {
  return /[A-Za-z0-9]$/.test(s) ? `${s}.` : s;
}
