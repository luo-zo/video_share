<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { communityClient, videoClient } from '../api';
import type { FollowedUser } from '../api/community';
import type { VideoItem } from '../api/video';
import VideoCard from '../components/VideoCard.vue';
import { useAuthStore } from '../stores/auth';

type ProfileTab = 'videos' | 'favorites' | 'history' | 'follows';

const tabs: readonly { id: ProfileTab; label: string }[] = [
  { id: 'videos', label: '投稿' }, { id: 'favorites', label: '收藏' },
  { id: 'history', label: '历史' }, { id: 'follows', label: '关注' },
];
const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const videos = ref<readonly VideoItem[]>([]);
const people = ref<readonly FollowedUser[]>([]);
const total = ref(0);
const pageSize = ref(12);
const loading = ref(false);
const error = ref('');
let activeRequest: AbortController | null = null;

const tab = computed<ProfileTab>(() => tabs.some((item) => item.id === route.query.tab) ? route.query.tab as ProfileTab : 'videos');
const page = computed(() => {
  const value = Number(route.query.page);
  return Number.isInteger(value) && value > 0 ? value : 1;
});
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

async function load(): Promise<void> {
  activeRequest?.abort();
  const controller = new AbortController();
  activeRequest = controller;
  loading.value = true;
  error.value = '';
  videos.value = [];
  people.value = [];
  try {
    const options = { page: page.value, pageSize: 12, signal: controller.signal };
    if (tab.value === 'videos') {
      const result = await videoClient.listMyVideos(options);
      videos.value = result.items; total.value = result.total; pageSize.value = result.page_size;
    } else if (tab.value === 'favorites') {
      const result = await communityClient.listFavorites(options);
      videos.value = result.items; total.value = result.total; pageSize.value = result.page_size;
    } else if (tab.value === 'history') {
      const result = await communityClient.listHistory(options);
      videos.value = result.items; total.value = result.total; pageSize.value = result.page_size;
    } else {
      const result = await communityClient.listFollows(options);
      people.value = result.items; total.value = result.total; pageSize.value = result.page_size;
    }
  } catch (caught) {
    if (controller.signal.aborted || (caught as { code?: string })?.code === 'REQUEST_CANCELLED') return;
    error.value = caught instanceof Error ? caught.message : '个人中心加载失败。';
  } finally {
    if (activeRequest === controller) loading.value = false;
  }
}

async function selectTab(next: ProfileTab): Promise<void> {
  await router.push({ path: '/me', query: { tab: next, page: '1' } });
}

async function changePage(next: number): Promise<void> {
  await router.push({ path: '/me', query: { tab: tab.value, page: String(next) } });
}

watch(() => route.fullPath, () => void load(), { immediate: true });
onUnmounted(() => activeRequest?.abort());
</script>

<template>
  <div class="app-content">
    <section class="app-panel" aria-labelledby="profile-title">
      <div class="panel-heading"><div><span class="panel-kicker">MY ORBIT / LIBRARY</span><h2 id="profile-title" tabindex="-1">{{ auth.user?.nickname || '个人中心' }}</h2><p>管理投稿，也重温喜欢过的片段。</p></div></div>
      <div class="profile-tabs-mount"><div class="profile-tabs" role="tablist" aria-label="个人内容"><button v-for="item in tabs" :key="item.id" class="profile-tab" type="button" role="tab" :aria-selected="tab === item.id" :aria-current="tab === item.id ? 'page' : undefined" @click="selectTab(item.id)">{{ item.label }}</button></div></div>
      <p v-if="error" class="app-message" data-kind="error" role="alert">{{ error }}</p>
      <div v-if="loading" class="empty-state"><p>正在整理你的片段…</p></div>
      <div v-else-if="tab !== 'follows'" class="video-grid"><VideoCard v-for="video in videos" :key="video.id" :video="video" /><div v-if="!videos.length" class="empty-state"><div><span class="empty-mark">🐾</span><strong>这里还是空的</strong><p>{{ tab === 'videos' ? '投稿后会出现在这里。' : tab === 'favorites' ? '收藏的视频会出现在这里。' : '观看历史会出现在这里。' }}</p></div></div></div>
      <ul v-else class="profile-grid"><li v-for="person in people" :key="person.id" class="profile-grid-item"><article class="user-card"><strong class="user-card-name">{{ person.nickname || person.username || '未命名用户' }}</strong><span class="user-card-handle">@{{ person.username || person.id }}</span></article></li><li v-if="!people.length" class="empty-state"><div><span class="empty-mark">🐾</span><strong>还没有关注</strong><p>关注的创作者会出现在这里。</p></div></li></ul>
      <nav class="pagination" aria-label="个人中心分页"><button type="button" :disabled="loading || page <= 1" @click="changePage(page - 1)">上一页</button><span>{{ page }} / {{ totalPages }}</span><button type="button" :disabled="loading || page >= totalPages" @click="changePage(page + 1)">下一页</button></nav>
    </section>
  </div>
</template>
