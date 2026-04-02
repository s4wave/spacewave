import sqlite3Init, { type Sqlite3Static, type SAHPoolUtil } from '@sqlite.org/sqlite-wasm'
// The package exports ./sqlite3.wasm as a named export.
// esbuild's file loader copies it to the output and returns the URL.
import sqlite3WasmUrl from '@sqlite.org/sqlite-wasm/sqlite3.wasm'

// SqliteLoadResult holds the result of loading and initializing sqlite.wasm.
export interface SqliteLoadResult {
  // sqlite3 is the initialized sqlite3 API.
  sqlite3: Sqlite3Static
  // vfsName is the VFS that was selected ("opfs" or "opfs-sahpool").
  vfsName: string
  // sahPool is the SAHPool utility, set when using opfs-sahpool VFS.
  sahPool: SAHPoolUtil | null
}

// SAHPOOL_DIR is the OPFS directory for the SAH pool metadata.
const SAHPOOL_DIR = '.hydra-sqlite'

// SAHPOOL_INITIAL_CAPACITY is the initial capacity of the SAH pool.
// Must be at least 2x expected databases (db + journal) plus temp files.
const SAHPOOL_INITIAL_CAPACITY = 24

// loadSqlite loads sqlite.wasm and initializes the OPFS VFS for the dedicated
// sqlite worker. Tries the full opfs VFS first (requires SharedArrayBuffer +
// COOP/COEP). Falls back to opfs-sahpool if unavailable. Throws if neither
// OPFS VFS is available.
export async function loadSqlite(): Promise<SqliteLoadResult> {
  // Cast needed: the .d.mts types declare init() with no args, but the
  // Emscripten module accepts a config object at runtime.
  const sqlite3 = await (sqlite3Init as (config?: Record<string, unknown>) => Promise<Sqlite3Static>)({
    locateFile: (path: string) => {
      if (path.endsWith('.wasm')) {
        return sqlite3WasmUrl
      }
      return path
    },
  })
  console.log(
    'sqlite: loaded version',
    sqlite3.version.libVersion,
  )

  // Try the full opfs VFS first (better concurrency, multi-connection).
  const hasOpfs = Boolean(sqlite3.capi.sqlite3_vfs_find('opfs'))
  if (hasOpfs) {
    console.log('sqlite: using opfs VFS')
    return { sqlite3, vfsName: 'opfs', sahPool: null }
  }

  // Fall back to opfs-sahpool.
  console.log('sqlite: opfs VFS unavailable, installing opfs-sahpool')
  const sahPool = await sqlite3.installOpfsSAHPoolVfs({
    directory: SAHPOOL_DIR,
    initialCapacity: SAHPOOL_INITIAL_CAPACITY,
  })
  console.log(
    'sqlite: using opfs-sahpool VFS, capacity:',
    sahPool.getCapacity(),
  )

  return { sqlite3, vfsName: sahPool.vfsName, sahPool }
}
