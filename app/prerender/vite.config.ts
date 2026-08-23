import { defineConfig } from 'vite'
import { resolve } from 'path'
import { dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

import {
  buildGoAliases,
  goTsResolver,
} from '../../bldr/web/bundler/vite/go-ts-resolver.js'

import { buildWorkspaceAliases } from './workspace-aliases.js'

const __dirname = dirname(fileURLToPath(import.meta.url))
const projectRoot = resolve(__dirname, '../../')
const goAliases = buildGoAliases(projectRoot)

export default defineConfig({
  root: __dirname,

  build: {
    outDir: resolve(__dirname, 'dist'),
    emptyOutDir: true,
    assetsInlineLimit: 2048,
    rolldownOptions: {
      input: {
        main: resolve(__dirname, 'index.html'),
      },
    },
  },

  resolve: {
    alias: [
      {
        find: '@aptre/bldr',
        replacement: resolve(projectRoot, './.bldr/src/web/bldr/index.js'),
      },
      {
        find: '@aptre/bldr-react',
        replacement: resolve(
          projectRoot,
          './.bldr/src/web/bldr-react/index.js',
        ),
      },
      {
        find: /^@aptre\/bldr-sdk\/(.*)$/,
        replacement: resolve(projectRoot, './.bldr/src/sdk/$1'),
      },
      ...goAliases,
      ...buildWorkspaceAliases(projectRoot),
    ],
  },

  plugins: [react(), tailwindcss(), goTsResolver(projectRoot)],
})
