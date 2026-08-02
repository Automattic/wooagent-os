import { describe, expect, it } from 'vitest';
import { scoreTone, scoreValueClass, type ScoreKind } from './score';
import type { KpiTone } from '../components/Kpi';

// Boundary table. These values are the regression guard for DSGWOO-1306:
// the two kinds share a `good` threshold (80) but NOT a `caution` one
// (SEO 70, Voice 65), and their high band renders as different tones
// (`success` vs `brand`). Collapsing the helpers must not flatten either
// difference.
const CASES: Array<{
  kind: ScoreKind;
  score: number;
  tone: KpiTone;
  cls: string;
}> = [
  // --- SEO: good ≥80, caution ≥70 ---
  { kind: 'seo', score: 100, tone: 'success', cls: 'wa-score-label__value--good' },
  { kind: 'seo', score: 80, tone: 'success', cls: 'wa-score-label__value--good' },
  { kind: 'seo', score: 79, tone: 'caution', cls: 'wa-score-label__value--caution' },
  { kind: 'seo', score: 70, tone: 'caution', cls: 'wa-score-label__value--caution' },
  { kind: 'seo', score: 69, tone: 'warning', cls: 'wa-score-label__value--warning' },
  { kind: 'seo', score: 0, tone: 'warning', cls: 'wa-score-label__value--warning' },

  // --- Voice: good ≥80 (brand tone), caution ≥65 ---
  { kind: 'voice', score: 100, tone: 'brand', cls: 'wa-score-label__value--good' },
  { kind: 'voice', score: 80, tone: 'brand', cls: 'wa-score-label__value--good' },
  { kind: 'voice', score: 79, tone: 'caution', cls: 'wa-score-label__value--caution' },
  { kind: 'voice', score: 65, tone: 'caution', cls: 'wa-score-label__value--caution' },
  { kind: 'voice', score: 64, tone: 'warning', cls: 'wa-score-label__value--warning' },
  { kind: 'voice', score: 0, tone: 'warning', cls: 'wa-score-label__value--warning' },
];

describe('scoreTone', () => {
  for (const c of CASES) {
    it(`${c.kind} ${c.score} → ${c.tone}`, () => {
      expect(scoreTone(c.kind, c.score)).toBe(c.tone);
    });
  }
});

describe('scoreValueClass', () => {
  for (const c of CASES) {
    it(`${c.kind} ${c.score} → ${c.cls}`, () => {
      expect(scoreValueClass(c.kind, c.score)).toBe(c.cls);
    });
  }
});

describe('the two kinds diverge', () => {
  // 65..69 is the band where the differing caution thresholds actually
  // show up: SEO has already dropped to warning, Voice has not.
  it.each([65, 66, 67, 68, 69])('at %i, SEO warns but Voice cautions', (score) => {
    expect(scoreTone('seo', score)).toBe('warning');
    expect(scoreTone('voice', score)).toBe('caution');
    expect(scoreValueClass('seo', score)).toBe('wa-score-label__value--warning');
    expect(scoreValueClass('voice', score)).toBe('wa-score-label__value--caution');
  });

  it('renders the high band with a different tone per kind', () => {
    expect(scoreTone('seo', 85)).toBe('success');
    expect(scoreTone('voice', 85)).toBe('brand');
    // ...but the same CSS class.
    expect(scoreValueClass('seo', 85)).toBe(scoreValueClass('voice', 85));
  });
});
