// 账号与会话：输入校验、登录/注册、带会话请求。
// 传输细节在 ./client，这里保持与旧 auth.js 完全一致的对外语义，
// 以便既有断言可以逐条迁移过来。
import type { FetchLike, RequestOptions, ValidationResult } from './client';
import { AuthError, createRequester, invalidResponse, text } from './client';

export { AuthError };

export interface LoginInput {
  username?: unknown;
  password?: unknown;
}

export interface RegistrationInput extends LoginInput {
  nickname?: unknown;
}

export interface LoginValues {
  username: string;
  password: string;
}

export interface RegistrationValues extends LoginValues {
  nickname: string;
}

export function validateLogin(input: LoginInput = {}): ValidationResult<LoginValues> {
  const values: LoginValues = {
    username: text(input.username).trim().toLowerCase(),
    password: text(input.password),
  };
  const errors: Record<string, string> = {};
  if (!values.username) errors.username = '请输入用户名。';
  if (!values.password) errors.password = '请输入密码。';
  return { values, errors, valid: Object.keys(errors).length === 0 };
}

export function validateRegistration(
  input: RegistrationInput = {},
): ValidationResult<RegistrationValues> {
  const values: RegistrationValues = { ...validateLogin(input).values, nickname: text(input.nickname).trim() };
  const errors: Record<string, string> = {};
  if (!/^[a-z0-9_]{3,32}$/.test(values.username)) {
    errors.username = '用户名需为 3–32 位字母、数字或下划线。';
  }
  if (Array.from(values.password).length < 8) {
    errors.password = '密码至少需要 8 个字符。';
  } else if (new TextEncoder().encode(values.password).length > 72) {
    errors.password = '密码不能超过 72 个 UTF-8 字节，请适当缩短。';
  }
  const nicknameLength = Array.from(values.nickname).length;
  if (nicknameLength < 1 || nicknameLength > 64) {
    errors.nickname = '昵称需为 1–64 个字符。';
  }
  return { values, errors, valid: Object.keys(errors).length === 0 };
}

export interface SessionUser {
  readonly id: string | number;
  readonly username: string;
  readonly nickname: string;
  readonly created_at: string;
}

export interface PublicSession {
  readonly user: SessionUser;
  readonly expiresAt: number;
}

interface InternalSession {
  token: string;
  user: SessionUser;
  expiresAt: number;
}

function userFrom(data: unknown): SessionUser {
  if (!data || typeof data !== 'object') throw invalidResponse();
  const source = data as Record<string, unknown>;
  const { id, username, nickname } = source;
  const createdAt = source.created_at;
  if (typeof username !== 'string' || typeof nickname !== 'string' ||
      typeof createdAt !== 'string' || !['string', 'number'].includes(typeof id)) {
    throw invalidResponse();
  }
  return Object.freeze({ id: id as string | number, username, nickname, created_at: createdAt });
}

export interface AuthClientOptions {
  fetchImpl?: FetchLike;
  now?: () => number;
  timeoutMs?: number;
}

export interface AuthClient {
  register(input: RegistrationInput): Promise<SessionUser>;
  signIn(input: LoginInput): Promise<SessionUser>;
  getProfile(): Promise<SessionUser>;
  requestWithSession(path: string, options?: RequestOptions): Promise<unknown>;
  requestWithOptionalSession(path: string, options?: RequestOptions): Promise<unknown>;
  requestPublic(path: string, options?: RequestOptions): Promise<unknown>;
  getSession(): PublicSession | null;
  signOut(): void;
}

function checked<TValues>(validation: ValidationResult<TValues>): TValues {
  if (!validation.valid) {
    throw new AuthError('请检查标记的输入项。', {
      code: 'INVALID_PARAMETER',
      fieldErrors: validation.errors,
    });
  }
  return validation.values;
}

