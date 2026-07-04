import { describe, expect, it } from 'vitest';
import { formatLatency, triggerLabel } from './run';

describe('triggerLabel', () => {
  it('maps scheduler trigger slugs to operator-facing labels', () => {
    expect(triggerLabel('tick')).toBe('Scheduled');
    expect(triggerLabel('manual')).toBe('Manual');
    expect(triggerLabel('bootstrap')).toBe('Bootstrap');
    expect(triggerLabel('retry')).toBe('Retry');
  });
});

describe('formatLatency', () => {
  it('uses milliseconds below one second', () => {
    expect(formatLatency(875)).toBe('875ms');
  });

  it('uses one decimal place for seconds', () => {
    expect(formatLatency(1250)).toBe('1.3s');
  });
});
