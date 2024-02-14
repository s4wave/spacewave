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
} from 'starpc'
import { pipe } from 'it-pipe'
import { Duplex, Source } from 'it-stream-types'

import {
  WebRuntimeClientInit,
  WebRuntime as WebRuntimeService,
  WebRuntimeDefinition,
  CreateWebDocumentRequest,
  CreateWebDocumentResponse,
  RemoveWebDocumentRequest,
  RemoveWebDocumentResponse,
  WebRuntimeStatus,
  WebDocumentStatus,
  WebRuntimeClientType,
  webRuntimeClientTypeToJSON,
  WebRuntimeHostClientImpl,
} from '../runtime/runtime.pb.js'
import { ClientToWebRuntime, WebRuntimeToClient } from '../runtime/runtime.js'
import { ItState } from './it-state.js'
import { timeoutPromise } from './timeout.js'

// WebRuntimeClientChannelStreamOpts are common opts for the WebRuntimeClient ChannelStream.
export const WebRuntimeClientChannelStreamOpts: ChannelStreamOpts = {
  keepAliveMs: 1000,
  idleTimeoutMs: 2500,
} as const

// WebRuntimeClientInstance is an attached client instance.
class WebRuntimeClientInstance {
  constructor(
    private readonly host: WebRuntime,
    public readonly port: MessagePort,
    public readonly init: WebRuntimeClientInit,
  ) {
    port.onmessage = this.onClientMessage.bind(this)
    port.start()
  }

  // openStream opens a RPC stream with the remote client.
  //
  // times out if the client does not ack within 3 seconds.
  //
  // note: the stream has message framing (via postMessage)
  // it is not necessary to use length prefixing for packets
  public async openStream(): Promise<Duplex<Source<Uint8Array>>> {
    const channel = new MessageChannel()
    const localPort = channel.port1
    const remotePort = channel.port2
    // construct the message channel backed stream.
    const stream = new ChannelStream(
      this.host.webRuntimeId,
      localPort,
      WebRuntimeClientChannelStreamOpts,
    )
    this.postMessage({ openStream: true }, [remotePort])
    // wait for ack or timeout
    await Promise.race([stream.waitRemoteAck, timeoutPromise(1000)])
    if (!stream.isAcked) {
      stream.close()
      throw new Error('timed out waiting for ack')
    }
    // wait for the stream to be fully opened
    await stream.waitRemoteOpen
    // return the stream
    return stream
  }

  // close closes the client.
  public close() {
    try {
      this.port.close()
    } finally {
      console.log(
        `WebRuntime: client connection removed: ${this.init.clientUuid}`,
      )
      this.host.removeConnection(this.init.clientUuid, this.init.clientType)
    }
  }

  // postMessage writes a message via the client MessagePort.
  private postMessage(msg: Partial<WebRuntimeToClient>, xfer?: MessagePort[]) {
    try {
      if (xfer && xfer.length) {
        this.port.postMessage(msg, xfer)
      } else {
        this.port.postMessage(msg)
      }
    } catch (err) {
      // error: indicates port is closed.
      console.error('client closed with error', err)
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
    if (msg.close) {
      console.log(
        `WebRuntimeClientInstance: remote client closed session: ${this.init.clientUuid}`,
      )
      this.close()
    }
  }

  // openWebRuntimeClientInstanceStream opens a stream with the Go runtime on behalf of a client.
  private async openWebRuntimeClientInstanceStream(port: MessagePort) {
    const channelStream = new ChannelStream(
      this.host.webRuntimeId,
      port,
      {...WebRuntimeClientChannelStreamOpts, remoteOpen: true}
    )
    try {
      let streamPromise: Promise<PacketStream>
      switch (this.init.clientType) {
        case WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT:
          streamPromise = this.host.openWebDocumentHostStream(
            this.init.clientUuid,
          )
          break
        case WebRuntimeClientType.WebRuntimeClientType_SERVICE_WORKER:
          streamPromise = this.host.openServiceWorkerHostStream(
            this.init.clientUuid,
          )
          break
        default:
          throw new Error('unknown client type: ' + this.init.clientType)
      }

      const stream = await streamPromise
      pipe(channelStream, stream, channelStream)
        .catch((err) => channelStream.close(err))
        .then(() => channelStream.close())
    } catch (errAny) {
      const err = castToError(errAny, 'open stream failed')
      channelStream.close(err)
    }
  }
}

// WebRuntimeImpl implements the WebRuntime RPC API.
class WebRuntimeImpl implements WebRuntimeService {
  constructor(private readonly host: WebRuntime) {}

  // WatchWebRuntimeStatus returns an initial snapshot of WebRuntimes followed by updates.
  public WatchWebRuntimeStatus(): AsyncIterable<WebRuntimeStatus> {
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
    request: AsyncIterable<RpcStreamPacket>,
  ): AsyncIterable<RpcStreamPacket> {
    return handleRpcStream(
      request[Symbol.asyncIterator](),
      this.buildWebDocumentRpcGetter(),
    )
  }

  // buildWebDocumentRpcGetter builds the RpcGetter for a WebDocument.
  private buildWebDocumentRpcGetter(): RpcStreamGetter {
    return (webDocumentId: string) => {
      return this.getClientRpcHandler(webDocumentId)
    }
  }

