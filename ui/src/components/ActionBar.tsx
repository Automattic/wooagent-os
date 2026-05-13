import type { CSSProperties, ReactNode } from 'react';
import { Stack, Text } from '@wordpress/ui';
import { Button } from '@wordpress/components';
import { Icon, check } from '@wordpress/icons';

// Entity discriminator. 'variant' is the prose-rewrite default (Marketing).
// 'price' is the pricing-persona path. 'message' is the sales-support path
// — the primary button reads "Approve & send" and helper text names the
// recipient. Adding a new entity is purely additive: extend the union,
// add labels in DoneBar/ReviewBar, route in IssueDetail.
export type EntityKind = 'variant' | 'price' | 'message';

interface ReviewProps {
  state: 'review';
  /** Defaults to 'variant'. */
  entity?: EntityKind;
  productBound: boolean;
  busy: 'approve' | 'reject' | null;
  disabled: boolean;
  /** Used when entity='variant'. The letter shown in the round badge. */
  selectedVariantId?: string | null;
  /**
   * Used when entity='price'. The primary line, e.g.
   * "$39.00 → $44.99 · +15.4%".
   */
  priceSummary?: string;
  /**
   * Used when entity='message'. The recipient label, e.g.
   * "Maria · maria@example.com" or "Internal note".
   */
  messageRecipient?: string;
  /** Used when entity='message'. 'customer' or 'internal'. */
  messageNoteType?: 'customer' | 'internal';
  onApprove: () => void;
  onReject: () => void;
  onCancel: () => void;
}

interface DoneProps {
  state: 'done';
  /** Defaults to 'variant'. */
  entity?: EntityKind;
  /** When entity='variant': the letter that was approved. */
  variantId?: string;
  /** When entity='price': summary line, e.g. "$44.99 (was $39.00)". */
  priceSummary?: string;
  /** When entity='message': recipient label. */
  messageRecipient?: string;
  /** When entity='message': 'customer' or 'internal'. */
  messageNoteType?: 'customer' | 'internal';
  /** Product / scope label, e.g., "Handwoven Wool Throw - Slate". */
  scope: string;
  onUndo: () => void;
  onView: () => void;
}

type Props = ReviewProps | DoneProps;

const badgeBaseStyle: CSSProperties = {
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
};

// Sticky bar pinned to the bottom of the IssueDetail viewport. Two states
// (review / done) and two entities (variant / price). Done state for
// pricing reads "Price change written to WooCommerce …"; review state for
// pricing names the change and uses "Approve & apply price" instead of
// "Approve & apply to store".
export default function ActionBar(props: Props) {
  if (props.state === 'done') {
    return <DoneBar {...props} />;
  }
  return <ReviewBar {...props} />;
}

function DoneBar(props: DoneProps) {
  const entity = props.entity ?? 'variant';
  let headline: ReactNode;
  if (entity === 'price') {
    headline = (
      <Stack direction="row" gap="sm" align="center" wrap="wrap">
        <Text
          variant="body-sm"
          style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
        >
          Price change written to WooCommerce
        </Text>
        {props.priceSummary && (
          <Text
            variant="body-sm"
            className="wa-mono"
            style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
          >
            {props.priceSummary}
          </Text>
        )}
        <Text
          variant="body-sm"
          style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
        >
          {props.scope}
        </Text>
      </Stack>
    );
  } else if (entity === 'message') {
    const sent =
      props.messageNoteType === 'internal'
        ? 'Internal note added'
        : 'Customer note sent';
    headline = (
      <Stack direction="row" gap="sm" align="center" wrap="wrap">
        <Text
          variant="body-sm"
          style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
        >
          {sent}
        </Text>
        {props.messageRecipient && (
          <Text
            variant="body-sm"
            style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
          >
            {props.messageRecipient}
          </Text>
        )}
        <Text
          variant="body-sm"
          style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
        >
          {props.scope}
        </Text>
      </Stack>
    );
  } else {
    headline = (
      <Stack direction="row" gap="sm" align="center" wrap="wrap">
        <Text
          variant="body-sm"
          style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
        >
          Variant {props.variantId ?? 'A'} written to WooCommerce
        </Text>
        <Text
          variant="body-sm"
          style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
        >
          {props.scope}
        </Text>
      </Stack>
    );
  }
  return (
    <div className="wa-action-bar">
      <div className="wa-action-bar-row">
        <div className="wa-action-bar__left">
          <span className="wa-done-check" aria-hidden="true">
            <Icon icon={check} size={20} />
          </span>
          <Stack direction="column" gap="xs">
            {headline}
            <Text
              variant="body-sm"
              style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
            >
              Just now · snapshot saved · reversible from the Done column
            </Text>
          </Stack>
        </div>
        <div className="wa-action-bar-actions">
          <Button variant="tertiary" __next40pxDefaultSize onClick={props.onUndo}>
            Undo
          </Button>
          <Button variant="primary" __next40pxDefaultSize onClick={props.onView}>
            View in WooCommerce
          </Button>
        </div>
      </div>
    </div>
  );
}

