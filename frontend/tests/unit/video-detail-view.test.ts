import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { createMemoryHistory, createRouter } from 'vue-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { communityClient, videoClient } from '../../src/api';
import type { VideoItem } from '../../src/api/video';
import { useAuthStore } from '../../src/stores/auth';
import VideoDetailView from '../../src/views/VideoDetailView.vue';

vi.mock('../../src/api', () => ({
  videoClient: {
    getVideo: vi.fn(), getMyVideo: vi.fn(), updateVideo: vi.fn(), deleteVideo: vi.fn(),
  },
  communityClient: {
    listComments: vi.fn(), setLike: vi.fn(), setFavorite: vi.fn(), setFollow: vi.fn(),
    addComment: vi.fn(), deleteComment: vi.fn(), reportWatch: vi.fn(),
  },
}));

const item = (id: number, overrides: Partial<VideoItem> = {}): VideoItem => ({
  id, user_id: 7, title: `视频 ${id}`, description: '简介', status: 'ready', visibility: 'public',
  play_url: `/media/${id}.mp4`, play_type: 'mp4', author: { id: 7, nickname: '小猫' },
  stats: { like_count: 4, favorite_count: 2, comment_count: 0, view_count: 8 },
  viewer_state: { liked: false, favorited: false, following_author: false },
  ...overrides,
});

function deferred<T>(): { promise: Promise<T>; resolve: (value: T) => void } {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}

async function mountAt(path: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/video/:id', component: VideoDetailView },
      { path: '/me', component: { template: '<div>profile</div>' } },
      { path: '/login', name: 'login', component: { template: '<div>login</div>' } },
    ],
  });
  await router.push(path);
  await router.isReady();
  const wrapper = mount(VideoDetailView, {
    global: { plugins: [router], stubs: { VideoPlayer: true, CommentList: true } },
  });
  await flushPromises();
  return { router, wrapper };
}

describe('VideoDetailView owner and route lifecycle', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    useAuthStore().$patch({
      user: { id: 7, username: 'milo', nickname: '小猫', created_at: '2026-09-07T00:00:00Z' },
      expiresAt: Date.now() + 60_000,
    });
    vi.mocked(videoClient.getVideo).mockReset().mockImplementation(async (id) => item(Number(id)));
    vi.mocked(videoClient.getMyVideo).mockReset().mockImplementation(async (id) => item(Number(id)));
    vi.mocked(videoClient.updateVideo).mockReset().mockImplementation(async (id, patch) => item(Number(id), {
      ...(typeof patch.title === 'string' ? { title: patch.title } : {}),
      ...(typeof patch.description === 'string' ? { description: patch.description } : {}),
      ...(typeof patch.visibility === 'string' ? { visibility: patch.visibility } : {}),
    }));
    vi.mocked(videoClient.deleteVideo).mockReset().mockImplementation(async (id) => item(Number(id), { status: 'deleted' }));
    vi.mocked(communityClient.listComments).mockReset().mockResolvedValue({ items: [], page: 1, page_size: 50, total: 0 });
    vi.mocked(communityClient.setLike).mockReset();
    vi.mocked(communityClient.setFavorite).mockReset();
    vi.mocked(communityClient.setFollow).mockReset();
    vi.mocked(communityClient.addComment).mockReset();
    vi.mocked(communityClient.deleteComment).mockReset();
  });

  it('loads private submissions through getMyVideo and supports owner editing', async () => {
    vi.mocked(videoClient.getMyVideo).mockResolvedValue(item(9, {
      title: '私密猫片', status: 'failed', visibility: 'private', play_url: undefined,
    }));
    const { wrapper } = await mountAt('/video/9?owner=1');

    expect(videoClient.getMyVideo).toHaveBeenCalledWith('9', expect.objectContaining({ signal: expect.any(AbortSignal) }));
    expect(videoClient.getVideo).not.toHaveBeenCalled();
    await wrapper.get('.detail-owner > .soft-button').trigger('click');
    await wrapper.get('#owner-title').setValue('修复后的标题');
    await wrapper.get('form.owner-edit').trigger('submit');
    await vi.waitFor(() => expect(videoClient.updateVideo).toHaveBeenCalledWith('9', expect.objectContaining({ title: '修复后的标题' })));
  });

  it('keeps ordinary detail navigation on the public endpoint', async () => {
    await mountAt('/video/3');
    expect(videoClient.getVideo).toHaveBeenCalledWith('3', expect.objectContaining({ signal: expect.any(AbortSignal) }));
    expect(videoClient.getMyVideo).not.toHaveBeenCalled();
  });

  it('ignores a late like response after switching to a new video', async () => {
    const pending = deferred<{ active: boolean; count: number }>();
    vi.mocked(communityClient.setLike).mockReturnValue(pending.promise);
    const { router, wrapper } = await mountAt('/video/1');
    await wrapper.get('.action-button').trigger('click');
    await router.push('/video/2');
    await vi.waitFor(() => expect(wrapper.get('#detail-title').text()).toBe('视频 2'));

    pending.resolve({ active: true, count: 99 });
    await flushPromises();

    const like = wrapper.get('.action-button');
    expect(like.attributes('aria-pressed')).toBe('false');
    expect(like.text()).toContain('4');
  });

  it('does not navigate away when an old owner delete resolves after a route switch', async () => {
    const pending = deferred<VideoItem>();
    vi.mocked(videoClient.deleteVideo).mockReturnValue(pending.promise);
    const { router, wrapper } = await mountAt('/video/1?owner=1');
    await wrapper.get('.detail-owner > .soft-button').trigger('click');
    await wrapper.get('.owner-delete').trigger('click');
    await wrapper.get('.confirm-delete').trigger('click');
    await router.push('/video/2?owner=1');
    await vi.waitFor(() => expect(wrapper.get('#detail-title').text()).toBe('视频 2'));

    pending.resolve(item(1, { status: 'deleted' }));
    await flushPromises();
    expect(router.currentRoute.value.fullPath).toBe('/video/2?owner=1');
  });
});
