// Shared horizontal market-range bar with a marker for the proposed price.
// Used by the single-product price view (PriceIssueView) and by the
// variable-batch per-variation row (BatchProductCard).
//
// Not a real chart — a token-styled sparkline-equivalent that gives the
// operator instant visual context for "is the proposed price inside the
// observed band?".

import { Stack, Text } from '@wordpress/ui';
import { formatPrice } from '../screens/IssueDetail';

interface Props {
  low: number;
  median?: number;
  high: number;
  proposed: number;
  currency: string;
}

export default function ObservedRange({ low, median, high, proposed, currency }: Props) {
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
