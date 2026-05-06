import { useEffect, useState } from 'react';
import { Card, Stack, Text } from '@wordpress/ui';
import { Button, Notice, Spinner } from '@wordpress/components';
import { Icon, check } from '@wordpress/icons';
import {
  api,
  type AbilitiesResponse,
  type Connection,
  type Store,
} from '../api/client';

interface Props {
  connection: Connection;
  store: Store;
  onContinue(): void;
  onBack(): void;
}

const MUTED = { color: 'var(--wpds-color-fg-content-neutral-weak)' } as const;

export default function Step3Discovery({
  connection,
  store,
  onContinue,
  onBack,
}: Props) {
  const [data, setData] = useState<AbilitiesResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await api.abilities.list(connection, store.id);
        if (!cancelled) setData(res);
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [connection, store.id]);

  const host = (() => {
    try {
      return new URL(store.url).host;
    } catch {
      return store.url;
    }
  })();

  return (
    <Card.Root>
      <Card.Content>
        <Stack direction="column" gap="xl">
          <Stack direction="column" gap="sm">
            <Text variant="heading-md">What did we find?</Text>
            <Text variant="body-sm" style={MUTED}>
              Connected to <strong>{host}</strong>. Here's what its plugins
              expose to agents.
            </Text>
          </Stack>

          {error && (
            <Notice status="error" isDismissible={false}>
              {error}
            </Notice>
          )}

          {!data && !error && (
            <Stack direction="row" align="center" gap="md">
              <Spinner />
              <Text variant="body-sm" style={MUTED}>
                Discovering abilities…
              </Text>
            </Stack>
          )}

          {data && data.total === 0 && (
            <Notice status="warning" isDismissible={false}>
              We connected to your store but didn't find any abilities. This
              usually means the MCP Adapter is installed but no plugin has
              registered abilities yet.
            </Notice>
          )}

          {data && data.total > 0 && (
            <Stack
              direction="column"
              gap="md"
              style={{
                padding: 'var(--wpds-dimension-padding-md)',
                background: 'var(--wpds-color-bg-surface-neutral-weak)',
                borderRadius: 'var(--wpds-border-radius-md)',
              }}
            >
              <Text variant="body-sm" style={MUTED}>
                Discovered{' '}
                <span className="wa-mono">
                  <strong>{data.total}</strong>
                </span>{' '}
                abilities across{' '}
                <span className="wa-mono">
                  <strong>{data.by_plugin.length}</strong>
                </span>{' '}
                plugins.
              </Text>
              <ul className="wa-onboarding-plugin-list">
                {data.by_plugin.map((p) => (
                  <li key={p.plugin}>
                    <span className="wa-mono">{p.plugin}</span>
                    <span className="wa-onboarding-plugin-list__count">
                      {p.count} abilities
                    </span>
                    {p.signed && p.unapproved === 0 && (
                      <span className="wa-onboarding-plugin-list__tag wa-onboarding-plugin-list__tag--ok">
                        <Icon icon={check} size={12} /> pre-signed
                      </span>
                    )}
                    {p.preview && (
                      <span className="wa-onboarding-plugin-list__tag wa-onboarding-plugin-list__tag--info">
                        developer preview
                      </span>
                    )}
                    {p.unapproved > 0 && (
                      <span className="wa-onboarding-plugin-list__tag wa-onboarding-plugin-list__tag--warn">
                        {p.unapproved} need review
                      </span>
                    )}
                  </li>
                ))}
              </ul>
            </Stack>
          )}

          {data && data.total > 0 && (
            <Text variant="body-sm" style={MUTED}>
              You can review and approve the unapproved abilities later from
              the Abilities screen. For now, your three v0.1 personas have
              everything they need.
            </Text>
          )}

          <Stack direction="row" justify="space-between" align="center">
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
              disabled={!data}
              onClick={onContinue}
            >
              Continue
            </Button>
          </Stack>
        </Stack>
      </Card.Content>
    </Card.Root>
  );
}
