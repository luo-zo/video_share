import { AuthError, createAuthClient, validateLogin, validateRegistration } from './auth.js';
import { createVideoClient, isSessionError, validateVideoSubmission } from './video.js';
import { createCommunityClient } from './community.js';
import { renderInto } from './view-kit.js';
import {
  applyQuery, applySort, changePage, discoverGrid, discoverPagination,
  discoverRequest, initialDiscoverState, paginationBounds, resultSummary, searchForm,
} from './discover-view.js';
import {
  actionBar, commentComposer, commentDraft, commentList, optimisticRelation,
  relationFromServer, shouldReportWatch, watchPayload, withCleanupOnFailure,
} from './detail-view.js';
import {
  deleteConfirmation, normalizeTab, ownerEditForm, ownerPatch, profileGrid,
  profileRequest, tabList,
} from './profile-view.js';
import { attachVideoSource } from './player.js';
import { initCat } from './cat.js';

const byId = (id) => document.getElementById(id);
const pageShell = document.querySelector('.page-shell');
const skipLink = document.querySelector('.skip-link');
const form = byId('auth-form');
const fields = { username: byId('username'), password: byId('password'), nickname: byId('nickname') };
const authView = byId('auth-view');
const accountView = byId('account-view');
const submit = byId('submit-button');
const submitLabel = byId('submit-label');
const remember = byId('remember');
const passwordToggle = byId('toggle-password');
const modeToggle = byId('mode-toggle');
const switchMode = byId('switch-mode');
const storageKey = 'video_share.remembered-username';
const auth = createAuthClient();
const videos = createVideoClient({ authClient: auth });
const community = createCommunityClient({ authClient: auth });
let mode = 'login';
let busy = false;
let disabledStates = [];
let discover = initialDiscoverState();
let profileTab = 'videos';
const profilePages = { videos: 1, favorites: 1, history: 1, follows: 1 };
let discoverRequestGeneration = 0;
let mineRequestGeneration = 0;
let detailRequest = 0;
let commentsRequest = 0;
let detailId = null;
let detailReturnView = 'discover';
let releasePlayer = () => {};
let processingMonitor = null;

function currentUserId() {
  const id = Number(auth.getSession()?.user?.id);
  return Number.isInteger(id) && id > 0 ? id : null;
}

function clearPlayer() {
  releasePlayer();
  releasePlayer = () => {};
  detailRequest += 1;
  commentsRequest += 1;
  for (const id of ['detail-actions', 'detail-owner', 'detail-comment-composer', 'detail-comment-list']) {
    byId(id).replaceChildren();
  }
  byId('detail-actions').hidden = true;
  byId('detail-owner').hidden = true;
  byId('detail-comments').hidden = true;
}

function stopProcessingMonitor() {
  processingMonitor?.abort();
  processingMonitor = null;
}

function setText(id, value) {
  const element = byId(id);
  if (element) element.textContent = value;
}

function setMessage(message = '', kind = '') {
  const element = byId('form-message');
  if (!element) return;
  element.textContent = message;
  element.dataset.kind = kind;
}

function setAppMessage(message = '', kind = '') {
  const element = byId('app-message');
  element.textContent = message;
  element.dataset.kind = kind;
}

function setFieldError(name, message = '') {
  const input = fields[name];
  input.setAttribute('aria-invalid', String(Boolean(message)));
  input.closest('.field-group')?.classList.toggle('is-invalid', Boolean(message));
  setText(`${name}-error`, message);
}

function clearErrors() {
  Object.keys(fields).forEach((name) => setFieldError(name));
  setMessage();
}

function hidePassword() {
  fields.password.type = 'password';
  passwordToggle.setAttribute('aria-pressed', 'false');
  passwordToggle.setAttribute('aria-label', '显示密码');
  setText('toggle-password-label', '显示密码');
}

function inputValues() {
  return Object.fromEntries(Object.entries(fields).map(([name, input]) => [name, input.value]));
}

function validateAuthForm() {
  return mode === 'register' ? validateRegistration(inputValues()) : validateLogin(inputValues());
}

function showAuth() {
  pageShell.classList.remove('app-mode');
  authView.hidden = false;
  accountView.hidden = true;
  modeToggle.hidden = false;
  byId('header-prompt').hidden = false;
  skipLink.href = '#username';
  skipLink.textContent = '跳到登录表单';
}

function resetProfileState() {
  profileTab = 'videos';
  for (const tab of Object.keys(profilePages)) profilePages[tab] = 1;
  mineRequestGeneration += 1;
  renderProfileTabs();
  byId('mine-list').replaceChildren();
  byId('mine-pagination').replaceChildren();
}

function showGuestApp(notice = '') {
  authView.hidden = true;
  accountView.hidden = false;
  pageShell.classList.add('app-mode');
  modeToggle.hidden = false;
  byId('header-prompt').hidden = false;
  setText('header-prompt', '想投稿、评论或收藏？');
  setText('mode-toggle-label', '登录 / 注册');
  setText('account-nickname', '游客放映厅');
  setText('account-username', 'guest');
  byId('signout').hidden = true;
  document.querySelectorAll('[data-session-required]').forEach((control) => { control.hidden = true; });
  skipLink.href = '#app-content';
  skipLink.textContent = '跳到视频内容';
  setAppMessage(notice);
}

