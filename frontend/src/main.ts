import { createPinia } from 'pinia';
import { createApp } from 'vue';

import App from './App.vue';
import { createAppRouter } from './router';
import { useAuthStore } from './stores/auth';

async function bootstrap(): Promise<void> {
  const app = createApp(App);
  const pinia = createPinia();
  app.use(pinia);
  const auth = useAuthStore(pinia);
  try { await auth.restore(); } catch { /* network errors keep the anonymous shell usable */ }
  const router = createAppRouter();
  app.use(router);
  await router.isReady();
  app.mount('#app');
}

void bootstrap();
