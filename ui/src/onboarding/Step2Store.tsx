import { useEffect, useRef, useState } from 'react';
import { Card, Notice, Stack, Text } from '@wordpress/ui';
import {
  Button,
  Spinner,
  TextControl,
} from '@wordpress/components';
import {
  Icon,
  arrowUpRight,
  copy as copyIcon,
} from '@wordpress/icons';
import { api, type Connection, type Store } from '../api/client';

interface Props {
  connection: Connection;
  onPaired(store: Store): void;
  onBack(): void;
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

export default function Step2Store({ connection, onPaired, onBack }: Props) {
  const [storeUrl, setStoreUrl] = useState('');
  const [phase, setPhase] = useState<Phase>({ kind: 'idle' });
  const [error, setError] = useState<string | null>(null);
  const [pluginPanelOpen, setPluginPanelOpen] = useState(false);
  const [now, setNow] = useState(() => Date.now());
  const [copied, setCopied] = useState(false);
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
          <Stack direction="column" gap="xl">
            <Stack direction="column" gap="sm">
              <Text variant="heading-md">Approve pairing</Text>
              <Text variant="body-sm" style={MUTED}>
                Open the WooAgent Companion Plugin in wp-admin, type the code
                below into the WooAgent OS pairing screen, and click Approve.
              </Text>
            </Stack>

            <Notice.Root intent="success">
              <Notice.Title>Paired with {deviceLabel}</Notice.Title>
              <Notice.Description>
                The Companion Plugin will keep the connection open for this
                device.
              </Notice.Description>
            </Notice.Root>

            <Stack direction="row" justify="flex-end" align="center" gap="sm">
              <Button
                variant="tertiary"
                __next40pxDefaultSize
                onClick={onBack}
              >
                Back
              </Button>
              <Button
                variant="primary"
                __next40pxDefaultSize
                onClick={() => onPaired(phase.store)}
              >
                Continue
              </Button>
            </Stack>
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
              <Text variant="heading-md">Approve pairing</Text>
              <Text variant="body-sm" style={MUTED}>
                Open the WooAgent Companion Plugin in wp-admin, type the code
                below into the WooAgent OS pairing screen, and click Approve.
              </Text>
            </Stack>

            {error && (
              <Notice.Root intent="error">
                <Notice.Description>{error}</Notice.Description>
              </Notice.Root>
            )}

            <Stack direction="column" gap="sm">
              <Text variant="body-sm" style={MUTED}>
                PAIRING CODE
              </Text>
              {/* CUSTOM: pairing-code display as a click-to-copy target. (a) WPDS Button chrome doesn't fit prominent code display. (b) <button> with .wa-onboarding-pairing-code chrome rendering the code as its label. (c) Follow-up: revisit if WPDS adds a CodeDisplay / copyable-token primitive. */}
              <button
                type="button"
                className="wa-onboarding-pairing-code"
                onClick={() => {
                  if (!phase.store.pairing_code) return;
                  navigator.clipboard
                    .writeText(phase.store.pairing_code)
                    .catch(() => {});
                  setCopied(true);
                  window.setTimeout(() => setCopied(false), 1500);
                }}
                aria-label="Copy pairing code"
              >
                {phase.store.pairing_code ?? '— — — —'}
                <span
                  className="wa-onboarding-pairing-code__hover-icon"
                  aria-hidden="true"
                >
                  <Icon icon={copyIcon} size={20} />
                </span>
              </button>
              <Text variant="body-sm" style={MUTED}>
                {copied ? 'Copied!' : `Click to copy. Expires in ${countdown}.`}
              </Text>
            </Stack>

            <Notice.Root intent="info" icon={null}>
              <Notice.Description
                style={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 'var(--wpds-dimension-gap-sm)',
                }}
              >
                <Spinner style={{ margin: 0 }} />
                Waiting for approval in wp-admin…
              </Notice.Description>
            </Notice.Root>

