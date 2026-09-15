import { AuthError } from './auth.js';

const text = (value) => typeof value === 'string' ? value : '';
const MAX_VIDEO_BYTES = 500 * 1024 * 1024;

export class VideoError extends Error {
  constructor(message, { code = 'VIDEO_ERROR', status = 0, fieldErrors = {} } = {}) {
    super(message);
    this.name = 'VideoError';
    this.code = code;
    this.status = status;
    this.fieldErrors = fieldErrors;
  }
}

export function validateVideoSubmission(input = {}) {
  const values = {
    title: text(input.title).trim(),
    description: text(input.description).trim(),
    file: input.file ?? null,
  };
  const errors = {};
  const titleLength = Array.from(values.title).length;
  if (titleLength < 1 || titleLength > 100) errors.title = '标题需为 1–100 个字符。';
  if (Array.from(values.description).length > 2000) errors.description = '简介不能超过 2000 个字符。';
  if (!values.file || typeof values.file !== 'object') {
    errors.file = '请选择一个 MP4 视频。';
  } else {
    const fileName = text(values.file.name);
    const contentType = text(values.file.type).toLowerCase();
    const isMP4 = contentType === 'video/mp4' || (!contentType && /\.mp4$/i.test(fileName));
    if (!isMP4) errors.file = '仅支持 MP4 格式的视频。';
    else if (!Number.isFinite(values.file.size) || values.file.size <= 0) errors.file = '所选视频为空，请重新选择。';
    else if (values.file.size > MAX_VIDEO_BYTES) errors.file = '视频不能超过 500 MiB。';
  }
  return { values, errors, valid: Object.keys(errors).length === 0 };
}

// 作者的部分更新：只接受显式提供的字段，未提供的字段保持原值。
export function validateVideoPatch(input = {}) {
  const values = {};
  const errors = {};
  if (input.title !== undefined) {
    const title = text(input.title).trim();
    if (Array.from(title).length < 1 || Array.from(title).length > 100) {
      errors.title = '标题需为 1–100 个字符。';
    } else {
      values.title = title;
    }
  }
  if (input.description !== undefined) {
    const description = text(input.description).trim();
    if (Array.from(description).length > 2000) errors.description = '简介不能超过 2000 个字符。';
    else values.description = description;
  }
  if (input.visibility !== undefined) {
    const visibility = text(input.visibility);
    if (!['public', 'private'].includes(visibility)) errors.visibility = '可见性只能是 public 或 private。';
    else values.visibility = visibility;
  }
  if (Object.keys(values).length === 0 && Object.keys(errors).length === 0) {
    errors.title = '请至少提供一个要修改的字段。';
  }
  return { values, errors, valid: Object.keys(errors).length === 0 };
}

function positiveInteger(value, fallback) {
  const number = Number(value);
  return Number.isInteger(number) && number > 0 ? number : fallback;
}

function videoFrom(data) {
  if (!data || typeof data !== 'object' || !['string', 'number'].includes(typeof data.id)) {
    throw new VideoError('服务器返回了无法识别的视频数据。', { code: 'INVALID_RESPONSE' });
  }
  return data;
}

function listFrom(data) {
  if (!data || typeof data !== 'object' || !Array.isArray(data.items) ||
      !Number.isInteger(data.page) || !Number.isInteger(data.page_size) ||
      !Number.isInteger(data.total)) {
    throw new VideoError('服务器返回了无法识别的视频列表。', { code: 'INVALID_RESPONSE' });
  }
  data.items.forEach(videoFrom);
  return data;
}

