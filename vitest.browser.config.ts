import { defineConfig, mergeConfig } from 'vitest/config'
import { playwright } from '@vitest/browser-playwright'
import { resolve, dirname } from 'path'
import { access } from 'node:fs/promises'
import type { Plugin } from 'vite'
import viteConfig from './vite.config'

// bldrTsResolver resolves .js imports to .ts/.tsx files in .bldr directory.
function bldrTsResolver(): Plugin {
  return {
    name: 'bldr-ts-resolver',
    async resolveId(source, importer) {
      // Only handle .js imports from within .bldr
      if (!source.endsWith('.js') || !importer?.includes('.bldr')) {
        return null
      }

      // Resolve relative paths
      if (source.startsWith('./') || source.startsWith('../')) {
        const importerDir = dirname(importer)
        const basePath = resolve(importerDir, source.replace(/\.js$/, ''))

        // Try .ts first, then .tsx
        for (const ext of ['.ts', '.tsx']) {
          const fullPath = basePath + ext
          try {
            await access(fullPath)
            return fullPath
          } catch {
            // Continue to next extension
          }
        }
      }

      return null
    },
  }
}

// Check for UI or watch mode via environment variable
// Set BROWSER_TEST_UI=1 or BROWSER_TEST_WATCH=1 to run tests with visible browser (headless: false)
const showUI =
  process.env.BROWSER_TEST_UI === '1' || process.env.BROWSER_TEST_WATCH === '1'

export default mergeConfig(
  viteConfig,
  defineConfig({
    plugins: [bldrTsResolver()],
    optimizeDeps: {
      // Force optimization of .bldr modules to bundle type exports correctly
      include: ['@aptre/bldr', '@aptre/bldr-react'],
    },
    test: {
      browser: {
        enabled: true,
        headless: !showUI,
        provider: playwright({
          launchOptions: {
            headless: !showUI,
          },
          contextOptions: {
            // Force dark mode for Vitest UI and tests
            colorScheme: 'dark',
          },
        }),
        // https://vitest.dev/guide/browser/playwright
        instances: [{ browser: 'chromium' }],
        // Disable automatic screenshots - they just capture loading states
        screenshotFailures: false,
        // Force dark mode for Vitest UI
        orchestratorScripts: [
          {
            content: `localStorage.setItem('vueuse-color-scheme', 'dark')`,
            type: 'module',
          },
        ],
        // Use a desktop viewport (default 414x896 is mobile)
        viewport: { width: 1280, height: 800 },
      },
      // Include E2E test files
      include: ['**/*.e2e.test.{ts,tsx}'],
      // Setup files for E2E tests
      setupFiles: ['./web/test/browser-setup.ts'],
      // Default timeout for all tests (60 seconds)
      testTimeout: 60000,
    },
  }),
)
