import { useState } from 'react';
import { Card, Stack, Text } from '@wordpress/ui';
import { Button } from '@wordpress/components';
import { Icon, chevronDown, chevronUp } from '@wordpress/icons';
import SectionHeader from './SectionHeader';
import SourceRow from './SourceRow';
import ProductThumbnail from './ProductThumbnail';
import ObservedRange from './ObservedRange';
import type { PriceSource } from '../api/client';

export interface BatchProduct {
  /** Child issue ID — needed for the per-row Approve / Dismiss buttons to
   *  fire `/v1/issues/<id>/approve` and `/reject`. */
  issueId: string;
  /** Child issue status — when not 'in_review', the per-row action footer
   *  shows a "Already approved/dismissed" message instead of buttons. */
  status: string;
  productId: number;
  sku: string;
  name: string;
  imageUrl?: string;
  imageAlt?: string;
  categoryPath?: string;
  previousPrice: number;
  proposedPrice: number;
  percentChange: number;
  direction: 'increase' | 'decrease' | 'flat';
  currency: string;
  rationale: string;
  sources: PriceSource[];
  /** Optional observed market band — when all three numbers are present and
   *  high > low, the expanded state renders the shared ObservedRange slider
   *  per row. */
  observedLow?: number;
  observedMedian?: number;
  observedHigh?: number;
}

interface Props {
  product: BatchProduct;
  defaultExpanded?: boolean;
  /** When true, the expanded state suppresses the per-card rationale + sources
   *  blocks. Used by variable-product batches (intent='pricing_variable')
   *  where all children share the same rationale/sources and the parent renders
   *  them once below the row list — matches the single-product PriceIssueView
   *  pattern. Defaults to false for category batches. */
  hideRationaleAndSources?: boolean;
  /** Per-row busy state. The parent's busy state is namespaced per row
   *  ('approve-row:<id>' / 'reject-row:<id>') — the card flips its buttons
   *  to a spinner when its own row is in flight. */
  busy?: string | null;
  /** When false, the per-row Approve / Dismiss buttons are disabled. */
  reviewable?: boolean;
  /** Per-row approve handler. When omitted, no buttons render. */
  onApprove?: (issueId: string) => void;
  /** Per-row dismiss handler. When omitted, no buttons render. */
  onReject?: (issueId: string) => void;
}

