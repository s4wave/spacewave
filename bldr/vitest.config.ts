import { configDefaults, defineConfig } from 'vitest/config'
import { playwright } from '@vitest/browser-playwright'
import type { Plugin } from 'vite'
import { resolve } from 'path'
import { dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { buildGoAliases, goTsResolver } from './web/bundler/vite/go-ts-resolver.js'

const __dirname = dirname(fileURLToPath(import.meta.url))
const repoRoot = resolve(__dirname, '..')

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

const commonPlugins = [crossOriginIsolationPlugin(), goTsResolver(repoRoot)]
const bldrAliases = [
  ...buildGoAliases(repoRoot),
  {
    find: /^@aptre\/bldr$/,
    replacement: resolve(__dirname, 'web/bldr/index.js'),
  },
  {
    find: /^@aptre\/bldr-react$/,
    replacement: resolve(__dirname, 'web/bldr-react/index.js'),
  },
  {
    find: /^@aptre\/bldr-sdk$/,
    replacement: resolve(__dirname, 'sdk/plugin.ts'),
  },
  {
    find: /^@aptre\/bldr-sdk\/(.*)$/,
    replacement: resolve(__dirname, 'sdk/$1'),
  },
  {
    find: /^web\/(.*)$/,
    replacement: resolve(__dirname, 'web/$1'),
  },
]

export default defineConfig({
  root: repoRoot,
  plugins: commonPlugins,
  resolve: {
    alias: bldrAliases,
  },
  test: {
    projects: [
      {
        plugins: commonPlugins,
        resolve: {
          alias: bldrAliases,
        },
        test: {
          name: 'unit',
          environment: 'happy-dom',
          include: ['bldr/**/*.test.ts'],
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
        plugins: commonPlugins,
        resolve: {
          alias: bldrAliases,
        },
        test: {
          name: 'browser',
          include: ['bldr/**/*.browser.test.ts', 'bldr/**/*.e2e.test.ts'],
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
