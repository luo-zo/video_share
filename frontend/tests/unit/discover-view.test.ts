import { flushPromises, mount } from '@vue/test-utils';
import { createMemoryHistory, createRouter } from 'vue-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import DiscoverView from '../../src/views/DiscoverView.vue';
import { videoClient } from '../../src/api';

vi.mock('../../src/api', () => ({
  videoClient: { listVideos: vi.fn() },
  taxonomyClient: { listCategories: vi.fn().mockResolvedValue({ items: [] }) },
}));

describe('DiscoverView', () => {
  beforeEach(() => {
    vi.mocked(videoClient.listVideos).mockReset().mockResolvedValue({ items: [], page: 1, page_size: 12, total: 0 });
  });

  it('loads from URL query and keeps pagination in browser history', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/', component: DiscoverView }],
    });
    await router.push('/?q=%E7%8C%AB&sort=popular&page=2');
    await router.isReady();
    const wrapper = mount(DiscoverView, { global: { plugins: [router] } });
    await flushPromises();

    expect(videoClient.listVideos).toHaveBeenLastCalledWith(expect.objectContaining({
      query: '猫', sort: 'popular', page: 2, signal: expect.any(AbortSignal),
    }));
    expect((wrapper.get('input[name="q"]').element as HTMLInputElement).value).toBe('猫');

    await wrapper.get('[data-action="previous-page"]').trigger('click');
    await vi.waitFor(() => expect(router.currentRoute.value.query.page).toBe('1'));
    router.back();
    await vi.waitFor(() => expect(router.currentRoute.value.query.page).toBe('2'));
    expect((wrapper.get('input[name="q"]').element as HTMLInputElement).value).toBe('猫');
  });

  it('aborts the active list request when unmounted', async () => {
    let capturedSignal: AbortSignal | undefined;
    vi.mocked(videoClient.listVideos).mockImplementation((options) => {
      capturedSignal = options?.signal;
      return new Promise(() => {});
    });
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: DiscoverView }] });
    await router.push('/');
    await router.isReady();
    const wrapper = mount(DiscoverView, { global: { plugins: [router] } });
    await vi.waitFor(() => expect(capturedSignal).toBeTruthy());

    wrapper.unmount();
    expect(capturedSignal?.aborted).toBe(true);
  });
});