function ReviewBar(props: ReviewProps) {
  const entity = props.entity ?? 'variant';
  const variantLabel = props.selectedVariantId ?? 'A';
  const isPrice = entity === 'price';
  const isMessage = entity === 'message';
  const isInternal = isMessage && props.messageNoteType === 'internal';

  let badge: ReactNode;
  if (isPrice) {
    badge = (
      <span style={badgeBaseStyle} aria-hidden="true">
        $
      </span>
    );
  } else if (isMessage) {
    badge = (
      <span style={badgeBaseStyle} aria-hidden="true">
        {isInternal ? '#' : '✉'}
      </span>
    );
  } else {
    badge = <span style={badgeBaseStyle}>{variantLabel}</span>;
  }

  let primaryLine: string;
  let helperLine: string;
  let approveLabel: string;
  let approveBusy: string;

  if (isPrice) {
    primaryLine = 'Price change ready';
    helperLine = props.productBound
      ? 'Approval will update regular_price on this product'
      : 'no live product bound · approval will write via the dev proxy';
    approveLabel = 'Approve & apply price';
    approveBusy = 'Applying price…';
  } else if (isMessage) {
    primaryLine = isInternal ? 'Internal note ready' : 'Customer note ready';
    helperLine = isInternal
      ? 'Approval will save this note to wp-admin · not visible to the customer'
      : props.messageRecipient
        ? `Approval will email this note to ${props.messageRecipient}`
        : 'Approval will email this note to the customer';
    approveLabel = isInternal ? 'Approve & save note' : 'Approve & send';
    approveBusy = isInternal ? 'Saving…' : 'Sending…';
  } else {
    primaryLine = `Variant ${variantLabel} selected`;
    helperLine = props.productBound
      ? 'Approval will write to WooCommerce via WooAgent'
      : 'no live product bound · approval will write via the dev proxy';
    approveLabel = 'Approve & apply to store';
    approveBusy = 'Applying to store…';
  }

  return (
    <div className="wa-action-bar">
      <div className="wa-action-bar-row">
        <div className="wa-action-bar__left">
          {badge}
          <Stack direction="column" gap="xs">
            <Stack direction="row" gap="sm" align="center" wrap="wrap">
              <Text
                variant="body-sm"
                style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
              >
                {primaryLine}
              </Text>
              {isPrice && props.priceSummary && (
                <Text
                  variant="body-sm"
                  className="wa-mono"
                  style={{ color: 'var(--wpds-color-fg-content-neutral)' }}
                >
                  {props.priceSummary}
                </Text>
              )}
              {isMessage && props.messageRecipient && !isInternal && (
                <Text
                  variant="body-sm"
                  style={{ color: 'var(--wpds-color-fg-content-neutral)' }}
                >
                  {props.messageRecipient}
                </Text>
              )}
            </Stack>
            <Text
              variant="body-sm"
              style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
            >
              {helperLine}
            </Text>
          </Stack>
        </div>
        <div className="wa-action-bar-actions">
          <span className="wa-reversible-pill">
            <span
              aria-hidden="true"
              style={{
                height: 6,
                width: 6,
                borderRadius: '50%',
                background: 'currentColor',
                display: 'inline-block',
              }}
            />
            Reversible · always
          </span>
          <Button
            __next40pxDefaultSize
            variant="tertiary"
            isDestructive
            onClick={props.onReject}
            disabled={props.disabled || props.busy !== null}
          >
            {isPrice || isMessage ? 'Dismiss' : 'Dismiss all'}
          </Button>
          <Button variant="tertiary" __next40pxDefaultSize onClick={props.onCancel}>
            Cancel
          </Button>
          <Button
            __next40pxDefaultSize
            variant="primary"
            onClick={props.onApprove}
            disabled={props.disabled || props.busy !== null}
          >
            {props.busy === 'approve' ? approveBusy : approveLabel}
          </Button>
        </div>
      </div>
    </div>
  );
}
