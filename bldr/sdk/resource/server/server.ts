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
import type { Pushable } from 'it-pushable'
import { pipe } from 'it-pipe'
import type {
  ResourceAttachRequest,
  ResourceAttachResponse,
  ResourceClientRequest,
  ResourceClientResponse,
} from '../resource.pb.js'
import { ResourceServiceDefinition } from '../resource_srpc.pb.js'
import type { ResourceServiceHandler } from '../resource_srpc.pb.js'
import { RemoteResourceClient } from './tracked-client.js'

import { withResourceCall } from './context.js'
async function nextResourceClientControl(
  packetRx: AsyncIterator<ResourceClientRequest>,
  signal?: AbortSignal,
): Promise<IteratorResult<ResourceClientRequest>> {
  if (!signal) return await packetRx.next()
  if (signal.aborted) return { value: undefined as never, done: true }
  return await new Promise<IteratorResult<ResourceClientRequest>>(
    (resolve, reject) => {
      const cleanup = () => signal.removeEventListener('abort', onAbort)
      const onAbort = () => {
        cleanup()
        resolve({ value: undefined as never, done: true })
      }
      signal.addEventListener('abort', onAbort, { once: true })
      if (signal.aborted) {
        onAbort()
        return
      }
      void packetRx.next().then(
        (result) => {
          cleanup()
          resolve(result)
        },
        (error: unknown) => {
          cleanup()
          reject(error)
        },
      )
    },
  )
}

