import type { ReactNode } from 'react';
import { Stack, Text } from '@wordpress/ui';

interface Props {
  /** Small-cap eyebrow label (e.g., "Current description"). */
  eyebrow?: string;
  /** Optional larger heading line below or alongside the eyebrow. */
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
// Renders as a single horizontal row. Used inside <Card.Header> (replacing
// its children) and standalone above lists. When `title` is set, it stacks
// above the badge/meta row.
export default function SectionHeader({ eyebrow, title, badge, meta, action }: Props) {
  const rightSlot = action ?? meta;
  return (
    <Stack direction="column" gap="xs" style={{ width: '100%' }}>
      <Stack direction="row" justify="space-between" align="center" style={{ width: '100%' }}>
        <Stack direction="row" gap="sm" align="center">
          {eyebrow && <span className="wa-eyebrow">{eyebrow}</span>}
          {badge}
        </Stack>
        {rightSlot}
      </Stack>
      {title && (
        <Text
          variant="heading-md"
          style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
        >
          {title}
        </Text>
      )}
    </Stack>
  );
}
