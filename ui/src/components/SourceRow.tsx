import { Stack, Text } from '@wordpress/ui';
import { Button } from '@wordpress/components';
import { Icon, arrowUpRight } from '@wordpress/icons';
import type { PriceSource } from '../api/client';
import { formatPrice } from '../screens/IssueDetail';

interface SourceRowProps {
  source: PriceSource;
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
        background: 'var(--wpds-color-background-surface-neutral-weak)',
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
                background: 'var(--wpds-color-background-surface-neutral-weak)',
                color: 'var(--wpds-color-foreground-content-neutral)',
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
              color: 'var(--wpds-color-foreground-content-neutral-weak)',
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
            color: 'var(--wpds-color-foreground-content-neutral-weak)',
            fontSize: 'var(--wpds-typography-font-size-xs)',
          }}
        >
          {diffStr}
        </Text>
      </Stack>
    </Stack>
  );
}

export default SourceRow;
