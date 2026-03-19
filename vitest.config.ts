import path from 'path'
import { configDefaults, defineConfig } from 'vitest/config'

export default defineConfig({
  resolve: {
    alias: {
      '@go': path.resolve(__dirname, 'vendor'),
    },
  },
  test: {
    environment: 'happy-dom',
    exclude: [...configDefaults.exclude, 'dist', 'vendor', '.bldr', 'prototypes'],
  },
})