/** 令牌只留在本实例内存里，且必须等 /users/me 成功后才提交。 */
export function createAuthClient({
  fetchImpl = globalThis.fetch as FetchLike,
  now = Date.now,
  timeoutMs = 10_000,
}: AuthClientOptions = {}): AuthClient {
  const request = createRequester({ fetchImpl, timeoutMs });
  let session: InternalSession | null = null;
  let generation = 0;

  const clearSession = (): void => {
    session = null;
    generation += 1;
  };

  const activeSession = (): InternalSession | null => {
    if (session && now() >= session.expiresAt) clearSession();
    return session;
  };

  return {
    async register(input: RegistrationInput): Promise<SessionUser> {
      const values = checked(validateRegistration(input));
      return userFrom(await request('/auth/register', { method: 'POST', body: values }));
    },

    async signIn(input: LoginInput): Promise<SessionUser> {
      const values = checked(validateLogin(input));
      clearSession();
      const attempt = generation;
      const startedAt = now();
      const data = await request('/auth/login', { method: 'POST', body: values });
      if (!data || typeof data !== 'object') throw invalidResponse();
      const { access_token: accessToken, token_type: tokenType, expires_in: expiresIn } = data as Record<string, unknown>;
      if (typeof accessToken !== 'string' || !accessToken ||
          typeof tokenType !== 'string' || tokenType.toLowerCase() !== 'bearer' ||
          typeof expiresIn !== 'number' || !Number.isFinite(expiresIn) || expiresIn <= 0) {
        throw invalidResponse();
      }
      const expiresAt = startedAt + expiresIn * 1000;
      const user = userFrom(await request('/users/me', { token: accessToken }));
      if (attempt !== generation) {
        throw new AuthError('这次登录已取消，请重新登录。', { code: 'REQUEST_CANCELLED' });
      }
      if (now() >= expiresAt) {
        throw new AuthError('登录状态已失效，请重新登录。', { code: 'SESSION_EXPIRED' });
      }
      session = { token: accessToken, user, expiresAt };
      return user;
    },

    async getProfile(): Promise<SessionUser> {
      const current = activeSession();
      if (!current) throw new AuthError('请先登录账号。', { code: 'SESSION_EXPIRED' });
      const attempt = generation;
      try {
        const user = userFrom(await request('/users/me', { token: current.token }));
        if (attempt !== generation || !activeSession()) {
          throw new AuthError('登录状态已失效，请重新登录。', { code: 'SESSION_EXPIRED' });
        }
        session = { ...current, user };
        return user;
      } catch (error) {
        if (error instanceof AuthError && error.status === 401 && attempt === generation) clearSession();
        throw error;
      }
    },

    async requestWithSession(path: string, options: RequestOptions = {}): Promise<unknown> {
      const current = activeSession();
      if (!current) throw new AuthError('请先登录账号。', { code: 'SESSION_EXPIRED' });
      const attempt = generation;
      try {
        const data = await request(path, { ...options, token: current.token });
        if (attempt !== generation || !activeSession()) {
          throw new AuthError('登录状态已失效，请重新登录。', { code: 'SESSION_EXPIRED' });
        }
        return data;
      } catch (error) {
        if (error instanceof AuthError && error.status === 401 && attempt === generation) clearSession();
        throw error;
      }
    },

    async requestWithOptionalSession(path: string, options: RequestOptions = {}): Promise<unknown> {
      const current = activeSession();
      if (!current) return request(path, options);
      const attempt = generation;
      try {
        return await request(path, { ...options, token: current.token });
      } catch (error) {
        // 过期的可选凭据不能把公开 GET 变成登录墙。
        if (error instanceof AuthError && error.status === 401 && attempt === generation &&
            (options.method ?? 'GET') === 'GET') {
          clearSession();
          return request(path, options);
        }
        throw error;
      }
    },

    // 完全匿名的公开读取；需要 viewer_state 时使用 requestWithOptionalSession。
    async requestPublic(path: string, options: RequestOptions = {}): Promise<unknown> {
      return request(path, options);
    },

    getSession(): PublicSession | null {
      const current = activeSession();
      return current ? { user: current.user, expiresAt: current.expiresAt } : null;
    },

    signOut: clearSession,
  };
}