function setMode(nextMode, { message = '', focus = true } = {}) {
  mode = nextMode;
  const registering = mode === 'register';
  showAuth();
  form.dataset.mode = mode;
  byId('nickname-field').hidden = !registering;
  fields.nickname.disabled = !registering;
  fields.nickname.required = registering;
  byId('remember-option').hidden = registering;
  fields.password.value = '';
  fields.password.autocomplete = registering ? 'new-password' : 'current-password';
  hidePassword();
  clearErrors();
  setText('auth-kicker', registering ? '从这里，认识新朋友' : '再次见面，真好');
  setText('auth-title', registering ? '创建你的账号' : '欢迎回来');
  setText('auth-description', registering ? '取一个喜欢的名字，开启属于你的新故事。' : '输入账号信息，让好奇心继续出发。');
  setText('auth-method-label', registering ? '填写账号信息' : '账号密码登录');
  setText('header-prompt', registering ? '已经有账号？' : '还没有账号？');
  setText('mode-toggle-label', registering ? '返回登录' : '创建账号');
  setText('switch-copy', registering ? '已经有账号了？' : '第一次来到这里？');
  setText('switch-label', registering ? '直接登录' : '注册一个账号');
  submitLabel.textContent = registering ? '创建账号' : '登录';
  if (message) setMessage(message, message.includes('失效') ? 'error' : 'success');
  if (focus) fields.username.focus();
}

function setBusy(value) {
  if (busy === value) return;
  busy = value;
  if (value) {
    const controls = [...new Set([...form.querySelectorAll('input, button'), modeToggle, switchMode])];
    disabledStates = controls.map((control) => [control, control.disabled]);
    controls.forEach((control) => { control.disabled = true; });
    form.setAttribute('aria-busy', 'true');
    submit.setAttribute('aria-busy', 'true');
    submit.classList.add('is-loading');
    submitLabel.textContent = mode === 'register' ? '正在创建账号…' : '正在登录…';
    return;
  }
  disabledStates.forEach(([control, disabled]) => { control.disabled = disabled; });
  disabledStates = [];
  form.removeAttribute('aria-busy');
  submit.removeAttribute('aria-busy');
  submit.classList.remove('is-loading');
  submitLabel.textContent = mode === 'register' ? '创建账号' : '登录';
}

function rememberUsername(username, enabled) {
  try {
    if (enabled) localStorage.setItem(storageKey, username);
    else localStorage.removeItem(storageKey);
    return '';
  } catch {
    return '浏览器限制了本地存储访问，无法更新用户名保存设置。';
  }
}

function showAccount(user, notice = '') {
  fields.password.value = '';
  hidePassword();
  setText('account-nickname', user.nickname || user.username);
  setText('account-username', user.username);
  setText('account-id', String(user.id));
  setText('account-created', user.created_at);
  authView.hidden = true;
  accountView.hidden = false;
  modeToggle.hidden = true;
  byId('header-prompt').hidden = true;
  byId('signout').hidden = false;
  document.querySelectorAll('[data-session-required]').forEach((control) => { control.hidden = false; });
  pageShell.classList.add('app-mode');
  skipLink.href = '#app-content';
  skipLink.textContent = '跳到视频内容';
  setAppMessage(notice);
  byId('account-nickname').focus();
}

function reportAuthError(error) {
  const message = error instanceof AuthError ? error.message : '发生了意外错误，请稍后重试。';
  setMessage(message, 'error');
  if (!(error instanceof AuthError)) return;
  for (const [name, fieldMessage] of Object.entries(error.fieldErrors)) {
    if (fields[name]) setFieldError(name, fieldMessage);
  }
  if (error.code === 'USER_ALREADY_EXISTS') setFieldError('username', message);
  Object.values(fields).find((field) => field.getAttribute('aria-invalid') === 'true')?.focus();
}

function leaveExpiredSession(error) {
  if (!isSessionError(error)) return false;
  auth.signOut();
  stopProcessingMonitor();
  clearPlayer();
  resetProfileState();
  showGuestApp();
  selectView('discover', { focus: true });
  setAppMessage(error.message || '请登录后继续操作。', 'error');
  return true;
}

function requireSession(message = '请先登录，再继续这个操作。') {
  if (currentUserId() !== null) return true;
  setAppMessage(message, 'error');
  modeToggle.focus();
  return false;
}

function reportAppError(error) {
  if (leaveExpiredSession(error)) return;
  setAppMessage(error instanceof Error ? error.message : '视频服务暂时不可用，请稍后重试。', 'error');
}

const statusNames = {
  pending: '等待上传', uploading: '上传中', uploaded: '等待确认', processing: '处理中',
  ready: '可播放', published: '已发布', failed: '处理失败',
};
const statusName = (status) => statusNames[status] || status || '状态未知';

function dateLabel(value) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleDateString('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit',
  });
}

function make(tag, className = '', text = '') {
  const element = document.createElement(tag);
  if (className) element.className = className;
  if (text) element.textContent = text;
  return element;
}

function renderEmpty(container, message) {
  const empty = make('div', 'empty-state');
  empty.append(make('span', 'empty-mark', '✦'), make('strong', '', message), make('p', '', '小黑会在这里替你守着下一段故事。'));
  container.replaceChildren(empty);
}

