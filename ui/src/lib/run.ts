import type { Run } from '../api/client';

export function triggerLabel(trigger: Run['trigger']): string {
  switch (trigger) {
    case 'tick':
      return 'Scheduled';
    case 'manual':
      return 'Manual';
    case 'bootstrap':
      return 'Bootstrap';
    case 'retry':
      return 'Retry';
  }
}

export function formatLatency(ms: number): string {
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`;
}
