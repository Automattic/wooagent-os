import { useCallback, useEffect, useState } from 'react';
import { Route, Routes, Navigate, useLocation } from 'react-router-dom';
import { Stack, Text } from '@wordpress/ui';
import OnboardingShell from './onboarding/OnboardingShell';
import LeftNav from './components/LeftNav';
import AskAgentDrawer from './components/AskAgentDrawer';
import AuthExpiredModal from './components/AuthExpiredModal';
import NeedsReview from './screens/NeedsReview';
import Done from './screens/Done';
import IssueDetail from './screens/IssueDetail';
import BatchReview from './screens/BatchReview';
import Agents from './screens/Agents';
import AddAgent from './screens/AddAgent';
import EditAgent from './screens/EditAgent';
import Abilities from './screens/Abilities';
import Archived from './screens/Archived';
import Stores from './screens/Stores';
import Models from './screens/Models';
import Placeholder from './screens/Placeholder';
import Runs from './screens/Runs';
import RunDetail from './screens/RunDetail';
import {
  clearConnection,
  isEmbedded,
  loadConnection,
  type Batch,
  type Connection,
  type Issue,
  type Store,
  api,
} from './api/client';
import { useIsMobile } from './lib/useMediaQuery';
import { AskAgentProvider } from './lib/askAgent';

// Mirror of AUTH_NOTICE_FLAG in api/client.ts. Kept inline (rather than
// imported) because the flag is a private state-machine detail of the
// auth-recovery handler; App reads it but must not mutate it directly.
const AUTH_NOTICE_FLAG = 'wooagent.authNoticeReason';