function renderCards(container, items, emptyMessage, { owner = false } = {}) {
  if (!items.length) {
    renderEmpty(container, emptyMessage);
    return;
  }
  const fragment = document.createDocumentFragment();
  items.forEach((item, index) => {
    const card = make('button', 'video-card');
    card.type = 'button';
    card.dataset.videoId = String(item.id);
    card.setAttribute('aria-label', `查看视频：${item.title || '未命名视频'}`);
    const art = make('span', 'video-card-art');
    art.dataset.tone = String((index % 3) + 1);
    if (item.cover_url) {
      const cover = make('img', 'video-card-cover');
      cover.src = item.cover_url;
      cover.alt = '';
      cover.loading = 'lazy';
      cover.addEventListener('error', () => cover.remove(), { once: true });
      art.append(cover);
    }
    art.append(make('span', 'video-card-number', String(index + 1).padStart(2, '0')), make('span', 'video-card-play', '▶'));
    const content = make('span', 'video-card-content');
    const meta = make('span', 'video-card-meta');
    meta.append(make('span', `status-chip status-${item.status || 'unknown'}`, statusName(item.status)), make('span', '', dateLabel(item.created_at)));
    content.append(meta, make('strong', 'video-card-title', item.title || '未命名视频'));
    if (item.status === 'processing') {
      const progress = Math.max(0, Math.min(100, Number(item.processing_progress) || 0));
      const progressWrap = make('span', 'processing-progress');
      const progressTrack = make('span', 'processing-track');
      const progressFill = make('span', 'processing-fill');
      progressFill.style.width = `${progress}%`;
      progressTrack.setAttribute('role', 'progressbar');
      progressTrack.setAttribute('aria-label', '视频处理进度');
      progressTrack.setAttribute('aria-valuemin', '0');
      progressTrack.setAttribute('aria-valuemax', '100');
      progressTrack.setAttribute('aria-valuenow', String(progress));
      progressTrack.append(progressFill);
      progressWrap.append(progressTrack, make('span', 'processing-label', `${progress}%`));
      content.append(progressWrap);
    }
    if (item.status === 'failed' && item.processing_error) {
      content.append(make('span', 'processing-error', item.processing_error));
    }
    if (item.description) content.append(make('span', 'video-card-description', item.description));
    const stats = item.stats || {};
    content.append(make(
      'span',
      'video-card-stats',
      `${Number(stats.like_count) || 0} 赞 · ${Number(stats.view_count) || 0} 播放 · ${Number(stats.comment_count) || 0} 评论`,
    ));
    const author = item.author?.nickname || item.author?.username;
    if (author) content.append(make('span', 'video-card-author', `BY ${author}`));
    card.append(art, content);
    if (owner) {
      card.addEventListener('click', () => openOwnerDetail(item.id));
    } else if (item.status === 'ready' || item.status === 'published') {
      card.addEventListener('click', () => openDetail(item.id));
    } else {
      card.disabled = true;
      card.classList.add('is-waiting');
      card.setAttribute('aria-label', `${item.title || '未命名视频'}，${statusName(item.status)}`);
    }
    fragment.append(card);
  });
  const grid = make('div', 'video-grid');
  grid.append(fragment);
  container.replaceChildren(grid);
}

function renderDiscoverForm() {
  renderInto(byId('discover-search'), searchForm(discover));
}

async function loadDiscover() {
  const ownRequest = ++discoverRequestGeneration;
  const requestedState = { ...discover };
  const container = byId('discover-list');
  container.setAttribute('aria-busy', 'true');
  container.replaceChildren(make('p', 'loading-state', '正在寻找值得放映的故事…'));
  try {
    const result = await videos.listVideos(discoverRequest(requestedState));
    if (ownRequest !== discoverRequestGeneration) return;
    const bounds = paginationBounds(requestedState, result);
    if (bounds.page !== requestedState.page) {
      discover = { ...discover, page: bounds.page };
      loadDiscover();
      return;
    }
    renderInto(container, discoverGrid(result));
    byId('discover-summary').textContent = resultSummary(requestedState, result);
    renderInto(byId('discover-pagination'), discoverPagination(requestedState, result));
    container.querySelectorAll('.video-card').forEach((card) => {
      card.addEventListener('click', () => openDetail(Number(card.dataset.videoId)));
    });
  } catch (error) {
    if (ownRequest !== discoverRequestGeneration) return;
    container.replaceChildren();
    byId('discover-summary').textContent = '';
    renderInto(byId('discover-pagination'), null);
    reportAppError(error);
  } finally {
    if (ownRequest === discoverRequestGeneration) container.removeAttribute('aria-busy');
  }
}

function renderProfileTabs() {
  renderInto(byId('mine-tabs'), tabList(profileTab));
}

async function loadMine() {
  if (!requireSession('请先登录，再查看你的个人中心。')) return;
  const ownRequest = ++mineRequestGeneration;
  const requestedTab = profileTab;
  const container = byId('mine-list');
  container.setAttribute('aria-busy', 'true');
  container.replaceChildren(make('p', 'loading-state', '正在整理你的放映室…'));
  try {
    const page = profilePages[requestedTab];
    const { loader } = profileRequest(requestedTab, { page });
    const result = loader === 'listMyVideos'
      ? await videos.listMyVideos({ page })
      : await community[loader]({ page });
    if (ownRequest !== mineRequestGeneration || requestedTab !== profileTab) return;
    const bounds = paginationBounds({ page, pageSize: result.page_size }, result);
    if (bounds.page !== page) {
      profilePages[requestedTab] = bounds.page;
      loadMine();
      return;
    }
    profilePages[requestedTab] = bounds.page;
    if (requestedTab === 'videos') renderCards(container, result.items, '你还没有投稿', { owner: true });
    else renderInto(container, profileGrid(requestedTab, result));
    renderInto(byId('mine-pagination'), discoverPagination({ page: bounds.page, pageSize: result.page_size }, result));
  } catch (error) {
    if (ownRequest !== mineRequestGeneration || requestedTab !== profileTab) return;
    container.replaceChildren();
    reportAppError(error);
  } finally {
    if (ownRequest === mineRequestGeneration) container.removeAttribute('aria-busy');
  }
}

