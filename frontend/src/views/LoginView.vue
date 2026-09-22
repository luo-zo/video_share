<script setup lang="ts">
import { nextTick, reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { AuthError, validateLogin, validateRegistration } from '../api/auth';
import { safeReturnTo } from '../router';
import { useAuthStore } from '../stores/auth';

const auth = useAuthStore();
const route = useRoute();
const router = useRouter();
const mode = ref<'login' | 'register'>('login');
const form = reactive({ username: localStorage.getItem('video-share:username') ?? '', password: '', nickname: '' });
const remember = ref(Boolean(form.username));
const showPassword = ref(false);
const errors = reactive<Record<string, string>>({});
const message = ref('');
const usernameInput = ref<HTMLInputElement | null>(null);
const passwordInput = ref<HTMLInputElement | null>(null);
const nicknameInput = ref<HTMLInputElement | null>(null);

function replaceErrors(next: Readonly<Record<string, string>>): void {
  for (const key of Object.keys(errors)) delete errors[key];
  Object.assign(errors, next);
}

async function focusFirstError(): Promise<void> {
  await nextTick();
  if (errors.username) usernameInput.value?.focus();
  else if (errors.password) passwordInput.value?.focus();
  else if (errors.nickname) nicknameInput.value?.focus();
}

async function submit(): Promise<void> {
  message.value = '';
  const validation = mode.value === 'login' ? validateLogin(form) : validateRegistration(form);
  replaceErrors(validation.errors);
  if (!validation.valid) {
    await focusFirstError();
    return;
  }
  try {
    if (mode.value === 'register') {
      await auth.register(form);
      mode.value = 'login';
      form.password = '';
      message.value = '账号已创建，请使用新账号登录。';
      await nextTick();
      passwordInput.value?.focus();
      return;
    }
    await auth.signIn({ username: form.username, password: form.password });
    if (remember.value) localStorage.setItem('video-share:username', validation.values.username);
    else localStorage.removeItem('video-share:username');
    await router.replace(safeReturnTo(route.query.returnTo, '/'));
  } catch (caught) {
    const error = caught instanceof AuthError ? caught : new AuthError('登录未能完成，请稍后重试。');
    replaceErrors(error.fieldErrors);
    message.value = error.message;
    await focusFirstError();
  }
}

function switchMode(): void {
  mode.value = mode.value === 'login' ? 'register' : 'login';
  replaceErrors({});
  message.value = '';
  form.password = '';
}
</script>

<template>
  <div class="login-content">
    <div class="form-emblem" aria-hidden="true"><span>🐾</span>🐈‍⬛</div>
    <div class="form-heading">
      <p class="welcome-kicker"><span></span>{{ mode === 'login' ? 'WELCOME BACK' : 'JOIN THE ORBIT' }}</p>
      <h2>{{ mode === 'login' ? '欢迎回来' : '创建账号' }} <span class="heading-spark" aria-hidden="true">✦</span></h2>
      <p>{{ mode === 'login' ? '登录后收藏、评论并分享你的片段。' : '只需几步，就能开始分享你眼中的精彩。' }}</p>
    </div>
    <form novalidate @submit.prevent="submit">
      <div class="field-group" :class="{ 'is-invalid': errors.username }">
        <label for="username">用户名</label>
        <div class="input-wrap"><span class="input-icon" aria-hidden="true">＠</span><input id="username" ref="usernameInput" v-model="form.username" name="username" autocomplete="username" maxlength="32"></div>
        <p class="field-error">{{ errors.username }}</p>
      </div>
      <div v-if="mode === 'register'" class="field-group" :class="{ 'is-invalid': errors.nickname }">
        <label for="nickname">昵称</label>
        <div class="input-wrap"><span class="input-icon" aria-hidden="true">✦</span><input id="nickname" ref="nicknameInput" v-model="form.nickname" name="nickname" autocomplete="nickname" maxlength="64"></div>
        <p class="field-error">{{ errors.nickname }}</p>
      </div>
      <div class="field-group" :class="{ 'is-invalid': errors.password }">
        <div class="field-label-row"><label for="password">密码</label><span v-if="mode === 'register'" class="password-hint">至少 8 个字符</span></div>
        <div class="input-wrap">
          <span class="input-icon" aria-hidden="true">●</span>
          <input id="password" ref="passwordInput" v-model="form.password" name="password" :type="showPassword ? 'text' : 'password'" :autocomplete="mode === 'login' ? 'current-password' : 'new-password'">
          <button class="password-toggle" type="button" :aria-pressed="showPassword" :aria-label="showPassword ? '隐藏密码' : '显示密码'" @click="showPassword = !showPassword">◉</button>
        </div>
        <p class="field-error">{{ errors.password }}</p>
      </div>
      <label v-if="mode === 'login'" class="remember-option"><input v-model="remember" type="checkbox"><span class="checkbox-visual">✓</span>记住用户名</label>
      <button class="submit-button" type="submit" :disabled="auth.busy" :aria-busy="auth.busy"><span>{{ mode === 'login' ? '登录并继续' : '创建账号' }}</span><span class="submit-icon">→</span></button>
      <p class="form-message" :class="{ 'is-error': Boolean(message) && Object.keys(errors).length > 0 }" aria-live="polite">{{ message }}</p>
    </form>
    <p class="switch-prompt">{{ mode === 'login' ? '第一次来这里？' : '已经有账号？' }}<button class="text-button" type="button" @click="switchMode">{{ mode === 'login' ? '创建账号' : '返回登录' }} →</button></p>
    <p class="form-footnote">🔒 登录令牌只保存在当前页面，刷新后需重新登录</p>
  </div>
</template>
