import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Badge, Card, CollapsibleCard, Notice, Stack, Text } from '@wordpress/ui';
import { Spinner, VisuallyHidden } from '@wordpress/components';
import { Page } from '@wordpress/admin-ui';
import {
  ApiError,
  api,
  isReversibleProposalType,
  messageProposalFromProposal,
  priceProposalFromProposal,
  variantsFromProposal,
  type Connection,
  type DismissReason,
  type IssueDetail as IssueDetailPayload,
  type MessageProposal,
  type PriceProposal,
  type Proposal,
  type Variant,
} from '../api/client';
import { StatusBadge } from '../components/StatusBadge';
import { useAskAgentContext } from '../lib/askAgent';
import { issueToVisible } from '../lib/visibleItems';
import Kpi from '../components/Kpi';
import type { KpiTone } from '../components/Kpi';
import ActionBar from '../components/ActionBar';
import DismissDialog from '../components/DismissDialog';
import PageGlobalActions from '../components/PageGlobalActions';
import Breadcrumbs from '../components/Breadcrumbs';
import SectionHeader from '../components/SectionHeader';
import SourceRow from '../components/SourceRow';
import ProposalHeader from '../components/ProposalHeader';

interface Props {
  connection: Connection;
  onChanged?: () => void;
  onAskAgent: () => void;
}

// Choose the score-value color class based on the score band. SEO and Voice
// use slightly different bands (SEO 80+, Voice 80+ for "good") to match the
// Yoast/voice-model conventions and the corpus-based voice scoring design.
function seoColorClass(score: number): string {
  if (score >= 80) return 'wa-score-label__value--good';
  if (score >= 70) return 'wa-score-label__value--caution';
  return 'wa-score-label__value--warning';
}
function voiceColorClass(score: number): string {
  if (score >= 80) return 'wa-score-label__value--good';
  if (score >= 65) return 'wa-score-label__value--caution';
  return 'wa-score-label__value--warning';
}

// Score → Kpi tone. SEO uses success/caution/warning.
function seoToneBand(score: number): KpiTone {
  if (score >= 80) return 'success';
  if (score >= 70) return 'caution';
  return 'warning';
}

// Voice tone band — brand (matches the Figma frame's blue for high-match
// voice) / caution / warning. Thresholds relaxed from 90/75 → 80/65 — 90%
// against a small sample is unrealistic.
function voiceToneBand(score: number): KpiTone {
  if (score >= 80) return 'brand';
  if (score >= 65) return 'caution';
  return 'warning';
}

const CURRENCY_SYMBOL: Record<string, string> = {
  USD: '$',
  CAD: '$',
  AUD: '$',
  GBP: '£',
  EUR: '€',
};

