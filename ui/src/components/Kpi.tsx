import type { ReactNode, CSSProperties } from 'react';
import { Card, Stack, Text } from '@wordpress/ui';

export type KpiTone = 'neutral' | 'brand' | 'success' | 'warning' | 'caution';

interface Props {
  label: string;
  value: ReactNode;
  hint?: string;
  /** 0–100 score; renders a thin progress bar above the hint when set. */
  score?: number;
  /** Color cue for value text + bar fill. Defaults to neutral. */
  tone?: KpiTone;
}

// Token mapping for non-neutral tones. Verified against the WPDS token
// reference; success/warning/caution bars use the normal-strength
// bg-surface-* tokens because no -strong variant exists for those tones.
const TONE_FG: Record<KpiTone, string> = {
  neutral: 'var(--wpds-color-fg-content-neutral)',
  brand: 'var(--wpds-color-fg-interactive-brand)',
  success: 'var(--wpds-color-fg-content-success)',
  warning: 'var(--wpds-color-fg-content-warning)',
  caution: 'var(--wpds-color-fg-content-caution)',
};

const TONE_BAR: Record<KpiTone, string | null> = {
  neutral: null,
  brand: 'var(--wpds-color-bg-interactive-brand-strong)',
  success: 'var(--wpds-color-bg-surface-success)',
  warning: 'var(--wpds-color-bg-surface-warning)',
  caution: 'var(--wpds-color-bg-surface-caution)',
};

// One KPI tile from the IssueDetail header row. Stays a thin wrapper around
// Card so the four tiles read as a unit.
export default function Kpi({ label, value, hint, score, tone = 'neutral' }: Props) {
  const barFill = TONE_BAR[tone];
  const barStyle: CSSProperties | undefined = barFill
    ? ({ ['--bar-fill' as string]: barFill } as CSSProperties)
    : undefined;
  return (
    <Card.Root style={{ flex: 1, minWidth: 0 }}>
      <Card.Content>
        <Stack direction="column" gap="sm">
          <span className="wa-eyebrow">{label}</span>
          <Text
            variant="heading-lg"
            style={{
              color: TONE_FG[tone],
              fontWeight: 'var(--wpds-typography-font-weight-medium)',
            }}
          >
            {value}
          </Text>
          {typeof score === 'number' && (
            <div className="wa-score-bar" style={barStyle} aria-hidden="true">
              <div style={{ width: `${Math.max(0, Math.min(100, score))}%` }} />
            </div>
          )}
          {hint && (
            <Text
              variant="body-sm"
              style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
            >
              {hint}
            </Text>
          )}
        </Stack>
      </Card.Content>
    </Card.Root>
  );
}
