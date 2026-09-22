// @vitest-environment node
import { describe, expect, it } from 'vitest';

import {
  isBlockedDevPath,
  shouldFallbackToAppShell,
  shouldHandleApiPath,
} from '../../vite.config';

describe('Vite development boundary', () => {
  it.each([
    '/server.mjs', '/package.json', '/.nvmrc', '/package-lock.json', '/tests/unit/router.test.ts',
    '/.gitignore', '/.vite/deps/private.js', '/src/.env', '/legacy.html', '/src/main.js',
    '/@fs/C:/workspace/video-share/frontend/server.mjs',
    '/%40fs/C%3A/workspace/video-share/frontend/package.json',
    '/%2540fs/C%253A/workspace/video-share/frontend/server.mjs',
  ]) (
    'blocks project and test file %s',
    (path) => expect(isBlockedDevPath(path)).toBe(true),
  );

  // 重复斜杠、末尾斜杠等写法必须归一到同一条路径再比对，否则黑名单会被字符串差异绕过。
  it.each([
    '/src//auth.js', '/src///main.js', '//server.mjs', '//package.json',
    '/server.mjs/', '/package.json/', '/src/main.js/', '/LEGACY.HTML',
    '//@fs/C:/workspace/video-share/frontend/server.mjs',
  ]) (
    'blocks a normalised spelling of a protected path %s',
    (path) => expect(isBlockedDevPath(path)).toBe(true),
  );

  it.each([
    '/', '/login', '/video/9', '/src/main.ts', '/src/styles.css', '/@vite/client',
    '/@id/__x00__plugin-vue:export-helper', '/node_modules/.vite/deps/vue.js?v=1', '/assets/app.js',
  ]) (
    'allows application route or asset %s',
    (path) => expect(isBlockedDevPath(path)).toBe(false),
  );

  it('sends every API-prefixed path through the restrictive API middleware', () => {
    expect(shouldHandleApiPath('/api/v1/videos')).toBe(true);
    expect(shouldHandleApiPath('/api/v1/not-allowlisted')).toBe(true);
    expect(shouldHandleApiPath('/src/main.ts')).toBe(false);
  });

  // 只有应用页面路径可以回退到 index.html；缺失的静态资源、未知扩展名和 Vite 内部
  // 请求必须保持 404，不能伪装成首页（任务书第 3 节信任边界）。
  it.each([
    '/', '/login', '/video/9', '/me', '/upload', '/search/deep-link',
  ]) (
    'falls back to the application shell for page route %s',
    (path) => expect(shouldFallbackToAppShell(path)).toBe(true),
  );

  it.each([
    '/missing.js', '/missing.png', '/assets/nope.css', '/src/does-not-exist.js',
    '/@vite/client', '/@id/__x00__plugin-vue:export-helper', '/node_modules/.vite/deps/vue.js',
    '/favicon.ico', '/docs/stage5/baseline.md',
  ]) (
    'does not fall back to the application shell for %s',
    (path) => expect(shouldFallbackToAppShell(path)).toBe(false),
  );
});
