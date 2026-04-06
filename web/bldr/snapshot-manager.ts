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

// DEFAULT_SNAPSHOT_INTERVAL_MS is the default periodic snapshot interval.
export const DEFAULT_SNAPSHOT_INTERVAL_MS = 30_000

// SnapshotEntry tracks a registered plugin's WASM memory.
interface SnapshotEntry {
  pluginId: string
  memory: WebAssembly.Memory
  // generation incremented by the plugin to signal memory changes.
  generation: number
  // lastSnapshotGeneration is the generation at the last snapshot.
  lastSnapshotGeneration: number
  // lastSnapshotSize is the buffer byteLength at the last snapshot.
  lastSnapshotSize: number
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
  private periodicTimer: ReturnType<typeof setInterval> | null = null
  private snapshotIntervalMs = DEFAULT_SNAPSHOT_INTERVAL_MS

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
    this.plugins.set(pluginId, {
      pluginId,
      memory,
      generation: 0,
      lastSnapshotGeneration: -1,
      lastSnapshotSize: 0,
    })
  }

  // unregister removes a plugin from snapshot management and stops periodic
  // scheduling if no plugins remain.
  unregister(pluginId: string): void {
    this.plugins.delete(pluginId)
    if (this.plugins.size === 0) {
      this.stopPeriodic()
    }
  }

  // markDirty increments the generation for a plugin, signaling that memory
  // has changed and a snapshot should be taken on the next periodic tick.
  markDirty(pluginId: string): void {
    const entry = this.plugins.get(pluginId)
    if (entry) {
      entry.generation++
    }
  }

  // isDirty returns true if the plugin's memory has changed since the last
  // snapshot (generation incremented or buffer size changed).
  isDirty(pluginId: string): boolean {
    const entry = this.plugins.get(pluginId)
    if (!entry) return false
    return (
      entry.generation !== entry.lastSnapshotGeneration ||
      entry.memory.buffer.byteLength !== entry.lastSnapshotSize
    )
  }

  // snapshot writes the current WASM memory for a single plugin to storage.
  // If force is false (default), skips the snapshot if memory is not dirty.
  async snapshot(pluginId: string, force?: boolean): Promise<boolean> {
    if (!this.initialized) throw new Error('SnapshotManager: not initialized')
    const entry = this.plugins.get(pluginId)
    if (!entry) throw new Error(`SnapshotManager: plugin ${pluginId} not registered`)

    if (!force && !this.isDirty(pluginId)) {
      return false
    }

    // Copy the buffer to avoid detachment issues.
    const copy = entry.memory.buffer.slice(0)
    await this.storage.write(pluginId, copy)

    entry.lastSnapshotGeneration = entry.generation
    entry.lastSnapshotSize = copy.byteLength
    return true
  }

  // snapshotAll writes snapshots for all dirty registered plugins.
  // If force is true, snapshots all plugins regardless of dirty state.
  async snapshotAll(force?: boolean): Promise<number> {
    const promises: Promise<boolean>[] = []
    for (const pluginId of this.plugins.keys()) {
      promises.push(this.snapshot(pluginId, force))
    }
    const results = await Promise.all(promises)
    return results.filter(Boolean).length
  }

  // startPeriodic begins periodic snapshot scheduling.
  startPeriodic(intervalMs?: number): void {
    this.stopPeriodic()
    this.snapshotIntervalMs = intervalMs ?? DEFAULT_SNAPSHOT_INTERVAL_MS
    this.periodicTimer = setInterval(() => {
      this.snapshotAll().catch((err) => {
        console.warn('SnapshotManager: periodic snapshot failed:', err)
      })
    }, this.snapshotIntervalMs)
    console.log('SnapshotManager: periodic snapshots started, interval:', this.snapshotIntervalMs, 'ms')
  }

  // stopPeriodic stops periodic snapshot scheduling.
  stopPeriodic(): void {
    if (this.periodicTimer != null) {
      clearInterval(this.periodicTimer)
      this.periodicTimer = null
    }
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
