// dedicated.ts - DedicatedWorker hosting test fixture.
//
// Creates a DedicatedWorker using the plugin-host wrapper (simplified shw.mjs),
// sends init with busSab and busPluginId, verifies the worker registers on the
// SAB bus and the plugin script executes and receives a bus message.

import {
  SabBusEndpoint,
  createBusSab,
} from '../../../web/bldr/sab-bus.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      registered: boolean
      pluginStarted: boolean
      pluginReceived: boolean
    }
  }
}

function waitWorkerMsg(
  worker: Worker,
  type: string,
  timeoutMs: number,
): Promise<any> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(
      () => reject(new Error(`timeout waiting for ${type}`)),
      timeoutMs,
    )
    const handler = (ev: MessageEvent) => {
      if (ev.data.type === type) {
        clearTimeout(timer)
        worker.removeEventListener('message', handler)
        resolve(ev.data)
      }
    }
    worker.addEventListener('message', handler)
  })
}

async function run() {
  const log = document.getElementById('log')!
  const errors: string[] = []

  try {
    const busOpts = { slotSize: 256, numSlots: 32 }
    const busSab = createBusSab(busOpts)

    // Main thread endpoint (pluginId=0).
    const mainEndpoint = new SabBusEndpoint(busSab, 0, busOpts)
    mainEndpoint.register()

    // Create DedicatedWorker with the plugin-host wrapper.
    const worker = new Worker(
      new URL('./workers/plugin-host.js', import.meta.url),
      { type: 'module' },
    )

    // Plugin script URL: served from dist by the test server.
    const pluginUrl = '/workers/plugin-stub.js'

    // Send init message with busSab, busPluginId, and plugin script URL.
    worker.postMessage({
      busSab,
      busPluginId: 1,
      scriptUrl: pluginUrl,
    })

    // Test 1: Worker registers on bus.
    let registered = false
    {
      const msg = await waitWorkerMsg(worker, 'registered', 5000)
      if (msg.busPluginId === 1) {
        registered = true
      } else {
        errors.push(`registered: unexpected busPluginId ${msg.busPluginId}`)
      }
    }

    // Test 2: Plugin script starts (default export called).
    let pluginStarted = false
    {
      await waitWorkerMsg(worker, 'plugin-started', 5000)
      pluginStarted = true
    }

    // Test 3: Send a bus message to the plugin, verify it receives it.
    let pluginReceived = false
    {
      mainEndpoint.write(1, new Uint8Array([0xff, 0x42]))
      const msg = await waitWorkerMsg(worker, 'plugin-received', 5000)
      if (
        msg.sourceId === 0 &&
        msg.data[0] === 0xff &&
        msg.data[1] === 0x42
      ) {
        pluginReceived = true
      } else {
        errors.push(`received: unexpected msg ${JSON.stringify(msg)}`)
      }
    }

    worker.terminate()
    mainEndpoint.close()

    const pass =
      registered && pluginStarted && pluginReceived && errors.length === 0
    window.__results = {
      pass,
      detail: errors.length > 0 ? errors.join('; ') : 'all tests passed',
      registered,
      pluginStarted,
      pluginReceived,
    }
  } catch (err) {
    window.__results = {
      pass: false,
      detail: `error: ${err}`,
      registered: false,
      pluginStarted: false,
      pluginReceived: false,
    }
  }

  log.textContent = 'DONE'
}

run()
