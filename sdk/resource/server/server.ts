import {
  createMux,
  createHandler,
  Server,
  handleRpcStream,
  StreamConn,
  combineUint8ArrayListTransform,
} from 'starpc'
import type { Mux, LookupMethod, MessageStream } from 'starpc'
import type { RpcStreamPacket } from 'starpc'
import { pushable } from 'it-pushable'
import { pipe } from 'it-pipe'
import type {
  ResourceAttachRequest,
  ResourceAttachResponse,
  ResourceClientRequest,
  ResourceClientResponse,
  ResourceRefReleaseRequest,
  ResourceRefReleaseResponse,
} from '../resource.pb.js'
import { ResourceServiceDefinition } from '../resource_srpc.pb.js'
import type { ResourceService } from '../resource_srpc.pb.js'
import { RemoteResourceClient } from './tracked-client.js'

// _currentRpcClient holds the RemoteResourceClient for the current
// RPC invocation. Set by the wrapped InvokeFn in ResourceRpc,
// cleared after dispatch.
let _currentRpcClient: RemoteResourceClient | undefined

// getCurrentResourceClient returns the RemoteResourceClient for
// the current RPC invocation. Must be called synchronously
// (before any await) within an RPC handler dispatched through
// ResourceRpc.
function getCurrentResourceClient(): RemoteResourceClient {
  if (!_currentRpcClient) {
    throw new Error('no resource client context')
  }
  return _currentRpcClient
}

// ResourceServer manages a tree of resources accessible over
// SRPC. Clients connect via ResourceClient, receive a root
// resource ID, and make RPCs to individual resources via
// ResourceRpc.
class ResourceServer implements ResourceService {
  private rootResourceMux: Mux
  private clientHandleIDCtr = 0
  private resourceIDCtr = 0
  private clients = new Map<number, RemoteResourceClient>()

  constructor(rootResourceMux?: Mux) {
    this.rootResourceMux = rootResourceMux ?? createMux()
  }

  // register wires this server into an outer SRPC mux.
  register(mux: { register(handler: { getServiceID(): string }): void }): void {
    mux.register(createHandler(ResourceServiceDefinition, this))
  }

  // nextResourceID allocates a globally unique resource ID.
  nextResourceID(): number {
    return ++this.resourceIDCtr
  }

  // ResourceClient implements the server-streaming RPC.
  // Sends Init, then transmit loop draining txQueue.
  async *ResourceClient(
    _request: ResourceClientRequest,
    abortSignal?: AbortSignal,
  ): MessageStream<ResourceClientResponse> {
    const clientID = ++this.clientHandleIDCtr
    const client = new RemoteResourceClient(
      () => this.nextResourceID(),
      clientID,
      abortSignal,
    )
    this.clients.set(clientID, client)

    const rootResourceID = client.addResource(
      this.rootResourceMux,
      undefined,
    )

    try {
      yield {
        body: {
          case: 'init' as const,
          value: {
            clientHandleId: clientID,
            rootResourceId: rootResourceID,
          },
        },
      }

      while (!abortSignal?.aborted && !client.released) {
        await client.waitForNotify(abortSignal)
        const msgs = client.drainQueue()
        for (const msg of msgs) {
          yield msg
        }
      }
    } finally {
      client.released = true
      this.clients.delete(clientID)
      client.controller.abort()

      for (const [, attached] of client.attachedResources) {
        attached.controller.abort()
      }
      client.attachedResources.clear()

      for (const [, resource] of client.resources) {
        if (resource.releaseFn) {
          const fn = resource.releaseFn
          queueMicrotask(() => fn())
        }
      }
      client.resources.clear()
    }
  }

  // findResource scans all clients for a resource by ID.
  // Resource IDs are globally unique.
  private findResource(
    resourceID: number,
  ): { mux: Mux; client: RemoteResourceClient } | undefined {
    for (const [, client] of this.clients) {
      if (client.released) continue
      const resource = client.resources.get(resourceID)
      if (resource) {
        return { mux: resource.mux, client }
      }
    }
    return undefined
  }

  // ResourceRpc implements the bidi-streaming RPC.
  // Routes sub-RPCs to resources by componentId (decimal resource ID).
  ResourceRpc(
    request: MessageStream<RpcStreamPacket>,
    _abortSignal?: AbortSignal,
  ): MessageStream<RpcStreamPacket> {
    return handleRpcStream(
      request[Symbol.asyncIterator](),
      async (componentId: string) => {
        const resourceID = parseInt(componentId, 10)
        if (isNaN(resourceID) || resourceID <= 0) {
          throw new Error('invalid component id format')
        }

        const found = this.findResource(resourceID)
        if (!found) {
          throw new Error('resource or client was released')
        }

        const { mux, client } = found

        const wrappedLookup: LookupMethod = async (serviceID, methodID) => {
          const invokeFn = await mux.lookupMethod(serviceID, methodID)
          if (!invokeFn) return null
          return async (dataSource, dataSink) => {
            _currentRpcClient = client
            try {
              await invokeFn(dataSource, dataSink)
            } finally {
              _currentRpcClient = undefined
            }
          }
        }

        const server = new Server(wrappedLookup)
        return server.rpcStreamHandler
      },
    )
  }

