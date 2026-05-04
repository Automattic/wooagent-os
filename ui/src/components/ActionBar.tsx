import { Stack, Text } from '@wordpress/ui';
import { Button } from '@wordpress/components';
import { Icon, check } from '@wordpress/icons';

interface ReviewProps {
  state: 'review';
  productBound: boolean;
  busy: 'approve' | 'reject' | null;
  disabled: boolean;
  selectedVariantId?: string | null;
  onApprove: () => void;
  onReject: () => void;
  onCancel: () => void;
}

interface DoneProps {
  state: 'done';
  /** Variant that was approved. */
  variantId: string;
  /** Product / scope label, e.g., "Handwoven Wool Throw - Slate". */
  scope: string;
  onUndo: () => void;
  onView: () => void;
}

type Props = ReviewProps | DoneProps;

// Sticky bar pinned to the bottom of the IssueDetail viewport. Two states:
// review (default — Reject / Cancel / Approve) and done (post-approval —
// Undo / View in WooCommerce). The state is driven by the parent based on
// the issue's current status.
export default function ActionBar(props: Props) {
  if (props.state === 'done') {
    return (
      <div className="wa-action-bar">
        <div className="wa-action-bar-row">
          <div className="wa-action-bar__left">
            <span className="wa-done-check" aria-hidden="true">
              <Icon icon={check} size={20} />
            </span>
            <Stack direction="column" gap="xs">
              <Stack direction="row" gap="sm" align="center" wrap="wrap">
                <Text
                  variant="body-sm"
                  style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
                >
                  Variant {props.variantId} written to WooCommerce
                </Text>
                <Text
                  variant="body-sm"
                  style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
                >
                  {props.scope}
                </Text>
              </Stack>
              <Text
                variant="body-sm"
                style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
              >
                Just now · snapshot saved · reversible from the Done column
              </Text>
            </Stack>
          </div>
          <div className="wa-action-bar-actions">
            <Button variant="tertiary" onClick={props.onUndo}>
              Undo
            </Button>
            <Button variant="primary" onClick={props.onView}>
              View in WooCommerce
            </Button>
          </div>
        </div>
      </div>
    );
  }

  const variantLabel = props.selectedVariantId ?? 'A';
  return (
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
              fontSize: 12,
              background: 'var(--wpds-color-bg-interactive-brand-strong)',
              color: '#ffffff',
              flex: 'none',
            }}
          >
            {variantLabel}
          </span>
          <Stack direction="column" gap="xs">
            <Text
              variant="body-sm"
              style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
            >
              Variant {variantLabel} selected
            </Text>
            <Text
              variant="body-sm"
              style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
            >
              {props.productBound
                ? 'Approval will write to WooCommerce via the daemon'
                : 'no live product bound · approval will write via the dev proxy'}
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
            variant="tertiary"
            isDestructive
            onClick={props.onReject}
            disabled={props.disabled || props.busy !== null}
          >
            {props.busy === 'reject' ? 'Rejecting…' : 'Reject all'}
          </Button>
          <Button variant="tertiary" onClick={props.onCancel}>
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={props.onApprove}
            disabled={props.disabled || props.busy !== null}
          >
            {props.busy === 'approve' ? 'Applying to store…' : 'Approve & apply to store'}
          </Button>
        </div>
      </div>
    </div>
  );
}
