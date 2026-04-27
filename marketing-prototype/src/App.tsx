import { createContext, useCallback, useContext, useMemo, useState } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';
import HostedBanner from './components/HostedBanner';
import LeftNav from './components/LeftNav';
import SettingsDrawer from './components/SettingsDrawer';
import ToastStack from './components/ToastStack';
import Kanban from './screens/Kanban';
import ContentReview from './screens/ContentReview';
import CampaignPlanner from './screens/CampaignPlanner';
import EmailReview from './screens/EmailReview';
import Placeholder from './screens/Placeholder';
import { SEED_TASKS } from './data/mockTasks';
import type { Task, TaskStatus } from './data/types';

interface Toast {
  id: string;
  kind: 'success' | 'error' | 'info';
  title: string;
  body?: string;
}

interface AppCtx {
  tasks: Task[];
  setTaskStatus: (id: string, status: TaskStatus) => void;
  pushToast: (t: Omit<Toast, 'id'>) => void;
  toasts: Toast[];
  dismissToast: (id: string) => void;
  openSettings: () => void;
}

const Ctx = createContext<AppCtx | null>(null);

export function useApp(): AppCtx {
  const v = useContext(Ctx);
  if (!v) throw new Error('useApp outside provider');
  return v;
}

export default function App() {
  const [tasks, setTasks] = useState<Task[]>(SEED_TASKS);
  const [toasts, setToasts] = useState<Toast[]>([]);
  const [settingsOpen, setSettingsOpen] = useState(false);

  const setTaskStatus = useCallback((id: string, status: TaskStatus) => {
    setTasks((prev) =>
      prev.map((t) => (t.id === id ? ({ ...t, status } as Task) : t)),
    );
  }, []);

  const pushToast = useCallback((t: Omit<Toast, 'id'>) => {
    const id = `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
    setToasts((prev) => [...prev, { ...t, id }]);
    setTimeout(() => {
      setToasts((prev) => prev.filter((x) => x.id !== id));
    }, 5500);
  }, []);

  const dismissToast = useCallback((id: string) => {
    setToasts((prev) => prev.filter((x) => x.id !== id));
  }, []);

  const openSettings = useCallback(() => setSettingsOpen(true), []);

  const ctx = useMemo<AppCtx>(
    () => ({
      tasks,
      setTaskStatus,
      pushToast,
      toasts,
      dismissToast,
      openSettings,
    }),
    [tasks, setTaskStatus, pushToast, toasts, dismissToast, openSettings],
  );

  return (
    <Ctx.Provider value={ctx}>
      <div className="flex min-h-screen">
        <LeftNav onOpenSettings={openSettings} />
        <div className="flex-1 min-w-0">
          <HostedBanner />
          <Routes>
            <Route path="/" element={<Kanban />} />
            <Route path="/issues/:id/content" element={<ContentReview />} />
            <Route path="/issues/:id/campaign" element={<CampaignPlanner />} />
            <Route path="/issues/:id/email" element={<EmailReview />} />
            <Route
              path="/activity"
              element={
                <Placeholder
                  area="Activity"
                  description="Audit log of every agent action with the underlying tool calls, ability versions, and operator approvals. Reference area only — see the daemon UI for the live feed."
                  status="demo"
                />
              }
            />
            <Route
              path="/abilities"
              element={
                <Placeholder
                  area="Abilities"
                  description="Browser for every signed ability the agents can call (WooCommerce, Yoast, WordPress.com, etc.). Tune permissions, see version pins, audit recent calls."
                  status="demo"
                />
              }
            />
            <Route
              path="/agents/marketing"
              element={<Navigate to="/" replace />}
            />
            <Route
              path="/agents/chief"
              element={
                <Placeholder
                  area="Chief of Staff"
                  description="Orchestrates the specialist agents, dispatches work, and keeps the queue balanced. Out of scope for the marketing prototype."
                />
              }
            />
            <Route
              path="/agents/pricing"
              element={
                <Placeholder
                  area="Pricing agent"
                  description="Watches margins, competitor signals, and sales velocity to propose price moves. Paused for the marketing prototype."
                />
              }
            />
            <Route
              path="/agents/inventory"
              element={
                <Placeholder
                  area="Inventory agent"
                  description="Reorder points, supplier nudges, low-stock alerts. Paused for the marketing prototype."
                />
              }
            />
            <Route
              path="/agents/accounting"
              element={
                <Placeholder
                  area="Accounting agent"
                  description="Reconciles WooPayments + Stripe + bank, drafts month-end summaries. Paused for the marketing prototype."
                />
              }
            />
            <Route
              path="/agents/reporting"
              element={
                <Placeholder
                  area="Reporting agent"
                  description="Weekly digests, anomaly alerts, ad-hoc questions. Paused for the marketing prototype."
                />
              }
            />
            <Route
              path="/agents/sales-support"
              element={
                <Placeholder
                  area="Sales Support agent"
                  description="Drafts customer replies, handles refund triage, escalates edge cases. Paused for the marketing prototype."
                />
              }
            />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </div>
      </div>
      <SettingsDrawer
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
      />
      <ToastStack />
    </Ctx.Provider>
  );
}
