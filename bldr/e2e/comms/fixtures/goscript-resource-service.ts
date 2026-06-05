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
import { SpaceResourceServiceClient } from '../../../../sdk/space/space_srpc.pb.js'
import {
  EngineResourceServiceDefinition,
  ObjectStateResourceServiceDefinition,
  TxResourceServiceDefinition,
  WorldStateResourceServiceDefinition,
} from '../../../../sdk/world/world_srpc.pb.js'

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
      nestedEngineReleaseOnce: boolean
      worldTreeParentResponse: boolean
      worldTreeEngineRpc: boolean
      worldTreeWorldStateRpc: boolean
      worldTreeObjectStateRpc: boolean
      worldTreeObjectReleaseOnce: boolean
      worldTreeWorldStateReleaseOnce: boolean
      worldTreeEngineReleaseOnce: boolean
      concurrentChildEchoUnary: boolean
      concurrentChildEchoStreams: boolean
      spaceMountContents: boolean
      spaceWatchMountContents: boolean
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
  nestedEngineReleaseOnce: boolean
  worldTreeParentResponse: boolean
  worldTreeEngineRpc: boolean
  worldTreeWorldStateRpc: boolean
  worldTreeObjectStateRpc: boolean
  worldTreeObjectReleaseOnce: boolean
  worldTreeWorldStateReleaseOnce: boolean
  worldTreeEngineReleaseOnce: boolean
  concurrentChildEchoUnary: boolean
  concurrentChildEchoStreams: boolean
  spaceMountContents: boolean
  spaceWatchMountContents: boolean
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
    const worldTree = await proveAttachedWorldResourceTrees(
      resourceClient,
      rootRef,
    )
    const concurrentChild = await proveConcurrentChildResourceRpc(
      resourceClient,
      rootRef,
    )
    const spaceChild = await proveSpaceResourceMountRpc(resourceClient, rootRef)

    return {
      rootResource: rootRef.resourceId > 0,
      registeredViewer,
      releaseRemovedViewer,
      ...nested,
      ...worldTree,
      ...concurrentChild,
      ...spaceChild,
    }
  } finally {
    registrationRef?.release()
    rootRef?.release()
    abort.abort()
    resourceClient.dispose()
  }
}

async function proveSpaceResourceMountRpc(
  resourceClient: ResourceClient,
  rootRef: Awaited<ReturnType<ResourceClient['accessRootResource']>>,
): Promise<{
  spaceMountContents: boolean
  spaceWatchMountContents: boolean
}> {
  const rootMock = new MockClient(rootRef.client)
  const response = await rootMock.MockRequest({ body: 'create-space-child' })
  const childID = Number(
    response.body?.startsWith('space-child:')
      ? response.body.slice('space-child:'.length)
      : 0,
  )
  if (!Number.isFinite(childID) || childID <= 0) {
    throw new Error(`unexpected space child response: ${response.body}`)
  }

  const childRef = resourceClient.createResourceReference(childID)
  try {
    const space = new SpaceResourceServiceClient(childRef.client)
    const mounted = await space.MountSpaceContents({})
    const stateStream = space.WatchSpaceState({})
    const sharingStream = space.WatchSpaceSharingState({})
    const stateIt = stateStream[Symbol.asyncIterator]()
    const sharingIt = sharingStream[Symbol.asyncIterator]()
    const [state, sharing] = await Promise.all([
      stateIt.next(),
      sharingIt.next(),
    ])
    const watchedMount = await space.MountSpaceContents({})
    await Promise.all([
      stateIt.return?.() ?? Promise.resolve(),
      sharingIt.return?.() ?? Promise.resolve(),
    ])
    return {
      spaceMountContents: mounted.resourceId === 4242,
      spaceWatchMountContents:
        !state.done &&
        state.value?.ready === true &&
        !sharing.done &&
        watchedMount.resourceId === 4242,
    }
  } finally {
    childRef.release()
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
  nestedEngineReleaseOnce: boolean
}> {
  let childRpcObserved = false
  let afterReleaseEngineRpcObserved = false
  let childReleaseCount = 0
  let engineReleaseCount = 0
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
    undefined,
    () => {
      engineReleaseCount++
    },
  )
  let response: MockMsg | undefined
  try {
    const rootMock = new MockClient(rootRef.client)
    response = await rootMock.MockRequest({
      body: `run-nested:${engineRef.resourceId}`,
    })
  } finally {
    child?.cleanup()
    engineRef.cleanup()
  }

  return {
    nestedParentResponse: response?.body === 'seed-ok',
    nestedChildRpc: childRpcObserved,
    nestedAfterReleaseEngineRpc: afterReleaseEngineRpcObserved,
    nestedChildReleaseOnce: childReleaseCount === 1,
    nestedEngineReleaseOnce: engineReleaseCount === 1,
  }
}

