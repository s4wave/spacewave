import { defineConfig } from 'vitest/config'
import base from './vitest.browser.config.js'

export default defineConfig({
  ...base,
  optimizeDeps: {
    ...base.optimizeDeps,
    // Prebundle the renderer, controls and SDK used by the real app environment together.
    include: [
      'react',
      'react-dom/client',
      '@aptre/bldr',
      '@aptre/bldr-react',
      '@radix-ui/react-tabs',
      '@radix-ui/react-label',
      '@radix-ui/react-collapsible',
      '@radix-ui/react-dropdown-menu',
      '@radix-ui/react-dialog',
      '@radix-ui/react-slot',
      'react-icons',
      'react-icons/lu',
      'cnfast',
    ],
  },
  test: {
    ...base.test,
    include: ['web/style/stylex-pilots.e2e.test.tsx'],
    // The measured cold CSS and renderer preparation takes about 75 seconds on the shared host.
    env: { ...base.test?.env, VITEST_BROWSER_IFRAME_TIMEOUT: '120000' },
    browser: {
      ...base.test.browser,
      connectTimeout: 30000,
      instances: [{ browser: 'chromium' }, { browser: 'webkit' }],
    },
  },
})
