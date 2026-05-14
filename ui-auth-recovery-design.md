# UI Auth Recovery — Design

When the daemon returns 401 to a UI fetch (stale `window.__WOOAGENT_TOKEN__`, stale `localStorage`, daemon DB out of sync, etc.), silently recover by clearing local state and reloading. If a second 401 fires inside the same session, surface a modal that takes the operator to a fresh onboarding step. Closes finding F2 from the 2026-05-14 internal-testing smoke pass.

**Status:** design, ready for implementation planning. Small scope — one global handler + a modal.

## Goal

Today, a 401 from the daemon surfaces three different ways depending on where it fires:

- **In the onboarding resume probe** (`OnboardingShell.tsx:65-69`): `.catch(() => ({ stores: [] }))` swallows the 401 and routes the operator to Step 2 (Connect store) as if zero stores exist — where the next API call also 401s and renders the daemon's raw `"token not recognized"` string inside the store-pair card.
- **In any in-app fetch**: the call's local `.catch` either bubbles the error to a `<Notice>` or the screen renders an unhelpful "Failed to load" state.
- **In the embedded UI specifically**: there's no escape hatch — `Stepper.tsx:21-22` hides the Daemon step, so the operator can't re-paste a fresh token.

F3 (persistent ui-session) substantially mitigates the root cause — restarts no longer rotate the token. But scenarios that still trigger 401:

