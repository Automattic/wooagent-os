import { useCallback, useEffect, useRef, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { Card, Notice, Stack, Text } from '@wordpress/ui';
import { Spinner } from '@wordpress/components';
import { Page } from '@wordpress/admin-ui';
import { api, type Connection, type Run } from '../api/client';
import { PersonaAvatar, personaKeyFrom } from '../components/PersonaAvatar';
import { RunStatusBadge } from '../components/RunStatusBadge';
import PageGlobalActions from '../components/PageGlobalActions';

interface Props {
  connection: Connection;
  onAskAgent: () => void;
}

function relativeTime(iso: string | null): string {
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

function triggerLabel(trigger: Run['trigger']): string {
  switch (trigger) {
    case 'tick':
      return 'Scheduled';
    case 'manual':
      return 'Manual';
    case 'bootstrap':
      return 'Bootstrap';
    case 'retry':
      return 'Retry';
  }
}

function personaDisplayName(slug: string): string {
  if (slug === 'marketing') return 'Marketing & SEO';
  if (slug === 'inventory') return 'Inventory manager';
  if (slug === 'sales-support') return 'Sales support';
  if (slug === 'chief') return 'Chief of staff';
  return slug.charAt(0).toUpperCase() + slug.slice(1);
}

export default function Runs({ connection, onAskAgent }: Props) {
  const navigate = useNavigate();
  const [runs, setRuns] = useState<Run[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const fetchRuns = useCallback(
    async (signal: { cancelled: boolean }) => {
      try {
        const res = await api.runs.list(connection, { limit: 50 });
        if (!signal.cancelled) setRuns(res.runs);
      } catch (e) {
        if (!signal.cancelled) {
          setError(e instanceof Error ? e.message : String(e));
        }
      }
    },
    [connection],
  );

  useEffect(() => {
    const signal = { cancelled: false };
    void fetchRuns(signal);

    // Auto-refresh every 5s while the tab is visible.
    intervalRef.current = setInterval(() => {
      if (document.visibilityState === 'visible') {
        void fetchRuns(signal);
      }
    }, 5_000);

    return () => {
      signal.cancelled = true;
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [fetchRuns]);

  if (error) {
    return (
      <Page
        title="Runs"
        subTitle="Fleet-wide run history"
        actions={<PageGlobalActions onAskAgent={onAskAgent} />}
        hasPadding
      >
        <Notice.Root intent="error">
          <Notice.Description>
            Failed to load runs: {error}
          </Notice.Description>
        </Notice.Root>
      </Page>
    );
  }

  if (runs === null) {
    return (
      <Page
        title="Runs"
        subTitle="Fleet-wide run history"
        actions={<PageGlobalActions onAskAgent={onAskAgent} />}
        hasPadding
      >
        <Stack direction="row" gap="sm" align="center">
          <Spinner /> <Text variant="body-sm">Loading runs…</Text>
        </Stack>
      </Page>
    );
  }

  return (
    <Page
      title="Runs"
      subTitle={`Fleet-wide run history · ${runs.length} recent`}
      actions={<PageGlobalActions onAskAgent={onAskAgent} />}
      hasPadding
    >
      {runs.length === 0 ? (
        <Stack
          direction="column"
          gap="sm"
          align="center"
          style={{ padding: 'var(--wpds-dimension-padding-2xl)' }}
        >
          <Text variant="body-md">No runs yet.</Text>
          <Text
            variant="body-sm"
            style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
          >
            Runs appear here when agents are triggered by the scheduler or
            manually via the Agents page.
          </Text>
        </Stack>
      ) : (
        <Stack direction="column" gap="sm">
          {runs.map((run) => {
            const personaKey = personaKeyFrom(run.persona);
            return (
              // CUSTOM: whole-card run row click target. (a) WPDS has no clickable-card component. (b) <button> with reset chrome wraps <Card.Root> which owns visuals. (c) Same CardLink pattern noted in Kanban.tsx.
              <button
                key={run.id}
                type="button"
                onClick={() => navigate(`/runs/${run.id}`)}
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
                    <Stack direction="row" gap="md" align="center" wrap="wrap">
                      <PersonaAvatar persona={personaKey} size="sm" />
                      <Stack direction="column" gap="xs" style={{ flex: 1, minWidth: 0 }}>
                        <Stack direction="row" gap="sm" align="center" wrap="wrap">
                          <Text
                            variant="body-sm"
                            style={{
                              fontWeight:
                                'var(--wpds-typography-font-weight-medium)',
                            }}
                          >
                            {personaDisplayName(run.persona)}
                          </Text>
                          <RunStatusBadge status={run.status} />
                          <Text
                            variant="body-sm"
                            style={{
                              color: 'var(--wpds-color-fg-content-neutral-weak)',
                              fontSize: 'var(--wpds-typography-font-size-xs)',
                            }}
                          >
                            {triggerLabel(run.trigger)}
                          </Text>
                          {run.issue_id && (
                            <Link
                              to={`/issues/${run.issue_id}`}
                              onClick={(e) => e.stopPropagation()}
                              style={{
                                fontSize:
                                  'var(--wpds-typography-font-size-xs)',
                                color:
                                  'var(--wpds-color-fg-content-neutral-weak)',
                              }}
                            >
                              Issue {run.issue_id.slice(0, 8).toUpperCase()}
                            </Link>
                          )}
                        </Stack>
                        <Stack direction="row" gap="md" align="center">
                          <span
                            className="wa-mono"
                            style={{
                              fontSize:
                                'var(--wpds-typography-font-size-xs)',
                              color:
                                'var(--wpds-color-fg-content-neutral-weak)',
                            }}
                          >
                            {run.id.slice(0, 8).toUpperCase()}
                          </span>
                          {run.latency_ms !== null && (
                            <Text
                              variant="body-sm"
                              style={{
                                fontSize:
                                  'var(--wpds-typography-font-size-xs)',
                                color:
                                  'var(--wpds-color-fg-content-neutral-weak)',
                              }}
                            >
                              {run.latency_ms >= 1000
                                ? `${(run.latency_ms / 1000).toFixed(1)}s`
                                : `${run.latency_ms}ms`}
                            </Text>
                          )}
                          {run.skip_reason && (
                            <Text
                              variant="body-sm"
                              style={{
                                fontSize:
                                  'var(--wpds-typography-font-size-xs)',
                                color:
                                  'var(--wpds-color-fg-content-neutral-weak)',
                              }}
                            >
                              {run.skip_reason}
                            </Text>
                          )}
                          {run.failure_reason && (
                            <Text
                              variant="body-sm"
                              style={{
                                fontSize:
                                  'var(--wpds-typography-font-size-xs)',
                                color:
                                  'var(--wpds-color-fg-content-warning)',
                              }}
                            >
                              {run.failure_reason}
                            </Text>
                          )}
                        </Stack>
                      </Stack>
                      <Text
                        variant="body-sm"
                        style={{
                          color: 'var(--wpds-color-fg-content-neutral-weak)',
                          fontSize: 'var(--wpds-typography-font-size-xs)',
                          flexShrink: 0,
                        }}
                      >
                        {relativeTime(run.scheduled_at)}
                      </Text>
                    </Stack>
                  </Card.Content>
                </Card.Root>
              </button>
            );
          })}
        </Stack>
      )}
    </Page>
  );
}
