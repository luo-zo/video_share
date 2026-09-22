// 社区互动：评论、点赞、收藏、关注、观看进度上报。
// 与旧 community.js 的路径、方法、校验规则保持一致。
import type { FieldErrors, RequestOptions, ValidationResult } from './client';
import { text } from './client';
import type { VideoItem } from './video';

const MAX_COMMENT_LENGTH = 500;
const DEFAULT_PAGE_SIZE = 12;

export interface CommunityErrorInit {
  code?: string;
  status?: number;
  fieldErrors?: FieldErrors;
}

export class CommunityError extends Error {
  readonly code: string;

  readonly status: number;

  readonly fieldErrors: FieldErrors;

  constructor(
    message: string,
    { code = 'COMMUNITY_ERROR', status = 0, fieldErrors = {} }: CommunityErrorInit = {},
  ) {
    super(message);
    this.name = 'CommunityError';
    this.code = code;
    this.status = status;
    this.fieldErrors = fieldErrors;
  }
}

export interface CommentInput {
  content?: unknown;
}

export interface CommentValues {
  content: string;
}

export function validateComment(input: CommentInput = {}): ValidationResult<CommentValues> {
  const source = (input ?? {}) as Record<string, unknown>;
  const content = text(source.content).trim();
  const errors: Record<string, string> = {};
  const length = Array.from(content).length;
  if (length < 1 || length > MAX_COMMENT_LENGTH) {
    errors.content = `评论内容需为 1–${MAX_COMMENT_LENGTH} 个字符。`;
  }
  return { values: { content }, errors, valid: Object.keys(errors).length === 0 };
}

export interface Identified {
  readonly id: string | number;
  readonly [key: string]: unknown;
}

export interface CommentAuthor {
  readonly id?: string | number;
  readonly username?: string;
  readonly nickname?: string;
}

export interface CommentItem extends Identified {
  readonly content: string;
  readonly created_at?: string;
  readonly author?: CommentAuthor;
}

export interface FollowedUser extends Identified {
  readonly username?: string;
  readonly nickname?: string;
}

/** 点赞与收藏返回写入后的最终状态，而不是「本次是否改变」，因此重复调用结果一致。 */
export interface Relation {
  readonly active: boolean;
  readonly count: number;
}

export interface FollowState {
  readonly following: boolean;
}

export interface WatchState {
  readonly progress_ms: number;
  readonly duration_ms: number;
}

export interface CommunityList<TItem> {
  readonly items: readonly TItem[];
  readonly page: number;
  readonly page_size: number;
  readonly total: number;
}

function invalidResponse(): CommunityError {
  return new CommunityError('服务器返回了无法识别的数据，请稍后重试。', { code: 'INVALID_RESPONSE' });
}

function positiveInteger(value: unknown, fallback: number): number {
  const number = Number(value);
  return Number.isInteger(number) && number > 0 ? number : fallback;
}

function checkedID(value: unknown, label: string): string {
  if (!/^\d+$/.test(String(value)) || Number(value) < 1) {
    throw new CommunityError(`${label}无效。`, { code: 'INVALID_PARAMETER' });
  }
  return encodeURIComponent(String(value));
}

// 条目不逐字段校验，与旧实现一致；结构断言交给上层渲染时兜底。
function listFrom<TItem>(data: unknown): CommunityList<TItem> {
  if (!data || typeof data !== 'object') throw invalidResponse();
  const source = data as Record<string, unknown>;
  if (!Array.isArray(source.items) || !Number.isInteger(source.page) ||
      !Number.isInteger(source.page_size) || !Number.isInteger(source.total)) {
    throw invalidResponse();
  }
  return {
    items: source.items as readonly TItem[],
    page: source.page as number,
    page_size: source.page_size as number,
    total: source.total as number,
  };
}

function identifiedFrom(data: unknown): Identified {
  if (!data || typeof data !== 'object') throw invalidResponse();
  const source = data as Record<string, unknown>;
  if (!['string', 'number'].includes(typeof source.id)) throw invalidResponse();
  return data as Identified;
}

function commentFrom(data: unknown): CommentItem {
  identifiedFrom(data);
  if (typeof (data as Record<string, unknown>).content !== 'string') throw invalidResponse();
  return data as CommentItem;
}

function relationFrom(data: unknown): Relation {
  if (!data || typeof data !== 'object') throw invalidResponse();
  const source = data as Record<string, unknown>;
  if (typeof source.active !== 'boolean' || !Number.isInteger(source.count)) throw invalidResponse();
  return { active: source.active, count: source.count as number };
}

function followFrom(data: unknown): FollowState {
  if (!data || typeof data !== 'object') throw invalidResponse();
  const source = data as Record<string, unknown>;
  if (typeof source.following !== 'boolean') throw invalidResponse();
  return { following: source.following };
}

function watchFrom(data: unknown): WatchState {
  if (!data || typeof data !== 'object') throw invalidResponse();
  const source = data as Record<string, unknown>;
  if (!Number.isInteger(source.progress_ms) || !Number.isInteger(source.duration_ms)) {
    throw invalidResponse();
  }
  return { progress_ms: source.progress_ms as number, duration_ms: source.duration_ms as number };
}

export interface CommunityRequester {
  requestWithSession(path: string, options?: RequestOptions): Promise<unknown>;
  requestPublic(path: string, options?: RequestOptions): Promise<unknown>;
}

