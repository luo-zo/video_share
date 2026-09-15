const text = (value) => typeof value === 'string' ? value : '';
const MAX_COMMENT_LENGTH = 500;

export class CommunityError extends Error {
  constructor(message, { code = 'COMMUNITY_ERROR', status = 0, fieldErrors = {} } = {}) {
    super(message);
    this.name = 'CommunityError';
    this.code = code;
    this.status = status;
    this.fieldErrors = fieldErrors;
  }
}

export function validateComment(input = {}) {
  const content = text(input.content).trim();
  const errors = {};
  const length = Array.from(content).length;
  if (length < 1 || length > MAX_COMMENT_LENGTH) {
    errors.content = `评论内容需为 1–${MAX_COMMENT_LENGTH} 个字符。`;
  }
  return { values: { content }, errors, valid: Object.keys(errors).length === 0 };
}

const invalidResponse = () => new CommunityError('服务器返回了无法识别的数据，请稍后重试。', {
  code: 'INVALID_RESPONSE',
});

function positiveInteger(value, fallback) {
  const number = Number(value);
  return Number.isInteger(number) && number > 0 ? number : fallback;
}

function checkedID(value, label) {
  if (!/^\d+$/.test(String(value)) || Number(value) < 1) {
    throw new CommunityError(`${label}无效。`, { code: 'INVALID_PARAMETER' });
  }
  return encodeURIComponent(String(value));
}

function listFrom(data) {
  if (!data || typeof data !== 'object' || !Array.isArray(data.items) ||
      !Number.isInteger(data.page) || !Number.isInteger(data.page_size) ||
      !Number.isInteger(data.total)) {
    throw invalidResponse();
  }
  return data;
}

function idFrom(data) {
  if (!data || typeof data !== 'object' || !['string', 'number'].includes(typeof data.id)) {
    throw invalidResponse();
  }
  return data;
}

function commentFrom(data) {
  idFrom(data);
  if (typeof data.content !== 'string') throw invalidResponse();
  return data;
}

// 点赞与收藏返回写入后的最终状态，而不是“本次是否改变”，因此重复调用结果一致。
function relationFrom(data) {
  if (!data || typeof data !== 'object' || typeof data.active !== 'boolean' || !Number.isInteger(data.count)) {
    throw invalidResponse();
  }
  return data;
}

function followFrom(data) {
  if (!data || typeof data !== 'object' || typeof data.following !== 'boolean') throw invalidResponse();
  return data;
}

function watchFrom(data) {
  if (!data || typeof data !== 'object' || !Number.isInteger(data.progress_ms)
      || !Number.isInteger(data.duration_ms)) {
    throw invalidResponse();
  }
  return data;
}

export function createCommunityClient({ authClient } = {}) {
  if (!authClient || typeof authClient.requestWithSession !== 'function'
      || typeof authClient.requestPublic !== 'function') {
    throw new TypeError('createCommunityClient requires an auth client');
  }

  const pagePath = (path, page, pageSize) => {
    const query = new URLSearchParams({
      page: String(positiveInteger(page, 1)),
      page_size: String(positiveInteger(pageSize, 12)),
    });
    return `${path}?${query}`;
  };

  const videoID = (id) => checkedID(id, '视频编号');
  const userID = (id) => checkedID(id, '用户编号');

  const setRelation = (path, active) => authClient.requestWithSession(path, {
    method: active ? 'PUT' : 'DELETE',
  });

  return {
    async listComments(id, { page = 1, pageSize = 12 } = {}) {
      return listFrom(await authClient.requestPublic(
        pagePath(`/videos/${videoID(id)}/comments`, page, pageSize),
      ));
    },

    async addComment(id, input) {
      const validation = validateComment(input);
      if (!validation.valid) {
        throw new CommunityError('请检查评论内容。', {
          code: 'INVALID_PARAMETER',
          fieldErrors: validation.errors,
        });
      }
      return commentFrom(await authClient.requestWithSession(`/videos/${videoID(id)}/comments`, {
        method: 'POST',
        body: { content: validation.values.content },
      }));
    },

    async deleteComment(id) {
      return idFrom(await authClient.requestWithSession(`/comments/${checkedID(id, '评论编号')}`, {
        method: 'DELETE',
      }));
    },

    async setLike(id, active) {
      return relationFrom(await setRelation(`/videos/${videoID(id)}/like`, active));
    },

    async setFavorite(id, active) {
      return relationFrom(await setRelation(`/videos/${videoID(id)}/favorite`, active));
    },

    async reportWatch(id, { progressMs, durationMs, keepalive = false } = {}) {
      if (!Number.isInteger(durationMs) || durationMs < 0) {
        throw new CommunityError('视频时长无效。', { code: 'INVALID_PARAMETER' });
      }
      if (!Number.isInteger(progressMs) || progressMs < 0 || (durationMs > 0 && progressMs > durationMs)) {
        throw new CommunityError('观看进度不能超过视频时长。', { code: 'INVALID_PARAMETER' });
      }
      return watchFrom(await authClient.requestWithSession(`/videos/${videoID(id)}/watch`, {
        method: 'POST',
        body: { progress_ms: progressMs, duration_ms: durationMs },
        keepalive,
      }));
    },

    async listFavorites({ page = 1, pageSize = 12 } = {}) {
      return listFrom(await authClient.requestWithSession(pagePath('/users/me/favorites', page, pageSize)));
    },

    async listHistory({ page = 1, pageSize = 12 } = {}) {
      return listFrom(await authClient.requestWithSession(pagePath('/users/me/history', page, pageSize)));
    },

    async listFollows({ page = 1, pageSize = 12 } = {}) {
      return listFrom(await authClient.requestWithSession(pagePath('/users/me/follows', page, pageSize)));
    },

    async setFollow(id, active) {
      return followFrom(await setRelation(`/users/${userID(id)}/follow`, active));
    },
  };
}
