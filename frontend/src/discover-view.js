import { element } from './view-kit.js';

export const DEFAULT_PAGE_SIZE = 12;
export const DISCOVER_SORTS = Object.freeze([
  { value: 'latest', label: '最新' },
  { value: 'popular', label: '热门' },
]);
const SORT_VALUES = DISCOVER_SORTS.map((sort) => sort.value);
const COVER_PREFIX = '/api/v1/videos/';

export function initialDiscoverState() {
  return { query: '', sort: 'latest', page: 1, pageSize: DEFAULT_PAGE_SIZE };
}

// 新的关键词或排序都从第一页重新开始，否则用户会落在越界的分页上。
export function applyQuery(state, rawQuery) {
  return { ...state, query: String(rawQuery ?? '').trim(), page: 1 };
}

export function applySort(state, rawSort) {
  return { ...state, sort: SORT_VALUES.includes(rawSort) ? rawSort : 'latest', page: 1 };
}

export function changePage(state, delta) {
  const step = Number(delta);
  return { ...state, page: Math.max(1, Math.trunc((Number(state.page) || 1) + (Number.isFinite(step) ? step : 0))) };
}

export function discoverRequest(state) {
  return { query: state.query, sort: state.sort, page: state.page, pageSize: state.pageSize };
}

export function resultSummary(state, result) {
  const total = Number(result?.total) || 0;
  return state.query
    ? `找到 ${total} 个与“${state.query}”相关的视频`
    : `放映厅里共有 ${total} 个视频`;
}

export function paginationBounds(state, result) {
  const pageSize = Number(result?.page_size) || Number(state?.pageSize) || DEFAULT_PAGE_SIZE;
  const total = Number(result?.total) || 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const page = Math.min(Math.max(1, Number(state?.page) || 1), totalPages);
  return { page, totalPages, canPrev: page > 1, canNext: page < totalPages };
}

export function searchForm(state) {
  return element('form', {
    className: 'discover-search',
    attrs: { role: 'search', 'aria-label': '搜索视频' },
    children: [
      element('label', { className: 'sr-only', attrs: { for: 'search-query' }, text: '搜索关键词' }),
      element('input', {
        className: 'search-input',
        id: 'search-query',
        attrs: { type: 'search', name: 'q', placeholder: '搜索标题或简介', autocomplete: 'off' },
        props: { value: state.query },
      }),
      element('label', { className: 'sr-only', attrs: { for: 'search-sort' }, text: '排序方式' }),
      element('select', {
        className: 'search-sort',
        id: 'search-sort',
        attrs: { name: 'sort' },
        props: { value: state.sort },
        children: DISCOVER_SORTS.map((sort) => element('option', {
          attrs: { value: sort.value },
          props: { value: sort.value, selected: sort.value === state.sort },
          text: sort.label,
        })),
      }),
      element('button', { className: 'search-submit', attrs: { type: 'submit' }, text: '搜索' }),
      element('button', {
        className: 'search-reset',
        attrs: { type: 'button' },
        props: { disabled: state.query === '' },
        dataset: { action: 'reset-search' },
        text: '重置',
      }),
    ],
  });
}

// 封面只接受后端生成的同源路径，避免把外部 URL 直接写进 img src。
function coverSource(item) {
  const cover = item?.cover_url;
  return typeof cover === 'string' && cover.startsWith(COVER_PREFIX) ? cover : '';
}

function videoCard(item, index) {
  const stats = item?.stats || {};
  const author = item?.author || {};
  const authorName = author.nickname || author.username || '';
  const cover = coverSource(item);
  return element('button', {
    className: 'video-card',
    attrs: { type: 'button' },
    dataset: { videoId: String(item?.id ?? '') },
    children: [
      element('span', {
        className: 'video-card-art',
        dataset: { tone: String((index % 3) + 1) },
        children: [
          cover ? element('img', { className: 'video-card-cover', attrs: { src: cover, alt: '', loading: 'lazy' } }) : null,
          element('span', { className: 'video-card-number', text: String(index + 1).padStart(2, '0') }),
          element('span', { className: 'video-card-play', text: '▶' }),
        ],
      }),
      element('span', {
        className: 'video-card-content',
        children: [
          element('strong', { className: 'video-card-title', text: item?.title || '未命名视频' }),
          authorName ? element('span', { className: 'video-card-author', text: `BY ${authorName}` }) : null,
          item?.description ? element('span', { className: 'video-card-description', text: item.description }) : null,
          element('span', {
            className: 'video-card-stats',
            text: `${Number(stats.like_count) || 0} 赞 · ${Number(stats.view_count) || 0} 播放 · ${Number(stats.comment_count) || 0} 评论`,
          }),
        ],
      }),
    ],
  });
}

export function discoverGrid(result) {
  const items = Array.isArray(result?.items) ? result.items : [];
  if (!items.length) {
    return element('div', {
      className: 'empty-state',
      children: [
        element('span', { className: 'empty-mark', text: '✦' }),
        element('strong', { text: '没有找到匹配的视频' }),
        element('p', { text: '换个关键词试试，或者看看最新上映的故事。' }),
      ],
    });
  }
  return element('ul', {
    className: 'video-grid',
    children: items.map((item, index) => element('li', { className: 'video-grid-item', children: [videoCard(item, index)] })),
  });
}

export function discoverPagination(state, result) {
  const bounds = paginationBounds(state, result);
  return element('div', {
    className: 'pagination',
    children: [
      element('button', { attrs: { type: 'button' }, props: { disabled: !bounds.canPrev }, dataset: { action: 'prev' }, text: '上一页' }),
      element('span', { className: 'pagination-page', attrs: { 'aria-live': 'polite' }, text: `${bounds.page} / ${bounds.totalPages}` }),
      element('button', { attrs: { type: 'button' }, props: { disabled: !bounds.canNext }, dataset: { action: 'next' }, text: '下一页' }),
    ],
  });
}
