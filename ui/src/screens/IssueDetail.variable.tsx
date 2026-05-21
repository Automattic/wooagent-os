import { Badge, Card, Notice, Stack, Text } from '@wordpress/ui';
import { Button } from '@wordpress/components';
import { chevronDown, chevronUp } from '@wordpress/icons';
import { useState } from 'react';
import { Page } from '@wordpress/admin-ui';

import type {
  IssueDetail as IssueDetailPayload,
  Proposal,
  VariablePriceProposal,
  VariationPriceRow,
} from '../api/client';
import { formatPrice } from './IssueDetail';
import ActionBar from '../components/ActionBar';
import Breadcrumbs from '../components/Breadcrumbs';
import Kpi from '../components/Kpi';
import PageGlobalActions from '../components/PageGlobalActions';
import ProposalHeader from '../components/ProposalHeader';
import SourceRow from '../components/SourceRow';
import { StatusBadge } from '../components/StatusBadge';

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

function formatDelta(percent: number): string {
  const sign = percent > 0 ? '+' : '';
  return `${sign}${percent.toFixed(1)}%`;
}

export function VariablePriceIssueView(props: VariableViewProps) {
  const { onAskAgent } = props;
  const { issue, proposal, rawProposal, rationale, personaLabel } = props;
  const currency = proposal.currency;
  const scope = proposal.productName ?? '—';

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

  const percentLabel = `${proposal.percentChange >= 0 ? '+' : ''}${proposal.percentChange.toFixed(1)}%`;

  const reviewSummary = `${proposal.variationCount} variations · ${percentLabel} · ${formatPrice(proposal.proposedPriceMin, currency)}–${formatPrice(proposal.proposedPriceMax, currency)}`;
  const doneSummary = `${proposal.variationCount} variations · ${formatPrice(proposal.proposedPriceMin, currency)}–${formatPrice(proposal.proposedPriceMax, currency)} (was ${formatPrice(proposal.previousPriceMin, currency)}–${formatPrice(proposal.previousPriceMax, currency)})`;
  const undoneSummary = `${formatPrice(proposal.previousPriceMin, currency)}–${formatPrice(proposal.previousPriceMax, currency)}`;

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
            verb="proposes a price change across variations"
            timestamp={issue.updated_at}
            modelLine="Claude Haiku 4.5 · web_search"
            title={issue.title}
            description={
              issue.description ??
              `Benchmarked against ${proposal.sources.length} comparable products. Approval writes ${proposal.variationCount} variations.`
            }
            imageUrl={typeof rawProposal?.target?.image_url === 'string' ? rawProposal.target.image_url : undefined}
            imageAlt={typeof rawProposal?.target?.image_alt === 'string' ? rawProposal.target.image_alt : undefined}
          />

          {/* KPI row — variable price tiles */}
          <div
            className="wa-kpi-row"
            style={{ marginBottom: 'var(--wpds-dimension-gap-xl)' }}
          >
            <Kpi
              label="Variations"
              value={proposal.variationCount}
              hint="Across this parent"
              tone="neutral"
            />
            <Kpi
              label="Avg change"
              value={
                <span style={{ color: directionTone.fg }}>{percentLabel}</span>
              }
              hint={`Capped at ±25% per step · ${proposal.direction}`}
            />
            <Kpi
              label="New price range"
              value={`${formatPrice(proposal.proposedPriceMin, currency)} – ${formatPrice(proposal.proposedPriceMax, currency)}`}
              hint={`from ${formatPrice(proposal.previousPriceMin, currency)} – ${formatPrice(proposal.previousPriceMax, currency)}`}
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
              {/* Per-variation prices */}
              <Card.Root>
                <Card.Header>
                  <Stack direction="row" gap="md" align="center">
                    <Text
                      variant="body-md"
                      style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
                    >
                      Per-variation prices
                    </Text>
                    <Badge intent="none">
                      {`${proposal.variationCount} variation${proposal.variationCount === 1 ? '' : 's'}`}
                    </Badge>
                  </Stack>
                </Card.Header>
                <Card.Content>
                  <Stack direction="column" gap="sm">
                    {proposal.variations.map((v) => (
                      <VariationRow key={v.variationId} variation={v} currency={currency} />
                    ))}
                  </Stack>
                </Card.Content>
              </Card.Root>

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
                          proposed={proposal.proposedPriceMin}
                        />
                      ))}
                    </Stack>
                  )}
                </Card.Content>
              </Card.Root>

              {props.actionMsg && (
                <Notice.Root
                  intent={props.actionMsg.kind === 'error' ? 'error' : 'success'}
                >
                  <Notice.Description>{props.actionMsg.text}</Notice.Description>
                </Notice.Root>
              )}

              {props.undoStale && (
                <Notice.Root intent="warning">
                  <Notice.Description>
                    The product was changed after this approval. Inspect it in WooCommerce.
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
              ? undoneSummary
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
          productBound={typeof proposal.productId === 'number'}
          busy={props.busy as 'approve' | 'reject' | null}
          disabled={!props.reviewable}
          priceSummary={reviewSummary}
          reversible
          onApprove={props.onApprove}
          onReject={props.onReject}
          onCancel={props.onCancel}
        />
      )}
    </div>
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
          <Stack direction="column" gap="sm" style={{ marginTop: 'var(--wpds-dimension-gap-sm)' }}>
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
