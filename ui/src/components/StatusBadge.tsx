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

// Persona-colored kind pill — see CLAUDE.md for the documented exception. The
// daemon's `proposal.type` is the long-term source for kind; for phase 1 every
// marketing-persona issue is "content".
export type IssueKind = 'content' | 'campaign' | 'email';

const KIND_LABEL: Record<IssueKind, string> = {
  content: 'Content',
  campaign: 'Campaign',
  email: 'Email',
};

export function KindBadge({ kind }: { kind: IssueKind }) {
  return (
    <Badge
      style={{
        background: 'var(--wa-persona-mk-bg)',
        color: 'var(--wa-persona-mk-ink)',
      }}
    >
      {KIND_LABEL[kind]}
    </Badge>
  );
}

// Map daemon Issue.persona/proposal type to a UI "kind" so we can render the
// right pill on cards. Phase-1 default: every marketing-persona issue maps to
// content. Extend this when other personas come online.
export function kindFromIssue(issue: Issue): IssueKind {
  if (issue.persona === 'marketing') return 'content';
  return 'content';
}
