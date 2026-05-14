import { Button, Modal } from '@wordpress/components';
import { Stack, Text } from '@wordpress/ui';

// Storage-key constants must stay in sync with api/client.ts. Kept inline
// here (rather than imported) because client.ts intentionally doesn't
// export them — they're a private state-machine detail. Re-exporting
// would invite stray callers to mutate the flags directly and bypass the
// state machine.
const AUTH_RELOAD_FLAG = 'wooagent.authReloadAttempted';
const AUTH_NOTICE_FLAG = 'wooagent.authNoticeReason';
const STORAGE_KEY = 'wooagent.connection';

// AuthExpiredModal renders when api/client.ts has fired its second 401 in
// a session (first 401 triggers a silent reload). The actual recovery in
// the embedded UI is a daemon restart — the daemon mints a fresh
// ui-session token at startup (per EnsureUISession, see F3) and templates
// it into every served HTML page via window.__WOOAGENT_TOKEN__. We don't
// expose a "Create new session token" button here because:
//   (a) loadConnection() prefers the daemon-injected token over
//       localStorage, so clearing localStorage doesn't recover.
//   (b) The realistic 401 frequency (operator deleted state, daemon
//       reinitialized, etc.) doesn't justify a new unauthed mutation
//       endpoint on the daemon side.
//
// The modal is non-dismissible — operator must reload.
export default function AuthExpiredModal() {
  const onReload = () => {
    sessionStorage.removeItem(AUTH_RELOAD_FLAG);
    sessionStorage.removeItem(AUTH_NOTICE_FLAG);
    localStorage.removeItem(STORAGE_KEY);
    window.location.reload();
  };

  return (
    <Modal
      title="Session expired"
      isDismissible={false}
      shouldCloseOnClickOutside={false}
      shouldCloseOnEsc={false}
      onRequestClose={() => {
        /* no-op — operator must take an action via the Reload button */
      }}
    >
      <Stack direction="column" gap="md">
        <Text variant="body-md">
          WooAgent's auth token for the daemon at{' '}
          <code>{typeof window !== 'undefined' ? window.location.host : ''}</code>{' '}
          no longer works. This usually means the daemon was reinitialized or
          its state directory was reset.
        </Text>
        <Text variant="body-md">To recover:</Text>
        <Stack direction="column" gap="xs" style={{ marginLeft: 'var(--wpds-dimension-padding-md)' }}>
          <Text variant="body-sm">
            1. Stop the daemon (<code>Ctrl+C</code> in its terminal)
          </Text>
          <Text variant="body-sm">
            2. Run <code>wooagent run</code> again
          </Text>
          <Text variant="body-sm">3. Reload this tab</Text>
        </Stack>
        <Stack direction="row" gap="sm" justify="end">
          <Button __next40pxDefaultSize variant="primary" onClick={onReload}>
            Reload this tab
          </Button>
        </Stack>
      </Stack>
    </Modal>
  );
}
