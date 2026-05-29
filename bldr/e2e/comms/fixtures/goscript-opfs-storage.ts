import { detectWorkerCommsConfig } from '../../../web/bldr/worker-comms-detect.js'
import { PluginStartInfo } from '../../../plugin/plugin.pb.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      workerReady: boolean
      write: boolean
      reloadRead: boolean
      cleanup: boolean
      failureReason?: string
    }
  }
}

type WorkerMessage = { type: string } & Record<string, unknown>

const workerId = 'plugin/goscript-opfs-storage-proof'
const documentId = 'goscript-opfs-storage-proof-doc'

async function holdWebDocumentLock(name: string): Promise<() => void> {
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

function encodeStartInfo(mode: string): Uint8Array {
  const json = PluginStartInfo.toJsonString({
    instanceId: 'inst1',
    pluginId: 'goscript-opfs-storage-proof',
    instanceKey: mode,
  })
  return new TextEncoder().encode(btoa(json))
}

function waitWorkerMsg(
  worker: Worker,
  type: string,
  timeoutMs: number,
): Promise<WorkerMessage> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      cleanup()
      reject(new Error(`timeout waiting for ${type}`))
    }, timeoutMs)
    const handler = (ev: MessageEvent<unknown>) => {
      if (typeof ev.data !== 'object' || ev.data === null) {
        return
      }
      const msg = ev.data as WorkerMessage
      if (msg.type !== type) {
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

function connectWorkerRuntime(documentPort: MessagePort): void {
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

async function runOpfsWorker(
  detect: Awaited<ReturnType<typeof detectWorkerCommsConfig>>,
  mode: string,
): Promise<{ workerReady: boolean; done: boolean; failureReason?: string }> {
  const worker = new Worker(
    new URL(
      './workers/goscript-plugin-wrapper.js?s=/workers/goscript-opfs-storage-plugin.js&p=1',
      import.meta.url,
    ),
    {
      type: 'module',
      name: `${workerId}?s=/workers/goscript-opfs-storage-plugin.js&p=1`,
    },
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
      if (typeof data === 'object' && data?.failureReason) {
        failureReason = String(data.failureReason)
      }
    })

    const donePromise = waitWorkerMsg(worker, 'opfs-done', 30000)
    const { port1, port2 } = new MessageChannel()
    connectWorkerRuntime(port2)
    const workerReadyPromise = waitWorkerReady(port2)

    worker.postMessage(
      {
        from: documentId,
        initData: encodeStartInfo(mode),
        initPort: port1,
        workerCommsDetect: detect,
      },
      [port1],
    )
    port2.postMessage({
      from: documentId,
      resumeReady: true,
    })

    await donePromise
    return {
      workerReady: await workerReadyPromise,
      done: true,
      failureReason,
    }
  } finally {
    worker.terminate()
  }
}

async function run() {
  const log = document.getElementById('log')!
  let releaseLock: (() => void) | undefined
  try {
    const detect = await detectWorkerCommsConfig()
    releaseLock = await holdWebDocumentLock(`bldr-doc-${documentId}`)
    const write = await runOpfsWorker(detect, 'write')
    const read = await runOpfsWorker(detect, 'read')
    const cleanup = await verifyCleanup()
    const failureReason = write.failureReason ?? read.failureReason
    const workerReady = write.workerReady && read.workerReady
    const pass =
      workerReady && write.done && read.done && cleanup && !failureReason
    window.__results = {
      pass,
      detail:
        pass ?
          'all tests passed'
        : [
            `workerReady=${workerReady}`,
            `write=${write.done}`,
            `reloadRead=${read.done}`,
            `cleanup=${cleanup}`,
            `failureReason=${failureReason ?? ''}`,
          ].join('; '),
      workerReady,
      write: write.done,
      reloadRead: read.done,
      cleanup,
      failureReason,
    }
  } catch (err) {
    window.__results = {
      pass: false,
      detail: `error: ${String(err)}`,
      workerReady: false,
      write: false,
      reloadRead: false,
      cleanup: false,
      failureReason: undefined,
    }
  } finally {
    releaseLock?.()
    log.textContent = 'DONE'
  }
}

async function verifyCleanup(): Promise<boolean> {
  const root = await navigator.storage.getDirectory()
  try {
    await root.getDirectoryHandle('goscript-opfs-storage-proof', {
      create: false,
    })
    return false
  } catch (err) {
    return err instanceof DOMException && err.name === 'NotFoundError'
  }
}

run()
