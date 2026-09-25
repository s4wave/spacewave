import { detectWorkerCommsConfig } from '../../../web/bldr/worker-comms-detect.js'
import { PluginStartInfo } from '../../../plugin/plugin.pb.js'

// WorkerMessage is a message a GoScript fixture worker posts.
export type WorkerMessage = { type: string } & Record<string, unknown>

// WorkerComms is the detected worker communication configuration.
export type WorkerComms = Awaited<ReturnType<typeof detectWorkerCommsConfig>>

// GoScriptWorkerRun describes one GoScript fixture worker run.
export interface GoScriptWorkerRun {
  // script is the worker bundle under /workers/.
  script: string
  // pluginId is the plugin id in the start info.
  pluginId: string
  // documentId is the WebDocument id the worker connects to.
  documentId: string
  // mode is passed to the worker as the start info instance key.
  mode: string
  // doneType is the message type that completes the run.
  doneType: string
  // failedType is the message type that fails the run.
  failedType: string
  // timeoutMs bounds the wait for doneType.
  timeoutMs: number
}

// GoScriptWorkerResult is the outcome of one GoScript fixture worker run.
export interface GoScriptWorkerResult {
  workerReady: boolean
  done: WorkerMessage
  failureReason?: string
}

export { detectWorkerCommsConfig }

// holdWebDocumentLock holds the WebDocument liveness lock until released.
export async function holdWebDocumentLock(name: string): Promise<() => void> {
  let releaseLock: (() => void) | undefined
  const waitReleased = new Promise<void>((resolve) => {
    releaseLock = resolve
  })
  const waitReady = new Promise<void>((resolve, reject) => {
    navigator.locks
      .request(name, async () => {
        resolve()
        await waitReleased
      })
      .catch(reject)
  })
  await waitReady
  return () => releaseLock?.()
}

// runGoScriptWorker starts a GoScript plugin worker, waits for its done
// message, and terminates it.
export async function runGoScriptWorker(
  detect: WorkerComms,
  run: GoScriptWorkerRun,
): Promise<GoScriptWorkerResult> {
  const path = `?s=/workers/${run.script}&p=1`
  const worker = new Worker(
    new URL('/workers/goscript-plugin-wrapper.js' + path, location.href),
    { type: 'module', name: `plugin/${run.pluginId}${path}` },
  )
  try {
    let failureReason: string | undefined
    worker.addEventListener('error', (ev) => {
      failureReason = `${ev.message} ${ev.filename}:${ev.lineno}:${ev.colno}`
    })
    worker.addEventListener('messageerror', () => {
      failureReason = 'worker messageerror'
    })
    worker.addEventListener('message', (ev) => {
      const data = ev.data
      if (typeof data !== 'object' || data === null) {
        return
      }
      if (data.failureReason) {
        failureReason = String(data.failureReason)
      }
      if (data.line) {
        console.log(`${run.pluginId}: ${String(data.line)}`)
      }
    })

    const donePromise = waitWorkerMsg(worker, run)
    const { port1, port2 } = new MessageChannel()
    connectWorkerRuntime(port2, run.documentId)
    const workerReadyPromise = waitWorkerReady(port2)

    worker.postMessage(
      {
        from: run.documentId,
        initData: encodeStartInfo(run),
        initPort: port1,
        workerCommsDetect: detect,
      },
      [port1],
    )
    port2.postMessage({
      from: run.documentId,
      resumeReady: true,
      runtimeConnected: true,
    })

    const done = await donePromise
    return { workerReady: await workerReadyPromise, done, failureReason }
  } finally {
    worker.terminate()
  }
}

function encodeStartInfo(run: GoScriptWorkerRun): Uint8Array {
  const json = PluginStartInfo.toJsonString({
    instanceId: 'inst1',
    pluginId: run.pluginId,
    instanceKey: run.mode,
  })
  return new TextEncoder().encode(btoa(json))
}

function waitWorkerMsg(
  worker: Worker,
  run: GoScriptWorkerRun,
): Promise<WorkerMessage> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      cleanup()
      reject(new Error(`timeout waiting for ${run.doneType}`))
    }, run.timeoutMs)
    const handler = (ev: MessageEvent<unknown>) => {
      if (typeof ev.data !== 'object' || ev.data === null) {
        return
      }
      const msg = ev.data as WorkerMessage
      if (msg.type === run.failedType) {
        cleanup()
        reject(new Error(String(msg.failureReason)))
        return
      }
      if (msg.type !== run.doneType) {
        return
      }
      cleanup()
      resolve(msg)
    }
    const cleanup = () => {
      clearTimeout(timer)
      worker.removeEventListener('message', handler)
    }
    worker.addEventListener('message', handler)
  })
}

function connectWorkerRuntime(
  documentPort: MessagePort,
  documentId: string,
): void {
  documentPort.addEventListener('message', (ev) => {
    const data = ev.data
    if (typeof data !== 'object' || data === null) {
      return
    }
    if (data.connectWebRtcBridge) {
      const { port1: clientPort, port2: bridgePort } = new MessageChannel()
      bridgePort.close()
      documentPort.postMessage(
        {
          from: documentId,
          requestId: data.connectWebRtcBridge.requestId,
          bridgePort: clientPort,
        },
        [clientPort],
      )
      return
    }
    if (!data.connectWebRuntime) {
      return
    }

    const ackPort = data.connectWebRuntime.port ?? ev.ports[0]
    if (!ackPort) {
      throw new Error('connectWebRuntime missing ack port')
    }

    const runtimeChannel = new MessageChannel()
    runtimeChannel.port2.start()
    runtimeChannel.port2.postMessage({ connected: true })
    ackPort.postMessage(
      {
        from: documentId,
        webRuntimePort: runtimeChannel.port1,
      },
      [runtimeChannel.port1],
    )
  })
  documentPort.start()
}

function waitWorkerReady(port: MessagePort): Promise<boolean> {
  return new Promise((resolve) => {
    const timer = setTimeout(() => resolve(false), 5000)
    const handler = (ev: MessageEvent) => {
      const data = ev.data
      if (typeof data !== 'object' || !data?.ready) {
        return
      }
      clearTimeout(timer)
      port.removeEventListener('message', handler)
      resolve(true)
    }
    port.addEventListener('message', handler)
  })
}
