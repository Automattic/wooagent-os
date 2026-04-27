import { useNavigate } from 'react-router-dom';
import { useApp } from '../App';
import type { Task, TaskStatus } from '../data/types';
import { KindBadge } from '../components/StatusBadge';

const COLUMNS: { status: TaskStatus; label: string; hint: string }[] = [
  { status: 'backlog', label: 'Backlog', hint: 'queued · agent will draft' },
  { status: 'drafting', label: 'Drafting', hint: 'agent working now' },
  { status: 'in_review', label: 'In Review', hint: 'needs your call' },
  { status: 'done', label: 'Done', hint: 'shipped · reversible' },
];

function routeFor(t: Task): string {
  return `/issues/${t.id}/${t.kind}`;
}

export default function Kanban() {
  const { tasks } = useApp();
  const nav = useNavigate();

  return (
    <main className="max-w-[1500px] mx-auto px-6 py-8">
      <div className="mb-6 flex items-end justify-between gap-6">
        <div>
          <div className="eyebrow !text-mk">Marketing agent · Today</div>
          <h1 className="display text-3xl font-semibold mt-2">
            Today's marketing queue.
          </h1>
          <p className="text-sm text-muted mt-2 max-w-2xl leading-relaxed">
            What the marketing agent has staged for you. Approve in review,
            adjust the queue, or let it work. Every shipped change is
            reversible.
          </p>
        </div>
        <div className="flex items-center gap-3 text-xs text-muted">
          <div className="flex items-center gap-1.5">
            <span className="h-2 w-2 rounded-full bg-ok inline-block" />
            Agent online
          </div>
          <div className="font-mono tabular">last scan · 8 min ago</div>
        </div>
      </div>

      <div className="grid grid-cols-4 gap-4">
        {COLUMNS.map((col) => {
          const colTasks = tasks.filter((t) => t.status === col.status);
          return (
            <section
              key={col.status}
              className="card p-3 flex flex-col gap-2 min-h-[60vh]"
              style={{ background: '#FBFBFC' }}
            >
              <header className="px-2 pt-1 pb-2 flex items-baseline justify-between">
                <div>
                  <div className="text-sm font-semibold">{col.label}</div>
                  <div className="eyebrow mt-0.5">{col.hint}</div>
                </div>
                <div className="text-xs tabular font-mono text-muted">
                  {colTasks.length}
                </div>
              </header>

              {colTasks.length === 0 && (
                <div className="text-xs text-muted px-2 py-3">No items.</div>
              )}

              {colTasks.map((t) => (
                <button
                  key={t.id}
                  type="button"
                  onClick={() => nav(routeFor(t))}
                  className="card text-left hover:border-primary hover:shadow transition p-3"
                >
                  <div className="flex items-center gap-2 mb-2">
                    <KindBadge kind={t.kind} />
                    <span className="text-[10px] tabular font-mono text-muted">
                      {t.id}
                    </span>
                  </div>
                  <div className="text-[13px] font-semibold leading-snug text-ink">
                    {t.title}
                  </div>
                  <div className="text-[11px] text-muted mt-2 flex items-center justify-between">
                    <span>
                      {t.kind === 'content' &&
                        (t.variants.length > 0
                          ? `${t.variants.length} variants`
                          : 'no variants yet')}
                      {t.kind === 'campaign' &&
                        `${t.children.length} child tasks`}
                      {t.kind === 'email' && t.subject}
                    </span>
                    <span className="font-mono tabular">{t.surfacedAt}</span>
                  </div>
                </button>
              ))}
            </section>
          );
        })}
      </div>
    </main>
  );
}
