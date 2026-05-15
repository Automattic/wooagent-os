import type { ReactNode } from 'react';
import { Stack, Text } from '@wordpress/ui';

interface Props {
  /** Small-cap eyebrow label (e.g., "Current description"). */
  eyebrow?: string;
  /** Larger heading line (e.g., "Proposed · pick one"). */
  title?: ReactNode;
  /** Inline badge or pill rendered alongside the eyebrow (e.g., bound state). */
  badge?: ReactNode;
  /** Right-aligned text content (e.g., "76 chars"). */
  meta?: ReactNode;
  /** Right-aligned action element (e.g., a Regenerate Button). Takes priority over `meta`. */
  action?: ReactNode;
}

// CUSTOM: shared section-header composite. (a) WPDS has no equivalent
// component; the recurring shape (eyebrow + inline badge | meta/action) is
// repeated across IssueDetail's Price, Message, and Marketing views.
// (b) It composes three WPDS primitives — Stack, eyebrow span, Text.
// (c) Documented in DESIGN.md Component inventory.
//
// When `eyebrow` (and optionally `badge`) is set, that pair occupies the left
// of the single horizontal row. When only `title` is set, the title becomes
// the left slot. When both are set, the title stacks below the eyebrow row.
export default function SectionHeader({ eyebrow, title, badge, meta, action }: Props) {
  const rightSlot = action ?? meta;
  const hasEyebrowRow = Boolean(eyebrow || badge);
  const titleEl = title ? (
    <Text
      variant="heading-md"
      style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
    >
      {title}
    </Text>
  ) : null;

  return (
    <Stack direction="column" gap="xs" style={{ width: '100%' }}>
      <Stack direction="row" justify="space-between" align="center" style={{ width: '100%' }}>
        <Stack direction="row" gap="sm" align="center">
          {eyebrow && <span className="wa-eyebrow">{eyebrow}</span>}
          {badge}
          {!hasEyebrowRow && titleEl}
        </Stack>
        {rightSlot}
      </Stack>
      {hasEyebrowRow && titleEl}
    </Stack>
  );
}
