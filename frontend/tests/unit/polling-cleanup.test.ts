import { afterEach, describe, expect, it, vi } from 'vitest';

import { createVideoClient } from '../../src/api/video';

describe('processing polling cleanup', () => {
  afterEach(() => vi.useRealTimers());

  it('clears the pending interval timer as soon as polling is aborted', async () => {
    vi.useFakeTimers();
    const authClient = {
      requestWithSession: vi.fn().mockResolvedValue({ id: 9, status: 'processing' }),
      requestPublic: vi.fn(),
      requestWithOptionalSession: vi.fn(),
    };
    const client = createVideoClient({ authClient });
    const controller = new AbortController();
    const pending = client.waitUntilProcessed(9, { intervalMs: 60_000, signal: controller.signal });
    const rejection = expect(pending).rejects.toMatchObject({ code: 'REQUEST_CANCELLED' });
    await Promise.resolve();
    await Promise.resolve();

    controller.abort();
    const timersAfterAbort = vi.getTimerCount();
    await vi.runAllTimersAsync();

    await rejection;
    expect(timersAfterAbort).toBe(0);
  });
});
