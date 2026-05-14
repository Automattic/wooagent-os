# UI Auth Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When the daemon returns 401, silently clear the session + reload once; if a second 401 fires, surface a modal that routes the operator to onboarding.

**Architecture:** One global 401 handler inside `api/client.ts request()` — the chokepoint every fetch goes through. Throttle via `sessionStorage` flags. `AuthExpiredModal` mounted at the App.tsx top level, gated by a sessionStorage reason flag.

**Tech Stack:** React + TypeScript, `@wordpress/components` (Modal, Button), `@wordpress/ui` (Stack, Text), `react-router-dom` (useNavigate).

**Spec:** [`ui-auth-recovery-design.md`](./ui-auth-recovery-design.md) — read before implementing.

**Note on tests:** The UI package has no test framework installed (no vitest, jest, or @testing-library in `ui/package.json`). The spec's "Test plan" assumed RTL + unit-test infrastructure; that doesn't exist yet. This plan ships the feature behind TypeScript checks + a documented manual smoke. Setting up Vitest + RTL is filed as a separate follow-up in Task 7. If the test framework lands first, the unit tests sketched in the spec can be added retroactively.

---

## File Structure

| Path | Responsibility |
|---|---|
| `ui/src/api/client.ts` (modify) | Add `handleAuthExpired()` helper + invoke it from `request()` on 401. Clear flags on successful response. |
| `ui/src/onboarding/OnboardingShell.tsx` (modify) | Narrow the resume probe's `.catch` to re-throw 401s instead of swallowing them. |
| `ui/src/components/AuthExpiredModal.tsx` (create) | WPDS Modal that says "Session expired" with a Reconnect action that routes to `/onboard/daemon`. |
| `ui/src/App.tsx` (modify) | Read sessionStorage flag, render `<AuthExpiredModal />` when set, poll every 500ms to detect cross-component state changes. |

---

## Task 1: `handleAuthExpired()` helper

**Files:**
- Modify: `ui/src/api/client.ts`

- [ ] **Step 1: Add the storage-key constants**

Add to `ui/src/api/client.ts`, near the top alongside the existing `STORAGE_KEY` constant:

```ts
const AUTH_RELOAD_FLAG = 'wooagent.authReloadAttempted';
const AUTH_NOTICE_FLAG = 'wooagent.authNoticeReason';
```

- [ ] **Step 2: Add the `handleAuthExpired` helper**

Add to `ui/src/api/client.ts`, immediately above the `request()` function:

```ts
// handleAuthExpired is the global 401-recovery state machine. First 401 in
// a session clears localStorage and reloads (the daemon will inject a fresh
// window.__WOOAGENT_TOKEN__ on serve). Second 401 sets a flag the App.tsx
// modal-gate watches and throws an ApiError so callers stop optimistically
// rendering data. See ui-auth-recovery-design.md.
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
  // window.location.reload() doesn't synchronously halt the JS task; the
  // throw is reachable before the navigation begins. Keep it explicit so
  // the return type stays `never` and callers' control flow is correct.
  throw new ApiError(401, 'auth_expired', 'Reloading to recover…');
}
```

- [ ] **Step 3: Wire it into `request()`'s 401 path**

Find the existing block inside `request()`:

```ts
const res = await fetch(url, { ...init, headers });
if (!res.ok) {
  let code = 'http_error';
  // ... existing error handling
}
```

