import { configDefaults, defineConfig } from 'vitest/config'
import { playwright } from '@vitest/browser-playwright'
import type { Plugin } from 'vite'

// Plugin to enable cross-origin isolation for SharedArrayBuffer support.
// Must use enforce: "pre" to run before vitest:browser plugin steals html requests.
// See: https://github.com/vitest-dev/vitest/issues/3743
function crossOriginIsolationPlugin(): Plugin {
  return {
    name: 'cross-origin-isolation',
    enforce: 'pre',
    configureServer(server) {
      server.middlewares.use((_req, res, next) => {
        res.setHeader('Cross-Origin-Embedder-Policy', 'require-corp')
        res.setHeader('Cross-Origin-Opener-Policy', 'same-origin')
        next()
      })
    },
  }
}

// Unit tests use happy-dom, browser tests (*.browser.test.ts, *.e2e.test.ts) use vitest browser mode.
// E2E tests (*.e2e.spec.ts) use Playwright directly and are run separately via `bun run test:e2e`.
export default defineConfig({
  plugins: [crossOriginIsolationPlugin()],
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
        // Browser project needs its own plugins config
        plugins: [crossOriginIsolationPlugin()],
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