export interface CommunityClientOptions {
  authClient?: CommunityRequester;
}

export interface PageOptions {
  page?: unknown;
  pageSize?: unknown;
}

export interface WatchReportOptions {
  progressMs?: number;
  durationMs?: number;
  keepalive?: boolean;
}

export interface CommunityClient {
  listComments(id: unknown, options?: PageOptions): Promise<CommunityList<CommentItem>>;
  addComment(id: unknown, input: CommentInput): Promise<CommentItem>;
  deleteComment(id: unknown): Promise<Identified>;
  setLike(id: unknown, active: boolean): Promise<Relation>;
  setFavorite(id: unknown, active: boolean): Promise<Relation>;
  reportWatch(id: unknown, options?: WatchReportOptions): Promise<WatchState>;
  listFavorites(options?: PageOptions): Promise<CommunityList<VideoItem>>;
  listHistory(options?: PageOptions): Promise<CommunityList<VideoItem>>;
  listFollows(options?: PageOptions): Promise<CommunityList<FollowedUser>>;
  setFollow(id: unknown, active: boolean): Promise<FollowState>;
}

export function createCommunityClient({ authClient }: CommunityClientOptions = {}): CommunityClient {
  if (!authClient || typeof authClient.requestWithSession !== 'function'
      || typeof authClient.requestPublic !== 'function') {
    throw new TypeError('createCommunityClient requires an auth client');
  }
  const session: CommunityRequester = authClient;

  const pagePath = (path: string, page: unknown, pageSize: unknown): string => {
    const query = new URLSearchParams({
      page: String(positiveInteger(page, 1)),
      page_size: String(positiveInteger(pageSize, DEFAULT_PAGE_SIZE)),
    });
    return `${path}?${query}`;
  };

  const videoID = (id: unknown): string => checkedID(id, '视频编号');
  const userID = (id: unknown): string => checkedID(id, '用户编号');

  const setRelation = (path: string, active: boolean): Promise<unknown> => session.requestWithSession(path, {
    method: active ? 'PUT' : 'DELETE',
  });

  return {
    async listComments(id: unknown, { page = 1, pageSize = DEFAULT_PAGE_SIZE }: PageOptions = {}): Promise<CommunityList<CommentItem>> {
      return listFrom<CommentItem>(await session.requestPublic(
        pagePath(`/videos/${videoID(id)}/comments`, page, pageSize),
      ));
    },

    async addComment(id: unknown, input: CommentInput): Promise<CommentItem> {
      const validation = validateComment(input);
      if (!validation.valid) {
        throw new CommunityError('请检查评论内容。', {
          code: 'INVALID_PARAMETER',
          fieldErrors: validation.errors,
        });
      }
      return commentFrom(await session.requestWithSession(`/videos/${videoID(id)}/comments`, {
        method: 'POST',
        body: { content: validation.values.content },
      }));
    },

    async deleteComment(id: unknown): Promise<Identified> {
      return identifiedFrom(await session.requestWithSession(`/comments/${checkedID(id, '评论编号')}`, {
        method: 'DELETE',
      }));
    },

    async setLike(id: unknown, active: boolean): Promise<Relation> {
      return relationFrom(await setRelation(`/videos/${videoID(id)}/like`, active));
    },

    async setFavorite(id: unknown, active: boolean): Promise<Relation> {
      return relationFrom(await setRelation(`/videos/${videoID(id)}/favorite`, active));
    },

    async reportWatch(id: unknown, { progressMs, durationMs, keepalive = false }: WatchReportOptions = {}): Promise<WatchState> {
      if (typeof durationMs !== 'number' || !Number.isInteger(durationMs) || durationMs < 0) {
        throw new CommunityError('视频时长无效。', { code: 'INVALID_PARAMETER' });
      }
      if (typeof progressMs !== 'number' || !Number.isInteger(progressMs) || progressMs < 0 ||
          (durationMs > 0 && progressMs > durationMs)) {
        throw new CommunityError('观看进度不能超过视频时长。', { code: 'INVALID_PARAMETER' });
      }
      return watchFrom(await session.requestWithSession(`/videos/${videoID(id)}/watch`, {
        method: 'POST',
        body: { progress_ms: progressMs, duration_ms: durationMs },
        keepalive,
      }));
    },

    async listFavorites({ page = 1, pageSize = DEFAULT_PAGE_SIZE }: PageOptions = {}): Promise<CommunityList<VideoItem>> {
      return listFrom<VideoItem>(await session.requestWithSession(pagePath('/users/me/favorites', page, pageSize)));
    },

    async listHistory({ page = 1, pageSize = DEFAULT_PAGE_SIZE }: PageOptions = {}): Promise<CommunityList<VideoItem>> {
      return listFrom<VideoItem>(await session.requestWithSession(pagePath('/users/me/history', page, pageSize)));
    },

    async listFollows({ page = 1, pageSize = DEFAULT_PAGE_SIZE }: PageOptions = {}): Promise<CommunityList<FollowedUser>> {
      return listFrom<FollowedUser>(await session.requestWithSession(pagePath('/users/me/follows', page, pageSize)));
    },

    async setFollow(id: unknown, active: boolean): Promise<FollowState> {
      return followFrom(await setRelation(`/users/${userID(id)}/follow`, active));
    },
  };
}
