import { configDefaults, defineConfig, mergeConfig } from 'vitest/config'
import { resolve } from 'path'
import { access } from 'node:fs/promises'
import viteConfig from './vite.config'

export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      environment: 'happy-dom',
      setupFiles: ['./web/test/setup.ts'],
      exclude: [
        ...configDefaults.exclude,
        'dist/**',
        'vendor/**',
        '**/.bldr/**',
        '.tmp/**',
        'prototypes/**',
        // E2E tests require browser environment, run separately via test:browser
        '**/*.e2e.test.{ts,tsx}',
        // Go-based end-to-end harnesses live under e2e/ and run separately.
        'e2e/**',
      ],
    },
    plugins: [
      {
        // This plugin fixes issues with resolving paths like @go/foo/bar/baz.js where baz.ts exists only.
        name: 'go-ts-resolver',
        async resolveId(source) {
          // Handle only @go/ paths that end in .js
          if (!source.startsWith('@go/') || !source.endsWith('.js')) {
            return null
          }

          // Convert @go/ path to vendor path
          const vendorPath = resolve(
            __dirname,
            'vendor',
            source.slice('@go/'.length),
          )
          const tsPath = vendorPath.replace(/\.js$/, '.ts')

          try {
            await access(tsPath)
            return tsPath
          } catch {
            return null
          }
        },
      },
    ],
  }),
)