export function createVideoClient({
  authClient,
  fetchImpl = globalThis.fetch,
  sleep = (milliseconds) => new Promise((resolve) => setTimeout(resolve, milliseconds)),
  now = Date.now,
} = {}) {
  if (!authClient || typeof authClient.requestWithSession !== 'function'
      || typeof authClient.requestPublic !== 'function'
      || typeof authClient.requestWithOptionalSession !== 'function') {
    throw new TypeError('createVideoClient requires an auth client');
  }

  const pagePath = (path, page, pageSize) => {
    const query = new URLSearchParams({
      page: String(positiveInteger(page, 1)),
      page_size: String(positiveInteger(pageSize, 12)),
    });
    return `${path}?${query}`;
  };

  const discoveryPath = ({ query, sort, page, pageSize } = {}) => {
    const params = new URLSearchParams();
    const keyword = text(query).trim();
    if (keyword) params.set('q', keyword);
    const order = text(sort);
    if (order) params.set('sort', order);
    params.set('page', String(positiveInteger(page, 1)));
    params.set('page_size', String(positiveInteger(pageSize, 12)));
    return `/videos?${params}`;
  };

  const checkedID = (id) => {
    if (!/^\d+$/.test(String(id)) || Number(id) < 1) {
      throw new VideoError('视频编号无效。', { code: 'INVALID_PARAMETER' });
    }
    return encodeURIComponent(String(id));
  };

  const getMyVideo = async (id) => videoFrom(
    await authClient.requestWithSession(`/users/me/videos/${checkedID(id)}`),
  );

  return {
    async listVideos({ query, sort, page = 1, pageSize = 12 } = {}) {
      return listFrom(await authClient.requestPublic(discoveryPath({ query, sort, page, pageSize })));
    },

    async getVideo(id) {
      return videoFrom(await authClient.requestWithOptionalSession(`/videos/${checkedID(id)}`));
    },

    async listMyVideos({ page = 1, pageSize = 12 } = {}) {
      return listFrom(await authClient.requestWithSession(pagePath('/users/me/videos', page, pageSize)));
    },

    getMyVideo,

    async updateVideo(id, input) {
      const validation = validateVideoPatch(input);
      if (!validation.valid) {
        throw new VideoError('请检查要修改的内容。', {
          code: 'INVALID_PARAMETER',
          fieldErrors: validation.errors,
        });
      }
      return videoFrom(await authClient.requestWithSession(`/users/me/videos/${checkedID(id)}`, {
        method: 'PATCH',
        body: validation.values,
      }));
    },

    async deleteVideo(id) {
      return videoFrom(await authClient.requestWithSession(`/users/me/videos/${checkedID(id)}`, {
        method: 'DELETE',
      }));
    },

    async waitUntilProcessed(id, {
      intervalMs = 2500,
      timeoutMs = 30 * 60 * 1000,
      onUpdate = () => {},
      signal,
    } = {}) {
      const deadline = now() + timeoutMs;
      for (;;) {
        if (signal?.aborted) {
          throw new VideoError('处理状态查询已取消。', { code: 'REQUEST_CANCELLED' });
        }
        const item = await getMyVideo(id);
        onUpdate(item);
        if (['ready', 'failed', 'deleted'].includes(item.status)) return item;
        if (now() >= deadline) {
          throw new VideoError('视频仍在处理中，请稍后到“我的投稿”查看。', { code: 'PROCESSING_TIMEOUT' });
        }
        await sleep(intervalMs);
      }
    },

    async uploadVideo(input, { onStep = () => {} } = {}) {
      const validation = validateVideoSubmission(input);
      if (!validation.valid) {
        throw new VideoError('请检查投稿信息。', {
          code: 'INVALID_PARAMETER',
          fieldErrors: validation.errors,
        });
      }
      const { title, description, file } = validation.values;
      const contentType = text(file.type).toLowerCase() || 'video/mp4';

      onStep('creating');
      const created = videoFrom(await authClient.requestWithSession('/videos', {
        method: 'POST',
        body: {
          title,
          description,
          file_name: file.name,
          content_type: contentType,
          file_size: file.size,
        },
      }));
      if (typeof created.upload_url !== 'string' || !created.upload_url) {
        throw new VideoError('上传地址无效，请重新创建投稿。', { code: 'INVALID_RESPONSE' });
      }

      onStep('uploading');
      let uploadResponse;
      try {
        uploadResponse = await fetchImpl(created.upload_url, {
          method: 'PUT',
          headers: { 'Content-Type': contentType },
          credentials: 'omit',
          body: file,
        });
      } catch {
        throw new VideoError('视频直传失败，请检查网络后重试。', { code: 'UPLOAD_NETWORK_ERROR' });
      }
      if (!uploadResponse?.ok) {
        throw new VideoError('视频直传失败，请重新投稿。', {
          code: 'UPLOAD_FAILED',
          status: uploadResponse?.status ?? 0,
        });
      }

      onStep('completing');
      const completed = videoFrom(await authClient.requestWithSession(`/videos/${encodeURIComponent(String(created.id))}/complete`, {
        method: 'POST',
        body: {},
      }));
      onStep(completed.status === 'processing' ? 'processing' : 'complete');
      return completed;
    },
  };
}

export function isSessionError(error) {
  return error instanceof AuthError && (error.status === 401 || error.code === 'SESSION_EXPIRED');
}
