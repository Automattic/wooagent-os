// Step ordering for the first-run flow. Each step has its own URL so the
// operator can refresh, share, or use the back button — see
// onboarding-design-brief.md §4.
//
// The list always has 4 entries so internal routing + the resumePath logic
// can index into it consistently regardless of how the UI was loaded. The
// stepper renders `getVisibleSteps()` instead, which hides the `daemon`
// step when the daemon templated `window.__WOOAGENT_TOKEN__` into
// index.html (release-binary install — embedded UI auto-connects). Vite
// dev users on :5173 don't get the auto-token, so they see the full
// 4-step flow including the URL+token paste in step 1.

export const ONBOARDING_STEPS = [
  { key: 'daemon', path: '/onboard/daemon', label: 'Connect daemon' },
  { key: 'store', path: '/onboard/store', label: 'Connect store' },
  { key: 'model', path: '/onboard/model', label: 'Model' },
  { key: 'done', path: '/onboard/done', label: 'Ready' },
] as const;

export type OnboardingStepKey = typeof ONBOARDING_STEPS[number]['key'];

export function stepIndex(key: OnboardingStepKey): number {
  return ONBOARDING_STEPS.findIndex((s) => s.key === key);
}

export function stepPath(key: OnboardingStepKey): string {
  const s = ONBOARDING_STEPS.find((x) => x.key === key);
  return s ? s.path : '/onboard';
}

/** True when the daemon templated an auto-auth token into index.html. */
export function isAutoAuth(): boolean {
  return typeof window !== 'undefined' && !!window.__WOOAGENT_TOKEN__;
}

/**
 * Steps the operator actually sees. Embedded-UI users (auto-auth) skip
 * the daemon step; Vite dev users see all four. Returns a fresh array
 * each call so callers can safely index/filter without mutating the
 * shared const.
 */
export function getVisibleSteps(): typeof ONBOARDING_STEPS[number][] {
  if (isAutoAuth()) {
    return ONBOARDING_STEPS.filter((s) => s.key !== 'daemon');
  }
  return [...ONBOARDING_STEPS];
}
