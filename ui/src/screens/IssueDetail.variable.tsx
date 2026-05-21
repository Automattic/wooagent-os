import { Card, Stack, Text, Badge } from '@wordpress/ui';
import { Button, Spinner } from '@wordpress/components';
import { chevronDown, chevronUp } from '@wordpress/icons';
import { useState } from 'react';

import type {
  IssueDetail as IssueDetailPayload,
  Proposal,
  VariablePriceProposal,
  VariationPriceRow,
} from '../api/client';

interface VariableViewProps {
  issue: IssueDetailPayload['issue'];
  proposal: VariablePriceProposal;
  rawProposal: Proposal | null;
  rationale: string;
  personaLabel: string;
  actionMsg: { kind: 'info' | 'error'; text: string } | null;
  undoStale: boolean;
  busy: 'approve' | 'reject' | 'undo' | 'ask' | null;
  reviewable: boolean;
  isDone: boolean;
  isArchived: boolean;
  connection: unknown;
  onApprove: () => void;
  onReject: () => void;
  onCancel: () => void;
  onUndo: () => void;
  onView: () => void;
  onAskAgent: () => void;
}

function formatPrice(value: number, currency: string): string {
  const symbol =
    currency === 'USD' || currency === 'CAD' || currency === 'AUD'
      ? '$'
      : currency === 'GBP'
        ? '£'
        : currency === 'EUR'
          ? '€'
          : '';
  return `${symbol}${value.toFixed(2)}`;
}

function formatDelta(percent: number): string {
  const sign = percent > 0 ? '+' : '';
  return `${sign}${percent.toFixed(1)}%`;
}

export function VariablePriceIssueView(props: VariableViewProps) {
  const {
    issue,
    proposal,
    rationale,
    personaLabel,
    actionMsg,
    undoStale,
    busy,
    reviewable,
    isDone,
    isArchived,
    onApprove,
    onReject,
    onCancel,
    onUndo,
    onView,
    onAskAgent,
  } = props;

  const currency = proposal.currency;
  const isReversible = isDone && !isArchived;

  return (
    <Stack gap="xl">
      <Card.Root>
        <Card.Header>
          <Stack gap="md">
            <Text
              variant="body-md"
              style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
            >
              {issue.title}
            </Text>
            <Text
              variant="body-sm"
              style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
            >
              {personaLabel}
            </Text>
          </Stack>
        </Card.Header>
        <Card.Content>
          <Stack direction="row" gap="xl" wrap="wrap">
            <KpiTile label="Variations" value={String(proposal.variationCount)} />
            <KpiTile
              label="Avg change"
              value={formatDelta(proposal.percentChange)}
            />
            <KpiTile
              label="New price range"
              value={`${formatPrice(proposal.proposedPriceMin, currency)} – ${formatPrice(proposal.proposedPriceMax, currency)}`}
            />
            {isReversible && (
              <Stack gap="xs">
                <Text
                  variant="body-sm"
                  style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
                >
                  Status
                </Text>
                <Badge intent="stable">Reversible · always</Badge>
              </Stack>
            )}
          </Stack>
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header>
          <Text
            variant="body-md"
            style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
          >
            Sources & rationale
          </Text>
        </Card.Header>
        <Card.Content>
          <Stack gap="md">
            <Text variant="body-md">{rationale}</Text>
            {proposal.sources.length > 0 && (
              <Stack gap="sm">
                {proposal.sources.map((s) => (
                  <Stack key={s.url} direction="row" gap="sm" align="center">
                    <Text variant="body-sm">
                      {s.retailer ?? new URL(s.url).hostname} · {s.comparable_product} ·{' '}
                      {formatPrice(s.observed_price, s.currency ?? currency)}
                    </Text>
                  </Stack>
                ))}
              </Stack>
            )}
          </Stack>
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header>
          <Text
            variant="body-md"
            style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
          >
            Per-variation prices ({proposal.variations.length})
          </Text>
        </Card.Header>
        <Card.Content>
          <Stack gap="sm">
            {proposal.variations.map((v) => (
              <VariationRow key={v.variationId} variation={v} currency={currency} />
            ))}
          </Stack>
        </Card.Content>
      </Card.Root>

      {actionMsg && (
        <Text
          variant="body-sm"
          style={{
            color:
              actionMsg.kind === 'error'
                ? 'var(--wpds-color-fg-content-error)'
                : 'var(--wpds-color-fg-content-neutral-weak)',
          }}
        >
          {actionMsg.text}
        </Text>
      )}

      <Stack direction="row" gap="md" justify="flex-end">
        {reviewable && (
          <>
            <Button
              variant="secondary"
              onClick={onReject}
              disabled={busy !== null}
              __next40pxDefaultSize
            >
              Dismiss
            </Button>
            <Button
              variant="primary"
              onClick={onApprove}
              disabled={busy !== null}
              __next40pxDefaultSize
            >
              {busy === 'approve' ? (
                <Stack direction="row" gap="sm" align="center">
                  <Spinner style={{ margin: 0 }} />
                  Applying…
                </Stack>
              ) : (
                'Approve & apply prices'
              )}
            </Button>
          </>
        )}
        {isDone && !isArchived && (
          <Button
            variant="secondary"
            onClick={onUndo}
            disabled={busy !== null || undoStale}
            __next40pxDefaultSize
          >
            {busy === 'undo' ? 'Reversing…' : 'Undo'}
          </Button>
        )}
        <Button
          variant="tertiary"
          onClick={onAskAgent}
          disabled={busy !== null}
          __next40pxDefaultSize
        >
          Ask agent
        </Button>
        <Button
          variant="tertiary"
          onClick={onCancel}
          disabled={busy !== null}
          __next40pxDefaultSize
        >
          Back
        </Button>
        <Button
          variant="tertiary"
          onClick={onView}
          disabled={busy !== null}
          __next40pxDefaultSize
        >
          View
        </Button>
      </Stack>
    </Stack>
  );
}

