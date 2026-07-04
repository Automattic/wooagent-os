import { useCallback, useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { Card, CollapsibleCard, Notice, Stack, Text } from '@wordpress/ui';
import { Button, Spinner } from '@wordpress/components';
import { Page } from '@wordpress/admin-ui';
import {
  ApiError,
  api,
  type Connection,
  type Run,
  type RunDetailResponse,
} from '../api/client';
import { PersonaAvatar, personaKeyFrom } from '../components/PersonaAvatar';
import { RunStatusBadge } from '../components/RunStatusBadge';
import PageGlobalActions from '../components/PageGlobalActions';
import Breadcrumbs from '../components/Breadcrumbs';
import { useAskAgentContext } from '../lib/askAgent';
import {
  formatDateTime,
  personaDisplayName,
  relativeTime,
} from '../lib/boardItems';
import { formatLatency, triggerLabel } from '../lib/run';
import { runToVisible } from '../lib/visibleItems';

interface Props {
  connection: Connection;
  onAskAgent: () => void;
  /** Fired when a polled run transitions from non-terminal to terminal
   *  with status='succeeded'. App.tsx wires this to refreshIssues so the
   *  board picks up any new issue the run created without waiting for
   *  the 10s background poll. */
  onRunTerminal?: () => void;
}

// Safely pull a named array out of the turn_event blob for trace rendering.
function extractEventArray(
  event: unknown,
  key: string,
): unknown[] {
  if (!event || typeof event !== 'object') return [];
  const r = event as Record<string, unknown>;
  return Array.isArray(r[key]) ? (r[key] as unknown[]) : [];
}

function TurnEventTrace({ event }: { event: unknown }) {
  const modelCalls = extractEventArray(event, 'model_calls');
  const skillCalls = extractEventArray(event, 'skill_calls');
  if (modelCalls.length === 0 && skillCalls.length === 0) return null;

  return (
    <Stack direction="column" gap="md">
      {modelCalls.length > 0 && (
        <CollapsibleCard.Root>
          <CollapsibleCard.Header>
            <Card.Title>Model calls ({modelCalls.length})</Card.Title>
          </CollapsibleCard.Header>
          <CollapsibleCard.Content>
            <Stack direction="column" gap="sm">
              {modelCalls.map((call, idx) => (
                <div
                  key={idx}
                  style={{
                    padding: 'var(--wpds-dimension-padding-sm)',
                    borderRadius: 'var(--wpds-border-radius-sm)',
                    background: 'var(--wpds-color-bg-surface-neutral-weak)',
                  }}
                >
                  <Text
                    variant="body-sm"
                    style={{
                      whiteSpace: 'pre-wrap',
                      fontFamily: 'var(--wpds-typography-font-family-body)',
                      fontSize: 'var(--wpds-typography-font-size-xs)',
                      color: 'var(--wpds-color-fg-content-neutral)',
                    }}
                  >
                    {JSON.stringify(call, null, 2)}
                  </Text>
                </div>
              ))}
            </Stack>
          </CollapsibleCard.Content>
        </CollapsibleCard.Root>
      )}
      {skillCalls.length > 0 && (
        <CollapsibleCard.Root>
          <CollapsibleCard.Header>
            <Card.Title>Skill calls ({skillCalls.length})</Card.Title>
          </CollapsibleCard.Header>
          <CollapsibleCard.Content>
            <Stack direction="column" gap="sm">
              {skillCalls.map((call, idx) => (
                <div
                  key={idx}
                  style={{
                    padding: 'var(--wpds-dimension-padding-sm)',
                    borderRadius: 'var(--wpds-border-radius-sm)',
                    background: 'var(--wpds-color-bg-surface-neutral-weak)',
                  }}
                >
                  <Text
                    variant="body-sm"
                    style={{
                      whiteSpace: 'pre-wrap',
                      fontFamily: 'var(--wpds-typography-font-family-body)',
                      fontSize: 'var(--wpds-typography-font-size-xs)',
                      color: 'var(--wpds-color-fg-content-neutral)',
                    }}
                  >
                    {JSON.stringify(call, null, 2)}
                  </Text>
                </div>
              ))}
            </Stack>
          </CollapsibleCard.Content>
        </CollapsibleCard.Root>
      )}
    </Stack>
  );
}

// Terminal statuses for a run — once the run reaches any of these, polling
// stops. Mirror of scheduler/types.go's terminal set. Kept inline rather
// than importing because the shared client types use strings, not enums.
const TERMINAL_STATUSES = new Set<Run['status']>([
  'succeeded',
  'skipped',
  'failed',
  'failed_permanent',
]);

export default function RunDetail({ connection, onAskAgent, onRunTerminal }: Props) {
  const { id } = useParams<{ id: string }>();
  const [data, setData] = useState<RunDetailResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [cancelBusy, setCancelBusy] = useState(false);
  const [cancelError, setCancelError] = useState<string | null>(null);

  const handleCancel = useCallback(async () => {
    if (!id) return;
    setCancelBusy(true);
    setCancelError(null);
    try {
      const res = await api.runs.cancel(connection, id);
      // Optimistically reflect the new terminal status in the header so the
      // operator sees the cancel land before the next 2s poll tick.
      setData((prev) => (prev ? { ...prev, run: res.run } : prev));
    } catch (e) {
      const msg =
        e instanceof ApiError
          ? `${e.code}: ${e.message}`
          : e instanceof Error
            ? e.message
            : String(e);
      setCancelError(msg);
    } finally {
      setCancelBusy(false);
    }
  }, [connection, id]);

  useAskAgentContext(
    () => ({
      page: 'run-detail',
      visible_items: data?.run ? [runToVisible(data.run)] : [],
    }),
    [data],
  );

  useEffect(() => {
    if (!id) return;
    let cancelled = false;
    let intervalId: ReturnType<typeof setInterval> | null = null;
    // Tracks the previous status so we only fire onRunTerminal once, on
    // the transition into a terminal state. Subsequent polls (which
    // shouldn't happen because we stop the interval) would otherwise
    // re-fire it.
    let firedTerminal = false;

    const fetchOnce = async () => {
      try {
        const res = await api.runs.get(connection, id);
        if (cancelled) return;
        setData(res);
        const status = res.run.status;
        if (TERMINAL_STATUSES.has(status)) {
          if (intervalId) {
            clearInterval(intervalId);
            intervalId = null;
          }
          if (!firedTerminal && status === 'succeeded' && onRunTerminal) {
            firedTerminal = true;
            onRunTerminal();
          }
        }
      } catch (e) {
        if (!cancelled) {
          setError(e instanceof Error ? e.message : String(e));
        }
      }
    };

    void fetchOnce();
    // Poll every 2s while the run is non-terminal. fetchOnce clears the
    // interval the moment it sees a terminal status, so the steady-state
    // cost on a long-since-completed run is one initial fetch.
    intervalId = setInterval(() => {
      if (document.visibilityState !== 'visible') return;
      void fetchOnce();
    }, 2_000);

    return () => {
      cancelled = true;
      if (intervalId) clearInterval(intervalId);
    };
  }, [connection, id, onRunTerminal]);

  const runLabel = id ? id.slice(0, 8).toUpperCase() : 'Run';

  if (error) {
    return (
      <Page
        breadcrumbs={
          <Breadcrumbs items={[{ label: 'Runs', to: '/runs' }, { label: runLabel }]} />
        }
        actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
        hasPadding
      >
        <div className="wa-subpage-content">
          <Notice.Root intent="error">
            <Notice.Description>
              Failed to load run: {error}
            </Notice.Description>
          </Notice.Root>
        </div>
      </Page>
    );
  }

  if (!data) {
    return (
      <Page
        breadcrumbs={
          <Breadcrumbs items={[{ label: 'Runs', to: '/runs' }, { label: runLabel }]} />
        }
        actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
        hasPadding
      >
        <div className="wa-subpage-content">
          <Stack direction="row" gap="sm" align="center">
            <Spinner /> <Text variant="body-sm">Loading run…</Text>
          </Stack>
        </div>
      </Page>
    );
  }

  const { run, turn_event, retry_chain } = data;
  const personaKey = personaKeyFrom(run.persona);

  return (
    <Page
      breadcrumbs={
        <Breadcrumbs
          items={[
            { label: 'Runs', to: '/runs' },
            { label: run.id.slice(0, 8).toUpperCase() },
          ]}
        />
      }
      badges={<RunStatusBadge status={run.status} />}
      actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
      hasPadding
    >
      <div className="wa-subpage-content">
      <Stack direction="column" gap="lg">
        {cancelError && (
          <Notice.Root intent="error">
            <Notice.Description>Cancel failed: {cancelError}</Notice.Description>
          </Notice.Root>
        )}

        {/* Header card */}
        <Card.Root>
          <Card.Header>
            <Stack direction="row" gap="sm" align="center" justify="space-between">
              <Stack direction="row" gap="sm" align="center">
                <PersonaAvatar persona={personaKey} size="md" />
                <Text
                  variant="body-sm"
                  style={{
                    fontWeight: 'var(--wpds-typography-font-weight-medium)',
                  }}
                >
                  {personaDisplayName(run.persona)}
                </Text>
                <RunStatusBadge status={run.status} />
              </Stack>
              {(run.status === 'queued' || run.status === 'running') && (
                <Button
                  variant="secondary"
                  __next40pxDefaultSize
                  isDestructive
                  disabled={cancelBusy}
                  isBusy={cancelBusy}
                  onClick={() => void handleCancel()}
                >
                  <Stack direction="row" gap="xs" align="center">
                    {/* CUSTOM: @wordpress/components Spinner ships with a
                        legacy admin-bar margin (5px 11px 0 0) that pushes
                        it low + adds dead space inside the button. Zero
                        it out so Stack's align="center" + gap="xs" govern
                        the layout. */}
                    <Spinner style={{ margin: 0 }} />
                    <span>Cancel run</span>
                  </Stack>
                </Button>
              )}
            </Stack>
          </Card.Header>
          <Card.Content>
            <Stack direction="column" gap="sm">
              <Stack direction="row" gap="xl" wrap="wrap">
                <Stack direction="column" gap="xs">
                  <span className="wa-eyebrow">Trigger</span>
                  <Text variant="body-sm">{triggerLabel(run.trigger)}</Text>
                </Stack>
                <Stack direction="column" gap="xs">
                  <span className="wa-eyebrow">Attempt</span>
                  <Text variant="body-sm">{run.attempt}</Text>
                </Stack>
                {run.latency_ms !== null && (
                  <Stack direction="column" gap="xs">
                    <span className="wa-eyebrow">Latency</span>
                    <Text variant="body-sm">{formatLatency(run.latency_ms)}</Text>
                  </Stack>
                )}
                {run.issue_id && (
                  <Stack direction="column" gap="xs">
                    <span className="wa-eyebrow">Issue</span>
                    <Link
                      to={`/issues/${run.issue_id}`}
                      style={{
                        fontSize: 'var(--wpds-typography-font-size-sm)',
                        color: 'var(--wpds-color-fg-content-neutral)',
                      }}
                    >
                      {run.issue_id.slice(0, 8).toUpperCase()}
                    </Link>
                  </Stack>
                )}
              </Stack>

              <Stack direction="row" gap="xl" wrap="wrap">
                <Stack direction="column" gap="xs">
                  <span className="wa-eyebrow">Scheduled</span>
                  <Text
                    variant="body-sm"
                    className="wa-mono"
                    style={{
                      fontSize: 'var(--wpds-typography-font-size-xs)',
                      color: 'var(--wpds-color-fg-content-neutral-weak)',
                    }}
                  >
                    {formatDateTime(run.scheduled_at)} · {relativeTime(run.scheduled_at)}
                  </Text>
                </Stack>
                {run.claimed_at && (
                  <Stack direction="column" gap="xs">
                    <span className="wa-eyebrow">Claimed</span>
                    <Text
                      variant="body-sm"
                      className="wa-mono"
                      style={{
                        fontSize: 'var(--wpds-typography-font-size-xs)',
                        color: 'var(--wpds-color-fg-content-neutral-weak)',
                      }}
                    >
                      {formatDateTime(run.claimed_at)}
                    </Text>
                  </Stack>
                )}
                {run.completed_at && (
                  <Stack direction="column" gap="xs">
                    <span className="wa-eyebrow">Completed</span>
                    <Text
                      variant="body-sm"
                      className="wa-mono"
                      style={{
                        fontSize: 'var(--wpds-typography-font-size-xs)',
                        color: 'var(--wpds-color-fg-content-neutral-weak)',
                      }}
                    >
                      {formatDateTime(run.completed_at)}
                    </Text>
                  </Stack>
                )}
              </Stack>

              {run.skip_reason && (
                <Stack direction="column" gap="xs">
                  <span className="wa-eyebrow">Skip reason</span>
                  <Text
                    variant="body-sm"
                    style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
                  >
                    {run.skip_reason}
                  </Text>
                </Stack>
              )}

              {run.failure_reason && (
                <Stack direction="column" gap="xs">
                  <span className="wa-eyebrow">Failure reason</span>
                  <Text
                    variant="body-sm"
                    style={{ color: 'var(--wpds-color-fg-content-warning)' }}
                  >
                    {run.failure_reason}
                    {run.failure_class
                      ? ` · ${run.failure_class}`
                      : ''}
                  </Text>
                </Stack>
              )}
            </Stack>
          </Card.Content>
        </Card.Root>

        {/* Trace (turn_event) */}
        {turn_event !== null && (
          <TurnEventTrace event={turn_event} />
        )}

        {/* Retry chain */}
        {retry_chain.length > 1 && (
          <Card.Root>
            <Card.Header>
              <span className="wa-eyebrow">
                Retry chain · {retry_chain.length} attempts
              </span>
            </Card.Header>
            <Card.Content>
              <Stack direction="column" gap="sm">
                {retry_chain.map((r, idx) => (
                  <Stack
                    key={r.id}
                    direction="row"
                    gap="md"
                    align="center"
                  >
                    <Text
                      variant="body-sm"
                      style={{
                        color: 'var(--wpds-color-fg-content-neutral-weak)',
                        fontSize: 'var(--wpds-typography-font-size-xs)',
                        minWidth: 20,
                      }}
                    >
                      #{idx + 1}
                    </Text>
                    <RunStatusBadge status={r.status} />
                    <Text
                      variant="body-sm"
                      style={{
                        fontSize: 'var(--wpds-typography-font-size-xs)',
                        color: 'var(--wpds-color-fg-content-neutral-weak)',
                      }}
                    >
                      {relativeTime(r.scheduled_at)}
                    </Text>
                    {r.id !== run.id ? (
                      <Link
                        to={`/runs/${r.id}`}
                        style={{
                          fontSize: 'var(--wpds-typography-font-size-xs)',
                          color: 'var(--wpds-color-fg-content-neutral-weak)',
                        }}
                      >
                        {r.id.slice(0, 8).toUpperCase()}
                      </Link>
                    ) : (
                      <span
                        className="wa-mono"
                        style={{
                          fontSize: 'var(--wpds-typography-font-size-xs)',
                          color: 'var(--wpds-color-fg-content-neutral)',
                          fontWeight: 'var(--wpds-typography-font-weight-medium)',
                        }}
                      >
                        {r.id.slice(0, 8).toUpperCase()} (current)
                      </span>
                    )}
                    {r.latency_ms !== null && (
                      <Text
                        variant="body-sm"
                        style={{
                          fontSize: 'var(--wpds-typography-font-size-xs)',
                          color: 'var(--wpds-color-fg-content-neutral-weak)',
                        }}
                      >
                        {formatLatency(r.latency_ms)}
                      </Text>
                    )}
                  </Stack>
                ))}
              </Stack>
            </Card.Content>
          </Card.Root>
        )}
      </Stack>
      </div>
    </Page>
  );
}
