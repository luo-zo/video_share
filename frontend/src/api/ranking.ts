import type { RequestOptions } from './client';
import type { SessionRequester, VideoItem } from './video';

export type RankingWindow = 'day' | 'week';

export interface RankingItem extends VideoItem {
  readonly video_id: string | number;
  readonly rank: number;
  readonly score: number;
}

export interface RankingList {
  readonly window: RankingWindow;
  readonly generated_at: string;
  readonly page: number;
  readonly page_size: number;
  readonly total: number;
  readonly items: readonly RankingItem[];
}

export interface RankingClient {
  list(options?: { window?: RankingWindow; page?: number; pageSize?: number; signal?: AbortSignal }): Promise<RankingList>;
}

function rankingItem(value: unknown): RankingItem {
  if (!value || typeof value !== 'object') throw new Error('服务器返回了无法识别的榜单数据。');
  const item = value as Record<string, unknown>;
  if (!['string', 'number'].includes(typeof item.id)
      || !['string', 'number'].includes(typeof item.video_id)
      || typeof item.rank !== 'number' || typeof item.score !== 'number') {
    throw new Error('服务器返回了无法识别的榜单数据。');
  }
  return value as RankingItem;
}

function listFrom(value: unknown): RankingList {
  if (!value || typeof value !== 'object') throw new Error('服务器返回了无法识别的榜单列表。');
  const data = value as Record<string, unknown>;
  if ((data.window !== 'day' && data.window !== 'week') || typeof data.generated_at !== 'string'
      || !Number.isInteger(data.page) || !Number.isInteger(data.page_size)
      || !Number.isInteger(data.total) || !Array.isArray(data.items)) {
    throw new Error('服务器返回了无法识别的榜单列表。');
  }
  data.items.forEach(rankingItem);
  return {
    window: data.window,
    generated_at: data.generated_at,
    page: data.page as number,
    page_size: data.page_size as number,
    total: data.total as number,
    items: data.items as RankingItem[],
  };
}

function positive(value: unknown, fallback: number): number {
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback;
}

export function createRankingClient({ authClient }: { authClient: SessionRequester }): RankingClient {
  if (!authClient || typeof authClient.requestPublic !== 'function') throw new TypeError('createRankingClient requires an auth client');
  return {
    async list({ window = 'day', page = 1, pageSize = 20, signal }: { window?: RankingWindow; page?: number; pageSize?: number; signal?: AbortSignal } = {}): Promise<RankingList> {
      const selected = window === 'week' ? 'week' : 'day';
      const params = new URLSearchParams({
        window: selected,
        page: String(positive(page, 1)),
        page_size: String(Math.min(50, positive(pageSize, 20))),
      });
      const options: RequestOptions = { signal };
      return listFrom(await authClient.requestPublic(`/videos/ranking?${params}`, options));
    },
  };
}
