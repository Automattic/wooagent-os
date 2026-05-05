import { useNavigate } from 'react-router-dom';
import { Icon, check } from '@wordpress/icons';
import { ONBOARDING_STEPS, type OnboardingStepKey, stepIndex } from './state';

interface Props {
  currentKey: OnboardingStepKey;
  /** Highest-completed step. Steps up to and including this one are
   *  navigable; steps after are dimmed and inert. */
  highestCompleted: number;
}

// Custom stepper — no stable WPDS stepper exists. Built from semantic markup
// + tokens per CLAUDE.md. Numbers turn into checks once a step completes.
export default function Stepper({ currentKey, highestCompleted }: Props) {
  const nav = useNavigate();
  const currentIdx = stepIndex(currentKey);

  return (
    <nav aria-label="Onboarding progress" className="wa-stepper">
      {ONBOARDING_STEPS.map((step, i) => {
        const isDone = i < currentIdx || i <= highestCompleted - 1;
        const isCurrent = i === currentIdx;
        const isReachable = i <= highestCompleted || i <= currentIdx;
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
