import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))

export default {
  resolve: {
    alias: [
      {
        find: 'non-index-root-pkg',
        replacement: resolve(__dirname, 'web/fixtures/non-index-root-pkg'),
      },
    ],
  },
}
