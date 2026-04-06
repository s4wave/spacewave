import { describe, it, expect } from 'vitest'
import { SnapshotManager } from './snapshot-manager.js'

// mockStorage returns a mock SnapshotStorage for unit tests.
function mockStorage() {
  const data = new Map<string, ArrayBuffer>()
  return {
    init: async () => {},
    write: async (id: string, buf: ArrayBuffer) => { data.set(id, buf) },
    read: async (id: string) => data.get(id) ?? null,
    delete: async (id: string) => { data.delete(id) },
    list: async () => [...data.keys()],
    data,
  }
}

function initMgr(): SnapshotManager {
  const mgr = new SnapshotManager(true)
  const storage = mockStorage()
  ;(mgr as any).initialized = true
  ;(mgr as any).storage = storage
  return mgr
}

describe('SnapshotManager', () => {
  it('can be constructed', () => {
    const mgr = new SnapshotManager(true)
    expect(mgr).toBeDefined()
  })

  it('register and unregister plugins', () => {
    const mgr = new SnapshotManager(true)
    const memory = new WebAssembly.Memory({ initial: 1 })
    mgr.register('plugin-1', memory)
    mgr.unregister('plugin-1')
  })

  it('throws on snapshot before init', async () => {
    const mgr = new SnapshotManager(true)
    const memory = new WebAssembly.Memory({ initial: 1 })
    mgr.register('plugin-1', memory)
    await expect(mgr.snapshot('plugin-1')).rejects.toThrow('not initialized')
  })

  it('throws on snapshot of unregistered plugin', async () => {
    const mgr = initMgr()
    await expect(mgr.snapshot('unknown')).rejects.toThrow('not registered')
  })

  it('skips snapshot when not dirty', async () => {
    const mgr = initMgr()
    const memory = new WebAssembly.Memory({ initial: 1 })
    mgr.register('p1', memory)

    // First snapshot with force=true to set baseline.
    const wrote = await mgr.snapshot('p1', true)
    expect(wrote).toBe(true)

    // Second snapshot without force should skip (not dirty).
    const skipped = await mgr.snapshot('p1')
    expect(skipped).toBe(false)
  })

  it('snapshots when marked dirty', async () => {
    const mgr = initMgr()
    const memory = new WebAssembly.Memory({ initial: 1 })
    mgr.register('p1', memory)

    await mgr.snapshot('p1', true)
    expect(mgr.isDirty('p1')).toBe(false)

    mgr.markDirty('p1')
    expect(mgr.isDirty('p1')).toBe(true)

    const wrote = await mgr.snapshot('p1')
    expect(wrote).toBe(true)
    expect(mgr.isDirty('p1')).toBe(false)
  })

  it('snapshotAll returns count of dirty plugins', async () => {
    const mgr = initMgr()
    const m1 = new WebAssembly.Memory({ initial: 1 })
    const m2 = new WebAssembly.Memory({ initial: 1 })
    mgr.register('p1', m1)
    mgr.register('p2', m2)

    // Force initial snapshots.
    await mgr.snapshotAll(true)

    // Mark only p1 dirty.
    mgr.markDirty('p1')
    const count = await mgr.snapshotAll()
    expect(count).toBe(1)
  })

  it('startPeriodic and stopPeriodic', () => {
    const mgr = initMgr()
    mgr.startPeriodic(100)
    expect((mgr as any).periodicTimer).not.toBeNull()
    mgr.stopPeriodic()
    expect((mgr as any).periodicTimer).toBeNull()
  })
})

// Full OPFS/IDB integration tests require browser environment (Playwright).
