import { useEffect, useState } from 'react';
import { NavLink, Route, Routes, Navigate } from 'react-router-dom';
import ConnectionForm from './auth/ConnectionForm';
import Kanban from './screens/Kanban';
import IssueDetail from './screens/IssueDetail';
import Settings from './screens/Settings';
import { loadConnection, type Connection, api } from './api/client';

export default function App() {
  const [connection, setConnection] = useState<Connection | null>(null);
  const [probed, setProbed] = useState(false);

  // On first mount, try the stored connection. If /v1/health fails, fall back
  // to the connection form so the operator can re-enter credentials.
  useEffect(() => {
    const stored = loadConnection();
    if (!stored) {
      setProbed(true);
      return;
    }
    api.health(stored)
      .then(() => setConnection(stored))
      .catch(() => {
        /* leave connection null, form will render */
      })
      .finally(() => setProbed(true));
  }, []);

  if (!probed) return null;

  if (!connection) {
    return <ConnectionForm onConnected={setConnection} />;
  }

  return (
    <div className="app-shell">
      <header className="app-topbar">
        <div className="app-topbar__brand">WooAgent OS</div>
        <nav>
          <NavLink to="/" end>Issues</NavLink>
          <NavLink to="/settings">Settings</NavLink>
        </nav>
      </header>
      <main className="app-main">
        <Routes>
          <Route path="/" element={<Kanban connection={connection} />} />
          <Route path="/issues/:id" element={<IssueDetail connection={connection} />} />
          <Route
            path="/settings"
            element={<Settings connection={connection} onDisconnect={() => setConnection(null)} />}
          />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
    </div>
  );
}
