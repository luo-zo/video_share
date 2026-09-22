import { mount } from '@vue/test-utils';
import { createMemoryHistory, createRouter } from 'vue-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { videoClient } from '../../src/api';
import { VideoError } from '../../src/api/video';
import UploadView from '../../src/views/UploadView.vue';

vi.mock('../../src/api', () => ({
  videoClient: { uploadVideo: vi.fn(), waitUntilProcessed: vi.fn() },
}));

function file(): File {
  return new File(['video'], 'cat.mp4', { type: 'video/mp4' });
}

async function chooseFile(wrapper: ReturnType<typeof mount>): Promise<void> {
  const input = wrapper.get('input[type="file"]');
  Object.defineProperty(input.element, 'files', { configurable: true, value: [file()] });
  await input.trigger('change');
}

describe('UploadView', () => {
  beforeEach(() => {
    vi.mocked(videoClient.uploadVideo).mockReset();
    vi.mocked(videoClient.waitUntilProcessed).mockReset();
  });

  it('keeps upload failures visible beside the form', async () => {
    vi.mocked(videoClient.uploadVideo).mockRejectedValue(new VideoError('视频直传失败，请重新投稿。', {
      code: 'UPLOAD_FAILED', fieldErrors: { file: '请重新选择视频。' },
    }));
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/upload', component: UploadView }] });
    await router.push('/upload');
    await router.isReady();
    const wrapper = mount(UploadView, { global: { plugins: [router] } });
    await wrapper.get('input[name="title"]').setValue('猫的下午');
    await chooseFile(wrapper);
    await wrapper.get('form').trigger('submit');

    await vi.waitFor(() => expect(wrapper.get('[role="alert"]').text()).toContain('视频直传失败'));
    expect(wrapper.text()).toContain('请重新选择视频。');
  });

  it('aborts processing polling on unmount', async () => {
    vi.mocked(videoClient.uploadVideo).mockResolvedValue({ id: 9, status: 'processing' });
    let pollingSignal: AbortSignal | undefined;
    vi.mocked(videoClient.waitUntilProcessed).mockImplementation((_id, options) => {
      pollingSignal = options?.signal;
      return new Promise(() => {});
    });
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/upload', component: UploadView }] });
    await router.push('/upload');
    await router.isReady();
    const wrapper = mount(UploadView, { global: { plugins: [router] } });
    await wrapper.get('input[name="title"]').setValue('猫的下午');
    await chooseFile(wrapper);
    await wrapper.get('form').trigger('submit');
    await vi.waitFor(() => expect(pollingSignal).toBeTruthy());

    wrapper.unmount();
    expect(pollingSignal?.aborted).toBe(true);
  });
});
