import type { ReactNode } from 'react';
import { Card, Stack, Text } from '@wordpress/ui';

interface Props {
  label: string;
  value: ReactNode;
  hint?: string;
  /** 0–100 score; renders a thin progress bar above the hint when set. */
  score?: number;
  /** Color cue for the value text — defaults to neutral. */
  intent?: 'neutral' | 'success';
}

// One KPI tile from the IssueDetail header row. Stays a thin wrapper around
// Card so the four tiles read as a unit.
export default function Kpi({ label, value, hint, score, intent = 'neutral' }: Props) {
  const valueColor =
    intent === 'success'
      ? 'var(--wpds-color-fg-content-success)'
      : 'var(--wpds-color-fg-content-neutral)';
  return (
    <Card.Root style={{ flex: 1, minWidth: 0 }}>
      <Card.Content>
        <Stack direction="column" gap="sm">
          <span className="wa-eyebrow">{label}</span>
          <Text
            variant="heading-lg"
            style={{ color: valueColor, fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
          >
            {value}
          </Text>
          {typeof score === 'number' && (
            <div className="wa-score-bar" aria-hidden="true">
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
