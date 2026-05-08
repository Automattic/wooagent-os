import { useEffect, useState } from 'react';
import { Card, Notice, Stack, Text } from '@wordpress/ui';
import { Button, Spinner, TextControl } from '@wordpress/components';
import {
  api,
  loadConnection,
  saveConnection,
  type Connection,
} from '../api/client';

interface Props {
  onConnected(connection: Connection): void;
}

type Mode =
  // Initial probe: we don't know yet whether to autodetect or show form.
  | { kind: 'detecting' }
  // Saved connection in localStorage probed clean — show "continue?" card.
  | { kind: 'existing'; connection: Connection }
  // No saved creds, but localhost:7777 is reachable — ask for token only.
  | { kind: 'detected_local'; daemonUrl: string }
  // Nothing reachable — full manual form.
  | { kind: 'manual' };

const DEFAULT_URL = 'http://localhost:7777';
const MUTED = { color: 'var(--wpds-color-fg-content-neutral-weak)' } as const;

export default function Step1Daemon({ onConnected }: Props) {
  const [mode, setMode] = useState<Mode>({ kind: 'detecting' });
  const [daemonUrl, setDaemonUrl] = useState(DEFAULT_URL);
  const [token, setToken] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Autodetect on mount: existing creds first, then a same-host probe. Per
  // the brief §9.1 — we autodetect, but always show a confirmation rather
  // than skipping silently.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      const stored = loadConnection();
      if (stored) {
        try {
          await api.health(stored);
          await api.agents(stored);
          if (!cancelled) setMode({ kind: 'existing', connection: stored });
          return;
        } catch {
          /* fall through to local probe */
        }
      }
      try {
        await api.health({ daemonUrl: DEFAULT_URL, token: '' });
        if (!cancelled) {
          setMode({ kind: 'detected_local', daemonUrl: DEFAULT_URL });
          setDaemonUrl(DEFAULT_URL);
        }
      } catch {
        if (!cancelled) setMode({ kind: 'manual' });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const submit = async (override?: Partial<Connection>) => {
    setError(null);
    setBusy(true);
    try {
      const c: Connection = {
        daemonUrl: (override?.daemonUrl ?? daemonUrl).trim(),
        token: (override?.token ?? token).trim(),
      };
      await api.health(c);
      await api.agents(c);
      saveConnection(c);
      onConnected(c);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  if (mode.kind === 'detecting') {
    return (
      <Card.Root>
        <Card.Content>
          <Stack direction="row" align="center" gap="md">
            <Spinner />
            <Text variant="body-sm" style={MUTED}>
              Looking for WooAgent OS on this machine…
            </Text>
          </Stack>
        </Card.Content>
      </Card.Root>
    );
  }

  if (mode.kind === 'existing') {
    const host = (() => {
      try {
        return new URL(mode.connection.daemonUrl).host;
      } catch {
        return mode.connection.daemonUrl;
      }
    })();
    return (
      <Card.Root>
        <Card.Content>
          <Stack direction="column" gap="lg">
            <Stack direction="column" gap="sm">
              <Text variant="heading-md">Connect to WooAgent OS</Text>
              <Text variant="body-sm" style={MUTED}>
                We found an existing connection in this browser. Continue
                with it, or connect to a different one.
              </Text>
            </Stack>

            <Stack
              direction="column"
              gap="xs"
              style={{
                padding: 'var(--wpds-dimension-padding-md)',
                background: 'var(--wpds-color-bg-surface-info-weak)',
                borderRadius: 'var(--wpds-border-radius-md)',
              }}
            >
              <Text variant="body-sm" style={MUTED}>
                WOOAGENT URL
              </Text>
              <Text variant="body-md" className="wa-mono">
                {host}
              </Text>
            </Stack>

            {error && (
              <Notice.Root intent="error">
                <Notice.Description>{error}</Notice.Description>
              </Notice.Root>
            )}

            <Stack direction="row" gap="md" align="center">
              <Button
                variant="primary"
                __next40pxDefaultSize
                disabled={busy}
                onClick={() =>
                  submit({
                    daemonUrl: mode.connection.daemonUrl,
                    token: mode.connection.token,
                  })
                }
              >
                {busy ? 'Connecting…' : 'Continue'}
              </Button>
              <Button
                variant="tertiary"
                __next40pxDefaultSize
                onClick={() => setMode({ kind: 'manual' })}
              >
                Connect to a different one
              </Button>
            </Stack>
          </Stack>
        </Card.Content>
      </Card.Root>
    );
  }

  if (mode.kind === 'detected_local') {
    return (
      <Card.Root>
        <Card.Content>
          <Stack direction="column" gap="lg">
            <Stack direction="column" gap="sm">
              <Text variant="heading-md">Connect to WooAgent OS</Text>
              <Text variant="body-sm" style={MUTED}>
                Found WooAgent OS at <code>{mode.daemonUrl}</code>. Paste the
                auth token printed by <code>wooagent run</code> in your
                terminal.
              </Text>
            </Stack>

            {error && (
              <Notice.Root intent="error">
                <Notice.Description>{error}</Notice.Description>
              </Notice.Root>
            )}

            <TextControl
              label="Auth token"
              value={token}
              onChange={(v: string | undefined) => setToken(v ?? '')}
              placeholder="wo_pat_…"
              type="password"
              __next40pxDefaultSize
              __nextHasNoMarginBottom
            />

            <Stack direction="row" gap="md" align="center">
              <Button
                variant="primary"
                __next40pxDefaultSize
                disabled={busy || !token.trim()}
                onClick={() => submit()}
              >
                {busy ? 'Connecting…' : 'Connect'}
              </Button>
              <Button
                variant="tertiary"
                __next40pxDefaultSize
                onClick={() => setMode({ kind: 'manual' })}
              >
                Use a different URL
              </Button>
            </Stack>
          </Stack>
        </Card.Content>
      </Card.Root>
    );
  }

  return (
    <Card.Root>
      <Card.Content>
        <Stack direction="column" gap="lg">
          <Stack direction="column" gap="sm">
            <Text variant="heading-md">Connect to WooAgent OS</Text>
            <Text variant="body-sm" style={MUTED}>
              WooAgent OS runs locally on your machine. Paste the URL and
              auth token from your terminal — both are printed when you run{' '}
              <code>wooagent run</code>.
            </Text>
          </Stack>

          {error && (
            <Notice.Root intent="error">
              <Notice.Description>{error}</Notice.Description>
            </Notice.Root>
          )}

          <TextControl
            label="WooAgent URL"
            value={daemonUrl}
            onChange={(v: string | undefined) => setDaemonUrl(v ?? '')}
            placeholder={DEFAULT_URL}
            __next40pxDefaultSize
            __nextHasNoMarginBottom
          />
          <TextControl
            label="Auth token"
            value={token}
            onChange={(v: string | undefined) => setToken(v ?? '')}
            placeholder="wo_pat_…"
            type="password"
            __next40pxDefaultSize
            __nextHasNoMarginBottom
          />

          <Stack direction="row" align="center">
            <Button
              variant="primary"
              __next40pxDefaultSize
              disabled={busy || !daemonUrl.trim() || !token.trim()}
              onClick={() => submit()}
            >
              {busy ? 'Connecting…' : 'Connect'}
            </Button>
          </Stack>
        </Stack>
      </Card.Content>
    </Card.Root>
  );
}
