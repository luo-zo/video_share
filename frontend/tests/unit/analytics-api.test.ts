import { describe, expect, it, vi } from 'vitest';

import { createAnalyticsClient } from '../../src/api/analytics';

describe('analytics API', () => {
  it('starts sessions, sends bounded heartbeats, and loads creator summaries', async () => {
    const requestWithSession = vi.fn(async (path: string, _options?: unknown) => {
      if (path.includes('creator/summary')) return { days: 7, metric_version: 'effective-watch-v1', as_of: '2026-01-01T00:00:00Z', current: {}, trend: [], top_videos: [] };
      if (path.endsWith('/watch-sessions')) return { session_id: 'session-1', video_id: 8, duration_ms: 10_000, resume_position_ms: 1200, expires_at: '2026-01-01T00:00:00Z' };
      return { session_id: 'session-1', seq: 1, accepted_delta_ms: 4000, effective_watch_ms: 4000, session_credited_ms: 4000, position_ms: 4000, qualified: true, completed: false };
    });
    const client = createAnalyticsClient({ requestWithSession, requestPublic: vi.fn(), requestWithOptionalSession: vi.fn() });
    await expect(client.startWatchSession(8)).resolves.toMatchObject({ session_id: 'session-1' });
    await client.heartbeat('session-1', { seq: 1, position_ms: 4000, watched_delta_ms: 4000 });
    await client.creatorSummary(7);
    expect(requestWithSession.mock.calls.map((call) => call[0])).toEqual([
      '/videos/8/watch-sessions',
      '/watch-sessions/session-1/heartbeat',
      '/users/me/creator/summary?days=7',
    ]);
    expect(requestWithSession.mock.calls[1][1]).toEqual(expect.objectContaining({ method: 'POST', body: { seq: 1, position_ms: 4000, watched_delta_ms: 4000 } }));
  });

  it('rejects invalid identifiers, sequence values, deltas, and ranges before requesting', async () => {
    const requestWithSession = vi.fn();
    const client = createAnalyticsClient({ requestWithSession, requestPublic: vi.fn(), requestWithOptionalSession: vi.fn() });
    await expect(client.startWatchSession('0')).rejects.toThrow('INVALID_PARAMETER');
    await expect(client.heartbeat('', { seq: 1, position_ms: 0, watched_delta_ms: 0 })).rejects.toThrow('INVALID_PARAMETER');
    await expect(client.heartbeat('s', { seq: 1, position_ms: 0, watched_delta_ms: 15_001 })).rejects.toThrow('INVALID_PARAMETER');
    await expect(client.creatorSummary(14 as 7)).rejects.toThrow('INVALID_PARAMETER');
    expect(requestWithSession).not.toHaveBeenCalled();
  });
});
