// 投稿与视频读取：校验规则、分页约定、MinIO 直传流程。
// 规则与旧 video.js 一一对应，保证既有断言可以逐条迁移。
import { AuthError } from './auth';
import type { FieldErrors, MinimalResponse, RequestOptions, ValidationResult } from './client';
import { text } from './client';

const MAX_VIDEO_BYTES = 500 * 1024 * 1024;
const DEFAULT_PAGE_SIZE = 12;
const PROCESSING_STATES = ['ready', 'failed', 'deleted'];

export interface VideoErrorInit {
  code?: string;
  status?: number;
  fieldErrors?: FieldErrors;
}

export class VideoError extends Error {
  readonly code: string;

  readonly status: number;

  readonly fieldErrors: FieldErrors;

  constructor(
    message: string,
    { code = 'VIDEO_ERROR', status = 0, fieldErrors = {} }: VideoErrorInit = {},
  ) {
    super(message);
    this.name = 'VideoError';
    this.code = code;
    this.status = status;
    this.fieldErrors = fieldErrors;
  }
}

/** 只依赖 name/type/size，浏览器 File 与测试里的普通对象都满足。 */
export interface UploadFile {
  readonly name: string;
  readonly type: string;
  readonly size: number;
}

export interface VideoSubmissionInput {
  title?: unknown;
  description?: unknown;
  file?: unknown;
  categoryId?: unknown;
  tags?: unknown;
}

export interface VideoSubmissionValues {
  title: string;
  description: string;
  file: UploadFile | null;
}

export function validateVideoSubmission(
  input: VideoSubmissionInput = {},
): ValidationResult<VideoSubmissionValues> {
  const source = (input ?? {}) as Record<string, unknown>;
  const file = (source.file ?? null) as UploadFile | null;
  const values: VideoSubmissionValues = {
    title: text(source.title).trim(),
    description: text(source.description).trim(),
    file,
  };
  const errors: Record<string, string> = {};
  const titleLength = Array.from(values.title).length;
  if (titleLength < 1 || titleLength > 100) errors.title = '标题需为 1–100 个字符。';
  if (Array.from(values.description).length > 2000) errors.description = '简介不能超过 2000 个字符。';
  if (!file || typeof file !== 'object') {
    errors.file = '请选择一个 MP4 视频。';
  } else {
    const fileName = text(file.name);
    const contentType = text(file.type).toLowerCase();
    const isMP4 = contentType === 'video/mp4' || (!contentType && /\.mp4$/i.test(fileName));
    if (!isMP4) errors.file = '仅支持 MP4 格式的视频。';
    else if (!Number.isFinite(file.size) || file.size <= 0) errors.file = '所选视频为空，请重新选择。';
    else if (file.size > MAX_VIDEO_BYTES) errors.file = '视频不能超过 500 MiB。';
  }
  return { values, errors, valid: Object.keys(errors).length === 0 };
}

export interface VideoPatchInput {
  title?: unknown;
  description?: unknown;
  visibility?: unknown;
  categoryId?: unknown;
  tags?: unknown;
}

export interface VideoPatchValues {
  title?: string;
  description?: string;
  visibility?: string;
  category_id?: number;
  tags?: string[];
}

// 作者的部分更新：只接受显式提供的字段，未提供的字段保持原值。
export function validateVideoPatch(
  input: VideoPatchInput = {},
): ValidationResult<VideoPatchValues> {
  const source = (input ?? {}) as Record<string, unknown>;
  const values: VideoPatchValues = {};
  const errors: Record<string, string> = {};
  if (source.title !== undefined) {
    const title = text(source.title).trim();
    if (Array.from(title).length < 1 || Array.from(title).length > 100) {
      errors.title = '标题需为 1–100 个字符。';
    } else {
      values.title = title;
    }
  }
  if (source.description !== undefined) {
    const description = text(source.description).trim();
    if (Array.from(description).length > 2000) errors.description = '简介不能超过 2000 个字符。';
    else values.description = description;
  }
  if (source.visibility !== undefined) {
    const visibility = text(source.visibility);
    if (!['public', 'private'].includes(visibility)) errors.visibility = '可见性只能是 public 或 private。';
    else values.visibility = visibility;
  }
  if (source.categoryId !== undefined) {
    const categoryID = Number(source.categoryId);
    if (!Number.isInteger(categoryID) || categoryID < 1) errors.category_id = '请选择有效分区。';
    else values.category_id = categoryID;
  }
  if (source.tags !== undefined) {
    if (!Array.isArray(source.tags) || source.tags.length > 5 || source.tags.some((tag) => Array.from(text(tag).trim()).length < 1 || Array.from(text(tag).trim()).length > 20)) {
      errors.tags = '标签最多 5 个，每个 1–20 个字符。';
    } else {
      values.tags = source.tags.map((tag) => text(tag).trim());
    }
  }
  if (Object.keys(values).length === 0 && Object.keys(errors).length === 0) {
    errors.title = '请至少提供一个要修改的字段。';
  }
  return { values, errors, valid: Object.keys(errors).length === 0 };
}