function KpiTile({ label, value }: { label: string; value: string }) {
  return (
    <Stack gap="xs">
      <Text
        variant="body-sm"
        style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
      >
        {label}
      </Text>
      <Text
        variant="body-md"
        style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
      >
        {value}
      </Text>
    </Stack>
  );
}

function VariationRow({
  variation,
  currency,
}: {
  variation: VariationPriceRow;
  currency: string;
}) {
  const [expanded, setExpanded] = useState(false);
  const delta =
    variation.previousPrice === 0
      ? 0
      : ((variation.proposedPrice - variation.previousPrice) / variation.previousPrice) * 100;

  return (
    <Card.Root>
      <Card.Content>
        <Stack direction="row" gap="md" align="center" justify="space-between">
          <Stack direction="row" gap="md" align="center">
            <Button
              icon={expanded ? chevronUp : chevronDown}
              label={expanded ? 'Collapse variation' : 'Expand variation'}
              onClick={() => setExpanded((e) => !e)}
              __next40pxDefaultSize
            />
            <Text
              variant="body-md"
              style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
            >
              {variation.attributesLabel}
            </Text>
            {variation.targetField === 'sale_price' && (
              <Badge intent="informational">sale</Badge>
            )}
            {variation.stockStatus === 'outofstock' && (
              <Badge intent="low">out of stock</Badge>
            )}
          </Stack>
          <Stack direction="row" gap="lg" align="center">
            <Text
              variant="body-sm"
              style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
            >
              {formatPrice(variation.previousPrice, currency)}
            </Text>
            <Text
              variant="body-md"
              style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
            >
              → {formatPrice(variation.proposedPrice, currency)}
            </Text>
            <Text
              variant="body-sm"
              style={{
                color:
                  delta >= 0
                    ? 'var(--wpds-color-fg-content-success)'
                    : 'var(--wpds-color-fg-content-error)',
              }}
            >
              {formatDelta(delta)}
            </Text>
          </Stack>
        </Stack>
        {expanded && (
          <Stack gap="sm" style={{ marginTop: 'var(--wpds-dimension-gap-sm)' }}>
            <Text
              variant="body-sm"
              style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
            >
              Variation {variation.variationId} · target field: {variation.targetField}
            </Text>
            <Text variant="body-sm">
              The benchmark anchor for this variation is{' '}
              {formatPrice(variation.previousPrice, currency)}; applying the
              parent-level {formatDelta(delta)} yields{' '}
              {formatPrice(variation.proposedPrice, currency)}.
            </Text>
          </Stack>
        )}
      </Card.Content>
    </Card.Root>
  );
}
