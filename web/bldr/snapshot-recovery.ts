// snapshot-recovery.ts orchestrates plugin recovery from WASM memory snapshots.
//
// On tab open, checks for orphaned snapshots (snapshot exists but no WebLock
// held for the plugin). Orphaned plugins are restored in new DedicatedWorkers
// by loading the snapshot into WASM memory and re-establishing RPC connections
// through the normal plugin registration path.

import { SnapshotManager, createSnapshotManager } from './snapshot-manager.js'

// PLUGIN_LOCK_PREFIX is the WebLock name prefix for plugin ownership.
const PLUGIN_LOCK_PREFIX = 'bldr-plugin-'

// SnapshotRecoveryOpts configures the recovery orchestration.
export interface SnapshotRecoveryOpts {
  // restorePlugin is called for each orphaned plugin snapshot found.
  // Receives the plugin ID and the snapshot ArrayBuffer.
  // The implementation should create a new DedicatedWorker, load the WASM
  // memory from the snapshot, and re-register the plugin.
  restorePlugin: (pluginId: string, snapshot: ArrayBuffer) => Promise<void>
}

// acquirePluginLock acquires a WebLock for the given plugin ID.
// Returns a release function. The lock is held until release is called.
// If WebLocks are unavailable, returns a no-op release.
export function acquirePluginLock(
  pluginId: string,
): Promise<() => void> {
  if (typeof navigator === 'undefined' || !navigator.locks) {
    return Promise.resolve(() => {})
  }

  const lockName = PLUGIN_LOCK_PREFIX + pluginId
  let releaseFn: () => void = () => {}

  return new Promise<() => void>((resolveOuter) => {
    navigator.locks
      .request(lockName, { mode: 'exclusive' }, () => {
        return new Promise<void>((resolveHold) => {
          releaseFn = resolveHold
          resolveOuter(releaseFn)
        })
      })
      .catch(() => {
        resolveOuter(() => {})
      })
  })
}

// isPluginLockHeld checks whether a WebLock is currently held for the plugin.
export async function isPluginLockHeld(pluginId: string): Promise<boolean> {
  if (typeof navigator === 'undefined' || !navigator.locks) {
    return false
  }

  const lockName = PLUGIN_LOCK_PREFIX + pluginId
  const state = await navigator.locks.query()
  const held = state.held ?? []
  return held.some((lock) => lock.name === lockName)
}

// findOrphanedSnapshots returns plugin IDs that have snapshots but no
// active WebLock (i.e. no tab currently owns them).
export async function findOrphanedSnapshots(
  mgr: SnapshotManager,
): Promise<string[]> {
  const snapshots = await mgr.listSnapshots()
  const orphaned: string[] = []
  for (const pluginId of snapshots) {
    const held = await isPluginLockHeld(pluginId)
    if (!held) {
      orphaned.push(pluginId)
    }
  }
  return orphaned
}

// recoverOrphanedPlugins checks for orphaned snapshots and restores them.
// Called on tab open. Returns the number of plugins recovered.
export async function recoverOrphanedPlugins(
  opts: SnapshotRecoveryOpts,
): Promise<number> {
  let mgr: SnapshotManager
  try {
    mgr = await createSnapshotManager()
  } catch (err) {
    console.warn('snapshot-recovery: unable to init snapshot manager:', err)
    return 0
  }

  const orphaned = await findOrphanedSnapshots(mgr)
  if (orphaned.length === 0) {
    return 0
  }

  console.log('snapshot-recovery: found', orphaned.length, 'orphaned plugins:', orphaned)
  let recovered = 0

  for (const pluginId of orphaned) {
    try {
      const snapshot = await mgr.restore(pluginId)
      if (!snapshot) {
        console.warn('snapshot-recovery: empty snapshot for', pluginId)
        await mgr.deleteSnapshot(pluginId)
        continue
      }

      await opts.restorePlugin(pluginId, snapshot)
      await mgr.deleteSnapshot(pluginId)
      recovered++
      console.log('snapshot-recovery: restored plugin', pluginId)
    } catch (err) {
      console.error('snapshot-recovery: failed to restore', pluginId, err)
      // Delete the snapshot to avoid retrying a broken snapshot.
      await mgr.deleteSnapshot(pluginId).catch(() => {})
    }
  }

  return recovered
}
