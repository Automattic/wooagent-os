import { useEffect, useState } from 'react';
import { Card, Stack, Text } from '@wordpress/ui';
import { Button, Notice, Spinner } from '@wordpress/components';
import { Icon, plus } from '@wordpress/icons';
import {
  api,
  clearConnection,
  type Connection,
  type ModelProvider,
} from '../api/client';

interface Props {
  connection: Connection;
  onDisconnect(): void;
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

export default function Settings({ connection, onDisconnect }: Props) {
  const [providers, setProviders] = useState<ModelProvider[] | null>(null);
  const [error, setError] = useState<string | null>(null);

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

  const disconnect = () => {
    clearConnection();
    onDisconnect();
  };

  return (
    <div className="wa-settings">
      <Stack direction="column" gap="xl">
        <Text variant="heading-lg">Settings</Text>

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
              <Button variant="primary" __next40pxDefaultSize>
                <Stack direction="row" gap="xs" align="center">
                  <Icon icon={plus} size={16} /> Add model
                </Stack>
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
              <Notice status="error" isDismissible={false}>{error}</Notice>
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
              <Stack direction="row">
                <Button
                  variant="secondary"
                  onClick={disconnect}
                  __next40pxDefaultSize
                >
                  Forget this connection
                </Button>
              </Stack>
            </Stack>
          </Card.Content>
        </Card.Root>

        <Notice status="info" isDismissible={false}>
          Guardrails, redaction rules, and per-persona opt-in settings land
          in later phases.
        </Notice>
      </Stack>
    </div>
  );
}
