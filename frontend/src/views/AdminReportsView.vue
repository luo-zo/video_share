<script setup lang="ts">
import { onMounted, ref } from 'vue';

import { moderationClient } from '../api';
import type { ReportItem } from '../api/moderation';

const items = ref<ReportItem[]>([]);
const loading = ref(false);
const error = ref('');
const busyID = ref<string | number | null>(null);

async function load(): Promise<void> {
  loading.value = true;
  error.value = '';
  try { items.value = [...(await moderationClient.listAdmin()).items]; }
  catch (caught) { error.value = caught instanceof Error ? caught.message : '治理工作台加载失败。'; }
  finally { loading.value = false; }
}

function actionFor(item: ReportItem): string {
  if (item.target_type === 'video') return 'hide_video';
  if (item.target_type === 'comment') return 'hide_comment';
  return 'disable_user';
}

async function assign(item: ReportItem): Promise<void> {
  busyID.value = item.id;
  try { const updated = await moderationClient.assign(item.id); Object.assign(item, updated); }
  catch (caught) { error.value = caught instanceof Error ? caught.message : '接单失败。'; }
  finally { busyID.value = null; }
}

async function decide(item: ReportItem, status: 'resolved' | 'rejected'): Promise<void> {
  busyID.value = item.id;
  try {
    const updated = await moderationClient.decide(item.id, { status, resolutionReason: status === 'resolved' ? '经核查，执行页面处置。' : '证据不足，驳回本次举报。', action: status === 'resolved' ? actionFor(item) : undefined, reason: status === 'resolved' ? '违反社区规范' : undefined });
    Object.assign(item, updated);
  } catch (caught) { error.value = caught instanceof Error ? caught.message : '结案失败。'; }
  finally { busyID.value = null; }
}

onMounted(() => void load());
</script>

<template>
  <div class="app-content"><section class="app-panel" aria-labelledby="admin-reports-title"><span class="panel-kicker">MODERATION / INBOX</span><h2 id="admin-reports-title">举报工作台</h2><p class="panel-lead">每次接单、结案和内容处置都会写入追加式审计。</p><p v-if="loading" class="loading-state">正在加载…</p><p v-if="error" class="app-message" data-kind="error" role="alert">{{ error }}</p><ul v-if="!loading && items.length" class="report-list"><li v-for="item in items" :key="item.id" class="report-row"><div><strong>#{{ item.id }} · {{ item.target_type }} #{{ item.target_id }}</strong><p>{{ item.reason_code }} · {{ item.detail || '未补充说明' }} · {{ item.status }}</p></div><div class="report-actions"><button v-if="item.status === 'open'" type="button" :disabled="busyID === item.id" @click="assign(item)">接单</button><button v-if="item.status === 'open' || item.status === 'investigating'" type="button" :disabled="busyID === item.id" @click="decide(item, 'resolved')">处置结案</button><button v-if="item.status === 'open' || item.status === 'investigating'" type="button" :disabled="busyID === item.id" @click="decide(item, 'rejected')">驳回</button></div></li></ul><div v-else-if="!loading && !error" class="empty-state"><strong>暂无待处理举报</strong><p>当前没有需要人工核查的内容。</p></div></section></div>
</template>
