import { useCallback, useEffect, useState } from 'react';
import { Route, Routes, Navigate } from 'react-router-dom';
import ConnectionForm from './auth/ConnectionForm';
import LeftNav from './components/LeftNav';
import Kanban from './screens/Kanban';
import IssueDetail from './screens/IssueDetail';
import Settings from './screens/Settings';
import Placeholder from './screens/Placeholder';
import { loadConnection, type Connection, type Issue, api } from './api/client';

export default function App() {
  const [connection, setConnection] = useState<Connection | null>(null);
  const [probed, setProbed] = useState(false);
  const [issues, setIssues] = useState<Issue[] | null>(null);
  const [issuesError, setIssuesError] = useState<string | null>(null);

  useEffect(() => {
    const stored = loadConnection();
    if (!stored) {
      setProbed(true);
      return;
    }
    api
      .health(stored)
      .then(() => setConnection(stored))
      .catch(() => {
        /* leave connection null, form will render */
      })
      .finally(() => setProbed(true));
  }, []);

  // Refresh shared issue state whenever a connection lands. Both the kanban
  // and the sidebar's "Marketing N in review" badge consume this.
  const refreshIssues = useCallback(
    async (c: Connection) => {
      setIssuesError(null);
      try {
        const res = await api.issues(c);
        setIssues(res.issues);
      } catch (e) {
        setIssuesError(e instanceof Error ? e.message : String(e));
        setIssues([]);
      }
    },
    [],
  );

  useEffect(() => {
    if (!connection) return;
    let cancelled = false;
    void (async () => {
      const res = await api.issues(connection).catch((e) => {
        if (!cancelled) {
          setIssuesError(e instanceof Error ? e.message : String(e));
          setIssues([]);
        }
        return null;
      });
      if (!cancelled && res) setIssues(res.issues);
    })();
    return () => {
      cancelled = true;
    };
  }, [connection]);

  if (!probed) return null;

  if (!connection) {
    return <ConnectionForm onConnected={setConnection} />;
  }

  const hostname = (() => {
    try {
      return new URL(connection.daemonUrl).host;
    } catch {
      return connection.daemonUrl;
    }
  })();
  const inReview = (issues ?? []).filter((i) => i.status === 'in_review').length;

  return (
    <div style={{ display: 'flex', minHeight: '100vh' }}>
      <LeftNav marketingInReview={inReview} daemonHostname={hostname} />
      <div style={{ flex: 1, minWidth: 0 }}>
        <Routes>
          <Route
            path="/"
            element={<Kanban issues={issues} error={issuesError} />}
          />
          <Route
            path="/issues/:id"
            element={
              <IssueDetail
                connection={connection}
                onChanged={() => refreshIssues(connection)}
              />
            }
          />
          <Route
            path="/settings"
            element={
              <Settings
                connection={connection}
                onDisconnect={() => {
                  setConnection(null);
                  setIssues(null);
                }}
              />
            }
          />
          <Route
            path="/activity"
            element={
              <Placeholder
                area="Activity"
                description="Audit log of every agent action with the underlying tool calls, ability versions, and operator approvals. Coming online with the daemon's run history endpoint."
                status="soon"
              />
            }
          />
          <Route
            path="/abilities"
            element={
              <Placeholder
                area="Abilities"
                description="Browser for every signed ability the agents can call (WooCommerce, Yoast, WordPress.com, etc.). Tune permissions, see version pins, audit recent calls."
                status="soon"
              />
            }
          />
          <Route path="/agents/marketing" element={<Navigate to="/" replace />} />
          <Route
            path="/agents/chief"
            element={
              <Placeholder
                area="Chief of Staff"
                description="Orchestrates the specialist agents, dispatches work, and keeps the queue balanced. Out of scope for phase 1."
              />
            }
          />
          <Route
            path="/agents/pricing"
            element={
              <Placeholder
                area="Pricing agent"
                description="Watches margins, competitor signals, and sales velocity to propose price moves. Paused for phase 1."
              />
            }
          />
          <Route
            path="/agents/inventory"
            element={
              <Placeholder
                area="Inventory agent"
                description="Reorder points, supplier nudges, low-stock alerts. Paused for phase 1."
              />
            }
          />
          <Route
            path="/agents/accounting"
            element={
              <Placeholder
                area="Accounting agent"
                description="Reconciles WooPayments + Stripe + bank, drafts month-end summaries. Paused for phase 1."
              />
            }
          />
          <Route
            path="/agents/reporting"
            element={
              <Placeholder
                area="Reporting agent"
                description="Weekly digests, anomaly alerts, ad-hoc questions. Paused for phase 1."
              />
            }
          />
          <Route
            path="/agents/sales-support"
            element={
              <Placeholder
                area="Sales Support agent"
                description="Drafts customer replies, handles refund triage, escalates edge cases. Paused for phase 1."
              />
            }
          />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </div>
    </div>
  );
}
