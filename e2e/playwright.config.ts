import { defineConfig, devices } from '@playwright/test';
import { authFile } from './tests/paths';

export default defineConfig({
  testDir: './tests',
  // CI runs the screenshot spec as its own job.
  testIgnore: process.env.E2E_SKIP_SCREENSHOTS ? /screenshots\.spec\.ts/ : undefined,
  // One server with shared state: run everything serially in a single worker.
  fullyParallel: false,
  workers: 1,
  retries: 0,
  forbidOnly: !!process.env.CI,
  timeout: 30_000,
  expect: { timeout: 5_000 },
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'http://127.0.0.1:18080',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    { name: 'setup', testMatch: /auth\.setup\.ts/ },
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'], storageState: authFile },
      dependencies: ['setup'],
    },
  ],
});
