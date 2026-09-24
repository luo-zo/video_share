import { defineStore } from 'pinia';
import { computed, onScopeDispose, ref } from 'vue';

import { authClient } from '../api';
import type { LoginInput, RegistrationInput, SessionUser } from '../api/auth';

/**
 * 只镜像会话的「用户 + 过期时间」。真正的令牌留在 auth 客户端实例内存里，
 * 这里既不存储也不暴露它，避免被顺手写进 localStorage 或日志。
 * 刷新令牌只在 HttpOnly Cookie 中，由后端轮换；访问 JWT 永远只在内存中。
 */
export const useAuthStore = defineStore('auth', () => {
  const user = ref<SessionUser | null>(null);
  const expiresAt = ref(0);
  const busy = ref(false);
  const clock = ref(Date.now());
  const timer = typeof window === 'undefined' ? undefined : window.setInterval(() => { clock.value = Date.now(); }, 1000);
  onScopeDispose(() => { if (timer !== undefined) window.clearInterval(timer); });

  const isAuthenticated = computed(() => { void clock.value; return user.value !== null && expiresAt.value > Date.now(); });

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
      const code = (error as { code?: string }).code;
      if (code === 'SESSION_EXPIRED' || code === 'UNAUTHORIZED' || (error as { status?: number }).status === 401) clear();
      throw error;
    }
  }

  async function updateProfile(input: { nickname: unknown; bio: unknown }): Promise<SessionUser> {
    const next = await authClient.updateProfile(input);
    return mirror(next);
  }

  async function changePassword(input: { oldPassword: unknown; newPassword: unknown }): Promise<void> {
    await authClient.changePassword(input);
    clear();
  }

  async function restore(): Promise<SessionUser | null> {
    try {
      const restored = await authClient.restore();
      if (!restored) { clear(); return null; }
      return mirror(restored);
    } catch (error) {
      const status = (error as { status?: number }).status;
      if (status === 401 || (error as { code?: string }).code === 'SESSION_EXPIRED') clear();
      throw error;
    }
  }

  async function signOut(): Promise<void> {
    clear();
    await authClient.signOut();
  }

  return { user, expiresAt, busy, isAuthenticated, signIn, register, refreshProfile, restore, updateProfile, changePassword, signOut };
});
