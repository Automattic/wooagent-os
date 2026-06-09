import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { Badge, Card, Notice, Stack, Text } from '@wordpress/ui';
import { Spinner, Button } from '@wordpress/components';
import { Icon, chevronDown, chevronUp, check } from '@wordpress/icons';
import { Page } from '@wordpress/admin-ui';
import {
  ApiError,
  api,
  variantsFromProposal,
  type BatchApproveChild,
  type BatchDetail,
  type Connection,
  type DismissReason,
  type PriceSource,
} from '../api/client';
import Kpi from '../components/Kpi';
import PageGlobalActions from '../components/PageGlobalActions';
import Breadcrumbs from '../components/Breadcrumbs';
import BatchProductCard, { type BatchProduct } from '../components/BatchProductCard';
import SourceRow from '../components/SourceRow';
import { useAskAgentContext } from '../lib/askAgent';
import { issueToVisible } from '../lib/visibleItems';
import DismissDialog from '../components/DismissDialog';
import ProductThumbnail from '../components/ProductThumbnail';
import ProposalHeader from '../components/ProposalHeader';
import ActionSnackbar from '../components/ActionSnackbar';
import { personaLabel as personaLabelFor } from '../lib/personaLabel';
import { batchApproveText, batchDismissText, type ProposalKind } from '../lib/snackbarCopy';
import { useActionBarHeightVar } from '../lib/useActionBarHeight';

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

