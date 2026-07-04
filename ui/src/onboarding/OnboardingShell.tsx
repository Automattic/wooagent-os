import { useEffect, useState } from 'react';
import {
  Navigate,
  Route,
  Routes,
  useLocation,
  useNavigate,
} from 'react-router-dom';
import { Stack, Text } from '@wordpress/ui';
import {
  ApiError,
  api,
  type Connection,
  type ModelProvider,
  type Store,
} from '../api/client';
import Stepper from './Stepper';
import Step1Daemon from './Step1Daemon';
import Step2Store from './Step2Store';
import Step4Model from './Step4Model';
import Step5Done from './Step5Done';
import {
  ONBOARDING_STEPS,
  isAutoAuth,
  type OnboardingStepKey,
} from './state';
import wordmarkUrl from '../assets/wooagent-wordmark.png';

interface Props {
  connection: Connection | null;
  onConnected(c: Connection): void;
  onComplete(): void;
}

export async function probeOnboardingResume(
  connection: Connection,
): Promise<{ store: Store | null; provider: ModelProvider | null }> {
  const [storeRes, providerRes] = await Promise.all([
    api.stores.list(connection).catch((e) => {
      if (e instanceof ApiError && e.status === 401) throw e;
      return { stores: [] as Store[] };
    }),
    api.modelProviders
      .list(connection)
      .catch((e) => {
        if (e instanceof ApiError && e.status === 401) throw e;
        return { providers: [] as ModelProvider[] };
      }),
  ]);
  const paired =
    storeRes.stores.find((s) => s.status === 'paired') ?? null;
  return {
    store: paired ?? storeRes.stores[0] ?? null,
    provider: providerRes.providers[0] ?? null,
  };
}

// Maps the URL suffix back to the step key. Anything else falls through to
// "daemon" (the resume index handles redirects from /onboard root).
function keyFromPath(pathname: string): OnboardingStepKey {
  const tail = pathname.replace(/^.*\/onboard\/?/, '').split('/')[0];
  const found = ONBOARDING_STEPS.find((s) => s.key === tail);
  return (found?.key ?? 'daemon') as OnboardingStepKey;
}

export default function OnboardingShell({
  connection,
  onConnected,
  onComplete,
}: Props) {
  const nav = useNavigate();
  const location = useLocation();
  const [store, setStore] = useState<Store | null>(null);
  const [provider, setProvider] = useState<ModelProvider | null>(null);
  // Probe state that drives initial-step redirect + stepper completion line.
  const [probed, setProbed] = useState(false);

  // Resume probe: if a connection lives in localStorage, find out how far
  // the operator got before reload — paired store, discovered abilities,
  // configured provider — and seed the cross-step state from the daemon.
  useEffect(() => {
    if (!connection) {
      setProbed(true);
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        const resume = await probeOnboardingResume(connection);
        if (cancelled) return;
        setStore(resume.store);
        setProvider(resume.provider);
      } finally {
        if (!cancelled) setProbed(true);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [connection]);

  // Highest completed step — drives both the stepper's "back-navigable" line
  // and the resume-redirect from /onboard. Indices match ONBOARDING_STEPS:
  // 0 daemon, 1 store, 2 model, 3 done.
  const highestCompleted = (() => {
    if (!connection) return -1;
    if (!store || store.status !== 'paired') return 0;
    if (!provider) return 1;
    return 3;
  })();

  const resumePath = (() => {
    if (highestCompleted < 0) return '/onboard/daemon';
    if (highestCompleted < 1) return '/onboard/store';
    if (highestCompleted < 2) return '/onboard/model';
    return '/onboard/done';
  })();

  const currentKey = keyFromPath(location.pathname);
  const currentStep = ONBOARDING_STEPS.find((s) => s.key === currentKey);

  // While we're probing, show nothing rather than flicker through the wrong
  // step's UI — the redirect below routes the operator to the right place.
  if (!probed) return null;

  return (
    <div
      className={`wa-onboarding-shell${
        currentKey === 'done' ? ' wa-onboarding-shell--done' : ''
      }`}
    >
      <header className="wa-onboarding-header">
        <Stack direction="column" gap="sm" align="center">
          <img
            src={wordmarkUrl}
            alt="WooAgent"
            className="wa-onboarding-wordmark"
          />
          <Text
            variant="body-md"
            style={{ color: 'var(--wpds-color-fg-content-neutral)' }}
          >
            {currentStep?.subheading ?? ''}
          </Text>
        </Stack>
      </header>

      {currentKey !== 'done' && (
        <div className="wa-onboarding-stepper-wrap">
          <Stepper
            currentKey={currentKey}
            highestCompleted={highestCompleted}
          />
        </div>
      )}

      <main className="wa-onboarding-surface">
        <Routes>
          <Route index element={<Navigate to={resumePath} replace />} />
          <Route
            path="daemon"
            element={
              isAutoAuth() ? (
                // Embedded-UI build auto-connects via window.__WOOAGENT_TOKEN__.
                // The Step 1 form is dead in this mode — redirect to the
                // resume path (Step 2 by default) rather than showing a
                // confusing URL+token paste form.
                <Navigate to={resumePath} replace />
              ) : (
                <Step1Daemon
                  onConnected={(c) => {
                    onConnected(c);
                    nav('/onboard/store');
                  }}
                />
              )
            }
          />
          <Route
            path="store"
            element={
              connection ? (
                <Step2Store
                  connection={connection}
                  onPaired={(s) => {
                    setStore(s);
                    nav('/onboard/model');
                  }}
                  onBack={() => nav('/onboard/daemon')}
                />
              ) : (
                <Navigate to="/onboard/daemon" replace />
              )
            }
          />
          <Route
            path="model"
            element={
              connection ? (
                <Step4Model
                  connection={connection}
                  onSaved={(p) => {
                    setProvider(p);
                    nav('/onboard/done');
                  }}
                  onBack={() => nav('/onboard/store')}
                />
              ) : (
                <Navigate to="/onboard/daemon" replace />
              )
            }
          />
          <Route
            path="done"
            element={
              <Step5Done onOpenKanban={onComplete} />
            }
          />
          <Route path="*" element={<Navigate to={resumePath} replace />} />
        </Routes>
      </main>
    </div>
  );
}
