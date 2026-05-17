import { Badge } from '@wordpress/ui';
import type { Issue } from '../api/client';

const STATUS_LABEL: Record<Issue['status'], string> = {
  backlog: 'Backlog',
  todo: 'Drafting',
  in_progress: 'Drafting',
  in_review: 'In Review',
  done: 'Done',
  rejected: 'Dismissed',
  dismissed: 'Dismissed',
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
  rejected: 'none',
  dismissed: 'none',
};

export function StatusBadge({ status }: { status: Issue['status'] }) {
  return <Badge intent={STATUS_INTENT[status]}>{STATUS_LABEL[status]}</Badge>;
}

// Issue "kind" — a coarse-grained categorization derived from the persona
// slug. Kept here (rather than as a daemon-side enum) because the kanban
// list endpoint doesn't surface proposal.type; we lean on the persona to
// pick a kind. Used for text labels in batch counters ("5 of 11 price
// changes pending"), the Dismiss dialog copy, and the queue card's
// Proposal field. The persona-colored KindBadge was retired when the
// queue layout moved to DataViews — agent identity (PersonaAvatar) is
// the canonical visual signal now.
//
// Phase 1 personas (marketing / pricing / sales-support) actually emit
// issues today. The remaining four (inventory / accounting / reporting /
// chief) are wired in for forward-compatibility so the Proposal field
// has a label ready when those personas start emitting work.
export type IssueKind =
  | 'content'
  | 'campaign'
  | 'email'
  | 'price'
  | 'message'
  | 'inventory'
  | 'accounting'
  | 'report'
  | 'summary';

export function kindFromPersonaSlug(persona: string | undefined): IssueKind {
  switch (persona) {
    case 'pricing':
      return 'price';
    case 'sales-support':
    case 'sales_support':
      return 'message';
    case 'inventory':
      return 'inventory';
    case 'accounting':
      return 'accounting';
    case 'reporting':
      return 'report';
    case 'chief':
    case 'chief-of-staff':
      return 'summary';
    case 'marketing':
    default:
      return 'content';
  }
}

export function kindFromIssue(issue: Issue): IssueKind {
  return kindFromPersonaSlug(issue.persona);
}
