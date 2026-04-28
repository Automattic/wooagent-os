import { useNavigate } from 'react-router-dom';
import { Card, Stack, Text } from '@wordpress/ui';
import { Button } from '@wordpress/components';

interface Props {
  area: string;
  description: string;
  status?: 'soon' | 'demo';
  ctaTo?: string;
  ctaLabel?: string;
}

export default function Placeholder({
  area,
  description,
  status = 'soon',
  ctaTo = '/',
  ctaLabel = '← Back to Board',
}: Props) {
  const nav = useNavigate();
  return (
    <main
      style={{
        maxWidth: 900,
        margin: '0 auto',
        padding: 'var(--wpds-dimension-padding-3xl) var(--wa-page-pad-x)',
      }}
    >
      <Card.Root
        style={{
          background:
            'radial-gradient(circle at 30% 20%, var(--wa-persona-mk-bg) 0%, transparent 60%), var(--wpds-color-bg-surface-neutral)',
        }}
      >
        <Card.Content>
          <Stack direction="column" gap="md" align="center" style={{ padding: 'var(--wpds-dimension-padding-2xl) 0', textAlign: 'center' }}>
            <div
              className="wa-eyebrow"
              style={{
                color:
                  status === 'soon'
                    ? 'var(--wa-persona-mk-ink)'
                    : 'var(--wpds-color-fg-interactive-brand)',
              }}
            >
              {status === 'soon' ? 'Out of scope · prototype' : 'Reference area'}
            </div>
            <Text variant="heading-xl" render={<h1 />}>
              {area}
            </Text>
            <Text variant="body-md" style={{ maxWidth: 560, color: 'var(--wpds-color-fg-content-neutral-weak)' }}>
              {description}
            </Text>
            <Button variant="primary" onClick={() => nav(ctaTo)}>
              {ctaLabel}
            </Button>
          </Stack>
        </Card.Content>
      </Card.Root>
    </main>
  );
}
