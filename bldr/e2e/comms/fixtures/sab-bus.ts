// sab-bus.ts - SAB shared bus multi-endpoint test fixture.
//
// Creates a bus SAB, spawns 2 DedicatedWorkers, and tests:
// 1. Unicast: main -> worker A
// 2. Relay: worker A -> worker B (unicast)
// 3. Broadcast: worker B -> all

import {
  SabBusEndpoint,
  createBusSab,
  BROADCAST_ID,
} from '../../../web/bldr/sab-bus.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      unicast: boolean
      relay: boolean
      broadcast: boolean
    }
  }
}

type WorkerMessage = { type: string } & Record<string, unknown>

// Wait for a specific message type from a worker.
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
    const busOpts = { slotSize: 256, numSlots: 32 }
    const busSab = createBusSab(busOpts)

    // Main thread endpoint (pluginId=0).
    const mainEndpoint = new SabBusEndpoint(busSab, 0, busOpts)
    mainEndpoint.register()

    // Spawn worker A (pluginId=1) and worker B (pluginId=2).
    const workerA = new Worker(new URL('./workers/bus-peer.js', import.meta.url), { type: 'module' })
    const workerB = new Worker(new URL('./workers/bus-peer.js', import.meta.url), { type: 'module' })

    // Init worker A: register, then read one message.
    workerA.postMessage({
      busSab,
      pluginId: 1,
      readOne: true,
    })
    await waitWorkerMsg(workerA, 'registered', 5000)

    // Init worker B: register, then read one message.
    workerB.postMessage({
      busSab,
      pluginId: 2,
      readOne: true,
    })
    await waitWorkerMsg(workerB, 'registered', 5000)

    // Test 1: Unicast main(0) -> worker A(1).
    let unicast = false
    {
      mainEndpoint.write(1, new Uint8Array([0xaa, 0x01]))
      const msg = await waitWorkerMsg(workerA, 'received', 5000)
      if (msg.sourceId === 0 && msg.data[0] === 0xaa && msg.data[1] === 0x01) {
        unicast = true
      } else {
        errors.push(`unicast: unexpected msg ${JSON.stringify(msg)}`)
      }
    }

    // Test 2: Relay worker A(1) -> worker B(2).
    // Re-init worker A to send, worker B to read.
    let relay = false
    {
      const workerA2 = new Worker(new URL('./workers/bus-peer.js', import.meta.url), { type: 'module' })
      const workerB2 = new Worker(new URL('./workers/bus-peer.js', import.meta.url), { type: 'module' })

      workerB2.postMessage({ busSab, pluginId: 12, readOne: true })
      await waitWorkerMsg(workerB2, 'registered', 5000)

      workerA2.postMessage({
        busSab,
        pluginId: 11,
        targetId: 12,
        payload: [0xbb, 0x02],
      })
      await waitWorkerMsg(workerA2, 'registered', 5000)
      await waitWorkerMsg(workerA2, 'sent', 5000)

      const msg = await waitWorkerMsg(workerB2, 'received', 5000)
      if (msg.sourceId === 11 && msg.data[0] === 0xbb && msg.data[1] === 0x02) {
        relay = true
      } else {
        errors.push(`relay: unexpected msg ${JSON.stringify(msg)}`)
      }

      workerA2.terminate()
      workerB2.terminate()
    }

    // Test 3: Broadcast from worker -> main.
    let broadcast = false
    {
      const workerC = new Worker(new URL('./workers/bus-peer.js', import.meta.url), { type: 'module' })
      workerC.postMessage({
        busSab,
        pluginId: 20,
        targetId: BROADCAST_ID,
        payload: [0xcc, 0x03],
      })
      await waitWorkerMsg(workerC, 'registered', 5000)
      await waitWorkerMsg(workerC, 'sent', 5000)

      // Main endpoint reads the broadcast.
      const msg = await mainEndpoint.read()
      if (
        msg &&
        msg.sourceId === 20 &&
        msg.targetId === BROADCAST_ID &&
        msg.data[0] === 0xcc
      ) {
        broadcast = true
      } else {
        errors.push(`broadcast: unexpected msg ${JSON.stringify(msg)}`)
      }

      workerC.terminate()
    }

    workerA.terminate()
    workerB.terminate()
    mainEndpoint.close()

    const pass = unicast && relay && broadcast && errors.length === 0
    window.__results = {
      pass,
      detail: errors.length > 0 ? errors.join('; ') : 'all tests passed',
      unicast,
      relay,
      broadcast,
    }
  } catch (err) {
    window.__results = {
      pass: false,
      detail: `error: ${err}`,
      unicast: false,
      relay: false,
      broadcast: false,
    }
  }

  log.textContent = 'DONE'
}

run()
