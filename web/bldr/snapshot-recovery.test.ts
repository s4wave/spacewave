import { describe, it, expect } from 'vitest'
import { recoverOrphanedPlugins, type SnapshotRecoveryOpts } from './snapshot-recovery.js'

describe('snapshot-recovery', () => {
  it('recoverOrphanedPlugins returns 0 when no snapshots', async () => {
    const restored: string[] = []
    const opts: SnapshotRecoveryOpts = {
      restorePlugin: async (pluginId) => {
        restored.push(pluginId)
      },
    }

    // In unit test env (happy-dom), navigator.storage.getDirectory will
    // likely fail, so createSnapshotManager falls back to IDB which also
    // fails. The function handles this gracefully and returns 0.
    const count = await recoverOrphanedPlugins(opts)
    expect(count).toBe(0)
    expect(restored).toEqual([])
  })
})

// Full recovery integration tests require browser environment with OPFS and
// WebLocks (Playwright).
