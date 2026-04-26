import { defineConfig } from 'vite'
import { resolve } from 'path'
import { dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

import {
  buildGoAliases,
  goTsResolver,
} from '../../.bldr/src/web/bundler/vite/go-ts-resolver.js'

const __dirname = dirname(fileURLToPath(import.meta.url))
const projectRoot = resolve(__dirname, '../../')

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
      ...buildGoAliases(projectRoot),
      {
        find: /^@s4wave\/core\/(.*)$/,
        replacement: resolve(projectRoot, './core/$1'),
      },
      {
        find: /^@s4wave\/sdk\/(.*)$/,
        replacement: resolve(projectRoot, './sdk/$1'),
      },
      {
        find: '@s4wave/sdk',
        replacement: resolve(projectRoot, './sdk'),
      },
      {
        find: /^@s4wave\/app\/(.*)$/,
        replacement: resolve(projectRoot, './app/$1'),
      },
      {
        find: '@s4wave/app',
        replacement: resolve(projectRoot, './app'),
      },
      {
        find: /^@s4wave\/web\/(.*)$/,
        replacement: resolve(projectRoot, './web/$1'),
      },
      {
        find: '@s4wave/web',
        replacement: resolve(projectRoot, './web'),
      },
    ],
  },

  plugins: [react(), tailwindcss(), goTsResolver(projectRoot)],
})
