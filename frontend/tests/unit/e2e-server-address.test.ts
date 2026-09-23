// @vitest-environment node
import { readFile, readdir } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

import { afterEach, describe, expect, it, vi } from 'vitest';

const E2E_DIR = fileURLToPath(new URL('../e2e', import.meta.url));

async function loadConfig(e2ePort?: string) {
  vi.resetModules();
  if (e2ePort === undefined) delete process.env.E2E_PORT;
  else process.env.E2E_PORT = e2ePort;
  const module = await import('../../playwright.config');
  return module.default;
}

afterEach(() => {
  delete process.env.E2E_PORT;
  vi.resetModules();
});

// 5173 可能被另一个工作树的 dev server 占住，而 reuseExistingServer 会安静地复用它，
// 于是跑出来的结果属于那个目录。E2E_PORT 是唯一的逃生口，必须三处同时生效。
describe('Playwright e2e server address', () => {
  it('honours E2E_PORT for baseURL, the readiness probe and the dev server port', async () => {
    const config = await loadConfig('5273');
    const server = Array.isArray(config.webServer) ? config.webServer[0] : config.webServer;

    expect(config.use?.baseURL).toBe('http://127.0.0.1:5273');
    expect(server?.url).toBe('http://127.0.0.1:5273');
    // 端口要一路传到 Vite：探针在 5273 等、Vite 却在 5173 监听，webServer 会超时。
    expect(server?.env?.PORT).toBe('5273');
  });

  it('falls back to the documented 5173 when E2E_PORT is unset', async () => {
    const config = await loadConfig();
    const server = Array.isArray(config.webServer) ? config.webServer[0] : config.webServer;

    expect(config.use?.baseURL).toBe('http://127.0.0.1:5173');
    expect(server?.url).toBe('http://127.0.0.1:5173');
    expect(server?.env?.PORT).toBe('5173');
  });

  // 规格文件自带 origin 会让整套用例重新变得无法归属；地址只允许来自 playwright.config.ts。
  it('keeps no hardcoded loopback origin in the spec files', async () => {
    const specs = (await readdir(E2E_DIR)).filter((name) => name.endsWith('.spec.ts'));
    expect(specs.length).toBeGreaterThan(0);
    for (const name of specs) {
      const source = await readFile(`${E2E_DIR}/${name}`, 'utf8');
      expect(source, name).not.toMatch(/127\.0\.0\.1:\d+/);
    }
  });
});
