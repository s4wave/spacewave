import { configDefaults, defineConfig } from 'vitest/config'
import { resolve } from 'path'

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
})
