<script setup lang="ts">
import { computed } from 'vue';

import type { VideoItem } from '../api/video';

const props = defineProps<{ video: VideoItem; owner?: boolean }>();

const title = computed(() => typeof props.video.title === 'string' && props.video.title ? props.video.title : '未命名视频');
const status = computed(() => typeof props.video.status === 'string' ? props.video.status : 'ready');
const statusLabel = computed(() => ({ ready: '可播放', processing: '处理中', failed: '处理失败', deleted: '已删除' }[status.value] ?? status.value));
const author = computed(() => props.video.author?.nickname || props.video.author?.username || '匿名创作者');
const tone = computed(() => String((Number(props.video.id) % 3 || 3)));
const destination = computed(() => ({
  path: `/video/${props.video.id}`,
  query: props.owner ? { owner: '1' } : {},
}));
</script>

<template>
  <RouterLink class="video-card" :class="{ 'is-waiting': status !== 'ready' }" :to="destination">
    <span class="video-card-art" :data-tone="tone">
      <img v-if="video.cover_url" class="video-card-cover" :src="video.cover_url" alt="" loading="lazy">
      <span class="video-card-number">NO. {{ video.id }}</span>
      <span class="video-card-play" aria-hidden="true">▶</span>
    </span>
    <span class="video-card-content">
      <span class="video-card-meta"><span :class="`status-chip status-${status}`">{{ statusLabel }}</span><span>{{ video.created_at?.slice(0, 10) }}</span></span>
      <strong class="video-card-title">{{ title }}</strong>
      <span v-if="video.description" class="video-card-description">{{ video.description }}</span>
      <span class="video-card-stats">
        {{ video.stats?.view_count ?? 0 }} 播放 · {{ video.stats?.like_count ?? 0 }} 喜欢 · {{ video.stats?.comment_count ?? 0 }} 评论
      </span>
      <span class="video-card-author">BY {{ author }}</span>
    </span>
  </RouterLink>
</template>
