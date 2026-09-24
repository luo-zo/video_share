import type { RequestOptions } from './client';
import type { VideoItem, VideoList } from './video';

const DEFAULT_PAGE_SIZE = 20;

export interface CreatorProfile {
  readonly id: string | number;
  readonly username: string;
  readonly nickname: string;
  readonly bio: string;
  readonly created_at: string;
  readonly video_count: number;
  readonly follower_count: number;
  readonly following_count: number;
  readonly following: boolean;
}

export interface CreatorRelation {
  readonly id: string | number;
  readonly username: string;
  readonly nickname: string;
}

export interface CreatorRelationList {
  readonly items: readonly CreatorRelation[];
  readonly page: number;
  readonly page_size: number;
  readonly total: number;
}

export interface CreatorRequester {
  requestWithOptionalSession(path: string, options?: RequestOptions): Promise<unknown>;
  requestPublic(path: string, options?: RequestOptions): Promise<unknown>;
}

function invalidResponse(): Error { return new Error('服务器返回了无法识别的创作者数据。'); }

function id(value: unknown): string {
  const raw = String(value);
  if (!/^\d+$/.test(raw) || Number(raw) < 1) throw invalidResponse();
  return encodeURIComponent(raw);
}

function pagePath(path: string, page: unknown, pageSize: unknown): string {
  const current = Number.isInteger(Number(page)) && Number(page) > 0 ? Number(page) : 1;
  const size = Number.isInteger(Number(pageSize)) && Number(pageSize) > 0 && Number(pageSize) <= 50 ? Number(pageSize) : DEFAULT_PAGE_SIZE;
  return `${path}?page=${current}&page_size=${size}`;
}

function profileFrom(data: unknown): CreatorProfile {
  if (!data || typeof data !== 'object') throw invalidResponse();
  const source = data as Record<string, unknown>;
  if (!['string', 'number'].includes(typeof source.id) || typeof source.username !== 'string' ||
      typeof source.nickname !== 'string' || typeof source.bio !== 'string' || typeof source.created_at !== 'string' ||
      !Number.isInteger(source.video_count) || !Number.isInteger(source.follower_count) ||
      !Number.isInteger(source.following_count) || typeof source.following !== 'boolean') throw invalidResponse();
  return data as CreatorProfile;
}

function videoListFrom(data: unknown): VideoList {
  if (!data || typeof data !== 'object') throw invalidResponse();
  const source = data as Record<string, unknown>;
  if (!Array.isArray(source.items) || !Number.isInteger(source.page) || !Number.isInteger(source.page_size) || !Number.isInteger(source.total)) throw invalidResponse();
  return { items: source.items as VideoItem[], page: source.page as number, page_size: source.page_size as number, total: source.total as number };
}

function relationListFrom(data: unknown): CreatorRelationList {
  if (!data || typeof data !== 'object') throw invalidResponse();
  const source = data as Record<string, unknown>;
  if (!Array.isArray(source.items) || !Number.isInteger(source.page) || !Number.isInteger(source.page_size) || !Number.isInteger(source.total)) throw invalidResponse();
  const items = source.items as unknown[];
  if (items.some((item) => !item || typeof item !== 'object' || !['string', 'number'].includes(typeof (item as Record<string, unknown>).id) || typeof (item as Record<string, unknown>).username !== 'string' || typeof (item as Record<string, unknown>).nickname !== 'string')) throw invalidResponse();
  return { items: items as CreatorRelation[], page: source.page as number, page_size: source.page_size as number, total: source.total as number };
}

export interface CreatorClient {
  getProfile(id: unknown, options?: RequestOptions): Promise<CreatorProfile>;
  listVideos(id: unknown, options?: { page?: unknown; pageSize?: unknown; signal?: AbortSignal }): Promise<VideoList>;
  listFollowers(id: unknown, options?: { page?: unknown; pageSize?: unknown; signal?: AbortSignal }): Promise<CreatorRelationList>;
  listFollowing(id: unknown, options?: { page?: unknown; pageSize?: unknown; signal?: AbortSignal }): Promise<CreatorRelationList>;
}

export function createCreatorClient({ authClient }: { authClient?: CreatorRequester } = {}): CreatorClient {
  if (!authClient) throw new TypeError('createCreatorClient requires an auth client');
  return {
    async getProfile(userID, options = {}) {
      return profileFrom(await authClient.requestWithOptionalSession(`/users/${id(userID)}`, options));
    },
    async listVideos(userID, { page = 1, pageSize = DEFAULT_PAGE_SIZE, signal } = {}) {
      return videoListFrom(await authClient.requestPublic(pagePath(`/users/${id(userID)}/videos`, page, pageSize), { signal }));
    },
    async listFollowers(userID, { page = 1, pageSize = DEFAULT_PAGE_SIZE, signal } = {}) {
      return relationListFrom(await authClient.requestPublic(pagePath(`/users/${id(userID)}/followers`, page, pageSize), { signal }));
    },
    async listFollowing(userID, { page = 1, pageSize = DEFAULT_PAGE_SIZE, signal } = {}) {
      return relationListFrom(await authClient.requestPublic(pagePath(`/users/${id(userID)}/follows`, page, pageSize), { signal }));
    },
  };
}
