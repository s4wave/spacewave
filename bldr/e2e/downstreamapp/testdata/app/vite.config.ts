import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))

const materializedSpacewaveRoot = resolve(__dirname, '.s4wave')
const materializedBldrSDKRoot = resolve(__dirname, '.aptre/bldr-sdk')

export default {
  resolve: {
    alias: [
      {
        find: '@aptre/bldr-sdk',
        replacement: materializedBldrSDKRoot,
      },
      {
        find: '@s4wave/sdk',
        replacement: resolve(materializedSpacewaveRoot, 'sdk'),
      },
      {
        find: '@s4wave/web',
        replacement: resolve(materializedSpacewaveRoot, 'web'),
      },
      {
        find: 'non-index-root-pkg',
        replacement: resolve(__dirname, 'web/fixtures/non-index-root-pkg'),
      },
    ],
  },
}
