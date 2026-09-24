<script setup lang="ts">
import { reactive, ref } from 'vue';

import { communityClient } from '../api';
import type { CommentItem } from '../api/community';
import ReportDialog from './ReportDialog.vue';

const props = defineProps<{
  videoId: string | number;
  comments: readonly CommentItem[];
  viewerId?: string | number;
  deletingId?: string | number | null;
  deletedIds?: readonly (string | number)[];
}>();

const emit = defineEmits<{ delete: [comment: CommentItem] }>();
const replies = reactive<Record<string, readonly CommentItem[]>>({});
const replyOpen = ref<string | null>(null);
const replyDraft = reactive<Record<string, string>>({});
const replyError = reactive<Record<string, string>>({});
const replyBusy = ref<string | null>(null);

function isDeleted(comment: CommentItem): boolean {
  return Boolean(comment.deleted) || (props.deletedIds ?? []).some((id) => String(id) === String(comment.id));
}

function dateLabel(value: string | undefined): string {
  if (!value) return '';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '' : date.toISOString().slice(0, 16).replace('T', ' ');
}

async function toggleReplies(comment: CommentItem): Promise<void> {
  const key = String(comment.id);
  if (replies[key]) { replyOpen.value = replyOpen.value === key ? null : key; return; }
  if (!communityClient.listReplies) return;
  try {
    const result = await communityClient.listReplies(comment.id, { page: 1, pageSize: 50 });
    replies[key] = result.items;
    replyOpen.value = key;
  } catch {
    replyError[key] = '回复加载失败，请稍后重试。';
  }
}

async function submitReply(comment: CommentItem): Promise<void> {
  const key = String(comment.id);
  const content = (replyDraft[key] || '').trim();
  if (!content) { replyError[key] = '回复内容不能为空。'; return; }
  replyBusy.value = key; replyError[key] = '';
  try {
    const created = await communityClient.addComment(props.videoId, { content, parentId: comment.id });
    replies[key] = [...(replies[key] || []), created];
    replyDraft[key] = '';
  } catch (caught) {
    replyError[key] = caught instanceof Error ? caught.message : '回复发布失败。';
  } finally { if (replyBusy.value === key) replyBusy.value = null; }
}
</script>

<template>
  <ul v-if="comments.length" class="comment-list">
    <li v-for="comment in comments" :key="comment.id" class="comment-item" :class="{ 'comment-deleted': isDeleted(comment) }">
      <div class="comment-meta"><RouterLink v-if="comment.author?.id" class="comment-author" :to="`/creator/${comment.author.id}`">{{ comment.author?.nickname || comment.author?.username || '匿名用户' }}</RouterLink><strong v-else class="comment-author">{{ comment.author?.nickname || comment.author?.username || '匿名用户' }}</strong><time class="comment-time">{{ dateLabel(comment.created_at) }}</time></div>
      <p class="comment-body">{{ isDeleted(comment) ? '评论已删除' : comment.content }}</p>
      <div class="comment-controls"><button class="soft-button" type="button" @click="toggleReplies(comment)">{{ replies[String(comment.id)] ? (replyOpen === String(comment.id) ? '收起回复' : '查看回复') : '查看/回复' }}</button><ReportDialog v-if="viewerId && !isDeleted(comment)" target-type="comment" :target-i-d="comment.id" /><button v-if="!isDeleted(comment) && String(comment.user_id ?? comment.author?.id) === String(viewerId)" class="comment-delete" type="button" :disabled="String(deletingId) === String(comment.id)" @click="emit('delete', comment)">删除评论</button></div>
      <form v-if="replyOpen === String(comment.id) && viewerId && !isDeleted(comment)" class="reply-composer" @submit.prevent="submitReply(comment)"><label class="sr-only" :for="`reply-${comment.id}`">回复评论</label><textarea :id="`reply-${comment.id}`" v-model="replyDraft[String(comment.id)]" maxlength="500" placeholder="写一条回复"></textarea><p v-if="replyError[String(comment.id)]" class="comment-error">{{ replyError[String(comment.id)] }}</p><button class="soft-button" type="submit" :disabled="replyBusy === String(comment.id)">回复</button></form>
      <ul v-if="replyOpen === String(comment.id) && replies[String(comment.id)]?.length" class="reply-list"><li v-for="reply in replies[String(comment.id)]" :key="reply.id" class="reply-item" :class="{ 'comment-deleted': isDeleted(reply) }"><div class="comment-meta"><strong class="comment-author">{{ reply.author?.nickname || reply.author?.username || '匿名用户' }}</strong><time class="comment-time">{{ dateLabel(reply.created_at) }}</time></div><p class="comment-body">{{ isDeleted(reply) ? '评论已删除' : reply.content }}</p><ReportDialog v-if="viewerId && !isDeleted(reply)" target-type="comment" :target-i-d="reply.id" /><button v-if="!isDeleted(reply) && String(reply.user_id ?? reply.author?.id) === String(viewerId)" class="comment-delete" type="button" @click="emit('delete', reply)">删除回复</button></li></ul>
    </li>
  </ul>
  <div v-else class="comment-empty"><div><strong>还没有评论</strong><p>来留下第一句话吧。</p></div></div>
</template>
