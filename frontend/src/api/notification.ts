import type { RequestOptions } from './client';

export interface NotificationItem {
  readonly id: string | number;
  readonly recipient_id: string | number;
  readonly actor_id?: string | number;
  readonly type: string;
  readonly video_id?: string | number;
  readonly comment_id?: string | number;
  readonly report_id?: string | number;
  readonly read_at?: string;
  readonly created_at: string;
}

export interface NotificationList {
  readonly items: readonly NotificationItem[];
  readonly page: number;
  readonly page_size: number;
  readonly total: number;
  readonly unread_count: number;
}

export interface NotificationRequester {
  requestWithSession(path: string, options?: RequestOptions): Promise<unknown>;
}

export class NotificationError extends Error {
  readonly code: string;
  readonly status: number;

  constructor(message: string, code = 'NOTIFICATION_ERROR', status = 0) {
    super(message);
    this.name = 'NotificationError';
    this.code = code;
    this.status = status;
  }
}

function listFrom(value: unknown): NotificationList {
  if (!value || typeof value !== 'object') throw new NotificationError('通知响应无效。', 'INVALID_RESPONSE');
  const source = value as Record<string, unknown>;
  if (!Array.isArray(source.items) || !Number.isInteger(source.page) || !Number.isInteger(source.page_size) || !Number.isInteger(source.total) || !Number.isInteger(source.unread_count)) {
    throw new NotificationError('通知响应无效。', 'INVALID_RESPONSE');
  }
  return { items: source.items as readonly NotificationItem[], page: source.page as number, page_size: source.page_size as number, total: source.total as number, unread_count: source.unread_count as number };
}

function positive(value: unknown, fallback: number): number {
  const n = Number(value);
  return Number.isInteger(n) && n > 0 ? n : fallback;
}

export function createNotificationClient(requester: NotificationRequester) {
  const pagePath = (page: unknown, pageSize: unknown) => `/notifications?page=${positive(page, 1)}&page_size=${positive(pageSize, 20)}`;
  return {
    async list(options: { page?: unknown; pageSize?: unknown; signal?: AbortSignal } = {}): Promise<NotificationList> {
      return listFrom(await requester.requestWithSession(pagePath(options.page, options.pageSize), { signal: options.signal }));
    },
    async markRead(id: string | number): Promise<void> {
      await requester.requestWithSession(`/notifications/${encodeURIComponent(String(id))}/read`, { method: 'PATCH', body: {}, withCredentials: true, csrf: true });
    },
    async markAllRead(): Promise<void> {
      await requester.requestWithSession('/notifications/read-all', { method: 'POST', body: {}, withCredentials: true, csrf: true });
    },
  };
}
