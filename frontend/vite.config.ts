import vue from '@vitejs/plugin-vue';
import { loadEnv, type Plugin } from 'vite';
import { defineConfig } from 'vitest/config';

import {
  DEFAULT_API_TARGET,
  DEFAULT_STORAGE_ORIGIN,
  createApiMiddleware,
} from './server.mjs';

const DEV_PORT = 5173;
const BLOCKED_DEV_FILES = new Set([
  '/.nvmrc',
  '/legacy.html',
  '/package.json',
  '/package-lock.json',
  '/playwright.config.ts',
  '/readme.md',
  '/server.d.mts',
  '/server.mjs',
  '/tsconfig.json',
  '/tsconfig.node.json',
  '/vite.config.ts',
]);
const BLOCKED_LEGACY_SOURCES = new Set([
  '/src/auth.js',
  '/src/cat.js',
  '/src/community.js',
  '/src/detail-view.js',
  '/src/discover-view.js',
  '/src/main.js',
  '/src/player.js',
  '/src/profile-view.js',
  '/src/video.js',
  '/src/view-kit.js',
  '/vendor/hls.mjs',
]);

export function isBlockedDevPath(rawPath: string): boolean {
  let decoded = rawPath;
  try {
    for (let round = 0; round < 5; round += 1) {
      const next = decodeURIComponent(decoded);
      if (next === decoded) break;
      decoded = next;
      if (round === 4) return true;
    }
  } catch {
    return true;
  }
  decoded = decoded.replaceAll('\\', '/').toLowerCase();
  const segments = decoded.split('/').filter(Boolean);
  if (segments.some((part) => part === '.' || part === '..')) return true;
  if (segments.some((part, index) => (
    part.startsWith('.') && !(part === '.vite' && segments[index - 1] === 'node_modules')
  ))) return true;
  // 先归一再比对：重复斜杠、末尾斜杠等写法必须收敛到同一条路径，否则
  // /src//auth.js、//server.mjs 这类拼写只靠字符串精确匹配就能绕过黑名单。
  const canonical = `/${segments.join('/')}`;
  if (canonical === '/@fs' || canonical.startsWith('/@fs/')) return true;
  return BLOCKED_DEV_FILES.has(canonical)
    || BLOCKED_LEGACY_SOURCES.has(canonical)
    || canonical.startsWith('/dist/')
    || canonical.startsWith('/tests/')
    || canonical.startsWith('/test-results/')
    || canonical.startsWith('/playwright-report/');
}

const FILE_EXTENSION = /\.[a-z0-9]+$/i;

/**
 * 判断一个未命中的请求是否应该回退到应用外壳（index.html）。只有无扩展名的应用
 * 页面路径可以回退；缺失的静态资源、带扩展名的文件和 Vite 内部请求（/@*）必须
 * 保持 404，不能返回首页冒充成功。
 */
export function shouldFallbackToAppShell(route: string): boolean {
  if (!route.startsWith('/') || route.startsWith('/@')) return false;
  const path = route.split('?')[0].split('#')[0];
  if (path === '/') return true;
  const segments = path.split('/').filter(Boolean);
  if (segments.length === 0) return false;
  const last = segments[segments.length - 1];
  return last.startsWith('.') || !FILE_EXTENSION.test(last);
}

export function shouldHandleApiPath(route: string): boolean {
  return route === '/healthz' || route === '/readyz' || route.startsWith('/api/');
}

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
        if (isBlockedDevPath(route)) {
          response.writeHead(404, {
            'Content-Type': 'application/json; charset=utf-8',
            'Cache-Control': 'no-store',
          });
          response.end(JSON.stringify({ error: { code: 'NOT_FOUND', message: 'Resource not found.' } }));
          return;
        }
        // 非 API 路径交给 Vite 自己的静态资源中间件。只有应用页面路径才改写成
        // index.html；其余未命中路径保持 404，由 Vite 的 mpa 行为直接返回，不再走
        // 默认的 SPA 全量回退。
        if (!shouldHandleApiPath(route)) {
          if (shouldFallbackToAppShell(route)) {
            request.url = '/index.html';
          }
          next();
          return;
        }
        void api(request, response, () => {
          response.writeHead(404, {
            'Content-Type': 'application/json; charset=utf-8',
            'Cache-Control': 'no-store',
          });
          response.end(JSON.stringify({ error: { code: 'NOT_FOUND', message: 'Resource not found.' } }));
        });
      });
    },
  };
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', '');
  const storageOrigin = env.STORAGE_ORIGIN || DEFAULT_STORAGE_ORIGIN;
  const port = Number(env.PORT) || DEV_PORT;

  return {
    // 关闭 Vite 默认的 SPA 全量回退（appType: 'spa' 会把缺失的 .js/.png 也回退成
    // index.html 并返回 200）。页面回退由 devApiBoundary 按白名单显式完成。
    appType: 'mpa',
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
