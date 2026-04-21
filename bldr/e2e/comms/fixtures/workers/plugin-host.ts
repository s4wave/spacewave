// plugin-host.ts - Simplified DedicatedWorker host for testing.
//
// Mirrors the shw.mjs pattern: receives busSab + busPluginId + scriptUrl,
// registers on SAB bus, dynamically imports the plugin script, and calls
// its default export with the bus endpoint and an AbortSignal.

import { SabBusEndpoint } from '../../../../web/bldr/sab-bus.js'
import type { WorkerCommsDetectResult } from '../../../../web/bldr/worker-comms-detect.js'

declare const self: DedicatedWorkerGlobalScope

interface InitMsg {
  busSab: SharedArrayBuffer
  busPluginId: number
  scriptUrl: string
  workerCommsDetect?: WorkerCommsDetectResult
}

const ac = new AbortController()

self.onmessage = async (ev: MessageEvent<InitMsg>) => {
  const { busSab, busPluginId, scriptUrl, workerCommsDetect } = ev.data
  const opts = { slotSize: 256, numSlots: 32 }
  const endpoint = new SabBusEndpoint(busSab, busPluginId, opts)
  endpoint.register()

  self.postMessage({ type: 'registered', busPluginId })

  // Echo back the received detection config to verify init message passthrough.
  if (workerCommsDetect) {
    self.postMessage({ type: 'config-received', config: workerCommsDetect.config })
  }

  // Dynamically import the plugin script and call its default export.
  const pluginModule = await import(/* @vite-ignore */ scriptUrl)
  if (typeof pluginModule.default !== 'function') {
    self.postMessage({
      type: 'error',
      detail: 'plugin script has no default export function',
    })
    return
  }

  // Call the plugin main with the bus endpoint and signal.
  pluginModule.default(endpoint, ac.signal)
}
