// WorkerCommsConfig enumerates the valid worker communication configurations.
export type WorkerCommsConfig = 'A' | 'B' | 'C' | 'F'

// WorkerCommsCapabilities holds the result of runtime feature detection probes.
export interface WorkerCommsCapabilities {
  // crossOriginIsolated is true when COOP+COEP headers are set.
  crossOriginIsolated: boolean
  // sabAvailable is true when SharedArrayBuffer can be constructed.
  sabAvailable: boolean
  // opfsAvailable is true when navigator.storage.getDirectory() succeeds.
  opfsAvailable: boolean
  // webLocksAvailable is true when navigator.locks exists.
  webLocksAvailable: boolean
  // broadcastChannelAvailable is true when BroadcastChannel exists.
  broadcastChannelAvailable: boolean
}

// WorkerCommsDetectResult holds the detected config and capabilities.
export interface WorkerCommsDetectResult {
  // config is the best worker communication configuration for this browser.
  config: WorkerCommsConfig
  // caps holds the individual capability probe results.
  caps: WorkerCommsCapabilities
}

// detectCrossOriginIsolated checks for COOP+COEP headers.
function detectCrossOriginIsolated(): boolean {
  return typeof self !== 'undefined' && !!self.crossOriginIsolated
}

// detectSabAvailable checks if SharedArrayBuffer can be allocated.
function detectSabAvailable(): boolean {
  try {
    if (typeof SharedArrayBuffer !== 'function') {
      return false
    }
    const buf = new SharedArrayBuffer(8)
    return buf.byteLength === 8
  } catch {
    return false
  }
}

// detectOpfsAvailable checks if the Origin Private File System is accessible.
async function detectOpfsAvailable(): Promise<boolean> {
  try {
    if (typeof navigator === 'undefined') {
      return false
    }
    if (!navigator.storage?.getDirectory) {
      return false
    }
    await navigator.storage.getDirectory()
    return true
  } catch {
    return false
  }
}

// detectWebLocksAvailable checks if the Web Locks API exists.
function detectWebLocksAvailable(): boolean {
  return typeof navigator !== 'undefined' && !!navigator.locks
}

// detectBroadcastChannelAvailable checks if BroadcastChannel is supported.
function detectBroadcastChannelAvailable(): boolean {
  return typeof BroadcastChannel === 'function'
}

// selectConfig picks the best WorkerCommsConfig given the capabilities.
//
// Config A: SharedWorker + MessagePort everywhere (all browsers).
// Config B: DedicatedWorker + SAB intra-tab, MessagePort cross-tab.
// Config C: Config B + OPFS snapshot recovery on tab close.
// Config F: Safari fallback, same as A.
function selectConfig(caps: WorkerCommsCapabilities): WorkerCommsConfig {
  if (!caps.crossOriginIsolated || !caps.sabAvailable) {
    return 'A'
  }

  // SAB works. Use DedicatedWorker + SAB intra-tab.
  if (caps.opfsAvailable && caps.webLocksAvailable) {
    // OPFS available for snapshot recovery.
    return 'C'
  }

  // SAB works but no OPFS for snapshots.
  return 'B'
}

// detectWorkerCommsConfig runs feature probes and returns the best
// worker communication configuration for this browser. All probes are
// fast (<5ms each) and non-destructive.
export async function detectWorkerCommsConfig(): Promise<WorkerCommsDetectResult> {
  const crossOriginIsolated = detectCrossOriginIsolated()
  const sabAvailable = detectSabAvailable()
  const webLocksAvailable = detectWebLocksAvailable()
  const broadcastChannelAvailable = detectBroadcastChannelAvailable()
  const opfsAvailable = await detectOpfsAvailable()

  const caps: WorkerCommsCapabilities = {
    crossOriginIsolated,
    sabAvailable,
    opfsAvailable,
    webLocksAvailable,
    broadcastChannelAvailable,
  }

  const config = selectConfig(caps)
  console.log('worker-comms: detected config', config, caps)
  return { config, caps }
}
