import { useEffect, useState } from 'react';
import { Notice, Stack, Text } from '@wordpress/ui';
import { Button, Spinner } from '@wordpress/components';
import { plus } from '@wordpress/icons';
import { Page } from '@wordpress/admin-ui';
import { api, type Connection, type ModelProvider } from '../api/client';
import PageGlobalActions from '../components/PageGlobalActions';

interface Props {
  connection: Connection;
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

export default function Models({ connection, onAskAgent }: Props) {
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

  return (
    <Page
      title="Models"
      subTitle="The LLM providers your agents use. The default is the fallback when a persona doesn't override it."
      actions={
        <Stack direction="row" align="center" gap="md">
          <Button variant="primary" icon={plus} __next40pxDefaultSize>
            Add model
          </Button>
          <PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />
        </Stack>
      }
      hasPadding
    >
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
    </Page>
  );
}
