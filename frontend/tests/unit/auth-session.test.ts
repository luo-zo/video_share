import { beforeEach, describe, expect, it, vi } from 'vitest';

import { createAuthClient } from '../../src/api/auth';

function response(data: unknown, status = 200) {
  return { ok: status >= 200 && status < 300, status, json: async () => status >= 400 ? { error: { code: data } } : { data } };
}

describe('persistent auth session', () => {
  beforeEach(() => { document.cookie = 'video_share_csrf=csrf-token; path=/'; });

  it('uses HttpOnly-cookie refresh flow while keeping access token in memory', async () => {
    const calls: Array<{ url: string; init: RequestInit }> = [];
    const fetchImpl = vi.fn(async (url: string, init: RequestInit) => {
      calls.push({ url, init });
      if (url.endsWith('/auth/csrf')) return response({ csrf_token: 'csrf-token' });
      if (url.endsWith('/auth/login')) return response({ access_token: 'access-1', token_type: 'Bearer', expires_in: 900 });
      return response({ id: 1, username: 'alice', nickname: 'Alice', bio: '', created_at: '2026-01-01T00:00:00Z' });
    });
    const client = createAuthClient({ fetchImpl, persistent: true });
    const user = await client.signIn({ username: 'Alice', password: 'password123' });
    expect(user.username).toBe('alice');
    expect(calls[0].url).toContain('/auth/csrf');
    expect(calls[1].init.credentials).toBe('include');
    expect((calls[1].init.headers as Record<string, string>)['X-CSRF-Token']).toBe('csrf-token');
    expect(client.getSession()?.user.username).toBe('alice');
    expect(JSON.stringify(client.getSession())).not.toContain('access-1');
  });

  it('refreshes once for concurrent 401s and does not clear on network failure', async () => {
    let protectedCalls = 0;
    let refreshCalls = 0;
    const fetchImpl = vi.fn(async (url: string) => {
      if (url.endsWith('/auth/csrf')) return response({ csrf_token: 'csrf-token' });
      if (url.endsWith('/auth/login')) return response({ access_token: 'access-1', token_type: 'Bearer', expires_in: 900 });
      if (url.endsWith('/auth/refresh')) { refreshCalls += 1; return response({ access_token: 'access-2', token_type: 'Bearer', expires_in: 900 }); }
      if (url.endsWith('/users/me')) return response({ id: 1, username: 'alice', nickname: 'Alice', bio: '', created_at: '2026-01-01T00:00:00Z' });
      protectedCalls += 1;
      if (protectedCalls <= 2) return response('UNAUTHORIZED', 401);
      return response({ ok: true });
    });
    const client = createAuthClient({ fetchImpl, persistent: true });
    await client.signIn({ username: 'alice', password: 'password123' });
    await Promise.all([client.requestWithSession('/protected'), client.requestWithSession('/protected')]);
    expect(refreshCalls).toBe(1);

    let offline = false;
    const stableFetch = vi.fn(async (url: string) => {
      if (offline) throw new TypeError('offline');
      if (url.endsWith('/auth/csrf')) return response({ csrf_token: 'csrf-token' });
      if (url.endsWith('/auth/login')) return response({ access_token: 'access-1', token_type: 'Bearer', expires_in: 900 });
      return response({ id: 1, username: 'alice', nickname: 'Alice', bio: '', created_at: '2026-01-01T00:00:00Z' });
    });
    const resilient = createAuthClient({ persistent: true, fetchImpl: stableFetch });
    await resilient.signIn({ username: 'alice', password: 'password123' });
    offline = true;
    await expect(resilient.requestWithSession('/protected')).rejects.toMatchObject({ code: 'NETWORK_ERROR' });
    expect(resilient.getSession()?.user.username).toBe('alice');

    let invalidCredentialRefreshes = 0;
    const noRetry = createAuthClient({
      persistent: true,
      fetchImpl: vi.fn(async (url: string) => {
        if (url.endsWith('/auth/csrf')) return response({ csrf_token: 'csrf-token' });
        if (url.endsWith('/auth/login')) return response({ access_token: 'access-1', token_type: 'Bearer', expires_in: 900 });
        if (url.endsWith('/auth/refresh')) { invalidCredentialRefreshes += 1; return response({ access_token: 'access-2', token_type: 'Bearer', expires_in: 900 }); }
        if (url.endsWith('/users/me/change-password')) return response('INVALID_CREDENTIALS', 401);
        return response({ id: 1, username: 'alice', nickname: 'Alice', bio: '', created_at: '2026-01-01T00:00:00Z' });
      }),
    });
    await noRetry.signIn({ username: 'alice', password: 'password123' });
    await expect(noRetry.changePassword({ oldPassword: 'wrong', newPassword: 'password456' })).rejects.toMatchObject({ code: 'INVALID_CREDENTIALS' });
    expect(invalidCredentialRefreshes).toBe(0);
  });

  it('restores through the HttpOnly cookie and caps refresh-conflict retries', async () => {
    const calls: string[] = [];
    const fetchImpl = vi.fn(async (url: string) => {
      calls.push(url);
      if (url.endsWith('/auth/csrf')) return response({ csrf_token: 'csrf-token' });
      if (url.endsWith('/auth/refresh')) return response({ access_token: 'access-restored', token_type: 'Bearer', expires_in: 900 });
      return response({ id: 2, username: 'restored', nickname: '恢复用户', bio: '', created_at: '2026-01-01T00:00:00Z' });
    });
    const client = createAuthClient({ fetchImpl, persistent: true });
    const restored = await client.restore();
    expect(restored?.username).toBe('restored');
    expect(calls).toEqual(['/api/v1/auth/csrf', '/api/v1/auth/refresh', '/api/v1/users/me']);
    expect(client.getSession()).not.toBeNull();

    let conflictCalls = 0;
    const conflict = createAuthClient({
      persistent: true,
      fetchImpl: vi.fn(async (url: string) => {
        if (url.endsWith('/auth/csrf')) return response({ csrf_token: 'csrf-token' });
        conflictCalls += 1;
        return response('REFRESH_CONFLICT', 409);
      }),
      sleep: vi.fn(async () => undefined),
    });
    await expect(conflict.restore()).rejects.toMatchObject({ code: 'REFRESH_CONFLICT', status: 409 });
    expect(conflictCalls).toBe(3);
  });

  it('does not restore a session when refresh completes after sign-out', async () => {
    let releaseRefresh: (() => void) | undefined;
    const refreshStarted = new Promise<void>((resolve) => { releaseRefresh = resolve; });
    let finishRefresh: (() => void) | undefined;
    const refreshResponse = new Promise<void>((resolve) => { finishRefresh = resolve; });
    const fetchImpl = vi.fn(async (url: string) => {
      if (url.endsWith('/auth/csrf')) return response({ csrf_token: 'csrf-token' });
      if (url.endsWith('/auth/login')) return response({ access_token: 'access-1', token_type: 'Bearer', expires_in: 900 });
      if (url.endsWith('/auth/refresh')) {
        releaseRefresh?.();
        await refreshResponse;
        return response({ access_token: 'access-late', token_type: 'Bearer', expires_in: 900 });
      }
      return response({ id: 3, username: 'race', nickname: '竞态', bio: '', created_at: '2026-01-01T00:00:00Z' });
    });
    const client = createAuthClient({ persistent: true, fetchImpl });
    await client.signIn({ username: 'race', password: 'password123' });
    const pending = client.refresh();
    await refreshStarted;
    await client.signOut();
    finishRefresh?.();
    await expect(pending).rejects.toMatchObject({ code: 'REQUEST_CANCELLED' });
    expect(client.getSession()).toBeNull();
  });
});