export interface VideoAuthor {
  readonly id?: string | number;
  readonly username?: string;
  readonly nickname?: string;
}

export interface VideoStats {
  readonly view_count?: number;
  readonly like_count?: number;
  readonly favorite_count?: number;
  readonly comment_count?: number;
  readonly [key: string]: unknown;
}

export interface VideoViewerState {
  readonly liked?: boolean;
  readonly favorited?: boolean;
  readonly following_author?: boolean;
}

// 只校验 id，其余字段按后端契约透传；未列出的字段通过索引签名保留。
export interface VideoItem {
  readonly id: string | number;
  readonly status?: string;
  readonly title?: string;
  readonly description?: string;
  readonly visibility?: string;
  readonly play_url?: string;
  readonly play_type?: string;
  readonly cover_url?: string;
  readonly upload_url?: string;
  readonly created_at?: string;
  readonly author?: VideoAuthor;
  readonly stats?: VideoStats;
  readonly viewer_state?: VideoViewerState;
  readonly [key: string]: unknown;
}

export interface VideoList {
  readonly items: readonly VideoItem[];
  readonly page: number;
  readonly page_size: number;
  readonly total: number;
}

function invalidVideo(): VideoError {
  return new VideoError('服务器返回了无法识别的视频数据。', { code: 'INVALID_RESPONSE' });
}

function videoFrom(data: unknown): VideoItem {
  if (!data || typeof data !== 'object') throw invalidVideo();
  const source = data as Record<string, unknown>;
  if (!['string', 'number'].includes(typeof source.id)) throw invalidVideo();
  return data as VideoItem;
}

function listFrom(data: unknown): VideoList {
  if (!data || typeof data !== 'object') {
    throw new VideoError('服务器返回了无法识别的视频列表。', { code: 'INVALID_RESPONSE' });
  }
  const source = data as Record<string, unknown>;
  if (!Array.isArray(source.items) || !Number.isInteger(source.page) ||
      !Number.isInteger(source.page_size) || !Number.isInteger(source.total)) {
    throw new VideoError('服务器返回了无法识别的视频列表。', { code: 'INVALID_RESPONSE' });
  }
  const items = source.items as unknown[];
  items.forEach(videoFrom);
  return {
    items: items as readonly VideoItem[],
    page: source.page as number,
    page_size: source.page_size as number,
    total: source.total as number,
  };
}

function positiveInteger(value: unknown, fallback: number): number {
  const number = Number(value);
  return Number.isInteger(number) && number > 0 ? number : fallback;
}

/** 直传只需要 method/headers/credentials/body，避免把 RequestInit 的 BodyInit 约束外泄。 */
export interface UploadRequest {
  method: string;
  headers: Record<string, string>;
  credentials: RequestCredentials;
  body: UploadFile;
  signal?: AbortSignal;
}

export type UploadFetch = (url: string, init: UploadRequest) => Promise<MinimalResponse>;

export type UploadStep = 'creating' | 'uploading' | 'completing' | 'processing' | 'complete';

/** 结构化校验只需要这几个方法，便于测试注入替身。 */
export interface SessionRequester {
  requestWithSession(path: string, options?: RequestOptions): Promise<unknown>;
  requestPublic(path: string, options?: RequestOptions): Promise<unknown>;
  requestWithOptionalSession(path: string, options?: RequestOptions): Promise<unknown>;
}

export interface VideoClientOptions {
  authClient?: SessionRequester;
  fetchImpl?: UploadFetch;
  sleep?: (milliseconds: number, signal?: AbortSignal) => Promise<void>;
  now?: () => number;
}

export interface ListVideosOptions {
  query?: unknown;
  sort?: unknown;
  page?: unknown;
  pageSize?: unknown;
  signal?: AbortSignal;
  categoryId?: unknown;
  tags?: readonly unknown[];
}

export interface PageOptions {
  page?: unknown;
  pageSize?: unknown;
  signal?: AbortSignal;
}

export interface WaitOptions {
  intervalMs?: number;
  timeoutMs?: number;
  onUpdate?: (item: VideoItem) => void;
  signal?: AbortSignal;
}

export interface UploadOptions {
  onStep?: (step: UploadStep) => void;
  signal?: AbortSignal;
}

