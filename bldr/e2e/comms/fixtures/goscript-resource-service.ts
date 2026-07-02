import { ChannelStream, Client as SRPCClient, type PacketStream } from 'starpc'

import { detectWorkerCommsConfig } from '../../../web/bldr/worker-comms-detect.js'
import { PluginStartInfo } from '../../../plugin/plugin.pb.js'
import { Client as ResourceClient } from '../../../sdk/resource/client.js'
import { ResourceServiceClient } from '../../../resource/resource_srpc.pb.js'
import { ViewerRegistryResourceServiceClient } from '../../../../sdk/viewer/registry/registry_srpc.pb.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      workerReady: boolean
      startInfo: boolean
      rootResource: boolean
      registeredViewer: boolean
      releaseRemovedViewer: boolean
      failureReason?: string
    }
  }
}

type WorkerMessage = { type: string } & Record<string, unknown>

const workerId = 'plugin/goscript-resource-service-proof'
const documentId = 'goscript-resource-service-proof-doc'

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
    pluginId: 'goscript-resource-service-proof',
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
  openStream: () => Promise<PacketStream>
} {
  let runtimePort: MessagePort | undefined
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
    runtimePort = runtimeChannel.port2
    runtimePort.start()
    runtimePort.postMessage({ connected: true })
    ackPort.postMessage(
      {
        from: documentId,
        webRuntimePort: runtimeChannel.port1,
      },
      [runtimeChannel.port1],
    )
  })
  documentPort.start()

  return {
    openStream: async () => {
      if (!runtimePort) {
        throw new Error('runtime port is not connected')
      }
      const channel = new MessageChannel()
      const stream = new ChannelStream(
        'goscript-resource-service-proof',
        channel.port1,
      )
      runtimePort.postMessage({ openStream: true }, [channel.port2])
      await stream.waitRemoteOpen
      return stream
    },
  }
}

async function proveResourceService(
  openStream: () => Promise<PacketStream>,
  mark: (step: string) => void,
): Promise<{
  rootResource: boolean
  registeredViewer: boolean
  releaseRemovedViewer: boolean
}> {
  const abort = new AbortController()
  const srpc = new SRPCClient(openStream)
  const resourceService = new ResourceServiceClient(srpc)
  const resourceClient = new ResourceClient(resourceService, abort.signal)
  let rootRef:
    | Awaited<ReturnType<ResourceClient['accessRootResource']>>
    | undefined
  let registrationRef:
    | ReturnType<ResourceClient['createResourceReference']>
    | undefined
  try {
    mark('resource-access-root')
    rootRef = await resourceClient.accessRootResource()
    const registry = new ViewerRegistryResourceServiceClient(rootRef.client)
    mark('resource-register-viewer')
    const response = await registry.RegisterViewer(
      {
        registration: {
          typeId: 'spacewave/test',
          viewerName: 'Test Viewer',
          scriptPath: '/viewer.js',
          componentId: 'spacewave.test.viewer',
        },
      },
      abort.signal,
    )
    mark('resource-list-viewers')
    const registered = await registry.ListViewers({}, abort.signal)
    const registeredRegistrations = registered.registrations ?? []
    const registeredViewer =
      Boolean(response.resourceId) &&
      registeredRegistrations.length === 1 &&
      registeredRegistrations[0]?.componentId === 'spacewave.test.viewer'

    mark('resource-watch-viewers')
    const watch = registry.WatchViewers({}, abort.signal)
    const watchIterator = watch[Symbol.asyncIterator]()
    const initial = await watchIterator.next()
    mark('resource-release-registration')
    registrationRef = resourceClient.createResourceReference(
      response.resourceId ?? 0,
    )
    registrationRef.release()
    const released = await watchIterator.next()
    await watchIterator.return?.()

    const initialRegistrations = initial.value?.registrations ?? []
    const releasedRegistrations = released.value?.registrations ?? []
    const releaseRemovedViewer =
      initialRegistrations.length === 1 && releasedRegistrations.length === 0

    return {
      rootResource: rootRef.resourceId > 0,
      registeredViewer,
      releaseRemovedViewer,
    }
  } finally {
    registrationRef?.release()
    rootRef?.release()
    abort.abort()
    resourceClient.dispose()
  }
}

async function run() {
  const log = document.getElementById('log')!
  const mark = (step: string) => {
    log.textContent = `RUNNING ${step}`
  }
  const errors: string[] = []
  let worker: Worker | undefined
  let releaseLock: (() => void) | undefined
  try {
    mark('detect-comms')
    const detect = await detectWorkerCommsConfig()
    mark('hold-document-lock')
    releaseLock = await holdWebDocumentLock(`bldr-doc-${documentId}`)
    mark('start-worker')
    worker = new Worker(
      new URL(
        './workers/goscript-plugin-wrapper.js?s=/workers/goscript-resource-service-plugin.js&p=1',
        import.meta.url,
      ),
      {
        type: 'module',
        name: `${workerId}?s=/workers/goscript-resource-service-plugin.js&p=1`,
      },
    )

    const startInfoPromise = waitWorkerMsg(worker, 'start-info', 30000)
    const acceptReadyPromise = waitWorkerMsg(worker, 'accept-ready', 30000)

    const { port1, port2 } = new MessageChannel()
    const runtime = connectWorkerRuntime(port2)
    const workerReadyPromise = waitWorkerReady(port2)
    let failureReason: string | undefined
    worker.addEventListener('error', (ev) => {
      failureReason = `${ev.message} ${ev.filename}:${ev.lineno}:${ev.colno}`
    })
    worker.addEventListener('messageerror', () => {
      failureReason = 'worker messageerror'
    })
    port2.addEventListener('message', (ev) => {
      const data = ev.data
      if (typeof data === 'object' && data?.failureReason) {
        failureReason = String(data.failureReason)
      }
    })
    worker.addEventListener('message', (ev) => {
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

    mark('wait-start-info')
    const startInfo = await startInfoPromise
    const startInfoOk = startInfo.startInfoPresent === true
    if (!startInfoOk) {
      errors.push(`unexpected start info ${JSON.stringify(startInfo)}`)
    }

    mark('wait-accept-ready')
    await acceptReadyPromise
    mark('wait-worker-ready')
    const workerReady = await workerReadyPromise
    mark('prove-resource-service')
    const resource = await proveResourceService(runtime.openStream, mark)

    const pass =
      workerReady &&
      startInfoOk &&
      resource.rootResource &&
      resource.registeredViewer &&
      resource.releaseRemovedViewer &&
      !failureReason &&
      errors.length === 0
    window.__results = {
      pass,
      detail: pass
        ? 'all tests passed'
        : [
            ...errors,
            `workerReady=${workerReady}`,
            `startInfo=${startInfoOk}`,
            `rootResource=${resource.rootResource}`,
            `registeredViewer=${resource.registeredViewer}`,
            `releaseRemovedViewer=${resource.releaseRemovedViewer}`,
            `failureReason=${failureReason ?? ''}`,
          ].join('; '),
      workerReady,
      startInfo: startInfoOk,
      rootResource: resource.rootResource,
      registeredViewer: resource.registeredViewer,
      releaseRemovedViewer: resource.releaseRemovedViewer,
      failureReason,
    }
  } catch (err) {
    window.__results = {
      pass: false,
      detail: `error: ${String(err)}`,
      workerReady: false,
      startInfo: false,
      rootResource: false,
      registeredViewer: false,
      releaseRemovedViewer: false,
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
