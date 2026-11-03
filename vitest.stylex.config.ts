import { playwright } from '@vitest/browser-playwright'
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
    include: [
      'web/style/stylex-pilots.e2e.test.tsx',
      'web/ui/FloatingWindow.e2e.test.tsx',
      'web/forge/ForgeViewerShell.e2e.test.tsx',
    ],
    browser: {
      ...base.test.browser,
      connectTimeout: 30000,
      instances: [
        { browser: 'chromium' },
        { browser: 'webkit' },
        ...(['chromium', 'webkit'] as const).map((browser) => ({
          browser,
          name: browser + '-touch',
          include: ['web/forge/ForgeViewerShell.e2e.test.tsx'],
          provider: playwright({
            contextOptions: { hasTouch: true, colorScheme: 'dark' },
          }),
        })),
      ],
    },
  },
})
