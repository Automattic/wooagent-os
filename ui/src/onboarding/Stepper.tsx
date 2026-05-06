import { useNavigate } from 'react-router-dom';
import { Icon, check } from '@wordpress/icons';
import {
  getVisibleSteps,
  stepIndex,
  type OnboardingStepKey,
} from './state';

interface Props {
  currentKey: OnboardingStepKey;
  /** Highest-completed step in the original 4-entry ONBOARDING_STEPS
   *  (NOT in the filtered visible list — App + OnboardingShell think in
   *  the canonical index space, the stepper translates). */
  highestCompleted: number;
}

// Custom stepper — no stable WPDS stepper exists. Built from semantic markup
// + tokens per CLAUDE.md. Numbers turn into checks once a step completes.
//
// Renders `getVisibleSteps()`, not the full ONBOARDING_STEPS, so the
// embedded-UI flow shows 3 steps (Connect store / Model / Ready) and Vite
// dev shows all 4. The `currentKey` and `highestCompleted` props are in
// the canonical index space (always 0..3); we re-index against the
// visible list for marker numbers + completion math.
export default function Stepper({ currentKey, highestCompleted }: Props) {
  const nav = useNavigate();
  const visible = getVisibleSteps();
  const currentCanonicalIdx = stepIndex(currentKey);

  return (
    <nav aria-label="Onboarding progress" className="wa-stepper">
      {visible.map((step, i) => {
        const canonicalIdx = stepIndex(step.key);
        const isDone =
          canonicalIdx < currentCanonicalIdx ||
          canonicalIdx <= highestCompleted - 1;
        const isCurrent = canonicalIdx === currentCanonicalIdx;
        const isReachable =
          canonicalIdx <= highestCompleted ||
          canonicalIdx <= currentCanonicalIdx;
        const stateClass = isCurrent
          ? 'wa-stepper__step--current'
          : isDone
            ? 'wa-stepper__step--done'
            : isReachable
              ? 'wa-stepper__step--upcoming'
              : 'wa-stepper__step--locked';

        return (
          <div key={step.key} className="wa-stepper__item">
            {i > 0 && (
              <span className="wa-stepper__connector" aria-hidden="true" />
            )}
            <button
              type="button"
              className={`wa-stepper__step ${stateClass}`}
              onClick={() => {
                if (isReachable && !isCurrent) nav(step.path);
              }}
              disabled={!isReachable}
              aria-current={isCurrent ? 'step' : undefined}
            >
              <span className="wa-stepper__marker" aria-hidden="true">
                {isDone ? <Icon icon={check} size={16} /> : i + 1}
              </span>
              <span className="wa-stepper__label">{step.label}</span>
            </button>
          </div>
        );
      })}
    </nav>
  );
}
