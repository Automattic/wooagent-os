import { Badge } from '@wordpress/ui';
import type { TaskKind, TaskStatus } from '../data/types';

const STATUS_LABEL: Record<TaskStatus, string> = {
  backlog: 'Backlog',
  drafting: 'Drafting',
  in_review: 'In Review',
  done: 'Done',
};

const STATUS_INTENT: Record<
  TaskStatus,
  'none' | 'informational' | 'medium' | 'stable'
> = {
  backlog: 'none',
  drafting: 'informational',
  in_review: 'medium',
  done: 'stable',
};

const KIND_LABEL: Record<TaskKind, string> = {
  content: 'Content',
  campaign: 'Campaign',
  email: 'Email',
};

// Each kind is owned by an agent persona. Today only the marketing agent emits
// content / campaign / email work, so all three currently render in mk-pink.
// When other agents come online (pricing -> "discount", etc.) the mapping
// extends to the corresponding --wa-persona-* variables.
const KIND_PERSONA: Record<TaskKind, { bg: string; ink: string }> = {
  content: { bg: 'var(--wa-persona-mk-bg)', ink: 'var(--wa-persona-mk-ink)' },
  campaign: { bg: 'var(--wa-persona-mk-bg)', ink: 'var(--wa-persona-mk-ink)' },
  email: { bg: 'var(--wa-persona-mk-bg)', ink: 'var(--wa-persona-mk-ink)' },
};

export function StatusBadge({ status }: { status: TaskStatus }) {
  return <Badge intent={STATUS_INTENT[status]}>{STATUS_LABEL[status]}</Badge>;
}

// Persona-colored kind pill — the documented exception to "WPDS intents only"
// (see CLAUDE.md). Inline-styled because Badge's `intent` palette doesn't
// expose pink/teal/etc. and we don't want a parallel theming system.
export function KindBadge({ kind }: { kind: TaskKind }) {
  const colors = KIND_PERSONA[kind];
  return (
    <Badge
      style={{ background: colors.bg, color: colors.ink }}
    >
      {KIND_LABEL[kind]}
    </Badge>
  );
}
