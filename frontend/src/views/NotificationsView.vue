<script setup lang="ts">
import { onMounted } from 'vue';

import { useNotificationsStore } from '../stores/notifications';

const notifications = useNotificationsStore();

function label(type: string): string {
  if (type === 'reply') return '有人回复了你的评论';
  if (type === 'comment') return '你的视频收到新评论';
  if (type === 'follow') return '有人关注了你';
  return '你收到一条通知';
}

function dateLabel(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '' : date.toISOString().slice(0, 16).replace('T', ' ');
}

onMounted(() => void notifications.refresh());
</script>

<template>
  <div class="app-content">
    <section class="app-panel" aria-labelledby="notifications-title">
      <div class="panel-heading"><div><span class="panel-kicker">INBOX / EVENTS</span><h2 id="notifications-title">站内通知</h2><p>通知只保存结构化事件，目标失效时不会泄露原始内容。</p></div><button class="soft-button" type="button" :disabled="!notifications.unreadCount" @click="notifications.markAllRead()">全部已读</button></div>
      <p v-if="notifications.error" class="app-message" data-kind="error" role="alert">{{ notifications.error }}</p>
      <div v-if="notifications.loading && !notifications.items.length" class="empty-state"><p>正在读取通知…</p></div>
      <ul v-else-if="notifications.items.length" class="notification-list">
        <li v-for="item in notifications.items" :key="item.id" class="notification-item" :class="{ 'is-unread': !item.read_at }">
          <div><strong>{{ label(item.type) }}</strong><time>{{ dateLabel(item.created_at) }}</time></div>
          <p v-if="item.video_id"><RouterLink :to="`/video/${item.video_id}`">打开相关视频</RouterLink></p>
          <p v-else>打开通知查看详情。</p>
          <button v-if="!item.read_at" class="soft-button" type="button" @click="notifications.markRead(item.id)">标为已读</button>
        </li>
      </ul>
      <div v-else class="empty-state"><p>还没有通知。</p></div>
    </section>
  </div>
</template>
