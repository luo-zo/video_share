<script setup lang="ts">
import { onUnmounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { videoClient } from '../api';
import type { VideoItem } from '../api/video';
import VideoCard from '../components/VideoCard.vue';

const route = useRoute();
const router = useRouter();
const items = ref<readonly VideoItem[]>([]);
const total = ref(0);
const pageSize = ref(12);
const loading = ref(false);
const error = ref('');
let activeRequest: AbortController | null = null;
let generation = 0;

const page = (): number => {
  const value = Number(route.query.page);
  return Number.isInteger(value) && value > 0 ? value : 1;
};
const totalPages = (): number => Math.max(1, Math.ceil(total.value / pageSize.value));

async function load(): Promise<void> {
  const current = ++generation;
  activeRequest?.abort();
  const controller = new AbortController();
  activeRequest = controller;
  loading.value = true;
  error.value = '';
  try {
    const result = await videoClient.listFollowing({ page: page(), pageSize: 12, signal: controller.signal });
    if (current !== generation || controller.signal.aborted) return;
    items.value = result.items; total.value = result.total; pageSize.value = result.page_size;
  } catch (caught) {
    if (current !== generation || controller.signal.aborted) return;
    error.value = caught instanceof Error ? caught.message : '关注流加载失败。';
    items.value = [];
  } finally {
    if (current === generation && activeRequest === controller) loading.value = false;
  }
}

async function changePage(next: number): Promise<void> {
  await router.push({ query: { ...route.query, page: String(Math.max(1, next)) } });
}

watch(() => route.fullPath, () => void load(), { immediate: true });
onUnmounted(() => { generation += 1; activeRequest?.abort(); });
</script>

<template>
  <div class="app-content">
    <section class="app-panel" aria-labelledby="following-title">
      <div class="panel-heading"><div><span class="panel-kicker">FOLLOWING / PERSONAL</span><h2 id="following-title">关注流</h2><p>只看你关注的正常创作者公开作品。</p></div></div>
      <p v-if="error" class="app-message" data-kind="error" role="alert">{{ error }}</p>
      <div class="video-grid" :aria-busy="loading">
        <p v-if="loading" class="loading-state">正在收集关注动态…</p>
        <template v-else-if="items.length"><VideoCard v-for="video in items" :key="video.id" :video="video" /></template>
        <div v-else class="empty-state"><div><span class="empty-mark">🐾</span><strong>关注流还是空的</strong><p>去发现页认识一些创作者吧。</p></div></div>
      </div>
      <nav class="pagination" aria-label="关注流分页"><button type="button" :disabled="loading || page() <= 1" @click="changePage(page() - 1)">上一页</button><span>{{ page() }} / {{ totalPages() }}</span><button type="button" :disabled="loading || page() >= totalPages()" @click="changePage(page() + 1)">下一页</button></nav>
    </section>
  </div>
</template>
