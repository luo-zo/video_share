// 账号与会话：输入校验、短时访问 JWT、HttpOnly refresh Cookie 和资料设置。
import type { FetchLike, RequestOptions, ValidationResult } from './client';
import { AuthError, createRequester, invalidResponse, text } from './client';

export { AuthError };

export interface LoginInput { username?: unknown; password?: unknown }
export interface RegistrationInput extends LoginInput { nickname?: unknown }
export interface LoginValues { username: string; password: string }
export interface RegistrationValues extends LoginValues { nickname: string }

export function validateLogin(input: LoginInput = {}): ValidationResult<LoginValues> {
  const values = { username: text(input.username).trim().toLowerCase(), password: text(input.password) };
  const errors: Record<string, string> = {};
  if (!values.username) errors.username = '请输入用户名。';
  if (!values.password) errors.password = '请输入密码。';
  return { values, errors, valid: Object.keys(errors).length === 0 };
}

export function validateRegistration(input: RegistrationInput = {}): ValidationResult<RegistrationValues> {
  const values: RegistrationValues = { ...validateLogin(input).values, nickname: text(input.nickname).trim() };
  const errors: Record<string, string> = {};
  if (!/^[a-z0-9_]{3,32}$/.test(values.username)) errors.username = '用户名需为 3–32 位字母、数字或下划线。';
  if (Array.from(values.password).length < 8) errors.password = '密码至少需要 8 个字符。';
  else if (new TextEncoder().encode(values.password).length > 72) errors.password = '密码不能超过 72 个 UTF-8 字节，请适当缩短。';
  const nicknameLength = Array.from(values.nickname).length;
  if (nicknameLength < 1 || nicknameLength > 64) errors.nickname = '昵称需为 1–64 个字符。';
  return { values, errors, valid: Object.keys(errors).length === 0 };
}

export interface SessionUser {
  readonly id: string | number;
  readonly username: string;
  readonly nickname: string;
  readonly bio?: string;
  readonly role?: string;
  readonly created_at: string;
}

export interface PublicSession { readonly user: SessionUser; readonly expiresAt: number }
interface InternalSession { token: string; user: SessionUser; expiresAt: number }

function userFrom(data: unknown): SessionUser {
  if (!data || typeof data !== 'object') throw invalidResponse();
  const source = data as Record<string, unknown>;
  const { id, username, nickname, bio, role, created_at: createdAt } = source;
  if (typeof username !== 'string' || typeof nickname !== 'string' || typeof createdAt !== 'string' || !['string', 'number'].includes(typeof id)) throw invalidResponse();
  return Object.freeze({ id: id as string | number, username, nickname, bio: typeof bio === 'string' ? bio : '', role: typeof role === 'string' ? role : undefined, created_at: createdAt });
}

export interface ProfilePatchInput { nickname: unknown; bio: unknown }
export interface ChangePasswordInput { oldPassword: unknown; newPassword: unknown }

export interface AuthClientOptions {
  fetchImpl?: FetchLike;
  now?: () => number;
  timeoutMs?: number;
  /** Production singleton enables Cookie refresh; unit clients keep legacy mode by default. */
  persistent?: boolean;
  sleep?: (milliseconds: number) => Promise<void>;
}

export interface AuthClient {
  register(input: RegistrationInput): Promise<SessionUser>;
  signIn(input: LoginInput): Promise<SessionUser>;
  restore(): Promise<SessionUser | null>;
  refresh(): Promise<SessionUser | null>;
  getProfile(): Promise<SessionUser>;
  updateProfile(input: ProfilePatchInput): Promise<SessionUser>;
  changePassword(input: ChangePasswordInput): Promise<void>;
  requestWithSession(path: string, options?: RequestOptions): Promise<unknown>;
  requestWithOptionalSession(path: string, options?: RequestOptions): Promise<unknown>;
  requestPublic(path: string, options?: RequestOptions): Promise<unknown>;
  getSession(): PublicSession | null;
  signOut(): Promise<void>;
}

function checked<TValues>(validation: ValidationResult<TValues>): TValues {
  if (!validation.valid) throw new AuthError('请检查标记的输入项。', { code: 'INVALID_PARAMETER', fieldErrors: validation.errors });
  return validation.values;
}

function accessPayload(data: unknown): { token: string; expiresIn: number } {
  if (!data || typeof data !== 'object') throw invalidResponse();
  const source = data as Record<string, unknown>;
  if (typeof source.access_token !== 'string' || !source.access_token || source.token_type !== 'Bearer' || typeof source.expires_in !== 'number' || !Number.isFinite(source.expires_in) || source.expires_in <= 0) throw invalidResponse();
  return { token: source.access_token, expiresIn: source.expires_in };
}

function isKnownAuthRejection(error: unknown): error is AuthError {
  if (!(error instanceof AuthError) || error.status !== 401) return false;
  return error.code === 'UNAUTHORIZED' || error.code === 'SESSION_EXPIRED' || error.code === 'HTTP_401';
}

