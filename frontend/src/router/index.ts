import { createRouter, createWebHistory } from 'vue-router';
import type { RouteRecordRaw } from 'vue-router';

import { useAuthStore } from '../stores/auth';

declare module 'vue-router' {
  interface RouteMeta {
    /** 未登录访问时先跳转登录页，并在 returnTo 里带回原地址。 */
    requiresSession?: boolean;
  }
}

// 用同一个固定站点做基准解析，凡是解析后逃逸出该站点的值一律拒绝。
const SAME_ORIGIN = 'http://localhost';
const LOGIN_PATH = '/login';

/**
 * 只接受站内绝对路径。协议相对地址（//host）、反斜杠变体、带协议的绝对地址
 * 解析后 origin 都会变化，因此统一被 origin 校验挡下，避免开放重定向。
 */
export function safeReturnTo(value: unknown, fallback = '/'): string {
  if (typeof value !== 'string' || !value.startsWith('/')) return fallback;
  let parsed: URL;
  try {
    parsed = new URL(value, SAME_ORIGIN);
  } catch {
    return fallback;
  }
  if (parsed.origin !== SAME_ORIGIN || parsed.pathname === LOGIN_PATH) return fallback;
  return `${parsed.pathname}${parsed.search}${parsed.hash}`;
}

const routes: RouteRecordRaw[] = [
  { path: '/', name: 'discover', component: () => import('../views/DiscoverView.vue') },
  { path: '/following', name: 'following', component: () => import('../views/FollowingView.vue'), meta: { requiresSession: true } },
  { path: '/ranking', name: 'ranking', component: () => import('../views/RankingView.vue') },
  { path: '/login', name: 'login', component: () => import('../views/LoginView.vue') },
  {
    path: '/video/:id',
    name: 'video-detail',
    component: () => import('../views/VideoDetailView.vue'),
    props: true,
  },
  {
    path: '/creator/:id',
    name: 'creator',
    component: () => import('../views/CreatorView.vue'),
  },
  {
    path: '/upload',
    name: 'upload',
    component: () => import('../views/UploadView.vue'),
    meta: { requiresSession: true },
  },
  {
    path: '/me',
    name: 'profile',
    component: () => import('../views/ProfileView.vue'),
    meta: { requiresSession: true },
  },
  {
    path: '/settings',
    name: 'settings',
    component: () => import('../views/SettingsView.vue'),
    meta: { requiresSession: true },
  },
  {
    path: '/notifications',
    name: 'notifications',
    component: () => import('../views/NotificationsView.vue'),
    meta: { requiresSession: true },
  },
  {
    path: '/reports',
    name: 'reports',
    component: () => import('../views/ReportsView.vue'),
    meta: { requiresSession: true },
  },
  {
    path: '/admin/reports',
    name: 'admin-reports',
    component: () => import('../views/AdminReportsView.vue'),
    meta: { requiresSession: true },
  },
  {
    path: '/admin/audit',
    name: 'admin-audit',
    component: () => import('../views/AuditView.vue'),
    meta: { requiresSession: true },
  },
  {
    path: '/creator-center',
    name: 'creator-center',
    component: () => import('../views/CreatorDashboardView.vue'),
    meta: { requiresSession: true },
  },
  { path: '/:pathMatch(.*)*', name: 'not-found', redirect: { name: 'discover' } },
];

export function createAppRouter() {
  const router = createRouter({
    history: createWebHistory(),
    routes,
    // 浏览器返回时回到原滚动位置，新导航则从页首开始。
    scrollBehavior: (_to, _from, saved) => saved ?? { top: 0 },
  });

  router.beforeEach((to) => {
    const auth = useAuthStore();
    if (to.name === 'login') {
      return auth.isAuthenticated ? { path: safeReturnTo(to.query.returnTo, '/') } : true;
    }
    const ownerDetail = to.name === 'video-detail' && to.query.owner === '1';
    if ((to.meta.requiresSession || ownerDetail) && !auth.isAuthenticated) {
      return { name: 'login', query: { returnTo: safeReturnTo(to.fullPath, '/') } };
    }
    return true;
  });

  return router;
}
