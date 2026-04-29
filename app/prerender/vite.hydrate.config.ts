import { defineConfig, type Plugin } from 'vite'
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

// Resolves image imports to /static/assets/<basename> so the hydrate
// bundle produces the same URLs as the SSR prerender build.
function staticAssetPlugin(): Plugin {
  return {
    name: 'prerender-static-assets',
    enforce: 'pre',
    resolveId(source) {
      if (/\.(png|svg|jpg|gif|ico)(\?.*)?$/.test(source)) {
        return { id: '\0static-asset:' + source, moduleSideEffects: false }
      }
      return null
    },
    load(id) {
      if (!id.startsWith('\0static-asset:')) return null
      const source = id.slice('\0static-asset:'.length)
      const basename = source.split('/').pop()?.replace(/\?.*$/, '') ?? ''
      return `export default "/static/assets/${basename}";`
    },
  }
}

export default defineConfig({
  root: projectRoot,

  build: {
    outDir: resolve(__dirname, 'dist'),
    emptyOutDir: true,
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

  plugins: [
    staticAssetPlugin(),
    react(),
    tailwindcss(),
    goTsResolver(projectRoot),
  ],
})
