import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { resolve } from 'path'
import { access } from 'node:fs/promises'
import { dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))

// Detect if we are running in .bldr or not.
const bldrProjectRoot =
  process.env['BLDR_PROJECT_ROOT'] || resolve(__dirname, '../../../')
const bldrDistRoot =
  process.env['BLDR_DIST_ROOT'] || resolve(bldrProjectRoot, '.bldr/src')

// https://vite.dev/config/
export default defineConfig({
  build: {
    outDir: './vite-dist',
    rollupOptions: {
      output: {},
    },
    minify: false,
    sourcemap: true,
    cssCodeSplit: true,
    terserOptions: {
      compress: false,
      mangle: false,
    },
  },
  define: { 'process.env.NODE_ENV': '"production"' },
  resolve: {
    alias: {
      '@go/*': resolve(bldrProjectRoot, './vendor/*'),
      '@aptre/bldr': resolve(bldrDistRoot, './web/bldr/index.js'),
      '@aptre/bldr-react': resolve(bldrDistRoot, './web/bldr-react/index.js'),
    },
  },
  plugins: [
    react(),
    // This plugin fixes issues with resolving paths like @go/foo/bar/baz.js where baz.ts exists only.
    {
      name: 'go-ts-resolver',
      async resolveId(source) {
        // Handle only @go/ paths that end in .js
        if (!source.startsWith('@go/') || !source.endsWith('.js')) {
          return null
        }

        // Convert @go/ path to vendor path
        const vendorPath = resolve(
          bldrProjectRoot,
          './vendor',
          source.slice('@go/'.length),
        )
        const tsPath = vendorPath.replace(/\.js$/, '.ts')

        try {
          await access(tsPath)
          return tsPath
        } catch {
          return null
        }
      },
    },
  ],
})
