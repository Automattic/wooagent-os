import { describe, expect, it, vi } from 'vitest';
import { ApiError, api, type Connection } from './client';

const STORAGE_KEY = 'wooagent.connection';
const AUTH_RELOAD_FLAG = 'wooagent.authReloadAttempted';
const AUTH_NOTICE_FLAG = 'wooagent.authNoticeReason';

const connection: Connection = {
  daemonUrl: 'http://daemon.test',
  token: 'token-123',
};

function jsonResponse(body: unknown, init: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
}

describe('auth recovery request handling', () => {
  it('clears stored connection and records reload attempt on the first 401', async () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(connection));
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        jsonResponse(
          { error: { code: 'unauthorized', message: 'Bad token' } },
          { status: 401, statusText: 'Unauthorized' },
        ),
      ),
    );

    await expect(api.agents(connection)).rejects.toMatchObject({
      status: 401,
      code: 'auth_expired',
    } satisfies Partial<ApiError>);

    expect(localStorage.getItem(STORAGE_KEY)).toBeNull();
    expect(sessionStorage.getItem(AUTH_RELOAD_FLAG)).toBe('1');
    expect(sessionStorage.getItem(AUTH_NOTICE_FLAG)).toBeNull();
  });

  it('sets the persistent notice flag on a second 401 in the same session', async () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(connection));
    sessionStorage.setItem(AUTH_RELOAD_FLAG, '1');
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        jsonResponse(
          { error: { code: 'unauthorized', message: 'Bad token' } },
          { status: 401, statusText: 'Unauthorized' },
        ),
      ),
    );

    await expect(api.agents(connection)).rejects.toMatchObject({
      status: 401,
      code: 'auth_expired',
      message: 'Session expired and reload did not recover.',
    } satisfies Partial<ApiError>);

    expect(localStorage.getItem(STORAGE_KEY)).toBe(JSON.stringify(connection));
    expect(sessionStorage.getItem(AUTH_RELOAD_FLAG)).toBe('1');
    expect(sessionStorage.getItem(AUTH_NOTICE_FLAG)).toBe('persistent_401');
  });

  it('clears auth recovery flags after a successful authenticated request', async () => {
    sessionStorage.setItem(AUTH_RELOAD_FLAG, '1');
    sessionStorage.setItem(AUTH_NOTICE_FLAG, 'persistent_401');
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        jsonResponse({ agents: [] }, { status: 200, statusText: 'OK' }),
      ),
    );

    await expect(api.agents(connection)).resolves.toEqual({ agents: [] });

    expect(sessionStorage.getItem(AUTH_RELOAD_FLAG)).toBeNull();
    expect(sessionStorage.getItem(AUTH_NOTICE_FLAG)).toBeNull();
  });
});
