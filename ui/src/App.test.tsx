import { MemoryRouter } from 'react-router-dom';
import { render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import App from './App';

vi.mock('./components/AskAgentDrawer', () => ({
  default: () => null,
}));

const STORAGE_KEY = 'wooagent.connection';
const AUTH_NOTICE_FLAG = 'wooagent.authNoticeReason';

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    statusText: 'OK',
    headers: { 'Content-Type': 'application/json' },
  });
}

describe('App auth-expired flag detection', () => {
  it('renders the auth expired modal when the persistent 401 flag is present', async () => {
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ daemonUrl: 'http://daemon.test', token: 'token-123' }),
    );
    sessionStorage.setItem(AUTH_NOTICE_FLAG, 'persistent_401');
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValueOnce(
          jsonResponse({ status: 'ok', version: 'dev', schema_version: '1' }),
        )
        .mockResolvedValueOnce(
          jsonResponse({
            stores: [
              { id: 'store-1', url: 'https://store.test', status: 'paired' },
            ],
          }),
        )
        .mockResolvedValueOnce(
          jsonResponse({
            providers: [
              {
                id: 'provider-1',
                kind: 'anthropic',
                default_model: 'claude',
                is_default: true,
              },
            ],
          }),
        )
        .mockResolvedValueOnce(jsonResponse({ issues: [] }))
        .mockResolvedValueOnce(jsonResponse({ batches: [] })),
    );

    render(
      <MemoryRouter
        initialEntries={['/needs-review']}
        future={{ v7_relativeSplatPath: true, v7_startTransition: true }}
      >
        <App />
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(
        screen.getByRole('dialog', { name: 'Session expired' }),
      ).toBeInTheDocument();
    });
  });
});
