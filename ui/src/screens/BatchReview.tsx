import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
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
  type PriceSource,
} from '../api/client';
import { PersonaAvatar, personaKeyFrom } from '../components/PersonaAvatar';
import Kpi from '../components/Kpi';
import PageGlobalActions from '../components/PageGlobalActions';
import Breadcrumbs from '../components/Breadcrumbs';
import BatchProductCard, { type BatchProduct } from '../components/BatchProductCard';
import ProductThumbnail from '../components/ProductThumbnail';

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
        setActionMsg({
          kind: 'success',
          text: `Approved ${okCount} of ${pendingChildren.length}.`,
        });
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

  const rejectAll = async () => {
    if (!id) return;
    setBusy('reject-all');
    setActionMsg(null);
    try {
      const res = await api.batches.rejectAll(connection, id);
      const okCount = res.results.filter((r) => r.ok).length;
      setActionMsg({
        kind: 'success',
        text: `Rejected ${okCount} ${okCount === 1 ? 'child' : 'children'}.`,
      });
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
              Failed to load batch: {error} <Link to="/">Back to board</Link>
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
  const personaKey = personaKeyFrom(batch.persona);
  const personaLabel =
    batch.persona === 'marketing' ? 'Marketing agent' : `${batch.persona ?? 'unassigned'} agent`;
  const pendingCount = batch.pending;
  const firstProposal = data.issues[0]?.proposal;
  const isPricingBatch = firstProposal?.type === 'product_price_change';
  const isColdDraftBatch = firstProposal?.type === 'product_cold_draft';

  // Marketing-shape body: KPI strip + per-child variant accordion. Captures
  // component state (selectedVariants, expanded, busy, approveRow, rejectRow).
  // Behavior is unchanged from before the pricing dispatch landed.
  const renderMarketingBody = () => (
    <>
      {/* KPI row */}
      <div className="wa-kpi-row" style={{ marginBottom: 'var(--wpds-dimension-gap-lg)' }}>
        <Kpi
          label="Scope"
          value={`${batch.total} products selected`}
          hint="3 variants each"
        />
        <Kpi
          label="Brand voice match"
          value="96%"
          score={96}
          hint={isColdDraftBatch ? 'vs. your voice model' : 'vs. your existing copy'}
        />
        <Kpi
          label="SEO score"
          value="91"
          score={91}
          hint="Product-copy rubric · out of 100"
        />
        <Kpi label="Est. impact" value="+14% CTR" hint="on product listing pages" tone="success" />
      </div>

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
          const draftingFields: string[] = Array.isArray((target as any).drafting)
            ? ((target as any).drafting as unknown[]).filter(
                (f): f is string => typeof f === 'string',
              )
            : [];
          const applyTargetParts = draftingFields.reduce<string[]>((acc, f) => {
            if (f === 'short') acc.push('product.short_description');
            else if (f === 'long') acc.push('product.description');
            return acc;
          }, []);
          const applyHint =
            isColdDraftBatch && applyTargetParts.length > 0
              ? ` — will write ${applyTargetParts.join(' + ')} on approve`
              : '';
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
                <PersonaAvatar persona={personaKey} size="md" />
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
                    Best SEO{' '}
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
                <span className={`wa-status-pill wa-status-pill--${rowStatusTone(issue.status)}`}>
                  {rowStatusLabel(issue.status)}
                </span>
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
                            <span
                              className="wa-batch-col__letter"
                              style={{
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
                    <Text
                      variant="body-sm"
                      style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
                    >
                      {!reviewable
                        ? `Already ${issue.status === 'done' ? 'approved' : issue.status}`
                        : selectedVariantID
                          ? `Variant ${selectedVariantID} selected${applyHint}`
                          : 'No variant selected yet — click a column above to choose'}
                    </Text>
                    <div className="wa-batch-row__footer-actions">
                      <Button
                        __next40pxDefaultSize
                        variant="tertiary"
                        isDestructive
                        onClick={() => rejectRow(issue.id)}
                        disabled={!reviewable || rowBusy || busy !== null}
                      >
                        {busy === `reject-row:${issue.id}` ? 'Rejecting…' : 'Reject'}
                      </Button>
                      <Button
                        __next40pxDefaultSize
                        variant="secondary"
                        onClick={() => approveRow(issue.id)}
                        disabled={!reviewable || !selectedVariantID || rowBusy || busy !== null}
                      >
                        {busy === `approve-row:${issue.id}`
                          ? 'Applying…'
                          : selectedVariantID
                            ? `Approve Variant ${selectedVariantID}`
                            : 'Approve selected'}
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
          <>
            {batch.intent && (
              <span
                style={{
                  fontSize: 'var(--wpds-typography-font-size-xs)',
                  padding: '2px 8px',
                  borderRadius: 'var(--wpds-border-radius-sm)',
                  background: 'var(--wpds-color-bg-surface-info-weak)',
                  color: 'var(--wpds-color-fg-interactive-brand)',
                }}
              >
                {batch.intent}
              </span>
            )}
          </>
        }
        actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
        hasPadding
        className="wa-detail-shell-page"
      >
        <div className="wa-subpage-content">
        {/* Persona eyebrow */}
        <Stack direction="row" gap="sm" align="center" style={{ marginBottom: 'var(--wpds-dimension-gap-sm)' }}>
          <PersonaAvatar persona={personaKey} size="md" />
          <Text variant="body-sm" style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}>
            <strong style={{ color: 'var(--wpds-color-fg-content-neutral)' }}>
              {personaLabel}
            </strong>{' '}
            {isPricingBatch ? 'proposes a pricing run' : 'proposes content'} ·{' '}
            {relativeTime(batch.updated_at)} ·{' '}
            <span className="wa-mono">
              {isPricingBatch ? 'Claude Haiku 4.5 · web_search' : 'Claude Sonnet 4.6'}
            </span>
          </Text>
        </Stack>

        {/* Title + subhead */}
        <Text
          variant="heading-2xl"
          render={<h2 style={{ margin: 0, marginBottom: 'var(--wpds-dimension-gap-sm)' }} />}
        >
          {batch.title}
        </Text>
        <Text
          variant="body-md"
          style={{
            color: 'var(--wpds-color-fg-content-neutral-weak)',
            maxWidth: 760,
            marginBottom: 'var(--wpds-dimension-gap-xl)',
          }}
        >
          {isPricingBatch
            ? 'Review every product in this run. Approve to apply all proposed price changes to your store; the previous prices are snapshotted — reversible from the Done column.'
            : 'Three voice variants per product. Pick one, approve, and the agent writes it straight to WooCommerce. The previous copy is snapshotted — reversible from the Done column.'}
        </Text>

        {/* Counter strip */}
        <Card.Root style={{ marginBottom: 'var(--wpds-dimension-gap-lg)' }}>
          <Card.Content>
            <div className="wa-batch-counter">
              <div className="wa-batch-counter__cells">
                <Counter label="Approved" value={batch.approved} tone="success" />
                <Counter label="Rejected" value={batch.rejected} tone="error" />
                <Counter label="Pending" value={batch.pending} tone="neutral" />
              </div>
              <div className="wa-batch-counter__progress">
                <span className="wa-eyebrow">Progress</span>
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
            </div>
          </Card.Content>
        </Card.Root>

        {/* Branch body on first child's proposal type. Marketing keeps the
            existing variant-accordion + Marketing KPI strip; pricing renders
            BatchProductCard rows with a pricing KPI strip. */}
        {isPricingBatch ? renderPricingBody(data) : renderMarketingBody()}

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

      {/* Sticky bottom action bar — top-level Approve all / Reject all. */}
      <div className="wa-action-bar">
        <div className="wa-action-bar-row">
          <div className="wa-action-bar__left">
            <span
              style={{
                height: 28,
                width: 28,
                borderRadius: '50%',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                fontWeight: 700,
                fontSize: 'var(--wpds-typography-font-size-sm)',
                background: 'var(--wpds-color-bg-interactive-brand-strong)',
                color: 'var(--wpds-color-fg-interactive-brand-strong)',
                flex: 'none',
              }}
            >
              {pendingCount}
            </span>
            <Stack direction="column" gap="xs">
              <Text
                variant="body-sm"
                style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
              >
                {pendingCount === 0
                  ? 'All children settled'
                  : `${pendingCount} pending · ${countSelected(data, selectedVariants)} variants picked`}
              </Text>
              <Text
                variant="body-sm"
                style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
              >
                Approve-all writes one issue at a time, granular per-child PEP audit.
              </Text>
            </Stack>
          </div>
          <div className="wa-action-bar-actions">
            <Badge intent="none">Reversible · always</Badge>
            <Button
              __next40pxDefaultSize
              variant="tertiary"
              isDestructive
              onClick={rejectAll}
              disabled={pendingCount === 0 || busy !== null}
            >
              {busy === 'reject-all'
                ? 'Rejecting…'
                : isPricingBatch
                  ? 'Dismiss batch'
                  : 'Reject all'}
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
              {busy === 'approve-all'
                ? 'Applying to store…'
                : isPricingBatch
                  ? `Approve & apply ${data.issues.length} prices to store`
                  : 'Approve & apply to store'}
            </Button>
          </div>
        </div>
      </div>
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
      return 'Rejected';
    case 'in_progress':
      return 'In progress';
    default:
      return status;
  }
}

function rowStatusTone(status: string): 'neutral' | 'success' | 'error' | 'warning' {
  switch (status) {
    case 'done':
      return 'success';
    case 'rejected':
      return 'error';
    case 'in_progress':
      return 'warning';
    default:
      return 'neutral';
  }
}

function countSelected(
  data: BatchDetail,
  selected: Record<string, string>,
): number {
  let n = 0;
  for (const { issue } of data.issues) {
    if (issue.status === 'in_review' && selected[issue.id]) n++;
  }
  return n;
}

interface CounterProps {
  label: string;
  value: number;
  tone: 'success' | 'error' | 'neutral';
}

function Counter({ label, value, tone }: CounterProps) {
  const color =
    tone === 'success'
      ? 'var(--wpds-color-fg-content-success)'
      : tone === 'error'
        ? 'var(--wpds-color-fg-content-error)'
        : 'var(--wpds-color-fg-content-neutral)';
  return (
    <div className="wa-batch-counter__cell">
      <Text
        variant="heading-md"
        style={{
          color,
          fontWeight: 'var(--wpds-typography-font-weight-medium)',
        }}
      >
        {value}
      </Text>
      <span className="wa-eyebrow">{label}</span>
    </div>
  );
}

// Pricing-shape body: KPI strip aggregated across the batch + a vertical
// list of BatchProductCards. Pure: no closure over component state — the
// Approve / Reject wiring still flows through the sticky action bar in the
// parent. Task 7 will refine the action-bar copy for pricing batches.
function renderPricingBody(data: BatchDetail) {
  const products: BatchProduct[] = data.issues.map((iwp) => {
    const t = (iwp.proposal?.target ?? {}) as Record<string, unknown>;
    const direction: BatchProduct['direction'] =
      t.direction === 'increase' || t.direction === 'decrease'
        ? t.direction
        : 'flat';
    return {
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

      <Stack direction="column" gap="sm">
        {products.map((p, idx) => (
          <BatchProductCard
            key={p.productId || idx}
            product={p}
            defaultExpanded={idx === 0}
          />
        ))}
      </Stack>
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
