import { useCallback, useEffect, useRef, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { Card, CollapsibleCard, Notice, Stack, Text } from '@wordpress/ui';
import { Button, Spinner } from '@wordpress/components';
import { Page } from '@wordpress/admin-ui';
import { api, type Connection, type Run } from '../api/client';
import { PersonaAvatar, personaKeyFrom } from '../components/PersonaAvatar';
import { RunStatusBadge } from '../components/RunStatusBadge';
import PageGlobalActions from '../components/PageGlobalActions';
import { useAskAgentContext } from '../lib/askAgent';
import { runToVisible } from '../lib/visibleItems';
import { isLongReason, summarizeReason } from '../lib/runText';

const PAGE_SIZE = 50;

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

function formatLatency(ms: number): string {
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`;
}

// Run row for a run whose skip/failure reason is long (see isLongReason). The
// collapsed header carries the same meta as a short row plus a one-clause
// summary; expanding reveals the full reason and links out to the run detail.
// Used instead of the click-to-open navigate card so the verbose trace no
// longer floods the list inline (the prior behaviour). The whole-card navigate
// affordance moves to the "View run details" link inside the expanded content.
function RunReasonCard({
  run,
  output,
  isFailure,
}: {
  run: Run;
  output: string;
  isFailure: boolean;
}) {
  const personaKey = personaKeyFrom(run.persona);
  const metaColor = 'var(--wpds-color-fg-content-neutral-weak)';
  const xs = 'var(--wpds-typography-font-size-xs)';
  return (
    <CollapsibleCard.Root>
      <CollapsibleCard.Header>
        <Stack direction="column" gap="xs" style={{ flex: 1, minWidth: 0 }}>
          <Stack direction="row" gap="sm" align="center" wrap="wrap">
            <PersonaAvatar persona={personaKey} size="sm" />
            <Text
              variant="body-sm"
              style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
            >
              {personaDisplayName(run.persona)}
            </Text>
            <RunStatusBadge status={run.status} />
            <Text variant="body-sm" style={{ color: metaColor, fontSize: xs }}>
              {triggerLabel(run.trigger)}
            </Text>
            <Text variant="body-sm" style={{ color: metaColor, fontSize: xs }}>
              {relativeTime(run.scheduled_at)}
            </Text>
          </Stack>
          <Stack direction="row" gap="md" align="center" wrap="wrap">
            <span className="wa-mono" style={{ fontSize: xs, color: metaColor }}>
              {run.id.slice(0, 8).toUpperCase()}
            </span>
            {run.latency_ms !== null && (
              <Text variant="body-sm" style={{ fontSize: xs, color: metaColor }}>
                {formatLatency(run.latency_ms)}
              </Text>
            )}
            <Text
              variant="body-sm"
              style={{
                fontSize: xs,
                color: isFailure
                  ? 'var(--wpds-color-fg-content-warning)'
                  : metaColor,
              }}
            >
              {summarizeReason(output)}
            </Text>
          </Stack>
        </Stack>
      </CollapsibleCard.Header>
      <CollapsibleCard.Content>
        <Stack direction="column" gap="sm">
          <Text
            variant="body-sm"
            style={{
              whiteSpace: 'pre-wrap',
              fontSize: xs,
              color: isFailure
                ? 'var(--wpds-color-fg-content-warning)'
                : 'var(--wpds-color-fg-content-neutral)',
            }}
          >
            {output}
          </Text>
          <Stack direction="row" gap="md" align="center" wrap="wrap">
            {run.issue_id && (
              <Link
                to={`/issues/${run.issue_id}`}
                style={{ fontSize: xs, color: metaColor }}
              >
                Issue {run.issue_id.slice(0, 8).toUpperCase()}
              </Link>
            )}
            <Link
              to={`/runs/${run.id}`}
              style={{ fontSize: 'var(--wpds-typography-font-size-sm)' }}
            >
              View run details
            </Link>
          </Stack>
        </Stack>
      </CollapsibleCard.Content>
    </CollapsibleCard.Root>
  );
}

export default function Runs({ connection, onAskAgent }: Props) {
  const navigate = useNavigate();
  const [runs, setRuns] = useState<Run[] | null>(null);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  // paginated flips true the first time the operator clicks Load more. Auto-
  // refresh pauses while paginated — refreshing only page 1 would either
  // drop the appended tail or require merge logic that this view doesn't
  // need. A manual Refresh resets to page 1 and re-enables auto-refresh.
  const [paginated, setPaginated] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useAskAgentContext(
    () => ({
      page: 'runs',
      visible_items: (runs ?? []).map(runToVisible),
    }),
    [runs],
  );

  const fetchFirstPage = useCallback(
    async (signal: { cancelled: boolean }) => {
      try {
        const res = await api.runs.list(connection, { limit: PAGE_SIZE });
        if (!signal.cancelled) {
          setRuns(res.runs);
          setNextCursor(res.next_cursor);
        }
      } catch (e) {
        if (!signal.cancelled) {
          setError(e instanceof Error ? e.message : String(e));
        }
      }
    },
    [connection],
  );

  const loadMore = useCallback(async () => {
    if (!nextCursor || loadingMore) return;
    setLoadingMore(true);
    setPaginated(true);
    try {
      const res = await api.runs.list(connection, {
        limit: PAGE_SIZE,
        cursor: nextCursor,
      });
      setRuns((prev) => (prev ? [...prev, ...res.runs] : res.runs));
      setNextCursor(res.next_cursor);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoadingMore(false);
    }
  }, [connection, nextCursor, loadingMore]);

  const refreshFromTop = useCallback(() => {
    setPaginated(false);
    void fetchFirstPage({ cancelled: false });
  }, [fetchFirstPage]);

  useEffect(() => {
    const signal = { cancelled: false };
    void fetchFirstPage(signal);

    // Auto-refresh every 5s while the tab is visible. Suspended once the
    // operator clicks Load more (see `paginated`).
    intervalRef.current = setInterval(() => {
      if (document.visibilityState === 'visible' && !paginated) {
        void fetchFirstPage(signal);
      }
    }, 5_000);

    return () => {
      signal.cancelled = true;
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [fetchFirstPage, paginated]);

  if (error) {
    return (
      <Page
        title="Runs"
        subTitle="Fleet-wide run history"
        actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
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
        actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
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
      actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
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
            // Failure takes precedence over skip (a run is one or the other in
            // practice). When that reason is long, render the progressive-
            // disclosure card instead of dumping the full trace inline.
            const output = run.failure_reason ?? run.skip_reason ?? null;
            if (isLongReason(output)) {
              return (
                <RunReasonCard
                  key={run.id}
                  run={run}
                  output={(output as string).trim()}
                  isFailure={run.failure_reason !== null}
                />
              );
            }
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
          {(nextCursor || paginated) && (
            <Stack
              direction="column"
              gap="xs"
              align="center"
              style={{ paddingTop: 'var(--wpds-dimension-padding-md)' }}
            >
              {nextCursor && (
                <Button
                  variant="secondary"
                  onClick={loadMore}
                  isBusy={loadingMore}
                  disabled={loadingMore}
                  __next40pxDefaultSize
                >
                  {loadingMore ? 'Loading…' : 'Load more'}
                </Button>
              )}
              {paginated && (
                <Stack direction="row" gap="xs" align="center">
                  <Text
                    variant="body-sm"
                    style={{
                      color: 'var(--wpds-color-fg-content-neutral-weak)',
                      fontSize: 'var(--wpds-typography-font-size-xs)',
                    }}
                  >
                    Auto-refresh paused while paginated.
                  </Text>
                  <Button
                    variant="link"
                    onClick={refreshFromTop}
                    __next40pxDefaultSize
                  >
                    Refresh
                  </Button>
                </Stack>
              )}
            </Stack>
          )}
        </Stack>
      )}
    </Page>
  );
}
