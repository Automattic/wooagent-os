import { useEffect, useRef, useState } from 'react';
import { Card, Stack, Text } from '@wordpress/ui';
import {
  Button,
  ExternalLink,
  Notice,
  Spinner,
  TextControl,
} from '@wordpress/components';
import { Icon, check } from '@wordpress/icons';
import { api, type Connection, type Store } from '../api/client';

interface Props {
  connection: Connection;
  onPaired(store: Store): void;
}

type Phase =
  | { kind: 'idle' }
  | { kind: 'creating' }
  | { kind: 'pairing'; store: Store; regens: number }
  | { kind: 'paired'; store: Store }
  | { kind: 'expired_hard' };

const POLL_INTERVAL_MS = 2000;
const MAX_REGENS = 1; // brief §6 — auto-regen once, hard-fail on second timeout
const MUTED = { color: 'var(--wpds-color-fg-content-neutral-weak)' } as const;

export default function Step2Store({ connection, onPaired }: Props) {
  const [storeUrl, setStoreUrl] = useState('');
  const [phase, setPhase] = useState<Phase>({ kind: 'idle' });
  const [error, setError] = useState<string | null>(null);
  const [pluginPanelOpen, setPluginPanelOpen] = useState(false);
  const [now, setNow] = useState(() => Date.now());
  const pollTimer = useRef<number | null>(null);
  const tickTimer = useRef<number | null>(null);

  // Resume in-flight pairing if the operator reloads mid-flow.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const { stores } = await api.stores.list(connection);
        if (cancelled) return;
        const paired = stores.find((s) => s.status === 'paired');
        if (paired) {
          setPhase({ kind: 'paired', store: paired });
          return;
        }
        const pending = stores.find((s) => s.status === 'pairing');
        if (pending) setPhase({ kind: 'pairing', store: pending, regens: 0 });
      } catch {
        /* fall through to idle */
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [connection]);

  // Poll the daemon while we're waiting for wp-admin approval. Stops
  // automatically on paired / expired / unmount.
  useEffect(() => {
    if (phase.kind !== 'pairing') return;
    let cancelled = false;
    const tick = async () => {
      try {
        const updated = await api.stores.get(connection, phase.store.id);
        if (cancelled) return;
        if (updated.status === 'paired') {
          setPhase({ kind: 'paired', store: updated });
          onPaired(updated);
        }
      } catch (e) {
        if (!cancelled) {
          setError(e instanceof Error ? e.message : String(e));
        }
      }
    };
    pollTimer.current = window.setInterval(tick, POLL_INTERVAL_MS);
    return () => {
      cancelled = true;
      if (pollTimer.current) window.clearInterval(pollTimer.current);
    };
  }, [phase, connection, onPaired]);

  // Countdown ticker so the "expires in 9:43" label re-renders each second.
  useEffect(() => {
    if (phase.kind !== 'pairing') return;
    tickTimer.current = window.setInterval(() => setNow(Date.now()), 1000);
    return () => {
      if (tickTimer.current) window.clearInterval(tickTimer.current);
    };
  }, [phase]);

  // Expiry handling — auto-regen once, then surface a hard error per brief §6.
  useEffect(() => {
    if (phase.kind !== 'pairing') return;
    if (!phase.store.expires_at) return;
    const expiresAt = new Date(phase.store.expires_at).getTime();
    if (now < expiresAt) return;

    if (phase.regens >= MAX_REGENS) {
      setPhase({ kind: 'expired_hard' });
      return;
    }
    void (async () => {
      try {
        const fresh = await api.stores.create(connection, phase.store.url);
        setPhase({
          kind: 'pairing',
          store: fresh,
          regens: phase.regens + 1,
        });
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      }
    })();
  }, [now, phase, connection]);

  const startPairing = async () => {
    setError(null);
    setPhase({ kind: 'creating' });
    try {
      const created = await api.stores.create(connection, storeUrl.trim());
      setPhase({ kind: 'pairing', store: created, regens: 0 });
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      setPhase({ kind: 'idle' });
    }
  };

  if (phase.kind === 'paired') {
    const deviceLabel = (() => {
      if (phase.store.device_name) return phase.store.device_name;
      try {
        return new URL(phase.store.url).host;
      } catch {
        return phase.store.url;
      }
    })();
    return (
      <Card.Root>
        <Card.Content>
          <Stack direction="column" gap="lg" align="flex-start">
            <Stack direction="row" align="center" gap="md">
              <span className="wa-done-check" aria-hidden="true">
                <Icon icon={check} size={18} />
              </span>
              <Stack direction="column" gap="xs">
                <Text variant="heading-md">Paired with {deviceLabel}</Text>
                <Text variant="body-sm" style={MUTED}>
                  The Companion Plugin will keep the connection open for this
                  device.
                </Text>
              </Stack>
            </Stack>
            <Button
              variant="primary"
              __next40pxDefaultSize
              onClick={() => onPaired(phase.store)}
            >
              Continue
            </Button>
          </Stack>
        </Card.Content>
      </Card.Root>
    );
  }

  if (phase.kind === 'pairing') {
    const expiresAt = phase.store.expires_at
      ? new Date(phase.store.expires_at).getTime()
      : 0;
    const remainingMs = Math.max(0, expiresAt - now);
    const mm = Math.floor(remainingMs / 60_000);
    const ss = Math.floor((remainingMs % 60_000) / 1000);
    const countdown = `${mm}:${String(ss).padStart(2, '0')}`;

    return (
      <Card.Root>
        <Card.Content>
          <Stack direction="column" gap="xl">
            <Stack direction="column" gap="sm">
              <Text variant="heading-md">Approve pairing in wp-admin</Text>
              <Text variant="body-sm" style={MUTED}>
                Open this URL in your browser, type the code below into the
                WooAgent OS pairing screen, and click Approve.
              </Text>
            </Stack>

            <Stack direction="column" gap="sm">
              <Text variant="body-sm" style={MUTED}>
                PAIRING CODE
              </Text>
              <button
                type="button"
                className="wa-onboarding-pairing-code"
                onClick={() =>
                  phase.store.pairing_code &&
                  navigator.clipboard
                    .writeText(phase.store.pairing_code)
                    .catch(() => {})
                }
                aria-label="Copy pairing code"
              >
                {phase.store.pairing_code ?? '— — — —'}
              </button>
              <Text variant="body-sm" style={MUTED}>
                Click to copy. Expires in{' '}
                <span className="wa-mono">{countdown}</span>.
              </Text>
            </Stack>

            {phase.store.pair_url && (
              <Stack direction="row" gap="md" align="center">
                <Button
                  variant="primary"
                  __next40pxDefaultSize
                  href={phase.store.pair_url}
                  target="_blank"
                  rel="noreferrer noopener"
                >
                  Open wp-admin → Pair device
                </Button>
              </Stack>
            )}

            {error && (
              <Notice status="error" isDismissible={false}>
                {error}
              </Notice>
            )}

            <Stack
              direction="row"
              align="center"
              gap="sm"
              style={{
                padding:
                  'var(--wpds-dimension-padding-sm) var(--wpds-dimension-padding-md)',
                background: 'var(--wpds-color-bg-surface-neutral-weak)',
                borderRadius: 'var(--wpds-border-radius-md)',
              }}
            >
              <Spinner />
              <Text variant="body-sm" style={MUTED}>
                Waiting for approval in wp-admin…
              </Text>
            </Stack>
          </Stack>
        </Card.Content>
      </Card.Root>
    );
  }

  if (phase.kind === 'expired_hard') {
    return (
      <Card.Root>
        <Card.Content>
          <Stack direction="column" gap="lg">
            <Stack direction="column" gap="sm">
              <Text variant="heading-md">Pairing took too long</Text>
              <Text variant="body-sm" style={MUTED}>
                We regenerated the code once, but it expired again before
                wp-admin approved it. Try again — make sure the Companion
                Plugin is installed and you're signed into wp-admin before
                clicking the deep link.
              </Text>
            </Stack>
            <Button
              variant="primary"
              __next40pxDefaultSize
              onClick={() => setPhase({ kind: 'idle' })}
            >
              Try again
            </Button>
          </Stack>
        </Card.Content>
      </Card.Root>
    );
  }

  return (
    <Card.Root>
      <Card.Content>
        <Stack direction="column" gap="xl">
          <Stack direction="column" gap="sm">
            <Text variant="heading-md">Add your store</Text>
            <Text variant="body-sm" style={MUTED}>
              WooAgent OS connects to your WooCommerce store through the
              WooAgent Companion Plugin. If you haven't installed it yet, do
              that first — it takes about a minute.
            </Text>
          </Stack>

          <Stack
            direction="column"
            gap="sm"
            style={{
              padding: 'var(--wpds-dimension-padding-md)',
              background: 'var(--wpds-color-bg-surface-neutral-weak)',
              border:
                'var(--wpds-border-width-sm) solid var(--wpds-color-stroke-surface-neutral-weak)',
              borderRadius: 'var(--wpds-border-radius-md)',
            }}
          >
            <button
              type="button"
              onClick={() => setPluginPanelOpen((o) => !o)}
              aria-expanded={pluginPanelOpen}
              className="wa-onboarding-disclosure"
            >
              <Text
                variant="body-sm"
                style={{
                  fontWeight: 'var(--wpds-typography-font-weight-medium)',
                }}
              >
                {pluginPanelOpen ? '−' : '+'} Install the Companion Plugin
              </Text>
            </button>
            {pluginPanelOpen && (
              <Stack direction="column" gap="sm">
                <Text variant="body-sm">
                  Download the plugin ZIP, upload it via{' '}
                  <code>Plugins → Add New → Upload</code> in wp-admin, and
                  activate. The MCP endpoint goes live the moment it
                  activates.
                </Text>
                <ExternalLink href="https://wordpress.org/plugins/wooagent-companion/">
                  Get the WooAgent Companion Plugin
                </ExternalLink>
              </Stack>
            )}
          </Stack>

          {error && (
            <Notice status="error" isDismissible={false}>
              {error}
            </Notice>
          )}

          <TextControl
            label="Store URL"
            value={storeUrl}
            onChange={(v: string | undefined) => setStoreUrl(v ?? '')}
            placeholder="https://mystore.com"
            help="The front-page URL of your WooCommerce site — not the MCP endpoint."
            __next40pxDefaultSize
            __nextHasNoMarginBottom
          />

          <Stack direction="row" align="center">
            <Button
              variant="primary"
              __next40pxDefaultSize
              disabled={phase.kind === 'creating' || !storeUrl.trim()}
              onClick={startPairing}
            >
              {phase.kind === 'creating' ? 'Detecting…' : 'Pair this store'}
            </Button>
          </Stack>
        </Stack>
      </Card.Content>
    </Card.Root>
  );
}
