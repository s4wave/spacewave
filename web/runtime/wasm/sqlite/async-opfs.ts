// async-opfs.ts provides an async OPFS-backed sqlite database for use in
// SharedWorker and other contexts where createSyncAccessHandle is unavailable.
//
// Pattern: reads the DB file from OPFS using async FileSystemFileHandle APIs,
// deserializes it into an in-memory sqlite database. Supports refresh on demand
// (e.g. triggered by BroadcastChannel notification). DedicatedWorkers write to
// OPFS using the sync VFS; SharedWorker reads via this async loader.

import type { Sqlite3Static, Database } from '@aptre/sqlite-wasm'

// OPFS_COMMS_DIR is the OPFS directory for cross-tab communication databases.
const OPFS_COMMS_DIR = '.bldr-comms'

// AsyncOpfsDb wraps an in-memory sqlite database backed by an OPFS file.
// The database is read-only from the perspective of this context; writes
// happen in DedicatedWorker contexts using sync OPFS.
export class AsyncOpfsDb {
  // sqlite3 is the initialized sqlite3 API.
  private sqlite3: Sqlite3Static
  // db is the current in-memory database instance.
  private db: Database | null = null
  // dirHandle is the OPFS directory handle.
  private dirHandle: FileSystemDirectoryHandle | null = null
  // filename is the database filename within the OPFS directory.
  private filename: string

  constructor(sqlite3: Sqlite3Static, filename: string) {
    this.sqlite3 = sqlite3
    this.filename = filename
  }

  // open initializes the OPFS directory handle and performs the initial load.
  // If the file does not exist yet, creates an empty in-memory database.
  async open(): Promise<void> {
    const root = await navigator.storage.getDirectory()
    this.dirHandle = await root.getDirectoryHandle(OPFS_COMMS_DIR, {
      create: true,
    })
    await this.refresh()
  }

  // refresh re-reads the OPFS file and replaces the in-memory database.
  // Call this when a BroadcastChannel notification indicates new data.
  async refresh(): Promise<void> {
    if (!this.dirHandle) {
      throw new Error('AsyncOpfsDb: not opened')
    }

    let data: ArrayBuffer | null = null
    try {
      const fileHandle = await this.dirHandle.getFileHandle(this.filename, {
        create: false,
      })
      const file = await fileHandle.getFile()
      if (file.size > 0) {
        data = await file.arrayBuffer()
      }
    } catch (err: unknown) {
      if (err instanceof DOMException && err.name === 'NotFoundError') {
        // File does not exist yet, start with empty DB.
        data = null
      } else {
        throw err
      }
    }

    // Close the old database.
    if (this.db) {
      this.db.close()
      this.db = null
    }

    // Open a new in-memory database.
    const db = new this.sqlite3.oo1.DB(':memory:', 'cw')

    if (data && data.byteLength > 0) {
      // Allocate WASM memory and copy the file contents.
      const bytes = new Uint8Array(data)
      const ptr = this.sqlite3.wasm.allocFromTypedArray(bytes)
      const rc = this.sqlite3.capi.sqlite3_deserialize(
        db.pointer!,
        'main',
        ptr,
        bytes.byteLength,
        bytes.byteLength,
        // FREEONCLOSE: sqlite frees the pointer when the DB closes.
        // READONLY: this is a read-only snapshot.
        this.sqlite3.capi.SQLITE_DESERIALIZE_FREEONCLOSE |
          this.sqlite3.capi.SQLITE_DESERIALIZE_READONLY,
      )
      if (rc !== 0) {
        this.sqlite3.wasm.dealloc(ptr)
        db.close()
        throw new Error(`sqlite3_deserialize failed: rc=${rc}`)
      }
    }

    this.db = db
  }

  // getDb returns the current in-memory database, or null if not loaded.
  getDb(): Database | null {
    return this.db
  }

  // exec runs a read-only SQL statement and returns the results.
  exec(sql: string, bind?: readonly unknown[]): unknown[][] {
    if (!this.db) {
      throw new Error('AsyncOpfsDb: database not loaded')
    }
    return this.db.exec({ sql, bind: bind as never, returnValue: 'resultRows' })
  }

  // close disposes the in-memory database.
  close(): void {
    if (this.db) {
      this.db.close()
      this.db = null
    }
    this.dirHandle = null
  }
}

// COMMS_BROADCAST_CHANNEL is the BroadcastChannel name for cross-tab
// sqlite change notifications.
export const COMMS_BROADCAST_CHANNEL = 'bldr-comms-sqlite'

// CommsNotification is the notification payload sent over BroadcastChannel
// when a writer updates the cross-tab communication database.
export interface CommsNotification {
  // table is the table that was modified.
  table: string
  // seq is a monotonically increasing sequence number from the writer.
  seq: number
}
