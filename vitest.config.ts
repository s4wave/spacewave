import { configDefaults, defineConfig } from 'vitest/config'
import { resolve } from 'path'

export default defineConfig({
  test: {
    exclude: [...configDefaults.exclude, 'dist', 'vendor', '.bldr', 'prototypes'],
    alias: {
      "@go/*": resolve(__dirname, "./vendor/*"),
      "@aptre/bldr": resolve(__dirname, "./web/bldr/index.js"),
      "@aptre/bldr-react": resolve(__dirname, "./web/bldr-react/index.js"),
    },
  },
})
