import { defineConfig } from 'vite'
import { resolve } from 'path'
import { existsSync } from 'node:fs'
import { dirname } from 'node:path'
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
  const isPatternReplacement = segments.some((segment) =>
    segment.includes('$'),
  )
  const monorepoPath = resolve(bldrDistRoot, 'bldr', ...segments)
  if (existsSync(monorepoPath)) {
    return monorepoPath
  }
  const distPath = resolve(bldrDistRoot, ...segments)
  if (isPatternReplacement || existsSync(distPath)) {
    return distPath
  }
  return resolve(__dirname, 'bldr', ...segments)
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
    react(),
    tailwindcss(),
    goTsResolver(__dirname),
  ],
})
