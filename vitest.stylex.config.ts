import { defineConfig } from 'vitest/config'
import base from './vitest.browser.config.js'

export default defineConfig({
  ...base,
  test: {
    ...base.test,
    include: ['web/style/stylex-pilots.e2e.test.tsx'],
    browser: {
      ...base.test.browser,
      instances: [{ browser: 'chromium' }, { browser: 'webkit' }],
    },
  },
})
