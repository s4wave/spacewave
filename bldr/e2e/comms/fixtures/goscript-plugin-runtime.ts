import { pushable } from 'it-pushable'
import { ChannelStream, type PacketStream } from 'starpc'

import { detectWorkerCommsConfig } from '../../../web/bldr/worker-comms-detect.js'
import { PluginStartInfo } from '../../../plugin/plugin.pb.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      workerReady: boolean
      startInfo: boolean
      pluginToHostStream: boolean
      hostToPluginStream: boolean
      failureReason?: string
    }
  }
}

type WorkerMessage = { type: string } & Record<string, unknown>

const workerId = 'plugin/goscript-runtime-proof'
const documentId = 'goscript-runtime-proof-doc'

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

function encodeStartInfo(): Uint8Array {
  const json = PluginStartInfo.toJsonString({
    instanceId: 'inst1',
    pluginId: 'goscript-runtime-proof',
    instanceKey: 'default',
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

function connectWorkerRuntime(documentPort: MessagePort): {
  waitPluginToHost: Promise<boolean>
  openHostToPluginStream: () => Promise<boolean>
} {
  let runtimePort: MessagePort | undefined
  const waitPluginToHost = new Promise<boolean>((resolve, reject) => {
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
        reject(new Error('connectWebRuntime missing ack port'))
        return
      }

      const runtimeChannel = new MessageChannel()
      runtimePort = runtimeChannel.port2
      installRuntimePort(runtimePort, resolve, reject)
      ackPort.postMessage(
        {
          from: documentId,
          webRuntimePort: runtimeChannel.port1,
        },
        [runtimeChannel.port1],
      )
    })
    documentPort.start()
  })

  return {
    waitPluginToHost,
    openHostToPluginStream: async () => {
      if (!runtimePort) {
        throw new Error('runtime port is not connected')
      }
      return await openHostToPluginStream(runtimePort)
    },
  }
}

function installRuntimePort(
  port: MessagePort,
  resolvePluginToHost: (ok: boolean) => void,
  rejectPluginToHost: (err: Error) => void,
): void {
  port.onmessage = (ev: MessageEvent) => {
    const data = ev.data
    if (typeof data !== 'object' || data === null) {
      return
    }
    if (data.openStream && ev.ports.length) {
      const stream = new ChannelStream('goscript-runtime-proof', ev.ports[0], {
        remoteOpen: true,
      })
      void handlePluginToHostStream(stream).then(
        resolvePluginToHost,
        rejectPluginToHost,
      )
    }
    if (data.armWebLock) {
      return
    }
  }
  port.start()
  port.postMessage({ connected: true })
}

async function handlePluginToHostStream(stream: PacketStream): Promise<boolean> {
  const outbound = pushable<Uint8Array>({ objectMode: true })
  const sinkDone = stream.sink(outbound)
  for await (const packet of stream.source) {
    if (packet[0] !== 11) {
      throw new Error(`unexpected plugin-to-host packet ${packet[0]}`)
    }
    outbound.push(new Uint8Array([12]))
    outbound.end()
    await sinkDone
    return true
  }
  throw new Error('plugin-to-host stream closed before packet')
}

async function openHostToPluginStream(runtimePort: MessagePort): Promise<boolean> {
  const channel = new MessageChannel()
  const stream = new ChannelStream('goscript-runtime-proof', channel.port1)
  runtimePort.postMessage({ openStream: true }, [channel.port2])
  await stream.waitRemoteOpen

  const outbound = pushable<Uint8Array>({ objectMode: true })
  const sinkDone = stream.sink(outbound)
  outbound.push(new Uint8Array([21]))

  for await (const packet of stream.source) {
    outbound.end()
    await sinkDone
    return packet[0] === 22
  }
  throw new Error('host-to-plugin stream closed before response')
}

async function run() {
  const log = document.getElementById('log')!
  const errors: string[] = []
  let worker: Worker | undefined
  let releaseLock: (() => void) | undefined
  try {
    const detect = await detectWorkerCommsConfig()
    releaseLock = await holdWebDocumentLock(`bldr-doc-${documentId}`)
    worker = new Worker(
      new URL(
        './workers/goscript-plugin-wrapper.js?s=/workers/goscript-runtime-plugin.js&p=1',
        import.meta.url,
      ),
      {
        type: 'module',
        name: `${workerId}?s=/workers/goscript-runtime-plugin.js&p=1`,
      },
    )

    const startInfoPromise = waitWorkerMsg(worker, 'start-info', 5000)
    const acceptReadyPromise = waitWorkerMsg(worker, 'accept-ready', 5000)
    const pluginToHostMessagePromise = waitWorkerMsg(
      worker,
      'plugin-to-host-ok',
      5000,
    )

    const { port1, port2 } = new MessageChannel()
    const runtime = connectWorkerRuntime(port2)
    const workerReadyPromise = waitWorkerReady(port2)
    let failureReason: string | undefined
    port2.addEventListener('message', (ev) => {
      const data = ev.data
      if (typeof data === 'object' && data?.failureReason) {
        failureReason = String(data.failureReason)
      }
    })

    worker.postMessage(
      {
        from: documentId,
        initData: encodeStartInfo(),
        initPort: port1,
        workerCommsDetect: detect,
      },
      [port1],
    )
    port2.postMessage({
      from: documentId,
      resumeReady: true,
    })

    const startInfo = await startInfoPromise
    const startInfoOk =
      startInfo.instanceId === 'inst1' &&
      startInfo.pluginId === 'goscript-runtime-proof' &&
      startInfo.instanceKey === 'default'
    if (!startInfoOk) {
      errors.push(`unexpected start info ${JSON.stringify(startInfo)}`)
    }

    await acceptReadyPromise
    const pluginToHostStream = await runtime.waitPluginToHost
    await pluginToHostMessagePromise
    const hostToPluginStream = await runtime.openHostToPluginStream()
    const workerReady = await workerReadyPromise

    const pass =
      workerReady &&
      startInfoOk &&
      pluginToHostStream &&
      hostToPluginStream &&
      !failureReason &&
      errors.length === 0
    window.__results = {
      pass,
      detail:
        pass ?
          'all tests passed'
        : [
            ...errors,
            `workerReady=${workerReady}`,
            `startInfo=${startInfoOk}`,
            `pluginToHostStream=${pluginToHostStream}`,
            `hostToPluginStream=${hostToPluginStream}`,
            `failureReason=${failureReason ?? ''}`,
          ].join('; '),
      workerReady,
      startInfo: startInfoOk,
      pluginToHostStream,
      hostToPluginStream,
      failureReason,
    }
  } catch (err) {
    window.__results = {
      pass: false,
      detail: `error: ${String(err)}`,
      workerReady: false,
      startInfo: false,
      pluginToHostStream: false,
      hostToPluginStream: false,
      failureReason: undefined,
    }
  } finally {
    releaseLock?.()
    worker?.terminate()
    log.textContent = 'DONE'
  }
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

run()
