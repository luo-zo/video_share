<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { communityClient, creatorClient } from '../api';
import type { CreatorProfile, CreatorRelation } from '../api/creator';
import type { VideoItem } from '../api/video';
import CreatorCard from '../components/CreatorCard.vue';
import VideoCard from '../components/VideoCard.vue';
import { useAuthStore } from '../stores/auth';

type Tab = 'videos' | 'followers' | 'following';
const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const profile = ref<CreatorProfile | null>(null);
const videos = ref<readonly VideoItem[]>([]);
const people = ref<readonly CreatorRelation[]>([]);
const tab = ref<Tab>('videos');
const page = ref(1);
const total = ref(0);
const pageSize = ref(20);
const loading = ref(false);
const error = ref('');
const followBusy = ref(false);
const title = ref<HTMLElement | null>(null);
let activeRequest: AbortController | null = null;
let listRequest: AbortController | null = null;
let requestGeneration = 0;

const creatorID = computed(() => String(route.params.id || ''));
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
const isSelf = computed(() => Boolean(profile.value && String(profile.value.id) === String(auth.user?.id)));

async function load(): Promise<void> {
  const generation = ++requestGeneration;
  activeRequest?.abort();
  listRequest?.abort();
  const controller = new AbortController();
  activeRequest = controller;
  loading.value = true;
  error.value = '';
  profile.value = null;
  videos.value = [];
  people.value = [];
  page.value = 1;
  tab.value = 'videos';
  try {
    profile.value = await creatorClient.getProfile(creatorID.value, { signal: controller.signal });
    const result = await creatorClient.listVideos(creatorID.value, { page: page.value, pageSize: pageSize.value, signal: controller.signal });
    if (generation !== requestGeneration || controller.signal.aborted) return;
    videos.value = result.items;
    total.value = result.total;
    pageSize.value = result.page_size;
    await nextTick();
    title.value?.focus();
  } catch (caught) {
    if (generation !== requestGeneration || controller.signal.aborted || (caught as { code?: string })?.code === 'REQUEST_CANCELLED') return;
    error.value = caught instanceof Error ? caught.message : '创作者主页加载失败。';
  } finally {
    if (activeRequest === controller && generation === requestGeneration) loading.value = false;
  }
}

async function selectTab(next: Tab): Promise<void> {
  const generation = ++requestGeneration;
  listRequest?.abort();
  const controller = new AbortController();
  listRequest = controller;
  tab.value = next;
  page.value = 1;
  loading.value = true;
  error.value = '';
  try {
    if (next === 'videos') {
      const result = await creatorClient.listVideos(creatorID.value, { page: 1, pageSize: pageSize.value, signal: controller.signal });
      if (generation !== requestGeneration || controller.signal.aborted) return;
      videos.value = result.items; total.value = result.total; pageSize.value = result.page_size;
      return;
    }
    const result = next === 'followers'
      ? await creatorClient.listFollowers(creatorID.value, { page: 1, pageSize: pageSize.value, signal: controller.signal })
      : await creatorClient.listFollowing(creatorID.value, { page: 1, pageSize: pageSize.value, signal: controller.signal });
    if (generation !== requestGeneration || controller.signal.aborted) return;
    people.value = result.items; total.value = result.total; pageSize.value = result.page_size;
  } catch (caught) {
    if (controller.signal.aborted || generation !== requestGeneration) return;
    error.value = caught instanceof Error ? caught.message : '创作者关系加载失败。';
  } finally {
    if (generation === requestGeneration) loading.value = false;
  }
}

async function changePage(next: number): Promise<void> {
  const generation = ++requestGeneration;
  listRequest?.abort();
  const controller = new AbortController();
  listRequest = controller;
  const target = Math.max(1, next);
  page.value = target;
  loading.value = true;
  error.value = '';
  try {
    if (tab.value === 'videos') {
      const result = await creatorClient.listVideos(creatorID.value, { page: target, pageSize: pageSize.value, signal: controller.signal });
      if (generation !== requestGeneration || controller.signal.aborted) return;
      videos.value = result.items; total.value = result.total;
      return;
    }
    const result = tab.value === 'followers'
      ? await creatorClient.listFollowers(creatorID.value, { page: target, pageSize: pageSize.value, signal: controller.signal })
      : await creatorClient.listFollowing(creatorID.value, { page: target, pageSize: pageSize.value, signal: controller.signal });
    if (generation !== requestGeneration || controller.signal.aborted) return;
    people.value = result.items; total.value = result.total;
  } catch (caught) {
    if (controller.signal.aborted || generation !== requestGeneration) return;
    error.value = caught instanceof Error ? caught.message : '创作者分页加载失败。';
  } finally {
    if (generation === requestGeneration) loading.value = false;
  }
}

