import { useMemo } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Card, Stack, Text } from '@wordpress/ui';
import { Button, Notice } from '@wordpress/components';
import { useApp } from '../App';
import type { CampaignTask } from '../data/types';
import { KindBadge, StatusBadge } from '../components/StatusBadge';

export default function CampaignPlanner() {
  const { id } = useParams<{ id: string }>();
  const { tasks, setTaskStatus, pushToast } = useApp();
  const nav = useNavigate();

  const task = useMemo(
    () => tasks.find((t): t is CampaignTask => t.id === id && t.kind === 'campaign'),
    [tasks, id],
  );

  if (!task) {
    return (
      <main
        style={{
          maxWidth: 900,
          margin: '0 auto',
          padding: 'var(--wpds-dimension-padding-2xl) var(--wpds-dimension-padding-2xl)',
        }}
      >
        <Notice status="warning" isDismissible={false}>
          That campaign isn't in the queue.{' '}
          <Link to="/" style={{ textDecoration: 'underline' }}>
            Back to board
          </Link>
        </Notice>
      </main>
    );
  }

  const onApprovePlan = () => {
    setTaskStatus(task.id, 'drafting');
    pushToast({
      kind: 'success',
      title: 'Plan approved · child tasks scheduled',
      body: `${task.children.length} components are queued. Each one still hits In Review when drafted — approving the plan doesn't ship anything.`,
    });
    nav('/');
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', minHeight: '100vh' }}>
      <div
        style={{
          background: 'var(--wpds-color-bg-surface-neutral)',
          borderBottom:
            'var(--wpds-border-width-sm) solid var(--wpds-color-stroke-surface-neutral-weak)',
        }}
      >
        <div
          style={{
            maxWidth: 1500,
            margin: '0 auto',
            padding: '0 var(--wpds-dimension-padding-lg)',
            height: 48,
            display: 'flex',
            alignItems: 'center',
            gap: 'var(--wpds-dimension-gap-sm)',
          }}
        >
          <Link
            to="/"
            style={{
              color: 'var(--wpds-color-fg-content-neutral-weak)',
              fontSize: 'var(--wpds-typography-font-size-xs)',
            }}
          >
            ← Board
          </Link>
          <span
            style={{
              height: 16,
              width: 1,
              background: 'var(--wpds-color-stroke-surface-neutral)',
            }}
          />
          <span
            className="wa-mono"
            style={{
              fontSize: 'var(--wpds-typography-font-size-xs)',
              color: 'var(--wpds-color-fg-content-neutral-weak)',
            }}
          >
            {task.id}
          </span>
          <StatusBadge status={task.status} />
          <KindBadge kind="campaign" />
        </div>
      </div>

      <main
        style={{
          flex: 1,
          maxWidth: 1200,
          width: '100%',
          margin: '0 auto',
          padding: 'var(--wpds-dimension-padding-2xl) var(--wpds-dimension-padding-2xl)',
        }}
      >
        <Stack direction="column" gap="sm" style={{ marginBottom: 'var(--wpds-dimension-gap-xl)' }}>
          <Stack direction="row" gap="sm" align="center">
            <span
              style={{
                height: 24,
                width: 24,
                borderRadius: 'var(--wpds-border-radius-md)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                fontWeight: 700,
                fontSize: 11,
                background: 'var(--wa-persona-mk-bg)',
                color: 'var(--wa-persona-mk-ink)',
                flex: 'none',
              }}
            >
              MK
            </span>
            <Text variant="body-sm" style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}>
              <strong style={{ color: 'var(--wpds-color-fg-content-neutral)' }}>Marketing agent</strong>{' '}
              proposes a plan
            </Text>
            <span style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}>·</span>
            <span
              className="wa-mono"
              style={{ fontSize: 'var(--wpds-typography-font-size-xs)', color: 'var(--wpds-color-fg-content-neutral-weak)' }}
            >
              {task.surfacedAt}
            </span>
          </Stack>
          <Text variant="heading-2xl" render={<h1 style={{ margin: 0 }} />}>
            {task.title}
          </Text>
          <Text variant="body-sm" style={{ maxWidth: 760, color: 'var(--wpds-color-fg-content-neutral-weak)' }}>
            One strategic decision: is this the right plan? If yes, the agent
            schedules the components and brings each one back through Review as
            it's drafted.
          </Text>
        </Stack>

        <div className="wa-detail-body">
          <div className="wa-detail-main">
            <Card.Root>
              <Card.Header>
                <span className="wa-eyebrow">Brief</span>
              </Card.Header>
              <Card.Content>
                <Stack direction="row" gap="md" wrap="wrap">
                  {(
                    [
                      ['Goal', task.goal],
                      ['Audience', task.audience],
                      ['Window', `${task.startsOn} → ${task.endsOn}`],
                      ['Budget', task.budget],
                    ] as const
                  ).map(([label, value]) => (
                    <Stack
                      key={label}
                      direction="column"
                      gap="xs"
                      style={{ flex: '1 1 240px', minWidth: 0 }}
                    >
                      <span
                        style={{
                          fontSize: 'var(--wpds-typography-font-size-xs)',
                          color: 'var(--wpds-color-fg-content-neutral-weak)',
                        }}
                      >
                        {label}
                      </span>
                      <Text variant="body-sm">{value}</Text>
                    </Stack>
                  ))}
                </Stack>
              </Card.Content>
            </Card.Root>

            <Card.Root>
              <Card.Header>
                <Stack direction="row" justify="space-between" align="center">
                  <span className="wa-eyebrow">
                    Proposed components · {task.children.length}
                  </span>
                  <span
                    style={{
                      fontSize: 'var(--wpds-typography-font-size-xs)',
                      color: 'var(--wpds-color-fg-content-neutral-weak)',
                    }}
                  >
                    Each drafts on its own schedule, returns to In Review.
                  </span>
                </Stack>
              </Card.Header>
              <Card.Content>
                <Stack direction="column" gap="sm">
                  {task.children.map((c, i) => (
                    <Stack
                      key={c.id}
                      direction="row"
                      gap="sm"
                      align="center"
                      style={{
                        padding:
                          'var(--wpds-dimension-padding-sm) var(--wpds-dimension-padding-md)',
                        borderBottom:
                          i === task.children.length - 1
                            ? 'none'
                            : 'var(--wpds-border-width-sm) solid var(--wpds-color-stroke-surface-neutral-weak)',
                      }}
                    >
                      <span
                        style={{
                          height: 28,
                          width: 28,
                          borderRadius: '50%',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          fontWeight: 700,
                          fontSize: 12,
                          background: 'var(--wa-persona-mk-bg)',
                          color: 'var(--wa-persona-mk-ink)',
                          flex: 'none',
                        }}
                      >
                        {i + 1}
                      </span>
                      <Stack direction="column" gap="xs" style={{ flex: 1, minWidth: 0 }}>
                        <Text
                          variant="body-sm"
                          style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
                        >
                          {c.title}
                        </Text>
                        <span
                          className="wa-mono"
                          style={{
                            fontSize: 11,
                            color: 'var(--wpds-color-fg-content-neutral-weak)',
                          }}
                        >
                          {c.id} · {c.scheduledFor}
                        </span>
                      </Stack>
                      <StatusBadge status={c.status} />
                    </Stack>
                  ))}
                </Stack>
              </Card.Content>
            </Card.Root>
          </div>

          <div className="wa-detail-rail">
          <Stack direction="column" gap="md" style={{ width: '100%' }}>
            <Card.Root>
              <Card.Header>
                <span
                  className="wa-eyebrow"
                  style={{ color: 'var(--wpds-color-fg-interactive-brand)' }}
                >
                  Why split like this
                </span>
              </Card.Header>
              <Card.Content>
                <Text variant="body-sm">
                  A campaign has many pieces but you only make one strategic
                  decision:{' '}
                  <strong>is this the right plan?</strong> The rest are
                  executions.
                </Text>
              </Card.Content>
            </Card.Root>

            <Card.Root
              style={{
                background: 'var(--wpds-color-bg-surface-warning-weak)',
                borderColor: 'var(--wpds-color-stroke-surface-warning)',
              }}
            >
              <Card.Content>
                <Stack direction="column" gap="xs">
                  <Text
                    variant="body-sm"
                    style={{
                      fontWeight: 'var(--wpds-typography-font-weight-medium)',
                      color: 'var(--wpds-color-fg-content-warning)',
                    }}
                  >
                    Failsafe
                  </Text>
                  <Text
                    variant="body-sm"
                    style={{ color: 'var(--wpds-color-fg-content-warning-weak)' }}
                  >
                    Approving the plan doesn't auto-ship anything. Every child
                    returns through In Review when drafted.
                  </Text>
                </Stack>
              </Card.Content>
            </Card.Root>
          </Stack>
          </div>
        </div>
      </main>

      <div className="wa-action-bar">
        <div className="wa-action-bar-row">
          <Stack direction="column" gap="xs">
            <Text
              variant="body-sm"
              style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
            >
              Approve the plan
            </Text>
            <Text
              variant="body-sm"
              style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
            >
              Schedules {task.children.length} child tasks · no content ships
              without your review.
            </Text>
          </Stack>
          <div className="wa-action-bar-actions">
            <Button variant="tertiary">Modify</Button>
            <Button variant="tertiary" isDestructive>
              Reject
            </Button>
            <Button
              variant="primary"
              onClick={onApprovePlan}
              disabled={task.status !== 'in_review'}
            >
              ✓ Approve plan
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
