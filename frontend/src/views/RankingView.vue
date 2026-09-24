<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { rankingClient } from '../api';
import type { RankingItem, RankingWindow } from '../api/ranking';
import VideoCard from '../components/VideoCard.vue';

const route = useRoute();
const router = useRouter();
const selectedWindow = ref<RankingWindow>('day');
const items = ref<readonly RankingItem[]>([]);
const total = ref(0);
const pageSize = ref(20);
const generatedAt = ref('');
const loading = ref(false);
const error = ref('');
let activeRequest: AbortController | null = null;

const page = computed(() => {
  const value = Number(route.query.page);
  return Number.isInteger(value) && value > 0 ? value : 1;
});
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
const generatedLabel = computed(() => {
  if (!generatedAt.value) return '尚未生成快照';
  const date = new Date(generatedAt.value);
  if (Number.isNaN(date.getTime())) return '快照时间未知';
  return `快照于 ${new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', dateStyle: 'medium', timeStyle: 'short' }).format(date)}`;
});

async function load(): Promise<void> {
  activeRequest?.abort();
  const controller = new AbortController();
  activeRequest = controller;
  selectedWindow.value = route.query.window === 'week' ? 'week' : 'day';
  loading.value = true;
  error.value = '';
  try {
    const result = await rankingClient.list({ window: selectedWindow.value, page: page.value, pageSize: 20, signal: controller.signal });
    if (controller.signal.aborted) return;
    items.value = result.items;
    total.value = result.total;
    pageSize.value = result.page_size;
    generatedAt.value = result.generated_at;
  } catch (caught) {
    if (controller.signal.aborted || (caught as { code?: string })?.code === 'REQUEST_CANCELLED') return;
    error.value = caught instanceof Error ? caught.message : '榜单加载失败。';
    items.value = [];
  } finally {
    if (activeRequest === controller) loading.value = false;
  }
}

async function changeWindow(window: RankingWindow): Promise<void> {
  await router.push({ query: { window, page: '1' } });
}

async function changePage(next: number): Promise<void> {
  await router.push({ query: { ...route.query, page: String(Math.max(1, next)) } });
}

watch(() => route.fullPath, () => void load(), { immediate: true });
onUnmounted(() => activeRequest?.abort());
</script>

<template>
  <div class="app-content">
    <section class="app-panel ranking-panel" aria-labelledby="ranking-title">
      <div class="panel-heading">
        <div><span class="panel-kicker">RANKING / SNAPSHOT</span><h2 id="ranking-title">热度榜</h2><p>按有效观看与互动指标生成的公开日榜、周榜。</p></div>
        <div class="dashboard-range" role="tablist" aria-label="榜单周期">
          <button type="button" :class="{ 'is-active': selectedWindow === 'day' }" @click="changeWindow('day')">日榜</button>
          <button type="button" :class="{ 'is-active': selectedWindow === 'week' }" @click="changeWindow('week')">周榜</button>
        </div>
      </div>
      <p class="ranking-as-of">{{ generatedLabel }} · 共 {{ total }} 部公开作品</p>
      <p v-if="error" class="app-message" data-kind="error" role="alert">{{ error }}</p>
      <div class="video-grid" :aria-busy="loading">
        <p v-if="loading" class="loading-state">正在整理榜单…</p>
        <template v-else-if="items.length">
          <VideoCard v-for="item in items" :key="item.video_id" :video="item" />
        </template>
        <div v-else class="empty-state"><div><span class="empty-mark">🐾</span><strong>榜单还没有数据</strong><p>指标快照生成后，这里会出现公开作品。</p></div></div>
      </div>
      <nav class="pagination" aria-label="榜单分页"><button type="button" :disabled="loading || page <= 1" @click="changePage(page - 1)">上一页</button><span>{{ page }} / {{ totalPages }}</span><button type="button" :disabled="loading || page >= totalPages" @click="changePage(page + 1)">下一页</button></nav>
    </section>
  </div>
</template>
