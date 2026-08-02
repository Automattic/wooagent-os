import type { KpiTone } from '../components/Kpi';

/**
 * The two scored dimensions the Marketing persona emits per variant.
 */
export type ScoreKind = 'seo' | 'voice';

/**
 * Score bands, keyed by kind. SEO and Voice deliberately do NOT share
 * thresholds:
 *
 *   - SEO cautions at 70, matching the Yoast/product-copy-rubric convention
 *     of "5 of 6 checks clear".
 *   - Voice cautions at 65. Relaxed from the original 90/75 — a 90% match
 *     against a small corpus is unrealistic (see DSGWOO-1329 on corpus
 *     sparsity).
 *
 * The high band also renders differently: SEO uses `success` (green), Voice
 * uses `brand` (the Figma frame's blue for a high-match voice score).
 *
 * Thresholds are re-tuned once there's a week of real data — DSGWOO-1328.
 */
const BANDS: Record<ScoreKind, { good: number; caution: number; goodTone: KpiTone }> = {
  seo: { good: 80, caution: 70, goodTone: 'success' },
  voice: { good: 80, caution: 65, goodTone: 'brand' },
};

/**
 * Score → `Kpi` tone, for the KPI row on the Marketing detail page.
 */
export function scoreTone(kind: ScoreKind, score: number): KpiTone {
  const band = BANDS[kind];
  if (score >= band.good) return band.goodTone;
  if (score >= band.caution) return 'caution';
  return 'warning';
}

/**
 * Score → `wa-score-label__value--*` CSS modifier, for the score labels on
 * the right side of each variant card. Unlike `scoreTone`, the high band is
 * the same `--good` class for both kinds.
 */
export function scoreValueClass(kind: ScoreKind, score: number): string {
  const band = BANDS[kind];
  if (score >= band.good) return 'wa-score-label__value--good';
  if (score >= band.caution) return 'wa-score-label__value--caution';
  return 'wa-score-label__value--warning';
}
