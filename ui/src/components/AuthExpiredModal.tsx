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