Replace it with:

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
if (!res.ok) {
  let code = 'http_error';
  // ... existing error handling continues
}
```

- [ ] **Step 4: TypeScript check**

Run: `cd ui && npx tsc -b`
Expected: clean compile.

- [ ] **Step 5: Don't commit yet — Tasks 2-5 ship together.**

---

## Task 2: Narrow OnboardingShell resume probe's `.catch`

After Task 1, a 401 in the probe will trigger `handleAuthExpired` before the `.catch` ever runs — reload supersedes. But the existing `.catch(() => ({ stores: [] }))` is broad and would swallow other unexpected errors too. Tighten it to make intent explicit.

**Files:**
- Modify: `ui/src/onboarding/OnboardingShell.tsx:65-69`

- [ ] **Step 1: Import `ApiError`**

In the existing import block at the top of `OnboardingShell.tsx`, add `ApiError`:

```ts
import {
  ApiError,
  api,
  type Connection,
  type ModelProvider,
  type Store,
} from '../api/client';
```

- [ ] **Step 2: Narrow both `.catch` handlers**

Replace lines 65-69 in `OnboardingShell.tsx`:

```ts
const [storeRes, providerRes] = await Promise.all([
  api.stores.list(connection).catch((e) => {
    if (e instanceof ApiError && e.status === 401) throw e;
    return { stores: [] as Store[] };
  }),
  api.modelProviders.list(connection).catch((e) => {
    if (e instanceof ApiError && e.status === 401) throw e;
    return { providers: [] as ModelProvider[] };
  }),
]);
```

- [ ] **Step 3: TypeScript check**

Run: `cd ui && npx tsc -b`
Expected: clean compile.

- [ ] **Step 4: Don't commit yet.**

---

## Task 3: `AuthExpiredModal` component

**Files:**
- Create: `ui/src/components/AuthExpiredModal.tsx`

- [ ] **Step 1: Create the file**

Create `ui/src/components/AuthExpiredModal.tsx`:

```tsx
import { Button, Modal } from '@wordpress/components';
import { Stack, Text } from '@wordpress/ui';
import { useNavigate } from 'react-router-dom';

// Storage-key constants must stay in sync with api/client.ts. Kept inline
// here (rather than imported) because client.ts intentionally doesn't
// export them — they're a private state-machine detail. Re-exporting
// would invite stray callers to mutate the flags directly and bypass the
// state machine.
const AUTH_RELOAD_FLAG = 'wooagent.authReloadAttempted';
const AUTH_NOTICE_FLAG = 'wooagent.authNoticeReason';
const STORAGE_KEY = 'wooagent.connection';

// AuthExpiredModal renders when api/client.ts has fired its second 401 in
// a session (first 401 triggers a silent reload). Operator must take the
// Reconnect action — the modal is non-dismissible by design.
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
      onRequestClose={() => {
        /* no-op — operator must take an action via the Reconnect button */
      }}
    >
      <Stack direction="column" gap="md">
        <Text variant="body-md">
          WooAgent couldn't authenticate against the daemon at{' '}
          <code>{typeof window !== 'undefined' ? window.location.host : ''}</code>.
          The most common cause is a daemon restart that minted a fresh token
          while this tab was open. Reconnect to continue.
        </Text>
        <Text
          variant="body-sm"
          style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
        >
          If reconnecting doesn't work, restart the daemon
          (<code>wooagent run</code>) and reload this tab.
        </Text>
        <Stack direction="row" gap="sm" justify="end">
          <Button
            __next40pxDefaultSize
            variant="primary"
            onClick={onReconnect}
          >
            Reconnect
          </Button>
        </Stack>
      </Stack>
    </Modal>
  );
}
```

- [ ] **Step 2: Verify Modal props exist on the installed `@wordpress/components` version**

Run: `grep -E "isDismissible|shouldCloseOnClickOutside|shouldCloseOnEsc" ui/node_modules/@wordpress/components/build-types/modal/types.d.ts | head -5`

Expected: all three appear (the props are part of the standard `@wordpress/components` Modal API).

If any prop is missing, drop the missing one(s) — the modal will still work (the worst case is `shouldCloseOnEsc=false` not being honored, which is acceptable for v0.1.0 since operators rarely hit Escape on a modal asking them to reconnect).

- [ ] **Step 3: TypeScript check**

Run: `cd ui && npx tsc -b`
Expected: clean compile.

- [ ] **Step 4: Don't commit yet.**

---

## Task 4: Gate the modal from App.tsx

**Files:**
- Modify: `ui/src/App.tsx`

- [ ] **Step 1: Import the modal + `useState`/`useEffect`** (useEffect/useState are already imported; verify the existing import block)

Run: `grep -E "useEffect|useState" ui/src/App.tsx | head -2`

Expected: both already imported. If not, add to the existing `from 'react'` line.

Add the modal import next to the other component imports:

```ts
import AuthExpiredModal from './components/AuthExpiredModal';
```

- [ ] **Step 2: Add the gate state + polling effect**

Inside the App component, after the existing `useState` declarations:

```ts
// Watch sessionStorage for the auth-expired notice flag. api/client.ts
// sets it after a second 401 in a session; the modal renders until the
// operator clicks Reconnect. 500ms poll is the simplest cross-component
// signal that doesn't require new infrastructure (custom event bus,
// context provider). The modal only renders once per recovery cycle so
// the poll cost is negligible.
const [authExpired, setAuthExpired] = useState(
  () =>
    typeof sessionStorage !== 'undefined' &&
    sessionStorage.getItem('wooagent.authNoticeReason') === 'persistent_401',
);
useEffect(() => {
  if (typeof sessionStorage === 'undefined') return;
  const id = setInterval(() => {
    const flag =
      sessionStorage.getItem('wooagent.authNoticeReason') === 'persistent_401';
    setAuthExpired((prev) => (prev === flag ? prev : flag));
  }, 500);
  return () => clearInterval(id);
}, []);
```

- [ ] **Step 3: Render the modal at the App's top level**

Find the App component's return statement. Add the modal at the top of the returned JSX, before the existing layout:

```tsx
return (
  <>
    {authExpired && <AuthExpiredModal />}
    {/* existing layout (Routes, etc.) follows */}
    ...
  </>
);
```

If the existing return is already wrapped in a fragment or single root, keep that structure — just add `{authExpired && <AuthExpiredModal />}` as a sibling above the existing root content.

- [ ] **Step 4: TypeScript check**

Run: `cd ui && npx tsc -b`
Expected: clean compile.

- [ ] **Step 5: Don't commit yet.**

---

## Task 5: Build + commit

**Files (already staged from Tasks 1-4):**
- `ui/src/api/client.ts`
- `ui/src/onboarding/OnboardingShell.tsx`
- `ui/src/components/AuthExpiredModal.tsx` (new)
- `ui/src/App.tsx`

- [ ] **Step 1: Full UI build**

Run: `cd ui && npm run build`
Expected: build succeeds, dist regenerated with new bundle hash.

- [ ] **Step 2: Copy dist into daemon embed**

Run: `scripts/build-ui-into-daemon.sh`
Expected: "Copied 2.4M of UI assets..."

- [ ] **Step 3: Rebuild daemon binary**

Run: `cd daemon && go build -o /tmp/wooagent-smoke ./cmd/wooagent`
Expected: clean compile.

- [ ] **Step 4: Stage + commit**

```bash
git add \
  ui/src/api/client.ts \
  ui/src/onboarding/OnboardingShell.tsx \
  ui/src/components/AuthExpiredModal.tsx \
  ui/src/App.tsx \
  daemon/internal/uiassets/dist/index.html

