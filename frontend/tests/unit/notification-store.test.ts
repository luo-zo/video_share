import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  markRead: vi.fn(),
  markAllRead: vi.fn(),
}));

vi.mock('../../src/api', () => ({
  notificationClient: mocks,
}));

import { useNotificationsStore } from '../../src/stores/notifications';

const page = (unreadCount: number) => ({
  items: [{ id: 1, recipient_id: 7, type: 'follow', created_at: '2026-09-23T00:00:00Z' }],
  page: 1, page_size: 20, total: 50, unread_count: unreadCount,
});

describe('notification store mutations', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    mocks.list.mockReset();
    mocks.markRead.mockReset().mockResolvedValue(undefined);
    mocks.markAllRead.mockReset().mockResolvedValue(undefined);
  });

  it('keeps the server global unread count when marking an item outside the first page', async () => {
    mocks.list.mockResolvedValue(page(50));
    const store = useNotificationsStore();
    await store.refresh();

    await store.markRead(999);

    expect(store.unreadCount).toBe(50);
    expect(mocks.markRead).toHaveBeenCalledWith(999);
  });
});
