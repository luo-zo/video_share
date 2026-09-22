<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue';

import type { VideoItem } from '../api/video';
import { attachVideoSource } from '../lib/player';
import type { Detach } from '../lib/player';

const props = defineProps<{ video: VideoItem | null }>();
const emit = defineEmits<{
  error: [message: string];
  progress: [payload: { progressMs: number; durationMs: number; keepalive: boolean }];
}>();

const element = ref<HTMLVideoElement | null>(null);
const error = ref('');
let detach: Detach | null = null;
let generation = 0;
let lastReportedMs = 0;

function cleanup(): void {
  detach?.();
  detach = null;
}

async function attach(video: VideoItem | null): Promise<void> {
    generation += 1;
    const current = generation;
    cleanup();
    error.value = '';
    lastReportedMs = 0;
    if (!video || !element.value) return;
    try {
      const attached = await attachVideoSource(element.value, video, {
        onError: (info) => {
          if (!info.fatal) return;
          error.value = '视频播放中断，请稍后重试。';
          emit('error', error.value);
        },
      });
      if (current !== generation) attached();
      else detach = attached;
    } catch (caught) {
      if (current !== generation) return;
      error.value = caught instanceof Error ? caught.message : '视频无法播放。';
      emit('error', error.value);
    }
}

watch(() => props.video, (video) => void attach(video));
onMounted(() => void attach(props.video));

function report(keepalive = false): void {
  const video = element.value;
  if (!video || !Number.isFinite(video.currentTime) || !Number.isFinite(video.duration)) return;
  const progressMs = Math.max(0, Math.floor(video.currentTime * 1000));
  const durationMs = Math.max(0, Math.floor(video.duration * 1000));
  if (!keepalive && progressMs - lastReportedMs < 15_000) return;
  lastReportedMs = progressMs;
  emit('progress', { progressMs: Math.min(progressMs, durationMs || progressMs), durationMs, keepalive });
}

onUnmounted(() => {
  generation += 1;
  report(true);
  cleanup();
});
</script>

<template>
  <div class="player-wrap">
    <video
      ref="element"
      controls
      playsinline
      preload="metadata"
      :poster="video?.cover_url"
      :aria-label="video?.title ? `播放 ${video.title}` : '视频播放器'"
      @timeupdate="report(false)"
      @pause="report(true)"
      @ended="report(true)"
    />
    <p v-if="error" class="form-message is-error" aria-live="polite">{{ error }}</p>
  </div>
</template>
