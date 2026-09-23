import { defineConfig, devices } from '@playwright/test';

// 端口可由 E2E_PORT 覆盖：默认 5173 常被另一个工作树的 dev server 占住，而
// `reuseExistingServer` 会安静地复用它——那样跑出来的结果属于别的目录，不能算本树的验收。
// 换端口后该端口上无人监听，Playwright 只能自己拉起本目录的 dev server。
const port = Number(process.env.E2E_PORT) || 5173;
const baseURL = `http://127.0.0.1:${port}`;

export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 2 : 0,
  reporter: 'list',
  use: {
    baseURL,
    trace: 'on-first-retry',
  },
  webServer: {
    command: 'npm run dev',
    url: baseURL,
    // vite.config.ts 从环境变量 PORT 取监听端口，这里把它对齐到本次选定的端口。
    env: { PORT: String(port) },
    reuseExistingServer: !process.env.CI,
    timeout: 30_000,
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],
});