function selectView(view, { focus = true, load = true } = {}) {
  if (['upload', 'mine'].includes(view) && !requireSession('请先登录，再使用投稿和个人中心。')) return;
  if (view !== 'detail') clearPlayer();
  document.querySelectorAll('[data-app-panel]').forEach((panel) => { panel.hidden = panel.dataset.appPanel !== view; });
  document.querySelectorAll('[data-app-view]').forEach((button) => {
    const active = button.dataset.appView === view || (view === 'detail' && button.dataset.appView === 'discover');
    button.classList.toggle('is-active', active);
    if (active) button.setAttribute('aria-current', 'page');
    else button.removeAttribute('aria-current');
  });
  setAppMessage();
  if (focus) byId(`${view}-title`)?.focus();
  if (load && view === 'discover') loadDiscover();
  if (load && view === 'mine') loadMine();
}

function relationState(item) {
  return {
    like: {
      active: Boolean(item?.viewer_state?.liked),
      count: Math.max(0, Number(item?.stats?.like_count) || 0),
    },
    favorite: {
      active: Boolean(item?.viewer_state?.favorited),
      count: Math.max(0, Number(item?.stats?.favorite_count) || 0),
    },
    follow: { active: Boolean(item?.viewer_state?.following_author) },
  };
}

// 就地改写按钮而不是整块重绘，避免乐观更新时焦点与滚动位置被丢掉。
function paintActions(bar, relations) {
  for (const [action, relation] of [['like', relations.like], ['favorite', relations.favorite]]) {
    const button = bar.querySelector(`[data-action="${action}"]`);
    if (!button) continue;
    button.setAttribute('aria-pressed', relation.active ? 'true' : 'false');
    const count = button.querySelector('.action-count');
    if (count) count.textContent = String(relation.count);
  }
  const follow = bar.querySelector('[data-action="follow"]');
  if (follow) {
    follow.setAttribute('aria-pressed', relations.follow.active ? 'true' : 'false');
    follow.textContent = relations.follow.active ? '已关注' : '关注';
  }
}

async function toggleRelation(action, item, relations, bar) {
  if (!requireSession(`请先登录，再${action === 'like' ? '点赞' : action === 'favorite' ? '收藏' : '关注'}。`)) return;
  const ownDetailRequest = detailRequest;
  const button = bar.querySelector(`[data-action="${action}"]`);
  const snapshot = { ...relations[action] };
  const next = !snapshot.active;
  relations[action] = action === 'follow' ? { active: next } : optimisticRelation(snapshot, next);
  if (button) button.disabled = true;
  paintActions(bar, relations);
  try {
    const server = action === 'like'
      ? await community.setLike(item.id, next)
      : action === 'favorite'
        ? await community.setFavorite(item.id, next)
        : await community.setFollow(item.author?.id, next);
    if (ownDetailRequest !== detailRequest) return;
    relations[action] = action === 'follow'
      ? { active: Boolean(server.following) }
      : relationFromServer(server, relations[action]);
  } catch (error) {
    if (ownDetailRequest !== detailRequest) return;
    relations[action] = snapshot;
    reportAppError(error);
  } finally {
    if (ownDetailRequest !== detailRequest) return;
    if (button) button.disabled = false;
    paintActions(bar, relations);
  }
}

async function loadComments(videoId, host) {
  const ownDetailRequest = detailRequest;
  const ownRequest = ++commentsRequest;
  host.setAttribute('aria-busy', 'true');
  host.replaceChildren(make('p', 'loading-state', '正在加载评论…'));
  try {
    const result = await community.listComments(videoId, { pageSize: 50 });
    if (ownDetailRequest !== detailRequest || ownRequest !== commentsRequest) return;
    renderInto(host, commentList(result.items, currentUserId()));
  } catch (error) {
    if (ownDetailRequest !== detailRequest || ownRequest !== commentsRequest) return;
    host.replaceChildren();
    reportAppError(error);
  } finally {
    if (ownDetailRequest === detailRequest && ownRequest === commentsRequest) {
      host.removeAttribute('aria-busy');
    }
  }
}

function renderComposer(item, state = {}) {
  const ownDetailRequest = detailRequest;
  const mount = byId('detail-comment-composer');
  const draft = {
    content: state.content ?? '',
    valid: false,
    error: '',
    disabled: Boolean(state.submitting),
  };
  renderInto(mount, commentComposer(draft, { error: state.error ?? '' }));
  const form = mount.querySelector('.comment-composer');
  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    if (!requireSession('请先登录，再发表评论。')) return;
    const typed = form.querySelector('textarea').value;
    const checked = commentDraft(typed);
    if (!checked.valid) {
      renderComposer(item, { content: typed, error: checked.error });
      mount.querySelector('textarea')?.focus();
      return;
    }
    renderComposer(item, { content: typed, submitting: true });
    try {
      await community.addComment(item.id, { content: checked.content });
      if (ownDetailRequest !== detailRequest) return;
      renderComposer(item);
      await loadComments(item.id, byId('detail-comment-list'));
    } catch (error) {
      if (ownDetailRequest !== detailRequest) return;
      renderComposer(item, { content: typed, error: error.message || '评论发布失败。' });
      reportAppError(error);
    }
  });
}

