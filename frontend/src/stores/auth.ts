import { defineStore } from 'pinia';
import { computed, ref } from 'vue';

import { authClient } from '../api';
import type { LoginInput, RegistrationInput, SessionUser } from '../api/auth';

/**
 * 只镜像会话的「用户 + 过期时间」。真正的令牌留在 auth 客户端实例内存里，
 * 这里既不存储也不暴露它，避免被顺手写进 localStorage 或日志。
 * 注意：本任务保留原有登录语义，刷新页面即结束会话，持久会话在 T03 落地。
 */
export const useAuthStore = defineStore('auth', () => {
  const user = ref<SessionUser | null>(null);
  const expiresAt = ref(0);
  const busy = ref(false);

  const isAuthenticated = computed(() => user.value !== null && expiresAt.value > Date.now());

  function clear(): void {
    user.value = null;
    expiresAt.value = 0;
  }

  function mirror(next: SessionUser): SessionUser {
    user.value = next;
    expiresAt.value = authClient.getSession()?.expiresAt ?? 0;
    return next;
  }

  async function signIn(input: LoginInput): Promise<SessionUser> {
    busy.value = true;
    try {
      return mirror(await authClient.signIn(input));
    } finally {
      busy.value = false;
    }
  }

  /** 注册只创建账号，不会顺带登录——沿用原本的两步流程。 */
  async function register(input: RegistrationInput): Promise<SessionUser> {
    busy.value = true;
    try {
      return await authClient.register(input);
    } finally {
      busy.value = false;
    }
  }

  async function refreshProfile(): Promise<SessionUser | null> {
    if (!authClient.getSession()) {
      clear();
      return null;
    }
    try {
      return mirror(await authClient.getProfile());
    } catch (error) {
      // 客户端已判定会话失效并自行清理；这里同步本地镜像后继续向上抛出。
      clear();
      throw error;
    }
  }

  function signOut(): void {
    authClient.signOut();
    clear();
  }

  return { user, expiresAt, busy, isAuthenticated, signIn, register, refreshProfile, signOut };
});
