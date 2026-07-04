import { describe, expect, it, vi } from 'vitest';
import { ApiError, api, type Connection } from '../api/client';
import { probeOnboardingResume } from './OnboardingShell';

const connection: Connection = {
  daemonUrl: 'http://daemon.test',
  token: 'token-123',
};

describe('probeOnboardingResume', () => {
  it('rethrows 401s from the stores probe so auth recovery can take over', async () => {
    const authError = new ApiError(401, 'auth_expired', 'Session expired.');
    vi.spyOn(api.stores, 'list').mockRejectedValue(authError);
    vi.spyOn(api.modelProviders, 'list').mockResolvedValue({ providers: [] });

    await expect(probeOnboardingResume(connection)).rejects.toBe(authError);
  });

  it('treats non-auth resume probe failures as empty state', async () => {
    vi.spyOn(api.stores, 'list').mockRejectedValue(new Error('offline'));
    vi.spyOn(api.modelProviders, 'list').mockRejectedValue(new Error('offline'));

    await expect(probeOnboardingResume(connection)).resolves.toEqual({
      store: null,
      provider: null,
    });
  });
});
