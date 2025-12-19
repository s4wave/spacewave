import { configDefaults, defineConfig } from 'vitest/config'
import { playwright } from '@vitest/browser-playwright'

// Unit tests use happy-dom, browser tests (*.browser.test.ts, *.e2e.test.ts) use vitest browser mode.
// E2E tests (*.e2e.spec.ts) use Playwright directly and are run separately via `bun run test:e2e`.
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
            '.bldr-*',
            'prototypes',
            '**/*.browser.test.ts',
            '**/*.e2e.test.ts',
            '**/*.e2e.spec.ts',
          ],
        },
      },
      {
        test: {
          name: 'browser',
          include: ['**/*.browser.test.ts', '**/*.e2e.test.ts'],
          exclude: [
            ...configDefaults.exclude,
            'dist',
            'vendor',
            '.bldr',
            '.bldr-*',
            'prototypes',
            '**/*.e2e.spec.ts',
          ],
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
