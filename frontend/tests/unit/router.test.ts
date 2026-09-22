import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';

import { createAppRouter, safeReturnTo } from '../../src/router';
import { useAuthStore } from '../../src/stores/auth';

beforeEach(() => {
  setActivePinia(createPinia());
  window.history.replaceState({}, '', '/');
  window.scrollTo = () => undefined;
});

describe('safeReturnTo', () => {
  it('keeps an internal path with its query and hash', () => {
    expect(safeReturnTo('/video/7?from=cat#comments')).toBe('/video/7?from=cat#comments');
  });

  it.each(['//evil.test/path', '/\\evil.test/path', 'https://evil.test/path', '/login?returnTo=/me']) (
    'rejects unsafe or recursive login destination %s',
    (value) => {
      expect(safeReturnTo(value, '/fallback')).toBe('/fallback');
    },
  );
});

describe('owner detail route protection', () => {
  it('redirects an anonymous owner deep link to login with the complete safe return path', async () => {
    const router = createAppRouter();
    await router.push('/video/9?owner=1&tab=processing#owner');
    await router.isReady();

    expect(router.currentRoute.value.name).toBe('login');
    expect(router.currentRoute.value.query.returnTo).toBe('/video/9?owner=1&tab=processing#owner');
  });

  it('keeps public detail anonymous and permits authenticated owner mode', async () => {
    const router = createAppRouter();
    await router.push('/video/9');
    await router.isReady();
    expect(router.currentRoute.value.name).toBe('video-detail');

    useAuthStore().$patch({
      user: { id: 7, username: 'milo', nickname: '小猫', created_at: '2026-09-07T00:00:00Z' },
      expiresAt: Date.now() + 60_000,
    });
    await router.push('/video/9?owner=1');
    expect(router.currentRoute.value.name).toBe('video-detail');
    expect(router.currentRoute.value.query.owner).toBe('1');
  });
});
