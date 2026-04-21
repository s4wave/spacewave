// Vite config for building worker comms test fixtures.
// Each *.ts file in fixtures/ (excluding workers/) is built as an ES module.
// The Go test server generates HTML pages that load each fixture.

import { defineConfig } from 'vite'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'
import { readdirSync } from 'fs'
import { buildGoAliases, goTsResolver } from '../../web/bundler/vite/go-ts-resolver.js'

const __dirname = dirname(fileURLToPath(import.meta.url))
const repoRoot = resolve(__dirname, '../../..')
const bldrRoot = resolve(__dirname, '../..')
const fixturesDir = resolve(__dirname, 'fixtures')

// Discover fixture entry points (*.ts files in fixtures/).
const entries: Record<string, string> = {}
for (const file of readdirSync(fixturesDir)) {
  if (file.endsWith('.ts') && !file.startsWith('_')) {
    entries[file.replace('.ts', '')] = resolve(fixturesDir, file)
  }
}

// Discover worker entry points (fixtures/workers/*.ts).
const workersDir = resolve(fixturesDir, 'workers')
try {
  for (const file of readdirSync(workersDir)) {
    if (file.endsWith('.ts')) {
      entries['workers/' + file.replace('.ts', '')] = resolve(workersDir, file)
    }
  }
} catch {
  // workers/ dir may not exist yet
}

export default defineConfig({
  plugins: [goTsResolver(repoRoot)],
  resolve: {
    alias: [
      ...buildGoAliases(repoRoot),
      {
        find: /^@aptre\/bldr$/,
        replacement: resolve(bldrRoot, 'web/bldr/index.js'),
      },
      {
        find: /^@aptre\/bldr-react$/,
        replacement: resolve(bldrRoot, 'web/bldr-react/index.js'),
      },
      {
        find: /^@aptre\/bldr-sdk$/,
        replacement: resolve(bldrRoot, 'sdk/plugin.ts'),
      },
      {
        find: /^@aptre\/bldr-sdk\/(.*)$/,
        replacement: resolve(bldrRoot, 'sdk/$1'),
      },
    ],
  },
  build: {
    outDir: resolve(__dirname, 'dist'),
    emptyDirBeforeWrite: true,
    lib: {
      entry: entries,
      formats: ['es'],
    },
    rollupOptions: {
      output: {
        entryFileNames: '[name].js',
        chunkFileNames: 'chunks/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash][extname]',
      },
    },
    minify: false,
    sourcemap: true,
  },
})
