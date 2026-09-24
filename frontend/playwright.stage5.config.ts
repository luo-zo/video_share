import { defineConfig, devices } from '@playwright/test';

const baseURL = process.env.STAGE5_E2E_BASE_URL || 'http://127.0.0.1:15173';

export default defineConfig({
  testDir: './tests/e2e',
  testMatch: '**/stage5.spec.ts',
  fullyParallel: false,
  workers: 1,
  forbidOnly: true,
  retries: 0,
  reporter: 'list',
  use: {
    baseURL,
    trace: 'retain-on-failure',
    ...devices['Desktop Chrome'],
  },
});