  // ResourceAttach handles a client attaching resources that
  // server-side RPC handlers can invoke via getAttachedRef(id).
  // Session-only Init/Ack, then Add/AddAck per resource.
  // After Init/Ack, mux_data carries yamux frames for all resources.
  async *ResourceAttach(
    request: MessageStream<ResourceAttachRequest>,
    _abortSignal?: AbortSignal,
  ): MessageStream<ResourceAttachResponse> {
    const packetRx = request[Symbol.asyncIterator]()

    // 1. Read Init packet.
    const initResult = await packetRx.next()
    if (initResult.done) {
      throw new Error('stream closed before init')
    }
    const initBody = initResult.value?.body
    if (initBody?.case !== 'init') {
      throw new Error('expected init packet')
    }
    const clientHandleId = initBody.value.clientHandleId ?? 0

    // 2. Find owning client.
    const client = this.clients.get(clientHandleId)
    if (!client || client.released) {
      yield {
        body: {
          case: 'ack' as const,
          value: { error: 'client not found' },
        },
      }
      return
    }

    // 3. Send session Ack.
    const outgoing = pushable<ResourceAttachResponse>({ objectMode: true })
    outgoing.push({
      body: {
        case: 'ack' as const,
        value: {},
      },
    })

    // 4. Create yamux StreamConn.
    // SERVER side is yamux client (outbound) -- opens sub-streams
    // to invoke the client's muxes via routed SRPC.
    const attachController = new AbortController()
    const conn = new StreamConn(undefined, {
      direction: 'outbound',
      yamuxParams: { enableKeepAlive: false },
    })
    const baseClient = conn.buildClient()

    // Track attached resource IDs for cleanup.
    const attachedIds: number[] = []

    // 5. onControl handles Add and Detach messages.
    const onControl = (req: ResourceAttachRequest) => {
      const body = req.body
      if (body?.case === 'add') {
        const attachId = body.value.attachId ?? 0
        const label = body.value.label ?? ''
        const resourceId = this.nextResourceID()

        // Create routed client for this resource.
        const resClient = createRoutedClient(baseClient, resourceId)

        client.attachedResources.set(resourceId, {
          label,
          client: resClient,
          signal: attachController.signal,
          controller: attachController,
        })
        attachedIds.push(resourceId)

        outgoing.push({
          body: {
            case: 'addAck' as const,
            value: { attachId, resourceId },
          },
        })
      } else if (body?.case === 'detach') {
        const resourceId = body.value.resourceId ?? 0
        client.attachedResources.delete(resourceId)
        const idx = attachedIds.indexOf(resourceId)
        if (idx >= 0) attachedIds.splice(idx, 1)

        outgoing.push({
          body: {
            case: 'detachAck' as const,
            value: { resourceId },
          },
        })
      }
    }

    // 6. Pipe mux_data between the bidi stream and yamux.
    // Incoming packets -> dispatch control or extract mux_data bytes.
    const incomingBytes = (async function* () {
      for (;;) {
        const result = await packetRx.next()
        if (result.done) break
        const body = result.value?.body
        if (body?.case === 'muxData') {
          yield body.value
        } else if (body?.case === 'add' || body?.case === 'detach') {
          onControl(result.value)
        }
      }
    })()

    // conn.source (yamux output) -> wrap as mux_data -> push to outgoing.
    const pipePromise = pipe(
      incomingBytes,
      conn,
      combineUint8ArrayListTransform(),
      async (source: AsyncIterable<Uint8Array>) => {
        for await (const chunk of source) {
          outgoing.push({
            body: {
              case: 'muxData' as const,
              value: chunk,
            },
          })
        }
        outgoing.end()
      },
    ).catch((err: Error) => {
      outgoing.end(err)
    })

    // 7. Yield outgoing packets and clean up.
    try {
      yield* outgoing
      await pipePromise
    } finally {
      attachController.abort()
      conn.close()
      for (const id of attachedIds) {
        client.attachedResources.delete(id)
      }
    }
  }

  // ResourceRefRelease handles client-initiated resource release.
  async ResourceRefRelease(
    request: ResourceRefReleaseRequest,
    _abortSignal?: AbortSignal,
  ): Promise<ResourceRefReleaseResponse> {
    const clientID = request.clientHandleId ?? 0
    const resourceID = request.resourceId ?? 0

    if (clientID === 0) {
      throw new Error('invalid client id')
    }

    const client = this.clients.get(clientID)
    if (!client || client.released) {
      throw new Error('resource not found')
    }

    const resource = client.resources.get(resourceID)
    if (!resource) {
      throw new Error('resource not found')
    }

    // Root resource (no releaseFn) is never deleted.
    if (!resource.releaseFn) {
      return {}
    }

    client.resources.delete(resourceID)
    resource.releaseFn()

    return {}
  }
}

// createRoutedClient wraps an SRPC client so all calls are prefixed with
// a resource ID for routing to the correct attached resource mux.
function createRoutedClient(
  inner: ReturnType<StreamConn['buildClient']>,
  resourceId: number,
): ReturnType<StreamConn['buildClient']> {
  const prefix = `${resourceId}/`
  return {
    request(service: string, method: string, data: Uint8Array, signal?: AbortSignal) {
      return inner.request(prefix + service, method, data, signal)
    },
    clientStreamingRequest(service: string, method: string, data: AsyncIterable<Uint8Array>, signal?: AbortSignal) {
      return inner.clientStreamingRequest(prefix + service, method, data, signal)
    },
    serverStreamingRequest(service: string, method: string, data: Uint8Array, signal?: AbortSignal) {
      return inner.serverStreamingRequest(prefix + service, method, data, signal)
    },
    bidirectionalStreamingRequest(service: string, method: string, data: AsyncIterable<Uint8Array>, signal?: AbortSignal) {
      return inner.bidirectionalStreamingRequest(prefix + service, method, data, signal)
    },
  } as ReturnType<StreamConn['buildClient']>
}

export { ResourceServer, getCurrentResourceClient }
