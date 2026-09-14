import { element } from './view-kit.js';

// 观看进度上报的最小间隔：太频繁会淹没后端，太长则丢失断点续看的精度。
export const WATCH_REPORT_INTERVAL_MS = 15_000;
const ACTION_LABELS = Object.freeze({ like: '点赞', favorite: '收藏' });
const COMMENT_FIELD_ID = 'comment-content';

// 点赞/收藏的乐观更新：只在状态真正翻转时才增减计数，且计数不会掉到零以下。
export function optimisticRelation(snapshot, nextActive) {
  const active = Boolean(nextActive);
  const count = Math.max(0, Number(snapshot?.count) || 0);
  if (active === Boolean(snapshot?.active)) return { active, count };
  return { active, count: Math.max(0, count + (active ? 1 : -1)) };
}

// 服务端只回传它知道的字段，缺失的计数沿用乐观更新后的本地值。
export function relationFromServer(server, fallback) {
  const base = fallback && typeof fallback === 'object' ? fallback : { active: false, count: 0 };
  const baseCount = Math.max(0, Number(base.count) || 0);
  if (!server || typeof server !== 'object') return { active: Boolean(base.active), count: baseCount };
  const count = Number(server.count);
  return { active: Boolean(server.active), count: Number.isFinite(count) ? count : baseCount };
}

export function shouldReportWatch({ lastReportedMs = 0, positionMs = 0, durationMs = 0 } = {}) {
  const duration = Number(durationMs);
  if (!Number.isFinite(duration) || duration <= 0) return false;
  const elapsed = Number(positionMs) - Number(lastReportedMs);
  return Number.isFinite(elapsed) && elapsed >= WATCH_REPORT_INTERVAL_MS;
}

// 上报前把进度夹到时长之内，避免播放器回退或元数据未就绪时写出越界数据。
export function watchPayload(progressMs, durationMs) {
  const progress = Math.floor(Number(progressMs));
  const duration = Math.floor(Number(durationMs));
  if (!Number.isFinite(progress) || !Number.isFinite(duration) || progress < 0 || duration <= 0) return null;
  return { progress_ms: Math.min(progress, duration), duration_ms: duration };
}

export function commentDraft(raw, { submitting = false } = {}) {
  const content = String(raw ?? '').trim();
  const valid = content.length > 0;
  return { content, valid, error: valid ? '' : '请输入评论内容', disabled: Boolean(submitting) };
}

// 统一按 UTC 格式化，保证同一时间在不同时区的读者看到一致的服务端记录。
export function commentDate(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return date.toISOString().slice(0, 16).replace('T', ' ');
}

function statCount(stats, key) {
  return Math.max(0, Number(stats?.[key]) || 0);
}

function toggleButton({ action, label, active, count }) {
  return element('button', {
    className: 'action-button',
    attrs: { type: 'button', 'aria-pressed': active ? 'true' : 'false' },
    dataset: { action },
    children: [
      element('span', { className: 'action-label', text: label }),
      element('span', { className: 'action-count', text: String(count) }),
    ],
  });
}

function followButton({ active, userId }) {
  return element('button', {
    className: 'action-button action-follow',
    attrs: { type: 'button', 'aria-pressed': active ? 'true' : 'false' },
    dataset: { action: 'follow', userId: String(userId ?? '') },
    text: active ? '已关注' : '关注',
  });
}

export function actionBar(detail) {
  const stats = detail?.stats || {};
  const viewer = detail?.viewer_state || {};
  return element('div', {
    className: 'action-bar',
    children: [
      toggleButton({
        action: 'like',
        label: ACTION_LABELS.like,
        active: Boolean(viewer.liked),
        count: statCount(stats, 'like_count'),
      }),
      toggleButton({
        action: 'favorite',
        label: ACTION_LABELS.favorite,
        active: Boolean(viewer.favorited),
        count: statCount(stats, 'favorite_count'),
      }),
      followButton({ active: Boolean(viewer.following_author), userId: detail?.author?.id }),
    ],
  });
}

export function commentComposer(draft, { error = '' } = {}) {
  const message = error || draft?.error || '';
  const disabled = Boolean(draft?.disabled);
  return element('form', {
    className: 'comment-composer',
    children: [
      element('label', { className: 'sr-only', attrs: { for: COMMENT_FIELD_ID }, text: '评论内容' }),
      element('textarea', {
        className: 'comment-input',
        id: COMMENT_FIELD_ID,
        attrs: { name: 'content', rows: 3, placeholder: '写下你的评论' },
        props: { value: draft?.content ?? '', disabled },
      }),
      element('p', { className: 'comment-error', attrs: { role: 'alert' }, text: message }),
      element('button', {
        className: 'comment-submit',
        attrs: { type: 'submit' },
        props: { disabled },
        text: '发表评论',
      }),
    ],
  });
}

function commentItem(comment, viewerId) {
  const author = comment?.author || {};
  const authorName = author.nickname || author.username || '匿名用户';
  const own = Number.isFinite(viewerId) && Number(comment?.user_id) === viewerId;
  const children = [
    element('span', {
      className: 'comment-meta',
      children: [
        element('strong', { className: 'comment-author', text: authorName }),
        element('time', { className: 'comment-time', text: commentDate(comment?.created_at) }),
      ],
    }),
    element('p', { className: 'comment-body', text: comment?.content || '' }),
  ];
  if (own) {
    children.push(element('button', {
      className: 'comment-delete',
      attrs: { type: 'button' },
      dataset: { action: 'delete-comment', commentId: String(comment?.id ?? '') },
      text: '删除',
    }));
  }
  return element('li', {
    className: 'comment-item',
    dataset: { commentId: String(comment?.id ?? '') },
    children,
  });
}

export function commentList(comments, viewerId) {
  const items = Array.isArray(comments) ? comments : [];
  if (!items.length) {
    return element('div', {
      className: 'comment-empty',
      children: [
        element('strong', { text: '还没有评论' }),
        element('p', { text: '来抢个沙发吧。' }),
      ],
    });
  }
  const viewer = Number(viewerId);
  return element('ul', {
    className: 'comment-list',
    children: items.map((comment) => commentItem(comment, viewer)),
  });
}
