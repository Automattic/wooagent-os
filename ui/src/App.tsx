import { useCallback, useEffect, useState } from 'react';
import { Route, Routes, Navigate, useLocation } from 'react-router-dom';
import { Text } from '@wordpress/ui';
import OnboardingShell from './onboarding/OnboardingShell';
import LeftNav from './components/LeftNav';
import TopBar from './components/TopBar';
import AskAgentDrawer from './components/AskAgentDrawer';
import Kanban from './screens/Kanban';
import IssueDetail from './screens/IssueDetail';
import BatchReview from './screens/BatchReview';
import Agents from './screens/Agents';
import Settings from './screens/Settings';
import Placeholder from './screens/Placeholder';
import {
  loadConnection,
  type Batch,
  type Connection,
  type Issue,
  api,
} from './api/client';
import { useIsMobile } from './lib/useMediaQuery';

export default function App() {
  const [connection, setConnection] = useState<Connection | null>(null);
  const [probed, setProbed] = useState(false);
  // Onboarding gate. Tri-state until probed: null = checking, true = ready
  // for kanban, false = onboarding flow takes over the whole window.
  const [onboardingComplete, setOnboardingComplete] = useState<boolean | null>(
    null,
  );
  const [issues, setIssues] = useState<Issue[] | null>(null);
  const [batches, setBatches] = useState<Batch[]>([]);
  const [issuesError, setIssuesError] = useState<string | null>(null);
  const [askAgentOpen, setAskAgentOpen] = useState(false);

  // Global ⌘K / Ctrl+K opens the Ask Agent drawer from anywhere in the app.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setAskAgentOpen((o) => !o);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  useEffect(() => {
    const stored = loadConnection();
    if (!stored) {
      setProbed(true);
      setOnboardingComplete(false);
      return;
    }
    (async () => {
      try {
        await api.health(stored);
        setConnection(stored);
        // Probe onboarding completion: kanban opens only if there's a paired
        // store AND a configured model provider (brief §3 — "kanban opens
        // only when the underlying setup is sound"). Either endpoint
        // missing or empty means onboarding isn't done.
        const [storesRes, providersRes] = await Promise.all([
          api.stores.list(stored).catch(() => ({ stores: [] })),
          api.modelProviders.list(stored).catch(() => ({ providers: [] })),
        ]);
        const hasStore = storesRes.stores.some((s) => s.status === 'paired');
        const hasProvider = providersRes.providers.length > 0;
        setOnboardingComplete(hasStore && hasProvider);
      } catch {
        setOnboardingComplete(false);
      } finally {
        setProbed(true);
      }
    })();
  }, []);

  // Re-probe completion when the operator finishes the flow. Step 5's
  // "Open the kanban" calls onComplete() which lands here.
  const markOnboardingComplete = useCallback(() => {
    setOnboardingComplete(true);
  }, []);

  // Refresh shared board state whenever a connection lands or an issue
  // changes status. Both the kanban (issues + batches) and the sidebar's
  // "Marketing N in review" badge consume this. Batches load best-effort
  // — a /v1/batches failure must not block the kanban from rendering.
  const refreshIssues = useCallback(
    async (c: Connection) => {
      setIssuesError(null);
      try {
        const [issuesRes, batchesRes] = await Promise.all([
          api.issues(c),
          api.batches.list(c).catch(() => ({ batches: [] as Batch[] })),
        ]);
        setIssues(issuesRes.issues);
        setBatches(batchesRes.batches);
      } catch (e) {
        setIssuesError(e instanceof Error ? e.message : String(e));
        setIssues([]);
        setBatches([]);
      }
    },
    [],
  );

  useEffect(() => {
    if (!connection) return;
    let cancelled = false;
    void (async () => {
      try {
        const [issuesRes, batchesRes] = await Promise.all([
          api.issues(connection),
          api.batches.list(connection).catch(() => ({ batches: [] as Batch[] })),
        ]);
        if (!cancelled) {
          setIssues(issuesRes.issues);
          setBatches(batchesRes.batches);
        }
      } catch (e) {
        if (!cancelled) {
          setIssuesError(e instanceof Error ? e.message : String(e));
          setIssues([]);
          setBatches([]);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [connection]);

  if (!probed) return null;

  if (!connection || !onboardingComplete) {
    return (
      <OnboardingRoutes
        connection={connection}
        onConnected={setConnection}
        onComplete={markOnboardingComplete}
      />
    );
  }

  const hostname = (() => {
    try {
      return new URL(connection.daemonUrl).host;
    } catch {
      return connection.daemonUrl;
    }
  })();
  const inReview = (issues ?? []).filter((i) => i.status === 'in_review').length;
  const askAgentContext = `Board · Today's marketing queue · ${(issues ?? []).length} items`;

  return (
    <Shell>
      {(drawer) => (
        <>
          <LeftNav
            marketingInReview={inReview}
            daemonHostname={hostname}
            isOpen={drawer.isOpen}
            onItemClick={drawer.close}
          />
          <div
            className={`wa-sidebar-backdrop${drawer.isOpen ? ' is-open' : ''}`}
            onClick={drawer.close}
            aria-hidden="true"
          />
          <div style={{ flex: 1, minWidth: 0 }}>
            <header className="wa-mobile-bar">
              <button
                type="button"
                className="wa-icon-btn"
                aria-label={drawer.isOpen ? 'Close menu' : 'Open menu'}
                onClick={drawer.toggle}
              >
                <span aria-hidden="true" style={{ fontSize: 18, lineHeight: 1 }}>
                  ☰
                </span>
              </button>
              <Text variant="heading-sm">WooAgent OS</Text>
            </header>
            <TopBar onAskAgent={() => setAskAgentOpen(true)} />
            <AskAgentDrawer
              isOpen={askAgentOpen}
              onClose={() => setAskAgentOpen(false)}
              contextLabel={askAgentContext}
            />
        <Routes>
          <Route
            path="/"
            element={
              <Kanban issues={issues} batches={batches} error={issuesError} />
            }
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
            path="/batches/:id"
            element={
              <BatchReview
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
                  setBatches([]);
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
          <Route
            path="/my-issues"
            element={
              <Placeholder
                area="My issues"
                description="User-generated issues you'd like the agents to take a look at. Coming in V2."
                status="soon"
              />
            }
          />
          <Route path="/agents" element={<Agents connection={connection} />} />
          <Route
            path="/runtimes"
            element={
              <Placeholder
                area="Runtimes"
                description="Daemons, MCP servers, and the LLM endpoints they connect to. Health, latency, model in use."
                status="soon"
              />
            }
          />
          <Route
            path="/guardrails"
            element={
              <Placeholder
                area="Guardrails"
                description="Policy enforcement rules — what each agent can do without your sign-off, what always needs review."
                status="soon"
              />
            }
          />
          <Route
            path="/secrets"
            element={
              <Placeholder
                area="Secrets"
                description="API keys and tokens used by the abilities. Rotate, scope, and audit access."
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
        </>
      )}
    </Shell>
  );
}

interface DrawerControls {
  isOpen: boolean;
  open: () => void;
  close: () => void;
  toggle: () => void;
}

interface ShellProps {
  children: (drawer: DrawerControls) => React.ReactNode;
}

// Mounts the OnboardingShell only inside the /onboard/* route tree so the
// shell's nested <Routes> resolves against onboarding paths. Anything else
// gets sent to /onboard for the resume-redirect to take over.
interface OnboardingRoutesProps {
  connection: Connection | null;
  onConnected(c: Connection): void;
  onComplete(): void;
}
function OnboardingRoutes({
  connection,
  onConnected,
  onComplete,
}: OnboardingRoutesProps) {
  return (
    <Routes>
      <Route
        path="/onboard/*"
        element={
          <OnboardingShell
            connection={connection}
            onConnected={onConnected}
            onComplete={onComplete}
          />
        }
      />
      <Route path="*" element={<Navigate to="/onboard" replace />} />
    </Routes>
  );
}

// Shell owns the mobile-drawer state and closes it whenever the route
// changes. Lives inside the BrowserRouter (declared in main.tsx) so
// useLocation works.
function Shell({ children }: ShellProps) {
  const isMobile = useIsMobile();
  const location = useLocation();
  const [isOpen, setIsOpen] = useState(false);

  // Auto-close on route change (mobile drawer behavior).
  useEffect(() => {
    setIsOpen(false);
  }, [location.pathname]);

  // Auto-close when crossing back to desktop so the off-canvas state isn't
  // left "open" after the drawer becomes the static sidebar.
  useEffect(() => {
    if (!isMobile) setIsOpen(false);
  }, [isMobile]);

  const drawer: DrawerControls = {
    isOpen,
    open: () => setIsOpen(true),
    close: () => setIsOpen(false),
    toggle: () => setIsOpen((v) => !v),
  };

  return (
    <div style={{ display: 'flex', minHeight: '100vh' }}>{children(drawer)}</div>
  );
}
