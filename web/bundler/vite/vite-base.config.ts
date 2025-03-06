import { defineConfig } from 'vite'
import { resolve } from 'path'
import { dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { goTsResolver } from './go-ts-resolver.js'

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
  define: { 'process.env.NODE_ENV': JSON.stringify(process.env.NODE_ENV || 'development') },
  resolve: {
    alias: {
      '@go/*': resolve(bldrProjectRoot, './vendor/*'),
      '@aptre/bldr': resolve(bldrDistRoot, './web/bldr/index.js'),
      '@aptre/bldr-react': resolve(bldrDistRoot, './web/bldr-react/index.js'),
    },
  },
  plugins: [
    // This plugin fixes issues with resolving paths like @go/foo/bar/baz.js where baz.ts exists only.
    goTsResolver(bldrProjectRoot),
  ],
})
