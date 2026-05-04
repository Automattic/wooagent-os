import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Card, Stack, Text } from '@wordpress/ui';
import { Notice, Spinner, Button } from '@wordpress/components';
import { Icon, rotateRight, check } from '@wordpress/icons';
import {
  ApiError,
  api,
  variantsFromProposal,
  type Connection,
  type IssueDetail as IssueDetailPayload,
  type Variant,
} from '../api/client';
import { KindBadge, StatusBadge, kindFromIssue } from '../components/StatusBadge';
import { PersonaAvatar, personaKeyFrom } from '../components/PersonaAvatar';
import Kpi from '../components/Kpi';
import ActionBar from '../components/ActionBar';

interface Props {
  connection: Connection;
  onChanged?: () => void;
}

function relativeTime(iso: string): string {
  const then = new Date(iso).getTime();
  if (!Number.isFinite(then)) return '—';
  const diffMs = Date.now() - then;
  const min = Math.round(diffMs / 60_000);
  if (min < 1) return 'just now';
  if (min < 60) return `${min} minutes ago`;
  const hr = Math.round(min / 60);
  if (hr < 24) return `${hr} hours ago`;
  const days = Math.round(hr / 24);
  return `${days} day${days === 1 ? '' : 's'} ago`;
}

// Choose the score-value color class based on the score band. SEO and Voice
// use slightly different bands (SEO 80+, Voice 90+ for "good") to match the
// Yoast/voice-model conventions.
function seoColorClass(score: number): string {
  if (score >= 80) return 'wa-score-label__value--good';
  if (score >= 70) return 'wa-score-label__value--caution';
  return 'wa-score-label__value--warning';
}
function voiceColorClass(score: number): string {
  if (score >= 90) return 'wa-score-label__value--good';
  if (score >= 75) return 'wa-score-label__value--caution';
  return 'wa-score-label__value--warning';
}

