<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';

import { analyticsClient } from '../api';
import type { CreatorSummary } from '../api/analytics';

const summary = ref<CreatorSummary | null>(null);
const days = ref<7 | 30>(7);
const loading = ref(false);
const error = ref('');

const latestTrend = computed(() => summary.value?.trend.at(-1) ?? null);
const formatShanghaiDate = (value: string): string => {
  const parts = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(new Date(value));
  const part = (type: string): string => parts.find((item) => item.type === type)?.value ?? '';
  return `${part('year')}-${part('month')}-${part('day')}`;
};
const formatDuration = (milliseconds: number): string => {
  const seconds = Math.round(Math.max(0, milliseconds) / 1000);
  if (seconds < 60) return `${seconds} 秒`;
  const minutes = Math.floor(seconds / 60);
  return `${minutes} 分 ${seconds % 60} 秒`;
};

async function load(nextDays: 7 | 30 = days.value): Promise<void> {
  days.value = nextDays;
  loading.value = true;
  error.value = '';
  try {
    summary.value = await analyticsClient.creatorSummary(nextDays);
  } catch (caught) {
    summary.value = null;
    error.value = caught instanceof Error ? caught.message : '数据中心加载失败，请稍后重试。';
  } finally {
    loading.value = false;
  }
}

onMounted(() => void load());
</script>

<template>
  <div class="app-content">
    <section class="app-panel creator-dashboard" aria-labelledby="creator-dashboard-title">
      <div class="panel-heading">
        <div>
          <span class="panel-kicker">CREATOR / ANALYTICS</span>
          <h2 id="creator-dashboard-title">创作者数据中心</h2>
          <p>有效观看按服务器确认的播放心跳统计，不把拖动或倍速直接当作观看时长。</p>
        </div>
        <div class="dashboard-range" role="group" aria-label="统计范围">
          <button type="button" :class="{ 'is-active': days === 7 }" :disabled="loading" @click="load(7)">近 7 天</button>
          <button type="button" :class="{ 'is-active': days === 30 }" :disabled="loading" @click="load(30)">近 30 天</button>
        </div>
      </div>

      <p v-if="error" class="app-message" data-kind="error" role="alert">{{ error }} <button class="text-button" type="button" @click="load()">重试</button></p>
      <p v-if="loading" class="loading-state">正在整理播放数据…</p>

      <template v-if="summary && !loading">
        <div class="dashboard-kpis" aria-label="当前累计数据">
          <article><span>公开作品</span><strong>{{ summary.current.video_count }}</strong></article>
          <article><span>累计播放</span><strong>{{ summary.current.view_count }}</strong></article>
          <article><span>喜欢</span><strong>{{ summary.current.like_count }}</strong></article>
          <article><span>收藏</span><strong>{{ summary.current.favorite_count }}</strong></article>
          <article><span>评论</span><strong>{{ summary.current.comment_count }}</strong></article>
          <article><span>粉丝</span><strong>{{ summary.current.follower_count }}</strong></article>
        </div>

        <section class="dashboard-section" aria-labelledby="dashboard-trend-title">
          <div class="dashboard-section-heading"><h3 id="dashboard-trend-title">每日趋势</h3><span>截至 {{ formatShanghaiDate(summary.as_of) }}</span></div>
          <div v-if="summary.trend.length" class="dashboard-table-wrap">
            <table class="dashboard-table"><thead><tr><th>日期</th><th>有效播放</th><th>观看时长</th><th>完播</th><th>净互动</th></tr></thead><tbody><tr v-for="point in summary.trend" :key="point.date"><td>{{ point.date }}</td><td>{{ point.effective_views }}</td><td>{{ formatDuration(point.watch_time_ms) }}</td><td>{{ point.completions }}</td><td>{{ point.net_likes + point.net_favorites + point.net_comments + point.net_followers }}</td></tr></tbody></table>
          </div>
          <p v-else class="dashboard-empty">这个时间范围还没有有效播放数据。</p>
        </section>

        <section class="dashboard-section" aria-labelledby="dashboard-top-title">
          <div class="dashboard-section-heading"><h3 id="dashboard-top-title">作品排行</h3><span>按有效播放排序 · Top 10</span></div>
          <ol v-if="summary.top_videos.length" class="dashboard-top-list"><li v-for="item in summary.top_videos" :key="item.video_id"><RouterLink :to="`/video/${item.video_id}`"><strong>{{ item.title || '未命名视频' }}</strong><span>{{ item.effective_views }} 有效播放 · {{ formatDuration(item.watch_time_ms) }} · {{ item.completions }} 次完播</span></RouterLink></li></ol>
          <p v-else class="dashboard-empty">还没有可排行的公开作品。</p>
        </section>

        <p v-if="latestTrend" class="dashboard-footnote">最近一天：{{ latestTrend.date }} · 净关注 {{ latestTrend.net_followers >= 0 ? '+' : '' }}{{ latestTrend.net_followers }}</p>
      </template>
    </section>
  </div>
</template>
