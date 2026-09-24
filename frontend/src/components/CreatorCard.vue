<script setup lang="ts">
defineProps<{
  creator: { id: string | number; username?: string; nickname?: string };
}>();
</script>

<template>
  <RouterLink class="creator-card" :to="`/creator/${creator.id}`">
    <span class="creator-card-avatar" aria-hidden="true">🐈‍⬛</span>
    <span class="creator-card-copy"><strong>{{ creator.nickname || creator.username }}</strong><small>@{{ creator.username }}</small></span>
  </RouterLink>
</template>
