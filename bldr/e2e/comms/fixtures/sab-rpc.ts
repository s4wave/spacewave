// sab-rpc.ts - StarPC echo RPC over a SAB pair between two DedicatedWorkers.
//
// Creates a bounded SAB pair, spawns a server worker and a client worker.
// Server runs the StarPC echo service. Client calls Echo through
// SabRingStream. Verifies the round-trip response.

import {
  SAB_PAIR_DIRECTION_MTU_BYTES,
  createSabPair,
  sabPairBufferSize,
} from '../../../web/bldr/sab-ring-stream.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      echoBody: string
      mtuBytes: number
      pairBufferBytes: number
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

  const { aSab, bSab } = createSabPair()
  const pairId = 'sab-pair-fixture-1'

  // Spawn server worker.
  const serverWorker = new Worker('/workers/rpc-peer.js', { type: 'module' })
  const serverReadyForPair = waitWorkerMsg(serverWorker, 'pair-ready')
  const serverReady = waitWorkerMsg(serverWorker, 'server-ready')
  serverWorker.postMessage({
    pairId,
    txSab: aSab,
    rxSab: bSab,
    role: 'server',
  })
  await serverReadyForPair
  await serverReady

  // Spawn client worker.
  const clientWorker = new Worker('/workers/rpc-peer.js', { type: 'module' })
  const clientReadyForPair = waitWorkerMsg(clientWorker, 'pair-ready')
  const rpcResult = waitWorkerMsg(clientWorker, 'rpc-result')
  clientWorker.postMessage({
    pairId,
    txSab: bSab,
    rxSab: aSab,
    role: 'client',
  })
  await clientReadyForPair

  // Tell client to start the RPC call.
  clientWorker.postMessage({ type: 'start' })

  // Wait for the RPC result.
  const result = await rpcResult
  const body = result.data?.body ?? ''

  window.__results = {
    pass: body === 'hello via SAB pair',
    detail:
      body === 'hello via SAB pair'
        ? 'echo round-trip ok'
        : `unexpected: ${body}`,
    echoBody: body,
    mtuBytes: SAB_PAIR_DIRECTION_MTU_BYTES,
    pairBufferBytes: sabPairBufferSize(),
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
    mtuBytes: 0,
    pairBufferBytes: 0,
  }
  document.getElementById('log')!.textContent = 'DONE'
})