export default function App() {
  // Every screen renders its own header via the WPDS <Page> component
  // (heading + global search + Ask agent on a single row, via the shared
  // PageGlobalActions helper), so the standalone TopBar component has been
  // retired. `useLocation` here drives the Inbox-section highlight in
  // LeftNav based on the entity status of the currently viewed detail page;
  // the inner Shell uses its own useLocation to auto-close the mobile drawer.
  const location = useLocation();
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
  // The paired store, surfaced in LeftNav's footer popover (skills count,
  // last-discovered timestamp, Open in WP-admin link). OS is single-store, so
  // we keep the first paired row; multi-store is a Cloud concern.
  const [pairedStore, setPairedStore] = useState<Store | null>(null);

  // Watch sessionStorage for the auth-expired notice flag. api/client.ts
  // sets it after a second 401 in a session; the modal renders until the
  // operator clicks Reconnect. 500ms poll is the simplest cross-component
  // signal that doesn't require new infrastructure (custom event bus,
  // context provider). The modal only renders once per recovery cycle so
  // the poll cost is negligible.
  const [authExpired, setAuthExpired] = useState(
    () =>
      typeof sessionStorage !== 'undefined' &&
      sessionStorage.getItem(AUTH_NOTICE_FLAG) === 'persistent_401',
  );
  useEffect(() => {
    if (typeof sessionStorage === 'undefined') return;
    const id = setInterval(() => {
      const flag =
        sessionStorage.getItem(AUTH_NOTICE_FLAG) === 'persistent_401';
      setAuthExpired((prev) => (prev === flag ? prev : flag));
    }, 500);
    return () => clearInterval(id);
  }, []);

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
        const paired = storesRes.stores.find((s) => s.status === 'paired') ?? null;
        const hasStore = paired !== null;
        const hasProvider = providersRes.providers.length > 0;
        setPairedStore(paired);
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

  // Poll for issue updates while the tab is visible so new issues from
  // manual triggers (Agents → "Run now") or scheduler ticks during a
  // long browser session show up without a manual reload. Same pattern
  // as Runs.tsx's auto-refresh. 10s strikes a balance between freshness
  // and request volume; backlog-observations-design.md proposes a faster
  // interval (~2s) while any issue is in_progress — defer that until the
  // backlog/observation work lands.
  useEffect(() => {
    if (!connection) return;
    const interval = setInterval(() => {
      if (document.visibilityState !== 'visible') return;
      void refreshIssues(connection);
    }, 10_000);
    return () => clearInterval(interval);
  }, [connection, refreshIssues]);

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

  // Badge mirrors what the operator sees on the Needs review board: batched
  // children collapse to a single batch row, so a 9-child marketing batch
  // counts as 1, not 9. Stand-alone issues (no batch_id, or batch_id refers
  // to a missing batch) still count individually.
  const batchIDs = new Set(batches.map((b) => b.id));
  const standaloneInReview = (issues ?? []).filter(
    (i) =>
      i.status === 'in_review' && (!i.batch_id || !batchIDs.has(i.batch_id)),
  ).length;
  const batchesInReview = batches.filter((b) => b.pending > 0).length;
  const inReview = standaloneInReview + batchesInReview;

  // Resolve which Inbox section LeftNav should highlight when on a detail
  // page (/issues/:id or /batches/:id). The entity's status carries this:
  // dismissed/rejected → Archived, done → Done, otherwise Needs review.
  // Falls back to null while the entity hasn't loaded yet so LeftNav keeps
  // its path-based default instead of flashing the wrong selection.
  const detailInbox = ((): 'needs-review' | 'done' | 'archived' | null => {
    const issueMatch = location.pathname.match(/^\/issues\/([^/]+)/);
    if (issueMatch) {
      const issue = (issues ?? []).find((i) => i.id === issueMatch[1]);
      if (!issue) return null;
      if (issue.status === 'dismissed' || issue.status === 'rejected')
        return 'archived';
      if (issue.status === 'done') return 'done';
      return 'needs-review';
    }
    const batchMatch = location.pathname.match(/^\/batches\/([^/]+)/);
    if (batchMatch) {
      const batch = batches.find((b) => b.id === batchMatch[1]);
      if (!batch) return null;
      if (batch.pending > 0) return 'needs-review';
      if (batch.approved === 0 && batch.rejected > 0) return 'archived';
      return 'done';
    }
    return null;
  })();

  return (
    <AskAgentProvider>
      {authExpired && <AuthExpiredModal />}
      <Shell>
      {(drawer) => (
        <>
          <LeftNav
            inReviewCount={inReview}
            activeInbox={detailInbox}
            connection={connection}
            store={pairedStore}
            embedded={isEmbedded()}
            onForgetConnection={() => {
              clearConnection();
              setConnection(null);
              setIssues(null);
              setBatches([]);
              setPairedStore(null);
            }}
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
              {/* CUSTOM: mobile sidebar-toggle icon button. (a) WPDS has no compact mobile-chrome icon-button matching .wa-icon-btn. (b) Unicode glyph child + shared .wa-icon-btn styles. (c) Follow-up: migrate to <Button icon={menu}> when .wa-icon-btn retires. */}
              <button
                type="button"
                className="wa-icon-btn"
                aria-label={drawer.isOpen ? 'Close menu' : 'Open menu'}
                onClick={drawer.toggle}
              >
                <span
                  aria-hidden="true"
                  style={{
                    fontSize: 'var(--wpds-typography-font-size-xl)',
                    lineHeight: 1,
                  }}
                >
                  ☰
                </span>
              </button>
              <Text variant="heading-sm">WooAgent OS</Text>
            </header>
            <AskAgentDrawer
              isOpen={askAgentOpen}
              onClose={() => setAskAgentOpen(false)}
              connection={connection}
            />
        <Routes>
          <Route path="/" element={<Navigate to="/needs-review" replace />} />
          <Route
            path="/needs-review"
            element={
              <NeedsReview
                issues={issues}
                batches={batches}
                error={issuesError}
                onAskAgent={() => setAskAgentOpen(true)}
              />
            }
          />
          <Route
            path="/done"
            element={
              <Done
                issues={issues}
                batches={batches}
                error={issuesError}
                onAskAgent={() => setAskAgentOpen(true)}
              />
            }
          />
          <Route
            path="/issues/:id"
            element={
              <IssueDetail
                connection={connection}
                onChanged={() => refreshIssues(connection)}
                onAskAgent={() => setAskAgentOpen(true)}
              />
            }
          />
          <Route
            path="/batches/:id"
            element={
              <BatchReview
                connection={connection}
                onChanged={() => refreshIssues(connection)}
                onAskAgent={() => setAskAgentOpen(true)}
              />
            }
          />
          <Route
            path="/stores"
            element={
              <Stores
                connection={connection}
                onStoreDisconnected={() => {
                  // Flip onboarding state so App's render branch picks
                  // OnboardingRoutes on the next render. OnboardingShell
                  // will re-probe /v1/stores, find none, and land the
                  // user on "Connect store".
                  setOnboardingComplete(false);
                  setIssues(null);
                  setBatches([]);
                }}
                onAskAgent={() => setAskAgentOpen(true)}
              />
            }
          />
          {/* Redirect legacy bookmarks. */}
          <Route path="/settings" element={<Navigate to="/stores" replace />} />
          <Route
            path="/models"
            element={
              <Models
                connection={connection}
                onAskAgent={() => setAskAgentOpen(true)}
              />
            }
          />
          <Route
            path="/runs"
            element={
              <Runs
                connection={connection}
                onAskAgent={() => setAskAgentOpen(true)}
              />
            }
          />
          <Route
            path="/runs/:id"
            element={
              <RunDetail
                connection={connection}
                onAskAgent={() => setAskAgentOpen(true)}
                onRunTerminal={() => refreshIssues(connection)}
              />
            }
          />
          <Route
            path="/activity"
            element={<Navigate to="/runs" replace />}
          />
          <Route
            path="/skills"
            element={
              <Abilities
                connection={connection}
                onAskAgent={() => setAskAgentOpen(true)}
              />
            }
          />
          <Route
            path="/abilities"
            element={<Navigate to="/skills" replace />}
          />
          <Route
            path="/archived"
            element={
              <Archived
                connection={connection}
                onAskAgent={() => setAskAgentOpen(true)}
              />
            }
          />
          <Route
            path="/my-issues"
            element={
              <Placeholder
                onAskAgent={() => setAskAgentOpen(true)}
                area="My issues"
                description="User-generated issues you'd like the agents to take a look at. Coming in V2."
                status="soon"
              />
            }
          />
          <Route
            path="/agents"
            element={
              <Agents
                connection={connection}
                onAskAgent={() => setAskAgentOpen(true)}
                onChanged={() => refreshIssues(connection)}
              />
            }
          />
          <Route
            path="/agents/add"
            element={
              <AddAgent
                connection={connection}
                onChanged={() => refreshIssues(connection)}
              />
            }
          />
          <Route
            path="/agents/:slug/edit"
            element={
              <EditAgent
                connection={connection}
                onChanged={() => refreshIssues(connection)}
              />
            }
          />
          <Route
            path="/runtimes"
            element={
              <Placeholder
                onAskAgent={() => setAskAgentOpen(true)}
                area="Routines"
                description="Set up custom automated routines that your agents can run for you."
                status="soon"
              />
            }
          />
          <Route
            path="/guardrails"
            element={
              <Placeholder
                onAskAgent={() => setAskAgentOpen(true)}
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
                onAskAgent={() => setAskAgentOpen(true)}
                area="Secrets"
                description="API keys and tokens used by the abilities. Rotate, scope, and audit access."
                status="soon"
              />
            }
          />
          <Route path="/agents/marketing" element={<Navigate to="/needs-review" replace />} />
          <Route
            path="/agents/chief"
            element={
              <Placeholder
                onAskAgent={() => setAskAgentOpen(true)}
                area="Chief of staff"
                description="Orchestrates the specialist agents, dispatches work, and keeps the queue balanced. Out of scope for phase 1."
              />
            }
          />
          <Route
            path="/agents/pricing"
            element={
              <Placeholder
                onAskAgent={() => setAskAgentOpen(true)}
                area="Pricing agent"
                description="Watches margins, competitor signals, and sales velocity to propose price moves. Paused for phase 1."
              />
            }
          />
          <Route
            path="/agents/inventory"
            element={
              <Placeholder
                onAskAgent={() => setAskAgentOpen(true)}
                area="Inventory agent"
                description="Reorder points, supplier nudges, low-stock alerts. Paused for phase 1."
              />
            }
          />
          <Route
            path="/agents/accounting"
            element={
              <Placeholder
                onAskAgent={() => setAskAgentOpen(true)}
                area="Accounting agent"
                description="Reconciles WooPayments + Stripe + bank, drafts month-end summaries. Paused for phase 1."
              />
            }
          />
          <Route
            path="/agents/reporting"
            element={
              <Placeholder
                onAskAgent={() => setAskAgentOpen(true)}
                area="Reporting agent"
                description="Weekly digests, anomaly alerts, ad-hoc questions. Paused for phase 1."
              />
            }
          />
          <Route
            path="/agents/sales-support"
            element={
              <Placeholder
                onAskAgent={() => setAskAgentOpen(true)}
                area="Sales support agent"
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
    </AskAgentProvider>
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
    <Stack direction="row" style={{ minHeight: '100vh' }}>
      {children(drawer)}
    </Stack>
  );
}
