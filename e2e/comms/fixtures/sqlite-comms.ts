// sqlite-comms.ts - SQLite cross-tab communication test fixture.
//
// Modes (via URL ?mode= param):
//   (none/single): Single-page round-trip: CommsWriter -> same DB -> CommsReader
//   writer: Write message to in-memory DB, export to OPFS, signal ready
//   reader: Load DB from OPFS via AsyncOpfsDb, read via CommsReader

import sqlite3Init, {
  type Sqlite3Static,
  type Database,
} from '@aptre/sqlite-wasm'
import {
  initCommsSchema,
  CommsWriter,
  CommsReader,
  COMMS_DB_FILENAME,
} from '../../../web/bldr/comms-table.js'
import {
  AsyncOpfsDb,
  COMMS_BROADCAST_CHANNEL,
  type CommsNotification,
} from '../../../web/runtime/wasm/sqlite/async-opfs.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      roundTrip?: boolean
      bcNotification?: boolean
      crossTabRead?: boolean
    }
  }
}

const OPFS_COMMS_DIR = '.bldr-comms-test'
const TEST_PAYLOAD = [0xde, 0xad, 0xbe, 0xef]
const SOURCE_PLUGIN = 10
const TARGET_PLUGIN = 20

async function loadSqlite3(): Promise<Sqlite3Static> {
  return (
    sqlite3Init as (config?: Record<string, unknown>) => Promise<Sqlite3Static>
  )({
    locateFile: (path: string) => {
      if (path.endsWith('.wasm')) return '/sqlite3.wasm'
      return path
    },
  })
}

// Write database bytes to OPFS at the test directory.
async function writeToOpfs(data: Uint8Array): Promise<void> {
  const root = await navigator.storage.getDirectory()
  const dir = await root.getDirectoryHandle(OPFS_COMMS_DIR, { create: true })
  const fileHandle = await dir.getFileHandle(COMMS_DB_FILENAME, {
    create: true,
  })
  const writable = await fileHandle.createWritable()
  await writable.write(data)
  await writable.close()
}

// Clean up OPFS test directory.
async function cleanOpfs(): Promise<void> {
  const root = await navigator.storage.getDirectory()
  try {
    await root.removeEntry(OPFS_COMMS_DIR, { recursive: true })
  } catch {
    // Directory may not exist.
  }
}

// Single-page round-trip test.
async function runSingle(): Promise<{
  roundTrip: boolean
  bcNotification: boolean
}> {
  const sqlite3 = await loadSqlite3()
  const db = new sqlite3.oo1.DB(':memory:', 'cw')

  const writer = new CommsWriter(db)
  const payload = new Uint8Array(TEST_PAYLOAD)

  // Listen for BroadcastChannel notification.
  let bcNotification = false
  const bc = new BroadcastChannel(COMMS_BROADCAST_CHANNEL)
  const bcPromise = new Promise<void>((resolve) => {
    bc.onmessage = (ev: MessageEvent<CommsNotification>) => {
      if (ev.data.table === 'messages') {
        bcNotification = true
        resolve()
      }
    }
    setTimeout(resolve, 3000)
  })

  writer.write(SOURCE_PLUGIN, TARGET_PLUGIN, payload)
  await bcPromise
  bc.close()

  // Read back.
  const reader = new CommsReader()
  const msgs = reader.readNew(db, TARGET_PLUGIN)

  let roundTrip = false
  if (
    msgs.length === 1 &&
    msgs[0].sourcePluginId === SOURCE_PLUGIN &&
    msgs[0].targetPluginId === TARGET_PLUGIN &&
    msgs[0].payload[0] === 0xde &&
    msgs[0].payload[3] === 0xef
  ) {
    roundTrip = true
  }

  writer.close()
  db.close()

  return { roundTrip, bcNotification }
}

