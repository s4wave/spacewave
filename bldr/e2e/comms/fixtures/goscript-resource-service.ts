import {
  ChannelStream,
  Client as SRPCClient,
  createHandler,
  createMux,
  type PacketStream,
} from 'starpc'
import {
  MockClient,
  MockDefinition,
  type Mock,
  type MockMsg,
} from 'starpc/mock'
import { EchoerClient } from 'starpc/echo'

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
      nestedParentResponse: boolean
      nestedChildRpc: boolean
      nestedAfterReleaseEngineRpc: boolean
      nestedChildReleaseOnce: boolean
      concurrentChildEchoUnary: boolean
      concurrentChildEchoStreams: boolean
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
): Promise<{
  rootResource: boolean
  registeredViewer: boolean
  releaseRemovedViewer: boolean
  nestedParentResponse: boolean
  nestedChildRpc: boolean
  nestedAfterReleaseEngineRpc: boolean
  nestedChildReleaseOnce: boolean
  concurrentChildEchoUnary: boolean
  concurrentChildEchoStreams: boolean
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
    rootRef = await resourceClient.accessRootResource()
    const registry = new ViewerRegistryResourceServiceClient(rootRef.client)
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
    const registered = await registry.ListViewers({}, abort.signal)
    const registeredRegistrations = registered.registrations ?? []
    const registeredViewer =
      Boolean(response.resourceId) &&
      registeredRegistrations.length === 1 &&
      registeredRegistrations[0]?.componentId === 'spacewave.test.viewer'

    const watch = registry.WatchViewers({}, abort.signal)
    const watchIterator = watch[Symbol.asyncIterator]()
    const initial = await watchIterator.next()
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

    const nested = await proveNestedResourceRpc(resourceClient, rootRef)
    const concurrentChild = await proveConcurrentChildResourceRpc(
      resourceClient,
      rootRef,
    )

    return {
      rootResource: rootRef.resourceId > 0,
      registeredViewer,
      releaseRemovedViewer,
      ...nested,
      ...concurrentChild,
    }
  } finally {
    registrationRef?.release()
    rootRef?.release()
    abort.abort()
    resourceClient.dispose()
  }
}

async function proveConcurrentChildResourceRpc(
  resourceClient: ResourceClient,
  rootRef: Awaited<ReturnType<ResourceClient['accessRootResource']>>,
): Promise<{
  concurrentChildEchoUnary: boolean
  concurrentChildEchoStreams: boolean
}> {
  const rootMock = new MockClient(rootRef.client)
  const response = await rootMock.MockRequest({ body: 'create-echo-child' })
  const childID = Number(
    response.body?.startsWith('echo-child:')
      ? response.body.slice('echo-child:'.length)
      : 0,
  )
  if (!Number.isFinite(childID) || childID <= 0) {
    throw new Error(`unexpected echo child response: ${response.body}`)
  }

  const childRef = resourceClient.createResourceReference(childID)
  try {
    const echo = new EchoerClient(childRef.client)
    const unary = echo.Echo({ body: 'child-unary' })
    const streamA = collectEchoServerStream(
      echo.EchoServerStream({ body: 'child-stream-a' }),
    )
    const streamB = collectEchoServerStream(
      echo.EchoServerStream({ body: 'child-stream-b' }),
    )
    const [unaryResp, streamAResp, streamBResp] = await Promise.all([
      unary,
      streamA,
      streamB,
    ])

    return {
      concurrentChildEchoUnary: unaryResp.body === 'child-unary',
      concurrentChildEchoStreams:
        streamAResp.length === 5 &&
        streamBResp.length === 5 &&
        streamAResp.every((msg) => msg.body === 'child-stream-a') &&
        streamBResp.every((msg) => msg.body === 'child-stream-b'),
    }
  } finally {
    childRef.release()
  }
}

async function collectEchoServerStream<T extends { body?: string }>(
  stream: AsyncIterable<T>,
): Promise<T[]> {
  const out: T[] = []
  for await (const msg of stream) {
    out.push(msg)
  }
  return out
}

