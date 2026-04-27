import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Button, Notice, Spinner } from '@wordpress/components';
import {
  ApiError,
  api,
  type Connection,
  type IssueDetail as IssueDetailPayload,
} from '../api/client';

interface Props {
  connection: Connection;
}

export default function IssueDetail({ connection }: Props) {
  const { id } = useParams<{ id: string }>();
  const nav = useNavigate();
  const [data, setData] = useState<IssueDetailPayload | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<'approve' | 'reject' | null>(null);
  const [actionMsg, setActionMsg] = useState<{
    kind: 'success' | 'error';
    text: string;
  } | null>(null);

  useEffect(() => {
    if (!id) return;
    let cancelled = false;
    api
      .issue(connection, id)
      .then((res) => {
        if (!cancelled) setData(res);
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      });
    return () => {
      cancelled = true;
    };
  }, [connection, id]);

  const onApprove = async () => {
    if (!id) return;
    setBusy('approve');
    setActionMsg(null);
    try {
      const res = await api.approve(connection, id);
      setActionMsg({
        kind: 'success',
        text: `Applied via ${res.ability ?? 'MCP'} — issue moved to ${res.status}.`,
      });
      // Reflect new status locally so the buttons disable; navigate back to
      // the board after a beat so the operator sees the card move to Done.
      setData((d) => (d ? { ...d, issue: { ...d.issue, status: res.status } } : d));
      setTimeout(() => nav('/'), 1200);
    } catch (e) {
      setActionMsg({
        kind: 'error',
        text:
          e instanceof ApiError
            ? `${e.code}: ${e.message}`
            : e instanceof Error
              ? e.message
              : String(e),
      });
    } finally {
      setBusy(null);
    }
  };

  const onReject = async () => {
    if (!id) return;
    setBusy('reject');
    setActionMsg(null);
    try {
      const res = await api.reject(connection, id);
      setActionMsg({
        kind: 'success',
        text: `Rejected — issue moved to ${res.status}.`,
      });
      setData((d) => (d ? { ...d, issue: { ...d.issue, status: res.status } } : d));
      setTimeout(() => nav('/'), 1200);
    } catch (e) {
      setActionMsg({
        kind: 'error',
        text:
          e instanceof ApiError
            ? `${e.code}: ${e.message}`
            : e instanceof Error
              ? e.message
              : String(e),
      });
    } finally {
      setBusy(null);
    }
  };

  if (error) {
    return (
      <Notice status="error" isDismissible={false}>
        Failed to load issue: {error}{' '}
        <Link to="/">Back to board</Link>
      </Notice>
    );
  }
  if (!data) {
    return (
      <div>
        <Spinner /> Loading issue…
      </div>
    );
  }

  const { issue, proposal } = data;
  // Pull the `previous` blob out of the proposal target so we can render a
  // before/after pair. Other target fields (product_id/sku/name) render
  // separately as kv metadata so it's clear what the agent is acting on.
  const target = (proposal?.target ?? {}) as Record<string, unknown>;
  const previous = typeof target.previous === 'string' ? target.previous : undefined;
  const otherTarget = Object.fromEntries(
    Object.entries(target).filter(([k]) => k !== 'previous'),
  );

  return (
    <div className="issue-detail">
      <div className="issue-detail__header">
        <Link to="/" className="issue-detail__back">
          ← Board
        </Link>
        <span>·</span>
        <span className="issue-detail__id">{issue.id.slice(0, 8)}</span>
        <span className="issue-detail__status" data-status={issue.status}>
          {issue.status.replace('_', ' ')}
        </span>
        {issue.persona && (
          <span className="issue-detail__persona">{issue.persona}</span>
        )}
      </div>

      <h1 className="issue-detail__title">{issue.title}</h1>

      {issue.description && (
        <p style={{ color: '#4b5563', marginBottom: 24 }}>{issue.description}</p>
      )}

      {!proposal ? (
        <Notice status="info" isDismissible={false}>
          No proposal attached to this issue yet.
        </Notice>
      ) : (
        <>
          {previous && (
            <div className="proposal proposal--previous">
              <div className="proposal__head">
                <span>Current</span>
                <span style={{ fontFamily: 'ui-monospace, Menlo, monospace' }}>
                  {previous.length} chars
                </span>
              </div>
              <div className="proposal__body">{previous}</div>
            </div>
          )}

          <div className="proposal">
            <div className="proposal__head">
              <span>Proposed</span>
              <span className="proposal__type">{proposal.type}</span>
            </div>
            <div className="proposal__body">{proposal.content}</div>
            {Object.keys(otherTarget).length > 0 && (
              <div className="proposal__target">
                {Object.entries(otherTarget)
                  .map(([k, v]) => `${k}=${formatVal(v)}`)
                  .join(' · ')}
              </div>
            )}
          </div>

          {actionMsg && (
            <Notice
              status={actionMsg.kind === 'success' ? 'success' : 'error'}
              isDismissible={false}
            >
              {actionMsg.text}
            </Notice>
          )}

          <div className="issue-detail__actions">
            <Button
              variant="secondary"
              isDestructive
              onClick={onReject}
              disabled={busy !== null || data.issue.status !== 'in_review'}
            >
              {busy === 'reject' ? 'Rejecting…' : 'Reject'}
            </Button>
            <Button
              variant="primary"
              onClick={onApprove}
              disabled={busy !== null || data.issue.status !== 'in_review'}
            >
              {busy === 'approve' ? 'Applying to store…' : 'Approve & apply'}
            </Button>
          </div>
        </>
      )}
    </div>
  );
}

function formatVal(v: unknown): string {
  if (v === null || v === undefined) return '';
  if (typeof v === 'string') return v;
  return JSON.stringify(v);
}
