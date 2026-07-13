import { defineConfig, devices } from '@playwright/test'
import { resolve } from 'node:path'

const repositoryRoot = resolve(import.meta.dirname, '..')

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
  timeout: 45_000,
  expect: { timeout: 10_000 },
  outputDir: 'test-results',
  snapshotPathTemplate: '{testDir}/{testFilePath}-snapshots/{arg}{ext}',
  reporter: [['list'], ['html', { open: 'never', outputFolder: 'playwright-report' }]],
  use: {
    ...devices['Desktop Chrome'],
    baseURL: 'http://127.0.0.1:15173',
    locale: 'zh-CN',
    timezoneId: 'Asia/Shanghai',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  webServer: [
    {
      command: 'go run ./cmd/server',
      cwd: repositoryRoot,
      env: {
        APP_COOKIE_SECURE: 'false',
        APP_ENCRYPTION_KEY: Buffer.alloc(32, 7).toString('base64'),
        APP_ENV: 'development',
        APP_PORT: '18081',
        DATA_DIR: resolve(repositoryRoot, '.tmp/e2e-data'),
      },
      url: 'http://127.0.0.1:18081/readyz',
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command: 'npm run dev -- --port 15173',
      cwd: resolve(repositoryRoot, 'web'),
      env: { VITE_API_PROXY_TARGET: 'http://127.0.0.1:18081' },
      url: 'http://127.0.0.1:15173',
      reuseExistingServer: false,
      timeout: 120_000,
    },
  ],
  projects: [
    {
      name: 'setup',
      testMatch: /setup\.spec\.ts/,
    },
    {
      name: 'chromium',
      dependencies: ['setup'],
      testIgnore: /(setup|backup)\.spec\.ts/,
    },
    {
      name: 'recovery',
      dependencies: ['chromium'],
      testMatch: /backup\.spec\.ts/,
    },
  ],
})
