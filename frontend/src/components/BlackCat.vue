<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue';

defineProps<{ covering?: boolean }>();

const scene = ref<HTMLElement | null>(null);
let frame = 0;

function followPointer(event: PointerEvent): void {
  if (matchMedia('(prefers-reduced-motion: reduce)').matches) return;
  cancelAnimationFrame(frame);
  frame = requestAnimationFrame(() => {
    const root = scene.value;
    if (!root) return;
    const bounds = root.getBoundingClientRect();
    const x = Math.max(-5, Math.min(5, ((event.clientX - bounds.left) / Math.max(bounds.width, 1) - 0.5) * 10));
    const y = Math.max(-4, Math.min(4, ((event.clientY - bounds.top) / Math.max(bounds.height, 1) - 0.5) * 8));
    root.style.setProperty('--look-x', `${x.toFixed(1)}px`);
    root.style.setProperty('--look-y', `${y.toFixed(1)}px`);
  });
}

onMounted(() => window.addEventListener('pointermove', followPointer));
onUnmounted(() => {
  window.removeEventListener('pointermove', followPointer);
  cancelAnimationFrame(frame);
});
</script>

<template>
  <div ref="scene" class="cat-scene" aria-hidden="true">
    <svg class="cat-art" viewBox="0 0 760 760" role="img">
      <g class="stars" fill="#d8f28e">
        <circle cx="105" cy="270" r="3"/><circle cx="630" cy="210" r="2"/><circle cx="586" cy="315" r="3"/>
        <path d="m145 355 4 10 10 4-10 4-4 10-4-10-10-4 10-4Z"/>
      </g>
      <g class="cat-character" :class="{ 'is-covering': covering }">
        <path class="cat-tail" d="M525 608c95 45 141-24 91-72-25-24-53-4-38 17 10 13 27 3 20-8" fill="none" stroke="#111916" stroke-width="36" stroke-linecap="round"/>
        <path d="M329 617c-8-103 20-188 24-253l-20-109 91 67c29-8 58-8 86 0l88-67-17 111c7 67 31 148 19 251Z" fill="#111916"/>
        <path d="m349 292 24-74 48 79m111 0 50-79 18 80" fill="#111916" stroke="#111916" stroke-width="18" stroke-linejoin="round"/>
        <g class="cat-eyes">
          <ellipse cx="420" cy="388" rx="31" ry="25" fill="#d8f28e"/>
          <ellipse cx="524" cy="388" rx="31" ry="25" fill="#d8f28e"/>
          <g class="cat-pupils" fill="#17231d"><ellipse cx="420" cy="389" rx="8" ry="17"/><ellipse cx="524" cy="389" rx="8" ry="17"/></g>
        </g>
        <path d="m465 431 8 7 8-7" fill="none" stroke="#d8f28e" stroke-width="4" stroke-linecap="round"/>
        <path class="cat-paw cat-paw-left" d="M392 604c-2-49 6-82 25-98" fill="none" stroke="#111916" stroke-width="35" stroke-linecap="round"/>
        <path class="cat-paw cat-paw-right" d="M546 604c2-49-6-82-25-98" fill="none" stroke="#111916" stroke-width="35" stroke-linecap="round"/>
      </g>
    </svg>
  </div>
</template>
