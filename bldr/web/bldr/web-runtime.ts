import {
  RpcStreamPacket,
  handleRpcStream,
  RpcStreamGetter,
  RpcStreamHandler,
  PacketStream,
  Client as RPCClient,
  Server,
  createHandler,
  createMux,
  openRpcStream,
  OpenStreamFunc,
  ChannelStream,
  castToError,
  ChannelStreamOpts,
  MessageStream,
  type StaticMux,
} from 'starpc'
import { pipe } from 'it-pipe'

import {
  WebRuntimeClientInit,
  CreateWebDocumentRequest,
  CreateWebDocumentResponse,
  FlushIndexCacheRequest,
  FlushIndexCacheResponse,
  RemoveWebDocumentRequest,
  RemoveWebDocumentResponse,
  WebRuntimeStatus,
  WebDocumentStatus,
  WebRuntimeClientType,
  WebRuntimeClientType_Enum,
} from '../runtime/runtime.pb.js'
import {
  WebRuntime as WebRuntimeService,
  WebRuntimeDefinition,
  WebRuntimeHostClient,
} from '../runtime/runtime_srpc.pb.js'
import {
  buildWebDocumentLockName,
  buildWebRuntimeClientLockName,
  buildWebWorkerLockName,
  ClientToWebRuntime,
  WebRuntimeToClient,
} from '../runtime/runtime.js'
import { ItState } from './it-state.js'

// WebRuntimeClientChannelStreamOpts are common opts for the WebRuntimeClient ChannelStream.
// Runtime/client invalidation already has explicit teardown paths. Leave
// watchdog timeouts disabled so quiet streams can stay idle without surfacing
// spurious ERR_STREAM_IDLE errors when browser timers are throttled.
export const WebRuntimeClientChannelStreamOpts: ChannelStreamOpts = {}

export function logWebRuntimeMessage(...args: unknown[]) {
  try {
    console.log(...args)
  } catch (err) {
    if (!isClosedStreamWriteError(err)) {
      throw err
    }
  }
}

function logWebRuntimeError(...args: unknown[]) {
  try {
    console.error(...args)
  } catch (err) {
    if (!isClosedStreamWriteError(err)) {
      throw err
    }
  }
}

export function isClosedStreamWriteError(err: unknown): boolean {
  return (
    typeof err === 'object' &&
    err !== null &&
    'code' in err &&
    (err.code === 'EPIPE' || err.code === 'ERR_STREAM_DESTROYED')
  )
}

// WebRuntimeClientInstance is an attached client instance.
class WebRuntimeClientChildStream implements PacketStream {
  public readonly source: PacketStream['source']
  public readonly sink: PacketStream['sink']
  private sourceClosed = false
  private sinkClosed = false
  private released = false

  constructor(
    private readonly inner: ChannelStream,
    private readonly release: () => void,
  ) {
    this.source = this.wrapSource(inner.source)
    this.sink = async (source) => {
      try {
        await inner.sink(source)
      } finally {
        this.sinkClosed = true
        this.releaseIfBothDirectionsClosed()
      }
    }
  }

  public close(err?: Error): void {
    this.releaseOnce()
    this.inner.close(err)
  }

  private async *wrapSource(
    source: PacketStream['source'],
  ): PacketStream['source'] {
    try {
      yield* source
    } finally {
      this.sourceClosed = true
      this.releaseIfBothDirectionsClosed()
    }
  }

  private releaseIfBothDirectionsClosed(): void {
    // Runtime-to-client streams are generation-owned until both duplex halves
    // end or the parent client closes; a single EOF can leave replies live.
    if (this.sourceClosed && this.sinkClosed) {
      this.releaseOnce()
    }
  }

  private releaseOnce(): void {
    if (this.released) {
      return
    }
    this.released = true
    this.release()
  }
}

type WebRuntimeClientInstanceState = 'active' | 'suspect' | 'closed'

class WebRuntimeClientInstance {
  // waitClosed is resolved when the instance is closed.
  public readonly waitClosed: Promise<void>
  // _resolveWaitClosed resolves waitClosed.
  private _resolveWaitClosed?: () => void

