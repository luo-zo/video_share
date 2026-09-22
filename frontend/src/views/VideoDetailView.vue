<script setup lang="ts">
import { computed, nextTick, onUnmounted, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { communityClient, videoClient } from '../api';
import { CommunityError, validateComment } from '../api/community';
import type { CommentItem } from '../api/community';
import { VideoError } from '../api/video';
import type { FieldErrors } from '../api/client';
import type { VideoItem } from '../api/video';
import CommentList from '../components/CommentList.vue';
import VideoPlayer from '../components/VideoPlayer.vue';
import { useAuthStore } from '../stores/auth';

const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const video = ref<VideoItem | null>(null);
const comments = ref<readonly CommentItem[]>([]);
const loading = ref(false);
const error = ref('');
const copyMessage = ref('');
const comment = ref('');
const commentError = ref('');
const commenting = ref(false);
const deletingComment = ref<string | number | null>(null);
const relationBusy = ref('');
const liked = ref(false);
const favorited = ref(false);
const following = ref(false);
const likeCount = ref(0);
const favoriteCount = ref(0);
const editing = ref(false);
const confirmingDelete = ref(false);
const ownerBusy = ref(false);
const ownerError = ref('');
const ownerFieldErrors = ref<FieldErrors>({});
const edit = reactive({ title: '', description: '', visibility: 'public' });
const title = ref<HTMLElement | null>(null);
let activeRequest: AbortController | null = null;
let routeGeneration = 0;

const videoId = computed(() => typeof route.params.id === 'string' ? route.params.id : '');
const ownerMode = computed(() => route.query.owner === '1');
const viewerId = computed(() => auth.user?.id);
const isOwner = computed(() => {
  const ownerId = video.value?.user_id ?? video.value?.author?.id;
  return auth.isAuthenticated && ownerId !== undefined && String(ownerId) === String(auth.user?.id);
});
const canUseCommunity = computed(() => video.value?.status === 'ready' && (!ownerMode.value || video.value.visibility === 'public'));

function isCurrent(generation: number, id: string): boolean {
  return generation === routeGeneration && videoId.value === id;
}

function syncVideo(item: VideoItem): void {
  video.value = item;
  liked.value = Boolean(item.viewer_state?.liked);
  favorited.value = Boolean(item.viewer_state?.favorited);
  following.value = Boolean(item.viewer_state?.following_author);
  likeCount.value = Number(item.stats?.like_count) || 0;
  favoriteCount.value = Number(item.stats?.favorite_count) || 0;
  edit.title = typeof item.title === 'string' ? item.title : '';
  edit.description = typeof item.description === 'string' ? item.description : '';
  edit.visibility = typeof item.visibility === 'string' ? item.visibility : 'public';
}

async function load(): Promise<void> {
  const generation = ++routeGeneration;
  activeRequest?.abort();
  const controller = new AbortController();
  activeRequest = controller;
  const id = videoId.value;
  loading.value = true;
  error.value = '';
  copyMessage.value = '';
  video.value = null;
  comments.value = [];
  relationBusy.value = '';
  commenting.value = false;
  deletingComment.value = null;
  editing.value = false;
  confirmingDelete.value = false;
  ownerBusy.value = false;
  ownerError.value = '';
  ownerFieldErrors.value = {};
  try {
    const item = ownerMode.value
      ? await videoClient.getMyVideo(id, { signal: controller.signal })
      : await videoClient.getVideo(id, { signal: controller.signal });
    if (controller.signal.aborted || !isCurrent(generation, id)) return;
    syncVideo(item);

    if (item.status === 'ready' && (!ownerMode.value || item.visibility === 'public')) {
      const commentPage = await communityClient.listComments(id, { page: 1, pageSize: 50, signal: controller.signal });
      if (controller.signal.aborted || !isCurrent(generation, id)) return;
      comments.value = commentPage.items;
    }
    await nextTick();
    if (isCurrent(generation, id)) title.value?.focus();
  } catch (caught) {
    if (controller.signal.aborted || !isCurrent(generation, id) || (caught as { code?: string })?.code === 'REQUEST_CANCELLED') return;
    error.value = caught instanceof Error ? caught.message : '视频详情加载失败。';
  } finally {
    if (activeRequest === controller && isCurrent(generation, id)) loading.value = false;
  }
}

async function requireSession(): Promise<boolean> {
  if (auth.isAuthenticated) return true;
  await router.push({ name: 'login', query: { returnTo: route.fullPath } });
  return false;
}

async function toggleRelation(kind: 'like' | 'favorite' | 'follow'): Promise<void> {
  if (!video.value || !await requireSession()) return;
  const generation = routeGeneration;
  const id = String(video.value.id);
  const nextLiked = !liked.value;
  const nextFavorited = !favorited.value;
  const nextFollowing = !following.value;
  const authorId = video.value.author?.id;
  relationBusy.value = kind;
  error.value = '';
  try {
    if (kind === 'like') {
      const result = await communityClient.setLike(id, nextLiked);
      if (!isCurrent(generation, id)) return;
      liked.value = result.active; likeCount.value = result.count;
    } else if (kind === 'favorite') {
      const result = await communityClient.setFavorite(id, nextFavorited);
      if (!isCurrent(generation, id)) return;
      favorited.value = result.active; favoriteCount.value = result.count;
    } else if (authorId !== undefined) {
      const result = await communityClient.setFollow(authorId, nextFollowing);
      if (!isCurrent(generation, id)) return;
      following.value = result.following;
    }
  } catch (caught) {
    if (!isCurrent(generation, id)) return;
    error.value = caught instanceof Error ? caught.message : '操作失败，请重试。';
  } finally {
    if (isCurrent(generation, id)) relationBusy.value = '';
  }
}

async function addComment(): Promise<void> {
  if (!video.value || !await requireSession()) return;
  const validation = validateComment({ content: comment.value });
  commentError.value = validation.errors.content ?? '';
  if (!validation.valid) return;
  const generation = routeGeneration;
  const id = String(video.value.id);
  const content = comment.value;
  commenting.value = true;
  try {
    const created = await communityClient.addComment(id, { content });
    if (!isCurrent(generation, id)) return;
    comments.value = [created, ...comments.value];
    comment.value = '';
  } catch (caught) {
    if (!isCurrent(generation, id)) return;
    const issue = caught instanceof CommunityError ? caught : new CommunityError('评论发布失败。');
    commentError.value = issue.fieldErrors.content ?? issue.message;
  } finally {
    if (isCurrent(generation, id)) commenting.value = false;
  }
}

async function removeComment(item: CommentItem): Promise<void> {
  if (!video.value) return;
  const generation = routeGeneration;
  const id = String(video.value.id);
  deletingComment.value = item.id;
  try {
    await communityClient.deleteComment(item.id);
    if (!isCurrent(generation, id)) return;
    comments.value = comments.value.filter((entry) => String(entry.id) !== String(item.id));
  } catch (caught) {
    if (!isCurrent(generation, id)) return;
    error.value = caught instanceof Error ? caught.message : '评论删除失败。';
  } finally {
    if (isCurrent(generation, id)) deletingComment.value = null;
  }
}

async function copyPageLink(): Promise<void> {
  const pageUrl = new URL(`/video/${encodeURIComponent(videoId.value)}`, window.location.origin).href;
  try {
    await navigator.clipboard.writeText(pageUrl);
    copyMessage.value = '页面链接已复制。';
  } catch {
    copyMessage.value = '复制失败，请从地址栏复制页面链接。';
  }
}

async function saveOwnerEdit(): Promise<void> {
  if (!video.value) return;
  const generation = routeGeneration;
  const id = String(video.value.id);
  const changes = { ...edit };
  ownerBusy.value = true; ownerError.value = ''; ownerFieldErrors.value = {};
  try {
    const updated = await videoClient.updateVideo(id, changes);
    if (!isCurrent(generation, id)) return;
    syncVideo(updated);
    editing.value = false;
  } catch (caught) {
    if (!isCurrent(generation, id)) return;
    const issue = caught instanceof VideoError ? caught : new VideoError('保存失败，请重试。');
    ownerFieldErrors.value = issue.fieldErrors;
    ownerError.value = issue.message;
  } finally {
    if (isCurrent(generation, id)) ownerBusy.value = false;
  }
}

async function deleteOwnedVideo(): Promise<void> {
  if (!video.value) return;
  const generation = routeGeneration;
  const id = String(video.value.id);
  ownerBusy.value = true;
  try {
    await videoClient.deleteVideo(id);
    if (!isCurrent(generation, id)) return;
    await router.replace('/me');
  } catch (caught) {
    if (!isCurrent(generation, id)) return;
    ownerError.value = caught instanceof Error ? caught.message : '删除失败，请重试。';
  } finally {
    if (isCurrent(generation, id)) ownerBusy.value = false;
  }
}

function reportWatch(payload: { progressMs: number; durationMs: number; keepalive: boolean }): void {
  if (!auth.isAuthenticated || !video.value) return;
  void communityClient.reportWatch(video.value.id, payload).catch(() => undefined);
}

watch([videoId, ownerMode], () => void load(), { immediate: true });
onUnmounted(() => {
  routeGeneration += 1;
  activeRequest?.abort();
});
</script>

<template>
  <div class="app-content">
    <section class="app-panel" aria-labelledby="detail-title">
      <button class="soft-button detail-back" type="button" @click="router.back()">← 返回</button>
      <p v-if="error" class="app-message" data-kind="error" role="alert">{{ error }}</p>
      <div v-if="loading" class="empty-state"><p>正在准备放映…</p></div>
      <template v-else-if="video">
        <article class="detail-card">
          <VideoPlayer :video="video" @error="error = $event" @progress="reportWatch" />
          <div class="detail-copy">
            <div class="detail-meta"><span :class="`status-chip status-${video.status || 'ready'}`">{{ video.status || 'ready' }}</span><span>{{ video.created_at?.slice(0, 10) }}</span></div>
            <h2 id="detail-title" ref="title" tabindex="-1">{{ video.title || '未命名视频' }}</h2>
            <p class="detail-description">{{ video.description || '创作者还没有写简介。' }}</p>
            <p class="detail-author">BY {{ video.author?.nickname || video.author?.username || '匿名创作者' }}</p>
            <p class="detail-media-facts">{{ video.stats?.view_count ?? 0 }} 播放 · {{ video.stats?.comment_count ?? comments.length }} 评论</p>
            <button class="soft-button" type="button" @click="copyPageLink">复制页面链接</button><p class="form-message" aria-live="polite">{{ copyMessage }}</p>
          </div>
        </article>
        <div v-if="canUseCommunity" class="detail-actions"><div class="action-bar">
          <button class="action-button" type="button" :disabled="relationBusy === 'like'" :aria-pressed="liked" @click="toggleRelation('like')">喜欢 <span class="action-count">{{ likeCount }}</span></button>
          <button class="action-button" type="button" :disabled="relationBusy === 'favorite'" :aria-pressed="favorited" @click="toggleRelation('favorite')">收藏 <span class="action-count">{{ favoriteCount }}</span></button>
          <button v-if="!isOwner && video.author?.id" class="action-button action-follow" type="button" :disabled="relationBusy === 'follow'" :aria-pressed="following" @click="toggleRelation('follow')">{{ following ? '已关注' : '关注作者' }}</button>
        </div></div>
        <section v-if="isOwner" class="detail-owner" aria-labelledby="owner-heading">
          <button v-if="!editing && !confirmingDelete" class="soft-button" type="button" @click="editing = true">编辑投稿</button>
          <form v-if="editing" class="owner-edit" @submit.prevent="saveOwnerEdit"><h3 id="owner-heading" class="owner-edit-heading">编辑“{{ video.title }}”</h3><label for="owner-title">标题</label><input id="owner-title" v-model="edit.title" class="owner-title"><p class="field-error">{{ ownerFieldErrors.title }}</p><label for="owner-description">简介</label><textarea id="owner-description" v-model="edit.description" class="owner-description"></textarea><p class="field-error">{{ ownerFieldErrors.description }}</p><label for="owner-visibility">可见性</label><select id="owner-visibility" v-model="edit.visibility" class="owner-visibility"><option value="public">公开</option><option value="private">私密</option></select><p class="field-error">{{ ownerFieldErrors.visibility }}</p><p v-if="ownerError" class="form-message is-error" role="alert">{{ ownerError }}</p><button class="owner-save" type="submit" :disabled="ownerBusy">保存修改</button><button class="owner-delete" type="button" @click="editing = false; confirmingDelete = true">删除投稿</button></form>
          <div v-if="confirmingDelete" class="delete-confirmation" role="alertdialog" aria-modal="true"><strong>确认删除“{{ video.title }}”？</strong><p>删除后无法恢复。</p><button class="confirm-cancel" type="button" @click="confirmingDelete = false">取消</button><button class="confirm-delete" type="button" :disabled="ownerBusy" @click="deleteOwnedVideo">确认删除</button></div>
        </section>
        <section v-if="canUseCommunity" id="comments" class="detail-comments" aria-labelledby="comments-heading"><h3 id="comments-heading" class="comments-heading">评论</h3><form class="comment-composer" @submit.prevent="addComment"><label class="sr-only" for="comment-input">写评论</label><textarea id="comment-input" v-model="comment" class="comment-input" maxlength="500" placeholder="写下你的想法"></textarea><p class="comment-error" aria-live="polite">{{ commentError }}</p><button class="comment-submit" type="submit" :disabled="commenting">{{ commenting ? '发布中…' : '发布评论' }}</button></form><CommentList :comments="comments" :viewer-id="viewerId" :deleting-id="deletingComment" @delete="removeComment" /></section>
      </template>
    </section>
  </div>
</template>
