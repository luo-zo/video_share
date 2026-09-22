import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { createMemoryHistory, createRouter } from 'vue-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { videoClient } from '../../src/api';
import ProfileView from '../../src/views/ProfileView.vue';

vi.mock('../../src/api', () => ({
  videoClient: { listMyVideos: vi.fn() },
  communityClient: { listFavorites: vi.fn(), listHistory: vi.fn(), listFollows: vi.fn() },
}));

describe('ProfileView owner navigation', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.mocked(videoClient.listMyVideos).mockReset().mockResolvedValue({
      items: [{ id: 9, title: '私密猫片', status: 'failed', visibility: 'private' }],
      page: 1, page_size: 12, total: 1,
    });
  });

  it('links owned submissions to the owner detail path and focuses the route title', async () => {
    const router = createRouter({ history: createMemoryHistory(), routes: [
      { path: '/me', component: ProfileView },
      { path: '/video/:id', component: { template: '<div>detail</div>' } },
    ] });
    await router.push('/me');
    await router.isReady();
    const wrapper = mount(ProfileView, { attachTo: document.body, global: { plugins: [router] } });
    await flushPromises();

    expect(wrapper.get('.video-card').attributes('href')).toBe('/video/9?owner=1');
    expect(document.activeElement).toBe(wrapper.get('#profile-title').element);
    wrapper.unmount();
  });
});
