import { useEffect, useState } from 'react';
import { Badge, Card, Notice, Stack, Text } from '@wordpress/ui';
import { Button, Modal, Spinner } from '@wordpress/components';
import { plus } from '@wordpress/icons';
import { useNavigate } from 'react-router-dom';
import { Page } from '@wordpress/admin-ui';
import {
  api,
  clearConnection,
  isEmbedded,
  type Connection,
  type ModelProvider,
  type Store,
} from '../api/client';
import PageGlobalActions from '../components/PageGlobalActions';

interface Props {
  connection: Connection;
  onDisconnect(): void;
  onStoreDisconnected(): void;
  onAskAgent: () => void;
}

const MUTED = { color: 'var(--wpds-color-fg-content-neutral-weak)' } as const;

function providerLabel(p: ModelProvider): string {
  if (p.name) return p.name;
  const kind =
    p.kind === 'anthropic' ? 'Anthropic'
    : p.kind === 'openai' ? 'OpenAI'
    : 'Ollama';
  return `${kind} · ${p.default_model}`;
}

function relativeTime(iso?: string): string {
  if (!iso) return 'never tested';
  const then = new Date(iso).getTime();
  if (!Number.isFinite(then)) return 'never tested';
  const diffMs = Date.now() - then;
  const min = Math.round(diffMs / 60_000);
  if (min < 1) return 'just now';
  if (min < 60) return `${min}m ago`;
  const hr = Math.round(min / 60);
  if (hr < 24) return `${hr}h ago`;
  const days = Math.round(hr / 24);
  return `${days}d ago`;
}

