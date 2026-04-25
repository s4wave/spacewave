import { defineConfig } from 'vite'
import { resolve } from 'path'
import { existsSync } from 'node:fs'
import { dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

const __dirname = dirname(fileURLToPath(import.meta.url))

function resolveBldrAliasPath(...segments: string[]) {
  const distPath = resolve(__dirname, '.bldr/src', ...segments)
  if (existsSync(distPath)) {
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
      {
        find: '@aptre/bldr',
        replacement: resolveBldrAliasPath('web/bldr/index.js'),
      },
      {
        find: '@aptre/bldr-react',
        replacement: resolveBldrAliasPath('web/bldr-react/index.js'),
      },
      {
        find: /^@aptre\/bldr-sdk\/(.*)$/,
        replacement: resolve(__dirname, './.bldr/src/sdk/$1'),
      },
      {
        find: /^@go\/(.*)$/,
        replacement: resolve(__dirname, './vendor/$1'),
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
    // goTsResolver is injected by bldr's vite-base.config.ts during builds.
  ],
})
