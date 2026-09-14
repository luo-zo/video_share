import { AuthError, createAuthClient, validateLogin, validateRegistration } from './auth.js';
import { createVideoClient, isSessionError, validateVideoSubmission } from './video.js';
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
const pageSize = 12;
let mode = 'login';
let busy = false;
let disabledStates = [];
let discoverPage = 1;
let minePage = 1;
let listRequest = 0;
let detailRequest = 0;
let releasePlayer = () => {};
let processingMonitor = null;

function clearPlayer() {
  releasePlayer();
  releasePlayer = () => {};
  detailRequest += 1;
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
  setMode('login', { message: error.message || '登录状态已失效，请重新登录。' });
  return true;
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

function renderCards(container, items, emptyMessage) {
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
    const author = item.author?.nickname || item.author?.username;
    if (author) content.append(make('span', 'video-card-author', `BY ${author}`));
    card.append(art, content);
    if (item.status === 'ready' || item.status === 'published') {
      card.addEventListener('click', () => openDetail(item.id));
    } else {
      card.disabled = true;
      card.classList.add('is-waiting');
      card.setAttribute('aria-label', `${item.title || '未命名视频'}，${statusName(item.status)}`);
    }
    fragment.append(card);
  });
  container.replaceChildren(fragment);
}

function updatePagination(prefix, result) {
  const pagination = byId(`${prefix}-pagination`);
  const totalPages = Math.max(1, Math.ceil(result.total / result.page_size));
  pagination.hidden = result.total <= result.page_size;
  byId(`${prefix}-page`).textContent = `${result.page} / ${totalPages}`;
  byId(`${prefix}-prev`).disabled = result.page <= 1;
  byId(`${prefix}-next`).disabled = result.page >= totalPages;
}

async function loadVideos(kind) {
  const ownRequest = ++listRequest;
  const mine = kind === 'mine';
  const page = mine ? minePage : discoverPage;
  const container = byId(mine ? 'mine-list' : 'discover-list');
  container.setAttribute('aria-busy', 'true');
  container.replaceChildren(make('p', 'loading-state', '正在寻找值得放映的故事…'));
  try {
    const result = mine ? await videos.listMyVideos({ page, pageSize }) : await videos.listVideos({ page, pageSize });
    if (ownRequest !== listRequest) return;
    renderCards(container, result.items, mine ? '你还没有投稿' : '放映厅暂时还没有视频');
    updatePagination(mine ? 'mine' : 'discover', result);
  } catch (error) {
    if (ownRequest !== listRequest) return;
    container.replaceChildren();
    reportAppError(error);
  } finally {
    container.removeAttribute('aria-busy');
  }
}

function selectView(view, { focus = true, load = true } = {}) {
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
  if (load && view === 'discover') loadVideos('discover');
  if (load && view === 'mine') loadVideos('mine');
}

async function openDetail(id) {
  clearPlayer();
  const ownRequest = detailRequest;
  selectView('detail', { focus: false, load: false });
  const container = byId('video-detail');
  container.setAttribute('aria-busy', 'true');
  container.replaceChildren(make('p', 'loading-state', '正在准备放映…'));
  try {
    const item = await videos.getVideo(id);
    if (ownRequest !== detailRequest) return;
    const article = make('article', 'detail-card');
    const playerWrap = make('div', 'player-wrap');
    const player = document.createElement('video');
    player.controls = true;
    player.preload = 'metadata';
    player.playsInline = true;
    if (item.cover_url) player.poster = item.cover_url;
    player.setAttribute('aria-label', `播放 ${item.title}`);
    playerWrap.append(player);

    const copy = make('div', 'detail-copy');
    const meta = make('div', 'detail-meta');
    meta.append(make('span', 'status-chip status-ready', '可播放'), make('span', '', dateLabel(item.created_at)));
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
    const cleanup = await attachVideoSource(player, item, {
      onError: (playbackError) => {
        if (!playbackError.fatal) return;
        const diagnostic = JSON.stringify(playbackError);
        console.error(`HLS playback error: ${diagnostic}`);
        setAppMessage(`视频播放失败（${playbackError.details || playbackError.type || 'HLS_ERROR'}）。`, 'error');
      },
    });
    if (ownRequest !== detailRequest) {
      cleanup();
      return;
    }
    releasePlayer = cleanup;
    title.focus();
  } catch (error) {
    container.replaceChildren();
    reportAppError(error);
  } finally {
    container.removeAttribute('aria-busy');
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
    await loadVideos('mine');
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

for (const toggle of [modeToggle, switchMode]) {
  toggle.addEventListener('click', () => setMode(mode === 'login' ? 'register' : 'login'));
}

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
    showAccount(user, storageNotice);
    discoverPage = 1;
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
  setMode('login', { message: '你已安全退出。' });
});

document.querySelectorAll('[data-app-view]').forEach((button) => {
  button.addEventListener('click', () => selectView(button.dataset.appView));
});

byId('refresh-discover').addEventListener('click', () => loadVideos('discover'));
byId('refresh-mine').addEventListener('click', () => loadVideos('mine'));
byId('detail-back').addEventListener('click', () => selectView('discover'));

byId('discover-prev').addEventListener('click', () => {
  if (discoverPage <= 1) return;
  discoverPage -= 1;
  loadVideos('discover');
});
byId('discover-next').addEventListener('click', () => {
  discoverPage += 1;
  loadVideos('discover');
});
byId('mine-prev').addEventListener('click', () => {
  if (minePage <= 1) return;
  minePage -= 1;
  loadVideos('mine');
});
byId('mine-next').addEventListener('click', () => {
  minePage += 1;
  loadVideos('mine');
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
    minePage = 1;
    selectView('mine', { focus: true, load: false });
    await loadVideos('mine');
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

setMode('login', { focus: false });
initCat();
submit.disabled = false;
