// recovery.ts - Plugin recovery test fixture.
//
// Modes (via URL ?mode= param):
//   setup: Acquire plugin lock, write snapshot, signal ready
//   recover: Find orphaned snapshots, verify recovery

import { createSnapshotManager } from '../../../web/bldr/snapshot-manager.js'
import {
  acquirePluginLock,
  findOrphanedSnapshots,
  recoverOrphanedPlugins,
} from '../../../web/bldr/snapshot-recovery.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      lockAcquired?: boolean
      snapshotWritten?: boolean
      orphanDetected?: boolean
      recovered?: boolean
    }
  }
}

const PLUGIN_ID = 'test-recovery-plugin'
const PATTERN = [0xde, 0xad, 0xc0, 0xde]

// Setup mode: acquire lock, write snapshot, signal ready.
async function runSetup() {
  const mgr = await createSnapshotManager()

  // Create a WASM memory with a known pattern and snapshot it.
  const memory = new WebAssembly.Memory({ initial: 1 })
  const view = new Uint8Array(memory.buffer)
  view[0] = PATTERN[0]
  view[1] = PATTERN[1]
  view[2] = PATTERN[2]
  view[3] = PATTERN[3]

  mgr.register(PLUGIN_ID, memory)
  mgr.markDirty(PLUGIN_ID)
  await mgr.snapshot(PLUGIN_ID)

  // Acquire the plugin lock. This holds until the page closes.
  const release = await acquirePluginLock(PLUGIN_ID)
  void release // held until page closes

  window.__results = {
    pass: true,
    detail: 'setup: lock acquired and snapshot written',
    lockAcquired: true,
    snapshotWritten: true,
  }
}

// Recover mode: find orphans, recover, verify snapshot data.
async function runRecover() {
  const mgr = await createSnapshotManager()

  // Find orphaned snapshots (lock should be released after page A closed).
  const orphans = await findOrphanedSnapshots(mgr)
  const orphanDetected = orphans.includes(PLUGIN_ID)

  if (!orphanDetected) {
    window.__results = {
      pass: false,
      detail: `recover: orphan not found, got: ${JSON.stringify(orphans)}`,
      orphanDetected: false,
      recovered: false,
    }
    return
  }

  // Recover the orphaned plugin.
  let recoveredData: ArrayBuffer | null = null
  const count = await recoverOrphanedPlugins({
    restorePlugin: async (_pluginId: string, snapshot: ArrayBuffer) => {
      recoveredData = snapshot
    },
  })

  let recovered = false
  if (count === 1 && recoveredData) {
    const view = new Uint8Array(recoveredData)
    recovered =
      view[0] === PATTERN[0] &&
      view[1] === PATTERN[1] &&
      view[2] === PATTERN[2] &&
      view[3] === PATTERN[3]
  }

  window.__results = {
    pass: orphanDetected && recovered,
    detail:
      orphanDetected && recovered
        ? 'recover: orphan detected and snapshot restored'
        : `recover: orphan=${orphanDetected} recovered=${recovered} count=${count}`,
    orphanDetected,
    recovered,
  }
}

async function run() {
  const log = document.getElementById('log')!
  const params = new URLSearchParams(location.search)
  const mode = params.get('mode') || 'setup'

  try {
    if (mode === 'recover') {
      await runRecover()
    } else {
      await runSetup()
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
