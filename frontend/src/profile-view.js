import { element } from './view-kit.js';
import { DEFAULT_PAGE_SIZE } from './discover-view.js';

export const PROFILE_TABS = Object.freeze([
  { id: 'videos', label: '投稿' },
  { id: 'favorites', label: '收藏' },
  { id: 'history', label: '历史' },
  { id: 'follows', label: '关注' },
]);
const TAB_IDS = PROFILE_TABS.map((tab) => tab.id);
const TAB_LOADERS = Object.freeze({
  videos: 'listMyVideos',
  favorites: 'listFavorites',
  history: 'listHistory',
  follows: 'listFollows',
});
const EMPTY_STATES = Object.freeze({
  videos: { title: '还没有投稿', hint: '上传你的第一支视频，让大家看到。' },
  favorites: { title: '还没有收藏', hint: '在视频页点收藏，把喜欢的作品留在这里。' },
  history: { title: '还没有观看历史', hint: '看过的视频会自动出现在这里。' },
  follows: { title: '还没有关注的人', hint: '关注创作者，第一时间看到更新。' },
});
const VISIBILITIES = Object.freeze(['public', 'private']);
const VISIBILITY_LABELS = Object.freeze({ public: '公开', private: '私密' });
const TITLE_MIN_LENGTH = 1;
const TITLE_MAX_LENGTH = 100;
const DESCRIPTION_MAX_LENGTH = 2000;

export function normalizeTab(tab) {
  const value = String(tab ?? '');
  return TAB_IDS.includes(value) ? value : TAB_IDS[0];
}

export function profileRequest(tab, { page = 1, pageSize = DEFAULT_PAGE_SIZE } = {}) {
  return { loader: TAB_LOADERS[normalizeTab(tab)], page, pageSize };
}

export function tabList(activeTab) {
  const active = normalizeTab(activeTab);
  return element('div', {
    className: 'profile-tabs',
    attrs: { role: 'tablist', 'aria-label': '个人中心分区' },
    children: PROFILE_TABS.map((tab) => element('button', {
      className: 'profile-tab',
      attrs: {
        type: 'button',
        role: 'tab',
        'aria-selected': tab.id === active ? 'true' : 'false',
        'aria-current': tab.id === active ? 'page' : undefined,
      },
      props: { disabled: false },
      dataset: { tab: tab.id },
      text: tab.label,
    })),
  });
}

// 校验和取值都基于同一份裁剪后的 values，调用方直接把它当补丁提交即可。
export function ownerPatch(fields = {}, original = null) {
  const normalized = {
    title: String(fields.title ?? '').trim(),
    description: String(fields.description ?? '').trim(),
    visibility: String(fields.visibility ?? '').trim(),
  };
  const errors = {};
  const titleLength = Array.from(normalized.title).length;
  if (titleLength < TITLE_MIN_LENGTH) errors.title = `标题至少需要 ${TITLE_MIN_LENGTH} 个字符`;
  if (titleLength > TITLE_MAX_LENGTH) errors.title = `标题最多 ${TITLE_MAX_LENGTH} 个字符`;
  if (Array.from(normalized.description).length > DESCRIPTION_MAX_LENGTH) {
    errors.description = `简介最多 ${DESCRIPTION_MAX_LENGTH} 个字符`;
  }
  if (!VISIBILITIES.includes(normalized.visibility)) errors.visibility = '可见性只能是 public 或 private';
  let values = normalized;
  if (original && typeof original === 'object') {
    values = Object.fromEntries(Object.entries(normalized).filter(([key, value]) => (
      value !== String(original[key] ?? '').trim()
    )));
    if (Object.keys(values).length === 0 && Object.keys(errors).length === 0) {
      errors.form = '没有需要保存的修改';
    }
  }
  return { valid: Object.keys(errors).length === 0, values, errors };
}

function visibilityOptions(current) {
  return VISIBILITIES.map((value) => element('option', {
    attrs: { value },
    props: { value, selected: value === current },
    text: VISIBILITY_LABELS[value],
  }));
}

