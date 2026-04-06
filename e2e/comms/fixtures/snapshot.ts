// snapshot.ts - SnapshotManager test fixture.
//
// Creates a SnapshotManager, registers a WebAssembly.Memory, writes a known
// pattern, snapshots, clears memory, restores, verifies pattern intact.
// Also tests markDirty/isDirty generation tracking and listSnapshots.

import {
  SnapshotManager,
  createSnapshotManager,
} from '../../../web/bldr/snapshot-manager.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      snapshotRestore: boolean
      dirtyTracking: boolean
      listSnapshots: boolean
    }
  }
}

const PLUGIN_ID = 'test-plugin-snapshot'
const PATTERN = [0xca, 0xfe, 0xba, 0xbe]

async function run() {
  const log = document.getElementById('log')!
  const errors: string[] = []

  try {
    const mgr = await createSnapshotManager()

    // Test 1: Snapshot and restore.
    let snapshotRestore = false
    {
      const memory = new WebAssembly.Memory({ initial: 1 }) // 1 page = 64KB
      const view = new Uint8Array(memory.buffer)
      view[0] = PATTERN[0]
      view[1] = PATTERN[1]
      view[2] = PATTERN[2]
      view[3] = PATTERN[3]

      mgr.register(PLUGIN_ID, memory)
      mgr.markDirty(PLUGIN_ID)
      const wrote = await mgr.snapshot(PLUGIN_ID)
      if (!wrote) {
        errors.push('snapshot: snapshot() returned false')
      }

      // Clear memory.
      view.fill(0)
      if (view[0] !== 0) {
        errors.push('snapshot: memory not cleared')
      }

      // Restore.
      const restored = await mgr.restore(PLUGIN_ID)
      if (!restored) {
        errors.push('snapshot: restore returned null')
      } else {
        const restoredView = new Uint8Array(restored)
        if (
          restoredView[0] === PATTERN[0] &&
          restoredView[1] === PATTERN[1] &&
          restoredView[2] === PATTERN[2] &&
          restoredView[3] === PATTERN[3]
        ) {
          snapshotRestore = true
        } else {
          errors.push(
            `snapshot: pattern mismatch: ${restoredView[0]},${restoredView[1]},${restoredView[2]},${restoredView[3]}`,
          )
        }
      }
    }

    // Test 2: Dirty tracking.
    let dirtyTracking = false
    {
      // After snapshot, isDirty should be false.
      if (mgr.isDirty(PLUGIN_ID)) {
        errors.push('dirty: still dirty after snapshot')
      } else {
        // After markDirty, isDirty should be true.
        mgr.markDirty(PLUGIN_ID)
        if (!mgr.isDirty(PLUGIN_ID)) {
          errors.push('dirty: not dirty after markDirty')
        } else {
          // Snapshot clears dirty.
          await mgr.snapshot(PLUGIN_ID)
          if (mgr.isDirty(PLUGIN_ID)) {
            errors.push('dirty: still dirty after second snapshot')
          } else {
            dirtyTracking = true
          }
        }
      }
    }

    // Test 3: listSnapshots.
    let listSnapshots = false
    {
      const ids = await mgr.listSnapshots()
      if (ids.includes(PLUGIN_ID)) {
        listSnapshots = true
      } else {
        errors.push(`list: ${PLUGIN_ID} not in ${JSON.stringify(ids)}`)
      }
    }

    // Cleanup.
    await mgr.deleteSnapshot(PLUGIN_ID)
    mgr.unregister(PLUGIN_ID)

    const pass =
      snapshotRestore && dirtyTracking && listSnapshots && errors.length === 0
    window.__results = {
      pass,
      detail: errors.length > 0 ? errors.join('; ') : 'all tests passed',
      snapshotRestore,
      dirtyTracking,
      listSnapshots,
    }
  } catch (err) {
    window.__results = {
      pass: false,
      detail: `error: ${err}`,
      snapshotRestore: false,
      dirtyTracking: false,
      listSnapshots: false,
    }
  }

  log.textContent = 'DONE'
}

run()