async function toggleFollow(): Promise<void> {
  if (!profile.value) return;
  if (!auth.isAuthenticated) {
    await router.push({ name: 'login', query: { returnTo: route.fullPath } });
    return;
  }
  if (followBusy.value) return;
  const targetID = String(profile.value.id);
  const generation = requestGeneration;
  const next = !profile.value.following;
  followBusy.value = true;
  try {
    const result = await communityClient.setFollow(profile.value.id, next);
    if (generation === requestGeneration && profile.value && String(profile.value.id) === targetID) {
      const delta = result.following === profile.value.following ? 0 : (result.following ? 1 : -1);
      profile.value = { ...profile.value, following: result.following, follower_count: Math.max(0, profile.value.follower_count + delta) };
    }
  } catch (caught) {
    if (generation === requestGeneration) error.value = caught instanceof Error ? caught.message : '关注操作失败。';
  } finally {
    followBusy.value = false;
  }
}

watch(creatorID, () => void load(), { immediate: true });
onUnmounted(() => { requestGeneration += 1; activeRequest?.abort(); listRequest?.abort(); });
</script>

<template>
  <div class="app-content">
    <section class="app-panel creator-page" aria-labelledby="creator-title">
      <p v-if="loading" class="loading-state">正在打开创作者的小宇宙…</p>
      <p v-if="error" class="app-message" data-kind="error" role="alert">{{ error }}</p>
      <template v-if="profile">
        <div class="creator-hero">
          <div class="creator-hero-avatar" aria-hidden="true">🐈‍⬛</div>
          <div class="creator-hero-copy"><span class="panel-kicker">CREATOR / PUBLIC</span><h2 id="creator-title" ref="title" tabindex="-1">{{ profile.nickname || profile.username }}</h2><p>@{{ profile.username }} · {{ profile.bio || '这个创作者还没有写简介。' }}</p><div class="creator-metrics"><span><strong>{{ profile.video_count }}</strong> 作品</span><span><strong>{{ profile.follower_count }}</strong> 粉丝</span><span><strong>{{ profile.following_count }}</strong> 关注</span></div></div>
          <button v-if="!isSelf" class="action-button" type="button" :disabled="followBusy" :aria-pressed="profile.following" @click="toggleFollow">{{ followBusy ? '处理中…' : profile.following ? '已关注' : '关注' }}</button>
        </div>
        <div class="profile-tabs" role="tablist" aria-label="创作者内容"><button v-for="item in ([['videos', '作品'], ['followers', '粉丝'], ['following', '关注']] as const)" :key="item[0]" class="profile-tab" type="button" role="tab" :aria-selected="tab === item[0]" @click="selectTab(item[0])">{{ item[1] }}</button></div>
        <div v-if="tab === 'videos'" class="video-grid"><VideoCard v-for="video in videos" :key="video.id" :video="video" /><div v-if="!videos.length" class="empty-state"><div><span class="empty-mark">🐾</span><strong>还没有公开作品</strong><p>这个创作者暂时没有可展示的视频。</p></div></div></div>
        <ul v-else class="profile-grid"><li v-for="person in people" :key="person.id" class="profile-grid-item"><CreatorCard :creator="person" /></li><li v-if="!people.length" class="empty-state"><div><span class="empty-mark">🐾</span><strong>这里还是空的</strong><p>暂时没有公开关系记录。</p></div></li></ul>
        <nav class="pagination" aria-label="创作者分页"><button type="button" :disabled="loading || page <= 1" @click="changePage(page - 1)">上一页</button><span>{{ page }} / {{ totalPages }}</span><button type="button" :disabled="loading || page >= totalPages" @click="changePage(page + 1)">下一页</button></nav>
      </template>
    </section>
  </div>
</template>
