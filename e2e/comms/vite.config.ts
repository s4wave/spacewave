// Vite config for building worker comms test fixtures.
// Each *.ts file in fixtures/ (excluding workers/) is built as an ES module.
// The Go test server generates HTML pages that load each fixture.

import { defineConfig, type Plugin } from 'vite'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'
import { readdirSync, copyFileSync, mkdirSync } from 'fs'

const __dirname = dirname(fileURLToPath(import.meta.url))
const repoRoot = resolve(__dirname, '../..')
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

// Plugin to copy sqlite3.wasm to dist for test fixtures.
function sqliteWasmPlugin(): Plugin {
  const wasmSrc = resolve(repoRoot, 'node_modules/@aptre/sqlite-wasm/dist/sqlite3.wasm')
  return {
    name: 'sqlite-wasm-copy',
    writeBundle() {
      const outDir = resolve(__dirname, 'dist')
      mkdirSync(outDir, { recursive: true })
      copyFileSync(wasmSrc, resolve(outDir, 'sqlite3.wasm'))
    },
  }
}

export default defineConfig({
  resolve: {
    alias: {
      '@go': resolve(repoRoot, 'vendor'),
      '@aptre/bldr': resolve(repoRoot, 'web/bldr/index.js'),
      '@aptre/bldr-react': resolve(repoRoot, 'web/bldr-react/index.js'),
      '@aptre/bldr-sdk': resolve(repoRoot, 'sdk/plugin.ts'),
      '@aptre/bldr-sdk/': resolve(repoRoot, 'sdk') + '/',
    },
  },
  plugins: [sqliteWasmPlugin()],
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