// 观看上报是尽力而为的：匿名观众没有会话，静默跳过，不能影响播放体验。
function watchReporter(player, videoId, durationMs) {
  if (!Number.isFinite(durationMs) || durationMs < 0 || currentUserId() === null) return () => {};
  let lastReportedMs = 0;
  let lastSentMs = -1;
  let started = false;
  const knownDurationMs = () => {
    const mediaDuration = Math.floor(Number(player.duration) * 1000);
    return Number.isFinite(mediaDuration) && mediaDuration > 0 ? mediaDuration : durationMs;
  };
  const send = (positionMs, { keepalive = false, force = false } = {}) => {
    const payload = watchPayload(positionMs, knownDurationMs());
    if (!payload) return;
    if (force && payload.progress_ms === lastSentMs) return;
    lastReportedMs = payload.progress_ms;
    lastSentMs = payload.progress_ms;
    community.reportWatch(videoId, {
      progressMs: payload.progress_ms,
      durationMs: payload.duration_ms,
      keepalive,
    })
      .catch((error) => { if (!isSessionError(error)) reportAppError(error); });
  };
  const positionMs = () => Math.floor(Math.max(0, Number(player.currentTime) || 0) * 1000);
  const onPlaying = () => {
    if (started) return;
    started = true;
    send(positionMs());
  };
  const onTimeUpdate = () => {
    const current = positionMs();
    if (shouldReportWatch({ lastReportedMs, positionMs: current, durationMs: knownDurationMs() })) send(current);
  };
  const onVisibilityChange = () => {
    if (started && document.visibilityState === 'hidden') send(positionMs(), { keepalive: true, force: true });
  };
  const onPageHide = () => { if (started) send(positionMs(), { keepalive: true, force: true }); };
  player.addEventListener('playing', onPlaying);
  player.addEventListener('timeupdate', onTimeUpdate);
  document.addEventListener('visibilitychange', onVisibilityChange);
  window.addEventListener('pagehide', onPageHide);
  return () => {
    if (started) send(positionMs(), { keepalive: true, force: true });
    player.removeEventListener('playing', onPlaying);
    player.removeEventListener('timeupdate', onTimeUpdate);
    document.removeEventListener('visibilitychange', onVisibilityChange);
    window.removeEventListener('pagehide', onPageHide);
  };
}

function renderOwnerForm(item) {
  const mount = byId('detail-owner');
  renderInto(mount, ownerEditForm(item));
  mount.hidden = false;
  mount.querySelector('.owner-edit').addEventListener('submit', (event) => {
    event.preventDefault();
    submitOwnerPatch(item, mount);
  });
  mount.querySelector('[data-action="delete-video"]').addEventListener('click', () => renderDeleteConfirmation(item));
}

function renderDeleteConfirmation(item) {
  const mount = byId('detail-owner');
  renderInto(mount, deleteConfirmation(item));
  mount.hidden = false;
  mount.querySelector('[data-action="cancel-delete"]').addEventListener('click', () => renderOwnerForm(item));
  mount.querySelector('[data-action="confirm-delete"]').addEventListener('click', () => confirmDeleteVideo(item));
}

async function submitOwnerPatch(item, mount) {
  const ownDetailRequest = detailRequest;
  const patch = ownerPatch({
    title: mount.querySelector('[name="title"]').value,
    description: mount.querySelector('[name="description"]').value,
    visibility: mount.querySelector('[name="visibility"]').value,
  }, item);
  if (!patch.valid) {
    setAppMessage(Object.values(patch.errors).join(' '), 'error');
    return;
  }
  const save = mount.querySelector('.owner-save');
  save.disabled = true;
  try {
    await videos.updateVideo(item.id, patch.values);
    if (ownDetailRequest !== detailRequest || Number(item.id) !== detailId) return;
    setAppMessage('视频信息已更新。', 'success');
    await openOwnerDetail(item.id);
  } catch (error) {
    if (ownDetailRequest !== detailRequest || Number(item.id) !== detailId) return;
    save.disabled = false;
    reportAppError(error);
  }
}

async function confirmDeleteVideo(item) {
  const ownDetailRequest = detailRequest;
  const confirm = byId('detail-owner').querySelector('[data-action="confirm-delete"]');
  if (confirm) confirm.disabled = true;
  try {
    await videos.deleteVideo(item.id);
    if (ownDetailRequest !== detailRequest || Number(item.id) !== detailId) return;
    profilePages.videos = 1;
    profileTab = 'videos';
    selectView('mine', { focus: false });
    setAppMessage('视频已删除。', 'success');
  } catch (error) {
    if (ownDetailRequest !== detailRequest || Number(item.id) !== detailId) return;
    if (confirm) confirm.disabled = false;
    reportAppError(error);
  }
}

function openOwnerDetail(id) {
  return openDetail(id, { owner: true });
}

