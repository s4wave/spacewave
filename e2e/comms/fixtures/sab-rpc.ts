// sab-rpc.ts - StarPC echo RPC over SAB bus between two DedicatedWorkers.
//
// Creates a shared bus, spawns a server worker (pluginId=1) and a client
// worker (pluginId=2). Server runs the StarPC echo service. Client calls
// Echo({body: "hello via SAB bus"}) through SabBusStream. Verifies the
// round-trip response.

import { createBusSab } from '../../../web/bldr/sab-bus.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      echoBody: string
    }
  }
}

function waitWorkerMsg(
  worker: Worker,
  type: string,
  timeoutMs = 5000,
): Promise<MessageEvent> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(
      () => reject(new Error(`timeout waiting for worker message: ${type}`)),
      timeoutMs,
    )
    const handler = (ev: MessageEvent) => {
      if (ev.data?.type === type) {
        clearTimeout(timer)
        worker.removeEventListener('message', handler)
        resolve(ev)
      }
    }
    worker.addEventListener('message', handler)
  })
}

async function run() {
  const log = document.getElementById('log')!

  // Create shared bus.
  const busSab = createBusSab({ slotSize: 8192, numSlots: 64 })

  // Spawn server worker (pluginId=1).
  const serverWorker = new Worker('/workers/rpc-peer.js', { type: 'module' })
  const serverRegistered = waitWorkerMsg(serverWorker, 'registered')
  const serverReady = waitWorkerMsg(serverWorker, 'server-ready')
  serverWorker.postMessage({
    busSab,
    pluginId: 1,
    targetId: 2,
    role: 'server',
  })
  await serverRegistered
  await serverReady

  // Spawn client worker (pluginId=2).
  const clientWorker = new Worker('/workers/rpc-peer.js', { type: 'module' })
  const clientRegistered = waitWorkerMsg(clientWorker, 'registered')
  const rpcResult = waitWorkerMsg(clientWorker, 'rpc-result')
  clientWorker.postMessage({
    busSab,
    pluginId: 2,
    targetId: 1,
    role: 'client',
  })
  await clientRegistered

  // Tell client to start the RPC call.
  clientWorker.postMessage({ type: 'start' })

  // Wait for the RPC result.
  const result = await rpcResult
  const body = result.data?.body ?? ''

  window.__results = {
    pass: body === 'hello via SAB bus',
    detail: body === 'hello via SAB bus' ? 'echo round-trip ok' : `unexpected: ${body}`,
    echoBody: body,
  }

  // Clean up.
  serverWorker.terminate()
  clientWorker.terminate()
  log.textContent = 'DONE'
}

run().catch((err) => {
  window.__results = {
    pass: false,
    detail: `error: ${err}`,
    echoBody: '',
  }
  document.getElementById('log')!.textContent = 'DONE'
})
