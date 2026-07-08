import { defineConfig, type Plugin } from 'vite'
import { dirname, resolve } from 'node:path'
import { existsSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

import {
  buildGoAliases,
  goTsResolver,
} from './bldr/web/bundler/vite/go-ts-resolver.js'

const __dirname = dirname(fileURLToPath(import.meta.url))

const bldrDistRoot =
  process.env['BLDR_DIST_ROOT'] || resolve(__dirname, '.bldr/src')

function resolveBldrSourcePath(...segments: string[]) {
  const isPatternReplacement = segments.some((segment) => segment.includes('$'))
  const patternProbeSegments = segments
    .map((segment) => segment.split('$')[0])
    .filter((segment) => segment.length > 0)
  const monorepoPath = resolve(bldrDistRoot, 'bldr', ...segments)
  const monorepoProbePath = resolve(
    bldrDistRoot,
    'bldr',
    ...patternProbeSegments,
  )
  if (
    existsSync(monorepoPath) ||
    (isPatternReplacement && existsSync(monorepoProbePath))
  ) {
    return monorepoPath
  }
  const distPath = resolve(bldrDistRoot, ...segments)
  const distProbePath = resolve(bldrDistRoot, ...patternProbeSegments)
  if (
    existsSync(distPath) ||
    (isPatternReplacement && existsSync(distProbePath))
  ) {
    return distPath
  }
  return isPatternReplacement
    ? monorepoPath
    : resolve(__dirname, 'bldr', ...segments)
}

// canonicalReactOptimizedDep keeps React's optimized entry canonical when Vite
// serves both /react.js and /react.js?v=... during dependency optimization.
// ReactDOM's optimized chunks import /react.js without the query; source
// modules import /react.js?v=..., and those must share one dispatcher.
function canonicalReactOptimizedDep(): Plugin {
  return {
    name: 'spacewave-canonical-react-optimized-dep',
    apply: 'serve',
    transform(_code, id) {
      const queryStart = id.indexOf('?')
      if (queryStart === -1) return null
      const file = id.slice(0, queryStart).replaceAll('\\', '/')
      if (
        !file.includes('/node_modules/.vite/') ||
        !file.includes('/deps/') ||
        !file.endsWith('/react.js')
      ) {
        return null
      }
      return {
        code: 'export { default } from "./react.js"\nexport * from "./react.js"\n',
        map: null,
      }
    },
  }
}

export default defineConfig({
  build: {
    assetsInlineLimit: 2048,
    rolldownOptions: {
      // This is overridden by bldr, but useful for "vite build" testing.
      input: {
        app: resolve(__dirname, './app/App.tsx'),
      },
      preserveEntrySignatures: 'strict',
      output: {
        format: 'es',
      },
    },
  },

  resolve: {
    dedupe: ['react', 'react-dom'],
    alias: [
      ...buildGoAliases(__dirname),
      {
        find: '@aptre/bldr',
        replacement: resolveBldrSourcePath('web/bldr/index.js'),
      },
      {
        find: '@aptre/bldr-react',
        replacement: resolveBldrSourcePath('web/bldr-react/index.js'),
      },
      {
        find: /^@aptre\/bldr-sdk$/,
        replacement: resolveBldrSourcePath('sdk/plugin.ts'),
      },
      {
        find: /^@aptre\/bldr-sdk\/(.*)$/,
        replacement: resolveBldrSourcePath('sdk/$1'),
      },
      {
        find: /^@s4wave\/app\/(.*)$/,
        replacement: resolve(__dirname, './app/$1'),
      },
      {
        find: '@s4wave/app',
        replacement: resolve(__dirname, './app'),
      },
      {
        find: /^@s4wave\/web\/(.*)$/,
        replacement: resolve(__dirname, './web/$1'),
      },
      {
        find: '@s4wave/web',
        replacement: resolve(__dirname, './web'),
      },
      {
        find: /^@s4wave\/core\/(.*)$/,
        replacement: resolve(__dirname, './core/$1'),
      },
      {
        find: /^@s4wave\/sdk\/(.*)$/,
        replacement: resolve(__dirname, './sdk/$1'),
      },
      {
        find: '@s4wave/sdk',
        replacement: resolve(__dirname, './sdk'),
      },
    ],
  },

  plugins: [
    canonicalReactOptimizedDep(),
    react(),
    tailwindcss(),
    goTsResolver(__dirname),
  ],
})
