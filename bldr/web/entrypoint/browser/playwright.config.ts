import { defineConfig, devices } from '@playwright/test'

const distPath =
  process.env.PLAYWRIGHT_RELEASE_DIST_PATH ??
  '.bldr-dist/build/web/js/wasm/bldr-demo-release/dist'
const port =
  Number.parseInt(process.env.PLAYWRIGHT_WEB_PORT ?? '', 10) ||
  30000 + Math.floor(Math.random() * 10000)
const url = `http://localhost:${port}`

export default defineConfig({
  testDir: '.',
  testMatch: '*.e2e.spec.ts',
  timeout: 60000,
  expect: {
    timeout: 10000,
  },
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
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
    command: `bun run bldr -- static --path ./${distPath} --listen :${port}`,
    cwd: '../../..',
    url,
    reuseExistingServer: false,
    timeout: 30000,
  },
})
