import { useNavigate } from 'react-router-dom';
import { Card, Stack, Text } from '@wordpress/ui';
import { Notice, Spinner } from '@wordpress/components';
import { type Batch, type Issue } from '../api/client';
import {
  KindBadge,
  kindFromIssue,
  kindFromPersonaSlug,
  type IssueKind,
} from '../components/StatusBadge';
import { PersonaAvatar, personaKeyFrom } from '../components/PersonaAvatar';

type ColumnKey = 'backlog' | 'drafting' | 'in_review' | 'done';

const COLUMNS: { key: ColumnKey; label: string; hint: string }[] = [
  { key: 'backlog', label: 'Backlog', hint: 'Queued · agent will draft' },
  { key: 'drafting', label: 'Drafting', hint: 'Agent working now' },
  { key: 'in_review', label: 'In Review', hint: 'Needs your call' },
  { key: 'done', label: 'Done', hint: 'Shipped · reversible' },
];

// Daemon has 6 statuses; the board surfaces 4. todo+in_progress collapse to
// "drafting" because operators don't differentiate them; rejected drops off
// the board (matches prototype semantics — undo lives in the Done column).
function columnForIssue(status: Issue['status']): ColumnKey | null {
  switch (status) {
    case 'backlog':
      return 'backlog';
    case 'todo':
    case 'in_progress':
      return 'drafting';
    case 'in_review':
      return 'in_review';
    case 'done':
      return 'done';
    case 'rejected':
      return null;
  }
}

// Batches don't have a status column on the daemon — their column is
// derived from child counts. Pending children mean the operator still owes
// a call → in_review. No pending and at least one approved → done. All
// rejected (or empty) → off-board.
function columnForBatch(b: Batch): ColumnKey | null {
  if (b.pending > 0) return 'in_review';
  if (b.approved > 0) return 'done';
  return null;
}

