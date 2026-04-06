// snapshot-manager.ts manages periodic and urgent WASM memory snapshots to
// OPFS (Chrome/Firefox) or IndexedDB (Safari fallback) storage. Enables
// recovery of plugin state after tab close.
//
// Each registered plugin's WASM memory is snapshot to a file named by plugin ID.
// Snapshots are full copies of WebAssembly.Memory.buffer.

// OPFS_SNAPSHOT_DIR is the OPFS directory for WASM memory snapshots.
const OPFS_SNAPSHOT_DIR = '.bldr-snapshots'

// IDB_STORE is the IndexedDB database and store name for snapshots.
const IDB_DB_NAME = 'bldr-snapshots'
const IDB_STORE_NAME = 'snapshots'

// SnapshotEntry tracks a registered plugin's WASM memory.
interface SnapshotEntry {
  pluginId: string
  memory: WebAssembly.Memory
}

// SnapshotStorage abstracts OPFS vs IDB storage.
interface SnapshotStorage {
  write(pluginId: string, data: ArrayBuffer): Promise<void>
  read(pluginId: string): Promise<ArrayBuffer | null>
  delete(pluginId: string): Promise<void>
  list(): Promise<string[]>
}

// OpfsSnapshotStorage uses OPFS for snapshot persistence.
class OpfsSnapshotStorage implements SnapshotStorage {
  private dirHandle: FileSystemDirectoryHandle | null = null

  async init(): Promise<void> {
    const root = await navigator.storage.getDirectory()
    this.dirHandle = await root.getDirectoryHandle(OPFS_SNAPSHOT_DIR, {
      create: true,
    })
  }

  async write(pluginId: string, data: ArrayBuffer): Promise<void> {
    if (!this.dirHandle) throw new Error('OpfsSnapshotStorage: not initialized')
    const fileHandle = await this.dirHandle.getFileHandle(pluginId, {
      create: true,
    })
    const writable = await fileHandle.createWritable()
    await writable.write(data)
    await writable.close()
  }

  async read(pluginId: string): Promise<ArrayBuffer | null> {
    if (!this.dirHandle) throw new Error('OpfsSnapshotStorage: not initialized')
    try {
      const fileHandle = await this.dirHandle.getFileHandle(pluginId, {
        create: false,
      })
      const file = await fileHandle.getFile()
      if (file.size === 0) return null
      return file.arrayBuffer()
    } catch (err: unknown) {
      if (err instanceof DOMException && err.name === 'NotFoundError') {
        return null
      }
      throw err
    }
  }

  async delete(pluginId: string): Promise<void> {
    if (!this.dirHandle) throw new Error('OpfsSnapshotStorage: not initialized')
    try {
      await this.dirHandle.removeEntry(pluginId)
    } catch (err: unknown) {
      if (err instanceof DOMException && err.name === 'NotFoundError') {
        return
      }
      throw err
    }
  }

  async list(): Promise<string[]> {
    if (!this.dirHandle) throw new Error('OpfsSnapshotStorage: not initialized')
    const ids: string[] = []
    for await (const key of (this.dirHandle as any).keys()) {
      ids.push(key as string)
    }
    return ids
  }
}

// IdbSnapshotStorage uses IndexedDB for snapshot persistence (Safari fallback).
class IdbSnapshotStorage implements SnapshotStorage {
  private db: IDBDatabase | null = null

  async init(): Promise<void> {
    this.db = await new Promise<IDBDatabase>((resolve, reject) => {
      const req = indexedDB.open(IDB_DB_NAME, 1)
      req.onupgradeneeded = () => {
        req.result.createObjectStore(IDB_STORE_NAME)
      }
      req.onsuccess = () => resolve(req.result)
      req.onerror = () => reject(req.error)
    })
  }

  async write(pluginId: string, data: ArrayBuffer): Promise<void> {
    if (!this.db) throw new Error('IdbSnapshotStorage: not initialized')
    const tx = this.db.transaction(IDB_STORE_NAME, 'readwrite')
    const store = tx.objectStore(IDB_STORE_NAME)
    return new Promise((resolve, reject) => {
      const req = store.put(data, pluginId)
      req.onsuccess = () => resolve()
      req.onerror = () => reject(req.error)
    })
  }

