import { configDefaults, defineConfig } from 'vitest/config'
import { playwright } from '@vitest/browser-playwright'
import path from 'path'

// Browser tests for QuickJS browser worker prototype
// Run with: bun run vitest run --config prototypes/quickjs-browser-worker/vitest.config.ts
export default defineConfig({
  root: path.resolve(import.meta.dirname, '../..'),
  test: {
    name: 'quickjs-browser',
    include: ['prototypes/quickjs-browser-worker/**/*.browser.test.ts'],
    exclude: [...configDefaults.exclude],
    globalSetup: ['prototypes/quickjs-browser-worker/vitest.setup.ts'],
    browser: {
      enabled: true,
      provider: playwright(),
      headless: true,
      instances: [{ browser: 'chromium' }],
    },
  },
})
