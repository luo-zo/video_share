<script setup lang="ts">
import type { CommentItem } from '../api/community';

defineProps<{
  comments: readonly CommentItem[];
  viewerId?: string | number;
  deletingId?: string | number | null;
}>();

defineEmits<{ delete: [comment: CommentItem] }>();

function dateLabel(value: string | undefined): string {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return date.toISOString().slice(0, 16).replace('T', ' ');
}
</script>

<template>
  <ul v-if="comments.length" class="comment-list">
    <li v-for="comment in comments" :key="comment.id" class="comment-item">
      <div class="comment-meta">
        <strong class="comment-author">{{ comment.author?.nickname || comment.author?.username || '匿名用户' }}</strong>
        <time class="comment-time">{{ dateLabel(comment.created_at) }}</time>
      </div>
      <p class="comment-body">{{ comment.content }}</p>
      <button
        v-if="String(comment.user_id ?? comment.author?.id) === String(viewerId)"
        class="comment-delete"
        type="button"
        :disabled="String(deletingId) === String(comment.id)"
        @click="$emit('delete', comment)"
      >删除评论</button>
    </li>
  </ul>
  <div v-else class="comment-empty"><div><strong>还没有评论</strong><p>来留下第一句话吧。</p></div></div>
</template>