git commit -m "$(cat <<'EOF'
feat(ui): auth-expired recovery (F2)

Global 401 handler in api/client.ts request(): first 401 in a
session clears localStorage and reloads (daemon serves fresh
window.__WOOAGENT_TOKEN__ on the new HTML); second 401 sets a
sessionStorage flag and throws ApiError(401). App.tsx watches
the flag and renders AuthExpiredModal, which routes the
operator to /onboard/daemon via a Reconnect action.

OnboardingShell's resume probe narrows its .catch to re-throw
401s instead of swallowing them as "no stores yet" — defensive,
since the global handler reloads before the catch resolves,
but keeps intent explicit.

The modal is non-dismissible (no close button, no
click-outside, no Escape) — operator must take an action.
Copy includes the daemon hostname so they know where to look
if the daemon is genuinely unreachable.

Closes F2 from the 2026-05-14 internal-testing smoke pass.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Manual smoke

The plan ships behind TypeScript checks only. Verify the behavior end-to-end against the live daemon.

- [ ] **Step 1: Start the daemon**

If not running: `/tmp/wooagent-smoke run` in a terminal.

- [ ] **Step 2: Open the UI**

Navigate to `http://localhost:7777`. Authenticate as normal (the embedded UI auto-auth from `window.__WOOAGENT_TOKEN__` should land you on the Kanban).

- [ ] **Step 3: Simulate a stale token (silent recovery path)**

Open DevTools console. Paste:

```js
const c = JSON.parse(localStorage.getItem('wooagent.connection') || '{}');
c.token = 'wo_pat_invalid_synthetic_token';
localStorage.setItem('wooagent.connection', JSON.stringify(c));
```

Then trigger a fetch by navigating to `/agents`. Expected: the page reloads silently (the silent-recovery path fires). After reload, the embedded UI gets a fresh `window.__WOOAGENT_TOKEN__` injected by the daemon, and the Kanban / Agents page loads normally.