// Writer mode: create DB, write message, export to OPFS.
async function runWriter(): Promise<void> {
  await cleanOpfs()

  const sqlite3 = await loadSqlite3()
  const db = new sqlite3.oo1.DB(':memory:', 'cw')
  initCommsSchema(db)

  // Write message using raw SQL (CommsWriter would send BC notification too early).
  db.exec({
    sql: `INSERT INTO messages (source_plugin_id, target_plugin_id, payload)
          VALUES (?, ?, ?)`,
    bind: [SOURCE_PLUGIN, TARGET_PLUGIN, new Uint8Array(TEST_PAYLOAD)],
  })

  // Export DB to bytes and write to OPFS.
  const bytes = sqlite3.capi.sqlite3_js_db_export(db)
  await writeToOpfs(bytes)
  db.close()

  // Now send BroadcastChannel notification (after OPFS write is complete).
  const bc = new BroadcastChannel(COMMS_BROADCAST_CHANNEL)
  const notification: CommsNotification = { table: 'messages', seq: 1 }
  bc.postMessage(notification)
  bc.close()

  window.__results = {
    pass: true,
    detail: 'writer: wrote message to OPFS',
  }
}

// Reader mode: load from OPFS via AsyncOpfsDb, read via CommsReader.
async function runReader(): Promise<void> {
  const sqlite3 = await loadSqlite3()

  // AsyncOpfsDb reads from OPFS_COMMS_DIR but the constant is hardcoded
  // in async-opfs.ts as '.bldr-comms'. For the test, we use the same approach
  // but with our test directory.
  const root = await navigator.storage.getDirectory()
  const dir = await root.getDirectoryHandle(OPFS_COMMS_DIR, { create: false })
  const fileHandle = await dir.getFileHandle(COMMS_DB_FILENAME, {
    create: false,
  })
  const file = await fileHandle.getFile()
  const data = await file.arrayBuffer()

  // Load into in-memory DB.
  const db = new sqlite3.oo1.DB(':memory:', 'cw')
  if (data.byteLength > 0) {
    const bytes = new Uint8Array(data)
    const ptr = sqlite3.wasm.allocFromTypedArray(bytes)
    const rc = sqlite3.capi.sqlite3_deserialize(
      db,
      'main',
      ptr,
      bytes.byteLength,
      bytes.byteLength,
      sqlite3.capi.SQLITE_DESERIALIZE_FREEONCLOSE |
        sqlite3.capi.SQLITE_DESERIALIZE_READONLY,
    )
    if (rc !== 0) {
      sqlite3.wasm.dealloc(ptr)
      db.close()
      throw new Error(`sqlite3_deserialize failed: rc=${rc}`)
    }
  }

  // Read via CommsReader.
  const reader = new CommsReader()
  const msgs = reader.readNew(db, TARGET_PLUGIN)

  let crossTabRead = false
  if (
    msgs.length === 1 &&
    msgs[0].sourcePluginId === SOURCE_PLUGIN &&
    msgs[0].targetPluginId === TARGET_PLUGIN &&
    msgs[0].payload[0] === 0xde &&
    msgs[0].payload[3] === 0xef
  ) {
    crossTabRead = true
  }

  db.close()
  await cleanOpfs()

  window.__results = {
    pass: crossTabRead,
    detail: crossTabRead
      ? 'reader: cross-tab message verified'
      : `reader: failed, got ${msgs.length} msgs`,
    crossTabRead,
  }
}

async function run() {
  const log = document.getElementById('log')!
  const params = new URLSearchParams(location.search)
  const mode = params.get('mode') || 'single'

  try {
    if (mode === 'writer') {
      await runWriter()
    } else if (mode === 'reader') {
      await runReader()
    } else {
      // Single-page round-trip.
      const { roundTrip, bcNotification } = await runSingle()
      const pass = roundTrip && bcNotification
      window.__results = {
        pass,
        detail: pass
          ? 'all tests passed'
          : `roundTrip=${roundTrip} bc=${bcNotification}`,
        roundTrip,
        bcNotification,
      }
    }
  } catch (err) {
    window.__results = {
      pass: false,
      detail: `error: ${err}`,
    }
  }

  log.textContent = 'DONE'
}

run()
