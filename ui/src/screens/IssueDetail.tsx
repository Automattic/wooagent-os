import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Card, Stack, Text } from '@wordpress/ui';
import { Notice, Spinner } from '@wordpress/components';
import {
  ApiError,
  api,
  type Connection,
  type IssueDetail as IssueDetailPayload,
} from '../api/client';
import { KindBadge, StatusBadge, kindFromIssue } from '../components/StatusBadge';
import Kpi from '../components/Kpi';
import SidebarRail from '../components/SidebarRail';
import ActionBar from '../components/ActionBar';

interface Props {
  connection: Connection;
  onChanged?: () => void;
}

export default function IssueDetail({ connection, onChanged }: Props) {
  const { id } = useParams<{ id: string }>();
  const nav = useNavigate();
  const [data, setData] = useState<IssueDetailPayload | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<'approve' | 'reject' | null>(null);
  const [actionMsg, setActionMsg] = useState<{
    kind: 'success' | 'error';
    text: string;
  } | null>(null);

  useEffect(() => {
    if (!id) return;
    let cancelled = false;
    api
      .issue(connection, id)
      .then((res) => {
        if (!cancelled) setData(res);
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      });
    return () => {
      cancelled = true;
    };
  }, [connection, id]);

  const onApprove = async () => {
    if (!id) return;
    setBusy('approve');
    setActionMsg(null);
    try {
      const res = await api.approve(connection, id);
      setActionMsg({
        kind: 'success',
        text: `Applied via ${res.ability ?? 'MCP'} — issue moved to ${res.status}.`,
      });
      setData((d) => (d ? { ...d, issue: { ...d.issue, status: res.status } } : d));
      onChanged?.();
      setTimeout(() => nav('/'), 1200);
    } catch (e) {
      setActionMsg({
        kind: 'error',
        text:
          e instanceof ApiError
            ? `${e.code}: ${e.message}`
            : e instanceof Error
              ? e.message
              : String(e),
      });
    } finally {
      setBusy(null);
    }
  };

  const onReject = async () => {
    if (!id) return;
    setBusy('reject');
    setActionMsg(null);
    try {
      const res = await api.reject(connection, id);
      setActionMsg({
        kind: 'success',
        text: `Rejected — issue moved to ${res.status}.`,
      });
      setData((d) => (d ? { ...d, issue: { ...d.issue, status: res.status } } : d));
      onChanged?.();
      setTimeout(() => nav('/'), 1200);
    } catch (e) {
      setActionMsg({
        kind: 'error',
        text:
          e instanceof ApiError
            ? `${e.code}: ${e.message}`
            : e instanceof Error
              ? e.message
              : String(e),
      });
    } finally {
      setBusy(null);
    }
  };

  if (error) {
    return (
      <main style={{ padding: 'var(--wpds-dimension-padding-2xl)' }}>
        <Notice status="error" isDismissible={false}>
          Failed to load issue: {error} <Link to="/">Back to board</Link>
        </Notice>
      </main>
    );
  }
  if (!data) {
    return (
      <main style={{ padding: 'var(--wpds-dimension-padding-2xl)' }}>
        <Stack direction="row" gap="sm" align="center">
          <Spinner /> <Text variant="body-sm">Loading issue…</Text>
        </Stack>
      </main>
    );
  }

  const { issue, proposal } = data;
  const kind = kindFromIssue(issue);
  const target = (proposal?.target ?? {}) as Record<string, unknown>;
  const previous = typeof target.previous === 'string' ? target.previous : undefined;
  const productSku = typeof target.sku === 'string' ? target.sku : undefined;
  const productName =
    typeof target.product_name === 'string' ? target.product_name : undefined;
  const productBound = typeof target.product_id === 'number' || !!productSku;
  const scope = productName ?? productSku ?? '—';
  const persona = issue.persona ?? 'unassigned';
  const reviewable = issue.status === 'in_review';

  // Phase-1 placeholders: scoring data isn't emitted by the daemon yet.
  // SidebarRail and Kpi tiles render fixed content so the layout reads
  // correctly. Wire to real data when the daemon ships scorers.

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        minHeight: '100vh',
      }}
    >
      <main
        style={{
          flex: 1,
          maxWidth: 1500,
          width: '100%',
          margin: '0 auto',
          padding: 'var(--wpds-dimension-padding-xl) var(--wpds-dimension-padding-lg)',
        }}
      >
        {/* Breadcrumb */}
        <Stack direction="row" gap="sm" align="center" style={{ marginBottom: 'var(--wpds-dimension-gap-md)' }}>
          <Link
            to="/"
            style={{
              color: 'var(--wpds-color-fg-content-neutral-weak)',
              fontSize: 'var(--wpds-typography-font-size-sm)',
            }}
          >
            ← Board
          </Link>
          <span
            className="wa-mono"
            style={{
              fontSize: 'var(--wpds-typography-font-size-sm)',
              color: 'var(--wpds-color-fg-content-neutral-weak)',
            }}
          >
            {issue.id.slice(0, 8)}
          </span>
          <StatusBadge status={issue.status} />
          <KindBadge kind={kind} />
        </Stack>

        {/* Persona eyebrow */}
        <Stack direction="row" gap="sm" align="center" style={{ marginBottom: 'var(--wpds-dimension-gap-sm)' }}>
          <span
            style={{
              height: 24,
              width: 24,
              borderRadius: 'var(--wpds-border-radius-md)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontWeight: 700,
              fontSize: 10,
              background: 'var(--wa-persona-mk-bg)',
              color: 'var(--wa-persona-mk-ink)',
              flex: 'none',
            }}
          >
            MK
          </span>
          <Text variant="body-sm" style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}>
            <strong style={{ color: 'var(--wpds-color-fg-content-neutral)' }}>
              {persona === 'marketing' ? 'Marketing agent' : `${persona} agent`}
            </strong>{' '}
            proposes content · {issue.status.replace('_', ' ')} ·{' '}
            <span className="wa-mono">Claude Sonnet 4.6</span>
          </Text>
        </Stack>

        {/* Title + subhead */}
        <Text
          variant="heading-2xl"
          render={<h1 style={{ margin: 0, marginBottom: 'var(--wpds-dimension-gap-sm)' }} />}
        >
          {issue.title}
        </Text>
        <Text
          variant="body-md"
          style={{
            color: 'var(--wpds-color-fg-content-neutral-weak)',
            maxWidth: 760,
            marginBottom: 'var(--wpds-dimension-gap-xl)',
          }}
        >
          {issue.description ??
            'Three voice variants. Pick one, approve, and the agent writes it straight to WooCommerce. The previous copy is snapshotted — reversible from the Done column.'}
        </Text>

        {/* KPI row */}
        <Stack direction="row" gap="md" style={{ marginBottom: 'var(--wpds-dimension-gap-xl)' }}>
          <Kpi label="Scope" value={scope} hint={productBound ? '1 variant generated' : '0 variants generated'} />
          <Kpi label="Brand voice match" value="0%" score={0} hint="vs. your voice model" />
          <Kpi label="SEO score" value="0" score={0} hint="Yoast · out of 100" />
          <Kpi label="Est. impact" value="+14% CTR" hint="on product listing pages" intent="success" />
        </Stack>

        {/* Body: main column + sidebar rail */}
        <Stack direction="row" gap="lg" align="start">
          <Stack direction="column" gap="lg" style={{ flex: 1, minWidth: 0 }}>
            {/* Current description */}
            <Card.Root>
              <Card.Header>
                <Stack direction="row" justify="space-between" align="center">
                  <Stack direction="row" gap="sm" align="center">
                    <span className="wa-eyebrow">Current description</span>
                    {productBound ? (
                      <span
                        style={{
                          fontSize: 'var(--wpds-typography-font-size-xs)',
                          padding: '2px 8px',
                          borderRadius: 'var(--wpds-border-radius-sm)',
                          background: 'var(--wpds-color-bg-surface-success-weak)',
                          color: 'var(--wpds-color-fg-content-success)',
                        }}
                      >
                        Bound: {productSku}
                      </span>
                    ) : (
                      <span
                        style={{
                          fontSize: 'var(--wpds-typography-font-size-xs)',
                          padding: '2px 8px',
                          borderRadius: 'var(--wpds-border-radius-sm)',
                          background: 'var(--wpds-color-bg-surface-warning-weak)',
                          color: 'var(--wpds-color-fg-content-warning)',
                        }}
                      >
                        No product bound
                      </span>
                    )}
                  </Stack>
                  <span
                    className="wa-mono"
                    style={{
                      fontSize: 'var(--wpds-typography-font-size-xs)',
                      color: 'var(--wpds-color-fg-content-neutral-weak)',
                    }}
                  >
                    {previous ? `${previous.length} chars` : '0 chars · sample'}
                  </span>
                </Stack>
              </Card.Header>
              <Card.Content>
                <Text
                  variant="body-sm"
                  style={{
                    color: previous
                      ? 'var(--wpds-color-fg-content-neutral)'
                      : 'var(--wpds-color-fg-content-neutral-weak)',
                    whiteSpace: 'pre-wrap',
                    minHeight: 60,
                  }}
                >
                  {previous ?? '— no current copy on this product —'}
                </Text>
              </Card.Content>
            </Card.Root>

            {/* Proposed section */}
            <Stack direction="row" justify="space-between" align="end">
              <Stack direction="column" gap="xs">
                <span className="wa-eyebrow wa-eyebrow--persona">Proposed · pick one</span>
                <Text variant="heading-md">
                  {proposal
                    ? '1 variant · phase-1 single proposal'
                    : 'No proposal attached yet'}
                </Text>
              </Stack>
            </Stack>

            {!proposal ? (
              <Notice status="info" isDismissible={false}>
                The agent hasn't produced a proposal for this issue yet.
              </Notice>
            ) : (
              <Card.Root
                className="wa-variant-card wa-variant-card--selected"
                style={{ borderWidth: 'var(--wpds-border-width-md)' }}
              >
                <Card.Header>
                  <Stack direction="row" justify="space-between" align="center">
                    <Stack direction="row" gap="sm" align="center">
                      <span
                        style={{
                          fontFamily: 'var(--wpds-typography-font-family-mono)',
                          fontSize: 12,
                          fontWeight: 700,
                          padding: '2px 8px',
                          borderRadius: 'var(--wpds-border-radius-sm)',
                          background: 'var(--wpds-color-bg-interactive-brand-strong)',
                          color: '#ffffff',
                        }}
                      >
                        A
                      </span>
                      <Text
                        variant="body-sm"
                        style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
                      >
                        {proposal.type}
                      </Text>
                    </Stack>
                    <span
                      className="wa-mono"
                      style={{
                        fontSize: 'var(--wpds-typography-font-size-xs)',
                        color: 'var(--wpds-color-fg-content-neutral-weak)',
                      }}
                    >
                      {proposal.content.length} chars
                    </span>
                  </Stack>
                </Card.Header>
                <Card.Content>
                  <Text
                    variant="body-md"
                    style={{ whiteSpace: 'pre-wrap', lineHeight: 1.65 }}
                  >
                    {proposal.content}
                  </Text>
                </Card.Content>
              </Card.Root>
            )}

            {actionMsg && (
              <Notice
                status={actionMsg.kind === 'success' ? 'success' : 'error'}
                isDismissible={false}
              >
                {actionMsg.text}
              </Notice>
            )}
          </Stack>

          <SidebarRail />
        </Stack>
      </main>

      <ActionBar
        productBound={productBound}
        busy={busy}
        disabled={!reviewable || !proposal}
        onApprove={onApprove}
        onReject={onReject}
        onCancel={() => nav('/')}
      />
    </div>
  );
}