  // state tracks whether the client is usable, suspected by liveness evidence,
  // or confirmed closed.
  private state: WebRuntimeClientInstanceState = 'active'
  // abortController aborts the Web Lock request on close.
  private abortController?: AbortController
  // childStreams are the RPC streams opened through this client connection.
  // They must be closed when the parent client is invalidated or replaced,
  // otherwise Go-side document/view controllers stay stuck on orphaned streams
  // after the parent client generation is gone.
  private readonly childStreams = new Set<{ close: (err?: Error) => void }>()

  // isClosed checks if the instance is closed.
  public get isClosed(): boolean {
    return this.state === 'closed'
  }

  // clientId returns the stable logical id WebRuntime routes and keys locks by.
  public get clientId(): string {
    return this.host.getClientId(this.init)
  }

  constructor(
    private readonly host: WebRuntime,
    public readonly port: MessagePort,
    public readonly init: WebRuntimeClientInit,
  ) {
    this.waitClosed = new Promise<void>(
      (resolve) => (this._resolveWaitClosed = resolve),
    )
    port.onmessage = this.onClientMessage.bind(this)
    port.onmessageerror = () => {
      logWebRuntimeError(
        `WebRuntimeClientInstance: message error from client: ${this.init.clientUuid}`,
      )
      this.close()
    }
    port.start()

    // Ack that the runtime registered this client so the page-side
    // WebRuntimeClient.openClientChannel() can distinguish a live
    // connection from a dead MessagePort.
    const ack: WebRuntimeToClient = { connected: true }
    port.postMessage(ack)

    // Note: Web Lock watching is NOT started here to avoid a race condition.
    // The WebDocument must acquire its lock first, then send an armWebLock message.
    // See armWebLock() method.
  }

  // armWebLock starts watching the Web Lock for disconnect detection.
  // Called when the WebDocument sends an armWebLock message after acquiring its lock.
  private armWebLock() {
    const clientUuid = this.init.clientUuid
    if (
      !clientUuid ||
      this.init.disableWebLocks ||
      this.init.clientType !==
        WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT ||
      typeof navigator === 'undefined' ||
      !navigator.locks
    ) {
      return
    }

    // Already armed
    if (this.abortController) {
      return
    }

    this.abortController = new AbortController()
    const lockName = buildWebRuntimeClientLockName(
      this.init.clientType ?? WebRuntimeClientType.WebRuntimeClientType_UNKNOWN,
      clientUuid,
    )
    if (!lockName) {
      return
    }
    this.requestWebLock(lockName, this.abortController.signal, clientUuid)
  }

  private requestWebLock(
    lockName: string,
    signal: AbortSignal,
    clientUuid: string,
  ) {
    navigator.locks
      .request(lockName, { signal }, () => {
        if (!this.isClosed) {
          // Lock grant is only suspect evidence. Background or bfcache suspend
          // can release the page lock while the MessagePort and streams stay
          // alive, so deletion waits for a confirmed close path.
          this.state = 'suspect'
          logWebRuntimeMessage(
            `WebRuntime: client suspect via Web Lock: ${clientUuid}`,
          )
          this.requestWebLock(lockName, signal, clientUuid)
        }
        return Promise.resolve()
      })
      .catch((err) => {
        if (this.isClosed || isAbortError(err)) {
          return
        }
        logWebRuntimeError(
          `WebRuntime: client Web Lock watch failed: ${clientUuid}:`,
          err,
        )
      })
  }

  // openStream opens a RPC stream with the remote client.
  // note: the stream has message framing (via postMessage)
  // it is not necessary to use length prefixing for packets
  public async openStream(): Promise<PacketStream> {
    if (this.isClosed) {
      throw new Error('WebRuntimeClientInstance is closed')
    }

    const { port1: localPort, port2: remotePort } = new MessageChannel()
    // construct the message channel backed stream.
    const stream = new ChannelStream(
      this.host.webRuntimeId,
      localPort,
      WebRuntimeClientChannelStreamOpts,
    )
    const trackedStream = new WebRuntimeClientChildStream(stream, () => {
      this.childStreams.delete(trackedStream)
    })
    this.childStreams.add(trackedStream)
    this.postMessage({ openStream: true }, [remotePort])
    // Do not add timer timeouts to stream handshakes. Background tabs can
    // throttle timers, so the parent client invalidation owns cancellation.
    await Promise.race([stream.waitRemoteAck, this.waitClosed])
    if (this.isClosed) {
      trackedStream.close()
      throw new Error('WebRuntimeClientInstance is closed')
    }
    // wait for the stream to be fully opened
    await Promise.race([stream.waitRemoteOpen, this.waitClosed])
    if (this.isClosed) {
      trackedStream.close()
      throw new Error('WebRuntimeClientInstance is closed')
    }
    // return the stream
    return trackedStream
  }

