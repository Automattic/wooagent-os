// Shared issue + batch combiner used by the Needs review and Done screens.
// Batches and their child issues both appear in the same data feed; the
// daemon doesn't dedupe — that's the UI's job. A batch's "column" is
// derived from child counts (see columnForBatch) the same way the old
// Kanban screen did it.
//
// Returned items carry a discriminator so the screens can render an
// issue card or a batch card with the right click-through target.

import type { Batch, Issue } from '../api/client';

export type BoardItemKind = 'issue' | 'batch';

export type BoardItem =
  | { kind: 'issue'; issue: Issue; sortKey: string }
  | { kind: 'batch'; batch: Batch; sortKey: string };

export type BoardStatus = 'in_review' | 'done';

function issueMatches(issue: Issue, status: BoardStatus): boolean {
  if (status === 'in_review') return issue.status === 'in_review';
  if (status === 'done') return issue.status === 'done';
  return false;
}

function batchMatches(batch: Batch, status: BoardStatus): boolean {
  if (status === 'in_review') return batch.pending > 0;
  if (status === 'done') return batch.pending === 0 && batch.approved > 0;
  return false;
}

interface BuildOptions {
  /** Filter to a single persona slug (drives the `?persona=` query on
   *  Needs review). Undefined means include all personas. */
  persona?: string | null;
}

export function buildBoardItems(
  issues: Issue[],
  batches: Batch[],
  status: BoardStatus,
  options: BuildOptions = {},
): BoardItem[] {
  const persona = options.persona ?? null;
  const batchIDs = new Set(batches.map((b) => b.id));
  const items: BoardItem[] = [];

  for (const issue of issues) {
    // Child of a known batch — represented by the batch card instead.
    // Stale child rows (batch_id refers to a missing batch) still render
    // as individual cards; better than vanishing.
    if (issue.batch_id && batchIDs.has(issue.batch_id)) continue;
    if (persona && issue.persona !== persona) continue;
    if (!issueMatches(issue, status)) continue;
    items.push({ kind: 'issue', issue, sortKey: issue.updated_at });
  }

  for (const batch of batches) {
    if (persona && batch.persona !== persona) continue;
    if (!batchMatches(batch, status)) continue;
    items.push({ kind: 'batch', batch, sortKey: batch.updated_at });
  }

  // Newest first within the page.
  items.sort((a, b) =>
    a.sortKey < b.sortKey ? 1 : a.sortKey > b.sortKey ? -1 : 0,
  );

  return items;
}

export function relativeTime(iso: string | undefined): string {
  if (!iso) return '—';
  const then = new Date(iso).getTime();
  if (!Number.isFinite(then)) return '—';
  const diffMs = Date.now() - then;
  const min = Math.round(diffMs / 60_000);
  if (min < 1) return 'just now';
  if (min < 60) return `${min} min ago`;
  const hr = Math.round(min / 60);
  if (hr < 24) return `${hr} hr ago`;
  const days = Math.round(hr / 24);
  return `${days} day${days === 1 ? '' : 's'} ago`;
}

export function personaDisplayName(slug: string | undefined): string {
  if (!slug) return '—';
  if (slug === 'marketing') return 'Marketing & SEO';
  if (slug === 'inventory') return 'Inventory manager';
  if (slug === 'sales-support') return 'Sales support';
  if (slug === 'chief') return 'Chief of staff';
  return slug.charAt(0).toUpperCase() + slug.slice(1);
}
