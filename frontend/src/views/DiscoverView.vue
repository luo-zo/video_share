<script setup lang="ts">
import { computed, nextTick, onUnmounted, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { taxonomyClient, videoClient } from '../api';
import type { Category } from '../api/taxonomy';
import type { VideoItem } from '../api/video';
import VideoCard from '../components/VideoCard.vue';

const route = useRoute();
const router = useRouter();
const form = reactive({ query: '', sort: 'latest', categoryId: '', tags: '' });
const categories = ref<readonly Category[]>([]);
const items = ref<readonly VideoItem[]>([]);
const total = ref(0);
const pageSize = ref(12);
const loading = ref(false);
const error = ref('');
const title = ref<HTMLElement | null>(null);
let activeRequest: AbortController | null = null;

const page = computed(() => {
  const value = Number(route.query.page);
  return Number.isInteger(value) && value > 0 ? value : 1;
});
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

function queryText(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

async function load(): Promise<void> {
  activeRequest?.abort();
  const controller = new AbortController();
  activeRequest = controller;
  loading.value = true;
  error.value = '';
  form.query = queryText(route.query.q);
  form.sort = ['latest', 'popular'].includes(queryText(route.query.sort)) ? queryText(route.query.sort) : 'latest';
  form.categoryId = queryText(route.query.category_id);
  form.tags = Array.isArray(route.query.tag) ? route.query.tag.join(', ') : queryText(route.query.tag);
  try {
    const result = await videoClient.listVideos({
      query: form.query,
      sort: form.sort,
      page: page.value,
      pageSize: 12,
      categoryId: form.categoryId,
      tags: form.tags.split(',').map((tag) => tag.trim()).filter(Boolean),
      signal: controller.signal,
    });
    if (controller.signal.aborted) return;
    items.value = result.items;
    total.value = result.total;
    pageSize.value = result.page_size;
  } catch (caught) {
    if (controller.signal.aborted || (caught as { code?: string })?.code === 'REQUEST_CANCELLED') return;
    error.value = caught instanceof Error ? caught.message : '发现页加载失败。';
    items.value = [];
  } finally {
    if (activeRequest === controller) loading.value = false;
  }
}

async function submitSearch(): Promise<void> {
  await router.push({
    path: '/',
    query: {
      ...(form.query.trim() ? { q: form.query.trim() } : {}),
      ...(form.sort !== 'latest' ? { sort: form.sort } : {}),
      ...(form.categoryId ? { category_id: form.categoryId } : {}),
      ...(form.tags.trim() ? { tag: form.tags.split(',').map((tag) => tag.trim()).filter(Boolean) } : {}),
      page: '1',
    },
  });
}

async function resetSearch(): Promise<void> {
  form.query = '';
  form.sort = 'latest';
  form.categoryId = '';
  form.tags = '';
  await router.push({ path: '/', query: { page: '1' } });
}

async function changePage(next: number): Promise<void> {
  await router.push({ query: { ...route.query, page: String(Math.max(1, next)) } });
}

watch(() => route.fullPath, () => void load(), { immediate: true });
if (taxonomyClient && typeof taxonomyClient.listCategories === 'function') {
  const categoryRequest = taxonomyClient.listCategories();
  if (categoryRequest && typeof (categoryRequest as Promise<unknown>).then === 'function') {
    void categoryRequest.then((result) => { categories.value = result.items; }).catch(() => { categories.value = []; });
  }
}
nextTick(() => title.value?.focus());
onUnmounted(() => activeRequest?.abort());
</script>

<template>
  <div class="app-content">
    <section class="app-panel" aria-labelledby="discover-title">
      <div class="panel-heading"><div><span class="panel-kicker">DISCOVER / PUBLIC</span><h2 id="discover-title" ref="title" tabindex="-1">发现片段</h2><p>看看大家最近捕捉到的光。</p></div></div>
      <div class="discover-tools">
        <form class="discover-search" role="search" @submit.prevent="submitSearch">
          <label class="sr-only" for="discover-query">搜索视频</label><input id="discover-query" v-model="form.query" class="search-input" name="q" type="search" placeholder="搜索标题或简介">
          <label class="sr-only" for="discover-sort">排序</label><select id="discover-sort" v-model="form.sort" class="search-sort" name="sort"><option value="latest">最新发布</option><option value="popular">最多播放</option></select>
          <label class="sr-only" for="discover-category">分区</label><select id="discover-category" v-model="form.categoryId" class="search-sort" name="category_id"><option value="">全部分区</option><option v-for="category in categories" :key="category.id" :value="String(category.id)">{{ category.name }}</option></select>
          <label class="sr-only" for="discover-tags">标签</label><input id="discover-tags" v-model="form.tags" class="search-input" name="tag" placeholder="标签，用逗号分隔">
          <button class="search-submit" type="submit">搜索</button><button class="search-reset" type="button" :disabled="!form.query && form.sort === 'latest' && !form.categoryId && !form.tags" @click="resetSearch">重置</button>
        </form>
        <p class="discover-summary">{{ form.query ? `“${form.query}” · ` : '' }}{{ total }} 个结果</p>
      </div>
      <p v-if="error" class="app-message" data-kind="error" role="alert">{{ error }}</p>
      <div class="video-grid" :aria-busy="loading">
        <p v-if="loading" class="loading-state">正在寻找好片段…</p>
        <template v-else-if="items.length"><VideoCard v-for="video in items" :key="video.id" :video="video" /></template>
        <div v-else class="empty-state"><div><span class="empty-mark">🐾</span><strong>没有找到视频</strong><p>换个关键词，或者稍后再来看看。</p></div></div>
      </div>
      <nav class="pagination" aria-label="发现页分页"><button data-action="previous-page" type="button" :disabled="loading || page <= 1" @click="changePage(page - 1)">上一页</button><span>{{ page }} / {{ totalPages }}</span><button data-action="next-page" type="button" :disabled="loading || page >= totalPages" @click="changePage(page + 1)">下一页</button></nav>
    </section>
  </div>
</template>
