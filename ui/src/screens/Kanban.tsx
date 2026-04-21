import { useEffect, useState } from 'react';
import { Panel, PanelBody, Notice, Spinner } from '@wordpress/components';
import { api, type Connection, type Issue } from '../api/client';

const COLUMNS: { status: Issue['status']; label: string }[] = [
  { status: 'backlog', label: 'Backlog' },
  { status: 'todo', label: 'Todo' },
  { status: 'in_progress', label: 'In Progress' },
  { status: 'in_review', label: 'In Review' },
  { status: 'done', label: 'Done' },
];

interface Props {
  connection: Connection;
}

export default function Kanban({ connection }: Props) {
  const [issues, setIssues] = useState<Issue[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    api
      .issues(connection)
      .then((res) => {
        if (!cancelled) setIssues(res.issues);
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      });
    return () => {
      cancelled = true;
    };
  }, [connection]);

  if (error) {
    return (
      <Notice status="error" isDismissible={false}>
        Failed to load issues: {error}
      </Notice>
    );
  }
  if (issues === null) {
    return <div><Spinner /> Loading issues…</div>;
  }

  return (
    <div className="kanban-board">
      {COLUMNS.map((col) => {
        const colIssues = issues.filter((i) => i.status === col.status);
        return (
          <Panel key={col.status} className="kanban-column">
            <PanelBody
              title={`${col.label} (${colIssues.length})`}
              initialOpen
            >
              {colIssues.length === 0 ? (
                <div className="empty-hint">No issues.</div>
              ) : (
                colIssues.map((issue) => (
                  <div key={issue.id} className="kanban-card">
                    <div className="kanban-card__title">{issue.title}</div>
                    <div className="kanban-card__meta">
                      {issue.persona ?? 'unassigned'} · {issue.priority}
                    </div>
                  </div>
                ))
              )}
            </PanelBody>
          </Panel>
        );
      })}
    </div>
  );
}