async function openDetail(id, { owner = false } = {}) {
  clearPlayer();
  detailId = Number(id);
  detailReturnView = owner ? 'mine' : 'discover';
  setText('detail-back-label', owner ? '返回我的' : '返回发现');
  const ownRequest = detailRequest;
  selectView('detail', { focus: false, load: false });
  const container = byId('video-detail');
  let pendingWatchCleanup = () => {};
  container.setAttribute('aria-busy', 'true');
  container.replaceChildren(make('p', 'loading-state', '正在准备放映…'));
  try {
    const hadSession = currentUserId() !== null;
    const item = owner ? await videos.getMyVideo(id) : await videos.getVideo(id);
    if (ownRequest !== detailRequest) return;
    if (!owner && hadSession && currentUserId() === null) {
      stopProcessingMonitor();
      resetProfileState();
      showGuestApp('登录状态已失效，已切换为游客浏览。');
    }
    const article = make('article', 'detail-card');
    const playerWrap = make('div', 'player-wrap');
    const playable = ['ready', 'published'].includes(item.status)
      && typeof item.play_url === 'string' && item.play_url.length > 0;
    let player = null;
    if (playable) {
      player = document.createElement('video');
      player.controls = true;
      player.preload = 'metadata';
      player.playsInline = true;
      if (item.cover_url) player.poster = item.cover_url;
      player.setAttribute('aria-label', `播放 ${item.title}`);
      playerWrap.append(player);
    } else {
      playerWrap.append(make(
        'p',
        'loading-state',
        owner ? `${statusName(item.status)}，你仍可在右侧修改或删除这支投稿。` : '暂时无法播放这支视频。',
      ));
    }

    const copy = make('div', 'detail-copy');
    const meta = make('div', 'detail-meta');
    const visibilitySuffix = item.visibility === 'private' ? ' · 私密' : '';
    meta.append(
      make('span', `status-chip status-${item.status || 'unknown'}`, `${statusName(item.status)}${visibilitySuffix}`),
      make('span', '', dateLabel(item.created_at)),
    );
    const title = make('h2', '', item.title || '未命名视频');
    title.id = 'detail-title';
    title.tabIndex = -1;
    copy.append(meta, title);
    if (item.description) copy.append(make('p', 'detail-description', item.description));
    const author = item.author?.nickname || item.author?.username;
    if (author) copy.append(make('p', 'detail-author', `投稿人 · ${author}`));
    const mediaFacts = [
      item.width && item.height ? `${item.width} × ${item.height}` : '',
      Number.isFinite(item.duration_ms) ? `${Math.max(1, Math.round(item.duration_ms / 1000))} 秒` : '',
      item.play_type === 'hls' ? '自适应 HLS' : 'MP4 原片',
    ].filter(Boolean);
    if (mediaFacts.length) copy.append(make('p', 'detail-media-facts', mediaFacts.join(' · ')));
    article.append(playerWrap, copy);
    container.replaceChildren(article);

    const relations = relationState(item);
    if (!owner) {
      const actions = byId('detail-actions');
      renderInto(actions, actionBar(item, currentUserId()));
      actions.hidden = false;
      const bar = actions.firstElementChild;
      paintActions(bar, relations);
      for (const action of ['like', 'favorite', 'follow']) {
        bar.querySelector(`[data-action="${action}"]`)
          ?.addEventListener('click', () => toggleRelation(action, item, relations, bar));
      }
    }

    if (owner || Number(item.user_id) === currentUserId()) renderOwnerForm(item);

    if (!owner) {
      byId('detail-comments').hidden = false;
      renderComposer(item);
      loadComments(item.id, byId('detail-comment-list'));
    }

    if (player) {
      const stopWatch = watchReporter(player, item.id, Number(item.duration_ms) || 0);
      pendingWatchCleanup = stopWatch;
      const cleanup = await withCleanupOnFailure(() => attachVideoSource(player, item, {
        onError: (playbackError) => {
          if (!playbackError.fatal || ownRequest !== detailRequest) return;
          const diagnostic = JSON.stringify(playbackError);
          console.error(`HLS playback error: ${diagnostic}`);
          setAppMessage(`视频播放失败（${playbackError.details || playbackError.type || 'HLS_ERROR'}）。`, 'error');
        },
      }), stopWatch);
      pendingWatchCleanup = () => {};
      if (ownRequest !== detailRequest) {
        stopWatch();
        cleanup();
        return;
      }
      releasePlayer = () => { stopWatch(); cleanup(); };
    }
    title.focus();
  } catch (error) {
    pendingWatchCleanup();
    if (ownRequest !== detailRequest) return;
    commentsRequest += 1;
    for (const id of ['detail-actions', 'detail-owner', 'detail-comment-composer', 'detail-comment-list']) {
      byId(id).replaceChildren();
    }
    byId('detail-actions').hidden = true;
    byId('detail-owner').hidden = true;
    byId('detail-comments').hidden = true;
    container.replaceChildren();
    reportAppError(error);
  } finally {
    if (ownRequest === detailRequest) container.removeAttribute('aria-busy');
  }
}

function setVideoFieldError(name, message = '') {
  const input = byId(`video-${name}`);
  if (!input) return;
  input.setAttribute('aria-invalid', String(Boolean(message)));
  input.closest('.field-group')?.classList.toggle('is-invalid', Boolean(message));
  setText(`video-${name}-error`, message);
}