function relativeTime(iso: string): string {
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

// Right-side state label rendered on the card's meta row. Tracks the column
// — backlog "queued", drafting "drafting now", review/done show timestamps
// with success treatment for done.
function issueStateLabel(col: ColumnKey, issue: Issue): string {
  switch (col) {
    case 'backlog':
      return 'queued';
    case 'drafting':
      return 'drafting now';
    case 'in_review':
      return relativeTime(issue.updated_at);
    case 'done':
      return `shipped ${relativeTime(issue.updated_at)}`;
  }
}

function batchStateLabel(col: ColumnKey, batch: Batch): string {
  switch (col) {
    case 'backlog':
      return 'queued';
    case 'drafting':
      return 'drafting now';
    case 'in_review':
      return relativeTime(batch.updated_at);
    case 'done':
      return `shipped ${relativeTime(batch.updated_at)}`;
  }
}

// Pluralise the kind for a multi-child meta line.
function kindNoun(kind: IssueKind, count: number): string {
  switch (kind) {
    case 'content':
      return count === 1 ? 'rewrite' : 'rewrites';
    case 'campaign':
      return count === 1 ? 'campaign' : 'campaigns';
    case 'email':
      return count === 1 ? 'email' : 'emails';
    case 'price':
      return count === 1 ? 'price change' : 'price changes';
    case 'message':
      return count === 1 ? 'reply' : 'replies';
  }
}

// Per-batch meta line. Names the work in concrete terms ("5/7 pending"
// or "all 7 approved") rather than the per-child variant counts that
// only make sense for issues.
function batchMetaLabel(batch: Batch, kind: IssueKind): string {
  if (batch.pending > 0) {
    return `${batch.pending} of ${batch.total} ${kindNoun(kind, batch.total)} pending`;
  }
  if (batch.approved === batch.total) {
    return `${batch.total} ${kindNoun(kind, batch.total)} approved`;
  }
  if (batch.approved > 0) {
    return `${batch.approved}/${batch.total} ${kindNoun(kind, batch.total)} approved`;
  }
  return `${batch.total} ${kindNoun(kind, batch.total)}`;
}

// BoardItem is the union the kanban renders. Either a single-issue card
// (existing behavior) or a batch card representing N children grouped
// under one parent. The dedupe logic below ensures any issue with a
// batch_id matching a known batch is replaced by its batch's card.
type BoardItem =
  | { kind: 'issue'; issue: Issue; column: ColumnKey; sortKey: string }
  | { kind: 'batch'; batch: Batch; column: ColumnKey; sortKey: string };

interface Props {
  issues: Issue[] | null;
  batches: Batch[];
  error: string | null;
}

export default function Kanban({ issues, batches, error }: Props) {
  const nav = useNavigate();

  if (error) {
    return (
      <main style={{ padding: 'var(--wpds-dimension-padding-2xl)' }}>
        <Notice status="error" isDismissible={false}>
          Failed to load issues: {error}
        </Notice>
      </main>
    );
  }
  if (issues === null) {
    return (
      <main style={{ padding: 'var(--wpds-dimension-padding-2xl)' }}>
        <Stack direction="row" gap="sm" align="center">
          <Spinner /> <Text variant="body-sm">Loading issues…</Text>
        </Stack>
      </main>
    );
  }

  const lastUpdate = issues.reduce<string | null>((acc, i) => {
    if (!acc) return i.updated_at;
    return i.updated_at > acc ? i.updated_at : acc;
  }, null);

  // Build the board. Drop child issues whose batch_id matches a known
  // batch — they'll be represented by the batch's synthetic card. Issues
  // with a batch_id but no matching batch (stale data) keep rendering as
  // individual cards; better than vanishing.
  const batchIDs = new Set(batches.map((b) => b.id));
  const items: BoardItem[] = [];
  for (const issue of issues) {
    if (issue.batch_id && batchIDs.has(issue.batch_id)) continue;
    const col = columnForIssue(issue.status);
    if (col === null) continue;
    items.push({ kind: 'issue', issue, column: col, sortKey: issue.updated_at });
  }
  for (const batch of batches) {
    const col = columnForBatch(batch);
    if (col === null) continue;
    items.push({ kind: 'batch', batch, column: col, sortKey: batch.updated_at });
  }
  // Within a column, newer items first.
  items.sort((a, b) => (a.sortKey < b.sortKey ? 1 : a.sortKey > b.sortKey ? -1 : 0));

  return (
    <main
      style={{
        maxWidth: 1500,
        margin: '0 auto',
        padding:
          'var(--wpds-dimension-padding-2xl) var(--wa-page-pad-x)',
      }}
    >
      <div
        className="wa-page-header"
        style={{ marginBottom: 'var(--wpds-dimension-gap-xl)' }}
      >
        <Stack direction="column" gap="xs">
          <Stack direction="row" gap="xs" align="center">
            <PersonaAvatar persona="mk" size="xs" dotOnly />
            <span className="wa-eyebrow wa-eyebrow--persona">
              Marketing agent · Today
            </span>
          </Stack>
          <Text variant="heading-2xl" render={<h1 />}>
            Today's marketing queue
          </Text>
          <Text
            variant="body-sm"
            style={{
              maxWidth: 600,
              color: 'var(--wpds-color-fg-content-neutral-weak)',
            }}
          >
            What the marketing agent has staged for you. Approve in review,
            adjust the queue, or let it work. Every shipped change is reversible.
          </Text>
        </Stack>
        <Stack
          direction="row"
          gap="md"
          align="center"
          style={{
            color: 'var(--wpds-color-fg-content-neutral-weak)',
            fontSize: 'var(--wpds-typography-font-size-xs)',
          }}
        >
          <Stack direction="row" gap="xs" align="center">
            <span
              style={{
                height: 8,
                width: 8,
                borderRadius: '50%',
                background: 'var(--wpds-color-fg-content-success)',
                display: 'inline-block',
              }}
            />
            Agent online
          </Stack>
          <span className="wa-mono">
            {lastUpdate ? `last scan · ${relativeTime(lastUpdate)}` : 'no activity yet'}
          </span>
        </Stack>
      </div>

      <div className="wa-kanban-row">
        {COLUMNS.map((col) => {
          const colItems = items.filter((it) => it.column === col.key);
          const accentClass = `wa-kanban-col__accent wa-kanban-col__accent--${
            col.key === 'in_review' ? 'review' : col.key
          }`;
          return (
            <div key={col.key} className="wa-kanban-col">
              <div className="wa-kanban-col__header">
                <Stack direction="column" gap="xs">
                  <Text
                    variant="body-md"
                    style={{
                      fontWeight: 'var(--wpds-typography-font-weight-medium)',
                    }}
                  >
                    {col.label}
                  </Text>
                  <span className="wa-eyebrow">{col.hint}</span>
                </Stack>
                <span className="wa-kanban-col__count">{colItems.length}</span>
              </div>
              <div className={accentClass} />
              <div className="wa-kanban-col__cards">
                {colItems.length === 0 && (
                  <Text
                    variant="body-sm"
                    style={{
                      color: 'var(--wpds-color-fg-content-neutral-weak)',
                    }}
                  >
                    No items.
                  </Text>
                )}
                {colItems.map((item) =>
                  item.kind === 'issue'
                    ? renderIssueCard(item.issue, col.key, () =>
                        item.issue.batch_id
                          ? nav(`/batches/${item.issue.batch_id}`)
                          : nav(`/issues/${item.issue.id}`),
                      )
                    : renderBatchCard(item.batch, col.key, () =>
                        nav(`/batches/${item.batch.id}`),
                      ),
                )}
              </div>
            </div>
          );
        })}
      </div>
    </main>
  );
}

function renderIssueCard(
  issue: Issue,
  col: ColumnKey,
  onClick: () => void,
) {
  const kind = kindFromIssue(issue);
  const personaKey = personaKeyFrom(issue.persona);
  const stateLabel = issueStateLabel(col, issue);
  const stateClass =
    col === 'drafting'
      ? 'wa-card-meta--drafting'
      : col === 'done'
        ? 'wa-card-meta--success'
        : '';
  return (
    <button
      key={`issue-${issue.id}`}
      type="button"
      onClick={onClick}
      style={{
        display: 'block',
        width: '100%',
        textAlign: 'left',
        padding: 0,
        background: 'transparent',
        border: 'none',
        cursor: 'var(--wpds-cursor-control)',
      }}
    >
      <Card.Root>
        <Card.Content>
          <Stack direction="column" gap="sm">
            <Stack direction="row" gap="sm" align="center">
              <KindBadge kind={kind} />
              <span
                className="wa-mono"
                style={{
                  fontSize: 10,
                  color: 'var(--wpds-color-fg-content-neutral-weak)',
                }}
              >
                {issue.id.slice(0, 8).toUpperCase()}
              </span>
            </Stack>
            <Text
              variant="body-sm"
              style={{
                fontWeight: 'var(--wpds-typography-font-weight-medium)',
              }}
            >
              {issue.title}
            </Text>
            <div className="wa-card-meta">
              <span>
                {kind === 'content'
                  ? '3 variants'
                  : kind === 'price'
                    ? 'price change'
                    : kind === 'message'
                      ? 'reply draft'
                      : '1 item'}
              </span>
              <span className={`wa-card-meta-state ${stateClass}`}>
                {col === 'done' && (
                  <span className="wa-card-meta-dot" aria-hidden="true" />
                )}
                <span>{stateLabel}</span>
                <PersonaAvatar persona={personaKey} size="sm" />
              </span>
            </div>
          </Stack>
        </Card.Content>
      </Card.Root>
    </button>
  );
}

function renderBatchCard(batch: Batch, col: ColumnKey, onClick: () => void) {
  const kind = kindFromPersonaSlug(batch.persona);
  const personaKey = personaKeyFrom(batch.persona);
  const stateLabel = batchStateLabel(col, batch);
  const stateClass =
    col === 'drafting'
      ? 'wa-card-meta--drafting'
      : col === 'done'
        ? 'wa-card-meta--success'
        : '';
  const meta = batchMetaLabel(batch, kind);
  return (
    <button
      key={`batch-${batch.id}`}
      type="button"
      onClick={onClick}
      style={{
        display: 'block',
        width: '100%',
        textAlign: 'left',
        padding: 0,
        background: 'transparent',
        border: 'none',
        cursor: 'var(--wpds-cursor-control)',
      }}
    >
      <Card.Root>
        <Card.Content>
          <Stack direction="column" gap="sm">
            <Stack direction="row" gap="sm" align="center">
              <KindBadge kind={kind} />
              <span
                className="wa-mono"
                style={{
                  fontSize: 10,
                  color: 'var(--wpds-color-fg-content-neutral-weak)',
                  fontWeight: 'var(--wpds-typography-font-weight-medium)',
                }}
              >
                BATCH · {batch.id.slice(0, 6).toUpperCase()}
              </span>
            </Stack>
            <Text
              variant="body-sm"
              style={{
                fontWeight: 'var(--wpds-typography-font-weight-medium)',
              }}
            >
              {batch.title}
            </Text>
            <div className="wa-card-meta">
              <span>{meta}</span>
              <span className={`wa-card-meta-state ${stateClass}`}>
                {col === 'done' && (
                  <span className="wa-card-meta-dot" aria-hidden="true" />
                )}
                <span>{stateLabel}</span>
                <PersonaAvatar persona={personaKey} size="sm" />
              </span>
            </div>
          </Stack>
        </Card.Content>
      </Card.Root>
    </button>
  );
}