            <Stack direction="row" justify="flex-end" align="center" gap="sm">
              <Button
                variant="tertiary"
                __next40pxDefaultSize
                onClick={onBack}
              >
                Back
              </Button>
              <Button
                variant="secondary"
                __next40pxDefaultSize
                onClick={async () => {
                  setError(null);
                  setCopied(false);
                  try {
                    const fresh = await api.stores.create(
                      connection,
                      phase.store.url,
                    );
                    setPhase({ kind: 'pairing', store: fresh, regens: 0 });
                  } catch (e) {
                    setError(
                      e instanceof Error ? e.message : String(e),
                    );
                  }
                }}
              >
                Get new code
              </Button>
              {phase.store.pair_url && (
                <Button
                  variant="primary"
                  __next40pxDefaultSize
                  href={phase.store.pair_url}
                  target="_blank"
                  rel="noreferrer noopener"
                >
                  Open in wp-admin
                </Button>
              )}
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
            <Stack direction="row" justify="flex-end" align="center" gap="sm">
              <Button
                variant="tertiary"
                __next40pxDefaultSize
                onClick={onBack}
              >
                Back
              </Button>
              <Button
                variant="primary"
                __next40pxDefaultSize
                onClick={async () => {
                  setError(null);
                  setCopied(false);
                  try {
                    const fresh = await api.stores.create(
                      connection,
                      storeUrl.trim(),
                    );
                    setPhase({ kind: 'pairing', store: fresh, regens: 0 });
                  } catch (e) {
                    setError(
                      e instanceof Error ? e.message : String(e),
                    );
                  }
                }}
              >
                Get new code
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
        <Stack direction="column" gap="xl">
          <Stack direction="column" gap="sm">
            <Text variant="heading-md">Connect your store</Text>
            <Text
              variant="body-sm"
              style={{ ...MUTED, textWrap: 'pretty' }}
            >
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
                'var(--wpds-border-width-xs) solid var(--wpds-color-stroke-surface-neutral)',
              borderRadius: 'var(--wpds-border-radius-lg)',
            }}
          >
            {/* CUSTOM: manual disclosure / accordion toggle. (a) WPDS has CollapsibleCard from @wordpress/ui — should fit. (b) <button> with .wa-onboarding-disclosure chrome and aria-expanded. (c) Follow-up: migrate to CollapsibleCard. */}
            <button
              type="button"
              onClick={() => setPluginPanelOpen((o) => !o)}
              aria-expanded={pluginPanelOpen}
              className="wa-onboarding-disclosure"
            >
              <Text
                variant="body-md"
                style={{
                  fontWeight: 'var(--wpds-typography-font-weight-medium)',
                }}
              >
                {pluginPanelOpen ? '−' : '+'} Install the Companion Plugin
              </Text>
            </button>
            {pluginPanelOpen && (
              <Stack direction="column" gap="sm">
                <Text variant="body-md">
                  Download the plugin ZIP, upload it via Plugins &gt; Add
                  new &gt; Upload in wp-admin, and activate.
                </Text>
                <Button
                  variant="link"
                  href="https://wordpress.org/plugins/wooagent-companion/"
                  target="_blank"
                  rel="noreferrer noopener"
                >
                  Get the WooAgent Companion Plugin
                  <Icon
                    icon={arrowUpRight}
                    size={16}
                    style={{
                      verticalAlign: 'text-bottom',
                      marginInlineStart:
                        'var(--wpds-dimension-padding-xs)',
                    }}
                  />
                </Button>
              </Stack>
            )}
          </Stack>

          {error && (
            <Notice.Root intent="error">
              <Notice.Description>{error}</Notice.Description>
            </Notice.Root>
          )}

          <TextControl
            label="Store URL"
            value={storeUrl}
            onChange={(v: string | undefined) => setStoreUrl(v ?? '')}
            placeholder="https://mystore.com"
            help="The front-page URL of your WooCommerce site."
            __next40pxDefaultSize
            __nextHasNoMarginBottom
          />

          <Stack direction="row" justify="flex-end" align="center" gap="sm">
            <Button
              variant="tertiary"
              __next40pxDefaultSize
              onClick={onBack}
            >
              Back
            </Button>
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
