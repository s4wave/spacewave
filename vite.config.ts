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

// optimizedDepVersionImports keeps React-dependent optimized dependency chunks
// on one browser module URL. Vite 8 appends ?v=... to optimized dep entries
// imported by source modules, while shared chunks import each other without the
// query; React and ReactDOM then initialize separate dispatcher singletons.
function optimizedDepVersionImports(): Plugin {
  const rewriteImportSource = (
    code: string,
    versionQuery: string,
    pattern: RegExp,
  ) => {
    return code.replace(pattern, (match, prefix, quote, source, suffix = '') => {
      if (source.includes('?')) return match
      return `${prefix}${quote}${source}${versionQuery}${suffix}`
    })
  }
  const reactOptimizedDepImport =
    /["']\.\/(?:react(?:_jsx(?:-dev)?-runtime)?|react-dom(?:_client)?|client-[^"']+)\.js["']/
  const optimizedDepSource = "\\./[^\"']+\\.js"

  return {
    name: 'spacewave-optimized-dep-version-imports',
    apply: 'serve',
    transform(code, id) {
      const queryStart = id.indexOf('?')
      if (queryStart === -1) return null
      const file = id.slice(0, queryStart)
      if (!file.includes('/node_modules/.vite/') || !file.endsWith('.js')) {
        return null
      }
      if (!reactOptimizedDepImport.test(code)) return null
      const version = new URLSearchParams(id.slice(queryStart + 1)).get('v')
      if (!version) return null
      const versionQuery = `?v=${version}`
      let rewritten = rewriteImportSource(
        code,
        versionQuery,
        new RegExp(`(\\bfrom\\s*)(["'])(${optimizedDepSource})(["'])`, 'g'),
      )
      rewritten = rewriteImportSource(
        rewritten,
        versionQuery,
        new RegExp(`(\\bimport\\s*)(["'])(${optimizedDepSource})(["'])`, 'g'),
      )
      rewritten = rewriteImportSource(
        rewritten,
        versionQuery,
        new RegExp(
          `(\\bimport\\s*\\(\\s*)(["'])(${optimizedDepSource})(["']\\s*\\))`,
          'g',
        ),
      )
      if (rewritten === code) return null
      return { code: rewritten, map: null }
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
    optimizedDepVersionImports(),
    react(),
    tailwindcss(),
    goTsResolver(__dirname),
  ],
})
