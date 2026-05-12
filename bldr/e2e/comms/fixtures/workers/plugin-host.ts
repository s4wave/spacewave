// plugin-host.ts - Simplified DedicatedWorker host for testing.
//
// Mirrors the shw.mjs startup pattern: receives scriptUrl and workerCommsDetect,
// dynamically imports the plugin script, and calls its default export without
// eager SAB bus registration.

import type { WorkerCommsDetectResult } from '../../../../web/bldr/worker-comms-detect.js'

declare const self: DedicatedWorkerGlobalScope

interface InitMsg {
  scriptUrl: string
  workerCommsDetect?: WorkerCommsDetectResult
}

const ac = new AbortController()

self.onmessage = async (ev: MessageEvent<InitMsg>) => {
  const { scriptUrl, workerCommsDetect } = ev.data
  self.postMessage({ type: 'startup-no-sab' })

  // Echo back the received detection config to verify init message passthrough.
  if (workerCommsDetect) {
    self.postMessage({
      type: 'config-received',
      config: workerCommsDetect.config,
    })
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

  // Call the plugin main with only an AbortSignal.
  pluginModule.default(ac.signal)
}