  async read(pluginId: string): Promise<ArrayBuffer | null> {
    if (!this.db) throw new Error('IdbSnapshotStorage: not initialized')
    const tx = this.db.transaction(IDB_STORE_NAME, 'readonly')
    const store = tx.objectStore(IDB_STORE_NAME)
    return new Promise((resolve, reject) => {
      const req = store.get(pluginId)
      req.onsuccess = () => resolve((req.result as ArrayBuffer) ?? null)
      req.onerror = () => reject(req.error)
    })
  }

  async delete(pluginId: string): Promise<void> {
    if (!this.db) throw new Error('IdbSnapshotStorage: not initialized')
    const tx = this.db.transaction(IDB_STORE_NAME, 'readwrite')
    const store = tx.objectStore(IDB_STORE_NAME)
    return new Promise((resolve, reject) => {
      const req = store.delete(pluginId)
      req.onsuccess = () => resolve()
      req.onerror = () => reject(req.error)
    })
  }

  async list(): Promise<string[]> {
    if (!this.db) throw new Error('IdbSnapshotStorage: not initialized')
    const tx = this.db.transaction(IDB_STORE_NAME, 'readonly')
    const store = tx.objectStore(IDB_STORE_NAME)
    return new Promise((resolve, reject) => {
      const req = store.getAllKeys()
      req.onsuccess = () => resolve((req.result as string[]) ?? [])
      req.onerror = () => reject(req.error)
    })
  }
}

// SnapshotManager manages WASM memory snapshots for plugin recovery.
export class SnapshotManager {
  private storage: SnapshotStorage
  private plugins: Map<string, SnapshotEntry> = new Map()
  private initialized = false

  constructor(useIdb?: boolean) {
    this.storage = useIdb ? new IdbSnapshotStorage() : new OpfsSnapshotStorage()
  }

  // init initializes the storage backend.
  async init(): Promise<void> {
    await (this.storage as OpfsSnapshotStorage | IdbSnapshotStorage).init()
    this.initialized = true
  }

  // register adds a plugin's WASM memory for snapshot management.
  register(pluginId: string, memory: WebAssembly.Memory): void {
    this.plugins.set(pluginId, { pluginId, memory })
  }

  // unregister removes a plugin from snapshot management.
  unregister(pluginId: string): void {
    this.plugins.delete(pluginId)
  }

  // snapshot writes the current WASM memory for a single plugin to storage.
  async snapshot(pluginId: string): Promise<void> {
    if (!this.initialized) throw new Error('SnapshotManager: not initialized')
    const entry = this.plugins.get(pluginId)
    if (!entry) throw new Error(`SnapshotManager: plugin ${pluginId} not registered`)

    // Copy the buffer to avoid detachment issues.
    const copy = entry.memory.buffer.slice(0)
    await this.storage.write(pluginId, copy)
  }

  // snapshotAll writes snapshots for all registered plugins.
  async snapshotAll(): Promise<void> {
    const promises: Promise<void>[] = []
    for (const pluginId of this.plugins.keys()) {
      promises.push(this.snapshot(pluginId))
    }
    await Promise.all(promises)
  }

  // restore reads a snapshot from storage and returns the ArrayBuffer.
  // Returns null if no snapshot exists. The caller creates a new
  // WebAssembly.Memory from the buffer.
  async restore(pluginId: string): Promise<ArrayBuffer | null> {
    if (!this.initialized) throw new Error('SnapshotManager: not initialized')
    return this.storage.read(pluginId)
  }

  // deleteSnapshot removes a snapshot from storage.
  async deleteSnapshot(pluginId: string): Promise<void> {
    if (!this.initialized) throw new Error('SnapshotManager: not initialized')
    return this.storage.delete(pluginId)
  }

  // listSnapshots returns the plugin IDs with stored snapshots.
  async listSnapshots(): Promise<string[]> {
    if (!this.initialized) throw new Error('SnapshotManager: not initialized')
    return this.storage.list()
  }
}

// createSnapshotManager creates and initializes a SnapshotManager.
// Uses OPFS by default, falls back to IDB if OPFS is unavailable.
export async function createSnapshotManager(): Promise<SnapshotManager> {
  let useIdb = false
  try {
    await navigator.storage.getDirectory()
  } catch {
    useIdb = true
  }
  const mgr = new SnapshotManager(useIdb)
  await mgr.init()
  return mgr
}
