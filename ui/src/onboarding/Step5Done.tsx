import { Card, Stack, Text } from '@wordpress/ui';
import { Button } from '@wordpress/components';
import { Icon, check } from '@wordpress/icons';
import type { ModelProvider, Store } from '../api/client';

interface Props {
  store: Store | null;
  provider: ModelProvider | null;
  onOpenKanban(): void;
}

const MUTED = { color: 'var(--wpds-color-fg-content-neutral-weak)' } as const;

export default function Step5Done({ store, provider, onOpenKanban }: Props) {
  const host = (() => {
    if (!store) return '—';
    try {
      return new URL(store.url).host;
    } catch {
      return store.url;
    }
  })();

  const providerLabel = (() => {
    if (!provider) return '—';
    const kind =
      provider.kind === 'anthropic'
        ? 'Anthropic'
        : provider.kind === 'openai'
          ? 'OpenAI'
          : 'Ollama';
    return `${kind} · ${provider.default_model}`;
  })();

  const items: { label: string; value: string }[] = [
    { label: 'Connected to', value: host },
    {
      label: 'Discovered',
      value:
        store?.ability_count != null ? `${store.ability_count} abilities` : '—',
    },
    { label: 'Using', value: providerLabel },
    {
      label: 'Default fleet',
      value: 'Marketing, Pricing, Sales support',
    },
    {
      label: 'Chief of staff',
      value: 'Generating your store profile…',
    },
  ];

  return (
    <Card.Root>
      <Card.Content>
        <Stack direction="column" gap="xl">
          <Stack direction="column" gap="sm">
            <Text variant="heading-md">You're set up</Text>
            <Text variant="body-sm" style={MUTED}>
              Inventory, Accounting, and Reporting personas are coming soon.
              Your three-persona fleet is ready to work now.
            </Text>
          </Stack>

          <ul className="wa-onboarding-checklist">
            {items.map((it) => (
              <li key={it.label}>
                <span
                  className="wa-onboarding-checklist__icon"
                  aria-hidden="true"
                >
                  <Icon icon={check} size={14} />
                </span>
                <span className="wa-onboarding-checklist__label">
                  {it.label}
                </span>
                <span className="wa-onboarding-checklist__value">
                  {it.value}
                </span>
              </li>
            ))}
          </ul>

          <Stack direction="row" align="center">
            <Button
              variant="primary"
              __next40pxDefaultSize
              onClick={onOpenKanban}
            >
              Open the kanban
            </Button>
          </Stack>
        </Stack>
      </Card.Content>
    </Card.Root>
  );
}
