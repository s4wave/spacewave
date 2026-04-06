import { describe, it, expect } from 'vitest'
import { SnapshotManager } from './snapshot-manager.js'

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
    const mgr = new SnapshotManager(true)
    // Manually set initialized via prototype hack for unit test.
    ;(mgr as any).initialized = true
    ;(mgr as any).storage = {
      write: async () => {},
      read: async () => null,
      delete: async () => {},
      list: async () => [],
    }
    await expect(mgr.snapshot('unknown')).rejects.toThrow('not registered')
  })
})

// Full OPFS/IDB integration tests require browser environment (Playwright).
