// dedicated.ts - DedicatedWorker hosting test fixture.
//
// Creates a DedicatedWorker using the plugin-host wrapper (simplified shw.mjs),
// sends init without SAB bus fields, and verifies the plugin script executes
// with the authoritative worker comms config.

import { detectWorkerCommsConfig } from '../../../web/bldr/worker-comms-detect.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      noStartupSab: boolean
      pluginStarted: boolean
      configReceived: boolean
      manyWorkersStarted: boolean
    }
  }
}

type WorkerMessage = { type: string } & Record<string, unknown>

function waitWorkerMsg(
  worker: Worker,
  type: string,
  timeoutMs: number,
): Promise<WorkerMessage> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(
      () => reject(new Error(`timeout waiting for ${type}`)),
      timeoutMs,
    )
    const handler = (ev: MessageEvent<unknown>) => {
      if (typeof ev.data !== 'object' || ev.data === null) return
      const msg = ev.data as WorkerMessage
      if (msg.type === type) {
        clearTimeout(timer)
        worker.removeEventListener('message', handler)
        resolve(msg)
      }
    }
    worker.addEventListener('message', handler)
  })
}

async function run() {
  const log = document.getElementById('log')!
  const errors: string[] = []

  try {
    // Detect config on main thread (authoritative).
    const detect = await detectWorkerCommsConfig()

    // Create DedicatedWorker with the plugin-host wrapper.
    const worker = new Worker(
      new URL('./workers/plugin-host.js', import.meta.url),
      { type: 'module' },
    )

    // Plugin script URL: served from dist by the test server.
    const pluginUrl = '/workers/plugin-stub.js'

    // Set up all message listeners BEFORE sending init to avoid race conditions.
    const noStartupSabP = waitWorkerMsg(worker, 'startup-no-sab', 5000)
    const configReceivedP = waitWorkerMsg(worker, 'config-received', 5000)
    const pluginStartedP = waitWorkerMsg(worker, 'plugin-started', 5000)

    // Send init message without startup SAB transport fields.
    worker.postMessage({
      scriptUrl: pluginUrl,
      workerCommsDetect: detect,
    })

    // Test 1: Worker starts without SAB bus registration.
    let noStartupSab = false
    {
      await noStartupSabP
      noStartupSab = true
    }

    // Test 2: Worker received workerCommsDetect config via init message.
    let configReceived = false
    {
      const msg = await configReceivedP
      if (msg.config === detect.config) {
        configReceived = true
      } else {
        errors.push(`config: expected ${detect.config}, got ${msg.config}`)
      }
    }

    // Test 3: Plugin script starts (default export called).
    let pluginStarted = false
    {
      await pluginStartedP
      pluginStarted = true
    }

    worker.terminate()

    // Test 4: More than sixteen plugin workers start without SAB reader slots.
    let manyWorkersStarted = false
    {
      const workers: Worker[] = []
      try {
        const starts = Array.from({ length: 20 }, async (_, i) => {
          const w = new Worker(
            new URL('./workers/plugin-host.js', import.meta.url),
            { type: 'module' },
          )
          workers.push(w)
          const noSab = waitWorkerMsg(w, 'startup-no-sab', 5000)
          const started = waitWorkerMsg(w, 'plugin-started', 5000)
          w.postMessage({
            scriptUrl: pluginUrl,
            workerCommsDetect: detect,
            workerIndex: i,
          })
          await noSab
          await started
        })
        await Promise.all(starts)
        manyWorkersStarted = true
      } finally {
        for (const w of workers) {
          w.terminate()
        }
      }
    }

    const pass =
      noStartupSab &&
      pluginStarted &&
      configReceived &&
      manyWorkersStarted &&
      errors.length === 0
    window.__results = {
      pass,
      detail: errors.length > 0 ? errors.join('; ') : 'all tests passed',
      noStartupSab,
      pluginStarted,
      configReceived,
      manyWorkersStarted,
    }
  } catch (err) {
    window.__results = {
      pass: false,
      detail: `error: ${err}`,
      noStartupSab: false,
      pluginStarted: false,
      configReceived: false,
      manyWorkersStarted: false,
    }
  }

  log.textContent = 'DONE'
}

run()
