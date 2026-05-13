import { Badge } from '@wordpress/ui';
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

export function RunStatusBadge({ status }: Props) {
  return <Badge intent={INTENT[status]}>{LABEL[status]}</Badge>;
}