export default function Settings({
  connection,
  onDisconnect,
  onStoreDisconnected,
  onAskAgent,
}: Props) {
  const [providers, setProviders] = useState<ModelProvider[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [stores, setStores] = useState<Store[] | null>(null);
  const [storesError, setStoresError] = useState<string | null>(null);
  const [confirmStore, setConfirmStore] = useState<Store | null>(null);
  const [disconnecting, setDisconnecting] = useState(false);
  const [disconnectError, setDisconnectError] = useState<string | null>(null);
  const navigate = useNavigate();

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await api.modelProviders.list(connection);
        if (!cancelled) setProviders(res.providers);
      } catch (e) {
        if (!cancelled) {
          setError(e instanceof Error ? e.message : String(e));
          setProviders([]);
        }
      }
    })();
    return () => { cancelled = true; };
  }, [connection]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await api.stores.list(connection);
        if (!cancelled) setStores(res.stores);
      } catch (e) {
        if (!cancelled) {
          setStoresError(e instanceof Error ? e.message : String(e));
          setStores([]);
        }
      }
    })();
    return () => { cancelled = true; };
  }, [connection]);

  const disconnect = () => {
    clearConnection();
    onDisconnect();
  };

  const handleDisconnectStore = async () => {
    if (!confirmStore) return;
    setDisconnecting(true);
    setDisconnectError(null);
    try {
      await api.stores.delete(connection, confirmStore.id);
      setConfirmStore(null);
      onStoreDisconnected();
      navigate('/');
    } catch (e) {
      const code = (e as { code?: string }).code;
      if (code === 'store_not_found') {
        setDisconnectError('This store is already disconnected. Refresh to update.');
      } else if (code === 'keychain_error') {
        setDisconnectError(
          'Could not clear the local keychain entry. Try again, or remove the device from your WordPress admin and retry.',
        );
      } else {
        setDisconnectError(
          e instanceof Error ? e.message : 'Disconnect failed. Try again.',
        );
      }
    } finally {
      setDisconnecting(false);
    }
  };

  const embedded = isEmbedded();

  return (
    <Page
      title="Settings"
      subTitle="Model providers and WooAgent connection."
      actions={<PageGlobalActions onAskAgent={onAskAgent} />}
      hasPadding
    >
      <Stack direction="column" gap="xl">
        {/* Models — multiple-provider management */}
        <Card.Root>
          <Card.Header>
            <Stack
              direction="row"
              justify="space-between"
              align="center"
              style={{ width: '100%' }}
            >
              <Stack direction="column" gap="xs">
                <Text variant="heading-sm">Models</Text>
                <Text variant="body-sm" style={MUTED}>
                  The LLM providers your agents use. The default is the
                  fallback when a persona doesn't override it.
                </Text>
              </Stack>
              <Button variant="primary" icon={plus} __next40pxDefaultSize>
                Add model
              </Button>
            </Stack>
          </Card.Header>
          <Card.Content>
            {!providers && !error && (
              <Stack direction="row" gap="sm" align="center">
                <Spinner />
                <Text variant="body-sm" style={MUTED}>Loading providers…</Text>
              </Stack>
            )}

            {error && (
              <Notice.Root intent="error">
                <Notice.Description>{error}</Notice.Description>
              </Notice.Root>
            )}

            {providers && providers.length === 0 && !error && (
              <Text variant="body-sm" style={MUTED}>
                No providers configured. Add one to get your agents running.
              </Text>
            )}

            {providers && providers.length > 0 && (
              <ul className="wa-model-list">
                {providers.map((p) => (
                  <li key={p.id} className="wa-model-row">
                    <Stack direction="column" gap="xs">
                      <Stack direction="row" gap="sm" align="center">
                        <Text
                          variant="body-md"
                          style={{
                            fontWeight:
                              'var(--wpds-typography-font-weight-medium)',
                          }}
                        >
                          {providerLabel(p)}
                        </Text>
                        {p.is_default && (
                          <span className="wa-default-badge">Default</span>
                        )}
                      </Stack>
                      <Text variant="body-sm" style={MUTED}>
                        {p.last_test_status === 'ok'
                          ? `Connected · last tested ${relativeTime(p.last_tested_at)}`
                          : p.last_test_status === 'failed'
                            ? 'Connection failed — re-test required'
                            : 'Not yet tested'}
                      </Text>
                    </Stack>
                    <Stack direction="row" gap="xs">
                      {!p.is_default && (
                        <Button variant="tertiary" __next40pxDefaultSize>
                          Set as default
                        </Button>
                      )}
                      <Button variant="tertiary" __next40pxDefaultSize>
                        Edit
                      </Button>
                      <Button variant="tertiary" __next40pxDefaultSize>
                        Delete
                      </Button>
                    </Stack>
                  </li>
                ))}
              </ul>
            )}
          </Card.Content>
        </Card.Root>

        {stores && stores.length > 0 && (
          <Card.Root>
            <Card.Header>
              <Text variant="body-sm" style={MUTED}>CONNECTED STORE</Text>
            </Card.Header>
            <Card.Content>
              <Stack direction="column" gap="md">
                {stores.map((store) => (
                  <Stack key={store.id} direction="column" gap="sm">
                    <Stack direction="column" gap="xs">
                      <Text variant="body-sm" style={MUTED}>STORE URL</Text>
                      <Text variant="body-md">{store.url}</Text>
                    </Stack>
                    <Stack direction="row" gap="md" align="center">
                      <Badge
                        intent={
                          store.status === 'paired' ? 'stable'
                          : store.status === 'pairing' ? 'informational'
                          : 'low'
                        }
                      >
                        {store.status}
                      </Badge>
                      {store.paired_at && (
                        <Text variant="body-sm" style={MUTED}>
                          Paired {new Date(store.paired_at).toLocaleDateString()}
                        </Text>
                      )}
                    </Stack>
                    <Stack direction="row">
                      <Button
                        variant="secondary"
                        isDestructive
                        __next40pxDefaultSize
                        onClick={() => {
                          setDisconnectError(null);
                          setConfirmStore(store);
                        }}
                      >
                        Disconnect store
                      </Button>
                    </Stack>
                  </Stack>
                ))}
              </Stack>
            </Card.Content>
          </Card.Root>
        )}
        {storesError && stores?.length === 0 && (
          <Notice.Root intent="error">
            <Notice.Description>
              Could not load the connected store. Refresh to retry.
            </Notice.Description>
          </Notice.Root>
        )}

        {/* Connection — daemon URL + token */}
        <Card.Root>
          <Card.Header>
            <Text variant="heading-sm">Connection</Text>
          </Card.Header>
          <Card.Content>
            <Stack direction="column" gap="md">
              <Stack direction="column" gap="xs">
                <Text variant="body-sm" style={MUTED}>WOOAGENT URL</Text>
                <Text variant="body-md" className="wa-mono">
                  {connection.daemonUrl}
                </Text>
              </Stack>
              <Stack direction="column" gap="xs">
                <Text variant="body-sm" style={MUTED}>TOKEN</Text>
                <Text variant="body-md" className="wa-mono">
                  {connection.token.slice(0, 12)}…
                </Text>
              </Stack>
              {embedded ? (
                <Text variant="body-sm" style={MUTED}>
                  This UI is served by the local WooAgent daemon. Stop{' '}
                  <code>wooagent run</code> in your terminal to disconnect.
                </Text>
              ) : (
                <Stack direction="row">
                  <Button
                    variant="secondary"
                    onClick={disconnect}
                    __next40pxDefaultSize
                  >
                    Forget this connection
                  </Button>
                </Stack>
              )}
            </Stack>
          </Card.Content>
        </Card.Root>

        <Notice.Root intent="info">
          <Notice.Description>
            Guardrails, redaction rules, and per-persona opt-in settings land
            in later phases.
          </Notice.Description>
        </Notice.Root>
      </Stack>

      {confirmStore && (
        <Modal
          title="Disconnect this store?"
          onRequestClose={() => {
            if (!disconnecting) setConfirmStore(null);
          }}
          shouldCloseOnClickOutside={!disconnecting}
          shouldCloseOnEsc={!disconnecting}
        >
          <Stack direction="column" gap="md">
            <Text variant="body-md">
              You'll need to re-pair from the WordPress admin to reconnect.
              Any in-flight routines tied to {confirmStore.url} will stop.
            </Text>
            {disconnectError && (
              <Notice.Root intent="error">
                <Notice.Description>{disconnectError}</Notice.Description>
              </Notice.Root>
            )}
            <Stack direction="row" gap="sm" justify="flex-end">
              <Button
                variant="tertiary"
                __next40pxDefaultSize
                disabled={disconnecting}
                onClick={() => setConfirmStore(null)}
              >
                Cancel
              </Button>
              <Button
                variant="primary"
                isDestructive
                __next40pxDefaultSize
                disabled={disconnecting}
                onClick={handleDisconnectStore}
              >
                {disconnecting ? 'Disconnecting…' : 'Disconnect'}
              </Button>
            </Stack>
          </Stack>
        </Modal>
      )}
    </Page>
  );
}
