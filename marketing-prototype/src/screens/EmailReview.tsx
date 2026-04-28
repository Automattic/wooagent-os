import { useMemo } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Card, Stack, Text } from '@wordpress/ui';
import { Button, Notice } from '@wordpress/components';
import { useApp } from '../App';
import type { EmailTask } from '../data/types';
import { KindBadge, StatusBadge } from '../components/StatusBadge';

export default function EmailReview() {
  const { id } = useParams<{ id: string }>();
  const { tasks, setTaskStatus, pushToast } = useApp();
  const nav = useNavigate();

  const task = useMemo(
    () => tasks.find((t): t is EmailTask => t.id === id && t.kind === 'email'),
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
          That email isn't in the queue.{' '}
          <Link to="/" style={{ textDecoration: 'underline' }}>
            Back to board
          </Link>
        </Notice>
      </main>
    );
  }

  const onApprove = () => {
    setTaskStatus(task.id, 'done');
    pushToast({
      kind: 'success',
      title: 'Email queued for send',
      body: 'Snapshot saved · revert from Done column.',
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
          <KindBadge kind="email" />
        </div>
      </div>

      <main
        style={{
          flex: 1,
          maxWidth: 1100,
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
              drafted an email
            </Text>
            <span style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}>·</span>
            <span
              className="wa-mono"
              style={{
                fontSize: 'var(--wpds-typography-font-size-xs)',
                color: 'var(--wpds-color-fg-content-neutral-weak)',
              }}
            >
              {task.surfacedAt}
            </span>
          </Stack>
          <Text variant="heading-2xl" render={<h1 style={{ margin: 0 }} />}>
            {task.title}
          </Text>
        </Stack>

        <Card.Root style={{ marginBottom: 'var(--wpds-dimension-gap-xl)' }}>
          <Card.Header>
            <Stack direction="column" gap="xs">
              <span className="wa-eyebrow">Subject</span>
              <Text
                variant="body-md"
                style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
              >
                {task.subject}
              </Text>
              <Text variant="body-sm" style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}>
                {task.preview}
              </Text>
            </Stack>
          </Card.Header>
          <Card.Content>
            <pre
              style={{
                whiteSpace: 'pre-wrap',
                fontFamily: 'var(--wpds-typography-font-family-body)',
                fontSize: 'var(--wpds-typography-font-size-md)',
                lineHeight: 1.65,
                margin: 0,
                color: 'var(--wpds-color-fg-content-neutral)',
              }}
            >
              {task.body}
            </pre>
          </Card.Content>
        </Card.Root>
      </main>

      <div className="wa-action-bar">
        <div className="wa-action-bar-row">
          <Stack direction="column" gap="xs">
            <Text
              variant="body-sm"
              style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
            >
              Approve & queue for send
            </Text>
            <Text
              variant="body-sm"
              style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
            >
              Adds to the welcome series; first send 9am tomorrow.
            </Text>
          </Stack>
          <div className="wa-action-bar-actions">
            <Button variant="tertiary" isDestructive>
              Reject
            </Button>
            <Button
              variant="primary"
              onClick={onApprove}
              disabled={task.status !== 'in_review'}
            >
              ✓ Approve email
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