// ResourceServer manages a tree of resources accessible over
// SRPC. Clients connect via ResourceClient, receive a root
// resource ID, and make RPCs to individual resources via
// ResourceRpc.
class ResourceServer implements ResourceServiceHandler {
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
  // ResourceClient is a bidirectional control stream. The receive loop owns
  // controls while the returned pushable owns response ordering.
  ResourceClient(
    request: MessageStream<ResourceClientRequest>,
    abortSignal?: AbortSignal,
  ): MessageStream<ResourceClientResponse> {
    const outgoing = pushable<ResourceClientResponse>({ objectMode: true })
    const packetRx = request[Symbol.asyncIterator]()
    const run = async () => {
      const first = await packetRx.next()
      if (first.done || first.value.body?.case !== 'init') {
        throw new Error('expected ResourceClient init')
      }
      const clientID = ++this.clientHandleIDCtr
      const client = new RemoteResourceClient(
        () => this.nextResourceID(),
        clientID,
        abortSignal,
      )
      this.clients.set(clientID, client)
      const rootResourceID = client.addResource(this.rootResourceMux)
      client.setRetainedRootResourceID(rootResourceID)
      outgoing.push({
        body: {
          case: 'init' as const,
          value: { clientHandleId: clientID, rootResourceId: rootResourceID },
        },
      })
      const transmitTask = (async () => {
        for (;;) {
          await client.waitForNotify(abortSignal)
          const messages = client.drainQueue()
          for (const message of messages) outgoing.push(message)
          if (client.released && messages.length === 0) return
        }
      })()
      let lastControlID = 0
      try {
        while (!client.released) {
          const next = await nextResourceClientControl(packetRx, abortSignal)
          if (next.done) break
          const controlID = next.value.controlId ?? 0
          if (controlID === 0 || controlID !== lastControlID + 1) {
            throw new Error(
              `unexpected ResourceClient control ID ${controlID} after ${lastControlID}`,
            )
          }
          const body = next.value.body
          if (body?.case === 'adopt') {
            if (
              client.adoptResource(body.value.resourceId ?? 0) === 'invalid'
            ) {
              throw new Error('invalid resource id')
            }
          } else if (body?.case === 'release') {
            const resourceID = body.value.resourceId ?? 0
            if (!client.releaseResource(resourceID, false)) {
              throw new Error('invalid resource id')
            }
          } else {
            throw new Error('invalid ResourceClient control')
          }
          lastControlID = controlID
          client.pushMessage({
            body: {
              case: 'controlAck' as const,
              value: { controlId: controlID },
            },
          })
        }
      } finally {
        this.clients.delete(clientID)
        client.releaseAll()
        await transmitTask
        outgoing.end()
      }
    }
    void run().catch((error) => outgoing.end(error))
    return outgoing
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
        if (!found) throw new Error('resource or client was released')
        const { mux, client } = found
        const wrappedLookup: LookupMethod = async (serviceID, methodID) => {
          const invokeFn = await mux.lookupMethod(serviceID, methodID)
          if (!invokeFn) return null
          return async (dataSource, dataSink, callContext) => {
            const resourceContext = withResourceCall(callContext, {
              client,
              parentResourceId: resourceID,
              serviceId: serviceID,
              methodId: methodID,
            })
            await invokeFn(dataSource, dataSink, resourceContext)
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
  ResourceAttach(
    request: MessageStream<ResourceAttachRequest>,
    abortSignal?: AbortSignal,
  ): MessageStream<ResourceAttachResponse> {
    let cleanup: (() => void) | undefined
    const outgoing = pushable<ResourceAttachResponse>({
      objectMode: true,
      onEnd: () => {
        cleanup?.()
      },
    })

    this.runResourceAttach(request, outgoing, abortSignal, (cleanupFn) => {
      cleanup = cleanupFn
    })
      .catch((err: Error) => {
        outgoing.end(err)
      })
      .finally(() => {
        outgoing.end()
      })

    return outgoing
  }

  // runResourceAttach owns the ResourceAttach session protocol while
  // ResourceAttach itself returns a plain async queue. The browser QuickJS
  // backend has historically crashed when this long-lived control/mux stream
  // is represented as a class async generator resumed by unrelated RPC work.
  private async runResourceAttach(
    request: MessageStream<ResourceAttachRequest>,
    outgoing: Pushable<ResourceAttachResponse>,
    abortSignal: AbortSignal | undefined,
    setCleanup: (cleanup: () => void) => void,
  ): Promise<void> {
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
      outgoing.push({
        body: {
          case: 'ack' as const,
          value: { error: 'client not found' },
        },
      })
      return
    }

    // 3. Send session Ack.
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

        // Per-resource controller linked to the session controller.
        const resController = new AbortController()
        const resClient = createRoutedClient(
          baseClient,
          resourceId,
          resController.signal,
        )
        attachController.signal.addEventListener(
          'abort',
          () => resController.abort(),
          { once: true },
        )

        client.attachedResources.set(resourceId, {
          label,
          client: resClient,
          signal: resController.signal,
          controller: resController,
          release: () => {
            outgoing.push({
              body: {
                case: 'detachAck' as const,
                value: { resourceId },
              },
            })
          },
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
        const existing = client.attachedResources.has(resourceId)
        client.releaseResource(resourceId, false)
        const idx = attachedIds.indexOf(resourceId)
        if (idx >= 0) attachedIds.splice(idx, 1)
        if (!existing) {
          outgoing.push({
            body: {
              case: 'detachAck' as const,
              value: { resourceId },
            },
          })
        }
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

    // 7. Keep the session task alive until the transport ends, then clean up.
    let cleaned = false
    const cleanup = () => {
      if (cleaned) return
      cleaned = true
      attachController.abort()
      conn.close()
      for (const id of attachedIds) {
        client.releaseResource(id, false)
      }
    }
    setCleanup(cleanup)
    abortSignal?.addEventListener('abort', cleanup, { once: true })

    try {
      await pipePromise
    } finally {
      abortSignal?.removeEventListener('abort', cleanup)
      cleanup()
    }
  }
}

// createRoutedClient wraps an SRPC client so all calls are prefixed with
// a resource ID for routing to the correct attached resource mux.
function createRoutedClient(
  inner: ReturnType<StreamConn['buildClient']>,
  resourceId: number,
  resourceSignal: AbortSignal,
): ReturnType<StreamConn['buildClient']> {
  const prefix = `${resourceId}/`
  const callSignal = (signal?: AbortSignal) =>
    signal ? AbortSignal.any([resourceSignal, signal]) : resourceSignal
  return {
    request(
      service: string,
      method: string,
      data: Uint8Array,
      signal?: AbortSignal,
    ) {
      return inner.request(prefix + service, method, data, callSignal(signal))
    },
    clientStreamingRequest(
      service: string,
      method: string,
      data: AsyncIterable<Uint8Array>,
      signal?: AbortSignal,
    ) {
      return inner.clientStreamingRequest(
        prefix + service,
        method,
        data,
        callSignal(signal),
      )
    },
    serverStreamingRequest(
      service: string,
      method: string,
      data: Uint8Array,
      signal?: AbortSignal,
    ) {
      return inner.serverStreamingRequest(
        prefix + service,
        method,
        data,
        callSignal(signal),
      )
    },
    bidirectionalStreamingRequest(
      service: string,
      method: string,
      data: AsyncIterable<Uint8Array>,
      signal?: AbortSignal,
    ) {
      return inner.bidirectionalStreamingRequest(
        prefix + service,
        method,
        data,
        callSignal(signal),
      )
    },
  } as ReturnType<StreamConn['buildClient']>
}

export { ResourceServer }
