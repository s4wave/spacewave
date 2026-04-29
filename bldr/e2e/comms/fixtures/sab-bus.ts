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
interface TrackedWorker {
  name: string
  worker: Worker
  messages: WorkerMessage[]
  closed: boolean
}

function createTrackedWorker(name: string): TrackedWorker {
  const worker = new Worker(new URL('./workers/bus-peer.js', import.meta.url), {
    type: 'module',
  })
  const messages: WorkerMessage[] = []
  worker.addEventListener('message', (ev: MessageEvent<unknown>) => {
    if (typeof ev.data !== 'object' || ev.data === null) return
    messages.push(ev.data as WorkerMessage)
  })
  worker.addEventListener('error', (ev) => {
    messages.push({
      type: 'worker-error',
      message: ev.message,
      filename: ev.filename,
      lineno: ev.lineno,
      colno: ev.colno,
    })
  })
  return { name, worker, messages, closed: false }
}

function summarizeMessages(worker: TrackedWorker): string {
  const tail = worker.messages.slice(-8)
  if (tail.length === 0) {
    return 'no worker messages'
  }
  return JSON.stringify(tail)
}

function findWorkerMsg(
  tracked: TrackedWorker,
  type: string,
): WorkerMessage | undefined {
  return tracked.messages.find((msg) => msg.type === type)
}

async function waitWorkerMsg(
  tracked: TrackedWorker,
  stage: string,
  type: string,
  timeoutMs: number,
): Promise<WorkerMessage> {
  const existing = findWorkerMsg(tracked, type)
  if (existing) {
    return existing
  }
  return new Promise((resolve, reject) => {
    const timer = setTimeout(
      () =>
        reject(
          new Error(
            `${stage}: timeout waiting for ${tracked.name} ${type}; recent=${summarizeMessages(tracked)}`,
          ),
        ),
      timeoutMs,
    )
    const handler = (ev: MessageEvent<unknown>) => {
      if (typeof ev.data !== 'object' || ev.data === null) return
      const msg = ev.data as WorkerMessage
      if (msg.type === type) {
        clearTimeout(timer)
        tracked.worker.removeEventListener('message', handler)
        resolve(msg)
      }
    }
    tracked.worker.addEventListener('message', handler)
  })
}

async function closeWorker(tracked: TrackedWorker): Promise<void> {
  if (tracked.closed) {
    return
  }
  tracked.closed = true
  tracked.worker.postMessage({ type: 'close' })
  try {
    await waitWorkerMsg(tracked, 'cleanup', 'closed', 1000)
  } catch (err) {
    console.warn(`${tracked.name}: close ack failed`, err)
  }
  tracked.worker.terminate()
}

async function run() {
  const log = document.getElementById('log')!
  const errors: string[] = []
  const workers: TrackedWorker[] = []
  let mainEndpoint: SabBusEndpoint | undefined

  function spawnWorker(name: string): TrackedWorker {
    const worker = createTrackedWorker(name)
    workers.push(worker)
    return worker
  }

  try {
    const busOpts = { slotSize: 256, numSlots: 32 }
    const busSab = createBusSab(busOpts)

    // Main thread endpoint (pluginId=0).
    mainEndpoint = new SabBusEndpoint(busSab, 0, busOpts)
    mainEndpoint.register()

    // Spawn worker A (pluginId=1) and worker B (pluginId=2).
    const workerA = spawnWorker('workerA')
    const workerB = spawnWorker('workerB')

    // Init worker A: register, then read one message.
    workerA.worker.postMessage({
      busSab,
      pluginId: 1,
      stage: 'unicast-worker-a',
      readOne: true,
    })
    await waitWorkerMsg(workerA, 'register-worker-a', 'registered', 5000)
    await waitWorkerMsg(workerA, 'unicast-worker-a', 'read-started', 5000)

    // Init worker B: register, then read one message.
    workerB.worker.postMessage({
      busSab,
      pluginId: 2,
      stage: 'idle-worker-b',
      readOne: true,
    })
    await waitWorkerMsg(workerB, 'register-worker-b', 'registered', 5000)
    await waitWorkerMsg(workerB, 'idle-worker-b', 'read-started', 5000)

    // Test 1: Unicast main(0) -> worker A(1).
    let unicast = false
    {
      await mainEndpoint.write(1, new Uint8Array([0xaa, 0x01]))
      const msg = await waitWorkerMsg(
        workerA,
        'unicast-main-to-worker-a',
        'received',
        5000,
      )
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
      const workerA2 = spawnWorker('workerA2')
      const workerB2 = spawnWorker('workerB2')

      workerB2.worker.postMessage({
        busSab,
        pluginId: 12,
        stage: 'relay-receiver',
        readOne: true,
      })
      await waitWorkerMsg(
        workerB2,
        'register-relay-receiver',
        'registered',
        5000,
      )
      await waitWorkerMsg(workerB2, 'relay-receiver', 'read-started', 5000)

      workerA2.worker.postMessage({
        busSab,
        pluginId: 11,
        stage: 'relay-sender',
        targetId: 12,
        payload: [0xbb, 0x02],
      })
      await waitWorkerMsg(workerA2, 'register-relay-sender', 'registered', 5000)
      await waitWorkerMsg(workerA2, 'relay-sender', 'sent', 5000)

      const msg = await waitWorkerMsg(
        workerB2,
        'relay-worker-a-to-worker-b',
        'received',
        5000,
      )
      if (msg.sourceId === 11 && msg.data[0] === 0xbb && msg.data[1] === 0x02) {
        relay = true
      } else {
        errors.push(`relay: unexpected msg ${JSON.stringify(msg)}`)
      }
    }

    // Test 3: Broadcast from worker -> main.
    let broadcast = false
    {
      const workerC = spawnWorker('workerC')
      workerC.worker.postMessage({
        busSab,
        pluginId: 20,
        stage: 'broadcast-sender',
        targetId: BROADCAST_ID,
        payload: [0xcc, 0x03],
      })
      await waitWorkerMsg(
        workerC,
        'register-broadcast-sender',
        'registered',
        5000,
      )
      await waitWorkerMsg(workerC, 'broadcast-sender', 'sent', 5000)

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
    }

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
  } finally {
    await Promise.all(workers.map((worker) => closeWorker(worker)))
    mainEndpoint?.close()
  }

  log.textContent = 'DONE'
}

run()
