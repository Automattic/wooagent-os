import { useNavigate } from 'react-router-dom';
import { Card, Stack, Text } from '@wordpress/ui';
import { Button } from '@wordpress/components';
import { Page } from '@wordpress/admin-ui';
import PageGlobalActions from '../components/PageGlobalActions';
import { useAskAgentContext } from '../lib/askAgent';

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
  ctaLabel = 'Back to Board',
  onAskAgent,
}: Props) {
  const nav = useNavigate();
  useAskAgentContext(
    () => ({ page: `placeholder:${area}`, visible_items: [] }),
    [area],
  );
  return (
    <Page
      title={area}
      subTitle={
        status === 'soon'
          ? 'Out of scope for phase 1.'
          : 'Reference area.'
      }
      actions={<PageGlobalActions onAskAgent={onAskAgent} showSearch={false} />}
      hasPadding
    >
      <Card.Root
        style={{
          background:
            'radial-gradient(circle at 30% 20%, var(--wpds-color-background-surface-info-weak) 0%, transparent 60%), var(--wpds-color-background-surface-neutral)',
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
                    ? 'var(--wpds-color-foreground-content-info)'
                    : 'var(--wpds-color-foreground-interactive-brand)',
              }}
            >
              {status === 'soon' ? 'Out of scope · phase 1' : 'Reference area'}
            </div>
            <Text variant="heading-xl" render={<h1 />}>
              {area}
            </Text>
            <Text
              variant="body-md"
              style={{ maxWidth: 560, color: 'var(--wpds-color-foreground-content-neutral-weak)' }}
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
