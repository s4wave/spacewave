import { defineConfig, devices } from '@playwright/test'

const port = Number.parseInt(process.env.PLAYWRIGHT_WEB_PORT ?? '', 10) || 5593
const url = `http://127.0.0.1:${port}`

export default defineConfig({
  testDir: '.',
  testMatch: 'notes-combined-proof.spec.ts',
  timeout: 900_000,
  expect: {
    timeout: 10_000,
  },
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  workers: 1,
  reporter: 'list',
  use: {
    baseURL: url,
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
    {
      name: 'webkit',
      use: { ...devices['Desktop Safari'] },
    },
  ],
  webServer: {
    command: 'bun run start:web:goscript',
    cwd: '../..',
    url: `${url}/`,
    reuseExistingServer: false,
    timeout: 600_000,
  },
})