  // close closes the client.
  public close() {
    this.closeInternal(true)
  }

  public closeInternal(emitStatus: boolean) {
    if (this.isClosed) {
      return
    }
    this.state = 'closed'

    // Abort the Web Lock request if active.
    if (this.abortController) {
      this.abortController.abort()
      this.abortController = undefined
    }

    const streamErr = new Error(
      `WebRuntimeClientInstance closed: ${this.init.clientUuid ?? this.clientId}`,
    )
    for (const stream of this.childStreams) {
      try {
        stream.close(streamErr)
      } catch {
        // ignored
      }
    }
    this.childStreams.clear()

    this._resolveWaitClosed!()
    try {
      this.port.close()
    } finally {
      const clientUuid = this.init.clientUuid ?? ''
      logWebRuntimeMessage(
        `WebRuntime: client connection removed: ${clientUuid}`,
      )
      this.host.removeConnection(this.clientId, this, emitStatus)
    }
  }

  // postMessage writes a message via the client MessagePort.
  public postMessage(msg: WebRuntimeToClient, xfer?: MessagePort[]) {
    try {
      if (xfer && xfer.length) {
        this.port.postMessage(msg, xfer)
      } else {
        this.port.postMessage(msg)
      }
    } catch (err) {
      logWebRuntimeError(
        `WebRuntime: client connection error: ${this.init.clientUuid} => ${castToError(err).toString()}`,
      )
      this.close()
    }
  }

  // onClientMessage handles an incoming message.
  private async onClientMessage(ev: MessageEvent) {
    const msg: ClientToWebRuntime = ev.data
    if (typeof msg !== 'object') {
      return
    }
    const ports = ev.ports
    if (msg.openStream && ports.length) {
      await this.openWebRuntimeClientInstanceStream(ports[0])
    }
    if (msg.armWebLock) {
      this.armWebLock()
    }
    if (msg.close) {
      logWebRuntimeMessage(
        `WebRuntimeClientInstance: remote client closed session: ${this.init.clientUuid}`,
      )
      this.close()
    }
  }

  // openWebRuntimeClientInstanceStream opens a stream with the Go runtime on behalf of a client.
  private async openWebRuntimeClientInstanceStream(port: MessagePort) {
    const channelStream = new ChannelStream(this.host.webRuntimeId, port, {
      ...WebRuntimeClientChannelStreamOpts,
      remoteOpen: true,
    })
    this.childStreams.add(channelStream)
    try {
      let streamPromise: Promise<PacketStream>
      switch (this.init.clientType) {
        case WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT:
          streamPromise = this.host.openWebDocumentHostStream(this.clientId)
          break
        case WebRuntimeClientType.WebRuntimeClientType_SERVICE_WORKER:
          streamPromise = this.host.openServiceWorkerHostStream(this.clientId)
          break
        case WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER:
          streamPromise = this.host.openWebWorkerHostStream(this.clientId)
          break
        default:
          throw new Error('unknown client type: ' + this.init.clientType)
      }

      const stream = await streamPromise
      pipe(channelStream, stream, channelStream)
        .catch((err) => channelStream.close(err))
        .then(() => channelStream.close())
        .finally(() => this.childStreams.delete(channelStream))
    } catch (errAny) {
      this.childStreams.delete(channelStream)
      const err = castToError(errAny, 'open stream failed')
      channelStream.close(err)
    }
  }
}

// WebRuntimeClientWaiter waits for a client registration event.
interface WebRuntimeClientWaiter {
  resolve: (client: WebRuntimeClientInstance) => void
  reject: (err: Error) => void
  abortController?: AbortController
}

// WebRuntimeImpl implements the WebRuntime RPC API.
class WebRuntimeImpl implements WebRuntimeService {
  constructor(private readonly host: WebRuntime) {}

