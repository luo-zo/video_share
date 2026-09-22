<script setup lang="ts">
import { computed } from 'vue';
import { RouterView, useRoute } from 'vue-router';

import AppHeader from './components/AppHeader.vue';
import BlackCat from './components/BlackCat.vue';

const route = useRoute();
const appMode = computed(() => route.name !== 'login');
</script>

<template>
  <a class="skip-link" href="#main-content">跳到主要内容</a>
  <div class="page-shell" :class="{ 'app-mode': appMode }">
    <header v-if="!appMode" class="site-header">
      <RouterLink class="brand" to="/">
        <span class="brand-icon" aria-hidden="true">🐈‍⬛</span>
        <span class="brand-word">VIDEO<span>SHARE</span><sup>05</sup></span>
        <span class="brand-divider"></span><span class="brand-tagline">好奇心，即刻放映</span>
      </RouterLink>
      <div class="header-invite"><span>先逛逛大家的作品</span><RouterLink class="text-button" to="/">进入发现页 →</RouterLink></div>
    </header>
    <main id="main-content" class="main-layout" tabindex="-1">
      <aside v-if="!appMode" class="universe">
        <div class="universe-grain"></div>
        <div class="hero-topline"><span class="orbit-badge">● CAT ORBIT / 05</span><span class="hero-edition">CURIOUS MINDS</span></div>
        <div class="hero-heading"><p class="eyebrow">A SMALL SCREEN FOR BIG CURIOSITY</p><h1>让每一帧，<br>都有<span class="highlight-word">共鸣</span>。</h1><p class="hero-subtitle">收藏生活里闪光的片段，<br>也遇见别人的小宇宙。</p></div>
        <BlackCat />
      </aside>
      <section class="login-section">
        <div v-if="appMode" class="account-view">
          <AppHeader />
          <RouterView />
        </div>
        <RouterView v-else />
      </section>
    </main>
    <footer class="site-footer">
      <span>© 2026 video_share</span>
      <span class="footer-center"><span></span> 让每一帧，都有共鸣。</span>
      <span class="footer-signature">MADE FOR CURIOUS MINDS
        <svg class="icon" aria-hidden="true"><use href="#i-spark" /></svg>
      </span>
    </footer>
  </div>
</template>
