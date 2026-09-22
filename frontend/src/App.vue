<script setup lang="ts">
import { RouterView } from 'vue-router';
</script>

<template>
  <a class="skip-link" href="#main-content">跳到主要内容</a>
  <div class="page-shell">
    <RouterView />
    <footer class="site-footer">
      <span>© 2026 video_share</span>
      <span class="footer-center"><span></span> 让每一帧，都有共鸣。</span>
      <span class="footer-signature">MADE FOR CURIOUS MINDS
        <svg class="icon" aria-hidden="true"><use href="#i-spark" /></svg>
      </span>
    </footer>
  </div>
</template>
