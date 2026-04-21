import { defineConfig, devices } from '@playwright/test'

const port = Number.parseInt(process.env.E2E_PORT ?? '', 10) || 8080
const url = `http://localhost:${port}`

export default defineConfig({
  testDir: '.',
  testMatch: '*.spec.ts',
  timeout: 120_000,
  expect: { timeout: 30_000 },
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL: url,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: {
    command: `bun run start:web:wasm`,
    cwd: '..',
    url,
    reuseExistingServer: true,
    timeout: 600_000,
  },
})
