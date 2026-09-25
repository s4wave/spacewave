// sqlite-records is the device worker of the E3 volume replay target: a
// records store in one SQLite database on the opfs-sahpool VFS.
//
// Each request is {id, op, ...args}; each response is {id, error?, ...result}.
// Requests run one at a time in arrival order.

import sqlite3InitModule from '@sqlite.org/sqlite-wasm'
import type { Database, SAHPoolUtil } from '@sqlite.org/sqlite-wasm'

// Op is one write in a commit; an absent value deletes the key.
interface Op {
  key: Uint8Array
  value?: Uint8Array
}

// Request is one call from the store client.
type Request = { id: number } & (
  | { op: 'open'; name: string }
  | { op: 'close' | 'destroy' }
  | { op: 'get' | 'has'; keys: Uint8Array[] }
  | { op: 'scan'; prefix: Uint8Array }
  | { op: 'commit'; ops: Op[]; durable: boolean }
)

// state holds the open database and the tail of the request queue.
const state: { db?: Database; queue: Promise<void> } = {
  queue: Promise.resolve(),
}

// pool is the VFS every database opens on, installed once per worker.
const pool: Promise<SAHPoolUtil> = sqlite3InitModule().then((sqlite3) =>
  sqlite3.installOpfsSAHPoolVfs({
    name: 'volume-replay-sahpool',
    directory: '.volume-replay-sahpool',
  }),
)

// prefixEnd returns the least key greater than every key with prefix, or
// undefined when prefix is empty or all 0xff.
function prefixEnd(prefix: Uint8Array): Uint8Array | undefined {
  for (let i = prefix.length - 1; i >= 0; i--) {
    if (prefix[i] !== 0xff) {
      const end = prefix.slice(0, i + 1)
      end[i]++
      return end
    }
  }
  return undefined
}

// open opens the database at path, creating its table on first use.
async function open(path: string): Promise<Database> {
  const db = new (await pool).OpfsSAHPoolDb(path)
  db.exec([
    'PRAGMA journal_mode=PERSIST;',
    'PRAGMA locking_mode=EXCLUSIVE;',
    'CREATE TABLE IF NOT EXISTS records (key BLOB PRIMARY KEY, value BLOB NOT NULL) WITHOUT ROWID;',
  ])
  return db
}

// scan returns the records under prefix in key order.
function scan(db: Database, prefix: Uint8Array) {
  const end = prefixEnd(prefix)
  const rows = end
    ? db.selectArrays(
        'SELECT key, value FROM records WHERE key >= ? AND key < ? ORDER BY key',
        [prefix, end],
      )
    : db.selectArrays(
        'SELECT key, value FROM records WHERE key >= ? ORDER BY key',
        [prefix],
      )
  return { keys: rows.map((row) => row[0]), values: rows.map((row) => row[1]) }
}

// commit applies ops in one transaction, syncing it when durable is set.
function commit(db: Database, ops: Op[], durable: boolean) {
  db.exec(durable ? 'PRAGMA synchronous=FULL' : 'PRAGMA synchronous=OFF')
  db.transaction(() => {
    const put = db.prepare(
      'INSERT OR REPLACE INTO records (key, value) VALUES (?, ?)',
    )
    const del = db.prepare('DELETE FROM records WHERE key = ?')
    try {
      for (const op of ops) {
        if (op.value === undefined) {
          del.bind([op.key]).stepReset()
          continue
        }
        put.bind([op.key, op.value]).stepReset()
      }
    } finally {
      put.finalize()
      del.finalize()
    }
  })
  return {}
}

// close closes the open database, if any.
function close() {
  state.db?.close()
  state.db = undefined
}

// handle runs one request.
async function handle(req: Request): Promise<object> {
  if (req.op === 'open') {
    close()
    state.db = await open('/' + req.name + '.db')
    return {}
  }
  if (req.op === 'close') {
    close()
    return {}
  }
  if (req.op === 'destroy') {
    close()
    await (await pool).wipeFiles()
    return {}
  }

  const db = state.db
  if (!db) {
    throw new Error('database is not open')
  }
  switch (req.op) {
    case 'get':
      return {
        values: req.keys.map((key) =>
          db.selectValue('SELECT value FROM records WHERE key = ?', [key]),
        ),
      }
    case 'has':
      return {
        has: req.keys.map(
          (key) =>
            db.selectValue('SELECT 1 FROM records WHERE key = ?', [key]) !==
            undefined,
        ),
      }
    case 'scan':
      return scan(db, req.prefix)
    case 'commit':
      return commit(db, req.ops, req.durable)
  }
}

self.addEventListener('message', (ev: MessageEvent<Request>) => {
  const req = ev.data
  state.queue = state.queue.then(async () => {
    try {
      self.postMessage({ id: req.id, ...(await handle(req)) })
    } catch (err) {
      self.postMessage({ id: req.id, error: String(err) })
    }
  })
})
