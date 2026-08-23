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
  base: '/static/',

  build: {
    outDir: resolve(__dirname, 'dist'),
    emptyOutDir: true,
    manifest: true,
    // Externalize React packages so they resolve via the importmap
    // shared with the bldr entrypoint. This keeps the hydration bundle
    // small and avoids duplicate React instances.
    rolldownOptions: {
      input: resolve(__dirname, 'hydrate.tsx'),
      external: [
        'react',
        'react/jsx-runtime',
        'react/jsx-dev-runtime',
        'react-dom',
        'react-dom/client',
        'react-dom/test-utils',
        '@aptre/bldr',
        '@aptre/bldr-react',
        '@aptre/protobuf-es-lite',
        '@aptre/protobuf-es-lite/google/protobuf/empty',
        '@aptre/protobuf-es-lite/google/protobuf/timestamp',
      ],
      output: {
        format: 'es',
        entryFileNames: 'hydrate-[hash].js',
        codeSplitting: false,
      },
    },
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
