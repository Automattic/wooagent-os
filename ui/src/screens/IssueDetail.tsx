import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Card, CollapsibleCard, Notice, Stack, Text } from '@wordpress/ui';
import { Spinner, Button } from '@wordpress/components';
import { Icon, arrowUpRight, rotateRight, check } from '@wordpress/icons';
import { Page } from '@wordpress/admin-ui';
import {
  ApiError,
  api,
  messageProposalFromProposal,
  priceProposalFromProposal,
  variantsFromProposal,
  type Connection,
  type DismissReason,
  type IssueDetail as IssueDetailPayload,
  type MessageProposal,
  type PriceProposal,
  type Run,
  type Variant,
} from '../api/client';
import {
  KindBadge,
  StatusBadge,
  kindFromProposalType,
} from '../components/StatusBadge';
import { PersonaAvatar, personaKeyFrom } from '../components/PersonaAvatar';
import { RunStatusBadge } from '../components/RunStatusBadge';
import Kpi from '../components/Kpi';
import ActionBar from '../components/ActionBar';
import DismissDialog from '../components/DismissDialog';
import PageGlobalActions from '../components/PageGlobalActions';

interface Props {
  connection: Connection;
  onChanged?: () => void;
  onAskAgent: () => void;
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

const CURRENCY_SYMBOL: Record<string, string> = {
  USD: '$',
  CAD: '$',
  AUD: '$',
  GBP: '£',
  EUR: '€',
};

function formatPrice(amount: number, currency: string): string {
  const sym = CURRENCY_SYMBOL[currency.toUpperCase()] ?? '';
  return `${sym}${amount.toFixed(2)}`;
}

function formatDelta(prev: number, next: number, currency: string): string {
  const diff = next - prev;
  const sign = diff > 0 ? '+' : diff < 0 ? '−' : '';
  return `${sign}${formatPrice(Math.abs(diff), currency)}`;
}

export default function IssueDetail({ connection, onChanged, onAskAgent }: Props) {
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
  const [approvedVariant, setApprovedVariant] = useState<string | null>(null);
  const [dismissOpen, setDismissOpen] = useState(false);

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

  // The Reject button now opens the Dismiss dialog instead of dismissing
  // directly — the dialog captures a reason + optional comment, then calls
  // api.dismiss on confirm. See DismissDialog + handleDismissConfirm below.
  const onReject = () => {
    if (!id) return;
    setActionMsg(null);
    setDismissOpen(true);
  };

  const handleDismissConfirm = async ({
    reason,
    comment,
  }: {
    reason: DismissReason;
    comment?: string;
  }) => {
    if (!id) return;
    setBusy('reject');
    try {
      await api.dismiss(connection, id, { reason, comment });
      setDismissOpen(false);
      onChanged?.();
      nav('/', {
        state: {
          toast: { kind: 'success', text: 'Dismissed — moved to Archive.' },
        },
      });
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
      <Page
        title="Issue review"
        actions={<PageGlobalActions onAskAgent={onAskAgent} />}
        hasPadding
      >
        <Notice.Root intent="error">
          <Notice.Description>
            Failed to load issue: {error} <Link to="/">Back to board</Link>
          </Notice.Description>
        </Notice.Root>
      </Page>
    );
  }
  if (!data) {
    return (
      <Page
        title="Issue review"
        actions={<PageGlobalActions onAskAgent={onAskAgent} />}
        hasPadding
      >
        <Stack direction="row" gap="sm" align="center">
          <Spinner /> <Text variant="body-sm">Loading issue…</Text>
        </Stack>
      </Page>
    );
  }

  const { issue, proposal } = data;
  const kind = kindFromProposalType(proposal?.type, issue);
  const personaKey = personaKeyFrom(issue.persona);
  const personaLabel = personaLabelFrom(issue.persona);
  const reviewable = issue.status === 'in_review';
  const isDone = issue.status === 'done';

  // DismissDialog for the Price/Message early-return branches below. The
  // prose-path render at the bottom has its own DismissDialog because it
  // needs the variants-aware variantCount. Without this, clicking Dismiss
  // on Price/Message views was a no-op.
  const dismissDialog = (
    <DismissDialog
      open={dismissOpen}
      onOpenChange={setDismissOpen}
      persona={data?.issue.persona}
      variantCount={1}
      onConfirm={handleDismissConfirm}
      busy={busy === 'reject'}
    />
  );

  const priceProposal = priceProposalFromProposal(proposal);
  if (priceProposal !== null) {
    return (
      <>
        <PriceIssueView
          issue={issue}
          proposal={priceProposal}
          rationale={proposal?.content ?? ''}
          kind={kind}
          personaKey={personaKey}
          personaLabel={personaLabel}
          actionMsg={actionMsg}
          busy={busy}
          reviewable={reviewable}
          isDone={isDone}
          connection={connection}
          onApprove={onApprove}
          onReject={onReject}
          onCancel={() => nav('/')}
          onUndo={() => nav('/')}
          onView={() => nav('/')}
          onAskAgent={onAskAgent}
        />
        {dismissDialog}
      </>
    );
  }

  const messageProposal = messageProposalFromProposal(proposal);
  if (messageProposal !== null) {
    return (
      <>
        <MessageIssueView
          issue={issue}
          proposal={messageProposal}
          kind={kind}
          personaKey={personaKey}
          personaLabel={personaLabel}
          actionMsg={actionMsg}
          busy={busy}
          reviewable={reviewable}
          isDone={isDone}
          connection={connection}
          onApprove={onApprove}
          onReject={onReject}
          onCancel={() => nav('/')}
          onUndo={() => nav('/')}
          onView={() => nav('/')}
          onAskAgent={onAskAgent}
        />
        {dismissDialog}
      </>
    );
  }

  // ---------- Prose path (Marketing + future content/campaign/email) ----------

  const target = (proposal?.target ?? {}) as Record<string, unknown>;
  const previous = typeof target.previous === 'string' ? target.previous : undefined;
  const productSku = typeof target.sku === 'string' ? target.sku : undefined;
  const productName =
    typeof target.product_name === 'string' ? target.product_name : undefined;
  const productBound = typeof target.product_id === 'number' || !!productSku;
  const scope = productName ?? productSku ?? '—';

  const variants = variantsFromProposal(proposal);
  const activeVariant: Variant | null =
    variants?.find((v) => v.id === selectedVariant) ?? variants?.[0] ?? null;

  return (
    <div className="wa-detail-shell">
      <Page
        title="Issue review"
        actions={<PageGlobalActions onAskAgent={onAskAgent} />}
        hasPadding
        className="wa-detail-shell-page"
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

        {/* Persona eyebrow */}
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
              <Button
                __next40pxDefaultSize
                variant="tertiary"
                icon={rotateRight}
                disabled
              >
                Regenerate
              </Button>
            </Stack>

            {!proposal ? (
              <Notice.Root intent="info">
                <Notice.Description>
                  The agent hasn't produced a proposal for this issue yet.
                </Notice.Description>
              </Notice.Root>
            ) : variants ? (
              variants
                .filter((v) => !isDone || v.id === (approvedVariant ?? selectedVariant))
                .map((v) => {
                  const isSelected = selectedVariant === v.id;
                  // CUSTOM: whole-card variant selector. Same pattern as Kanban card-as-button — (a) no WPDS clickable-card / radio-card component, (b) reset chrome wraps <Card.Root>, (c) Follow-up: see CardLink composite note in Kanban.tsx.
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
                                  fontSize: 'var(--wpds-typography-font-size-sm)',
                                  flex: 'none',
                                  background: isSelected
                                    ? 'var(--wpds-color-bg-interactive-brand-strong)'
                                    : 'transparent',
                                  color: isSelected
                                    ? 'var(--wpds-color-fg-interactive-brand-strong)'
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
                                  background: 'var(--wpds-color-bg-surface-neutral-weak)',
                                  color: 'var(--wpds-color-fg-content-neutral)',
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
                          fontSize: 'var(--wpds-typography-font-size-sm)',
                          fontWeight: 700,
                          padding: '2px 8px',
                          borderRadius: 'var(--wpds-border-radius-sm)',
                          background: 'var(--wpds-color-bg-interactive-brand-strong)',
                          color: 'var(--wpds-color-fg-interactive-brand-strong)',
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
              <Notice.Root
                intent={actionMsg.kind === 'success' ? 'success' : 'error'}
              >
                <Notice.Description>{actionMsg.text}</Notice.Description>
              </Notice.Root>
            )}

            <IssueRuns connection={connection} issueId={issue.id} />
          </div>
        </div>
      </Page>

      {isDone ? (
        <ActionBar
          state="done"
          variantId={approvedVariant ?? activeVariant?.id ?? 'A'}
          scope={scope}
          onUndo={() => nav('/')}
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
      <DismissDialog
        open={dismissOpen}
        onOpenChange={setDismissOpen}
        persona={data?.issue.persona}
        variantCount={variants?.length ?? 1}
        onConfirm={handleDismissConfirm}
        busy={busy === 'reject'}
      />
    </div>
  );
}

// ---------- Price-change view (Pricing persona) ----------

interface PriceViewProps {
  issue: IssueDetailPayload['issue'];
  proposal: PriceProposal;
  rationale: string;
  kind: ReturnType<typeof kindFromProposalType>;
  personaKey: ReturnType<typeof personaKeyFrom>;
  personaLabel: string;
  actionMsg: { kind: 'success' | 'error'; text: string } | null;
  busy: 'approve' | 'reject' | null;
  reviewable: boolean;
  isDone: boolean;
  connection: Connection;
  onApprove: () => void;
  onReject: () => void;
  onCancel: () => void;
  onUndo: () => void;
  onView: () => void;
  onAskAgent: () => void;
}

function PriceIssueView(props: PriceViewProps) {
  const { onAskAgent } = props;
  const { issue, proposal, rationale, kind, personaKey, personaLabel } = props;
  const productBound = typeof proposal.productId === 'number';
  const scope = proposal.productName ?? proposal.productSku ?? '—';
  const currency = proposal.currency;

  const directionTone =
    proposal.direction === 'increase'
      ? {
          fg: 'var(--wpds-color-fg-content-warning)',
          bg: 'var(--wpds-color-bg-surface-warning-weak)',
        }
      : proposal.direction === 'decrease'
        ? {
            fg: 'var(--wpds-color-fg-content-success)',
            bg: 'var(--wpds-color-bg-surface-success-weak)',
          }
        : {
            fg: 'var(--wpds-color-fg-content-neutral)',
            bg: 'var(--wpds-color-bg-surface-neutral-weak)',
          };

  const arrow = proposal.direction === 'decrease' ? '↓' : proposal.direction === 'increase' ? '↑' : '·';
  const percentLabel = `${proposal.percentChange >= 0 ? '+' : ''}${proposal.percentChange.toFixed(1)}%`;
  const deltaLabel = formatDelta(proposal.previousPrice, proposal.proposedPrice, currency);
  const summary = `${formatPrice(proposal.previousPrice, currency)} → ${formatPrice(proposal.proposedPrice, currency)} · ${percentLabel}`;
  const doneSummary = `${formatPrice(proposal.proposedPrice, currency)} (was ${formatPrice(proposal.previousPrice, currency)})`;

  const hasObservedRange =
    typeof proposal.observedLow === 'number' &&
    typeof proposal.observedHigh === 'number' &&
    proposal.observedHigh > proposal.observedLow;

  return (
    <div className="wa-detail-shell">
      <Page
        title="Issue review"
        actions={<PageGlobalActions onAskAgent={onAskAgent} />}
        hasPadding
        className="wa-detail-shell-page"
      >
        {/* Breadcrumb */}
        <Stack
          direction="row"
          gap="sm"
          align="center"
          style={{ marginBottom: 'var(--wpds-dimension-gap-md)' }}
        >
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

        {/* Persona eyebrow */}
        <Stack
          direction="row"
          gap="sm"
          align="center"
          style={{ marginBottom: 'var(--wpds-dimension-gap-sm)' }}
        >
          <PersonaAvatar persona={personaKey} size="md" />
          <Text
            variant="body-sm"
            style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
          >
            <strong style={{ color: 'var(--wpds-color-fg-content-neutral)' }}>
              {personaLabel}
            </strong>{' '}
            proposes a price change · {relativeTime(issue.updated_at)} ·{' '}
            <span className="wa-mono">Claude Haiku 4.5 · web_search</span>
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
            `Benchmarked against ${proposal.sources.length} comparable products. Approval writes regular_price to WooCommerce; the previous price is snapshotted and reversible from the Done column.`}
        </Text>

        {/* KPI row — price tiles */}
        <div
          className="wa-kpi-row"
          style={{ marginBottom: 'var(--wpds-dimension-gap-xl)' }}
        >
          <Kpi
            label="Current price"
            value={formatPrice(proposal.previousPrice, currency)}
            hint={productBound ? `Bound · ${proposal.productSku ?? ''}` : 'No product bound'}
          />
          <Kpi
            label="Proposed price"
            value={formatPrice(proposal.proposedPrice, currency)}
            hint={`${arrow} ${deltaLabel} from current`}
          />
          <Kpi
            label="Change"
            value={
              <span style={{ color: directionTone.fg }}>{percentLabel}</span>
            }
            hint={`Capped at ±25% per step · ${proposal.direction}`}
          />
          <Kpi
            label="Sources"
            value={proposal.sources.length}
            hint="Comparable products cited"
            intent="success"
          />
        </div>

        {/* Body */}
        <div className="wa-detail-body">
          <div className="wa-detail-main">
            {/* Headline price comparison */}
            <Card.Root>
              <Card.Header>
                <span className="wa-eyebrow wa-eyebrow--persona">Price change</span>
              </Card.Header>
              <Card.Content>
                <Stack direction="row" gap="lg" align="center" wrap="wrap">
                  <Stack direction="column" gap="xs">
                    <Text
                      variant="body-sm"
                      style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
                    >
                      Current
                    </Text>
                    <Text
                      variant="heading-xl"
                      style={{
                        textDecoration: 'line-through',
                        color: 'var(--wpds-color-fg-content-neutral-weak)',
                      }}
                    >
                      {formatPrice(proposal.previousPrice, currency)}
                    </Text>
                  </Stack>
                  <Text
                    variant="heading-lg"
                    style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
                  >
                    →
                  </Text>
                  <Stack direction="column" gap="xs">
                    <Text
                      variant="body-sm"
                      style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
                    >
                      Proposed
                    </Text>
                    <Text
                      variant="heading-xl"
                      style={{
                        fontWeight: 'var(--wpds-typography-font-weight-medium)',
                      }}
                    >
                      {formatPrice(proposal.proposedPrice, currency)}
                    </Text>
                  </Stack>
                  <span
                    style={{
                      marginLeft: 'auto',
                      padding: '6px 14px',
                      borderRadius: 'var(--wpds-border-radius-md)',
                      fontWeight: 'var(--wpds-typography-font-weight-medium)',
                      fontSize: 'var(--wpds-typography-font-size-md)',
                      background: directionTone.bg,
                      color: directionTone.fg,
                    }}
                  >
                    {arrow} {deltaLabel} · {percentLabel}
                  </span>
                </Stack>
              </Card.Content>
            </Card.Root>

            {/* Observed market range */}
            {hasObservedRange && (
              <Card.Root>
                <Card.Header>
                  <Stack direction="row" justify="space-between" align="center">
                    <span className="wa-eyebrow">Observed market range</span>
                    <span
                      className="wa-mono"
                      style={{
                        fontSize: 'var(--wpds-typography-font-size-xs)',
                        color: 'var(--wpds-color-fg-content-neutral-weak)',
                      }}
                    >
                      from {proposal.sources.length} comparables
                    </span>
                  </Stack>
                </Card.Header>
                <Card.Content>
                  <ObservedRange
                    low={proposal.observedLow!}
                    median={proposal.observedMedian}
                    high={proposal.observedHigh!}
                    proposed={proposal.proposedPrice}
                    currency={currency}
                  />
                </Card.Content>
              </Card.Root>
            )}

            {/* Rationale */}
            <Card.Root
              className="wa-variant-card wa-variant-card--selected"
              style={{ borderWidth: 'var(--wpds-border-width-md)' }}
            >
              <Card.Header>
                <Stack direction="row" justify="space-between" align="center">
                  <span className="wa-eyebrow wa-eyebrow--persona">Rationale</span>
                  <span
                    className="wa-mono"
                    style={{
                      fontSize: 'var(--wpds-typography-font-size-xs)',
                      color: 'var(--wpds-color-fg-content-neutral-weak)',
                    }}
                  >
                    every numeric claim cited below
                  </span>
                </Stack>
              </Card.Header>
              <Card.Content>
                <Text
                  variant="body-md"
                  style={{ whiteSpace: 'pre-wrap', lineHeight: 1.65 }}
                >
                  {rationale || '— no rationale attached —'}
                </Text>
              </Card.Content>
            </Card.Root>

            {/* Sources */}
            <Card.Root>
              <Card.Header>
                <span className="wa-eyebrow">
                  Sources · {proposal.sources.length} comparable{proposal.sources.length === 1 ? '' : 's'}
                </span>
              </Card.Header>
              <Card.Content>
                {proposal.sources.length === 0 ? (
                  <Notice.Root intent="warning">
                    <Notice.Description>
                      Proposal has no cited sources. The skill requires at least 3
                      — this should not happen and indicates a WooAgent-side bug.
                    </Notice.Description>
                  </Notice.Root>
                ) : (
                  <Stack direction="column" gap="sm">
                    {proposal.sources.map((s, idx) => (
                      <SourceRow
                        key={idx}
                        source={s}
                        currency={currency}
                        proposed={proposal.proposedPrice}
                      />
                    ))}
                  </Stack>
                )}
              </Card.Content>
            </Card.Root>

            {props.actionMsg && (
              <Notice.Root
                intent={props.actionMsg.kind === 'success' ? 'success' : 'error'}
              >
                <Notice.Description>{props.actionMsg.text}</Notice.Description>
              </Notice.Root>
            )}

            <IssueRuns connection={props.connection} issueId={issue.id} />
          </div>
        </div>
      </Page>

      {props.isDone ? (
        <ActionBar
          state="done"
          entity="price"
          priceSummary={doneSummary}
          scope={scope}
          onUndo={props.onUndo}
          onView={props.onView}
        />
      ) : (
        <ActionBar
          state="review"
          entity="price"
          productBound={productBound}
          busy={props.busy}
          disabled={!props.reviewable}
          priceSummary={summary}
          onApprove={props.onApprove}
          onReject={props.onReject}
          onCancel={props.onCancel}
        />
      )}
    </div>
  );
}

interface ObservedRangeProps {
  low: number;
  median?: number;
  high: number;
  proposed: number;
  currency: string;
}

// Simple horizontal bar showing the observed market band with a marker for
// the proposed price. Not a real chart — a token-styled sparkline-equivalent
// that gives the operator instant visual context.
function ObservedRange({ low, median, high, proposed, currency }: ObservedRangeProps) {
  const span = high - low;
  const proposedPct = span > 0 ? ((proposed - low) / span) * 100 : 50;
  const medianPct =
    typeof median === 'number' && span > 0 ? ((median - low) / span) * 100 : null;
  const proposedClamped = Math.max(0, Math.min(100, proposedPct));
  const proposedInBand = proposed >= low && proposed <= high;
  const markerColor = proposedInBand
    ? 'var(--wpds-color-bg-interactive-brand-strong)'
    : 'var(--wpds-color-fg-content-warning)';

  return (
    <Stack direction="column" gap="sm">
      <Stack direction="row" justify="space-between">
        <Text
          variant="body-sm"
          className="wa-mono"
          style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
        >
          low {formatPrice(low, currency)}
        </Text>
        {typeof median === 'number' && (
          <Text
            variant="body-sm"
            className="wa-mono"
            style={{ color: 'var(--wpds-color-fg-content-neutral)' }}
          >
            median {formatPrice(median, currency)}
          </Text>
        )}
        <Text
          variant="body-sm"
          className="wa-mono"
          style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
        >
          high {formatPrice(high, currency)}
        </Text>
      </Stack>
      <div
        style={{
          position: 'relative',
          height: 8,
          borderRadius: 'var(--wpds-border-radius-sm)',
          background: 'var(--wpds-color-bg-surface-neutral-weak)',
          overflow: 'visible',
        }}
        aria-hidden="true"
      >
        {medianPct !== null && (
          <span
            style={{
              position: 'absolute',
              left: `${Math.max(0, Math.min(100, medianPct))}%`,
              top: -2,
              bottom: -2,
              width: 2,
              background: 'var(--wpds-color-stroke-surface-neutral-strong)',
              transform: 'translateX(-1px)',
            }}
          />
        )}
        <span
          style={{
            position: 'absolute',
            left: `${proposedClamped}%`,
            top: -4,
            height: 16,
            width: 16,
            borderRadius: '50%',
            background: markerColor,
            border: 'var(--wpds-border-width-md) solid var(--wpds-color-bg-surface-neutral)',
            boxShadow: 'var(--wpds-elevation-sm)',
            transform: 'translateX(-8px)',
          }}
        />
      </div>
      <Text
        variant="body-sm"
        style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
      >
        {proposedInBand
          ? `Proposed ${formatPrice(proposed, currency)} sits inside the observed band.`
          : `Proposed ${formatPrice(proposed, currency)} is outside the observed band — operator review recommended.`}
      </Text>
    </Stack>
  );
}

interface SourceRowProps {
  source: import('../api/client').PriceSource;
  currency: string;
  proposed: number;
}

function SourceRow({ source, currency, proposed }: SourceRowProps) {
  const sourceCurrency = source.currency ?? currency;
  const diff = source.observed_price - proposed;
  const diffStr =
    Math.abs(diff) < 0.005
      ? 'matches proposed'
      : diff > 0
        ? `${formatPrice(diff, sourceCurrency)} higher`
        : `${formatPrice(-diff, sourceCurrency)} lower`;
  let host = source.url;
  try {
    host = new URL(source.url).hostname.replace(/^www\./, '');
  } catch {
    /* fall through with raw url */
  }
  return (
    <Stack
      direction="row"
      justify="space-between"
      align="center"
      gap="md"
      wrap="wrap"
      style={{
        padding: 'var(--wpds-dimension-padding-md)',
        borderRadius: 'var(--wpds-border-radius-sm)',
        background: 'var(--wpds-color-bg-surface-neutral-weak)',
      }}
    >
      <Stack direction="column" gap="xs" style={{ minWidth: 0, flex: 1 }}>
        <Stack direction="row" gap="sm" align="center" wrap="wrap">
          {source.retailer && (
            <span
              style={{
                fontSize: 'var(--wpds-typography-font-size-xs)',
                fontWeight: 'var(--wpds-typography-font-weight-medium)',
                padding: '2px 8px',
                borderRadius: 'var(--wpds-border-radius-sm)',
                background: 'var(--wpds-color-bg-surface-neutral-weak)',
                color: 'var(--wpds-color-fg-content-neutral)',
              }}
            >
              {source.retailer}
            </span>
          )}
          <Text
            variant="body-sm"
            style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
          >
            {source.comparable_product}
          </Text>
        </Stack>
        <span style={{ fontSize: 'var(--wpds-typography-font-size-xs)' }}>
          <Button
            variant="link"
            href={source.url}
            target="_blank"
            rel="noreferrer noopener"
          >
            {host}
            <Icon
              icon={arrowUpRight}
              size={16}
              style={{
                verticalAlign: 'text-bottom',
                marginInlineStart: 'var(--wpds-dimension-padding-xs)',
              }}
            />
          </Button>
        </span>
        {source.note && (
          <Text
            variant="body-sm"
            style={{
              color: 'var(--wpds-color-fg-content-neutral-weak)',
              fontSize: 'var(--wpds-typography-font-size-xs)',
            }}
          >
            {source.note}
          </Text>
        )}
      </Stack>
      <Stack direction="column" gap="xs" align="end">
        <Text
          variant="body-sm"
          className="wa-mono"
          style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
        >
          {formatPrice(source.observed_price, sourceCurrency)}
        </Text>
        <Text
          variant="body-sm"
          style={{
            color: 'var(--wpds-color-fg-content-neutral-weak)',
            fontSize: 'var(--wpds-typography-font-size-xs)',
          }}
        >
          {diffStr}
        </Text>
      </Stack>
    </Stack>
  );
}

// ---------- Message-draft view (Sales Support persona) ----------

interface MessageViewProps {
  issue: IssueDetailPayload['issue'];
  proposal: MessageProposal;
  kind: ReturnType<typeof kindFromProposalType>;
  personaKey: ReturnType<typeof personaKeyFrom>;
  personaLabel: string;
  actionMsg: { kind: 'success' | 'error'; text: string } | null;
  busy: 'approve' | 'reject' | null;
  reviewable: boolean;
  isDone: boolean;
  connection: Connection;
  onApprove: () => void;
  onReject: () => void;
  onCancel: () => void;
  onUndo: () => void;
  onView: () => void;
  onAskAgent: () => void;
}

function MessageIssueView(props: MessageViewProps) {
  const { onAskAgent } = props;
  const { issue, proposal, kind, personaKey, personaLabel } = props;
  const isInternal = proposal.noteType === 'internal';
  const customerName = proposal.customerName ?? 'the customer';
  const recipientLabel = proposal.customerEmail
    ? `${customerName} · ${proposal.customerEmail}`
    : customerName;
  const orderLabel = proposal.orderNumber
    ? `#${proposal.orderNumber}`
    : `#${proposal.orderId}`;
  const productBound = true; // order_id always present here
  const scope = `Order ${orderLabel} · ${customerName}`;
  const orderTotal =
    proposal.orderTotal && proposal.orderCurrency
      ? `${proposal.orderTotal} ${proposal.orderCurrency}`
      : proposal.orderTotal ?? '—';
  const charCount = proposal.message.length;

  return (
    <div className="wa-detail-shell">
      <Page
        title="Issue review"
        actions={<PageGlobalActions onAskAgent={onAskAgent} />}
        hasPadding
        className="wa-detail-shell-page"
      >
        {/* Breadcrumb */}
        <Stack
          direction="row"
          gap="sm"
          align="center"
          style={{ marginBottom: 'var(--wpds-dimension-gap-md)' }}
        >
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
          <span
            style={{
              fontSize: 'var(--wpds-typography-font-size-xs)',
              padding: '2px 8px',
              borderRadius: 'var(--wpds-border-radius-sm)',
              background: isInternal
                ? 'var(--wpds-color-bg-surface-warning-weak)'
                : 'var(--wpds-color-bg-surface-success-weak)',
              color: isInternal
                ? 'var(--wpds-color-fg-content-warning)'
                : 'var(--wpds-color-fg-content-success)',
            }}
          >
            {isInternal ? 'Internal note' : 'Customer-facing'}
          </span>
        </Stack>

        {/* Persona eyebrow */}
        <Stack
          direction="row"
          gap="sm"
          align="center"
          style={{ marginBottom: 'var(--wpds-dimension-gap-sm)' }}
        >
          <PersonaAvatar persona={personaKey} size="md" />
          <Text
            variant="body-sm"
            style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
          >
            <strong style={{ color: 'var(--wpds-color-fg-content-neutral)' }}>
              {personaLabel}
            </strong>{' '}
            drafted a customer reply · {relativeTime(issue.updated_at)} ·{' '}
            <span className="wa-mono">Claude Haiku 4.5</span>
          </Text>
        </Stack>

        {/* Title + subhead */}
        <Text
          variant="heading-2xl"
          render={
            <h1 style={{ margin: 0, marginBottom: 'var(--wpds-dimension-gap-sm)' }} />
          }
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
            (isInternal
              ? `Internal note for order ${orderLabel}. Saves to wp-admin only — not visible to the customer.`
              : `Customer-facing note for ${customerName}. Approval emails this directly to ${proposal.customerEmail ?? 'the customer'}; reversible from the Done column.`)}
        </Text>

        {/* KPI row */}
        <div
          className="wa-kpi-row"
          style={{ marginBottom: 'var(--wpds-dimension-gap-xl)' }}
        >
          <Kpi label="Recipient" value={customerName} hint={proposal.customerEmail ?? '—'} />
          <Kpi label="Order" value={orderLabel} hint={proposal.orderStatus ?? '—'} />
          <Kpi label="Total" value={orderTotal} hint={proposal.orderDate ?? ''} />
          <Kpi
            label="Note type"
            value={isInternal ? 'Internal' : 'Customer'}
            hint={isInternal ? 'wp-admin only' : 'emailed on approval'}
            intent={isInternal ? 'neutral' : 'success'}
          />
        </div>

        {/* Body */}
        <div className="wa-detail-body">
          <div className="wa-detail-main">
            {/* Order summary */}
            <Card.Root>
              <Card.Header>
                <Stack direction="row" justify="space-between" align="center">
                  <span className="wa-eyebrow">Order context</span>
                  <span
                    className="wa-mono"
                    style={{
                      fontSize: 'var(--wpds-typography-font-size-xs)',
                      color: 'var(--wpds-color-fg-content-neutral-weak)',
                    }}
                  >
                    {proposal.orderStatus ?? '—'}
                  </span>
                </Stack>
              </Card.Header>
              <Card.Content>
                <Stack direction="column" gap="sm">
                  <Stack direction="row" gap="md" wrap="wrap">
                    <Stack direction="column" gap="xs">
                      <span className="wa-eyebrow">Customer</span>
                      <Text variant="body-sm">
                        {customerName}
                        {proposal.customerEmail ? ` · ${proposal.customerEmail}` : ''}
                      </Text>
                    </Stack>
                    <Stack direction="column" gap="xs">
                      <span className="wa-eyebrow">Placed</span>
                      <Text
                        variant="body-sm"
                        className="wa-mono"
                        style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
                      >
                        {proposal.orderDate ?? '—'}
                      </Text>
                    </Stack>
                  </Stack>
                  {proposal.lineItems.length > 0 && (
                    <Stack direction="column" gap="xs">
                      <span className="wa-eyebrow">Line items</span>
                      <ul
                        style={{
                          margin: 0,
                          paddingLeft: 'var(--wpds-dimension-padding-md)',
                          color: 'var(--wpds-color-fg-content-neutral)',
                        }}
                      >
                        {proposal.lineItems.map((li, idx) => (
                          <li key={idx}>
                            <Text variant="body-sm">
                              {li.name}
                              {typeof li.quantity === 'number' ? ` × ${li.quantity}` : ''}
                              {li.sku ? ` · ${li.sku}` : ''}
                              {li.total ? ` · ${li.total}` : ''}
                            </Text>
                          </li>
                        ))}
                      </ul>
                    </Stack>
                  )}
                </Stack>
              </Card.Content>
            </Card.Root>

            {/* Drafted message — preview-shaped so the operator reads it as the customer would */}
            <Card.Root
              className="wa-variant-card wa-variant-card--selected"
              style={{ borderWidth: 'var(--wpds-border-width-md)' }}
            >
              <Card.Header>
                <Stack direction="row" justify="space-between" align="center">
                  <Stack direction="row" gap="sm" align="center">
                    <span className="wa-eyebrow wa-eyebrow--persona">
                      {isInternal ? 'Internal note' : 'Customer-facing message'}
                    </span>
                    {proposal.subjectHint && (
                      <span
                        style={{
                          fontSize: 'var(--wpds-typography-font-size-xs)',
                          padding: '2px 8px',
                          borderRadius: 'var(--wpds-border-radius-sm)',
                          background: 'var(--wpds-color-bg-surface-neutral-weak)',
                          color: 'var(--wpds-color-fg-content-neutral)',
                        }}
                      >
                        {proposal.subjectHint}
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
                    {charCount} chars
                  </span>
                </Stack>
              </Card.Header>
              <Card.Content>
                {!isInternal && (
                  <div
                    style={{
                      paddingBottom: 'var(--wpds-dimension-padding-md)',
                      marginBottom: 'var(--wpds-dimension-gap-md)',
                      borderBottom:
                        'var(--wpds-border-width-sm) solid var(--wpds-color-stroke-surface-neutral-weak)',
                      fontSize: 'var(--wpds-typography-font-size-xs)',
                      color: 'var(--wpds-color-fg-content-neutral-weak)',
                    }}
                  >
                    To: {proposal.customerEmail ?? '(no email on file)'} · Order {orderLabel}
                  </div>
                )}
                <Text
                  variant="body-md"
                  style={{
                    whiteSpace: 'pre-wrap',
                    lineHeight: 1.65,
                    fontFamily: 'var(--wpds-typography-font-family-body)',
                  }}
                >
                  {proposal.message}
                </Text>
              </Card.Content>
            </Card.Root>

            {props.actionMsg && (
              <Notice.Root
                intent={props.actionMsg.kind === 'success' ? 'success' : 'error'}
              >
                <Notice.Description>{props.actionMsg.text}</Notice.Description>
              </Notice.Root>
            )}

            <IssueRuns connection={props.connection} issueId={issue.id} />
          </div>
        </div>
      </Page>

      {props.isDone ? (
        <ActionBar
          state="done"
          entity="message"
          messageRecipient={recipientLabel}
          messageNoteType={proposal.noteType}
          scope={scope}
          onUndo={props.onUndo}
          onView={props.onView}
        />
      ) : (
        <ActionBar
          state="review"
          entity="message"
          productBound={productBound}
          busy={props.busy}
          disabled={!props.reviewable}
          messageRecipient={recipientLabel}
          messageNoteType={proposal.noteType}
          onApprove={props.onApprove}
          onReject={props.onReject}
          onCancel={props.onCancel}
        />
      )}
    </div>
  );
}

// Collapsed runs card shown in the issue detail sidebar area. Loads runs
// matching the issue_id and renders them newest-first. Hidden entirely when
// there are no runs yet — don't dominate the layout with an empty section.
function IssueRuns({
  connection,
  issueId,
}: {
  connection: Connection;
  issueId: string;
}) {
  const [runs, setRuns] = useState<Run[]>([]);

  useEffect(() => {
    let cancelled = false;
    api.runs
      .list(connection, { issueId })
      .then((res) => {
        if (!cancelled) setRuns(res.runs);
      })
      .catch(() => {
        // Best-effort — don't surface a blocking error in the detail view.
      });
    return () => {
      cancelled = true;
    };
  }, [connection, issueId]);

  if (runs.length === 0) return null;

  return (
    <CollapsibleCard.Root>
      <CollapsibleCard.Header>
        <Card.Title>Runs ({runs.length})</Card.Title>
      </CollapsibleCard.Header>
      <CollapsibleCard.Content>
        <Stack direction="column" gap="sm">
          {runs.map((r) => (
            <Stack key={r.id} direction="row" gap="md" align="center">
              <RunStatusBadge status={r.status} />
              <Text
                variant="body-sm"
                style={{
                  color: 'var(--wpds-color-fg-content-neutral-weak)',
                  fontSize: 'var(--wpds-typography-font-size-xs)',
                }}
              >
                {r.scheduled_at
                  ? new Date(r.scheduled_at).toLocaleString()
                  : '—'}
              </Text>
              <Link
                to={`/runs/${r.id}`}
                style={{
                  fontSize: 'var(--wpds-typography-font-size-xs)',
                  color: 'var(--wpds-color-fg-content-neutral-weak)',
                }}
              >
                {r.id.slice(0, 8).toUpperCase()}
              </Link>
            </Stack>
          ))}
        </Stack>
      </CollapsibleCard.Content>
    </CollapsibleCard.Root>
  );
}

function personaLabelFrom(persona: string | undefined): string {
  switch (persona) {
    case 'marketing':
      return 'Marketing agent';
    case 'pricing':
      return 'Pricing agent';
    case 'sales-support':
      return 'Sales support agent';
    case 'inventory':
      return 'Inventory agent';
    case 'accounting':
      return 'Accounting agent';
    case 'reporting':
      return 'Reporting agent';
    case 'chief-of-staff':
      return 'Chief of staff';
    default:
      return `${persona ?? 'unassigned'} agent`;
  }
}