  // WatchWebRuntimeStatus returns an initial snapshot of WebRuntimes followed by updates.
  public WatchWebRuntimeStatus(): MessageStream<WebRuntimeStatus> {
    return this.host.statusStream.getIterable()
  }

  // CreateWebDocument requests to create a new WebDocument.
  public async CreateWebDocument(
    request: CreateWebDocumentRequest,
  ): Promise<CreateWebDocumentResponse> {
    const createCb = this.host.createDocCb
    if (!createCb) {
      return { created: false }
    }
    return createCb(request)
  }

  // RemoveWebDocument requests to remove a WebDocument.
  public async RemoveWebDocument(
    request: RemoveWebDocumentRequest,
  ): Promise<RemoveWebDocumentResponse> {
    const removeCb = this.host.removeDocCb
    if (!removeCb) {
      return { removed: false }
    }
    return removeCb(request)
  }

  // WebDocumentRpc opens a stream for a RPC call to a WebDocument.
  public WebDocumentRpc(
    request: MessageStream<RpcStreamPacket>,
  ): MessageStream<RpcStreamPacket> {
    return handleRpcStream(
      request[Symbol.asyncIterator](),
      this.buildWebDocumentRpcGetter(),
    )
  }

  // WebWorkerRpc opens a stream for a RPC call to a WebWorker.
  public WebWorkerRpc(
    request: MessageStream<RpcStreamPacket>,
  ): MessageStream<RpcStreamPacket> {
    return handleRpcStream(
      request[Symbol.asyncIterator](),
      this.buildWebWorkerRpcGetter(),
    )
  }

  // FlushIndexCache refreshes the cached browser index document.
  public async FlushIndexCache(
    _request: FlushIndexCacheRequest,
  ): Promise<FlushIndexCacheResponse> {
    await this.host.flushIndexCache()
    return {}
  }

  // buildWebDocumentRpcGetter builds the RpcGetter for a WebDocument.
  private buildWebDocumentRpcGetter(): RpcStreamGetter {
    return (webDocumentId: string) => {
      return this.getClientRpcHandler(
        webDocumentId,
        buildWebDocumentLockName(webDocumentId),
      )
    }
  }

  // buildWebWorkerRpcGetter builds the RpcGetter for a WebWorker.
  private buildWebWorkerRpcGetter(): RpcStreamGetter {
    return (webWorkerId: string) => {
      return this.getClientRpcHandler(
        webWorkerId,
        buildWebWorkerLockName(webWorkerId),
      )
    }
  }

  // getClientRpcHandler looks up the rpc stream handler for the given client ID.
  // Waits for the client to register if not yet connected.
  private async getClientRpcHandler(
    clientId: string,
    webLockName?: string,
  ): Promise<RpcStreamHandler | null> {
    const client = await this.host.waitForClient(clientId, webLockName)

    const stream = await client.openStream()
    return (rpcDataStream: PacketStream) => {
      return pipe(rpcDataStream, stream, rpcDataStream)
    }
  }
}

// CreateWebDocumentFunc is a function to create a WebDocument.
export type CreateWebDocumentFunc = (
  req: CreateWebDocumentRequest,
) => Promise<CreateWebDocumentResponse>

// RemoveWebDocumentFunc is a function to remove a WebDocument.
export type RemoveWebDocumentFunc = (
  req: RemoveWebDocumentRequest,
) => Promise<RemoveWebDocumentResponse>

interface WebRuntimeServerExtension {
  register(mux: StaticMux): void
}

// WebRuntime implements the WebDocumentHost with a SharedWorker.
export class WebRuntime {
  // webRuntimeId is the identifier of the WebRuntime.
  public readonly webRuntimeId: string
  // webRuntime manages the incoming RPC calls to the WebRuntime.
  private webRuntime: WebRuntimeImpl
  // webRuntimeMux routes incoming process-lifetime WebRuntime RPC services.
  private webRuntimeMux: StaticMux
  // webRuntimeServer is the server for incoming RPC connections to WebRuntime.
  private webRuntimeServer: Server