async function proveAttachedWorldResourceTrees(
  resourceClient: ResourceClient,
  rootRef: Awaited<ReturnType<ResourceClient['accessRootResource']>>,
): Promise<{
  worldTreeParentResponse: boolean
  worldTreeEngineRpc: boolean
  worldTreeWorldStateRpc: boolean
  worldTreeObjectStateRpc: boolean
  worldTreeObjectReleaseOnce: boolean
  worldTreeWorldStateReleaseOnce: boolean
  worldTreeEngineReleaseOnce: boolean
}> {
  const observed: {
    engineRpc: boolean
    worldStateRpc: boolean
    objectStateRpc: boolean
    objectReleaseCount: number
    worldStateReleaseCount: number
    engineReleaseCount: number
    objectRev: bigint
    object?: Awaited<ReturnType<ResourceClient['attachResourceTree']>>
    worldState?: Awaited<ReturnType<ResourceClient['attachResourceTree']>>
  } = {
    engineRpc: false,
    worldStateRpc: false,
    objectStateRpc: false,
    objectReleaseCount: 0,
    worldStateReleaseCount: 0,
    engineReleaseCount: 0,
    objectRev: 11n,
  }

  const objectMux = createMux()
  objectMux.register(
    createHandler(ObjectStateResourceServiceDefinition, {
      async GetKey() {
        observed.objectStateRpc = true
        return { objectKey: 'goscript/world-tree-object' }
      },
      async GetRootRef() {
        observed.objectStateRpc = true
        return {
          rootRef: { bucketId: 'goscript-world-tree-bucket' },
          rev: observed.objectRev,
        }
      },
      async SetRootRef() {
        observed.objectStateRpc = true
        observed.objectRev++
        return { rev: observed.objectRev }
      },
      async AccessWorldState() {
        observed.objectStateRpc = true
        return { resourceId: 0 }
      },
      async ApplyObjectOp() {
        observed.objectStateRpc = true
        return { rev: observed.objectRev, sysErr: false }
      },
      async IncrementRev() {
        observed.objectStateRpc = true
        observed.objectRev++
        return { rev: observed.objectRev }
      },
      async WaitRev(request: { rev?: bigint }) {
        observed.objectStateRpc = true
        const requestedRev = request.rev ?? 0n
        if (observed.objectRev < requestedRev) {
          observed.objectRev = requestedRev
        }
        return { rev: observed.objectRev }
      },
    }),
  )

  const worldStateMux = createMux()
  worldStateMux.register(
    createHandler(WorldStateResourceServiceDefinition, {
      async GetReadOnly() {
        observed.worldStateRpc = true
        return { readOnly: false }
      },
      async GetSeqno() {
        observed.worldStateRpc = true
        return { seqno: 8n }
      },
      async WaitSeqno(request: { seqno?: bigint }) {
        observed.worldStateRpc = true
        return { seqno: request.seqno ?? 8n }
      },
      async CreateObject(request: { objectKey?: string }) {
        observed.worldStateRpc = true
        observed.object = await resourceClient.attachResourceTree(
          'world-tree-object-state',
          objectMux.lookupMethod,
          undefined,
          () => {
            observed.objectReleaseCount++
          },
        )
        return {
          resourceId: observed.object.resourceId,
          objectKey: request.objectKey ?? '',
        }
      },
    }),
  )
  worldStateMux.register(
    createHandler(TxResourceServiceDefinition, {
      async Commit() {
        observed.worldStateRpc = true
        return {}
      },
      async Discard() {
        observed.worldStateRpc = true
        return {}
      },
    }),
  )

  const engineMux = createMux()
  engineMux.register(
    createHandler(EngineResourceServiceDefinition, {
      async GetEngineInfo() {
        observed.engineRpc = true
        return {
          engineInfo: {
            engineId: 'goscript-world-tree-engine',
            bucketId: 'goscript-world-tree-bucket',
          },
        }
      },
      async GetWorldRootSnapshot() {
        observed.engineRpc = true
        return {
          seqno: 7n,
          engineInfo: {
            engineId: 'goscript-world-tree-engine',
            bucketId: 'goscript-world-tree-bucket',
          },
        }
      },
      async NewTransaction(request: { write?: boolean }) {
        observed.engineRpc = true
        if (request.write !== true) {
          throw new Error(`unexpected NewTransaction write=${request.write}`)
        }
        observed.worldState = await resourceClient.attachResourceTree(
          'world-tree-world-state',
          worldStateMux.lookupMethod,
          undefined,
          () => {
            observed.worldStateReleaseCount++
          },
        )
        return { resourceId: observed.worldState.resourceId, readOnly: false }
      },
      async GetSeqno() {
        observed.engineRpc = true
        return { seqno: 7n }
      },
      async WaitSeqno(request: { seqno?: bigint }) {
        observed.engineRpc = true
        return { seqno: request.seqno ?? 7n }
      },
    }),
  )

  const engine = await resourceClient.attachResourceTree(
    'world-tree-engine',
    engineMux.lookupMethod,
    undefined,
    () => {
      observed.engineReleaseCount++
    },
  )
  try {
    const rootMock = new MockClient(rootRef.client)
    const response = await rootMock.MockRequest({
      body: `run-world-tree:${engine.resourceId}`,
    })
    await waitForResourceTreeRelease(() =>
      observed.objectReleaseCount === 1 &&
      observed.worldStateReleaseCount === 1 &&
      observed.engineReleaseCount === 1
        ? true
        : `object=${observed.objectReleaseCount} world=${observed.worldStateReleaseCount} engine=${observed.engineReleaseCount}`,
    )
    return {
      worldTreeParentResponse: response.body === 'world-tree-ok',
      worldTreeEngineRpc: observed.engineRpc,
      worldTreeWorldStateRpc: observed.worldStateRpc,
      worldTreeObjectStateRpc: observed.objectStateRpc,
      worldTreeObjectReleaseOnce: observed.objectReleaseCount === 1,
      worldTreeWorldStateReleaseOnce: observed.worldStateReleaseCount === 1,
      worldTreeEngineReleaseOnce: observed.engineReleaseCount === 1,
    }
  } finally {
    observed.object?.cleanup()
    observed.worldState?.cleanup()
    engine.cleanup()
  }
}

