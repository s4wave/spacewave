import { defineConfig } from 'vite'
import { resolve } from 'path'
import { dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { buildGoAliases, goTsResolver } from './go-ts-resolver.js'

const __dirname = dirname(fileURLToPath(import.meta.url))

// Detect if we are running in a monorepo checkout or a generated .bldr tree.
const bldrProjectRoot =
  process.env['BLDR_PROJECT_ROOT'] || resolve(__dirname, '../../../../')
const bldrSourceRoot = resolve(__dirname, '../../../')
const bldrDistRoot =
  process.env['BLDR_DIST_ROOT'] || resolve(bldrSourceRoot, '.bldr/src')

// Use inline sourcemaps in development for faster builds.
const isDevelopment = process.env.NODE_ENV !== 'production'

// https://vite.dev/config/
export default defineConfig({
  build: {
    outDir: './vite-dist',
    rolldownOptions: {
      output: {},
    },
    minify: false,
    sourcemap: isDevelopment ? 'inline' : true,
    cssCodeSplit: true, // set to true by Go as well.
    manifest: true, // set to true by Go as well.
  },
  define: {
    'process.env.NODE_ENV': JSON.stringify(
      process.env.NODE_ENV || 'development',
    ),
  },
  resolve: {
    alias: [
      ...buildGoAliases(bldrProjectRoot),
      {
        find: /^@aptre\/bldr$/,
        replacement: resolve(bldrDistRoot, './web/bldr/index.js'),
      },
      {
        find: /^@aptre\/bldr-react$/,
        replacement: resolve(bldrDistRoot, './web/bldr-react/index.js'),
      },
    ],
  },
  plugins: [
    // This plugin fixes issues with resolving paths like @go/foo/bar/baz.js where baz.ts exists only.
    goTsResolver(bldrProjectRoot),
  ],
})
