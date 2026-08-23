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

import {
  buildWorkspaceAliases,
  staticAssetPlugin,
} from './workspace-aliases.js'

const __dirname = dirname(fileURLToPath(import.meta.url))
const projectRoot = resolve(__dirname, '../../')
const bldrDistRoot = resolve(projectRoot, '.bldr-dist/src')
const goAliases = buildGoAliases(projectRoot, bldrDistRoot)

export default defineConfig({
  root: projectRoot,

  build: {
    ssr: resolve(__dirname, 'build.ts'),
    outDir: resolve(__dirname, 'ssr-dist'),
    emptyOutDir: true,
    rolldownOptions: {
      output: {
        format: 'es',
      },
    },
  },

  ssr: {
    // Bundle everything into a single file for portability.
    noExternal: true,
    // Keep node builtins external. Also externalize heavy blog deps
    // (shiki, gray-matter) so bun resolves them from node_modules at
    // runtime, keeping the SSR bundle small.
    external: [
      'fs',
      'path',
      'url',
      'os',
      'crypto',
      'stream',
      'util',
      'events',
      'buffer',
      'process',
      'shiki',
      'gray-matter',
    ],
  },

  resolve: {
    alias: [
      {
        find: '@aptre/bldr',
        replacement: resolve(bldrDistRoot, 'web/bldr/index.js'),
      },
      {
        find: '@aptre/bldr-react',
        replacement: resolve(bldrDistRoot, 'web/bldr-react/index.js'),
      },
      {
        find: /^@aptre\/bldr-sdk\/(.*)$/,
        replacement: resolve(bldrDistRoot, 'sdk/$1'),
      },
      ...goAliases,
      ...buildWorkspaceAliases(projectRoot),
    ],
  },

  plugins: [
    staticAssetPlugin(),
    react(),
    tailwindcss(),
    goTsResolver(projectRoot, bldrDistRoot),
  ],
})
