// API 传输层：统一请求信封、错误映射与超时处理。
// 这里只负责「把 HTTP 结果变成数据或 AuthError」，不了解任何业务语义，
// 方便 video.ts / community.ts 复用，也方便单测注入假的 fetch。
export const API_ROOT = '/api/v1';

export const text = (value: unknown): string => (typeof value === 'string' ? value : '');

export interface FieldErrors {
  readonly [field: string]: string;
}

/** 各 API 模块共用的校验结果形状：校验后的值与逐字段错误一一对应。 */
export interface ValidationResult<TValues> {
  values: TValues;
  errors: FieldErrors;
  valid: boolean;
}

export interface ApiErrorInit {
  code?: string;
  status?: number;
  requestId?: string;
  fieldErrors?: FieldErrors;
}

export class AuthError extends Error {
  readonly code: string;

  readonly status: number;

  readonly requestId: string;

  readonly fieldErrors: FieldErrors;

  constructor(
    message: string,
    { code = 'UNKNOWN_ERROR', status = 0, requestId = '', fieldErrors = {} }: ApiErrorInit = {},
  ) {
    super(message);
    this.name = 'AuthError';
    this.code = code;
    this.status = status;
    this.requestId = requestId;
    this.fieldErrors = fieldErrors;
  }
}

interface ErrorDetail {
  readonly code?: unknown;
  readonly request_id?: unknown;
}

function errorDetail(payload: unknown): ErrorDetail {
  if (!payload || typeof payload !== 'object') return {};
  const { error } = payload as { error?: unknown };
  if (!error || typeof error !== 'object') return {};
  const { code, request_id: requestId } = error as { code?: unknown; request_id?: unknown };
  return { code, request_id: requestId };
}

// 后端错误码 → 面向用户的中文文案。未覆盖的状态码走兜底文案，避免把内部信息透给用户。
export function responseError(status: number, detail: ErrorDetail = {}): AuthError {
  const code = typeof detail.code === 'string' ? detail.code : `HTTP_${status}`;
  let message = '请求未能完成，请稍后再试。';
  if (status === 429) message = '操作太频繁，请稍等片刻再试。';
  else if (code === 'INVALID_CREDENTIALS') message = '用户名或密码不正确，请重试。';
  else if (code === 'USER_ALREADY_EXISTS') message = '这个用户名已被使用，请换一个试试。';
  else if (status === 401) message = '登录状态已失效，请重新登录。';
  else if (code === 'FORBIDDEN') message = '你没有权限执行这个操作。';
  else if (code === 'NOT_FOUND') message = '请求的视频不存在或已被删除。';
  else if (code === 'UPLOAD_INCOMPLETE') message = '视频文件尚未上传完成，请稍后重试。';
  else if (code === 'UPLOAD_MISMATCH') message = '上传文件与投稿信息不一致，请重新投稿。';
  else if (code === 'VIDEO_STATE_CONFLICT') message = '当前视频状态不能执行这个操作。';
  else if (code === 'REFRESH_CONFLICT') message = '登录刷新正在其他标签页进行，请稍后重试。';
  else if (code === 'SESSION_REUSED' || code === 'SESSION_EXPIRED') message = '登录状态已失效，请重新登录。';
  else if (code === 'CSRF_FAILED') message = '请求安全校验失败，请刷新页面后重试。';
  else if (code === 'INVALID_PARAMETER') message = '输入信息不符合要求，请检查后重试。';
  else if (status >= 500) message = '服务暂时不可用，请稍后重试。';
  return new AuthError(message, {
    code,
    status,
    requestId: typeof detail.request_id === 'string' ? detail.request_id : '',
  });
}

export const invalidResponse = (): AuthError => new AuthError('服务器返回了无法识别的数据，请稍后重试。', {
  code: 'INVALID_RESPONSE',
});

// 只需要 Response 的一小部分能力，测试里传一个普通对象就能替代 fetch。
export interface MinimalResponse {
  readonly ok: boolean;
  readonly status: number;
  json(): Promise<unknown>;
}

export type FetchLike = (url: string, init: RequestInit) => Promise<MinimalResponse>;

export interface RequestOptions {
  method?: string;
  body?: unknown;
  token?: string;
  keepalive?: boolean;
  signal?: AbortSignal;
  withCredentials?: boolean;
  csrf?: boolean;
  headers?: Readonly<Record<string, string>>;
}

export type Requester = (path: string, options?: RequestOptions) => Promise<unknown>;

export interface RequesterOptions {
  fetchImpl?: FetchLike;
  timeoutMs?: number;
}

export function createRequester({
  fetchImpl = globalThis.fetch as FetchLike,
  timeoutMs = 10_000,
}: RequesterOptions = {}): Requester {
  return async function request(
    path: string,
    { method = 'GET', body, token, keepalive = false, signal, withCredentials = false, csrf = false, headers: extraHeaders }: RequestOptions = {},
  ): Promise<unknown> {
    const controller = new AbortController();
    let timedOut = false;
    const abortFromCaller = (): void => controller.abort();
    if (signal?.aborted) controller.abort();
    else signal?.addEventListener('abort', abortFromCaller, { once: true });
    const timer = setTimeout(() => {
      timedOut = true;
      controller.abort();
    }, timeoutMs);
    const headers: Record<string, string> = { Accept: 'application/json' };
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    if (token) headers.Authorization = `Bearer ${token}`;
    if (csrf) {
      const cookie = typeof document === 'undefined' ? '' : document.cookie.split('; ').find((item) => item.startsWith('video_share_csrf='));
      const value = cookie?.slice('video_share_csrf='.length) ?? '';
      if (value) headers['X-CSRF-Token'] = decodeURIComponent(value);
    }
    if (extraHeaders) Object.assign(headers, extraHeaders);
    try {
      const response = await fetchImpl(`${API_ROOT}${path}`, {
        method,
        headers,
        credentials: withCredentials ? 'include' : 'omit',
        cache: 'no-store',
        signal: controller.signal,
        keepalive: Boolean(keepalive),
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      });
      if (response.status === 204) return undefined;
      let payload: unknown;
      try {
        payload = await response.json();
      } catch (error) {
        // 超时导致的 abort 不能被误报成「数据格式错误」。
        if (controller.signal.aborted) throw error;
        if (!response.ok) throw responseError(response.status);
        throw invalidResponse();
      }
      if (!response.ok) throw responseError(response.status, errorDetail(payload));
      if (!payload || typeof payload !== 'object' || !('data' in payload)) throw invalidResponse();
      return (payload as { data: unknown }).data;
    } catch (error) {
      if (error instanceof AuthError) throw error;
      if (controller.signal.aborted) {
        if (!timedOut) {
          throw new AuthError('请求已取消。', { code: 'REQUEST_CANCELLED' });
        }
        throw new AuthError('请求超时，请检查连接后重试。', { code: 'REQUEST_TIMEOUT' });
      }
      throw new AuthError('无法连接账号服务，请检查网络或稍后重试。', { code: 'NETWORK_ERROR' });
    } finally {
      clearTimeout(timer);
      signal?.removeEventListener('abort', abortFromCaller);
    }
  };
}