export default function IssueDetail({ connection, onChanged }: Props) {
  const { id } = useParams<{ id: string }>();
  const nav = useNavigate();
  const [data, setData] = useState<IssueDetailPayload | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<'approve' | 'reject' | null>(null);
  const [selectedVariant, setSelectedVariant] = useState<string | null>(null);
  const [actionMsg, setActionMsg] = useState<{
    kind: 'success' | 'error';
    text: string;
  } | null>(null);
  // Variant the operator approved this session — drives the done-state action
  // bar so it can name the right variant after the daemon mutates the issue.
  const [approvedVariant, setApprovedVariant] = useState<string | null>(null);

  useEffect(() => {
    if (!id) return;
    let cancelled = false;
    api
      .issue(connection, id)
      .then((res) => {
        if (cancelled) return;
        setData(res);
        const vs = variantsFromProposal(res.proposal);
        if (vs && vs.length > 0) {
          setSelectedVariant(vs.find((v) => v.recommended)?.id ?? vs[0].id);
        }
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
      const res = await api.approve(connection, id, selectedVariant ?? undefined);
      setApprovedVariant(selectedVariant);
      setData((d) => (d ? { ...d, issue: { ...d.issue, status: res.status } } : d));
      onChanged?.();
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
  const personaKey = personaKeyFrom(issue.persona);
  const personaLabel =
    issue.persona === 'marketing' ? 'Marketing agent' : `${issue.persona ?? 'unassigned'} agent`;
  const reviewable = issue.status === 'in_review';
  const isDone = issue.status === 'done';

  const variants = variantsFromProposal(proposal);
  const activeVariant: Variant | null =
    variants?.find((v) => v.id === selectedVariant) ?? variants?.[0] ?? null;

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
          maxWidth: 1100,
          width: '100%',
          margin: '0 auto',
          padding: 'var(--wpds-dimension-padding-xl) var(--wa-page-pad-x)',
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
            {issue.id.slice(0, 8).toUpperCase()}
          </span>
          <StatusBadge status={issue.status} />
          <KindBadge kind={kind} />
        </Stack>

        {/* Persona eyebrow — avatar + "{Persona} proposes content · 8 minutes ago · Claude Sonnet 4.6" */}
        <Stack direction="row" gap="sm" align="center" style={{ marginBottom: 'var(--wpds-dimension-gap-sm)' }}>
          <PersonaAvatar persona={personaKey} size="md" />
          <Text variant="body-sm" style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}>
            <strong style={{ color: 'var(--wpds-color-fg-content-neutral)' }}>
              {personaLabel}
            </strong>{' '}
            proposes content · {relativeTime(issue.updated_at)} ·{' '}
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
        <div className="wa-kpi-row" style={{ marginBottom: 'var(--wpds-dimension-gap-xl)' }}>
          <Kpi
            label="Scope"
            value={scope}
            hint={
              variants
                ? `${variants.length} variants generated`
                : productBound
                  ? '1 variant generated'
                  : '0 variants generated'
            }
          />
          <Kpi
            label="Brand voice match"
            value={`${activeVariant?.voice ?? 0}%`}
            score={activeVariant?.voice ?? 0}
            hint="vs. your voice model"
          />
          <Kpi
            label="SEO score"
            value={String(activeVariant?.seo ?? 0)}
            score={activeVariant?.seo ?? 0}
            hint="Yoast · out of 100"
          />
          <Kpi label="Est. impact" value="+14% CTR" hint="on product listing pages" intent="success" />
        </div>

        {/* Body */}
        <div className="wa-detail-body">
          <div className="wa-detail-main">
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
                  {variants
                    ? `${variants.length} variants · each with different emphasis`
                    : proposal
                      ? '1 variant · phase-1 single proposal'
                      : 'No proposal attached yet'}
                </Text>
              </Stack>
              {/* Visual-only V1; daemon has no regenerate endpoint yet. */}
              <Button
                variant="tertiary"
                icon={<Icon icon={rotateRight} size={16} />}
                disabled
              >
                Regenerate
              </Button>
            </Stack>

            {!proposal ? (
              <Notice status="info" isDismissible={false}>
                The agent hasn't produced a proposal for this issue yet.
              </Notice>
            ) : variants ? (
              variants
                // Done state: show only the approved variant (others are
                // hidden once we've committed to one).
                .filter((v) => !isDone || v.id === (approvedVariant ?? selectedVariant))
                .map((v) => {
                  const isSelected = selectedVariant === v.id;
                  return (
                    <button
                      key={v.id}
                      type="button"
                      onClick={() => setSelectedVariant(v.id)}
                      disabled={isDone}
                      style={{
                        display: 'block',
                        width: '100%',
                        minWidth: 0,
                        textAlign: 'left',
                        padding: 0,
                        background: 'transparent',
                        border: 'none',
                        cursor: isDone ? 'default' : 'var(--wpds-cursor-control)',
                      }}
                    >
                      <Card.Root
                        className={`wa-variant-card${
                          isSelected ? ' wa-variant-card--selected' : ''
                        }`}
                      >
                        <Card.Content>
                          <div className="wa-variant-header">
                            <div className="wa-variant-header__left">
                              <span
                                style={{
                                  height: 24,
                                  width: 24,
                                  borderRadius: '50%',
                                  display: 'flex',
                                  alignItems: 'center',
                                  justifyContent: 'center',
                                  fontWeight: 700,
                                  fontSize: 12,
                                  flex: 'none',
                                  background: isSelected
                                    ? 'var(--wpds-color-bg-interactive-brand-strong)'
                                    : 'transparent',
                                  color: isSelected
                                    ? '#ffffff'
                                    : 'var(--wpds-color-fg-content-neutral)',
                                  border: isSelected
                                    ? 'none'
                                    : 'var(--wpds-border-width-sm) solid var(--wpds-color-stroke-surface-neutral-strong)',
                                }}
                              >
                                {v.id}
                              </span>
                              {v.recommended && (
                                <span className="wa-agent-pick-inline">Agent pick</span>
                              )}
                              <span
                                style={{
                                  fontSize: 'var(--wpds-typography-font-size-xs)',
                                  padding: '2px 8px',
                                  borderRadius: 'var(--wpds-border-radius-sm)',
                                  background: 'var(--wa-persona-mk-bg)',
                                  color: 'var(--wa-persona-mk-ink)',
                                }}
                              >
                                {v.label}
                              </span>
                            </div>
                            <div className="wa-variant-header__right">
                              <span className="wa-score-label">
                                SEO{' '}
                                <span className={`wa-score-label__value ${seoColorClass(v.seo)}`}>
                                  {v.seo}
                                </span>
                              </span>
                              <span className="wa-score-label">
                                Voice{' '}
                                <span
                                  className={`wa-score-label__value ${voiceColorClass(v.voice)}`}
                                >
                                  {v.voice}%
                                </span>
                              </span>
                              <span className="wa-score-label">
                                <span className="wa-score-label__value">
                                  {v.charCount} chars
                                </span>
                              </span>
                              <span
                                className={`wa-radio-mark${
                                  isSelected ? ' wa-radio-mark--selected' : ''
                                }`}
                                aria-hidden="true"
                              >
                                {isSelected && <Icon icon={check} size={14} />}
                              </span>
                            </div>
                          </div>
                          <Text
                            variant="body-md"
                            style={{ whiteSpace: 'pre-wrap', lineHeight: 1.65 }}
                          >
                            {v.body}
                          </Text>
                          {v.note && (
                            <div
                              style={{
                                marginTop: 'var(--wpds-dimension-gap-md)',
                                paddingTop: 'var(--wpds-dimension-padding-md)',
                                borderTop:
                                  'var(--wpds-border-width-sm) solid var(--wpds-color-stroke-surface-neutral-weak)',
                                fontSize: 'var(--wpds-typography-font-size-xs)',
                                color: 'var(--wpds-color-fg-content-neutral-weak)',
                              }}
                            >
                              {v.note}
                            </div>
                          )}
                        </Card.Content>
                      </Card.Root>
                    </button>
                  );
                })
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
          </div>
        </div>
      </main>

      {isDone ? (
        <ActionBar
          state="done"
          variantId={approvedVariant ?? activeVariant?.id ?? 'A'}
          scope={scope}
          // No daemon undo endpoint yet — for V1 the button just navigates
          // back to the board where Done items live.
          onUndo={() => nav('/')}
          // No store URL surfaced from the daemon; navigate back for V1.
          onView={() => nav('/')}
        />
      ) : (
        <ActionBar
          state="review"
          productBound={productBound}
          busy={busy}
          disabled={!reviewable || !proposal}
          selectedVariantId={activeVariant?.id ?? null}
          onApprove={onApprove}
          onReject={onReject}
          onCancel={() => nav('/')}
        />
      )}
    </div>
  );
}
