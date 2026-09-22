// @vitest-environment node
import { describe, expect, it } from 'vitest';

import { isBlockedDevPath, shouldHandleApiPath } from '../../vite.config';

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
});