export default function BatchReview({ connection, onChanged, onAskAgent }: Props) {
  const { id } = useParams<{ id: string }>();
  const nav = useNavigate();
  const [data, setData] = useState<BatchDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [selectedVariants, setSelectedVariants] = useState<Record<string, string>>({});
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [busy, setBusy] = useState<'approve-all' | 'reject-all' | string | null>(null);
  const [actionMsg, setActionMsg] = useState<{
    kind: 'success' | 'error';
    text: string;
  } | null>(null);
  const [dismissOpen, setDismissOpen] = useState(false);
  // Success confirmation snackbar for batch approve / dismiss. Failures and
  // partial results stay in the inline actionMsg Notice (it needs to persist
  // and itemize what failed); only clean successes graduate to a snackbar.
  const [actionToast, setActionToast] = useState<{ text: string } | null>(null);

  useAskAgentContext(
    () => ({
      page: 'batch-detail',
      visible_items: (data?.issues ?? []).map((row) => issueToVisible(row.issue)),
    }),
    [data],
  );

  const refresh = async () => {
    if (!id) return;
    try {
      const res = await api.batches.get(connection, id);
      setData(res);
      // Pre-select recommended variant per child the first time we see them.
      setSelectedVariants((prev) => {
        const next = { ...prev };
        for (const { issue, proposal } of res.issues) {
          if (next[issue.id]) continue;
          const vs = variantsFromProposal(proposal);
          if (vs && vs.length > 0) {
            next[issue.id] = vs.find((v) => v.recommended)?.id ?? vs[0].id;
          }
        }
        return next;
      });
      // Default expansion: in_review children expanded; others collapsed.
      setExpanded((prev) => {
        const next = { ...prev };
        for (const { issue } of res.issues) {
          if (issue.id in next) continue;
          next[issue.id] = issue.status === 'in_review';
        }
        return next;
      });
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [connection, id]);

  // Keep --wa-action-bar-height in sync so the confirmation snackbar floats
  // just above the batch action bar.
  useActionBarHeightVar([!!data]);

  const approveRow = async (issueID: string) => {
    const variantID = selectedVariants[issueID];
    setBusy(`approve-row:${issueID}`);
    setActionMsg(null);
    try {
      await api.approve(connection, issueID, variantID);
      await refresh();
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

  const rejectRow = async (issueID: string) => {
    setBusy(`reject-row:${issueID}`);
    setActionMsg(null);
    try {
      await api.reject(connection, issueID);
      await refresh();
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

  const approveAll = async () => {
    if (!id || !data) return;
    const pendingChildren: BatchApproveChild[] = data.issues
      .filter(({ issue }) => issue.status === 'in_review')
      .map(({ issue }) => ({
        issue_id: issue.id,
        variant_id: selectedVariants[issue.id],
      }));
    if (pendingChildren.length === 0) return;
    setBusy('approve-all');
    setActionMsg(null);
    try {
      const res = await api.batches.approveAll(connection, id, pendingChildren);
      const okCount = res.results.filter((r) => r.ok).length;
      const failed = res.results.filter((r) => !r.ok);
      if (failed.length === 0) {
        const kind = data.issues[0]?.proposal?.type as ProposalKind | undefined;
        setActionToast({ text: batchApproveText(kind) });
      } else {
        setActionMsg({
          kind: 'error',
          text: `Approved ${okCount} · ${failed.length} failed (${failed
            .map((f) => f.error?.code ?? 'unknown')
            .join(', ')})`,
        });
      }
      await refresh();
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

  // Dismiss-all is gated behind the same DismissDialog single proposals use,
  // so the operator captures a reason + optional comment. The daemon's
  // reject-all endpoint forwards the pair into dismiss_reason/dismiss_comment
  // for every in_review child — see handleRejectBatch.
  const openDismissAll = () => {
    setActionMsg(null);
    setDismissOpen(true);
  };

  const handleDismissAllConfirm = async ({
    reason,
    comment,
  }: {
    reason: DismissReason;
    comment?: string;
  }) => {
    if (!id) return;
    setBusy('reject-all');
    try {
      await api.batches.rejectAll(connection, id, { reason, comment });
      setDismissOpen(false);
      setActionToast({ text: batchDismissText(personaLabelFor(data?.batch.persona)) });
      await refresh();
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

  const totalProgress = useMemo(() => {
    if (!data) return { settled: 0, total: 0 };
    return {
      settled: data.batch.approved + data.batch.rejected,
      total: data.batch.total,
    };
  }, [data]);

  const batchLabel = id ? `BATCH·${id.slice(0, 6).toUpperCase()}` : 'Batch';

  if (error) {
    return (
      <Page
        breadcrumbs={
          <Breadcrumbs items={[{ label: 'Board', to: '/' }, { label: batchLabel }]} />
        }
        actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
        hasPadding
      >
        <div className="wa-subpage-content">
          <Notice.Root intent="error">
            <Notice.Description>
              Failed to load batch: {error}
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
          <Breadcrumbs items={[{ label: 'Board', to: '/' }, { label: batchLabel }]} />
        }
        actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
        hasPadding
      >
        <div className="wa-subpage-content">
          <Stack direction="row" gap="sm" align="center">
            <Spinner /> <Text variant="body-sm">Loading batch…</Text>
          </Stack>
        </div>
      </Page>
    );
  }

  const { batch, issues } = data;
  const personaLabel = personaLabelFor(batch.persona);
  const pendingCount = batch.pending;
  const firstProposal = data.issues[0]?.proposal;
  const isPricingBatch = firstProposal?.type === 'product_price_change';
  const isColdDraftBatch = firstProposal?.type === 'product_cold_draft';

  // Approved / Rejected / Pending counter strip + progress bar. Rendered
  // inside each body's KPI row → counter strip → children rhythm so the
  // settle status sits between the static batch summary (KPIs) and the
  // per-child list. Shared between marketing and pricing bodies.
  const counterStrip = (
    <Card.Root style={{ marginBottom: 'var(--wpds-dimension-gap-lg)' }}>
      <Card.Content>
        <div className="wa-batch-counter">
          <Counter label="Approved" value={batch.approved} tone="success" />
          <span className="wa-batch-counter__divider" aria-hidden="true" />
          <Counter label="Dismissed" value={batch.rejected} tone="error" />
          <span className="wa-batch-counter__divider" aria-hidden="true" />
          <Counter label="Pending" value={batch.pending} tone="neutral" />
          <span className="wa-batch-counter__divider" aria-hidden="true" />
          <div className="wa-batch-counter__progress">
            <div className="wa-batch-counter__progress-row">
              <span
                style={{
                  fontSize: 'var(--wpds-typography-font-size-xs)',
                  color: 'var(--wpds-color-fg-content-neutral-weak)',
                }}
              >
                Progress
              </span>
              <span
                className="wa-mono"
                style={{
                  fontSize: 'var(--wpds-typography-font-size-xs)',
                  color: 'var(--wpds-color-fg-content-neutral-weak)',
                }}
              >
                {totalProgress.settled} / {totalProgress.total}
              </span>
            </div>
            <div className="wa-batch-progress">
              <div
                className="wa-batch-progress__bar"
                style={{
                  width:
                    totalProgress.total > 0
                      ? `${(totalProgress.settled / totalProgress.total) * 100}%`
                      : '0%',
                }}
              />
            </div>
          </div>
        </div>
      </Card.Content>
    </Card.Root>
  );

  // Marketing-shape body: KPI strip + counter strip + per-child variant
  // accordion. Captures component state (selectedVariants, expanded, busy,
  // approveRow, rejectRow). Behavior is unchanged from before the pricing
  // dispatch landed.
  const renderMarketingBody = () => {
    // Best-of across all variants of all proposals in the batch — the
    // header reports the top-scoring candidate, not an average, so an
    // operator can see the ceiling of what's available to approve.
    let bestVoice: number | null = null;
    let bestSeo: number | null = null;
    for (const { proposal } of issues) {
      const vs = variantsFromProposal(proposal);
      if (!vs) continue;
      for (const v of vs) {
        if (typeof v.voice === 'number' && (bestVoice === null || v.voice > bestVoice)) {
          bestVoice = v.voice;
        }
        if (typeof v.seo === 'number' && (bestSeo === null || v.seo > bestSeo)) {
          bestSeo = v.seo;
        }
      }
    }

    return (
    <>
      {/* KPI row */}
      <div className="wa-kpi-row" style={{ marginBottom: 'var(--wpds-dimension-gap-lg)' }}>
        <Kpi
          label="Scope"
          value={`${batch.total} products selected`}
          hint="3 variants each"
        />
        <Kpi
          label="Best brand voice match"
          value={bestVoice !== null ? `${bestVoice}%` : '—'}
          score={bestVoice ?? undefined}
          hint={isColdDraftBatch ? 'vs. your voice model' : 'vs. your existing copy'}
        />
        <Kpi
          label="Best SEO score"
          value={bestSeo !== null ? `${bestSeo}` : '—'}
          score={bestSeo ?? undefined}
          hint="Product-copy rubric · out of 100"
        />
      </div>

      {counterStrip}

      {/* Per-child accordion */}
      <Stack direction="column" gap="md">
        {issues.map(({ issue, proposal }, idx) => {
          const variants = variantsFromProposal(proposal);
          const target = (proposal?.target ?? {}) as Record<string, unknown>;
          const previousCopy =
            typeof target.previous === 'string' ? target.previous : '';
          const previousShort =
            typeof target.previous_short === 'string' ? target.previous_short : '';
          const previousLong =
            typeof target.previous_long === 'string' ? target.previous_long : '';
          const productName =
            typeof target.product_name === 'string' ? target.product_name : issue.title;
          const productSku =
            typeof target.product_sku === 'string'
              ? target.product_sku
              : typeof target.sku === 'string'
                ? target.sku
                : issue.id.slice(0, 8);
          const isExpanded = expanded[issue.id] ?? false;
          const selectedVariantID = selectedVariants[issue.id];
          const selectedVariant = variants?.find((v) => v.id === selectedVariantID) ?? null;
          const reviewable = issue.status === 'in_review';
          const rowBusy = busy === `approve-row:${issue.id}` || busy === `reject-row:${issue.id}`;

          return (
            <Card.Root key={issue.id}>
              {/* CUSTOM: row-header click target to toggle expansion. (a) WPDS has no expandable-row primitive — DataViews owns its own row chrome; this is a non-DataViews list. (b) <button> wraps the row header with shared .wa-batch-row__header chrome + aria-expanded. (c) Follow-up: revisit if DataViews adds expandable-row support. */}
              <button
                type="button"
                className="wa-batch-row__header"
                onClick={() =>
                  setExpanded((prev) => ({ ...prev, [issue.id]: !prev[issue.id] }))
                }
                aria-expanded={isExpanded}
              >
                <span
                  className="wa-mono"
                  style={{
                    fontSize: 'var(--wpds-typography-font-size-xs)',
                    color: 'var(--wpds-color-fg-content-neutral-weak)',
                    width: 32,
                    flex: 'none',
                  }}
                >
                  {idx + 1} / {issues.length}
                </span>
                <ProductThumbnail
                  src={typeof target.image_url === 'string' ? target.image_url : undefined}
                  alt={typeof target.image_alt === 'string' ? target.image_alt : undefined}
                  persona={issue.persona}
                  size="sm"
                />
                <Stack direction="column" gap="xs" style={{ flex: 1, minWidth: 0 }}>
                  <Text
                    variant="body-md"
                    style={{
                      fontWeight: 'var(--wpds-typography-font-weight-medium)',
                      textAlign: 'left',
                    }}
                  >
                    {productName}
                  </Text>
                  <span
                    className="wa-mono"
                    style={{
                      fontSize: 'var(--wpds-typography-font-size-xs)',
                      color: 'var(--wpds-color-fg-content-neutral-weak)',
                    }}
                  >
                    {productSku}
                  </span>
                </Stack>
                {selectedVariant && typeof selectedVariant.seo === 'number' && (
                  <span className="wa-score-label">
                    SEO{' '}
                    <span className={`wa-score-label__value ${seoColorClass(selectedVariant.seo)}`}>
                      {selectedVariant.seo}
                    </span>
                  </span>
                )}
                {selectedVariant && typeof selectedVariant.voice === 'number' && (
                  <span className="wa-score-label">
                    Voice{' '}
                    <span className={`wa-score-label__value ${voiceColorClass(selectedVariant.voice)}`}>
                      {selectedVariant.voice}%
                    </span>
                  </span>
                )}
                <Badge intent={rowStatusIntent(issue.status)}>
                  {rowStatusLabel(issue.status)}
                </Badge>
                <span aria-hidden="true">
                  <Icon icon={isExpanded ? chevronUp : chevronDown} size={18} />
                </span>
              </button>

              {isExpanded && (
                <div className="wa-batch-row__body">
                  <div className="wa-batch-cols">
                    {/* Current column */}
                    <div className="wa-batch-col wa-batch-col--current">
                      <span className="wa-eyebrow">Current</span>
                      <Text
                        variant="body-sm"
                        style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
                      >
                        Live on store
                      </Text>
                      {isColdDraftBatch ? (
                        <>
                          <span className="wa-eyebrow" style={{ marginTop: 'var(--wpds-dimension-gap-md)' }}>
                            Short description
                          </span>
                          <Text
                            variant="body-sm"
                            style={{
                              color: previousShort
                                ? 'var(--wpds-color-fg-content-neutral)'
                                : 'var(--wpds-color-fg-content-neutral-weak)',
                              whiteSpace: 'pre-wrap',
                              lineHeight: 1.5,
                            }}
                          >
                            {previousShort || '— empty —'}
                          </Text>
                          <span className="wa-eyebrow" style={{ marginTop: 'var(--wpds-dimension-gap-md)' }}>
                            Long description
                          </span>
                          <Text
                            variant="body-sm"
                            style={{
                              color: previousLong
                                ? 'var(--wpds-color-fg-content-neutral)'
                                : 'var(--wpds-color-fg-content-neutral-weak)',
                              whiteSpace: 'pre-wrap',
                              lineHeight: 1.5,
                            }}
                          >
                            {previousLong || '— empty —'}
                          </Text>
                        </>
                      ) : (
                        <>
                          <span className="wa-eyebrow" style={{ marginTop: 'var(--wpds-dimension-gap-md)' }}>
                            Description
                          </span>
                          <Text
                            variant="body-sm"
                            style={{
                              color: previousCopy
                                ? 'var(--wpds-color-fg-content-neutral)'
                                : 'var(--wpds-color-fg-content-neutral-weak)',
                              whiteSpace: 'pre-wrap',
                              lineHeight: 1.5,
                            }}
                          >
                            {previousCopy || '— empty —'}
                          </Text>
                        </>
                      )}
                    </div>

                    {/* Variant columns */}
                    {(variants ?? []).map((v) => {
                      const isSelected = selectedVariantID === v.id;
                      // CUSTOM: variant-column click target inside a batch row. (a) WPDS has no selectable-column / radio-card component. (b) <button> wraps the column with .wa-batch-col chrome and selected state. (c) Follow-up: see CardLink composite note in Kanban.tsx.
                      return (
                        <button
                          key={v.id}
                          type="button"
                          className={`wa-batch-col wa-batch-col--variant${
                            isSelected ? ' wa-batch-col--selected' : ''
                          }`}
                          onClick={() =>
                            reviewable &&
                            setSelectedVariants((prev) => ({ ...prev, [issue.id]: v.id }))
                          }
                          disabled={!reviewable}
                        >
                          <div className="wa-batch-col__head">
                            {(() => {
                              // Mirror the IssueDetail variant-card treatment:
                              // letter comes from the variant id (`var_a` → A)
                              // so the colored circle stays consistent across
                              // data sources; the descriptor on the right comes
                              // from a multi-char `label` or the angle field.
                              const letter = (v.id.split('_').pop() ?? v.label ?? '')
                                .slice(0, 1)
                                .toUpperCase();
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
                            <span
                              className={`wa-radio-mark${
                                isSelected ? ' wa-radio-mark--selected' : ''
                              }`}
                              aria-hidden="true"
                              style={{ marginLeft: 'auto' }}
                            >
                              {isSelected && <Icon icon={check} size={14} />}
                            </span>
                          </div>
                          <div className="wa-batch-col__scores">
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
                                <span className={`wa-score-label__value ${voiceColorClass(v.voice)}`}>
                                  {v.voice}%
                                </span>
                              </span>
                            )}
                            <span className="wa-score-label">
                              <span className="wa-score-label__value">
                                {v.charCount} ch
                              </span>
                            </span>
                          </div>
                          {isColdDraftBatch ? (
                            <>
                              <span className="wa-eyebrow" style={{ marginTop: 'var(--wpds-dimension-gap-md)' }}>
                                Short description
                              </span>
                              <Text
                                variant="body-sm"
                                style={{
                                  color: v.body_short
                                    ? 'var(--wpds-color-fg-content-neutral)'
                                    : 'var(--wpds-color-fg-content-neutral-weak)',
                                  whiteSpace: 'pre-wrap',
                                  lineHeight: 1.5,
                                  textAlign: 'left',
                                }}
                              >
                                {v.body_short || 'no change'}
                              </Text>
                              <span className="wa-eyebrow" style={{ marginTop: 'var(--wpds-dimension-gap-md)' }}>
                                Long description
                              </span>
                              <Text
                                variant="body-sm"
                                style={{
                                  color: v.body_long
                                    ? 'var(--wpds-color-fg-content-neutral)'
                                    : 'var(--wpds-color-fg-content-neutral-weak)',
                                  whiteSpace: 'pre-wrap',
                                  lineHeight: 1.5,
                                  textAlign: 'left',
                                }}
                              >
                                {v.body_long || 'no change'}
                              </Text>
                            </>
                          ) : (
                            <>
                              <span className="wa-eyebrow" style={{ marginTop: 'var(--wpds-dimension-gap-md)' }}>
                                Description
                              </span>
                              <Text
                                variant="body-sm"
                                style={{ whiteSpace: 'pre-wrap', lineHeight: 1.5, textAlign: 'left' }}
                              >
                                {v.body}
                              </Text>
                            </>
                          )}
                          {v.note && (
                            <div
                              style={{
                                marginTop: 'var(--wpds-dimension-gap-sm)',
                                fontSize: 'var(--wpds-typography-font-size-xs)',
                                color: 'var(--wpds-color-fg-content-neutral-weak)',
                                fontStyle: 'italic',
                                textAlign: 'left',
                              }}
                            >
                              {v.note}
                            </div>
                          )}
                        </button>
                      );
                    })}
                  </div>

                  {/* Per-row footer */}
                  <div className="wa-batch-row__footer">
                    {!reviewable ? (
                      <Text
                        variant="body-sm"
                        style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
                      >
                        {`Already ${issue.status === 'done' ? 'approved' : issue.status}`}
                      </Text>
                    ) : selectedVariantID ? (
                      <Text variant="body-sm">
                        <strong>
                          Variant{' '}
                          {(selectedVariantID.split('_').pop() ?? '').toUpperCase()}{' '}
                          selected
                        </strong>
                        <span style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}>
                          {' — '}
                          {isColdDraftBatch
                            ? 'Long and short descriptions will update on approve'
                            : 'Description will update on approve'}
                        </span>
                      </Text>
                    ) : (
                      <Text
                        variant="body-sm"
                        style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
                      >
                        No variant selected yet — click a column above to choose
                      </Text>
                    )}
                    <div className="wa-batch-row__footer-actions">
                      <Button
                        __next40pxDefaultSize
                        variant="tertiary"
                        isDestructive
                        onClick={() => rejectRow(issue.id)}
                        disabled={!reviewable || rowBusy || busy !== null}
                      >
                        {busy === `reject-row:${issue.id}` ? 'Dismissing…' : 'Dismiss'}
                      </Button>
                      <Button
                        __next40pxDefaultSize
                        variant="secondary"
                        onClick={() => approveRow(issue.id)}
                        disabled={!reviewable || !selectedVariantID || rowBusy || busy !== null}
                      >
                        {busy === `approve-row:${issue.id}`
                          ? 'Applying…'
                          : 'Approve variant'}
                      </Button>
                    </div>
                  </div>
                </div>
              )}
            </Card.Root>
          );
        })}
      </Stack>
    </>
    );
  };

  return (
    <div className="wa-detail-shell">
      <Page
        breadcrumbs={
          <Breadcrumbs
            items={[
              { label: 'Board', to: '/' },
              { label: `BATCH·${batch.id.slice(0, 6).toUpperCase()}` },
            ]}
          />
        }
        badges={
          pendingCount > 0 ? (
            <Badge intent="medium">Needs review</Badge>
          ) : batch.approved === 0 && batch.rejected > 0 ? (
            <Badge intent="none">Archived</Badge>
          ) : (
            <Badge intent="stable">Done</Badge>
          )
        }
        actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
        hasPadding
        className="wa-detail-shell-page"
      >
        <div className="wa-subpage-content">
        <ProposalHeader
          persona={batch.persona ?? 'marketing'}
          personaLabel={personaLabel}
          verb={isPricingBatch ? 'proposes a pricing run' : 'proposes content'}
          timestamp={batch.updated_at}
          modelLine={isPricingBatch ? 'Claude Haiku 4.5 · web_search' : 'Claude Sonnet 4.6'}
          title={batch.title}
          description={
            isPricingBatch
              ? 'Review every product in this run. Approve to apply all proposed price changes to your store.'
              : 'Three voice variants per product. Pick one, approve, and the agent writes it straight to WooCommerce.'
          }
        />

        {/* Branch body on first child's proposal type. Marketing keeps the
            existing variant-accordion + Marketing KPI strip; pricing renders
            BatchProductCard rows with a pricing KPI strip. */}
        {isPricingBatch
          ? renderPricingBody(data, counterStrip, {
              busy,
              onApprove: approveRow,
              onReject: rejectRow,
            })
          : renderMarketingBody()}

        {actionMsg && (
          <div style={{ marginTop: 'var(--wpds-dimension-gap-md)' }}>
            <Notice.Root
              intent={actionMsg.kind === 'success' ? 'success' : 'error'}
            >
              <Notice.Description>{actionMsg.text}</Notice.Description>
            </Notice.Root>
          </div>
        )}
        </div>
      </Page>

      {/* Sticky bottom action bar — top-level Approve all / Dismiss all. */}
      <div className="wa-action-bar">
        <div className="wa-action-bar-row">
          <div className="wa-action-bar__left">
            <Stack direction="column" gap="xs">
              <Text
                variant="body-sm"
                style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
              >
                Approve each product or approve all at once
              </Text>
              <Text
                variant="body-sm"
                style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
              >
                Long and short descriptions will update on approve
              </Text>
            </Stack>
          </div>
          <div className="wa-action-bar-actions">
            {!isColdDraftBatch && <Badge intent="none">Reversible</Badge>}
            <Button
              __next40pxDefaultSize
              variant="tertiary"
              isDestructive
              onClick={openDismissAll}
              disabled={pendingCount === 0 || busy !== null}
            >
              {busy === 'reject-all' ? 'Dismissing…' : 'Dismiss all'}
            </Button>
            <Button variant="tertiary" __next40pxDefaultSize onClick={() => nav('/')}>
              Cancel
            </Button>
            <Button
              __next40pxDefaultSize
              variant="primary"
              onClick={approveAll}
              disabled={pendingCount === 0 || busy !== null}
            >
              {busy === 'approve-all' ? 'Applying to store…' : 'Approve all'}
            </Button>
          </div>
        </div>
      </div>

      <DismissDialog
        open={dismissOpen}
        onOpenChange={setDismissOpen}
        persona={batch.persona ?? undefined}
        variantCount={pendingCount}
        titleOverride={
          pendingCount > 1
            ? `Dismiss all ${pendingCount} products?`
            : 'Dismiss this product?'
        }
        bodyOverride={
          pendingCount > 1
            ? `The ${personaLabel.toLowerCase()} won't re-propose copy for these products unless you ask. They move to your archive — not deleted yet.`
            : `The ${personaLabel.toLowerCase()} won't re-propose copy for this product unless you ask. It moves to your archive — not deleted yet.`
        }
        confirmLabelOverride={pendingCount > 1 ? 'Dismiss all' : 'Dismiss'}
        onConfirm={handleDismissAllConfirm}
        busy={busy === 'reject-all'}
      />
      {actionToast && (
        <ActionSnackbar
          text={actionToast.text}
          onRemove={() => setActionToast(null)}
          placement="above-action-bar"
        />
      )}
    </div>
  );
}

function rowStatusLabel(status: string): string {
  switch (status) {
    case 'in_review':
      return 'Pending';
    case 'done':
      return 'Approved';
    case 'rejected':
      return 'Dismissed';
    case 'in_progress':
      return 'In progress';
    default:
      return status;
  }
}

function rowStatusIntent(
  status: string,
): 'none' | 'informational' | 'stable' {
  // Mirrors StatusBadge on IssueDetail so per-row badges in the batch
  // match the badges everywhere else in the app.
  switch (status) {
    case 'done':
      return 'stable';
    case 'in_progress':
      return 'informational';
    default:
      // in_review, rejected, anything else
      return 'none';
  }
}


interface CounterProps {
  label: string;
  value: number;
  tone: 'success' | 'error' | 'neutral';
}

function Counter({ label, value, tone }: CounterProps) {
  // Type style matches Kpi tiles (font-size-lg, weight 700, fg-content-neutral)
  // so the counters in the summary card share a visual rhythm with the KPI
  // strip above. Tone color (success/error) only kicks in once the count is
  // non-zero — a green 0 / red 0 would over-signal.
  const color =
    value === 0
      ? 'var(--wpds-color-fg-content-neutral)'
      : tone === 'success'
        ? 'var(--wpds-color-fg-content-success)'
        : tone === 'error'
          ? 'var(--wpds-color-fg-content-error)'
          : 'var(--wpds-color-fg-content-neutral)';
  return (
    <div className="wa-batch-counter__cell">
      <Text
        style={{
          color,
          fontSize: 'var(--wpds-typography-font-size-lg)',
          fontWeight: 700,
          lineHeight: 'var(--wpds-typography-line-height-lg)',
        }}
      >
        {value}
      </Text>
      <span
        style={{
          fontSize: 'var(--wpds-typography-font-size-xs)',
          color: 'var(--wpds-color-fg-content-neutral-weak)',
        }}
      >
        {label}
      </span>
    </div>
  );
}

// Pricing-shape body: KPI strip aggregated across the batch + counter
// strip + a vertical list of BatchProductCards. Per-row Approve / Dismiss
// callbacks come from the parent's batch-level state machine — the cards
// render the buttons themselves but the network calls live in
// approveRow / rejectRow in the parent. The counter strip is passed in by
// the caller so the same JSX serves marketing and pricing bodies.
interface PricingBodyHandlers {
  busy: string | null;
  onApprove: (issueId: string) => void;
  onReject: (issueId: string) => void;
}

function renderPricingBody(
  data: BatchDetail,
  counterStrip: ReactNode,
  handlers: PricingBodyHandlers,
) {
  const products: BatchProduct[] = data.issues.map((iwp) => {
    const t = (iwp.proposal?.target ?? {}) as Record<string, unknown>;
    const direction: BatchProduct['direction'] =
      t.direction === 'increase' || t.direction === 'decrease'
        ? t.direction
        : 'flat';
    return {
      issueId: iwp.issue.id,
      status: iwp.issue.status,
      productId: typeof t.product_id === 'number' ? t.product_id : 0,
      sku: typeof t.product_sku === 'string' ? t.product_sku : '',
      name: typeof t.product_name === 'string' ? t.product_name : iwp.issue.title,
      imageUrl: typeof t.image_url === 'string' ? t.image_url : undefined,
      imageAlt: typeof t.image_alt === 'string' ? t.image_alt : undefined,
      categoryPath:
        typeof t.product_category === 'string' ? t.product_category : undefined,
      previousPrice: typeof t.previous_price === 'number' ? t.previous_price : 0,
      proposedPrice: typeof t.proposed_price === 'number' ? t.proposed_price : 0,
      percentChange: typeof t.percent_change === 'number' ? t.percent_change : 0,
      direction,
      currency: typeof t.currency === 'string' ? t.currency : 'USD',
      rationale: iwp.proposal?.content ?? '',
      sources: Array.isArray(t.sources) ? (t.sources as PriceSource[]) : [],
      observedLow: typeof t.observed_low === 'number' ? t.observed_low : undefined,
      observedMedian:
        typeof t.observed_median === 'number' ? t.observed_median : undefined,
      observedHigh:
        typeof t.observed_high === 'number' ? t.observed_high : undefined,
    };
  });

  // Aggregate KPIs across the batch.
  const totalDelta = products.reduce(
    (acc, p) => acc + (p.proposedPrice - p.previousPrice),
    0,
  );
  const totalPct =
    products.length > 0
      ? products.reduce((acc, p) => acc + p.percentChange, 0) / products.length
      : 0;
  const uniqueSourceURLs = new Set(
    products.flatMap((p) => p.sources.map((s) => s.url)),
  );
  const currency = products[0]?.currency ?? 'USD';

  // Variable-product batches: all children share the same rationale and the
  // same source list. Lift them out of each row and render once below the
  // list — matches the single-product PriceIssueView pattern.
  const isVariableBatch = data.batch.intent === 'pricing_variable';
  const sharedRationale = isVariableBatch ? (products[0]?.rationale ?? '') : '';
  const dedupedSources: PriceSource[] = isVariableBatch
    ? (() => {
        const seen = new Set<string>();
        const out: PriceSource[] = [];
        for (const p of products) {
          for (const s of p.sources) {
            if (seen.has(s.url)) continue;
            seen.add(s.url);
            out.push(s);
          }
        }
        return out;
      })()
    : [];
  const sym = currencySymbol(currency);
  const totalToneCalc: 'success' | 'warning' = totalDelta >= 0 ? 'success' : 'warning';
  const medianToneCalc: 'neutral' | 'warning' | 'success' =
    Math.abs(totalPct) < 5 ? 'neutral' : totalPct >= 0 ? 'warning' : 'success';

  return (
    <Stack direction="column" gap="lg">
      <div className="wa-kpi-row">
        <Kpi
          label="Products"
          value={String(products.length)}
          hint={products[0]?.categoryPath ?? ''}
        />
        <Kpi
          label="Total impact"
          value={`${totalDelta >= 0 ? '+' : '−'}${sym}${Math.abs(totalDelta).toFixed(2)}`}
          tone={totalToneCalc}
          hint="across batch"
        />
        <Kpi
          label="Median change"
          value={`${totalPct >= 0 ? '+' : ''}${totalPct.toFixed(1)}%`}
          tone={medianToneCalc}
          hint="median across batch"
        />
        <Kpi
          label="Sources"
          value={String(uniqueSourceURLs.size)}
          tone={uniqueSourceURLs.size >= 3 ? 'success' : 'caution'}
          hint="comparable products"
        />
      </div>

      {counterStrip}

      <Stack direction="column" gap="sm">
        {products.map((p, idx) => (
          <BatchProductCard
            key={p.issueId || idx}
            product={p}
            defaultExpanded={idx === 0}
            hideRationaleAndSources={isVariableBatch}
            busy={handlers.busy}
            reviewable={p.status === 'in_review'}
            onApprove={handlers.onApprove}
            onReject={handlers.onReject}
          />
        ))}
      </Stack>

      {isVariableBatch && sharedRationale && (
        <Card.Root>
          <Card.Header>
            <Stack direction="row" gap="md" align="center" style={{ width: '100%' }}>
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
                shared across all variations
              </Text>
            </Stack>
          </Card.Header>
          <Card.Content>
            <Text
              variant="body-md"
              style={{ whiteSpace: 'pre-wrap', lineHeight: 1.65 }}
            >
              {sharedRationale}
            </Text>
          </Card.Content>
        </Card.Root>
      )}

      {isVariableBatch && dedupedSources.length > 0 && (
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
                {`${dedupedSources.length} comparable${dedupedSources.length === 1 ? '' : 's'}`}
              </Badge>
            </Stack>
          </Card.Header>
          <Card.Content>
            <Stack direction="column" gap="sm">
              {dedupedSources.map((s, idx) => (
                <SourceRow
                  key={idx}
                  source={s}
                  currency={currency}
                  proposed={products[0]?.proposedPrice ?? 0}
                />
              ))}
            </Stack>
          </Card.Content>
        </Card.Root>
      )}
    </Stack>
  );
}

function currencySymbol(code: string): string {
  switch (code.toUpperCase()) {
    case 'USD':
    case 'CAD':
    case 'AUD':
      return '$';
    case 'GBP':
      return '£';
    case 'EUR':
      return '€';
    default:
      return '';
  }
}
