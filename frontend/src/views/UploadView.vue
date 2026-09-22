<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';

import { videoClient } from '../api';
import { VideoError, validateVideoSubmission } from '../api/video';
import type { UploadStep } from '../api/video';

const router = useRouter();
const form = reactive<{ title: string; description: string; file: File | null }>({ title: '', description: '', file: null });
const errors = reactive<Record<string, string>>({});
const message = ref('');
const busy = ref(false);
const step = ref<UploadStep | ''>('');
const titleInput = ref<HTMLInputElement | null>(null);
const descriptionInput = ref<HTMLTextAreaElement | null>(null);
const fileInput = ref<HTMLInputElement | null>(null);
const heading = ref<HTMLElement | null>(null);
let operation: AbortController | null = null;

const steps: readonly { id: UploadStep; label: string; hint: string }[] = [
  { id: 'creating', label: '创建投稿', hint: '向服务端登记基本信息' },
  { id: 'uploading', label: '直传视频', hint: '安全上传到对象存储' },
  { id: 'completing', label: '确认文件', hint: '核对文件大小与类型' },
  { id: 'processing', label: '等待处理', hint: '生成适合播放的版本' },
];

function setErrors(next: Readonly<Record<string, string>>): void {
  for (const key of Object.keys(errors)) delete errors[key];
  Object.assign(errors, next);
}

function selectFile(event: Event): void {
  form.file = (event.target as HTMLInputElement).files?.[0] ?? null;
  delete errors.file;
}

async function focusFirstError(): Promise<void> {
  await nextTick();
  if (errors.title) titleInput.value?.focus();
  else if (errors.description) descriptionInput.value?.focus();
  else if (errors.file) fileInput.value?.focus();
}

async function submit(): Promise<void> {
  message.value = '';
  const validation = validateVideoSubmission(form);
  setErrors(validation.errors);
  if (!validation.valid) {
    await focusFirstError();
    return;
  }
  operation?.abort();
  const controller = new AbortController();
  operation = controller;
  busy.value = true;
  try {
    let item = await videoClient.uploadVideo(form, {
      signal: controller.signal,
      onStep: (next) => { step.value = next; },
    });
    if (item.status === 'processing') {
      step.value = 'processing';
      item = await videoClient.waitUntilProcessed(item.id, {
        signal: controller.signal,
        onUpdate: (next) => { message.value = `视频处理中：${Number(next.processing_progress) || 0}%`; },
      });
    }
    if (controller.signal.aborted) return;
    step.value = 'complete';
    await router.push(item.status === 'ready' ? `/video/${item.id}` : '/me');
  } catch (caught) {
    if (controller.signal.aborted || (caught as { code?: string })?.code === 'REQUEST_CANCELLED') return;
    const error = caught instanceof VideoError ? caught : new VideoError('投稿未能完成，请稍后重试。');
    setErrors(error.fieldErrors);
    message.value = error.message;
    await focusFirstError();
  } finally {
    if (operation === controller) busy.value = false;
  }
}

onMounted(() => void nextTick(() => heading.value?.focus()));
onUnmounted(() => operation?.abort());
</script>

<template>
  <div class="app-content">
    <section class="app-panel" aria-labelledby="upload-title">
      <div class="panel-heading"><div><span class="panel-kicker">UPLOAD / CREATOR</span><h2 id="upload-title" ref="heading" tabindex="-1">分享新片段</h2><p>仅支持 MP4，最大 500 MiB。</p></div></div>
      <form class="upload-form" novalidate @submit.prevent="submit">
        <div class="upload-fields">
          <div class="field-group" :class="{ 'is-invalid': errors.title }"><label for="upload-title-input">标题 <span>1–100 个字符</span></label><input id="upload-title-input" ref="titleInput" v-model="form.title" name="title" type="text" maxlength="100"><p class="field-error">{{ errors.title }}</p></div>
          <div class="field-group" :class="{ 'is-invalid': errors.description }"><label for="upload-description">简介 <span>最多 2000 个字符</span></label><textarea id="upload-description" ref="descriptionInput" v-model="form.description" name="description" maxlength="2000"></textarea><p class="field-error">{{ errors.description }}</p></div>
          <div class="field-group" :class="{ 'is-invalid': errors.file }"><label for="upload-file">视频文件</label><label class="file-picker" for="upload-file"><input id="upload-file" ref="fileInput" name="file" type="file" accept="video/mp4,.mp4" @change="selectFile"><span>选择 MP4</span><strong>{{ form.file?.name || '尚未选择文件' }}</strong></label><p class="field-error">{{ errors.file }}</p></div>
          <p v-if="message" class="form-message is-error" role="alert">{{ message }}</p>
        </div>
        <aside class="upload-journey"><span class="panel-kicker">PUBLISH JOURNEY</span><ol><li v-for="(item, index) in steps" :key="item.id" :class="{ 'is-active': step === item.id, 'is-complete': step && steps.findIndex((entry) => entry.id === step) > index || step === 'complete' }"><span>0{{ index + 1 }}</span><div><strong>{{ item.label }}</strong><small>{{ item.hint }}</small></div></li></ol><button class="submit-button" type="submit" :disabled="busy" :aria-busy="busy"><span>{{ busy ? '正在投稿…' : '开始投稿' }}</span><span class="submit-icon">→</span></button></aside>
      </form>
    </section>
  </div>
</template>
