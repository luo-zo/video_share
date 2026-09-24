<script setup lang="ts">
import { ref } from 'vue';

import { moderationClient } from '../api';
import type { ReportTarget } from '../api/moderation';

const props = defineProps<{ targetType: ReportTarget; targetID: string | number; targetLabel?: string }>();
const emit = defineEmits<{ submitted: [] }>();
const open = ref(false);
const reason = ref('spam');
const detail = ref('');
const busy = ref(false);
const message = ref('');

async function submit(): Promise<void> {
  if (busy.value) return;
  busy.value = true;
  message.value = '';
  try {
    await moderationClient.createReport({ targetType: props.targetType, targetID: props.targetID, reasonCode: reason.value, detail: detail.value });
    message.value = '举报已提交，后续处理结果会通过站内通知告知。';
    detail.value = '';
    emit('submitted');
  } catch (error) {
    message.value = error instanceof Error ? error.message : '举报提交失败，请稍后再试。';
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="report-control">
    <button type="button" class="text-button" @click="open = !open">举报</button>
    <form v-if="open" class="report-form" @submit.prevent="submit">
      <label>举报原因<select v-model="reason"><option value="spam">垃圾信息</option><option value="abuse">骚扰辱骂</option><option value="copyright">版权问题</option><option value="illegal">违法违规</option><option value="other">其他</option></select></label>
      <label>补充说明<textarea v-model="detail" maxlength="500" placeholder="最多 500 个字符"></textarea></label>
      <div class="report-actions"><button type="submit" :disabled="busy">{{ busy ? '提交中…' : '提交举报' }}</button><button type="button" @click="open = false">取消</button></div>
      <p v-if="message" class="app-message" role="status">{{ message }}</p>
    </form>
  </div>
</template>
