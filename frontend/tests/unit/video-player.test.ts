import { mount } from '@vue/test-utils';
import { nextTick } from 'vue';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { VideoItem } from '../../src/api/video';
import VideoPlayer from '../../src/components/VideoPlayer.vue';
import { attachVideoSource } from '../../src/lib/player';

vi.mock('../../src/lib/player', () => ({
  attachVideoSource: vi.fn(),
}));

const first: VideoItem = { id: 1, title: '第一段', play_url: '/one.m3u8', play_type: 'hls' };
const second: VideoItem = { id: 2, title: '第二段', play_url: '/two.mp4', play_type: 'mp4' };

describe('VideoPlayer', () => {
  beforeEach(() => {
    vi.mocked(attachVideoSource).mockReset();
  });

  it('detaches the previous source when videos change rapidly and again on unmount', async () => {
    const detachFirst = vi.fn();
    const detachSecond = vi.fn();
    vi.mocked(attachVideoSource)
      .mockResolvedValueOnce(detachFirst)
      .mockResolvedValueOnce(detachSecond);

    const wrapper = mount(VideoPlayer, { props: { video: first } });
    await nextTick();
    await vi.waitFor(() => expect(attachVideoSource).toHaveBeenCalledTimes(1));

    await wrapper.setProps({ video: second });
    await vi.waitFor(() => expect(attachVideoSource).toHaveBeenCalledTimes(2));
    expect(detachFirst).toHaveBeenCalledOnce();

    wrapper.unmount();
    expect(detachSecond).toHaveBeenCalledOnce();
  });

  it('discards a late attachment after the video has already changed', async () => {
    let resolveFirst: ((detach: () => void) => void) | undefined;
    const lateDetach = vi.fn();
    const currentDetach = vi.fn();
    vi.mocked(attachVideoSource)
      .mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve; }))
      .mockResolvedValueOnce(currentDetach);

    const wrapper = mount(VideoPlayer, { props: { video: first } });
    await nextTick();
    await wrapper.setProps({ video: second });
    await vi.waitFor(() => expect(attachVideoSource).toHaveBeenCalledTimes(2));

    resolveFirst?.(lateDetach);
    await nextTick();
    expect(lateDetach).toHaveBeenCalledOnce();

    wrapper.unmount();
    expect(currentDetach).toHaveBeenCalledOnce();
  });
});
