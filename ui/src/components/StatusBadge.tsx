import { Badge } from '@wordpress/ui';
import type { Issue } from '../api/client';

const STATUS_LABEL: Record<Issue['status'], string> = {
  backlog: 'Backlog',
  todo: 'Drafting',
  in_progress: 'Drafting',
  in_review: 'In Review',
  done: 'Done',
  rejected: 'Rejected',
};

const STATUS_INTENT: Record<
  Issue['status'],
  'none' | 'informational' | 'medium' | 'stable' | 'high'
> = {
  backlog: 'none',
  todo: 'informational',
  in_progress: 'informational',
  in_review: 'medium',
  done: 'stable',
  rejected: 'high',
};

export function StatusBadge({ status }: { status: Issue['status'] }) {
  return <Badge intent={STATUS_INTENT[status]}>{STATUS_LABEL[status]}</Badge>;
}

// Persona-colored kind pill — see CLAUDE.md for the documented exception.
// The daemon's `proposal.type` is the long-term source for kind, but the
// kanban list endpoint doesn't surface proposal so we map by persona for
// list views and reserve proposal-type-driven mapping for detail views.
export type IssueKind = 'content' | 'campaign' | 'email' | 'price' | 'message';

const KIND_LABEL: Record<IssueKind, string> = {
  content: 'Content',
  campaign: 'Campaign',
  email: 'Email',
  price: 'Price',
  message: 'Message',
};

// Each kind borrows one of the persona-color CSS variables documented in
// CLAUDE.md's persona-colors exception. New kinds should reuse an existing
// persona variable (mk/pr/in/ac/rp/ss/cs) rather than introducing new color.
const KIND_PERSONA_VAR: Record<IssueKind, { bg: string; ink: string }> = {
  content: { bg: 'var(--wa-persona-mk-bg)', ink: 'var(--wa-persona-mk-ink)' },
  campaign: { bg: 'var(--wa-persona-mk-bg)', ink: 'var(--wa-persona-mk-ink)' },
  email: { bg: 'var(--wa-persona-mk-bg)', ink: 'var(--wa-persona-mk-ink)' },
  price: { bg: 'var(--wa-persona-pr-bg)', ink: 'var(--wa-persona-pr-ink)' },
  message: { bg: 'var(--wa-persona-ss-bg)', ink: 'var(--wa-persona-ss-ink)' },
};

export function KindBadge({ kind }: { kind: IssueKind }) {
  const { bg, ink } = KIND_PERSONA_VAR[kind];
  return <Badge style={{ background: bg, color: ink }}>{KIND_LABEL[kind]}</Badge>;
}

// Map a persona slug to a UI "kind" for the kanban list. Used by both
// issue cards (which carry persona on the issue) and batch cards (which
// carry persona on the batch).
export function kindFromPersonaSlug(persona: string | undefined): IssueKind {
  switch (persona) {
    case 'pricing':
      return 'price';
    case 'sales-support':
      return 'message';
    case 'marketing':
    default:
      return 'content';
  }
}

// Issue-shape convenience around kindFromPersonaSlug. Detail views should
// prefer proposal.type when present (more precise — e.g., a marketing
// persona could later emit campaign or email proposals).
export function kindFromIssue(issue: Issue): IssueKind {
  return kindFromPersonaSlug(issue.persona);
}

// Detail-view counterpart that prefers the proposal.type when available.
// Falls back to persona-based mapping when the proposal is missing.
export function kindFromProposalType(
  proposalType: string | undefined,
  issue: Issue,
): IssueKind {
  switch (proposalType) {
    case 'product_price_change':
      return 'price';
    case 'product_description_rewrite':
      return 'content';
    case 'customer_reply_draft':
      return 'message';
  }
  return kindFromIssue(issue);
}
