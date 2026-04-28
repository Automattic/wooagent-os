import { Card, Stack, Text } from '@wordpress/ui';

// Right-rail content on the IssueDetail screen — Brand voice check, Yoast SEO
// breakdown, and the "Reversible · always" reminder. Daemon doesn't emit any
// of this data yet, so phase 1 ships fixed sample copy that mirrors what the
// agent will produce when scoring lands. Treat the cards as a contract: when
// daemon scoring goes live, the props will replace the sample arrays.

interface BrandTerm {
  word: string;
  kind: 'avoid' | 'prefer';
}

interface VoiceCheckLine {
  text: string;
  terms?: BrandTerm[];
}

interface SeoLine {
  label: string;
  state: 'good' | 'caution' | 'bad';
}

const SAMPLE_VOICE: VoiceCheckLine[] = [
  {
    text: 'Avoids',
    terms: [
      { word: 'luxe', kind: 'avoid' },
      { word: 'premium', kind: 'avoid' },
    ],
  },
  {
    text: 'Uses preferred terms',
    terms: [
      { word: 'small-batch', kind: 'prefer' },
      { word: 'handcrafted', kind: 'prefer' },
    ],
  },
  { text: 'Tone lands between warm and sincere — your target' },
];

const SAMPLE_SEO: SeoLine[] = [
  { label: 'Keyphrase in first sentence', state: 'good' },
  { label: 'Text length', state: 'good' },
  { label: 'Readability', state: 'good' },
  { label: 'Keyphrase density', state: 'caution' },
  { label: 'Passive voice', state: 'good' },
];

function termStyle(kind: BrandTerm['kind']): React.CSSProperties {
  if (kind === 'avoid') {
    return {
      background: 'var(--wpds-color-bg-surface-error-weak)',
      color: 'var(--wpds-color-fg-content-error)',
      padding: '1px 6px',
      borderRadius: 'var(--wpds-border-radius-sm)',
      fontFamily: 'var(--wpds-typography-font-family-mono)',
      fontSize: 12,
    };
  }
  return {
    background: 'var(--wpds-color-bg-surface-success-weak)',
    color: 'var(--wpds-color-fg-content-success)',
    padding: '1px 6px',
    borderRadius: 'var(--wpds-border-radius-sm)',
    fontFamily: 'var(--wpds-typography-font-family-mono)',
    fontSize: 12,
  };
}

function dotStyle(state: SeoLine['state']): React.CSSProperties {
  const color =
    state === 'good'
      ? 'var(--wpds-color-fg-content-success)'
      : state === 'caution'
        ? 'var(--wpds-color-fg-content-caution)'
        : 'var(--wpds-color-fg-content-error)';
  return {
    height: 8,
    width: 8,
    borderRadius: '50%',
    background: color,
    display: 'inline-block',
    flex: 'none',
  };
}

export default function SidebarRail() {
  return (
    <Stack direction="column" gap="md" style={{ width: '100%', flex: 'none' }}>
      <Card.Root>
        <Card.Header>
          <Stack direction="row" justify="space-between" align="center">
            <span className="wa-eyebrow">Brand voice check</span>
            <span
              className="wa-mono"
              style={{ fontSize: 11, color: 'var(--wpds-color-fg-content-neutral-weak)' }}
            >
              0%
            </span>
          </Stack>
        </Card.Header>
        <Card.Content>
          <Stack direction="column" gap="sm">
            {SAMPLE_VOICE.map((line, i) => (
              <Stack key={i} direction="row" gap="sm" align="start">
                <span
                  aria-hidden="true"
                  style={{ color: 'var(--wpds-color-fg-content-success)', flex: 'none', marginTop: 2 }}
                >
                  ✓
                </span>
                <Text variant="body-sm">
                  {line.text}
                  {line.terms?.map((t) => (
                    <span key={t.word}>
                      {' '}
                      <span style={termStyle(t.kind)}>{t.word}</span>
                      {line.terms!.indexOf(t) < line.terms!.length - 1 ? ' and' : ''}
                    </span>
                  ))}
                </Text>
              </Stack>
            ))}
          </Stack>
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header>
          <span className="wa-eyebrow">Yoast SEO breakdown</span>
        </Card.Header>
        <Card.Content>
          <Stack direction="column" gap="sm">
            {SAMPLE_SEO.map((line) => (
              <Stack key={line.label} direction="row" justify="space-between" align="center">
                <Text variant="body-sm">{line.label}</Text>
                <span style={dotStyle(line.state)} />
              </Stack>
            ))}
          </Stack>
        </Card.Content>
      </Card.Root>

      <Card.Root
        style={{
          background: 'var(--wpds-color-bg-surface-success-weak)',
          borderColor: 'var(--wpds-color-stroke-surface-success)',
        }}
      >
        <Card.Content>
          <Stack direction="column" gap="xs">
            <Text
              variant="body-sm"
              style={{
                fontWeight: 'var(--wpds-typography-font-weight-medium)',
                color: 'var(--wpds-color-fg-content-success)',
              }}
            >
              Reversible · always
            </Text>
            <Text variant="body-sm" style={{ color: 'var(--wpds-color-fg-content-success-weak)' }}>
              The previous copy is snapshotted before write. Revert any product
              in one click from the Done column.
            </Text>
          </Stack>
        </Card.Content>
      </Card.Root>
    </Stack>
  );
}
