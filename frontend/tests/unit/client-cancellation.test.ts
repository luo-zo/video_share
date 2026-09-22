import { describe, expect, it, vi } from 'vitest';

import { createRequester } from '../../src/api/client';

describe('API request cancellation', () => {
  it('maps a caller abort to REQUEST_CANCELLED instead of a timeout', async () => {
    const fetchImpl = vi.fn((_url: string, init: RequestInit) => new Promise<never>((_resolve, reject) => {
      init.signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')));
    }));
    const request = createRequester({ fetchImpl, timeoutMs: 60_000 });
    const controller = new AbortController();

    const pending = request('/videos?page=1', { signal: controller.signal });
    controller.abort();

    await expect(pending).rejects.toMatchObject({ code: 'REQUEST_CANCELLED' });
  });
});