function clearVideoErrors() {
  ['title', 'description', 'file'].forEach((name) => setVideoFieldError(name));
}

const uploadOrder = ['creating', 'uploading', 'completing', 'processing'];

function setUploadStep(step = '') {
  const activeIndex = uploadOrder.indexOf(step);
  document.querySelectorAll('[data-upload-step]').forEach((item, index) => {
    item.classList.toggle('is-active', index === activeIndex);
    item.classList.toggle('is-complete', step === 'complete' || (activeIndex >= 0 && index < activeIndex));
  });
}

function setUploadBusy(value) {
  const uploadForm = byId('upload-form');
  uploadForm.querySelectorAll('input, textarea, button').forEach((control) => {
    control.disabled = value;
  });
  uploadForm.toggleAttribute('aria-busy', value);
  const label = byId('upload-submit').querySelector('span');
  label.textContent = value ? '正在投递…' : '开始投稿';
}

function fileSizeLabel(bytes) {
  if (!Number.isFinite(bytes) || bytes <= 0) return '空文件';
  if (bytes < 1024 * 1024) return `${Math.ceil(bytes / 1024)} KiB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MiB`;
}

function showUploadErrors(error) {
  if (error?.fieldErrors && typeof error.fieldErrors === 'object') {
    Object.entries(error.fieldErrors).forEach(([name, message]) => setVideoFieldError(name, message));
    byId('upload-form').querySelector('[aria-invalid="true"]')?.focus();
  }
  reportAppError(error);
}

function updateProcessingCard(item) {
  const id = Number(item?.id);
  if (!Number.isInteger(id) || id < 1) return;
  const card = document.querySelector(`.video-card[data-video-id="${id}"]`);
  if (!card) return;
  const progress = Math.max(0, Math.min(100, Number(item.processing_progress) || 0));
  const track = card.querySelector('.processing-track');
  const fill = card.querySelector('.processing-fill');
  const label = card.querySelector('.processing-label');
  if (track) track.setAttribute('aria-valuenow', String(progress));
  if (fill) fill.style.width = `${progress}%`;
  if (label) label.textContent = `${progress}%`;
}

async function monitorProcessingVideo(id) {
  stopProcessingMonitor();
  const controller = new AbortController();
  processingMonitor = controller;
  try {
    const result = await videos.waitUntilProcessed(id, {
      signal: controller.signal,
      onUpdate: (item) => {
        updateProcessingCard(item);
        if (item.status === 'processing') {
          setAppMessage(`视频正在转码，当前进度 ${item.processing_progress || 0}%。`);
        }
      },
    });
    if (controller.signal.aborted) return;
    await loadMine();
    if (result.status === 'ready') {
      setAppMessage('视频处理完成，已经可以播放。', 'success');
    } else if (result.status === 'failed') {
      setAppMessage(result.processing_error || '视频处理失败，请稍后重试。', 'error');
    }
  } catch (error) {
    if (error?.code !== 'REQUEST_CANCELLED') reportAppError(error);
  } finally {
    if (processingMonitor === controller) processingMonitor = null;
  }
}

for (const [name, input] of Object.entries(fields)) {
  input.addEventListener('input', () => {
    setFieldError(name);
    setMessage();
  });
  input.addEventListener('blur', () => {
    const result = validateAuthForm();
    setFieldError(name, result.errors[name] || '');
  });
}

passwordToggle.addEventListener('click', () => {
  const visible = fields.password.type === 'text';
  fields.password.type = visible ? 'password' : 'text';
  passwordToggle.setAttribute('aria-pressed', String(!visible));
  passwordToggle.setAttribute('aria-label', visible ? '显示密码' : '隐藏密码');
  setText('toggle-password-label', visible ? '显示密码' : '隐藏密码');
  fields.password.focus();
});

modeToggle.addEventListener('click', () => {
  if (!accountView.hidden && currentUserId() === null) {
    setMode('login');
    return;
  }
  setMode(mode === 'login' ? 'register' : 'login');
});
switchMode.addEventListener('click', () => setMode(mode === 'login' ? 'register' : 'login'));
byId('browse-guest').addEventListener('click', () => {
  showGuestApp();
  selectView('discover', { focus: true });
});

remember.addEventListener('change', () => {
  if (!remember.checked) rememberUsername('', false);
});

form.addEventListener('submit', async (event) => {
  event.preventDefault();
  if (busy) return;
  clearErrors();
  const validation = validateAuthForm();
  if (!validation.valid) {
    Object.entries(validation.errors).forEach(([name, message]) => setFieldError(name, message));
    setMessage('请检查标记的输入项。', 'error');
    Object.values(fields).find((field) => field.getAttribute('aria-invalid') === 'true')?.focus();
    return;
  }

  setBusy(true);
  try {
    if (mode === 'register') {
      await auth.register(validation.values);
      const username = validation.values.username;
      form.reset();
      fields.username.value = username;
      setMode('login', { message: '账号创建成功，现在可以登录了。', focus: false });
      fields.password.focus();
      return;
    }

    const user = await auth.signIn(validation.values);
    const storageNotice = rememberUsername(validation.values.username, remember.checked);
    resetProfileState();
    showAccount(user, storageNotice);
    discover = initialDiscoverState();
    renderDiscoverForm();
    selectView('discover', { focus: false });
  } catch (error) {
    reportAuthError(error);
  } finally {
    setBusy(false);
  }
});