export function formatPrice(amount: number, currency: string): string {
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
  const [busy, setBusy] = useState<'approve' | 'reject' | 'undo' | null>(null);
  const [undoStale, setUndoStale] = useState<{ current: string } | null>(null);
  const [selectedVariant, setSelectedVariant] = useState<string | null>(null);
  const [actionMsg, setActionMsg] = useState<{
    kind: 'success' | 'error';
    text: string;
  } | null>(null);
  const [approvedVariant, setApprovedVariant] = useState<string | null>(null);
  const [dismissOpen, setDismissOpen] = useState(false);

  useAskAgentContext(
    () => ({
      page: 'proposal-detail',
      visible_items: data?.issue ? [issueToVisible(data.issue)] : [],
    }),
    [data],
  );

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

  const handleUndo = async () => {
    if (!id) return;
    setBusy('undo');
    setUndoStale(null);
    try {
      await api.undo(connection, id);
      // Re-fetch the issue so undone_at is reflected in the DoneBar.
      const updated = await api.issue(connection, id);
      setData(updated);
      onChanged?.();
    } catch (err) {
      if (err instanceof ApiError && err.code === 'undo_stale') {
        const rawCurrent =
          typeof err.payload?.current === 'string' ? err.payload.current : '';
        // Format prices with currency; truncate long description strings so
        // the Notice doesn't render raw HTML walls of text.
        let displayCurrent = rawCurrent;
        if (rawCurrent) {
          const proposalType = data?.proposal?.type;
          if (proposalType === 'product_price_change') {
            const parsed = parseFloat(rawCurrent);
            if (Number.isFinite(parsed)) {
              const priceProposal = priceProposalFromProposal(data?.proposal);
              const currency = priceProposal?.currency ?? 'USD';
              displayCurrent = formatPrice(parsed, currency);
            }
          } else if (proposalType === 'product_description_rewrite') {
            displayCurrent = rawCurrent.length > 80 ? rawCurrent.slice(0, 80) + '…' : rawCurrent;
          }
        }
        setUndoStale({ current: displayCurrent });
      } else {
        setActionMsg({
          kind: 'error',
          text: err instanceof Error ? err.message : 'Undo failed',
        });
      }
    } finally {
      setBusy(null);
    }
  };

  const idLabel = id ? id.slice(0, 8).toUpperCase() : 'Issue';

  if (error) {
    return (
      <Page
        breadcrumbs={
          <Breadcrumbs items={[{ label: 'Board', to: '/' }, { label: idLabel }]} />
        }
        actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
        hasPadding
      >
        <div className="wa-subpage-content">
          <Notice.Root intent="error">
            <Notice.Description>
              Failed to load issue: {error} <Link to="/">Back to board</Link>
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
          <Breadcrumbs items={[{ label: 'Board', to: '/' }, { label: idLabel }]} />
        }
        actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
        hasPadding
      >
        <div className="wa-subpage-content">
          <Stack direction="row" gap="sm" align="center">
            <Spinner /> <Text variant="body-sm">Loading issue…</Text>
          </Stack>
        </div>
      </Page>
    );
  }

  const { issue, proposal } = data;
  const personaLabel = personaLabelFrom(issue.persona);
  const reviewable = issue.status === 'in_review';
  const isDone = issue.status === 'done';
  const isArchived = issue.status === 'dismissed' || issue.status === 'rejected';

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
          rawProposal={proposal}
          rationale={proposal?.content ?? ''}
          personaLabel={personaLabel}
          actionMsg={actionMsg}
          undoStale={undoStale}
          busy={busy}
          reviewable={reviewable}
          isDone={isDone}
          isArchived={isArchived}
          connection={connection}
          onApprove={onApprove}
          onReject={onReject}
          onCancel={() => nav('/')}
          onUndo={handleUndo}
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
          personaLabel={personaLabel}
          actionMsg={actionMsg}
          busy={busy}
          reviewable={reviewable}
          isDone={isDone}
          isArchived={isArchived}
          connection={connection}
          onApprove={onApprove}
          onReject={onReject}
          onCancel={() => nav('/')}
          onUndo={handleUndo}
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
        breadcrumbs={
          <Breadcrumbs
            items={[
              { label: 'Board', to: '/' },
              { label: issue.id.slice(0, 8).toUpperCase() },
            ]}
          />
        }
        badges={<StatusBadge status={issue.status} />}
        actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
        hasPadding
        className="wa-detail-shell-page"
      >
        <div className="wa-subpage-content">
        <ProposalHeader
          persona={data.issue.persona ?? ''}
          personaLabel={personaLabel}
          verb="proposes content"
          timestamp={issue.updated_at}
          modelLine="Claude Sonnet 4.6"
          title={issue.title}
          description={
            issue.description ??
            'Three voice variants. Pick one, approve, and the agent writes it straight to WooCommerce.'
          }
          imageUrl={typeof data.proposal?.target?.image_url === 'string' ? data.proposal.target.image_url : undefined}
          imageAlt={typeof data.proposal?.target?.image_alt === 'string' ? data.proposal.target.image_alt : undefined}
        />

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
            value={
              typeof activeVariant?.voice === 'number'
                ? `${activeVariant.voice}%`
                : undefined
            }
            tone={
              typeof activeVariant?.voice === 'number'
                ? voiceToneBand(activeVariant.voice)
                : 'neutral'
            }
            hint={
              typeof activeVariant?.voice === 'number'
                ? 'vs. your existing copy'
                : 'Not yet scored'
            }
          />
          <Kpi
            label="SEO score"
            value={
              typeof activeVariant?.seo === 'number'
                ? String(activeVariant.seo)
                : undefined
            }
            tone={
              typeof activeVariant?.seo === 'number'
                ? seoToneBand(activeVariant.seo)
                : 'neutral'
            }
            hint={
              typeof activeVariant?.seo === 'number'
                ? 'Product-copy rubric · out of 100'
                : 'Not yet scored'
            }
          />
          <Kpi label="Est. impact" value="+14% CTR" hint="on product listing pages" tone="success" />
        </div>

        {/* Body */}
        <div className="wa-detail-body">
          <div className="wa-detail-main">
            {/* Current description */}
            <Card.Root>
              <Card.Header>
                <Stack
                  direction="row"
                  gap="md"
                  align="center"
                  style={{ width: '100%' }}
                >
                  <Text
                    variant="body-md"
                    style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
                  >
                    Current description
                  </Text>
                  <Text
                    variant="body-sm"
                    style={{
                      marginLeft: 'auto',
                      color: 'var(--wpds-color-fg-content-neutral-weak)',
                    }}
                  >
                    {previous ? `${previous.length} chars` : '0 chars · sample'}
                  </Text>
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

            {/* Proposed section. Negative margins shave the surrounding
                gap-xl (24px) down to 16px on top and bottom so the eyebrow
                sits tighter between cards. */}
            <div
              style={{
                marginTop: 'calc(-1 * var(--wpds-dimension-gap-sm))',
                marginBottom: 'calc(-1 * var(--wpds-dimension-gap-sm))',
              }}
            >
              <SectionHeader title="Proposed · pick one" />
            </div>


            {!proposal ? (
              <Notice.Root intent="info">
                <Notice.Description>
                  The agent hasn't produced a proposal for this issue yet.
                </Notice.Description>
              </Notice.Root>
            ) : variants ? (
              // CUSTOM: card-as-radio variant selector. WPDS has no
              // RadioCard primitive (RadioControl renders flat options).
              // (a) The whole list is a `role="radiogroup"` so screen readers
              // announce it as a radio group and arrow keys traverse it;
              // (b) each card wraps a visually-hidden native `<input
              // type="radio">` so focus, keyboard activation, and form
              // semantics are real; (c) the visual `wa-radio-mark` is a
              // decoration only (`aria-hidden`). Follow-up: see CardLink
              // composite note in Kanban.tsx.
              <div
                role="radiogroup"
                aria-label="Variant selection"
                style={{
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 'var(--wpds-dimension-gap-xl)',
                }}
              >
              {variants
                .filter((v) => !isDone || v.id === (approvedVariant ?? selectedVariant))
                .map((v) => {
                  const isSelected = selectedVariant === v.id;
                  return (
                    <label
                      key={v.id}
                      style={{
                        display: 'block',
                        width: '100%',
                        minWidth: 0,
                        cursor: isDone ? 'default' : 'var(--wpds-cursor-control)',
                      }}
                    >
                      <VisuallyHidden as="span">
                        <input
                          type="radio"
                          name="marketing-variant"
                          value={v.id}
                          checked={isSelected}
                          disabled={isDone}
                          onChange={() => setSelectedVariant(v.id)}
                        />
                      </VisuallyHidden>
                      <Card.Root
                        className={`wa-variant-card${
                          isSelected ? ' wa-variant-card--selected' : ''
                        }`}
                      >
                        <Card.Content>
                          <div className="wa-variant-header">
                            <div className="wa-variant-header__left">
                              {(() => {
                                // v.label is overloaded: in LLM output it's just
                                // the letter "A"/"B"/"C"; in seed/demo data it's
                                // a longer voice descriptor ("Warm · sincere").
                                // Derive a single letter for the circle from the
                                // variant id (`var_a` → A) so the circle stays
                                // consistent across data sources, and surface the
                                // descriptor as text alongside the circle when
                                // the label is more than one character.
                                const letter = (v.id.split('_').pop() ?? v.label ?? '').slice(0, 1).toUpperCase();
                                // Prefer a multi-char `label` (seed/demo data
                                // encodes the descriptor here as "Warm ·
                                // sincere"). Fall back to the LLM-emitted
                                // `angle` ("material" / "use" / "story"),
                                // title-cased, so the variant cards always
                                // have a descriptor to differentiate them.
                                const titleCase = (s: string) =>
                                  s.charAt(0).toUpperCase() + s.slice(1);
                                const descriptor =
                                  v.label && v.label.length > 1
                                    ? v.label
                                    : v.angle
                                      ? titleCase(v.angle)
                                      : null;
                                return (
                                  <>
                                    <span
                                      aria-hidden="true"
                                      className="wa-variant-letter"
                                      data-tone={letter}
                                    >
                                      {letter}
                                    </span>
                                    {v.recommended ? (
                                      <Text
                                        variant="body-sm"
                                        style={{
                                          color: 'var(--wpds-color-fg-interactive-brand)',
                                          fontWeight: 'var(--wpds-typography-font-weight-medium)',
                                        }}
                                      >
                                        Agent pick{descriptor ? ` · ${descriptor}` : ''}
                                      </Text>
                                    ) : (
                                      descriptor && (
                                        <Text
                                          variant="body-sm"
                                          style={{ color: 'var(--wpds-color-fg-content-neutral)' }}
                                        >
                                          {descriptor}
                                        </Text>
                                      )
                                    )}
                                  </>
                                );
                              })()}
                            </div>
                            <div className="wa-variant-header__right">
                              {typeof v.seo === 'number' && (
                                <span className="wa-score-label">
                                  SEO{' '}
                                  <span className={`wa-score-label__value ${seoColorClass(v.seo)}`}>
                                    {v.seo}
                                  </span>
                                </span>
                              )}
                              {typeof v.voice === 'number' && (
                                <span className="wa-score-label">
                                  Voice{' '}
                                  <span
                                    className={`wa-score-label__value ${voiceColorClass(v.voice)}`}
                                  >
                                    {v.voice}%
                                  </span>
                                </span>
                              )}
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
                              />
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
                    </label>
                  );
                })}
              </div>
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

            {undoStale && (
              <Notice.Root intent="warning">
                <Notice.Description>
                  The product was changed after this approval.{' '}
                  {undoStale.current ? `Current value is ${undoStale.current}. ` : ''}
                  Inspect it in WooCommerce.
                </Notice.Description>
              </Notice.Root>
            )}

          </div>
        </div>
        </div>
      </Page>

      {isArchived ? (
        <ActionBar state="archived" dismissedAt={issue.dismissed_at} />
      ) : isDone ? (
        <ActionBar
          state="done"
          variantId={approvedVariant ?? activeVariant?.id ?? 'A'}
          scope={scope}
          undoneAt={issue.undone_at ?? undefined}
          busy={busy === 'undo' ? 'undo' : null}
          onUndo={handleUndo}
          onView={() => nav('/')}
        />
      ) : (
        <ActionBar
          state="review"
          productBound={productBound}
          busy={busy as 'approve' | 'reject' | null}
          disabled={!reviewable || !proposal}
          selectedVariantId={activeVariant?.id ?? null}
          reversible={isReversibleProposalType(proposal?.type)}
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
  rawProposal?: Proposal | null;
  rationale: string;
  personaLabel: string;
  actionMsg: { kind: 'success' | 'error'; text: string } | null;
  undoStale?: { current: string } | null;
  busy: 'approve' | 'reject' | 'undo' | null;
  reviewable: boolean;
  isDone: boolean;
  isArchived: boolean;
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
  const { issue, proposal, rawProposal, rationale, personaLabel } = props;
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
        breadcrumbs={
          <Breadcrumbs
            items={[
              { label: 'Board', to: '/' },
              { label: issue.id.slice(0, 8).toUpperCase() },
            ]}
          />
        }
        badges={<StatusBadge status={issue.status} />}
        actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
        hasPadding
        className="wa-detail-shell-page"
      >
        <div className="wa-subpage-content">
        <ProposalHeader
          persona="pricing"
          personaLabel={personaLabel}
          verb="proposes a price change"
          timestamp={issue.updated_at}
          modelLine="Claude Haiku 4.5 · web_search"
          title={issue.title}
          description={
            issue.description ??
            `Benchmarked against ${proposal.sources.length} comparable products. Approval writes regular_price to WooCommerce.`
          }
          imageUrl={typeof rawProposal?.target?.image_url === 'string' ? rawProposal.target.image_url : undefined}
          imageAlt={typeof rawProposal?.target?.image_alt === 'string' ? rawProposal.target.image_alt : undefined}
        />

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
            tone="success"
          />
        </div>

        {/* Body */}
        <div className="wa-detail-body">
          <div className="wa-detail-main">
            {/* Headline price comparison */}
            <Card.Root>
              <Card.Header>
                <Text
                  variant="body-md"
                  style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
                >
                  Price change
                </Text>
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
                  <span style={{ marginLeft: 'auto' }}>
                    <Badge intent="none">
                      {`${arrow} ${deltaLabel} · ${percentLabel}`}
                    </Badge>
                  </span>
                </Stack>
              </Card.Content>
            </Card.Root>

            {/* Observed market range */}
            {hasObservedRange && (
              <Card.Root>
                <Card.Header>
                  <Stack
                    direction="row"
                    gap="md"
                    align="center"
                    style={{ width: '100%' }}
                  >
                    <Text
                      variant="body-md"
                      style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
                    >
                      Observed market range
                    </Text>
                    <Text
                      variant="body-sm"
                      className="wa-tabular"
                      style={{
                        marginLeft: 'auto',
                        color: 'var(--wpds-color-fg-content-neutral-weak)',
                      }}
                    >
                      from {proposal.sources.length} comparables
                    </Text>
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
            <Card.Root>
              <Card.Header>
                <Stack
                  direction="row"
                  gap="md"
                  align="center"
                  style={{ width: '100%' }}
                >
                  <Text
                    variant="body-md"
                    style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
                  >
                    Rationale
                  </Text>
                  <Text
                    variant="body-sm"
                    style={{
                      marginLeft: 'auto',
                      color: 'var(--wpds-color-fg-content-neutral-weak)',
                    }}
                  >
                    every numeric claim cited below
                  </Text>
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
                <Stack direction="row" gap="md" align="center">
                  <Text
                    variant="body-md"
                    style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
                  >
                    Sources
                  </Text>
                  <Badge intent="none">
                    {`${proposal.sources.length} comparable${proposal.sources.length === 1 ? '' : 's'}`}
                  </Badge>
                </Stack>
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

            {props.undoStale && (
              <Notice.Root intent="warning">
                <Notice.Description>
                  The product was changed after this approval.{' '}
                  {props.undoStale.current
                    ? `Current price is ${props.undoStale.current}. `
                    : ''}
                  Inspect it in WooCommerce.
                </Notice.Description>
              </Notice.Root>
            )}

          </div>
        </div>
        </div>
      </Page>

      {props.isArchived ? (
        <ActionBar state="archived" dismissedAt={props.issue.dismissed_at} />
      ) : props.isDone ? (
        <ActionBar
          state="done"
          entity="price"
          priceSummary={
            props.issue.undone_at
              ? formatPrice(proposal.previousPrice, currency)
              : doneSummary
          }
          scope={scope}
          undoneAt={props.issue.undone_at ?? undefined}
          busy={props.busy === 'undo' ? 'undo' : null}
          onUndo={props.onUndo}
          onView={props.onView}
        />
      ) : (
        <ActionBar
          state="review"
          entity="price"
          productBound={productBound}
          busy={props.busy as 'approve' | 'reject' | null}
          disabled={!props.reviewable}
          priceSummary={summary}
          reversible
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

// Single label/value row inside the Order context card's meta block.
// Labels are a fixed 80px column to match the 742:1331 Figma rhythm.
function OrderMetaRow({ label, value }: { label: string; value: string }) {
  return (
    <Stack direction="row" gap="sm" align="start">
      <span
        className="wa-eyebrow"
        style={{ width: 80, flex: 'none', paddingTop: 2 }}
      >
        {label}
      </span>
      <Text variant="body-sm" style={{ flex: 1, minWidth: 0 }}>
        {value}
      </Text>
    </Stack>
  );
}

// ---------- Message-draft view (Sales Support persona) ----------

interface MessageViewProps {
  issue: IssueDetailPayload['issue'];
  proposal: MessageProposal;
  personaLabel: string;
  actionMsg: { kind: 'success' | 'error'; text: string } | null;
  busy: 'approve' | 'reject' | 'undo' | null;
  reviewable: boolean;
  isDone: boolean;
  isArchived: boolean;
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
  const { issue, proposal, personaLabel } = props;
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
        breadcrumbs={
          <Breadcrumbs
            items={[
              { label: 'Board', to: '/' },
              { label: issue.id.slice(0, 8).toUpperCase() },
            ]}
          />
        }
        badges={<StatusBadge status={issue.status} />}
        actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
        hasPadding
        className="wa-detail-shell-page"
      >
        <div className="wa-subpage-content">
        <ProposalHeader
          persona="sales-support"
          personaLabel={personaLabel}
          verb={isInternal ? 'drafted an internal note' : 'drafted a customer reply'}
          timestamp={issue.updated_at}
          modelLine="Claude Haiku 4.5"
          title={issue.title}
          description={
            issue.description ??
            (isInternal
              ? `Internal note for order ${orderLabel}. Saves to wp-admin only — not visible to the customer.`
              : `Customer-facing note for ${customerName}. Approval emails this directly to ${proposal.customerEmail ?? 'the customer'}.`)
          }
        />

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
            tone={isInternal ? 'neutral' : 'success'}
          />
        </div>

        {/* Body */}
        <div className="wa-detail-body">
          <div className="wa-detail-main">
            {/* Order context — collapsible. Header: label + items/total summary
                pill + chevron. Expanded body: Customer/Placed/Status meta + a
                full-bleed list of line items separated by 1px hairlines.
                Matches the 742:1330/742:1331 Figma pair. */}
            <CollapsibleCard.Root>
              <CollapsibleCard.Header>
                <Stack direction="row" gap="md" align="center">
                  <Text
                    variant="body-md"
                    style={{
                      fontWeight: 'var(--wpds-typography-font-weight-medium)',
                    }}
                  >
                    Order context
                  </Text>
                  <Badge intent="none">
                    {`${proposal.lineItems.length} ${
                      proposal.lineItems.length === 1 ? 'item' : 'items'
                    }${orderTotal !== '—' ? ` · ${orderTotal}` : ''}`}
                  </Badge>
                </Stack>
              </CollapsibleCard.Header>
              <CollapsibleCard.Content className="wa-order-context-content">
                <Stack direction="column" gap="xs">
                  <OrderMetaRow
                    label="Customer"
                    value={
                      customerName +
                      (proposal.customerEmail
                        ? ` · ${proposal.customerEmail}`
                        : '')
                    }
                  />
                  <OrderMetaRow
                    label="Placed"
                    value={proposal.orderDate ?? '—'}
                  />
                  {proposal.orderStatus && (
                    <OrderMetaRow label="Status" value={proposal.orderStatus} />
                  )}
                </Stack>
                {proposal.lineItems.length > 0 && (
                  <Card.FullBleed
                    style={{
                      marginTop: 'var(--wpds-dimension-gap-xl)',
                    }}
                  >
                    {proposal.lineItems.map((li, idx) => (
                      <div key={idx} className="wa-order-line">
                        <Text variant="body-sm" style={{ flex: 1, minWidth: 0 }}>
                          {li.name}
                          {li.sku ? ` · ${li.sku}` : ''}
                        </Text>
                        {typeof li.quantity === 'number' && (
                          <Text
                            variant="body-sm"
                            style={{
                              color: 'var(--wpds-color-fg-content-neutral-weak)',
                            }}
                          >
                            × {li.quantity}
                          </Text>
                        )}
                        {li.total && (
                          <Text
                            variant="body-md"
                            style={{
                              fontWeight:
                                'var(--wpds-typography-font-weight-medium)',
                              minWidth: 72,
                              textAlign: 'right',
                            }}
                          >
                            {li.total}
                          </Text>
                        )}
                      </div>
                    ))}
                  </Card.FullBleed>
                )}
              </CollapsibleCard.Content>
            </CollapsibleCard.Root>

            {/* Drafted message — same surface treatment as the Order context
                card above (Card.Root with the Header/Content rhythm) but not
                collapsible. Header carries the message label, optional subject
                pill, and a char-count meta. Body is To/Subject meta rows
                (customer flow only) followed by the message body in body-md.
                Matches 815:33529 in Figma. The brand-blue
                `wa-variant-card--selected` outline marks this as the
                approved/active draft. */}
            <Card.Root className="wa-variant-card--selected">
              <Card.Header>
                <Stack
                  direction="row"
                  gap="md"
                  align="center"
                  style={{ width: '100%' }}
                >
                  <Stack direction="row" gap="md" align="center">
                    <Text
                      variant="body-md"
                      style={{
                        fontWeight: 'var(--wpds-typography-font-weight-medium)',
                      }}
                    >
                      {isInternal ? 'Internal note' : 'Customer-facing message'}
                    </Text>
                    {!isInternal && proposal.subjectHint && (
                      <Badge intent="none">{proposal.subjectHint}</Badge>
                    )}
                  </Stack>
                  <Text
                    variant="body-sm"
                    style={{
                      marginLeft: 'auto',
                      color: 'var(--wpds-color-fg-content-neutral-weak)',
                    }}
                  >
                    {charCount} chars
                  </Text>
                </Stack>
              </Card.Header>
              <Card.Content>
                {!isInternal && (
                  <Stack
                    direction="column"
                    gap="xs"
                    style={{ marginBottom: 'var(--wpds-dimension-gap-xl)' }}
                  >
                    <OrderMetaRow
                      label="To"
                      value={`${proposal.customerEmail ?? '(no email on file)'} · Order ${orderLabel}`}
                    />
                    {proposal.subjectHint && (
                      <OrderMetaRow label="Subject" value={proposal.subjectHint} />
                    )}
                  </Stack>
                )}
                <Text
                  variant="body-md"
                  style={{
                    whiteSpace: 'pre-wrap',
                    lineHeight: 1.65,
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

          </div>
        </div>
        </div>
      </Page>

      {props.isArchived ? (
        <ActionBar state="archived" dismissedAt={props.issue.dismissed_at} />
      ) : props.isDone ? (
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
          busy={props.busy as 'approve' | 'reject' | null}
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
