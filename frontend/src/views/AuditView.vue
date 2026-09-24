<script setup lang="ts">
import { onMounted, ref } from 'vue';

import { moderationClient } from '../api';
import type { ModerationActionItem } from '../api/moderation';

const items = ref<readonly ModerationActionItem[]>([]);
const loading = ref(false);
const error = ref('');

async function load(): Promise<void> {
  loading.value = true;
  error.value = '';
  try { items.value = (await moderationClient.listActions()).items; }
  catch (caught) { error.value = caught instanceof Error ? caught.message : '审计记录加载失败。'; }
  finally { loading.value = false; }
}

onMounted(() => void load());
</script>

<template>
  <div class="app-content"><section class="app-panel" aria-labelledby="audit-title"><span class="panel-kicker">MODERATION / AUDIT</span><h2 id="audit-title">处置审计</h2><p class="panel-lead">这里展示管理员操作的前后状态，不展示举报详情之外的敏感凭证。</p><p v-if="loading" class="loading-state">正在加载…</p><p v-if="error" class="app-message" data-kind="error" role="alert">{{ error }}</p><ul v-if="!loading && items.length" class="report-list"><li v-for="item in items" :key="item.id" class="report-row"><div><strong>#{{ item.id }} · {{ item.action }}</strong><p>{{ item.target_type }} #{{ item.target_id }} · {{ item.before_state }} → {{ item.after_state }}</p></div><time>{{ item.created_at.slice(0, 16).replace('T', ' ') }}</time></li></ul><div v-else-if="!loading && !error" class="empty-state"><strong>还没有处置记录</strong></div></section></div>
</template>