Verify in DevTools that `sessionStorage.wooagent.authReloadAttempted` was set to `"1"` briefly, then cleared after the first successful response.

- [ ] **Step 4: Simulate persistent 401 (modal path)**

In DevTools console:

```js
// Force the reload-attempted flag so the next 401 surfaces the modal.
sessionStorage.setItem('wooagent.authReloadAttempted', '1');
// Corrupt the token again.
const c = JSON.parse(localStorage.getItem('wooagent.connection') || '{}');
c.token = 'wo_pat_invalid_synthetic_token';
localStorage.setItem('wooagent.connection', JSON.stringify(c));
```

Trigger a fetch by navigating to `/runs`. Expected: within ~500ms (next App.tsx poll), the AuthExpiredModal appears. The modal title says "Session expired"; the body names `localhost:7777`; the Reconnect button is enabled.

- [ ] **Step 5: Click Reconnect**

Expected: navigates to `/onboard/daemon`. Verify in DevTools that both `wooagent.authReloadAttempted` and `wooagent.authNoticeReason` are cleared from sessionStorage, and `wooagent.connection` is cleared from localStorage.

- [ ] **Step 6: Walk onboarding fresh**

The Daemon step renders since `loadConnection()` returns null. Hit Continue (the embedded UI's auto-auth should re-pair via the injected window token). End-to-end recovery loop closed.

- [ ] **Step 7: No commit — this is a verification task.**

---

## Task 7: Follow-ups (file in Linear)

These are out-of-scope improvements identified during the design but deferred for v0.1.0 ship velocity.

- [ ] **Step 1: File Linear issue — UI test infrastructure**

Per memory `feedback_followups_in_linear`: team DSGWOO (Woo Design), project WooAgent OS, assignee Elizabeth.

Title: "Stand up UI test infrastructure (Vitest + React Testing Library)"

Body:
```
The UI codebase has no test framework installed (no vitest, jest, or
@testing-library/* in ui/package.json). Tests sketched in
ui-auth-recovery-design.md (and any future design doc) can't be
implemented without this.

Scope: install vitest + @testing-library/react + @testing-library/jest-dom,
configure vite.config.ts for the test runner, add a test/ directory
convention, set up a CI step (if applicable), and back-fill the unit
tests in ui-auth-recovery-design.md "Test plan" section.

Reasonable size: half a day. Blocks: nothing critical for v0.1.0 but
catches regressions on every UI change after.
```

- [ ] **Step 2: File Linear issue — Structured 401 error body**

Title: "Daemon: replace `token not recognized` 401 body with structured error code"

Body:
```
After ui-auth-recovery (F2) lands, the daemon's "token not recognized"
401 body is intercepted by the UI's global handler and never bubbles to
a card. The string is fine as-is for the UI.

For future programmatic consumers (CLI tools, integrations), the 401
body should be a structured error: `{ "error": { "code": "auth_expired",
"message": "..." } }`. This is one ~5-line change in
daemon/internal/httpapi/middleware.go and reuses the existing writeError
helper.

Not blocking for v0.1.0.
```

- [ ] **Step 3: No commit — these are external follow-ups, not code changes.**

---

## Self-Review Notes

**Spec coverage:** Every section of `ui-auth-recovery-design.md` maps to a task above.
- Global 401 interceptor → Task 1 (helper) + Task 1 (request integration)
- State machine (first/second 401, success-clears-flags) → Task 1
- OnboardingShell probe narrow → Task 2
- AuthExpiredModal component → Task 3
- App.tsx gate → Task 4
- Manual smoke test plan → Task 6
- Follow-ups (test infra, structured error body) → Task 7

**Placeholder scan:** Every step contains either complete code, an exact command with expected output, or concrete external action (file Linear issue with the body shown). No "TBD" / "TODO" / "Similar to Task N" without the actual content.

**Type consistency:** Storage-key constants (`AUTH_RELOAD_FLAG`, `AUTH_NOTICE_FLAG`, `STORAGE_KEY`) appear in client.ts (Task 1) and AuthExpiredModal.tsx (Task 3). Strings match exactly between definitions and usages. The duplicate strings (rather than a shared import) are intentional per the comment in Task 3 — client.ts keeps the flags as private state-machine details.

**Scope:** Single focused feature. No decomposition needed. Plan ships in one commit.
