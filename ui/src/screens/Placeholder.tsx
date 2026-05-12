import { useNavigate } from 'react-router-dom';
import { Card, Stack, Text } from '@wordpress/ui';
import { Button } from '@wordpress/components';
import { Page } from '@wordpress/admin-ui';
import PageGlobalActions from '../components/PageGlobalActions';

interface Props {
  area: string;
  description: string;
  status?: 'soon' | 'demo';
  ctaTo?: string;
  ctaLabel?: string;
  onAskAgent: () => void;
}

export default function Placeholder({
  area,
  description,
  status = 'soon',
  ctaTo = '/',
  ctaLabel = '← Back to Board',
  onAskAgent,
}: Props) {
  const nav = useNavigate();
  return (
    <Page
      title={area}
      subTitle={
        status === 'soon'
          ? 'Out of scope for phase 1.'
          : 'Reference area.'
      }
      actions={<PageGlobalActions onAskAgent={onAskAgent} />}
      hasPadding
    >
      <Card.Root
        style={{
          background:
            'radial-gradient(circle at 30% 20%, var(--wpds-color-bg-surface-info-weak) 0%, transparent 60%), var(--wpds-color-bg-surface-neutral)',
        }}
      >
        <Card.Content>
          <Stack
            direction="column"
            gap="md"
            align="center"
            style={{ padding: 'var(--wpds-dimension-padding-2xl) 0', textAlign: 'center' }}
          >
            <div
              className="wa-eyebrow"
              style={{
                color:
                  status === 'soon'
                    ? 'var(--wpds-color-fg-content-info)'
                    : 'var(--wpds-color-fg-interactive-brand)',
              }}
            >
              {status === 'soon' ? 'Out of scope · phase 1' : 'Reference area'}
            </div>
            <Text variant="heading-xl" render={<h1 />}>
              {area}
            </Text>
            <Text
              variant="body-md"
              style={{ maxWidth: 560, color: 'var(--wpds-color-fg-content-neutral-weak)' }}
            >
              {description}
            </Text>
            <Button variant="primary" __next40pxDefaultSize onClick={() => nav(ctaTo)}>
              {ctaLabel}
            </Button>
          </Stack>
        </Card.Content>
      </Card.Root>
    </Page>
  );
}
