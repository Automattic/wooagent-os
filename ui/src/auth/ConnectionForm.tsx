import { useState } from 'react';
import { Button, TextControl, Notice, Spinner } from '@wordpress/components';
import { api, saveConnection, type Connection } from '../api/client';

interface Props {
  onConnected(connection: Connection): void;
}

export default function ConnectionForm({ onConnected }: Props) {
  const [daemonUrl, setDaemonUrl] = useState('http://localhost:7777');
  const [token, setToken] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    setError(null);
    setBusy(true);
    try {
      const trimmedUrl = daemonUrl.trim();
      const trimmedToken = token.trim();
      const connection: Connection = { daemonUrl: trimmedUrl, token: trimmedToken };
      // Probe /v1/health first (unauthenticated) to catch bad URLs, then
      // /v1/agents (authenticated) to catch bad tokens.
      await api.health(connection);
      await api.agents(connection);
      saveConnection(connection);
      onConnected(connection);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="connect-shell">
      <h1 style={{ marginTop: 0 }}>Connect to a WooAgent daemon</h1>
      <p style={{ color: '#555' }}>
        Paste the daemon URL (default <code>http://localhost:7777</code>) and the auth token
        printed by <code>wooagent init</code> or <code>wooagent run</code>.
      </p>
      {error && (
        <div style={{ marginBottom: 16 }}>
          <Notice status="error" isDismissible={false}>{error}</Notice>
        </div>
      )}
      <TextControl
        label="Daemon URL"
        value={daemonUrl}
        onChange={(v: string | undefined) => setDaemonUrl(v ?? '')}
        placeholder="http://localhost:7777"
        __next40pxDefaultSize
        __nextHasNoMarginBottom
      />
      <div style={{ height: 12 }} />
      <TextControl
        label="Auth token"
        value={token}
        onChange={(v: string | undefined) => setToken(v ?? '')}
        placeholder="wo_pat_..."
        type="password"
        __next40pxDefaultSize
        __nextHasNoMarginBottom
      />
      <div style={{ height: 20 }} />
      <Button
        variant="primary"
        onClick={submit}
        disabled={busy || !daemonUrl.trim() || !token.trim()}
        __next40pxDefaultSize
      >
        {busy ? (<><Spinner /> Connecting…</>) : 'Connect'}
      </Button>
    </div>
  );
}
