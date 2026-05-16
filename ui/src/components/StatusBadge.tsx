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
// pick a kind. Used today only for text labels (e.g., "5 of 11 price
// changes pending") and copy in the Dismiss dialog. The persona-colored
// KindBadge was retired when the queue layout moved to DataViews — agent
// identity (PersonaAvatar) is the canonical visual signal now.
export type IssueKind = 'content' | 'campaign' | 'email' | 'price' | 'message';

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

export function kindFromIssue(issue: Issue): IssueKind {
  return kindFromPersonaSlug(issue.persona);
}
