import { defineConfig } from 'vitest/config'
import base from './vitest.browser.config.js'

export default defineConfig({
  ...base,
  optimizeDeps: {
    ...base.optimizeDeps,
    // Prebundle the fixture's renderer and Radix together without the unused Bldr entrypoints.
    include: [
      'react',
      'react-dom/client',
      '@radix-ui/react-collapsible',
      '@radix-ui/react-dropdown-menu',
      '@radix-ui/react-dialog',
      'cnfast',
    ],
  },
  test: {
    ...base.test,
    include: ['web/style/stylex-pilots.e2e.test.tsx'],
    browser: {
      ...base.test.browser,
      instances: [{ browser: 'chromium' }, { browser: 'webkit' }],
    },
  },
})