  // runtimeClient is the RPC client for the WebRuntimeHost.
  private runtimeClient: RPCClient
  // runtimeHost is the WebRuntimeHost.
  public readonly runtimeHost: WebRuntimeHostClient

  // _webStatusUpdates is a stream of web status updates.
  public readonly statusStream: ItState<WebRuntimeStatus>

  // clients contains the list of attached WebRuntime clients.
  // keyed by client ID
  private clients: Record<string, WebRuntimeClientInstance> = {}
  // clientWaiters contains promises waiting for a client to register.
  // keyed by client ID
  private clientWaiters: Record<string, WebRuntimeClientWaiter[]> = {}
  // webDocuments contains the list of attached WebDocuments.
  // keyed by web document ID
  private webDocuments: Record<string, WebDocumentStatus> = {}

  // closed indicates the instance is closed.
  private closed?: true

  // isClosed checks if the instance is closed.
  public get isClosed(): boolean {
    return this.closed ?? false
  }

  constructor(
    webRuntimeId: string,
    openStreamFn: OpenStreamFunc,
    public readonly createDocCb:
      | ((req: CreateWebDocumentRequest) => Promise<CreateWebDocumentResponse>)
      | null,
    public readonly removeDocCb:
      | ((req: RemoveWebDocumentRequest) => Promise<RemoveWebDocumentResponse>)
      | null,
  ) {
    this.webRuntimeId = webRuntimeId

    // Setup the WebRuntime service implementation.
    this.webRuntime = new WebRuntimeImpl(this)
    this.webRuntimeMux = createMux()
    this.webRuntimeMux.register(
      createHandler(WebRuntimeDefinition, this.webRuntime),
    )
    this.webRuntimeServer = new Server(this.webRuntimeMux.lookupMethod)

    // Setup the status stream.
    this.statusStream = new ItState<WebRuntimeStatus>(
      this.buildWebRuntimeStatusSnapshot.bind(this),
    )

    // Setup the runtime client.
    this.runtimeClient = new RPCClient(openStreamFn)
    this.runtimeHost = new WebRuntimeHostClient(this.runtimeClient)
  }

  // registerServerExtension wires an additional service into this runtime server.
  public registerServerExtension(extension: WebRuntimeServerExtension): void {
    extension.register(this.webRuntimeMux)
  }

  // getWebRuntimeServer returns the srpc Server for the web runtime service.
  public getWebRuntimeServer(): Server {
    return this.webRuntimeServer
  }

  // openWebDocumentHostStream opens a stream to the WebDocumentHost service.
  public openWebDocumentHostStream(
    webDocumentUuid: string,
  ): Promise<PacketStream> {
    return openRpcStream(
      webDocumentUuid,
      this.runtimeHost.WebDocumentRpc.bind(this.runtimeHost),
    )
  }

  // openWebWorkerHostStream opens a stream to the WebWorkerHost service.
  public openWebWorkerHostStream(webWorkerUuid: string): Promise<PacketStream> {
    return openRpcStream(
      webWorkerUuid,
      this.runtimeHost.WebWorkerRpc.bind(this.runtimeHost),
    )
  }

  // openServiceWorkerHostStream opens a stream to the ServiceWorkerHost service.
  public openServiceWorkerHostStream(
    webDocumentUuid: string,
  ): Promise<PacketStream> {
    return openRpcStream(
      webDocumentUuid,
      this.runtimeHost.ServiceWorkerRpc.bind(this.runtimeHost),
    )
  }
  // broadcastStartupMark forwards compact runtime accounting to attached documents.
  public broadcastStartupMark(
    label: string,
    detail: Record<string, unknown>,
  ): void {
    for (const client of Object.values(this.clients)) {
      if (
        client.init.clientType !==
        WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT
      ) {
        continue
      }
      client.postMessage({ startupMark: { label, detail } })
    }
  }

  // broadcastDurableMutation notifies documents that a user-authored world
  // mutation just fenced durable.
  public broadcastDurableMutation(): void {
    for (const client of Object.values(this.clients)) {
      if (
        client.init.clientType !==
        WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT
      ) {
        continue
      }
      client.postMessage({ durableMutation: true })
    }
  }

