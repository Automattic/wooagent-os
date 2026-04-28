import { useNavigate } from 'react-router-dom';
import { Card, Stack, Text } from '@wordpress/ui';
import { useApp } from '../App';
import type { Task, TaskStatus } from '../data/types';
import { KindBadge } from '../components/StatusBadge';

const COLUMNS: { status: TaskStatus; label: string; hint: string }[] = [
  { status: 'backlog', label: 'Backlog', hint: 'queued · agent will draft' },
  { status: 'drafting', label: 'Drafting', hint: 'agent working now' },
  { status: 'in_review', label: 'In Review', hint: 'needs your call' },
  { status: 'done', label: 'Done', hint: 'shipped · reversible' },
];

function routeFor(t: Task): string {
  return `/issues/${t.id}/${t.kind}`;
}

export default function Kanban() {
  const { tasks } = useApp();
  const nav = useNavigate();

  return (
    <main
      style={{
        maxWidth: 1500,
        margin: '0 auto',
        padding: 'var(--wpds-dimension-padding-2xl) var(--wpds-dimension-padding-2xl)',
      }}
    >
      <div className="wa-page-header" style={{ marginBottom: 'var(--wpds-dimension-gap-xl)' }}>
        <Stack direction="column" gap="xs">
          <span className="wa-eyebrow wa-eyebrow--persona">Marketing agent · Today</span>
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
        <Stack direction="row" gap="md" align="center" style={{ color: 'var(--wpds-color-fg-content-neutral-weak)', fontSize: 'var(--wpds-typography-font-size-xs)' }}>
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
          <span className="wa-mono">last scan · 8 min ago</span>
        </Stack>
      </div>

      <div className="wa-kanban-row">
        {COLUMNS.map((col) => {
          const colTasks = tasks.filter((t) => t.status === col.status);
          return (
            <div key={col.status} className="wa-kanban-col">
              <Card.Root style={{ height: '100%', background: 'var(--wpds-color-bg-surface-neutral-weak)' }}>
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
                      {colTasks.length}
                    </span>
                  </Stack>
                </Card.Header>
                <Card.Content>
                  <Stack direction="column" gap="sm">
                    {colTasks.length === 0 && (
                      <Text variant="body-sm" style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}>
                        No items.
                      </Text>
                    )}
                    {colTasks.map((t) => (
                      <button
                        key={t.id}
                        type="button"
                        onClick={() => nav(routeFor(t))}
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
                                <KindBadge kind={t.kind} />
                                <span
                                  className="wa-mono"
                                  style={{
                                    fontSize: 10,
                                    color: 'var(--wpds-color-fg-content-neutral-weak)',
                                  }}
                                >
                                  {t.id}
                                </span>
                              </Stack>
                              <Text variant="body-sm" style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}>
                                {t.title}
                              </Text>
                              <Stack direction="row" justify="space-between" align="center" style={{ color: 'var(--wpds-color-fg-content-neutral-weak)', fontSize: 11 }}>
                                <span>
                                  {t.kind === 'content' &&
                                    (t.variants.length > 0
                                      ? `${t.variants.length} variants`
                                      : 'no variants yet')}
                                  {t.kind === 'campaign' &&
                                    `${t.children.length} child tasks`}
                                  {t.kind === 'email' && t.subject}
                                </span>
                                <span className="wa-mono">{t.surfacedAt}</span>
                              </Stack>
                            </Stack>
                          </Card.Content>
                        </Card.Root>
                      </button>
                    ))}
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
