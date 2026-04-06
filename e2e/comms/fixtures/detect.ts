// detect.ts - feature detection probe verification fixture.
//
// Imports the actual detectWorkerCommsConfig and runs it in a real browser.
// Writes structured results to window.__results for playwright-go extraction.

import {
  detectWorkerCommsConfig,
  configDescription,
  type WorkerCommsDetectResult,
} from '../../../web/bldr/worker-comms-detect.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      config: string
      configDesc: string
      caps: Record<string, boolean>
      detail: string
    }
  }
}

async function run() {
  const log = document.getElementById('log')!
  try {
    const result: WorkerCommsDetectResult = await detectWorkerCommsConfig()
    const { config, caps } = result

    // Basic sanity: detection completed without throwing.
    // The Go test asserts specific capability values per browser.
    const pass = typeof config === 'string' && config.length > 0

    window.__results = {
      pass,
      config,
      configDesc: configDescription(config),
      caps: {
        crossOriginIsolated: caps.crossOriginIsolated,
        sabAvailable: caps.sabAvailable,
        opfsAvailable: caps.opfsAvailable,
        webLocksAvailable: caps.webLocksAvailable,
        broadcastChannelAvailable: caps.broadcastChannelAvailable,
      },
      detail: `config=${config} (${configDescription(config)})`,
    }
    log.textContent = 'DONE'
  } catch (err) {
    window.__results = {
      pass: false,
      config: '',
      configDesc: '',
      caps: {
        crossOriginIsolated: false,
        sabAvailable: false,
        opfsAvailable: false,
        webLocksAvailable: false,
        broadcastChannelAvailable: false,
      },
      detail: `error: ${err}`,
    }
    log.textContent = 'DONE'
  }
}

run()
