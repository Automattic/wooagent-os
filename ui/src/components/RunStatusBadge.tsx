import { Badge } from '@wordpress/ui';
import { Spinner } from '@wordpress/components';
import type { RunStatus } from '../api/client';

interface Props {
  status: RunStatus;
}

// Intent mapping per CLAUDE.md:
// succeeded → stable, skipped → informational, failed → medium,
// failed_permanent → high, queued / running → low.
const INTENT: Record<
  RunStatus,
  'stable' | 'informational' | 'medium' | 'high' | 'low'
> = {
  queued: 'low',
  running: 'low',
  succeeded: 'stable',
  skipped: 'informational',
  failed: 'medium',
  failed_permanent: 'high',
};

const LABEL: Record<RunStatus, string> = {
  queued: 'Queued',
  running: 'Running',
  succeeded: 'Succeeded',
  skipped: 'Skipped',
  failed: 'Failed',
  failed_permanent: 'Failed (permanent)',
};

const IN_FLIGHT = new Set<RunStatus>(['queued', 'running']);

export function RunStatusBadge({ status }: Props) {
  // For in-flight states, pair the badge with a spinner so operators can
  // tell at a glance that the run is actively working (Pricing routinely
  // takes 30-60s via web_search). Without it, a long-running "Running"
  // badge reads as frozen.
  if (IN_FLIGHT.has(status)) {
    return (
      <span
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: 'var(--wpds-dimension-gap-xs, 4px)',
        }}
      >
        <Badge intent={INTENT[status]}>{LABEL[status]}</Badge>
        <Spinner />
      </span>
    );
  }
  return <Badge intent={INTENT[status]}>{LABEL[status]}</Badge>;
}
