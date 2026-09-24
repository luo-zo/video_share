import { describe, expect, it, vi } from 'vitest';

import { createNotificationClient } from '../../src/api/notification';

describe('notification API client', () => {
  it('reads paginated notifications with an unread count', async () => {
    const requestWithSession = vi.fn().mockResolvedValue({
      items: [{ id: 3, recipient_id: 7, type: 'comment', created_at: '2026-09-23T00:00:00Z' }],
      page: 1, page_size: 20, total: 1, unread_count: 1,
    });
    const client = createNotificationClient({ requestWithSession });
    const result = await client.list({ page: 1, pageSize: 20 });
    expect(result.unread_count).toBe(1);
    expect(requestWithSession).toHaveBeenCalledWith('/notifications?page=1&page_size=20', expect.any(Object));
  });

  it('uses owner-scoped read endpoints', async () => {
    const requestWithSession = vi.fn().mockResolvedValue({});
    const client = createNotificationClient({ requestWithSession });
    await client.markRead(9);
    await client.markAllRead();
    expect(requestWithSession).toHaveBeenNthCalledWith(1, '/notifications/9/read', expect.objectContaining({ method: 'PATCH' }));
    expect(requestWithSession).toHaveBeenNthCalledWith(2, '/notifications/read-all', expect.objectContaining({ method: 'POST' }));
  });
});
