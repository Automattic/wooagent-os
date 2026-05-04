import { useNavigate } from 'react-router-dom';
import { Card, Stack, Text } from '@wordpress/ui';
import { Notice, Spinner } from '@wordpress/components';
import { type Issue } from '../api/client';
import { KindBadge, kindFromIssue } from '../components/StatusBadge';
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
function columnFor(status: Issue['status']): ColumnKey | null {
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
function cardStateLabel(col: ColumnKey, issue: Issue): string {
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

interface Props {
  issues: Issue[] | null;
  error: string | null;
}

export default function Kanban({ issues, error }: Props) {
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
          const colIssues = issues.filter((i) => columnFor(i.status) === col.key);
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
                <span className="wa-kanban-col__count">{colIssues.length}</span>
              </div>
              <div className={accentClass} />
              <div className="wa-kanban-col__cards">
                {colIssues.length === 0 && (
                  <Text
                    variant="body-sm"
                    style={{
                      color: 'var(--wpds-color-fg-content-neutral-weak)',
                    }}
                  >
                    No items.
                  </Text>
                )}
                {colIssues.map((issue) => {
                  const kind = kindFromIssue(issue);
                  const personaKey = personaKeyFrom(issue.persona);
                  const stateLabel = cardStateLabel(col.key, issue);
                  const stateClass =
                    col.key === 'drafting'
                      ? 'wa-card-meta--drafting'
                      : col.key === 'done'
                        ? 'wa-card-meta--success'
                        : '';
                  return (
                    <button
                      key={issue.id}
                      type="button"
                      onClick={() =>
                        issue.batch_id
                          ? nav(`/batches/${issue.batch_id}`)
                          : nav(`/issues/${issue.id}`)
                      }
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
                                  color:
                                    'var(--wpds-color-fg-content-neutral-weak)',
                                }}
                              >
                                {issue.id.slice(0, 8).toUpperCase()}
                              </span>
                            </Stack>
                            <Text
                              variant="body-sm"
                              style={{
                                fontWeight:
                                  'var(--wpds-typography-font-weight-medium)',
                              }}
                            >
                              {issue.title}
                            </Text>
                            <div className="wa-card-meta">
                              {/* Batched issues route to /batches/:id rather
                                  than /issues/:id; the meta line names the
                                  parent batch instead of the variant count. */}
                              <span>
                                {issue.batch_id
                                  ? `batch ${issue.batch_id.slice(0, 6).toUpperCase()}`
                                  : kind === 'content'
                                    ? '3 variants'
                                    : '5 child tasks'}
                              </span>
                              <span
                                className={`wa-card-meta-state ${stateClass}`}
                              >
                                {col.key === 'done' && (
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
                })}
              </div>
            </div>
          );
        })}
      </div>
    </main>
  );
}