async function waitForResourceTreeRelease(
  isReleased: () => true | string,
  remaining = 50,
  last = '',
): Promise<void> {
  const released = isReleased()
  if (released === true) {
    return
  }
  if (remaining <= 0) {
    throw new Error(`resource tree release counts did not settle: ${last}`)
  }
  await new Promise((resolve) => setTimeout(resolve, 20))
  await waitForResourceTreeRelease(isReleased, remaining - 1, released)
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
      resource.nestedEngineReleaseOnce &&
      resource.worldTreeParentResponse &&
      resource.worldTreeEngineRpc &&
      resource.worldTreeWorldStateRpc &&
      resource.worldTreeObjectStateRpc &&
      resource.worldTreeObjectReleaseOnce &&
      resource.worldTreeWorldStateReleaseOnce &&
      resource.worldTreeEngineReleaseOnce &&
      resource.concurrentChildEchoUnary &&
      resource.concurrentChildEchoStreams &&
      resource.spaceMountContents &&
      resource.spaceWatchMountContents &&
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
            `nestedEngineReleaseOnce=${resource.nestedEngineReleaseOnce}`,
            `worldTreeParentResponse=${resource.worldTreeParentResponse}`,
            `worldTreeEngineRpc=${resource.worldTreeEngineRpc}`,
            `worldTreeWorldStateRpc=${resource.worldTreeWorldStateRpc}`,
            `worldTreeObjectStateRpc=${resource.worldTreeObjectStateRpc}`,
            `worldTreeObjectReleaseOnce=${resource.worldTreeObjectReleaseOnce}`,
            `worldTreeWorldStateReleaseOnce=${resource.worldTreeWorldStateReleaseOnce}`,
            `worldTreeEngineReleaseOnce=${resource.worldTreeEngineReleaseOnce}`,
            `concurrentChildEchoUnary=${resource.concurrentChildEchoUnary}`,
            `concurrentChildEchoStreams=${resource.concurrentChildEchoStreams}`,
            `spaceMountContents=${resource.spaceMountContents}`,
            `spaceWatchMountContents=${resource.spaceWatchMountContents}`,
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
      nestedEngineReleaseOnce: resource.nestedEngineReleaseOnce,
      worldTreeParentResponse: resource.worldTreeParentResponse,
      worldTreeEngineRpc: resource.worldTreeEngineRpc,
      worldTreeWorldStateRpc: resource.worldTreeWorldStateRpc,
      worldTreeObjectStateRpc: resource.worldTreeObjectStateRpc,
      worldTreeObjectReleaseOnce: resource.worldTreeObjectReleaseOnce,
      worldTreeWorldStateReleaseOnce: resource.worldTreeWorldStateReleaseOnce,
      worldTreeEngineReleaseOnce: resource.worldTreeEngineReleaseOnce,
      concurrentChildEchoUnary: resource.concurrentChildEchoUnary,
      concurrentChildEchoStreams: resource.concurrentChildEchoStreams,
      spaceMountContents: resource.spaceMountContents,
      spaceWatchMountContents: resource.spaceWatchMountContents,
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
      nestedEngineReleaseOnce: false,
      worldTreeParentResponse: false,
      worldTreeEngineRpc: false,
      worldTreeWorldStateRpc: false,
      worldTreeObjectStateRpc: false,
      worldTreeObjectReleaseOnce: false,
      worldTreeWorldStateReleaseOnce: false,
      worldTreeEngineReleaseOnce: false,
      concurrentChildEchoUnary: false,
      concurrentChildEchoStreams: false,
      spaceMountContents: false,
      spaceWatchMountContents: false,
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
