<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { useAuthStore } from '../stores/auth';
import { useNotificationsStore } from '../stores/notifications';

const auth = useAuthStore();
const notifications = useNotificationsStore();
const route = useRoute();
const router = useRouter();
const displayName = computed(() => auth.user?.nickname || auth.user?.username || '游客');
const logoutError = ref('');

async function signOut(): Promise<void> {
  logoutError.value = '';
  try {
    await auth.signOut();
    notifications.stop();
  } catch {
    logoutError.value = '退出请求未获服务器确认，请检查网络后重试。';
  }
  await router.push('/');
}

watch(() => auth.isAuthenticated, (active) => {
  if (active) notifications.start();
  else notifications.stop();
}, { immediate: true });
onUnmounted(() => notifications.stop());
</script>

<template>
  <header class="app-header">
    <RouterLink class="app-identity" to="/" aria-label="返回发现页">
      <span class="app-avatar" aria-hidden="true">🐈‍⬛</span>
      <span><small>VIDEO SHARE</small><strong>{{ displayName }}</strong></span>
    </RouterLink>
    <nav class="app-nav" aria-label="主要导航">
      <RouterLink class="app-nav-button" :class="{ 'is-active': route.name === 'discover' }" to="/">发现</RouterLink>
      <RouterLink class="app-nav-button" :class="{ 'is-active': route.name === 'following' }" to="/following">关注流</RouterLink>
      <RouterLink class="app-nav-button" :class="{ 'is-active': route.name === 'ranking' }" to="/ranking">榜单</RouterLink>
      <RouterLink class="app-nav-button" :class="{ 'is-active': route.name === 'upload' }" to="/upload">投稿</RouterLink>
      <RouterLink class="app-nav-button" :class="{ 'is-active': route.name === 'profile' }" to="/me">个人中心</RouterLink>
      <RouterLink class="app-nav-button" :class="{ 'is-active': route.name === 'settings' }" to="/settings">设置</RouterLink>
      <RouterLink v-if="auth.isAuthenticated" class="app-nav-button notification-link" :class="{ 'is-active': route.name === 'notifications' }" to="/notifications">通知<span v-if="notifications.unreadCount" class="notification-badge">{{ notifications.unreadCount > 99 ? '99+' : notifications.unreadCount }}</span></RouterLink>
      <RouterLink v-if="auth.isAuthenticated" class="app-nav-button" :class="{ 'is-active': route.name === 'creator-center' }" to="/creator-center">数据中心</RouterLink>
      <RouterLink v-if="auth.user?.role === 'admin'" class="app-nav-button" :class="{ 'is-active': route.name === 'admin-reports' }" to="/admin/reports">治理</RouterLink>
      <RouterLink v-if="auth.user?.role === 'admin'" class="app-nav-button" :class="{ 'is-active': route.name === 'admin-audit' }" to="/admin/audit">审计</RouterLink>
      <RouterLink v-if="auth.isAuthenticated" class="app-nav-button" :class="{ 'is-active': route.name === 'reports' }" to="/reports">我的举报</RouterLink>
    </nav>
    <button v-if="auth.isAuthenticated" class="signout-button" type="button" @click="signOut">退出</button>
    <RouterLink v-else class="signout-button" :to="{ name: 'login', query: { returnTo: route.fullPath } }">登录</RouterLink>
    <span v-if="logoutError" class="logout-error" role="alert">
      {{ logoutError }}
      <button type="button" @click="signOut">重试退出</button>
    </span>
  </header>
</template>
