<script setup lang="ts">
import { onMounted, ref } from 'vue';

import { moderationClient } from '../api';
import type { ReportItem } from '../api/moderation';

const items = ref<readonly ReportItem[]>([]);
const loading = ref(false);
const error = ref('');

async function load(): Promise<void> {
  loading.value = true;
  error.value = '';
  try { items.value = (await moderationClient.listMine()).items; }
  catch (caught) { error.value = caught instanceof Error ? caught.message : '举报记录加载失败。'; }
  finally { loading.value = false; }
}

onMounted(() => void load());
</script>

<template>
  <div class="app-content"><section class="app-panel" aria-labelledby="my-reports-title"><span class="panel-kicker">REPORTS / MINE</span><h2 id="my-reports-title">我的举报</h2><p class="panel-lead">你提交的举报会经过人工核查，处理结果会通过通知送达。</p><p v-if="loading" class="loading-state">正在加载…</p><p v-if="error" class="app-message" data-kind="error" role="alert">{{ error }}</p><ul v-if="!loading && items.length" class="report-list"><li v-for="item in items" :key="item.id" class="report-row"><div><strong>#{{ item.id }} · {{ item.target_type }} #{{ item.target_id }}</strong><p>{{ item.reason_code }} · {{ item.detail || '未补充说明' }}</p></div><span class="status-chip">{{ item.status }}</span></li></ul><div v-else-if="!loading && !error" class="empty-state"><strong>还没有举报记录</strong><p>发现需要处理的内容时，可以在详情页点击“举报”。</p></div></section></div>
</template>
