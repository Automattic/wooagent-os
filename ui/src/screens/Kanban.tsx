import { useNavigate } from 'react-router-dom';
import { Card, Stack, Text } from '@wordpress/ui';
import { Notice, Spinner } from '@wordpress/components';
import { type Issue } from '../api/client';
import { KindBadge, kindFromIssue } from '../components/StatusBadge';

type ColumnKey = 'backlog' | 'drafting' | 'in_review' | 'done';

const COLUMNS: { key: ColumnKey; label: string; hint: string }[] = [
  { key: 'backlog', label: 'Backlog', hint: 'queued · agent will draft' },
  { key: 'drafting', label: 'Drafting', hint: 'agent working now' },
  { key: 'in_review', label: 'In Review', hint: 'needs your call' },
  { key: 'done', label: 'Done', hint: 'shipped · reversible' },
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
        padding: 'var(--wpds-dimension-padding-2xl) var(--wa-page-pad-x)',
      }}
    >
      <div
        className="wa-page-header"
        style={{ marginBottom: 'var(--wpds-dimension-gap-xl)' }}
      >
        <Stack direction="column" gap="xs">
          <span className="wa-eyebrow wa-eyebrow--persona">
            Marketing agent · Today
          </span>
          <Text variant="heading-2xl" render={<h1 />}>
            Today's marketing queue.
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
          return (
            <div key={col.key} className="wa-kanban-col">
              <Card.Root
                style={{
                  height: '100%',
                  background: 'var(--wpds-color-bg-surface-neutral-weak)',
                }}
              >
                <Card.Header>
                  <Stack direction="row" justify="space-between" align="end">
                    <Stack direction="column" gap="xs">
                      <Card.Title>{col.label}</Card.Title>
                      <span className="wa-eyebrow">{col.hint}</span>
                    </Stack>
                    <span
                      className="wa-mono"
                      style={{
                        fontSize: 'var(--wpds-typography-font-size-xs)',
                        color: 'var(--wpds-color-fg-content-neutral-weak)',
                      }}
                    >
                      {colIssues.length}
                    </span>
                  </Stack>
                </Card.Header>
                <Card.Content>
                  <Stack direction="column" gap="sm">
                    {colIssues.length === 0 && (
                      <Text
                        variant="body-sm"
                        style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
                      >
                        No items.
                      </Text>
                    )}
                    {colIssues.map((issue) => {
                      const kind = kindFromIssue(issue);
                      return (
                        <button
                          key={issue.id}
                          type="button"
                          onClick={() => nav(`/issues/${issue.id}`)}
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
                                    {issue.id.slice(0, 8)}
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
                                  <span>{issue.persona ?? 'unassigned'}</span>
                                  <span className="wa-mono">
                                    {relativeTime(issue.updated_at)}
                                  </span>
                                </div>
                              </Stack>
                            </Card.Content>
                          </Card.Root>
                        </button>
                      );
                    })}
                  </Stack>
                </Card.Content>
              </Card.Root>
            </div>
          );
        })}
      </div>
    </main>
  );
}
