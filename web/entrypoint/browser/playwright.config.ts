import { defineConfig, devices } from '@playwright/test'

const distPath = '.bldr-dist/build/native/js/wasm/bldr-demo-release/dist'

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
    baseURL: 'http://localhost:3000',
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: {
    command: `bun run bldr -- static --path ./${distPath} --listen :3000`,
    cwd: '../../..',
    url: 'http://localhost:3000',
    reuseExistingServer: !process.env.CI,
    timeout: 30000,
  },
})
