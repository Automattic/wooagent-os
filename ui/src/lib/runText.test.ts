import { describe, expect, it } from 'vitest';
import {
  LONG_REASON_THRESHOLD,
  isLongReason,
  summarizeReason,
} from './runText';

// The reason that motivated the threshold gate. 127 characters — under
// LONG_REASON_THRESHOLD — yet the clause-first heuristic used to reduce it to
// the bare operation name, hiding the actual store-side failure.
const MCP_ERROR_REASON =
  'mcp call wooagent-products/list: tool "mcp-adapter-execute-ability" returned error: An error occurred while executing the tool.';

// A real Pricing trace: long, colon-delimited, leading clause IS the summary.
const PRICING_TRACE =
  'tried 3 products, none drafted: product 3908: no_proposal: Ran 5 searches across jcrew.com, uniqlo.com and gap.com; none returned a visible regular price for a comparable v-neck, so the benchmark had too few sources to anchor on.';

describe('isLongReason', () => {
  it('is false for empty and nullish input', () => {
    expect(isLongReason('')).toBe(false);
    expect(isLongReason(null)).toBe(false);
    expect(isLongReason(undefined)).toBe(false);
  });

  it('boundaries on LONG_REASON_THRESHOLD', () => {
    expect(isLongReason('x'.repeat(LONG_REASON_THRESHOLD))).toBe(false);
    expect(isLongReason('x'.repeat(LONG_REASON_THRESHOLD + 1))).toBe(true);
  });
});

describe('summarizeReason — short reasons are shown whole', () => {
  // The regression this file exists for.
  it('keeps the cause of an error-shaped reason', () => {
    const got = summarizeReason(MCP_ERROR_REASON);
    expect(got).toBe(MCP_ERROR_REASON);
    expect(got).toContain('An error occurred while executing the tool');
    // The old behavior: everything after the first ": " was discarded.
    expect(got).not.toBe('mcp call wooagent-products/list.');
  });

  it('does not trim a short reason at its first colon', () => {
    expect(summarizeReason('tried 3 products, none drafted: product 3908')).toBe(
      'tried 3 products, none drafted: product 3908.',
    );
  });

  it('leaves a short reason with no delimiters alone, adding a period', () => {
    expect(
      summarizeReason('persona already has 2 or more open proposals; not enqueuing'),
    ).toBe('persona already has 2 or more open proposals; not enqueuing.');
  });

  it('does not append a period after non-word terminal punctuation', () => {
    expect(
      summarizeReason('no implementation registered for persona "reporting"'),
    ).toBe('no implementation registered for persona "reporting"');
  });
});

describe('summarizeReason — long reasons still condense', () => {
  // The behavior the helper was built for must survive the gate.
  it('keeps only the leading clause of a long trace', () => {
    expect(isLongReason(PRICING_TRACE)).toBe(true);
    expect(summarizeReason(PRICING_TRACE)).toBe(
      'tried 3 products, none drafted.',
    );
  });

  it('condenses at the first sentence terminator when it comes first', () => {
    const long =
      'Nothing to price right now. ' + 'Detail follows: '.repeat(12);
    expect(isLongReason(long)).toBe(true);
    expect(summarizeReason(long)).toBe('Nothing to price right now.');
  });

  it('keeps the whole string when the leading clause is a lone label', () => {
    // Guard from the original implementation: a one-word lead like "Error:"
    // must not collapse to just the label.
    const long = 'Error: ' + 'something went wrong in a verbose way. '.repeat(6);
    expect(isLongReason(long)).toBe(true);
    expect(summarizeReason(long)).toBe(long.trim());
  });
});

describe('summarizeReason — edges', () => {
  it('passes empty and whitespace-only input through', () => {
    expect(summarizeReason('')).toBe('');
    expect(summarizeReason('   ')).toBe('');
  });

  it('trims surrounding whitespace', () => {
    expect(summarizeReason('  a short reason  ')).toBe('a short reason.');
  });
});
