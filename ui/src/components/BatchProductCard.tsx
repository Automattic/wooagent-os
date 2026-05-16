import { useState } from 'react';
import { Card, Stack, Text } from '@wordpress/ui';
import { Icon, chevronDown, chevronUp } from '@wordpress/icons';
import SectionHeader from './SectionHeader';
import SourceRow from './SourceRow';
import ProductThumbnail from './ProductThumbnail';
import type { PriceSource } from '../api/client';

export interface BatchProduct {
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
}

interface Props {
  product: BatchProduct;
  defaultExpanded?: boolean;
}

// CUSTOM: expandable product card for the pricing batch review. (a) WPDS
// has no disclosure / accordion component; we compose Card.Root +
// SectionHeader + a chevron toggle button. (b) The collapsed state is a
// single horizontal row so 10+ rows can be skimmed without scroll.
// (c) Documented in DESIGN.md Component inventory.
export default function BatchProductCard({ product, defaultExpanded = false }: Props) {
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
