import { defineConfig, devices } from '@playwright/test'
import { fileURLToPath } from 'url'
import { dirname } from 'path'

const dir = dirname(fileURLToPath(import.meta.url))
const port =
  Number.parseInt(process.env.PLAYWRIGHT_WEB_PORT ?? '', 10) || 40719
const url = `http://localhost:${port}`

export default defineConfig({
  testDir: '.',
  testMatch: '*.spec.ts',
  timeout: 60000,
  expect: {
    timeout: 15000,
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
    reuseExistingServer: true,
    timeout: 10000,
  },
})
