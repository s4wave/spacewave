// Vite config for building worker comms test fixtures.
// Each *.ts file in fixtures/ (excluding workers/) is built as an ES module.
// The Go test server generates HTML pages that load each fixture.

import process from 'node:process'
import { readdirSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vite'
import {
  buildGoAliases,
  goTsResolver,
} from '../../web/bundler/vite/go-ts-resolver.js'

const __dirname = dirname(fileURLToPath(import.meta.url))
const repoRoot = resolve(__dirname, '../../..')
const bldrRoot = resolve(__dirname, '../..')
const fixturesDir = resolve(__dirname, 'fixtures')
const workersDir = resolve(fixturesDir, 'workers')

// Classic ServiceWorker entries must not participate in the shared fixture
// chunk graph: Firefox does not support module ServiceWorkers, so these outputs
// must be self-contained classic scripts with no import statements.
const classicServiceWorkerEntries: Record<string, string> = {
  'cross-tab-sw': resolve(fixturesDir, 'cross-tab-sw.ts'),
  'workers/webdocument-relay-service-worker': resolve(
    workersDir,
    'webdocument-relay-service-worker.ts',
  ),
}

const classicServiceWorkerEntry =
  process.env.BLDR_COMMS_CLASSIC_SERVICE_WORKER_ENTRY

function discoverFixtureEntries(): Record<string, string> {
  const entries: Record<string, string> = {}
  for (const file of readdirSync(fixturesDir)) {
    if (file.endsWith('.ts') && !file.startsWith('_')) {
      const name = file.replace('.ts', '')
      if (!(name in classicServiceWorkerEntries)) {
        entries[name] = resolve(fixturesDir, file)
      }
    }
  }

  try {
    for (const file of readdirSync(workersDir)) {
      if (file.endsWith('.ts')) {
        const name = 'workers/' + file.replace('.ts', '')
        if (!(name in classicServiceWorkerEntries)) {
          entries[name] = resolve(workersDir, file)
        }
      }
    }
  } catch {
    // workers/ dir may not exist yet.
  }

  return entries
}

let entries: Record<string, string>
if (classicServiceWorkerEntry === undefined) {
  entries = discoverFixtureEntries()
} else {
  const entryPath = classicServiceWorkerEntries[classicServiceWorkerEntry]
  if (!entryPath) {
    throw new Error(
      `unknown classic ServiceWorker fixture entry: ${classicServiceWorkerEntry}`,
    )
  }
  entries = {
    [classicServiceWorkerEntry]: entryPath,
  }
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
    emptyDirBeforeWrite: classicServiceWorkerEntry === undefined,
    emptyOutDir: classicServiceWorkerEntry === undefined,
    lib: {
      entry: entries,
      formats: ['es'],
    },
    rollupOptions: {
      output: {
        entryFileNames: '[name].js',
        chunkFileNames: 'chunks/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash][extname]',
        codeSplitting: classicServiceWorkerEntry === undefined,
      },
    },
    minify: false,
    sourcemap: true,
  },
})
