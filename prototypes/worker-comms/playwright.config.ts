import { defineConfig, devices } from '@playwright/test'
import { dirname } from 'path'
import { fileURLToPath } from 'url'

const dir = dirname(fileURLToPath(import.meta.url))
const port =
  Number.parseInt(process.env.PLAYWRIGHT_WEB_PORT ?? '', 10) || 40717
const url = `http://localhost:${port}`

export default defineConfig({
  testDir: '.',
  testMatch: '*.spec.ts',
  timeout: 120000,
  expect: {
    timeout: 10000,
  },
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: 0,
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
      name: 'firefox',
      use: { ...devices['Desktop Firefox'] },
    },
    {
      name: 'webkit',
      use: { ...devices['Desktop Safari'] },
    },
  ],
  webServer: {
    command: `bun server.ts ${port}`,
    cwd: dir,
    url,
    reuseExistingServer: false,
    timeout: 10000,
  },
})
