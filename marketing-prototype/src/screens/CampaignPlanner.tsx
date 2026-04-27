import { useMemo } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Notice } from '@wordpress/components';
import { useApp } from '../App';
import type { CampaignTask } from '../data/types';
import { KindBadge, StatusBadge } from '../components/StatusBadge';

export default function CampaignPlanner() {
  const { id } = useParams<{ id: string }>();
  const { tasks, setTaskStatus, pushToast } = useApp();
  const nav = useNavigate();

  const task = useMemo(
    () => tasks.find((t): t is CampaignTask => t.id === id && t.kind === 'campaign'),
    [tasks, id],
  );

  if (!task) {
    return (
      <main className="max-w-[900px] mx-auto px-6 py-12">
        <Notice status="warning" isDismissible={false}>
          That campaign isn’t in the queue.{' '}
          <Link to="/" className="underline">
            Back to board
          </Link>
        </Notice>
      </main>
    );
  }

  const onApprovePlan = () => {
    setTaskStatus(task.id, 'drafting');
    pushToast({
      kind: 'success',
      title: 'Plan approved · child tasks scheduled',
      body: `${task.children.length} components are queued. Each one still hits In Review when drafted — approving the plan doesn't ship anything.`,
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
          <KindBadge kind="campaign" />
        </div>
      </div>

      <main className="max-w-[1200px] mx-auto px-6 py-8">
        <div className="mb-6">
          <div className="flex items-center gap-2 mb-2">
            <div className="h-6 w-6 rounded bg-mk-bg text-mk flex items-center justify-center text-[11px] font-bold">
              MK
            </div>
            <div className="text-xs text-muted">
              <span className="font-semibold text-ink">Marketing agent</span>{' '}
              proposes a plan
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
          <p className="text-sm mt-2 max-w-2xl text-ink-soft">
            One strategic decision: is this the right plan? If yes, the agent
            schedules the components and brings each one back through Review as
            it’s drafted.
          </p>
        </div>

        <div
          className="grid"
          style={{ gridTemplateColumns: '1fr 320px', gap: 20 }}
        >
          <main>
            <div className="card p-5 mb-4">
              <div className="eyebrow mb-3">Brief</div>
              <div className="grid grid-cols-2 gap-4 text-sm">
                <div>
                  <div className="text-xs text-muted mb-1">Goal</div>
                  <div className="text-ink-soft leading-relaxed">
                    {task.goal}
                  </div>
                </div>
                <div>
                  <div className="text-xs text-muted mb-1">Audience</div>
                  <div className="text-ink-soft leading-relaxed">
                    {task.audience}
                  </div>
                </div>
                <div>
                  <div className="text-xs text-muted mb-1">Window</div>
                  <div className="text-ink-soft font-mono tabular">
                    {task.startsOn} → {task.endsOn}
                  </div>
                </div>
                <div>
                  <div className="text-xs text-muted mb-1">Budget</div>
                  <div className="text-ink-soft font-mono tabular">
                    {task.budget}
                  </div>
                </div>
              </div>
            </div>

            <div className="card overflow-hidden">
              <div className="px-5 py-3 border-b border-border flex items-center justify-between bg-neutral-50">
                <div className="eyebrow">Proposed components · {task.children.length}</div>
                <div className="text-xs text-muted">
                  Each drafts on its own schedule, returns to In Review.
                </div>
              </div>
              <div className="divide-y divide-border">
                {task.children.map((c, i) => (
                  <div
                    key={c.id}
                    className="px-5 py-3.5 flex items-center gap-3 hover:bg-neutral-50"
                  >
                    <div className="h-7 w-7 rounded-full flex items-center justify-center text-xs font-bold tabular bg-mk-bg text-mk flex-none">
                      {i + 1}
                    </div>
                    <div className="flex-1 min-w-0">
                      <div className="text-sm font-medium text-ink">
                        {c.title}
                      </div>
                      <div className="text-[11px] tabular font-mono text-muted">
                        {c.id} · {c.scheduledFor}
                      </div>
                    </div>
                    <StatusBadge status={c.status} />
                  </div>
                ))}
              </div>
            </div>
          </main>

          <aside className="space-y-4">
            <div className="card p-5">
              <div className="eyebrow mb-2 !text-primary">Why split like this</div>
              <p className="text-sm text-ink-soft leading-relaxed">
                A campaign has many pieces but you only make one strategic
                decision: <span className="font-semibold">is this the right plan?</span>{' '}
                The rest are executions.
              </p>
            </div>

            <div className="rounded-lg p-4 border bg-warn-bg border-warn-border">
              <div className="font-semibold text-xs mb-1 text-warn-strong">
                Failsafe
              </div>
              <div
                className="text-xs leading-relaxed"
                style={{ color: '#713F12' }}
              >
                Approving the plan doesn’t auto-ship anything. Every child
                returns through In Review when drafted.
              </div>
            </div>
          </aside>
        </div>

        <div className="sticky bottom-6 mt-6 z-20">
          <div className="card p-3 flex items-center gap-3 shadow-sticky">
            <div className="text-xs pl-2 pr-4 border-r border-border">
              <div className="font-medium">Approve the plan</div>
              <div className="text-muted">
                Schedules {task.children.length} child tasks · no content ships
                without your review.
              </div>
            </div>
            <div className="flex-1" />
            <button className="btn btn-ghost text-xs" type="button">
              Modify
            </button>
            <button className="btn btn-danger text-xs" type="button">
              Reject
            </button>
            <button
              className="btn btn-approve"
              type="button"
              onClick={onApprovePlan}
              disabled={task.status !== 'in_review'}
            >
              ✓ Approve plan
              <span className="kbd">⌘↵</span>
            </button>
          </div>
        </div>
      </main>
    </>
  );
}
