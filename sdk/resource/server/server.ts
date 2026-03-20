import {
  createMux,
  createHandler,
  Server,
  handleRpcStream,
} from 'starpc'
import type { Mux, LookupMethod, MessageStream } from 'starpc'
import type { RpcStreamPacket } from 'starpc'
import type {
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

export { ResourceServer, getCurrentResourceClient }