async function proveNestedResourceRpc(
  resourceClient: ResourceClient,
  rootRef: Awaited<ReturnType<ResourceClient['accessRootResource']>>,
): Promise<{
  nestedParentResponse: boolean
  nestedChildRpc: boolean
  nestedAfterReleaseEngineRpc: boolean
  nestedChildReleaseOnce: boolean
}> {
  let childRpcObserved = false
  let afterReleaseEngineRpcObserved = false
  let childReleaseCount = 0
  let child:
    | Awaited<ReturnType<ResourceClient['attachResourceTree']>>
    | undefined

  const engineMux = createMux()
  const engine: Mock = {
    async MockRequest(request: MockMsg): Promise<MockMsg> {
      if (request.body === 'create-child') {
        const childMux = createMux()
        const childService: Mock = {
          async MockRequest(childRequest: MockMsg): Promise<MockMsg> {
            if (childRequest.body === 'child-check') {
              childRpcObserved = true
              return { body: 'child-ok' }
            }
            return { body: 'child-unexpected' }
          },
        }
        childMux.register(createHandler(MockDefinition, childService))
        child = await resourceClient.attachResourceTree(
          'nested-child',
          childMux.lookupMethod,
          undefined,
          () => {
            childReleaseCount++
          },
        )
        return { body: `child:${child.resourceId}` }
      }
      if (request.body?.startsWith('release-child:')) {
        child?.cleanup()
        child = undefined
        return { body: 'released-ok' }
      }
      if (request.body === 'after-release') {
        afterReleaseEngineRpcObserved = true
        return { body: 'after-release-ok' }
      }
      return { body: 'engine-unexpected' }
    },
  }
  engineMux.register(createHandler(MockDefinition, engine))

  const engineRef = await resourceClient.attachResourceTree(
    'nested-engine',
    engineMux.lookupMethod,
  )
  try {
    const rootMock = new MockClient(rootRef.client)
    const response = await rootMock.MockRequest({
      body: `run-nested:${engineRef.resourceId}`,
    })
    return {
      nestedParentResponse: response.body === 'seed-ok',
      nestedChildRpc: childRpcObserved,
      nestedAfterReleaseEngineRpc: afterReleaseEngineRpcObserved,
      nestedChildReleaseOnce: childReleaseCount === 1,
    }
  } finally {
    child?.cleanup()
    engineRef.cleanup()
  }
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

    const startInfo = await startInfoPromise
    const startInfoOk = startInfo.startInfoPresent === true
    if (!startInfoOk) {
      errors.push(`unexpected start info ${JSON.stringify(startInfo)}`)
    }

    await acceptReadyPromise
    const workerReady = await workerReadyPromise
    const resource = await proveResourceService(runtime.openStream)

    const pass =
      workerReady &&
      startInfoOk &&
      resource.rootResource &&
      resource.registeredViewer &&
      resource.releaseRemovedViewer &&
      resource.nestedParentResponse &&
      resource.nestedChildRpc &&
      resource.nestedAfterReleaseEngineRpc &&
      resource.nestedChildReleaseOnce &&
      resource.concurrentChildEchoUnary &&
      resource.concurrentChildEchoStreams &&
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
            `nestedParentResponse=${resource.nestedParentResponse}`,
            `nestedChildRpc=${resource.nestedChildRpc}`,
            `nestedAfterReleaseEngineRpc=${resource.nestedAfterReleaseEngineRpc}`,
            `nestedChildReleaseOnce=${resource.nestedChildReleaseOnce}`,
            `concurrentChildEchoUnary=${resource.concurrentChildEchoUnary}`,
            `concurrentChildEchoStreams=${resource.concurrentChildEchoStreams}`,
            `failureReason=${failureReason ?? ''}`,
          ].join('; '),
      workerReady,
      startInfo: startInfoOk,
      rootResource: resource.rootResource,
      registeredViewer: resource.registeredViewer,
      releaseRemovedViewer: resource.releaseRemovedViewer,
      nestedParentResponse: resource.nestedParentResponse,
      nestedChildRpc: resource.nestedChildRpc,
      nestedAfterReleaseEngineRpc: resource.nestedAfterReleaseEngineRpc,
      nestedChildReleaseOnce: resource.nestedChildReleaseOnce,
      concurrentChildEchoUnary: resource.concurrentChildEchoUnary,
      concurrentChildEchoStreams: resource.concurrentChildEchoStreams,
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
      nestedParentResponse: false,
      nestedChildRpc: false,
      nestedAfterReleaseEngineRpc: false,
      nestedChildReleaseOnce: false,
      concurrentChildEchoUnary: false,
      concurrentChildEchoStreams: false,
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
