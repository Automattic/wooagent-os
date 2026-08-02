import { useEffect, useState } from 'react';
import { Badge, Card, Notice, Stack, Text } from '@wordpress/ui';
import { Button, Modal } from '@wordpress/components';
import { useNavigate } from 'react-router-dom';
import { Page } from '@wordpress/admin-ui';
import { api, type Connection, type Store } from '../api/client';
import PageGlobalActions from '../components/PageGlobalActions';
import { useAskAgentContext } from '../lib/askAgent';
import { formatDateTime } from '../lib/boardItems';
import { storeToVisible } from '../lib/visibleItems';

interface Props {
  connection: Connection;
  onStoreDisconnected(): void;
  onAskAgent: () => void;
}

const MUTED = { color: 'var(--wpds-color-foreground-content-neutral-weak)' } as const;

export default function Stores({
  connection,
  onStoreDisconnected,
  onAskAgent,
}: Props) {
  const [stores, setStores] = useState<Store[] | null>(null);
  const [storesError, setStoresError] = useState<string | null>(null);
  const [confirmStore, setConfirmStore] = useState<Store | null>(null);
  const [disconnecting, setDisconnecting] = useState(false);
  const [disconnectError, setDisconnectError] = useState<string | null>(null);

  useAskAgentContext(
    () => ({
      page: 'stores',
      visible_items: (stores ?? []).map(storeToVisible),
    }),
    [stores],
  );
  const navigate = useNavigate();

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

  return (
    <Page
      title="Stores"
      subTitle="WooCommerce stores paired with WooAgent."
      actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
      hasPadding
    >
      <Stack direction="column" gap="xl">
        {stores && stores.length > 0 && (
          <Card.Root>
            <Card.Header>
              <Text variant="heading-sm">Connected store</Text>
            </Card.Header>
            <Card.Content>
              <Stack direction="column" gap="md">
                {stores.map((store) => (
                  <Stack key={store.id} direction="column" gap="md">
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
                          Paired {formatDateTime(store.paired_at)}
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

        <Notice.Root intent="info">
          <Notice.Description>
            Guardrails, redaction rules, and per-agent opt-in settings land
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
