import { describe, expect, it, vi } from 'vitest';

import { createModerationClient } from '../../src/api/moderation';

describe('moderation API', () => {
  it('sends structured reports with CSRF and an idempotency key', async () => {
    const requestWithSession = vi.fn().mockResolvedValue({ id: 4, target_type: 'video', target_id: 8, status: 'open' });
    const client = createModerationClient({ requestWithSession });
    await client.createReport({ targetType: 'video', targetID: 8, reasonCode: 'spam', detail: '重复投稿', requestID: 'report-8' });
    expect(requestWithSession).toHaveBeenCalledWith('/reports', expect.objectContaining({ method: 'POST', csrf: true, withCredentials: true, headers: { 'Idempotency-Key': 'report-8' } }));
  });

  it('builds owner and admin pagination paths', async () => {
    const requestWithSession = vi.fn().mockResolvedValue({ items: [], page: 2, page_size: 20, total: 0 });
    const client = createModerationClient({ requestWithSession });
    await client.listMine({ page: 2, pageSize: 20 });
    await client.listAdmin({ page: 1, pageSize: 50 });
    await client.listActions({ page: 1, pageSize: 10 });
    expect(requestWithSession.mock.calls.map(([path]) => path)).toEqual([
      '/users/me/reports?page=2&page_size=20',
      '/admin/reports?page=1&page_size=50',
      '/admin/moderation-actions?page=1&page_size=10',
    ]);
  });
});
