import { defineStore } from 'pinia';
import { ref } from 'vue';

import { notificationClient } from '../api';
import type { NotificationItem } from '../api/notification';

const POLL_MS = 30_000;

export const useNotificationsStore = defineStore('notifications', () => {
  const items = ref<readonly NotificationItem[]>([]);
  const unreadCount = ref(0);
  const loading = ref(false);
  const error = ref('');
  let timer: number | undefined;
  let visibilityHandler: (() => void) | undefined;
  let generation = 0;
  let controller: AbortController | undefined;

  async function refresh(): Promise<void> {
    const current = ++generation;
    controller?.abort();
    controller = new AbortController();
    loading.value = true;
    try {
      const result = await notificationClient.list({ page: 1, pageSize: 20, signal: controller.signal });
      if (current !== generation) return;
      items.value = result.items;
      unreadCount.value = result.unread_count;
      error.value = '';
    } catch (caught) {
      if (current !== generation || controller?.signal.aborted) return;
      error.value = caught instanceof Error ? caught.message : '通知加载失败。';
    } finally {
      if (current === generation) loading.value = false;
    }
  }

  function start(): void {
    if (typeof window === 'undefined' || timer !== undefined) return;
    void refresh();
    const tick = (): void => {
      if (!document.hidden) void refresh();
    };
    visibilityHandler = tick;
    timer = window.setInterval(tick, POLL_MS);
    document.addEventListener('visibilitychange', tick);
  }

  function stop(): void {
    generation += 1;
    controller?.abort();
    controller = undefined;
    if (timer !== undefined) window.clearInterval(timer);
    timer = undefined;
    if (typeof document !== 'undefined' && visibilityHandler) document.removeEventListener('visibilitychange', visibilityHandler);
    visibilityHandler = undefined;
    items.value = [];
    unreadCount.value = 0;
    error.value = '';
  }

  async function markRead(id: string | number): Promise<void> {
    const target = String(id);
    const wasUnreadInPage = items.value.some((item) => String(item.id) === target && !item.read_at);
    const operationGeneration = ++generation;
    controller?.abort();
    controller = undefined;
    await notificationClient.markRead(id);
    // A poll may have started while the write was in flight. Its response is
    // no longer authoritative for this mutation; force a fresh fenced read.
    if (operationGeneration !== generation) {
      void refresh();
      return;
    }
    items.value = items.value.map((item) => String(item.id) === target ? { ...item, read_at: new Date().toISOString() } : item);
    // unread_count covers all pages; only decrement when this page contained
    // an unread item. A notification outside the first page must not reset the
    // server-provided global count to the number of visible rows.
    if (wasUnreadInPage) unreadCount.value = Math.max(0, unreadCount.value - 1);
    void refresh();
  }

  async function markAllRead(): Promise<void> {
    const operationGeneration = ++generation;
    controller?.abort();
    controller = undefined;
    await notificationClient.markAllRead();
    if (operationGeneration !== generation) {
      void refresh();
      return;
    }
    items.value = items.value.map((item) => item.read_at ? item : { ...item, read_at: new Date().toISOString() });
    unreadCount.value = 0;
    void refresh();
  }

  function clear(): void { items.value = []; unreadCount.value = 0; error.value = ''; }

  return { items, unreadCount, loading, error, refresh, start, stop, markRead, markAllRead, clear };
});
