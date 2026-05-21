import type { CSSProperties, ReactNode } from 'react';
import { Badge, Stack, Text } from '@wordpress/ui';
import { Button } from '@wordpress/components';

// CUSTOM: outlined check-circle icon for the DoneBar success affordance.
// (a) @wordpress/icons exposes a bare `check` glyph but no
// check-in-circle variant, and the Figma (5TXGXZEejxJ4sRejFyjLCC, node
// 2-9274 "Icon/ActionBar/Sucess") specifies an outlined circle with the
// check inside. (b) Inline SVG with `currentColor` so it picks up the
// success foreground token via .wa-done-check. (c) Documented in the
// IssueDetail Done-state spec in the Figma file above.
function CheckCircleIcon() {
  return (
    <svg
      width="24"
      height="24"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <circle cx="12" cy="12" r="10" />
      <path d="m8 12 3 3 5-6" />
    </svg>
  );
}

// CUSTOM: outlined u-turn arrow for the DoneBar undone-state affordance.
// (a) @wordpress/icons exposes only forward/back chevrons and a curved
// arrow that visually reads as "redo" not "undo." (b) Inline SVG so it
// picks up the neutral foreground token via .wa-undo-arrow. (c) Used
// only in DoneBar's undone-state — documented in the IssueDetail
// undo-affordance spec (~/Documents/wooagent-internal/2026-05-20-
// undo-and-sale-price-design.md).
function UndoArrowIcon() {
  return (
    <svg
      width="24"
      height="24"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M9 14l-4-4 4-4" />
      <path d="M5 10h11a4 4 0 010 8h-2" />
    </svg>
  );
}

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

interface ArchivedProps {
  state: 'archived';
  /** RFC3339 timestamp when the proposal was dismissed. Drives the
   *  "will be deleted on [date]" line — 30 days after dismissedAt. When
   *  absent, the line falls back to "soon." */
  dismissedAt?: string;
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
  /** RFC3339 timestamp when the operator clicked Undo. When set, the
   *  DoneBar renders the undone state (neutral glyph, "reverted"
   *  headline, no Undo button — only View). */
  undoneAt?: string;
  /** When 'undo', the Undo button is disabled to prevent double-click
   *  firing two POSTs in flight. */
  busy?: 'undo' | null;
  onUndo: () => void;
  onView: () => void;
}

type Props = ReviewProps | DoneProps | ArchivedProps;

// Used only for the non-variant badges (price `$`, message `✉` / `#`). The
// variant badge below uses the shared `.wa-variant-letter` class so the
// A/B/C circle stays in lockstep with the one on each variant card.
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
  if (props.state === 'archived') {
    return <ArchivedBar {...props} />;
  }
  return <ReviewBar {...props} />;
}

// Footer for dismissed/rejected proposals. No actions yet — the operator
// returns to the board via the breadcrumb. The deletion date is calculated
// client-side from dismissed_at + 30 days; the actual sweep job that
// honors the promise is DSGWOO-1350.
function ArchivedBar(props: ArchivedProps) {
  const deletionDate = (() => {
    const anchor = props.dismissedAt ? new Date(props.dismissedAt) : null;
    if (!anchor || Number.isNaN(anchor.getTime())) return null;
    anchor.setDate(anchor.getDate() + 30);
    return anchor.toLocaleDateString(undefined, {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
    });
  })();
  return (
    <div className="wa-action-bar">
      <div className="wa-action-bar-row">
        <div className="wa-action-bar__left">
          <Stack direction="column" gap="xs">
            <Text
              variant="body-sm"
              style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
            >
              Dismissed and archived
            </Text>
            <Text
              variant="body-sm"
              style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
            >
              {deletionDate
                ? `This proposal will be deleted on ${deletionDate} unless restored.`
                : 'This proposal will be deleted soon unless restored.'}
            </Text>
          </Stack>
        </div>
      </div>
    </div>
  );
}

function DoneBar(props: DoneProps) {
  const entity = props.entity ?? 'variant';
  const isUndone = !!props.undoneAt;
  const isMessage = entity === 'message';

  let headlineText: string;
  if (isUndone) {
    if (entity === 'price') {
      // priceSummary in undone mode is the value we reverted TO, e.g. "$35".
      headlineText = props.priceSummary
        ? `Price reverted to ${props.priceSummary}`
        : 'Price reverted';
    } else if (entity === 'message') {
      // Messages aren't undoable; this branch is defensive.
      headlineText = 'Reverted';
    } else {
      headlineText = 'Description reverted';
    }
  } else if (entity === 'price') {
    headlineText = 'Price change written to WooCommerce';
  } else if (entity === 'message') {
    headlineText =
      props.messageNoteType === 'internal'
        ? 'Internal note added'
        : 'Customer note sent';
  } else {
    headlineText = `Variant ${variantLetterFromId(props.variantId)} written to WooCommerce`;
  }

  return (
    <div className="wa-action-bar">
      <div className="wa-action-bar-row">
        <div className="wa-action-bar__left">
          <span
            className={isUndone ? 'wa-undo-arrow' : 'wa-done-check'}
            aria-hidden="true"
          >
            {isUndone ? <UndoArrowIcon /> : <CheckCircleIcon />}
          </span>
          <Text
            variant="body-sm"
            style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
          >
            {headlineText}
          </Text>
        </div>
        <div className="wa-action-bar-actions">
          {!isUndone && !isMessage && (
            <Button
              variant="tertiary"
              __next40pxDefaultSize
              onClick={props.onUndo}
              disabled={props.busy === 'undo'}
            >
              {props.busy === 'undo' ? 'Undoing…' : 'Undo'}
            </Button>
          )}
          <Button variant="primary" __next40pxDefaultSize onClick={props.onView}>
            View in WooCommerce
          </Button>
        </div>
      </div>
    </div>
  );
}

// Variant ids come from the daemon as `var_a` / `var_b` / `var_c`. The
// footer only needs the trailing letter, uppercased — matches the
// .wa-variant-letter circle on each variant card.
function variantLetterFromId(id: string | null | undefined): string {
  if (!id) return 'A';
  return (id.split('_').pop() ?? id).slice(0, 1).toUpperCase();
}

function ReviewBar(props: ReviewProps) {
  const entity = props.entity ?? 'variant';
  const variantLabel = variantLetterFromId(props.selectedVariantId);
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
    badge = (
      <span
        className="wa-variant-letter wa-variant-letter--lg"
        data-tone={variantLabel.toUpperCase()}
        aria-hidden="true"
      >
        {variantLabel}
      </span>
    );
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
          <Badge intent="none">Reversible · always</Badge>
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