byId('signout').addEventListener('click', () => {
  stopProcessingMonitor();
  clearPlayer();
  auth.signOut();
  resetProfileState();
  discover = initialDiscoverState();
  renderDiscoverForm();
  showGuestApp('你已安全退出，仍可继续浏览公开视频。');
  selectView('discover', { focus: true });
});

document.querySelectorAll('[data-app-view]').forEach((button) => {
  button.addEventListener('click', () => selectView(button.dataset.appView));
});

byId('refresh-discover').addEventListener('click', loadDiscover);
byId('refresh-mine').addEventListener('click', loadMine);
byId('detail-back').addEventListener('click', () => selectView(detailReturnView));

// 表单每次重绘都换新节点，所以监听器挂在不变的挂载点上做事件委托。
byId('discover-search').addEventListener('submit', (event) => {
  event.preventDefault();
  discover = applyQuery(discover, byId('search-query')?.value ?? '');
  renderDiscoverForm();
  loadDiscover();
});
byId('discover-search').addEventListener('change', (event) => {
  if (event.target?.id !== 'search-sort') return;
  discover = applySort(discover, event.target.value);
  renderDiscoverForm();
  loadDiscover();
});
byId('discover-search').addEventListener('click', (event) => {
  if (!event.target.closest('[data-action="reset-search"]')) return;
  discover = applyQuery(discover, '');
  renderDiscoverForm();
  loadDiscover();
});

byId('discover-pagination').addEventListener('click', (event) => {
  const button = event.target.closest('[data-action]');
  if (!button) return;
  discover = changePage(discover, button.dataset.action === 'next' ? 1 : -1);
  loadDiscover();
});

byId('mine-tabs').addEventListener('click', (event) => {
  const button = event.target.closest('[data-tab]');
  if (!button) return;
  const next = normalizeTab(button.dataset.tab);
  if (next === profileTab) return;
  profileTab = next;
  renderProfileTabs();
  loadMine();
});

byId('mine-pagination').addEventListener('click', (event) => {
  const button = event.target.closest('[data-action]');
  if (!button) return;
  const step = button.dataset.action === 'next' ? 1 : -1;
  profilePages[profileTab] = Math.max(1, profilePages[profileTab] + step);
  loadMine();
});

byId('mine-list').addEventListener('click', (event) => {
  const button = event.target.closest('[data-action="open-video"]');
  if (!button) return;
  openDetail(Number(button.dataset.videoId));
});

byId('detail-comment-list').addEventListener('click', async (event) => {
  const button = event.target.closest('[data-action="delete-comment"]');
  if (!button || detailId === null) return;
  if (!requireSession('请先登录，再删除评论。')) return;
  const ownDetailRequest = detailRequest;
  const videoId = detailId;
  button.disabled = true;
  try {
    await community.deleteComment(button.dataset.commentId);
    if (ownDetailRequest !== detailRequest || videoId !== detailId) return;
    await loadComments(videoId, byId('detail-comment-list'));
  } catch (error) {
    if (ownDetailRequest !== detailRequest) return;
    button.disabled = false;
    reportAppError(error);
  }
});

const uploadForm = byId('upload-form');
const videoFile = byId('video-file');

for (const name of ['title', 'description']) {
  byId(`video-${name}`).addEventListener('input', () => setVideoFieldError(name));
}

videoFile.addEventListener('change', () => {
  setVideoFieldError('file');
  const file = videoFile.files[0];
  setText('video-file-name', file ? `${file.name} · ${fileSizeLabel(file.size)}` : '尚未选择文件');
});

uploadForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  clearVideoErrors();
  setAppMessage();
  const validation = validateVideoSubmission({
    title: byId('video-title').value,
    description: byId('video-description').value,
    file: videoFile.files[0],
  });
  if (!validation.valid) {
    Object.entries(validation.errors).forEach(([name, message]) => setVideoFieldError(name, message));
    setAppMessage('请检查标记的投稿信息。', 'error');
    uploadForm.querySelector('[aria-invalid="true"]')?.focus();
    return;
  }

  setUploadBusy(true);
  setUploadStep('creating');
  try {
    const submitted = await videos.uploadVideo(validation.values, { onStep: setUploadStep });
    uploadForm.reset();
    setText('video-file-name', '尚未选择文件');
    profileTab = 'videos';
    profilePages.videos = 1;
    renderProfileTabs();
    selectView('mine', { focus: true, load: false });
    await loadMine();
    if (submitted.status === 'processing') {
      setAppMessage('原视频上传完成，后台正在生成封面和多清晰度视频。', 'success');
      monitorProcessingVideo(submitted.id);
    } else {
      setAppMessage('投稿完成，视频已经可以播放。', 'success');
    }
  } catch (error) {
    showUploadErrors(error);
  } finally {
    setUploadBusy(false);
    window.setTimeout(() => setUploadStep(), 900);
  }
});

try {
  const savedUsername = localStorage.getItem(storageKey);
  if (savedUsername) {
    fields.username.value = savedUsername;
    remember.checked = true;
  }
} catch {
  setMessage('浏览器限制了本地存储访问，用户名保存功能不可用。', 'error');
}

renderDiscoverForm();
renderProfileTabs();
showGuestApp();
selectView('discover', { focus: false });
initCat();
submit.disabled = false;