export interface VideoClient {
  listVideos(options?: ListVideosOptions): Promise<VideoList>;
  getVideo(id: unknown, options?: { signal?: AbortSignal }): Promise<VideoItem>;
  listMyVideos(options?: PageOptions): Promise<VideoList>;
  getMyVideo(id: unknown, options?: { signal?: AbortSignal }): Promise<VideoItem>;
  updateVideo(id: unknown, input: VideoPatchInput): Promise<VideoItem>;
  deleteVideo(id: unknown): Promise<VideoItem>;
  waitUntilProcessed(id: unknown, options?: WaitOptions): Promise<VideoItem>;
  uploadVideo(input: VideoSubmissionInput, options?: UploadOptions): Promise<VideoItem>;
  listFollowing(options?: ListVideosOptions): Promise<VideoList>;
  listRelated(id: unknown, options?: { signal?: AbortSignal }): Promise<VideoList>;
}

export function createVideoClient({
  authClient,
  fetchImpl = globalThis.fetch as unknown as UploadFetch,
  sleep,
  now = Date.now,
}: VideoClientOptions = {}): VideoClient {
  if (!authClient || typeof authClient.requestWithSession !== 'function'
      || typeof authClient.requestPublic !== 'function'
      || typeof authClient.requestWithOptionalSession !== 'function') {
    throw new TypeError('createVideoClient requires an auth client');
  }
  const session: SessionRequester = authClient;
  const pause = sleep ?? ((milliseconds: number, signal?: AbortSignal) => new Promise<void>((resolve, reject) => {
    if (signal?.aborted) {
      reject(new VideoError('处理状态查询已取消。', { code: 'REQUEST_CANCELLED' }));
      return;
    }
    const finish = (): void => {
      signal?.removeEventListener('abort', cancel);
      resolve();
    };
    const timer = setTimeout(finish, milliseconds);
    const cancel = (): void => {
      clearTimeout(timer);
      signal?.removeEventListener('abort', cancel);
      reject(new VideoError('处理状态查询已取消。', { code: 'REQUEST_CANCELLED' }));
    };
    signal?.addEventListener('abort', cancel, { once: true });
  }));

  const pagePath = (path: string, page: unknown, pageSize: unknown): string => {
    const query = new URLSearchParams({
      page: String(positiveInteger(page, 1)),
      page_size: String(positiveInteger(pageSize, DEFAULT_PAGE_SIZE)),
    });
    return `${path}?${query}`;
  };

  const discoveryPath = ({ query, sort, page, pageSize, categoryId, tags }: ListVideosOptions = {}, base = '/videos'): string => {
    const params = new URLSearchParams();
    const keyword = text(query).trim();
    if (keyword) params.set('q', keyword);
    const order = text(sort);
    if (order) params.set('sort', order);
    params.set('page', String(positiveInteger(page, 1)));
    params.set('page_size', String(positiveInteger(pageSize, DEFAULT_PAGE_SIZE)));
    const category = positiveInteger(categoryId, 0);
    if (category > 0) params.set('category_id', String(category));
    for (const tag of tags ?? []) {
      const value = text(tag).trim();
      if (value) params.append('tag', value);
    }
    return `${base}?${params}`;
  };

  const checkedID = (id: unknown): string => {
    if (!/^\d+$/.test(String(id)) || Number(id) < 1) {
      throw new VideoError('视频编号无效。', { code: 'INVALID_PARAMETER' });
    }
    return encodeURIComponent(String(id));
  };

  const getMyVideo = async (id: unknown, { signal }: { signal?: AbortSignal } = {}): Promise<VideoItem> => videoFrom(
    await session.requestWithSession(`/users/me/videos/${checkedID(id)}`, { signal }),
  );

  return {
    async listVideos(options: ListVideosOptions = {}): Promise<VideoList> {
      return listFrom(await session.requestPublic(discoveryPath(options), { signal: options.signal }));
    },

    async listFollowing(options: ListVideosOptions = {}): Promise<VideoList> {
      return listFrom(await session.requestWithSession(discoveryPath(options, '/feed/following'), { signal: options.signal }));
    },

    async listRelated(id: unknown, { signal }: { signal?: AbortSignal } = {}): Promise<VideoList> {
      return listFrom(await session.requestPublic(`/videos/${checkedID(id)}/related`, { signal }));
    },

    async getVideo(id: unknown, { signal }: { signal?: AbortSignal } = {}): Promise<VideoItem> {
      return videoFrom(await session.requestWithOptionalSession(`/videos/${checkedID(id)}`, { signal }));
    },

    async listMyVideos({ page = 1, pageSize = DEFAULT_PAGE_SIZE, signal }: PageOptions = {}): Promise<VideoList> {
      return listFrom(await session.requestWithSession(pagePath('/users/me/videos', page, pageSize), { signal }));
    },

    getMyVideo,

    async updateVideo(id: unknown, input: VideoPatchInput): Promise<VideoItem> {
      const validation = validateVideoPatch(input);
      if (!validation.valid) {
        throw new VideoError('请检查要修改的内容。', {
          code: 'INVALID_PARAMETER',
          fieldErrors: validation.errors,
        });
      }
      return videoFrom(await session.requestWithSession(`/users/me/videos/${checkedID(id)}`, {
        method: 'PATCH',
        body: validation.values,
      }));
    },

    async deleteVideo(id: unknown): Promise<VideoItem> {
      return videoFrom(await session.requestWithSession(`/users/me/videos/${checkedID(id)}`, {
        method: 'DELETE',
      }));
    },

    async waitUntilProcessed(id: unknown, {
      intervalMs = 2500,
      timeoutMs = 30 * 60 * 1000,
      onUpdate = () => {},
      signal,
    }: WaitOptions = {}): Promise<VideoItem> {
      const deadline = now() + timeoutMs;
      for (;;) {
        if (signal?.aborted) {
          throw new VideoError('处理状态查询已取消。', { code: 'REQUEST_CANCELLED' });
        }
        const item = await getMyVideo(id, { signal });
        onUpdate(item);
        const status = item.status;
        if (typeof status === 'string' && PROCESSING_STATES.includes(status)) return item;
        if (now() >= deadline) {
          throw new VideoError('视频仍在处理中，请稍后到“我的投稿”查看。', { code: 'PROCESSING_TIMEOUT' });
        }
        await pause(intervalMs, signal);
      }
    },

    async uploadVideo(input: VideoSubmissionInput, { onStep = () => {}, signal }: UploadOptions = {}): Promise<VideoItem> {
      const validation = validateVideoSubmission(input);
      if (!validation.valid) {
        throw new VideoError('请检查投稿信息。', {
          code: 'INVALID_PARAMETER',
          fieldErrors: validation.errors,
        });
      }
      const { title, description, file } = validation.values;
      if (!file) {
        throw new VideoError('请检查投稿信息。', {
          code: 'INVALID_PARAMETER',
          fieldErrors: validation.errors,
        });
      }
      const contentType = text(file.type).toLowerCase() || 'video/mp4';
      const categoryID = Number((input as Record<string, unknown>).categoryId);
      const rawTags = (input as Record<string, unknown>).tags;
      const tags = Array.isArray(rawTags)
        ? rawTags.map((tag) => text(tag).trim()).filter(Boolean)
        : text(rawTags).split(',').map((tag) => tag.trim()).filter(Boolean);

      onStep('creating');
      const created = videoFrom(await session.requestWithSession('/videos', {
        method: 'POST',
        body: {
          title,
          description,
          file_name: file.name,
          content_type: contentType,
          file_size: file.size,
          ...(Number.isInteger(categoryID) && categoryID > 0 ? { category_id: categoryID } : {}),
          ...(tags.length > 0 ? { tags } : {}),
        },
        signal,
      }));
      const uploadUrl = created.upload_url;
      if (typeof uploadUrl !== 'string' || !uploadUrl) {
        throw new VideoError('上传地址无效，请重新创建投稿。', { code: 'INVALID_RESPONSE' });
      }

      onStep('uploading');
      let uploadResponse: MinimalResponse;
      try {
        uploadResponse = await fetchImpl(uploadUrl, {
          method: 'PUT',
          headers: { 'Content-Type': contentType },
          credentials: 'omit',
          body: file,
          ...(signal ? { signal } : {}),
        });
      } catch {
        throw new VideoError('视频直传失败，请检查网络后重试。', { code: 'UPLOAD_NETWORK_ERROR' });
      }
      if (!uploadResponse.ok) {
        throw new VideoError('视频直传失败，请重新投稿。', {
          code: 'UPLOAD_FAILED',
          status: uploadResponse.status ?? 0,
        });
      }

      onStep('completing');
      const completed = videoFrom(await session.requestWithSession(
        `/videos/${encodeURIComponent(String(created.id))}/complete`,
        { method: 'POST', body: {}, signal },
      ));
      onStep(completed.status === 'processing' ? 'processing' : 'complete');
      return completed;
    },
  };
}

export function isSessionError(error: unknown): boolean {
  return error instanceof AuthError && (error.status === 401 || error.code === 'SESSION_EXPIRED');
}