  // getClientRpcHandler looks up the rpc stream handler for the given client ID.
  private async getClientRpcHandler(
    clientId: string,
  ): Promise<RpcStreamHandler | null> {
    const client = this.host.lookupClient(clientId)
    if (!client) {
      throw new Error(`unknown client: ${clientId}`)
    }

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

// WebRuntime implements the WebDocumentHost with a SharedWorker.
export class WebRuntime {
  // webRuntimeId is the identifier of the WebRuntime.
  public readonly webRuntimeId: string
  // webRuntime manages the incoming RPC calls to the WebRuntime.
  private webRuntime: WebRuntimeImpl
  // webRuntimeServer is the server for incoming RPC connections to WebRuntime.
  private webRuntimeServer: Server

  // runtimeClient is the RPC client for the WebRuntimeHost.
  private runtimeClient: RPCClient
  // runtimeHost is the WebRuntimeHost.
  public readonly runtimeHost: WebRuntimeHostClientImpl

  // _webStatusUpdates is a stream of web status updates.
  public readonly statusStream: ItState<WebRuntimeStatus>

  // clients contains the list of attached WebRuntime clients.
  // keyed by client ID
  private clients: Record<string, WebRuntimeClientInstance> = {}
  // webDocuments contains the list of attached WebDocuments.
  // keyed by web document ID
  private webDocuments: Record<string, WebDocumentStatus> = {}

  constructor(
    webRuntimeId: string,
    openStreamFn: OpenStreamFunc,
    public readonly createDocCb: CreateWebDocumentFunc | null,
    public readonly removeDocCb: RemoveWebDocumentFunc | null,
  ) {
    this.webRuntimeId = webRuntimeId

    // Setup the WebRuntime service implementation.
    this.webRuntime = new WebRuntimeImpl(this)
    const runtimeWorkerHostMux = createMux()
    runtimeWorkerHostMux.register(
      createHandler(WebRuntimeDefinition, this.webRuntime),
    )
    this.webRuntimeServer = new Server(runtimeWorkerHostMux.lookupMethodFunc)

    // Setup the status stream.
    this.statusStream = new ItState<WebRuntimeStatus>(
      this.buildWebRuntimeStatusSnapshot.bind(this),
    )

    // Setup the runtime client.
    this.runtimeClient = new RPCClient(openStreamFn)
    this.runtimeHost = new WebRuntimeHostClientImpl(this.runtimeClient)
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

  // openServiceWorkerHostStream opens a stream to the ServiceWorkerHost service.
  public openServiceWorkerHostStream(
    webDocumentUuid: string,
  ): Promise<PacketStream> {
    return openRpcStream(
      webDocumentUuid,
      this.runtimeHost.ServiceWorkerRpc.bind(this.runtimeHost),
    )
  }

  // lookupClient looks up an ongoing WebRuntime client connection.
  public lookupClient(webRuntimeUuid: string): WebRuntimeClientInstance | null {
    return this.clients[webRuntimeUuid] ?? null
  }

  // handleClient handles an incoming client connection MessagePort.
  // msg should contain a WebRuntimeClientInit message
  public handleClient(msg: WebRuntimeClientInit, port: MessagePort) {
    const clientUuid = msg.clientUuid
    if (!clientUuid) {
      throw new Error('connect init message: must contain client uuid')
    }
    const existing = this.lookupClient(clientUuid)
    if (existing) {
      // userp connection
      existing.close()
    }
    const clientTypeStr = webRuntimeClientTypeToJSON(msg.clientType)
    console.log(
      `WebRuntime: runtime ${msg.webRuntimeId}: registered client: ${msg.clientUuid} type ${clientTypeStr}`,
    )
    this.clients[clientUuid] = new WebRuntimeClientInstance(this, port, msg)
    if (
      msg.clientType === WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT
    ) {
      const status = <WebDocumentStatus>{
        id: clientUuid,
        deleted: false,
        permanent: false,
      }
      this.webDocuments[clientUuid] = status
      this.statusStream.pushChangeEvent({
        snapshot: false,
        webDocuments: [status],
      })
    }
  }

  // removeConnection removes a connection by clientUuid.
  public removeConnection(
    clientUuid: string,
    clientType: WebRuntimeClientType,
  ) {
    delete this.clients[clientUuid]
    if (
      clientType === WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT &&
      this.webDocuments[clientUuid]
    ) {
      delete this.webDocuments[clientUuid]
      this.statusStream.pushChangeEvent({
        snapshot: false,
        webDocuments: [
          {
            id: clientUuid,
            deleted: true,
            permanent: false,
          },
        ],
      })
    }
  }

  // buildWebRuntimeStatusSnapshot builds a snapshot of the status.
  // if allWorkers is set, includes web views from other active workers.
  // prevents duplicate web view entries
  public async buildWebRuntimeStatusSnapshot(): Promise<WebRuntimeStatus> {
    const webDocuments: WebDocumentStatus[] = []
    for (const webDocumentId of Object.keys(this.webDocuments)) {
      const webDocument = this.webDocuments[webDocumentId]
      if (webDocumentId && webDocument) {
        webDocuments.push(webDocument)
      }
    }
    webDocuments.sort((a, b) => {
      if (a < b) {
        return -1
      }
      return 1
    })
    return {
      snapshot: true,
      webDocuments,
    }
  }
}
