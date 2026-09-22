<script setup lang="ts">
import { computed } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { useAuthStore } from '../stores/auth';

const auth = useAuthStore();
const route = useRoute();
const router = useRouter();
const displayName = computed(() => auth.user?.nickname || auth.user?.username || '游客');

async function signOut(): Promise<void> {
  auth.signOut();
  await router.push('/');
}
</script>

<template>
  <header class="app-header">
    <RouterLink class="app-identity" to="/" aria-label="返回发现页">
      <span class="app-avatar" aria-hidden="true">🐈‍⬛</span>
      <span><small>VIDEO SHARE</small><strong>{{ displayName }}</strong></span>
    </RouterLink>
    <nav class="app-nav" aria-label="主要导航">
      <RouterLink class="app-nav-button" :class="{ 'is-active': route.name === 'discover' }" to="/">发现</RouterLink>
      <RouterLink class="app-nav-button" :class="{ 'is-active': route.name === 'upload' }" to="/upload">投稿</RouterLink>
      <RouterLink class="app-nav-button" :class="{ 'is-active': route.name === 'profile' }" to="/me">个人中心</RouterLink>
    </nav>
    <button v-if="auth.isAuthenticated" class="signout-button" type="button" @click="signOut">退出</button>
    <RouterLink v-else class="signout-button" :to="{ name: 'login', query: { returnTo: route.fullPath } }">登录</RouterLink>
  </header>
</template>
