import { configDefaults, defineConfig } from 'vitest/config'
import { playwright } from '@vitest/browser-playwright'

// Unit tests use happy-dom, browser tests (*.browser.test.ts, *.e2e.test.ts) use Playwright.
export default defineConfig({
  test: {
    projects: [
      {
        test: {
          name: 'unit',
          environment: 'happy-dom',
          include: ['**/*.test.ts'],
          exclude: [
            ...configDefaults.exclude,
            'dist',
            'vendor',
            '.bldr',
            'prototypes',
            '**/*.browser.test.ts',
            '**/*.e2e.test.ts',
          ],
        },
      },
      {
        test: {
          name: 'browser',
          include: ['**/*.browser.test.ts', '**/*.e2e.test.ts'],
          exclude: [...configDefaults.exclude, 'dist', 'vendor', '.bldr', 'prototypes'],
          browser: {
            enabled: true,
            provider: playwright(),
            headless: true,
            instances: [{ browser: 'chromium' }],
          },
        },
      },
    ],
  },
})
