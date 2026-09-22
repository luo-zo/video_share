import vue from '@vitejs/plugin-vue';
import { loadEnv, type Plugin } from 'vite';
import { defineConfig } from 'vitest/config';

import {
  DEFAULT_API_TARGET,
  DEFAULT_STORAGE_ORIGIN,
  createApiMiddleware,
  methodsForAPIPath,
} from './server.mjs';

const DEV_PORT = 5173;

// 开发期 CSP 必须给 Vite 的 HMR 留出口：模块脚本是内联注入的，<style> 是运行时插进
// document 的。生产环境的严格 CSP（script-src 'self'、style-src 'self'）由 Nginx 下发，
// 见 docs/stage5/decisions.md 第 1.3 节。
function devSecurityHeaders(storageOrigin: string): Record<string, string> {
  return {
    'X-Content-Type-Options': 'nosniff',
    'Referrer-Policy': 'no-referrer',
    'Cross-Origin-Resource-Policy': 'same-origin',
    'X-Frame-Options': 'DENY',
    'Content-Security-Policy': [
      "default-src 'self'",
      "script-src 'self' 'unsafe-inline'",
      "style-src 'self' 'unsafe-inline'",
      `img-src 'self' data: ${storageOrigin}`,
      `connect-src 'self' ws: wss: ${storageOrigin}`,
      `media-src 'self' blob: ${storageOrigin}`,
      "object-src 'none'",
      "base-uri 'none'",
      "frame-ancestors 'none'",
      "form-action 'self'",
    ].join('; '),
  };
}

// 复用旧的同源代理边界：API 路径白名单、写请求 Origin 校验、请求体上限、上游超时和
// 重定向白名单都在 server.mjs 里，开发服务器不再另写一套，避免两端行为漂移。
function devApiBoundary(env: Record<string, string>): Plugin {
  return {
    name: 'video-share:dev-api-boundary',
    apply: 'serve',
    configureServer(server) {
      const api = createApiMiddleware({
        apiTarget: env.API_TARGET || DEFAULT_API_TARGET,
        storageOrigin: env.STORAGE_ORIGIN || DEFAULT_STORAGE_ORIGIN,
      });
      server.middlewares.use((request, response, next) => {
        const route = (request.url || '').split('?')[0];
        // 非 API 路径交给 Vite 自己的静态资源和 SPA 回退中间件。
        if (!methodsForAPIPath(route)) {
          next();
          return;
        }
        void api(request, response, next);
      });
    },
  };
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', '');
  const storageOrigin = env.STORAGE_ORIGIN || DEFAULT_STORAGE_ORIGIN;
  const port = Number(env.PORT) || DEV_PORT;

  return {
    plugins: [vue(), devApiBoundary(env)],
    server: {
      host: '127.0.0.1',
      port,
      strictPort: true,
      headers: devSecurityHeaders(storageOrigin),
    },
    build: {
      target: 'es2022',
      outDir: 'dist',
      emptyOutDir: true,
    },
    test: {
      environment: 'jsdom',
      include: ['tests/unit/**/*.test.ts'],
      restoreMocks: true,
    },
  };
});
