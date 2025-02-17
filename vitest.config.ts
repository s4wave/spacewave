import { configDefaults, defineConfig } from 'vitest/config'
import { resolve } from 'path'
import { access } from 'node:fs/promises'

export default defineConfig({
  test: {
    environment: 'happy-dom',
    exclude: [...configDefaults.exclude, 'dist', 'vendor', '.bldr', 'prototypes'],
    alias: {
      "@go/*": resolve(__dirname, "./vendor/*"),
      "@aptre/bldr": resolve(__dirname, ".bldr/src/web/bldr/index.js"),
      "@aptre/bldr-react": resolve(__dirname, ".bldr/src/web/bldr-react/index.js"),
    },
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
})
