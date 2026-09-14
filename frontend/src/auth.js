const API_ROOT = '/api/v1';
const text = (value) => typeof value === 'string' ? value : '';

export function validateLogin(input = {}) {
  const values = {
    username: text(input.username).trim().toLowerCase(),
    password: text(input.password),
  };
  const errors = {};
  if (!values.username) errors.username = '请输入用户名。';
  if (!values.password) errors.password = '请输入密码。';
  return { values, errors, valid: Object.keys(errors).length === 0 };
}

export function validateRegistration(input = {}) {
  const result = validateLogin(input);
  result.values.nickname = text(input.nickname).trim();
  const { values, errors } = result;
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
  result.valid = Object.keys(errors).length === 0;
  return result;
}

export class AuthError extends Error {
  constructor(message, { code = 'UNKNOWN_ERROR', status = 0, requestId = '', fieldErrors = {} } = {}) {
    super(message);
    this.name = 'AuthError';
    this.code = code;
    this.status = status;
    this.requestId = requestId;
    this.fieldErrors = fieldErrors;
  }
}

function responseError(status, detail = {}) {
  const code = typeof detail?.code === 'string' ? detail.code : `HTTP_${status}`;
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
  else if (code === 'INVALID_PARAMETER') message = '输入信息不符合要求，请检查后重试。';
  else if (status >= 500) message = '服务暂时不可用，请稍后重试。';
  return new AuthError(message, {
    code,
    status,
    requestId: typeof detail?.request_id === 'string' ? detail.request_id : '',
  });
}

const invalidResponse = () => new AuthError('服务器返回了无法识别的数据，请稍后重试。', {
  code: 'INVALID_RESPONSE',
});

function userFrom(data) {
  if (!data || typeof data !== 'object' || typeof data.username !== 'string' ||
      typeof data.nickname !== 'string' || typeof data.created_at !== 'string' ||
      !['string', 'number'].includes(typeof data.id)) {
    throw invalidResponse();
  }
  return Object.freeze({
    id: data.id,
    username: data.username,
    nickname: data.nickname,
    created_at: data.created_at,
  });
}

/** Tokens stay inside this instance and are committed only after /users/me succeeds. */
export function createAuthClient({ fetchImpl = globalThis.fetch, now = Date.now, timeoutMs = 10000 } = {}) {
  let session = null;
  let generation = 0;

  const clearSession = () => {
    session = null;
    generation += 1;
  };

  const activeSession = () => {
    if (session && now() >= session.expiresAt) clearSession();
    return session;
  };

  async function request(path, { method = 'GET', body, token } = {}) {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);
    const headers = { Accept: 'application/json' };
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    if (token) headers.Authorization = `Bearer ${token}`;
    try {
      const response = await fetchImpl(`${API_ROOT}${path}`, {
        method,
        headers,
        credentials: 'omit',
        cache: 'no-store',
        signal: controller.signal,
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      });
      let payload;
      try {
        payload = await response.json();
      } catch (error) {
        if (controller.signal.aborted) throw error;
        if (!response.ok) throw responseError(response.status);
        throw invalidResponse();
      }
      if (!response.ok) throw responseError(response.status, payload?.error);
      if (!payload || typeof payload !== 'object' || !('data' in payload)) throw invalidResponse();
      return payload.data;
    } catch (error) {
      if (error instanceof AuthError) throw error;
      if (controller.signal.aborted) {
        throw new AuthError('请求超时，请检查连接后重试。', { code: 'REQUEST_TIMEOUT' });
      }
      throw new AuthError('无法连接账号服务，请检查网络或稍后重试。', { code: 'NETWORK_ERROR' });
    } finally {
      clearTimeout(timer);
    }
  }

  const checked = (validation) => {
    if (!validation.valid) {
      throw new AuthError('请检查标记的输入项。', {
        code: 'INVALID_PARAMETER',
        fieldErrors: validation.errors,
      });
    }
    return validation.values;
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
      const startedAt = now();
      const data = await request('/auth/login', { method: 'POST', body: values });
      if (!data || typeof data.access_token !== 'string' || !data.access_token ||
          typeof data.token_type !== 'string' || data.token_type.toLowerCase() !== 'bearer' ||
          typeof data.expires_in !== 'number' || !Number.isFinite(data.expires_in) || data.expires_in <= 0) {
        throw invalidResponse();
      }
      const expiresAt = startedAt + data.expires_in * 1000;
      const user = userFrom(await request('/users/me', { token: data.access_token }));
      if (attempt !== generation) {
        throw new AuthError('这次登录已取消，请重新登录。', { code: 'REQUEST_CANCELLED' });
      }
      if (now() >= expiresAt) {
        throw new AuthError('登录状态已失效，请重新登录。', { code: 'SESSION_EXPIRED' });
      }
      session = { token: data.access_token, user, expiresAt };
      return user;
    },

    async getProfile() {
      const current = activeSession();
      if (!current) throw new AuthError('请先登录账号。', { code: 'SESSION_EXPIRED' });
      const attempt = generation;
      try {
        const user = userFrom(await request('/users/me', { token: current.token }));
        if (attempt !== generation || !activeSession()) {
          throw new AuthError('登录状态已失效，请重新登录。', { code: 'SESSION_EXPIRED' });
        }
        session.user = user;
        return user;
      } catch (error) {
        if (error.status === 401 && attempt === generation) clearSession();
        throw error;
      }
    },

    async requestWithSession(path, options = {}) {
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
        if (error.status === 401 && attempt === generation) clearSession();
        throw error;
      }
    },

    // 公开读取与受保护请求共用响应解析和错误映射，但不要求会话，也不发送 Bearer 令牌。
    async requestPublic(path, options = {}) {
      return request(path, options);
    },

    getSession() {
      const current = activeSession();
      return current ? { user: current.user, expiresAt: current.expiresAt } : null;
    },

    signOut: clearSession,
  };
}
