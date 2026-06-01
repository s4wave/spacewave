import { defineConfig, devices } from '@playwright/test'

const port = Number.parseInt(process.env.PLAYWRIGHT_WEB_PORT ?? '', 10) || 5197
const url = `http://127.0.0.1:${port}`

export default defineConfig({
  testDir: '.',
  testMatch: 'bottom-bar-context-menu.spec.ts',
  timeout: 60_000,
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
  ],
  webServer: {
    command: `bun run vite --host 127.0.0.1 --port ${port} --strictPort`,
    cwd: '../..',
    url: `${url}/e2e/webframe/bottom-bar-context-menu.html`,
    reuseExistingServer: false,
    timeout: 60_000,
  },
})