1. Operator deletes `~/.wooagent/ui-session.token` manually.
2. Operator points a tab against a different daemon (e.g., a teammate's, or a fresh install).
3. Daemon DB row gets wiped (`wooagent init` re-runs, manual DB delete).
4. Multiple tabs against the same daemon when the daemon-side token rotates.

For testers and real operators, all four are plausible. A clean recovery path is required.

## Scope

**In:**

- A global 401 interceptor inside `api/client.ts request()` — the single chokepoint every fetch passes through.
- A two-step recovery state machine: silent reload first; modal on persistent 401.
- A throttle state machine driven by `sessionStorage` flags (not `localStorage` — the recovery is per-session, not durable across browser restarts).
- An `AuthExpiredModal` component using WPDS `Modal`.
- A narrow fix to `OnboardingShell.tsx` so the resume probe re-throws 401s (rather than swallowing them) — defensive, since the global handler will fire reload before the probe even returns, but keeps intent explicit.

**Out:**

- Changing the daemon's `"token not recognized"` 401 body string. After this design lands, that string is intercepted upstream — it never bubbles into a card. The daemon side is fine as-is.
- Re-implementing `Stepper.tsx` to show the Daemon step in embedded mode. The modal's "Reconnect" action routes the operator to `/onboard/daemon` directly; the stepper visibility quirk no longer matters.
- Rate-limiting or backoff for repeated 401s beyond the two-step throttle. If the daemon returns 401 forever (corrupted DB), the operator hits the modal and is told to restart the daemon.

## State machine

```
401 fires
   │
   ├─ sessionStorage.authReloadAttempted == "1" ?
   │     │
   │     ├─ yes → set sessionStorage.authNoticeReason = "persistent_401"
   │     │       throw ApiError(401) so call sites that surface errors can render fallback states
   │     │
   │     └─ no  → set sessionStorage.authReloadAttempted = "1"
   │              localStorage.removeItem("wooagent.connection")
   │              window.location.reload()
   │              (JS aborts at reload; throw after for type-checker happiness)
   │
   └─ App.tsx on every render:
         if sessionStorage.authNoticeReason == "persistent_401":
            render <AuthExpiredModal />

Any successful (non-401) response:
   sessionStorage.removeItem("authReloadAttempted")
   sessionStorage.removeItem("authNoticeReason")

Modal "Reconnect" click:
   sessionStorage.removeItem("authReloadAttempted")
   sessionStorage.removeItem("authNoticeReason")
   localStorage.removeItem("wooagent.connection")
   nav("/onboard/daemon")
```

## Components

### `api/client.ts` — `handleAuthExpired()` + integration

A single helper called from inside `request()` when `res.status === 401`:

```ts
const AUTH_RELOAD_FLAG = 'wooagent.authReloadAttempted';
const AUTH_NOTICE_FLAG = 'wooagent.authNoticeReason';

function handleAuthExpired(): never {
  if (typeof window === 'undefined') {
    throw new ApiError(401, 'auth_expired', 'Session expired (no window context).');
  }
  if (sessionStorage.getItem(AUTH_RELOAD_FLAG) === '1') {
    sessionStorage.setItem(AUTH_NOTICE_FLAG, 'persistent_401');
    throw new ApiError(401, 'auth_expired', 'Session expired and reload did not recover.');
  }
  sessionStorage.setItem(AUTH_RELOAD_FLAG, '1');
  localStorage.removeItem(STORAGE_KEY);
  window.location.reload();
  throw new ApiError(401, 'auth_expired', 'Reloading to recover…');
}
```

Integration inside `request()`:

```ts
const res = await fetch(url, { ...init, headers });
if (res.status === 401) {
  handleAuthExpired();
}
if (res.ok) {
  if (typeof sessionStorage !== 'undefined') {
    sessionStorage.removeItem(AUTH_RELOAD_FLAG);
    sessionStorage.removeItem(AUTH_NOTICE_FLAG);
  }
}
// existing !res.ok handling continues below
```

Note: clearing the flags on every successful response is what closes the loop. The moment the daemon authenticates a fetch (post-reload, fresh token), the modal flag clears.

### `OnboardingShell.tsx` — narrow the probe's `.catch`

```ts
api.stores.list(connection).catch((e) => {
  if (e instanceof ApiError && e.status === 401) throw e;
  return { stores: [] as Store[] };
})
```

(Same shape on the `modelProviders.list` call.) Defensive: with the global handler the 401 path reloads before the `.catch` resolves, but the intent should still be explicit.

### `AuthExpiredModal.tsx` (new)

```tsx
import { Modal, Button } from '@wordpress/components';
import { Stack, Text } from '@wordpress/ui';
import { useNavigate } from 'react-router-dom';

const AUTH_RELOAD_FLAG = 'wooagent.authReloadAttempted';
const AUTH_NOTICE_FLAG = 'wooagent.authNoticeReason';
const STORAGE_KEY = 'wooagent.connection';

export default function AuthExpiredModal() {
  const nav = useNavigate();
  const onReconnect = () => {
    sessionStorage.removeItem(AUTH_RELOAD_FLAG);
    sessionStorage.removeItem(AUTH_NOTICE_FLAG);
    localStorage.removeItem(STORAGE_KEY);
    nav('/onboard/daemon');
  };
  return (
    <Modal
      title="Session expired"
      isDismissible={false}
      shouldCloseOnClickOutside={false}
      shouldCloseOnEsc={false}
      onRequestClose={() => {/* no-op — operator must take an action */}}
    >
      <Stack direction="column" gap="md">
        <Text variant="body-md">
          WooAgent couldn't authenticate against the daemon at {window.location.host}.
          The most common cause is a daemon restart that minted a fresh token
          while this tab was open. Reconnect to continue.
        </Text>
        <Text variant="body-sm" style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}>
          If reconnecting doesn't work, restart the daemon (<code>wooagent run</code>)
          and reload this tab.
        </Text>
        <Stack direction="row" gap="sm" justify="end">
          <Button __next40pxDefaultSize variant="primary" onClick={onReconnect}>
            Reconnect
          </Button>
        </Stack>
      </Stack>
    </Modal>
  );
}
```

`isDismissible={false}` + `shouldCloseOn*` false-fields ensure the modal can't be dismissed without the operator taking the action.

### `App.tsx` — gate the modal

```tsx
const [authExpired, setAuthExpired] = useState(
  () => sessionStorage.getItem('wooagent.authNoticeReason') === 'persistent_401',
);

// Re-read after every render in case a fetch elsewhere just set the flag.
useEffect(() => {
  const id = setInterval(() => {
    const flag = sessionStorage.getItem('wooagent.authNoticeReason') === 'persistent_401';
    setAuthExpired(prev => prev === flag ? prev : flag);
  }, 500);
  return () => clearInterval(id);
}, []);

…
{authExpired && <AuthExpiredModal />}
```

The 500ms poll is a deliberately simple cross-component signal. The alternative — a custom event bus — is more code for the same outcome. The modal only renders once, so the poll cost is negligible.

## Error handling

| Scenario | Behavior |
|---|---|
| First 401 of the session | Silent reload. Embedded UI gets fresh `__WOOAGENT_TOKEN__`. Operator sees a flicker, stays where they were. |
| Second 401 of the session | Modal renders. Operator clicks Reconnect → /onboard/daemon. |
| Daemon unreachable (network error, not 401) | Existing fetch-error path. This design changes nothing. |
| Daemon returns 401 forever (DB wipe) | Operator hits modal. Reconnect routes to onboarding. If onboarding's own fetches 401 again (because daemon is fundamentally broken), the modal re-appears. Operator restarts daemon per the modal copy. |
| Multiple tabs | Each tab has its own `sessionStorage`. Recovery is tab-scoped. Acceptable — testers don't operate WooAgent in multiple tabs typically. |

## Test plan

| Layer | Test shape |
|---|---|
| `handleAuthExpired()` | Unit. Stub `sessionStorage` + spy on `location.reload`. Three cases: first-401 sets flag + clears localStorage + calls reload; second-401 sets notice flag + throws without calling reload; non-401 success path clears both flags. |
| `request()` 401 path | Unit. Mock `fetch` to return 401. Verify `handleAuthExpired` is invoked exactly once and the function throws afterward. |
| `OnboardingShell` resume probe | Unit. Mock `api.stores.list` to reject with `ApiError(401)`. Verify the error propagates (is not swallowed). |
| `AuthExpiredModal` | Unit (React Testing Library). Render with flags set, click Reconnect, verify nav('/onboard/daemon') called, verify flags cleared, verify localStorage cleared. |
| App.tsx integration | Unit. Set the notice flag in sessionStorage, render App, verify modal appears. Clear the flag, verify modal disappears within 1s. |
| Manual smoke | Open UI, paste invalid token into `localStorage.wooagent.connection`, force any fetch (navigate to /agents), watch the silent reload recover. Then set the invalid token + the `authReloadAttempted` sessionStorage flag, force a fetch, watch the modal appear. |

## Risks

- **Reload loops on a permanently-broken daemon.** Mitigated by the throttle — only one reload per session. The modal is the failsafe.
- **Sessionstorage is per-tab.** Two tabs both stale at the same time would each do their own silent reload. Acceptable. The reload is cheap.
- **The 500ms poll in App.tsx is a smell.** It's the smallest possible cross-component signal that doesn't require new infrastructure (event bus, context provider). Acceptable for v0.1.0; revisit if more cross-component signals start to need it.
- **Vite dev (`localhost:5173`) doesn't have the injected `__WOOAGENT_TOKEN__`.** A silent reload there will land on `loadConnection() === null`, which routes through onboarding. That's the desired outcome — same recovery path as the embedded UI's modal action.

## Follow-ups

- Once this lands, the daemon's `"token not recognized"` 401 body string never reaches the UI. Consider replacing it with a structured `{ "code": "auth_expired", "message": "..." }` JSON body so future programmatic consumers of the API (CLI tools, integrations) get a clean error code. Out of scope here.
- The `Stepper.tsx` "Daemon step hidden in embedded mode" quirk becomes purely cosmetic after this design — the modal's Reconnect action goes through `/onboard/daemon` route directly. If we ever want to re-show the Daemon step in the embedded Stepper for visual clarity, that's a separate UX call.