export function createAuthClient({
  fetchImpl = globalThis.fetch as FetchLike,
  now = Date.now,
  timeoutMs = 10_000,
  persistent = false,
  sleep = (milliseconds) => new Promise((resolve) => setTimeout(resolve, milliseconds)),
}: AuthClientOptions = {}): AuthClient {
  const request = createRequester({ fetchImpl, timeoutMs });
  let session: InternalSession | null = null;
  let generation = 0;
  let refreshPromise: Promise<SessionUser | null> | null = null;

  const clearSession = (): void => { session = null; generation += 1; };
  const activeSession = (): InternalSession | null => { if (session && now() >= session.expiresAt) clearSession(); return session; };
  const ensureCSRF = async (): Promise<void> => { if (persistent) await request('/auth/csrf', { withCredentials: true }); };

  const rotate = async (): Promise<SessionUser | null> => {
    if (!persistent) return null;
    const refreshGeneration = generation;
    await ensureCSRF();
    for (let attempt = 0; attempt <= 2; attempt += 1) {
      try {
        const data = accessPayload(await request('/auth/refresh', { method: 'POST', body: {}, withCredentials: true, csrf: true }));
        const startedAt = now();
        const user = userFrom(await request('/users/me', { token: data.token }));
        if (refreshGeneration !== generation) {
          throw new AuthError('这次刷新已被退出操作取消。', { code: 'REQUEST_CANCELLED' });
        }
        session = { token: data.token, user, expiresAt: startedAt + data.expiresIn * 1000 };
        return user;
      } catch (error) {
        if (!(error instanceof AuthError) || error.code !== 'REFRESH_CONFLICT' || attempt === 2) throw error;
        await sleep(50 * (attempt + 1));
      }
    }
    return null;
  };

  const refreshAccess = async (): Promise<SessionUser | null> => {
    if (refreshPromise) return refreshPromise;
    const run = async (): Promise<SessionUser | null> => {
      const locks = typeof navigator !== 'undefined' ? (navigator as Navigator & { locks?: { request<T>(name: string, callback: () => Promise<T>): Promise<T> } }).locks : undefined;
      if (locks) return locks.request('video-share-refresh', rotate);
      return rotate();
    };
    refreshPromise = run().finally(() => { refreshPromise = null; });
    return refreshPromise;
  };

  const requestSession = async (path: string, options: RequestOptions = {}, allowRefresh = true): Promise<unknown> => {
    const current = activeSession();
    if (!current) throw new AuthError('请先登录账号。', { code: 'SESSION_EXPIRED' });
    const attempt = generation;
    try {
      return await request(path, { ...options, token: current.token });
    } catch (error) {
      if (isKnownAuthRejection(error) && allowRefresh && persistent && attempt === generation) {
        try {
          await refreshAccess();
          const refreshed = activeSession();
          if (!refreshed) throw new AuthError('登录状态已失效，请重新登录。', { code: 'SESSION_EXPIRED', status: 401 });
          return await requestSession(path, options, false);
        } catch (refreshError) {
          if (refreshError instanceof AuthError && refreshError.status >= 400 && refreshError.status < 500) clearSession();
          throw refreshError;
        }
      }
      if (error instanceof AuthError && error.status === 401 && !persistent && attempt === generation) clearSession();
      throw error;
    }
  };

  return {
    async register(input) {
      const values = checked(validateRegistration(input));
      return userFrom(await request('/auth/register', { method: 'POST', body: values }));
    },

    async signIn(input) {
      const values = checked(validateLogin(input));
      clearSession();
      const attempt = generation;
      if (persistent) await ensureCSRF();
      const startedAt = now();
      const data = accessPayload(await request('/auth/login', { method: 'POST', body: values, withCredentials: persistent, csrf: persistent }));
      const user = userFrom(await request('/users/me', { token: data.token }));
      if (attempt !== generation) throw new AuthError('这次登录已取消，请重新登录。', { code: 'REQUEST_CANCELLED' });
      session = { token: data.token, user, expiresAt: startedAt + data.expiresIn * 1000 };
      return user;
    },

    async restore() {
      if (!persistent) return activeSession()?.user ?? null;
      try { return await refreshAccess(); } catch (error) {
        if (error instanceof AuthError && error.status >= 400 && error.status < 500) clearSession();
        throw error;
      }
    },

    async refresh() { return refreshAccess(); },

    async getProfile() { return userFrom(await requestSession('/users/me')); },

    async updateProfile(input) {
      const nickname = text(input.nickname).trim();
      const bio = text(input.bio).trim();
      if (Array.from(nickname).length < 1 || Array.from(nickname).length > 64 || Array.from(bio).length > 200) throw new AuthError('资料格式不符合要求。', { code: 'INVALID_PARAMETER' });
      const user = userFrom(await requestSession('/users/me', { method: 'PATCH', body: { nickname, bio }, csrf: persistent, withCredentials: persistent }));
      if (session) session = { ...session, user };
      return user;
    },

    async changePassword(input) {
      const oldPassword = text(input.oldPassword);
      const newPassword = text(input.newPassword);
      if (Array.from(newPassword).length < 8 || new TextEncoder().encode(newPassword).length > 72) throw new AuthError('密码须为 8 个字符以上且不超过 72 字节。', { code: 'INVALID_PARAMETER' });
      await requestSession('/users/me/change-password', { method: 'POST', body: { old_password: oldPassword, new_password: newPassword }, csrf: persistent, withCredentials: persistent });
      clearSession();
    },

    async requestWithSession(path, options = {}) { return requestSession(path, options); },

    async requestWithOptionalSession(path, options = {}) {
      if (!activeSession()) return request(path, options);
      try { return await requestSession(path, options); } catch (error) {
        if (error instanceof AuthError && error.status === 401 && (options.method ?? 'GET') === 'GET') { clearSession(); return request(path, options); }
        throw error;
      }
    },

    async requestPublic(path, options = {}) { return request(path, options); },

    getSession() { const current = activeSession(); return current ? { user: current.user, expiresAt: current.expiresAt } : null; },

    signOut(): Promise<void> {
      const current = session;
      clearSession();
      if (!persistent) return Promise.resolve();
      // Clear local auth immediately, but do not hide a failed server revoke.
      // The refresh-only backend path also works when the access JWT expired.
      return ensureCSRF().then(() => request('/auth/logout', {
        method: 'POST',
        body: {},
        token: current?.token,
        withCredentials: true,
        csrf: true,
      })).then(() => undefined);
    },
  };
}