export function ownerEditForm(video) {
  const title = video?.title || '';
  const visibility = VISIBILITIES.includes(video?.visibility) ? video.visibility : VISIBILITIES[0];
  return element('form', {
    className: 'owner-edit',
    children: [
      // 标题以文本回显（而非属性），保证作者自己填的恶意字符也只是普通文字。
      element('h3', { className: 'owner-edit-heading', text: `编辑：《${title}》` }),
      element('label', { attrs: { for: 'owner-title' }, text: '标题' }),
      element('input', {
        className: 'owner-title',
        id: 'owner-title',
        attrs: { name: 'title', type: 'text', maxlength: TITLE_MAX_LENGTH },
        props: { value: title },
      }),
      element('label', { attrs: { for: 'owner-description' }, text: '简介' }),
      element('textarea', {
        className: 'owner-description',
        id: 'owner-description',
        attrs: { name: 'description', rows: 3, maxlength: DESCRIPTION_MAX_LENGTH },
        props: { value: video?.description || '' },
      }),
      element('label', { attrs: { for: 'owner-visibility' }, text: '可见性' }),
      element('select', {
        className: 'owner-visibility',
        id: 'owner-visibility',
        attrs: { name: 'visibility' },
        props: { value: visibility },
        children: visibilityOptions(visibility),
      }),
      element('button', { className: 'owner-save', attrs: { type: 'submit' }, text: '保存修改' }),
      element('button', {
        className: 'owner-delete',
        attrs: { type: 'button' },
        dataset: { action: 'delete-video', videoId: String(video?.id ?? '') },
        text: '删除视频',
      }),
    ],
  });
}

export function deleteConfirmation(video) {
  const title = video?.title || '这支视频';
  return element('div', {
    className: 'delete-confirmation',
    attrs: { role: 'alertdialog', 'aria-modal': 'true', 'aria-label': '确认删除视频' },
    children: [
      element('strong', { text: `确认删除《${title}》吗？` }),
      element('p', { text: '删除后无法恢复，请谨慎操作。' }),
      element('button', {
        className: 'confirm-cancel',
        attrs: { type: 'button' },
        dataset: { action: 'cancel-delete' },
        text: '取消',
      }),
      element('button', {
        className: 'confirm-delete',
        attrs: { type: 'button' },
        dataset: { action: 'confirm-delete', videoId: String(video?.id ?? '') },
        text: '确认删除',
      }),
    ],
  });
}

function videoCard(video) {
  const stats = video?.stats || {};
  const author = video?.author || {};
  const authorName = author.nickname || author.username || '';
  return element('div', {
    className: 'profile-card',
    children: [
      element('strong', { className: 'profile-card-title', text: video?.title || '未命名视频' }),
      authorName ? element('span', { className: 'profile-card-author', text: `BY ${authorName}` }) : null,
      video?.description ? element('span', { className: 'profile-card-description', text: video.description }) : null,
      element('span', {
        className: 'profile-card-stats',
        text: `${Number(stats.like_count) || 0} 赞 · ${Number(stats.view_count) || 0} 播放 · ${Number(stats.comment_count) || 0} 评论`,
      }),
      element('button', {
        className: 'profile-card-open',
        attrs: { type: 'button' },
        dataset: { action: 'open-video', videoId: String(video?.id ?? '') },
        text: '打开',
      }),
    ],
  });
}

function userCard(user) {
  const name = user?.nickname || user?.username || '匿名用户';
  const children = [element('strong', { className: 'user-card-name', text: name })];
  if (user?.username) children.push(element('span', { className: 'user-card-handle', text: `@${user.username}` }));
  return element('div', { className: 'user-card', children });
}

function emptyState(tab) {
  const state = EMPTY_STATES[tab];
  return element('div', {
    className: 'empty-state',
    children: [
      element('span', { className: 'empty-mark', text: '✦' }),
      element('strong', { text: state.title }),
      element('p', { text: state.hint }),
    ],
  });
}

export function profileGrid(tab, result) {
  const key = normalizeTab(tab);
  const items = Array.isArray(result?.items) ? result.items : [];
  if (!items.length) return emptyState(key);
  const render = key === 'follows' ? userCard : videoCard;
  return element('ul', {
    className: 'profile-grid',
    children: items.map((item) => element('li', {
      className: 'profile-grid-item',
      dataset: { itemId: String(item?.id ?? '') },
      children: [render(item)],
    })),
  });
}
