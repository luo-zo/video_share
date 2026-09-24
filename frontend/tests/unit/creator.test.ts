import { describe, expect, it, vi } from 'vitest';

import { createCreatorClient } from '../../src/api/creator';

const profile = { id: 7, username: 'creator', nickname: '创作者', bio: '简介', created_at: '2026-01-01T00:00:00Z', video_count: 1, follower_count: 2, following_count: 3, following: false };

describe('creator API', () => {
  it('loads public profile with optional auth and paged relations', async () => {
    const requestWithOptionalSession = vi.fn(async (path: string) => path.endsWith('/users/7') ? profile : null);
    const requestPublic = vi.fn(async (path: string) => {
      if (path.includes('/followers')) return { items: [{ id: 8, username: 'follower', nickname: '粉丝' }], page: 1, page_size: 20, total: 1 };
      if (path.includes('/follows')) return { items: [], page: 2, page_size: 1, total: 2 };
      return { items: [{ id: 4, title: '公开作品' }], page: 1, page_size: 20, total: 1 };
    });
    const client = createCreatorClient({ authClient: { requestWithOptionalSession, requestPublic } });
    expect(await client.getProfile(7)).toEqual(profile);
    expect((await client.listVideos(7)).total).toBe(1);
    expect((await client.listFollowers(7)).items[0].username).toBe('follower');
    expect((await client.listFollowing(7, { page: 2, pageSize: 1 })).page).toBe(2);
    expect(requestWithOptionalSession).toHaveBeenCalledWith('/users/7', {});
    expect(requestPublic).toHaveBeenCalledWith('/users/7/follows?page=2&page_size=1', { signal: undefined });
  });

  it('rejects an invalid creator id before making a request', async () => {
    const requestPublic = vi.fn();
    const client = createCreatorClient({ authClient: { requestWithOptionalSession: vi.fn(), requestPublic } });
    await expect(client.listVideos('0')).rejects.toThrow();
    expect(requestPublic).not.toHaveBeenCalled();
  });
});
