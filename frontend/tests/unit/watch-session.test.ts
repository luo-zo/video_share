import { ref } from 'vue';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { useWatchSession } from '../../src/composables/useWatchSession';
import { analyticsClient } from '../../src/api';

vi.mock('../../src/api', () => ({
  analyticsClient: {
    startWatchSession: vi.fn(),
    heartbeat: vi.fn(),
  },
}));

describe('useWatchSession', () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.mocked(analyticsClient.startWatchSession).mockReset();
    vi.mocked(analyticsClient.heartbeat).mockReset();
  });

  it('applies resume position and sends wall-clock heartbeats', async () => {
    vi.useFakeTimers();
    vi.mocked(analyticsClient.startWatchSession).mockResolvedValue({ session_id: 'session-1', video_id: 4, duration_ms: 10_000, resume_position_ms: 2_000, expires_at: '2026-01-01T00:00:00Z' });
    vi.mocked(analyticsClient.heartbeat).mockResolvedValue({ session_id: 'session-1', seq: 1, position_ms: 5000, accepted_delta_ms: 5000, session_credited_ms: 5000, effective_watch_ms: 5000, qualified: true, completed: false });
    const video = new EventTarget() as HTMLVideoElement;
    let currentTime = 0;
    let paused = false;
    Object.defineProperties(video, {
      duration: { configurable: true, get: () => 10 },
      currentTime: { configurable: true, get: () => currentTime, set: (value: number) => { currentTime = value; } },
      paused: { configurable: true, get: () => paused },
      ended: { configurable: true, get: () => false },
    });
    const element = ref<HTMLVideoElement | null>(video);
    const session = useWatchSession({ element, enabled: () => true });

    await session.start(4);
    expect(analyticsClient.startWatchSession).toHaveBeenCalledWith(4);
    expect(video.duration).toBe(10);
    session.applyResume();
    expect(video.currentTime).toBe(2);
    video.dispatchEvent(new Event('play'));
    vi.advanceTimersByTime(5000);
    await Promise.resolve();
    await Promise.resolve();
    expect(analyticsClient.heartbeat).toHaveBeenCalledWith('session-1', expect.objectContaining({ seq: 1, watched_delta_ms: 5000 }), expect.anything());

    paused = true;
    video.dispatchEvent(new Event('pause'));
    await Promise.resolve();
    const callsWhilePaused = vi.mocked(analyticsClient.heartbeat).mock.calls.length;
    vi.advanceTimersByTime(10_000);
    expect(vi.mocked(analyticsClient.heartbeat).mock.calls.length).toBe(callsWhilePaused);

    session.stop();
    const calls = vi.mocked(analyticsClient.heartbeat).mock.calls.length;
    vi.advanceTimersByTime(10_000);
    expect(vi.mocked(analyticsClient.heartbeat).mock.calls.length).toBe(calls);
  });
});
