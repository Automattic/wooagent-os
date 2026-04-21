import { Button, Notice } from '@wordpress/components';
import { clearConnection, type Connection } from '../api/client';

interface Props {
  connection: Connection;
  onDisconnect(): void;
}

export default function Settings({ connection, onDisconnect }: Props) {
  const disconnect = () => {
    clearConnection();
    onDisconnect();
  };

  return (
    <div style={{ maxWidth: 640 }}>
      <h1>Settings</h1>
      <p>Daemon URL: <code>{connection.daemonUrl}</code></p>
      <p>Token: <code>{connection.token.slice(0, 10)}…</code></p>

      <div style={{ margin: '24px 0' }}>
        <Button variant="secondary" onClick={disconnect} __next40pxDefaultSize>
          Forget this daemon
        </Button>
      </div>

      <Notice status="info" isDismissible={false}>
        Model providers, guardrails, redaction rules, and per-persona opt-in settings land
        in later phases. This is a placeholder.
      </Notice>
    </div>
  );
}