  // broadcastPluginManifestRoot forwards selected root authority to documents.
  public broadcastPluginManifestRoot(pluginId: string, rootHash: string): void {
    for (const client of Object.values(this.clients)) {
      if (
        client.init.clientType !==
        WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT
      ) {
        continue
      }
      client.postMessage({ pluginManifestRoot: { pluginId, rootHash } })
    }
  }

  // flushIndexCache forces the service worker to refresh its browser index cache.
  public async flushIndexCache(): Promise<void> {
    const response = await fetch(
      new URL('/b/__index.html', globalThis.location.href).toString(),
      { cache: 'reload' },
    )
    if (!response.ok) {
      throw new Error(
        `browser index cache refresh failed: status=${response.status}`,
      )
    }
  }

  // lookupClient looks up an ongoing WebRuntime client connection.
  public lookupClient(webRuntimeId: string): WebRuntimeClientInstance | null {
    return this.clients[webRuntimeId] ?? null
  }

  // getClientId returns the stable logical id for a runtime client init.
  public getClientId(init: WebRuntimeClientInit): string {
    return init.logicalClientId || init.clientUuid || ''
  }

  // waitForClient waits for a client with the given ID to register.
  // Returns immediately if the client is already registered.
  public waitForClient(
    clientId: string,
    webLockName?: string,
  ): Promise<WebRuntimeClientInstance> {
    const existing = this.clients[clientId]
    if (existing) {
      return Promise.resolve(existing)
    }

    return new Promise<WebRuntimeClientInstance>((resolve, reject) => {
      const waiter: WebRuntimeClientWaiter = { resolve, reject }
      const waiters = this.clientWaiters[clientId] ?? []
      waiters.push(waiter)
      this.clientWaiters[clientId] = waiters
      this.watchClientWaiterLock(clientId, webLockName, waiter)
    })
  }

  // watchClientWaiterLock rejects the waiter when the matching client lock is released.
  private watchClientWaiterLock(
    clientId: string,
    webLockName: string | undefined,
    waiter: WebRuntimeClientWaiter,
  ) {
    if (!webLockName || typeof navigator === 'undefined' || !navigator.locks) {
      return
    }

    const abortController = new AbortController()
    waiter.abortController = abortController
    navigator.locks
      .request(webLockName, { signal: abortController.signal }, () => {
        if (!this.removeClientWaiter(clientId, waiter)) {
          return Promise.resolve()
        }

        const err = new Error(
          `WebRuntime: ${this.webRuntimeId}: client ${clientId} disconnected before registering`,
        )
        waiter.reject(err)
        return Promise.resolve()
      })
      .catch((err) => {
        if (isAbortError(err)) {
          return
        }
        logWebRuntimeError(
          `WebRuntime: ${this.webRuntimeId}: client waiter lock failed for ${clientId}:`,
          err,
        )
      })
  }

  // removeClientWaiter removes a pending client waiter.
  private removeClientWaiter(
    clientId: string,
    waiter: WebRuntimeClientWaiter,
  ): boolean {
    const waiters = this.clientWaiters[clientId]
    if (!waiters) {
      return false
    }

    const idx = waiters.indexOf(waiter)
    if (idx === -1) {
      return false
    }

    waiters.splice(idx, 1)
    waiter.abortController?.abort()
    waiter.abortController = undefined
    if (!waiters.length) {
      delete this.clientWaiters[clientId]
    }
    return true
  }

  // rejectClientWaiters rejects all pending waiters for a client.
  private rejectClientWaiters(clientId: string, err: Error) {
    const waiters = this.clientWaiters[clientId]
    if (!waiters) {
      return
    }

    delete this.clientWaiters[clientId]
    for (const waiter of waiters) {
      waiter.abortController?.abort()
      waiter.abortController = undefined
      waiter.reject(err)
    }
  }

