// Step ordering for the first-run flow. Each step has its own URL so the
// operator can refresh, share, or use the back button — see
// onboarding-design-brief.md §4.

export const ONBOARDING_STEPS = [
  { key: 'daemon', path: '/onboard/daemon', label: 'Connect' },
  { key: 'store', path: '/onboard/store', label: 'Add store' },
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