// CUSTOM: expandable product card for the pricing batch review. (a) WPDS
// has no disclosure / accordion component; we compose Card.Root +
// SectionHeader + a chevron toggle button. (b) The collapsed state is a
// single horizontal row so 10+ rows can be skimmed without scroll.
// (c) Documented in DESIGN.md Component inventory.
export default function BatchProductCard({
  product,
  defaultExpanded = false,
  hideRationaleAndSources = false,
  busy = null,
  reviewable = true,
  onApprove,
  onReject,
}: Props) {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const toggle = () => setExpanded((v) => !v);

  const sym = currencySymbol(product.currency);
  const arrow =
    product.direction === 'decrease'
      ? '↓'
      : product.direction === 'increase'
        ? '↑'
        : '·';
  const sign = product.percentChange >= 0 ? '+' : '';
  const pctLabel = `${sign}${product.percentChange.toFixed(1)}%`;
  const deltaAbs = Math.abs(product.proposedPrice - product.previousPrice);
  const deltaSign = product.proposedPrice >= product.previousPrice ? '+' : '−';
  const deltaLabel = `${deltaSign}${sym}${deltaAbs.toFixed(2)}`;
  const directionColor =
    product.direction === 'increase'
      ? 'var(--wpds-color-fg-content-warning)'
      : product.direction === 'decrease'
        ? 'var(--wpds-color-fg-content-success)'
        : 'var(--wpds-color-fg-content-neutral)';

  return (
    <Card.Root
      className={`wa-batch-product-card${expanded ? ' wa-batch-product-card--expanded' : ''}`}
    >
      <button
        type="button"
        onClick={toggle}
        className="wa-batch-product-card__header"
        aria-expanded={expanded}
      >
        <Stack direction="row" gap="md" align="center" style={{ width: '100%' }}>
          <ProductThumbnail
            src={product.imageUrl}
            alt={product.imageAlt}
            persona="pricing"
            size="sm"
          />
          <div style={{ flex: 1, minWidth: 0 }}>
        <SectionHeader
          eyebrow={product.sku}
          title={
            <Text
              variant="body-md"
              style={{
                fontWeight: 'var(--wpds-typography-font-weight-medium)',
              }}
            >
              {product.name}
            </Text>
          }
          badge={
            product.categoryPath ? (
              <span
                style={{
                  fontSize: 'var(--wpds-typography-font-size-xs)',
                  padding: '2px 8px',
                  borderRadius: 'var(--wpds-border-radius-sm)',
                  background: 'var(--wpds-color-bg-surface-neutral-weak)',
                  color: 'var(--wpds-color-fg-content-neutral)',
                }}
              >
                {product.categoryPath}
              </span>
            ) : undefined
          }
          meta={
            <Stack direction="row" gap="sm" align="center">
              <Text
                variant="body-sm"
                style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
              >
                {sym}
                {product.previousPrice.toFixed(2)} → {sym}
                {product.proposedPrice.toFixed(2)}
              </Text>
              <span
                style={{
                  fontSize: 'var(--wpds-typography-font-size-xs)',
                  fontWeight: 'var(--wpds-typography-font-weight-medium)',
                  padding: '2px 8px',
                  borderRadius: 'var(--wpds-border-radius-sm)',
                  background: 'var(--wpds-color-bg-surface-neutral-weak)',
                  color: directionColor,
                }}
              >
                {arrow} {deltaLabel} · {pctLabel}
              </span>
              <Icon icon={expanded ? chevronUp : chevronDown} size={20} />
            </Stack>
          }
        />
          </div>
        </Stack>
      </button>

      {expanded && (
        <Card.Content>
          <Stack direction="column" gap="md">
            {/* Per-row market range. Renders only when the proposal target
                carries observed_low/high (and they differ). Variable
                batches set these per child; category batches may or may
                not, depending on the persona's output. */}
            {typeof product.observedLow === 'number' &&
              typeof product.observedHigh === 'number' &&
              product.observedHigh > product.observedLow && (
                <ObservedRange
                  low={product.observedLow}
                  median={product.observedMedian}
                  high={product.observedHigh}
                  proposed={product.proposedPrice}
                  currency={product.currency}
                />
              )}

            {!hideRationaleAndSources && (
              <>
                <Text
                  variant="body-md"
                  style={{ whiteSpace: 'pre-wrap', lineHeight: 1.65 }}
                >
                  {product.rationale || '— no rationale attached —'}
                </Text>
                {product.sources.length > 0 && (
                  <Stack direction="column" gap="sm">
                    <span className="wa-eyebrow">
                      Sources · {product.sources.length}
                    </span>
                    {product.sources.map((s, idx) => (
                      <SourceRow
                        key={idx}
                        source={s}
                        currency={product.currency}
                        proposed={product.proposedPrice}
                      />
                    ))}
                  </Stack>
                )}
              </>
            )}

            {/* Per-row action footer. Mirrors the marketing batch-row footer
                shape (BatchReview.tsx line ~722). Only renders when both
                handlers are provided so the card stays usable in read-only
                contexts. */}
            {(onApprove || onReject) && (
              <Stack
                direction="row"
                gap="sm"
                justify="flex-end"
                align="center"
                style={{
                  paddingTop: 'var(--wpds-dimension-gap-sm)',
                  borderTop:
                    'var(--wpds-border-width-sm) solid var(--wpds-color-stroke-surface-neutral-weak)',
                }}
              >
                {product.status !== 'in_review' ? (
                  <Text
                    variant="body-sm"
                    style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
                  >
                    {`Already ${product.status === 'done' ? 'approved' : product.status}`}
                  </Text>
                ) : (
                  <>
                    {onReject && (
                      <Button
                        __next40pxDefaultSize
                        variant="tertiary"
                        isDestructive
                        onClick={() => onReject(product.issueId)}
                        disabled={!reviewable || busy !== null}
                      >
                        {busy === `reject-row:${product.issueId}`
                          ? 'Dismissing…'
                          : 'Dismiss'}
                      </Button>
                    )}
                    {onApprove && (
                      <Button
                        __next40pxDefaultSize
                        variant="secondary"
                        onClick={() => onApprove(product.issueId)}
                        disabled={!reviewable || busy !== null}
                      >
                        {busy === `approve-row:${product.issueId}`
                          ? 'Applying…'
                          : 'Approve'}
                      </Button>
                    )}
                  </>
                )}
              </Stack>
            )}
          </Stack>
        </Card.Content>
      )}
    </Card.Root>
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
