import { Stack, Text } from '@wordpress/ui';
import { Button } from '@wordpress/components';

interface Props {
  productBound: boolean;
  busy: 'approve' | 'reject' | null;
  disabled: boolean;
  /** Currently selected variant id (e.g., "A"). Null when the proposal has
   *  no variants; the bar shows "A" as a static label in that case. */
  selectedVariantId?: string | null;
  onApprove: () => void;
  onReject: () => void;
  onCancel: () => void;
}

// The sticky bar pinned to the bottom of the IssueDetail viewport. Wires the
// existing daemon approve/reject endpoints; "Ask the agent" is a phase-2 hook.
export default function ActionBar({
  productBound,
  busy,
  disabled,
  selectedVariantId,
  onApprove,
  onReject,
  onCancel,
}: Props) {
  const variantLabel = selectedVariantId ?? 'A';
  return (
    <div className="wa-action-bar">
      <div className="wa-action-bar-row">
        <Stack direction="row" gap="sm" align="center">
          <span
            style={{
              height: 28,
              width: 28,
              borderRadius: 'var(--wpds-border-radius-md)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontWeight: 700,
              fontSize: 12,
              background: 'var(--wpds-color-bg-surface-info-weak)',
              color: 'var(--wpds-color-fg-interactive-brand)',
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
              {selectedVariantId
                ? `Variant ${selectedVariantId} selected · ${
                    productBound ? 'bound to live product' : 'no live product bound'
                  }`
                : productBound
                  ? 'Variant selected · bound to live product'
                  : 'Variant selected · no live product bound'}
            </Text>
            <Text
              variant="body-sm"
              style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
            >
              Approval will write to WooCommerce via the daemon's MCP ability.
            </Text>
          </Stack>
        </Stack>
        <div className="wa-action-bar-actions">
          <Button variant="tertiary" disabled>
            Ask the agent
          </Button>
          <Button
            variant="tertiary"
            isDestructive
            onClick={onReject}
            disabled={disabled || busy !== null}
          >
            {busy === 'reject' ? 'Rejecting…' : 'Reject all'}
          </Button>
          <Button variant="tertiary" onClick={onCancel}>
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={onApprove}
            disabled={disabled || busy !== null}
          >
            {busy === 'approve' ? 'Applying to store…' : '✓ Approve & apply to store'}
          </Button>
        </div>
      </div>
    </div>
  );
}
