import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import AuthExpiredModal from './AuthExpiredModal';

const STORAGE_KEY = 'wooagent.connection';
const AUTH_RELOAD_FLAG = 'wooagent.authReloadAttempted';
const AUTH_NOTICE_FLAG = 'wooagent.authNoticeReason';

describe('AuthExpiredModal', () => {
  it('clears auth recovery state before reloading the tab', () => {
    localStorage.setItem(
      STORAGE_KEY,
      '{"daemonUrl":"http://daemon.test","token":"token"}',
    );
    sessionStorage.setItem(AUTH_RELOAD_FLAG, '1');
    sessionStorage.setItem(AUTH_NOTICE_FLAG, 'persistent_401');
    vi.spyOn(console, 'error').mockImplementation(() => {});

    render(<AuthExpiredModal />);

    fireEvent.click(screen.getByRole('button', { name: 'Reload this tab' }));

    expect(localStorage.getItem(STORAGE_KEY)).toBeNull();
    expect(sessionStorage.getItem(AUTH_RELOAD_FLAG)).toBeNull();
    expect(sessionStorage.getItem(AUTH_NOTICE_FLAG)).toBeNull();
  });
});
