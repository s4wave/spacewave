import { defineConfig, devices } from '@playwright/test'

// The frontend fixture owns its CLI. This lane never starts the application
// server.
export default defineConfig({
  testDir: '.',
  testMatch: 'frontend-hmr.spec.ts',
  timeout: 110_000,
  workers: 1,
  retries: 0,
  reporter: 'list',
  outputDir: '../../.tmp/frontend-hmr-results',
  use: { screenshot: 'off', trace: 'off', actionTimeout: 15_000 },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
    { name: 'webkit', use: { ...devices['Desktop Safari'] } },
  ],
})
