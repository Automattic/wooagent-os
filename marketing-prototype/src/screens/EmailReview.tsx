import { useMemo } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Notice } from '@wordpress/components';
import { useApp } from '../App';
import type { EmailTask } from '../data/types';
import { KindBadge, StatusBadge } from '../components/StatusBadge';

export default function EmailReview() {
  const { id } = useParams<{ id: string }>();
  const { tasks, setTaskStatus, pushToast } = useApp();
  const nav = useNavigate();

  const task = useMemo(
    () => tasks.find((t): t is EmailTask => t.id === id && t.kind === 'email'),
    [tasks, id],
  );

  if (!task) {
    return (
      <main className="max-w-[900px] mx-auto px-6 py-12">
        <Notice status="warning" isDismissible={false}>
          That email isn’t in the queue.{' '}
          <Link to="/" className="underline">
            Back to board
          </Link>
        </Notice>
      </main>
    );
  }

  const onApprove = () => {
    setTaskStatus(task.id, 'done');
    pushToast({
      kind: 'success',
      title: 'Email queued for send',
      body: 'Snapshot saved · revert from Done column.',
    });
    nav('/');
  };

  return (
    <>
      <div className="bg-white border-b border-border">
        <div className="max-w-[1500px] mx-auto px-6 h-12 flex items-center gap-3">
          <Link to="/" className="text-xs text-muted hover:text-ink">
            ← Board
          </Link>
          <div className="h-4 w-px bg-border" />
          <span className="text-xs font-mono tabular text-muted">{task.id}</span>
          <StatusBadge status={task.status} />
          <KindBadge kind="email" />
        </div>
      </div>

      <main className="max-w-[1100px] mx-auto px-6 py-8">
        <div className="mb-6">
          <div className="flex items-center gap-2 mb-2">
            <div className="h-6 w-6 rounded bg-mk-bg text-mk flex items-center justify-center text-[11px] font-bold">
              MK
            </div>
            <div className="text-xs text-muted">
              <span className="font-semibold text-ink">Marketing agent</span>{' '}
              drafted an email
            </div>
            <div className="text-xs text-muted-2">·</div>
            <div className="text-xs tabular font-mono text-muted">
              {task.surfacedAt}
            </div>
          </div>
          <h1
            className="display font-semibold"
            style={{ fontSize: 26, lineHeight: 1.25 }}
          >
            {task.title}
          </h1>
        </div>

        <div className="card overflow-hidden mb-6">
          <div className="px-5 py-3 border-b border-border bg-neutral-50">
            <div className="eyebrow mb-1">Subject</div>
            <div className="text-sm font-semibold">{task.subject}</div>
            <div className="text-xs text-muted mt-1">{task.preview}</div>
          </div>
          <div className="p-8">
            <pre className="content-body whitespace-pre-wrap text-ink leading-relaxed">
              {task.body}
            </pre>
          </div>
        </div>

        <div className="sticky bottom-6 z-20">
          <div className="card p-3 flex items-center gap-3 shadow-sticky">
            <div className="text-xs pl-2 pr-4 border-r border-border">
              <div className="font-medium">Approve & queue for send</div>
              <div className="text-muted">
                Adds to the welcome series; first send 9am tomorrow.
              </div>
            </div>
            <div className="flex-1" />
            <button className="btn btn-danger text-xs" type="button">
              Reject
            </button>
            <button
              className="btn btn-approve"
              type="button"
              onClick={onApprove}
              disabled={task.status !== 'in_review'}
            >
              ✓ Approve email
              <span className="kbd">⌘↵</span>
            </button>
          </div>
        </div>
      </main>
    </>
  );
}