  // handleClient handles an incoming client connection MessagePort.
  // msg should contain a WebRuntimeClientInit message
  public handleClient(msg: WebRuntimeClientInit, port: MessagePort) {
    if (this.closed) {
      throw new Error('web runtime is closed')
    }

    const clientUuid = msg.clientUuid
    if (!clientUuid) {
      throw new Error('connect init message: must contain client uuid')
    }
    const clientId = this.getClientId(msg)
    if (!clientId) {
      throw new Error('connect init message: must contain client routing id')
    }

    const existing = this.lookupClient(clientId)
    if (existing) {
      // Same logical-id reconnects replace only the channel generation. Emitting
      // deleted here tears down Go RemoteWebDocument state even though the
      // document identity is still alive.
      existing.closeInternal(false)
    }

    const clientTypeStr =
      WebRuntimeClientType_Enum.findNumber(msg.clientType ?? 0)?.name ??
      'unknown'
    logWebRuntimeMessage(
      `WebRuntime: ${this.webRuntimeId}: registered client: ${msg.clientUuid} => ${clientId} type ${clientTypeStr}`,
    )
    const client = new WebRuntimeClientInstance(this, port, msg)
    this.clients[clientId] = client

    // Notify any waiters for this client.
    const waiters = this.clientWaiters[clientId]
    if (waiters) {
      delete this.clientWaiters[clientId]
      for (const waiter of waiters) {
        waiter.abortController?.abort()
        waiter.abortController = undefined
        waiter.resolve(client)
      }
    }

    if (
      !existing &&
      msg.clientType === WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT
    ) {
      const status: WebDocumentStatus = {
        id: clientId,
        deleted: false,
        permanent: false,
      }
      this.webDocuments[clientId] = status
      this.statusStream.pushChangeEvent({
        snapshot: false,
        closed: false,
        webDocuments: [status],
      })
    }
  }

  // removeConnection removes a connection by client id.
  public removeConnection(
    clientId: string,
    expectedClient?: WebRuntimeClientInstance,
    emitStatus = true,
  ) {
    const client = this.clients[clientId]
    if (!client || (expectedClient && client !== expectedClient)) {
      return
    }
    delete this.clients[clientId]

    const clientType = client.init.clientType
    const clientTypeStr =
      WebRuntimeClientType_Enum.findNumber(clientType ?? 0)?.name ?? 'unknown'
    logWebRuntimeMessage(
      `WebRuntime: ${this.webRuntimeId}: removed client: ${clientId} type ${clientTypeStr}`,
    )
    if (
      emitStatus &&
      !this.closed &&
      clientType === WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT &&
      this.webDocuments[clientId]
    ) {
      delete this.webDocuments[clientId]
      this.statusStream.pushChangeEvent({
        snapshot: false,
        closed: false,
        webDocuments: [
          {
            id: clientId,
            deleted: true,
            permanent: false,
          },
        ],
      })
    }
  }

  // invalidateClient closes the active client and rejects current waiters.
  public invalidateClient(clientId: string, err: Error) {
    const client = this.clients[clientId]
    if (client) {
      client.close()
    }
    this.rejectClientWaiters(clientId, err)
  }

  // buildWebRuntimeStatusSnapshot builds a snapshot of the status.
  // if allWorkers is set, includes web views from other active workers.
  // prevents duplicate web view entries
  public async buildWebRuntimeStatusSnapshot(): Promise<WebRuntimeStatus> {
    if (this.closed) {
      return { snapshot: true, closed: true, webDocuments: [] }
    }

    const webDocuments: WebDocumentStatus[] = []
    for (const webDocumentId of Object.keys(this.webDocuments)) {
      const webDocument = this.webDocuments[webDocumentId]
      if (webDocumentId && webDocument) {
        webDocuments.push(webDocument)
      }
    }
    webDocuments.sort((a, b) => ((a.id ?? '') < (b.id ?? '') ? -1 : 1))
    return {
      snapshot: true,
      closed: false,
      webDocuments,
    }
  }

  public close() {
    if (this.closed) {
      return
    }
    this.closed = true

    this.webDocuments = {}
    for (const clientId of Object.keys(this.clientWaiters)) {
      this.rejectClientWaiters(
        clientId,
        new Error(`WebRuntime: ${this.webRuntimeId}: closed`),
      )
    }
    for (const clientID in this.clients) {
      const client = this.clients[clientID]
      client.close()
      delete this.clients[clientID]
    }

    this.statusStream.pushChangeEvent({
      snapshot: true,
      closed: true,
      webDocuments: [],
    })
  }
}

function isAbortError(err: unknown): boolean {
  return (
    typeof err === 'object' &&
    err !== null &&
    'name' in err &&
    (err as { name?: string }).name === 'AbortError'
  )
}
