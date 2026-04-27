import type { TaskKind, TaskStatus } from '../data/types';

const STATUS_LABEL: Record<TaskStatus, string> = {
  backlog: 'Backlog',
  drafting: 'Drafting',
  in_review: 'In Review',
  done: 'Done',
};

const KIND_LABEL: Record<TaskKind, string> = {
  content: 'Content',
  campaign: 'Campaign',
  email: 'Email',
};

export function StatusBadge({ status }: { status: TaskStatus }) {
  const cls =
    status === 'in_review'
      ? 'tag-warn'
      : status === 'done'
        ? 'tag-ok'
        : status === 'drafting'
          ? 'tag-info'
          : 'tag-neutral';
  return (
    <span className={`tag ${cls} eyebrow`} style={{ fontSize: 9 }}>
      {STATUS_LABEL[status]}
    </span>
  );
}

export function KindBadge({ kind }: { kind: TaskKind }) {
  return (
    <span className="tag tag-mk eyebrow" style={{ fontSize: 9 }}>
      {KIND_LABEL[kind]}
    </span>
  );
}
