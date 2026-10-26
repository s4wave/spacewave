import { playwright } from '@vitest/browser-playwright'
import { defineConfig } from 'vitest/config'

// The PureFunction runner must give the same output in every engine, so its
// browser test runs in Chromium and WebKit.
export default defineConfig({
  test: {
    include: ['sdk/function/**/*.e2e.test.ts'],
    browser: {
      enabled: true,
      headless: true,
      provider: playwright(),
      instances: [{ browser: 'chromium' }, { browser: 'webkit' }],
      screenshotFailures: false,
    },
  },
})
