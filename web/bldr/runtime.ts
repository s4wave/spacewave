import {
  MessagePortConn,
  Client,
  RpcStreamHandler,
  Server,
  Stream,
  OpenStreamFunc,
  createMux,
  createHandler,
  RpcStreamPacket,
  handleRpcStream,
  buildRpcStreamOpenStream,
  RpcStreamGetter,
} from 'starpc'
import { Duplex } from 'it-stream-types'
import { pushable } from 'it-pushable'
import { pipe } from 'it-pipe'
import { Workbox } from 'workbox-window'

import {
  WebInitRuntime,
  WebViewStatus,
  WebRuntimeDefinition,
  WebRuntime,
  WebStatus,
  WatchWebStatusRequest,
  CreateWebViewRequest,
  CreateWebViewResponse,
  HostRuntime,
  HostRuntimeClientImpl,
} from '../runtime/runtime.pb.js'
import { WebViewHostClientImpl } from '../runtime/view/view.pb.js'
import { isElectron, buildElectronPort } from './electron.js'
import { LeaderElect } from './leader-elect.js'
import { addShutdownCallback, DisposeCallback } from './shutdown.js'
import { detectWasmSupported } from './wasm-detect.js'
import { WebView, WebViewRegistration, buildWebViewStatus } from './web-view.js'
import { ChannelStream, newBroadcastChannelStream } from './channel.js'
import { timeoutPromise } from './timeout.js'

// workerWebStatusKey is the key used to store the worker WebStatus snapshot.
const workerWebStatusKey = 'web-status'

// CreateWebViewCallback is a callback to create a new web view when requested.
// Throws an error if unable to create the web view.
export type CreateWebViewCallback = (webViewID: string) => Promise<void>

// ReadyCallback is a callback indicating the runtime ready state changed.
export type ReadyCallback = (runtimeReady: boolean) => void

// WebRuntimeNotifyMessage is a message sent on the worker notify channel.
interface WebRuntimeNotifyMessage {
  // from is the id of the instance that sent the message.
  from: string
  // to is the id of the destination instance.
  // if empty: targets all (depending on the message).
  to?: string
  // webStatus contains a web status update message.
  webStatus?: WebStatus
  // openRpcStream contains a RPC open stream request.
  openRpcStream?: WorkerOpenRpcStream
}

// WebViewNotifyMessage is a message sent to a WebView notify channel.
interface WebViewNotifyMessage {
  // from is the id of the instance that sent the message.
  from: string
  // openRpcStream contains a RPC open stream request.
  // the format of the channel ID will be:
  // b/wv/${webViewId}/rpc/${from}/${openRpcStream}
  openRpcStream?: string
}

// ServiceWorkerMessage is a message sent on the service worker channel.
interface ServiceWorkerMessage {
  // openRpcStream requests to open a RPC stream with the attached MessagePort.
  openRpcStream?: boolean
}

// WorkerOpenRpcStream is a message to open a rpc stream.
interface WorkerOpenRpcStream {
  // streamNonce is the nonce of the stream to open.
  streamNonce: number
}

// buildWebViewRpcStreamChannelID formats the expected rpc channel id.
function buildWebViewRpcStreamChannelID(
  from: string,
  webViewId: string,
  streamNonce: string
): string {
  return `b/wv/${webViewId}/rpc/${from}/${streamNonce}`
}

// RuntimeWebView tracks a WebView associated with a Runtime.
class RuntimeWebView {
  // id is the web view id
  public readonly id: string
  // webView is the underlying web view object.
  public readonly webView: WebView
  // notifyChannel is the incoming notifications channel.
  private readonly notifyChannel: BroadcastChannel

  constructor(notifyChannel: BroadcastChannel, webView: WebView) {
    this.id = webView.getWebViewUuid()
    this.webView = webView
    this.notifyChannel = notifyChannel
    this.notifyChannel.onmessage = this.onNotifyMessage.bind(this)
  }

  // getRpcServer returns the Server implementing the WebView rpc.
  public getRpcServer(): Promise<Server> {
    return this.webView.getRpcServer()
  }

  // buildWebViewStatus returns the WebViewStatus for the WebView.
  public buildWebViewStatus(): WebViewStatus {
    return buildWebViewStatus(this.id, this.webView)
  }

  // close closes the web view resources.
  public close() {
    this.notifyChannel.onmessage = null
    this.notifyChannel.close()
  }

  // onNotifyMessage handles an incoming WebView channel notification.
  private onNotifyMessage(event: MessageEvent<WebViewNotifyMessage>) {
    const data = event.data
    const from = data?.from
    if (!data || !from || from === this.id) {
      return
    }
    if (data.openRpcStream) {
      this.handleNotifyOpenRpcStream(from, data.openRpcStream)
    }
  }

  // handleNotifyOpenRpcStream handles a remote requesting to open a rpc stream.
  private async handleNotifyOpenRpcStream(from: string, streamNonce: string) {
    const channelID = buildWebViewRpcStreamChannelID(from, this.id, streamNonce)
    await this.handleWebViewOpenBroadcastRpcStream(channelID)
  }

  // handleWebViewOpenBroadcastRpcStream handles a request to open a RPC stream.
  private async handleWebViewOpenBroadcastRpcStream(baseChannelID: string) {
    // read channel
    const readChannel = baseChannelID + '/r'
    // write channel
    const writeChannel = baseChannelID + '/w'
    // build the stream. we know they already have opened + acked the stream.
    const remoteOpen = true
    // this will ack the stream to the remote (but not fully open it).
    const conn = newBroadcastChannelStream<Uint8Array>(
      this.id,
      readChannel,
      writeChannel,
      remoteOpen
    )
    // wait for server to be ready
    const server = await this.webView.getRpcServer()
    // start the rpc call (and open the stream)
    server.handleDuplex(conn)
  }
}

// Runtime tracks all WebView associated with Runtime instances with the same ID
// and browser context (usually based on the URL).
//
// Attaches to or mounts the root Go runtime and provides an API to interact
// with it over IPC (usually BroadcastChannel or Electron IPC).
//
// There can be multiple Runtime in a page, although it best to have 1 Runtime
// per HTML Document.
//
// Events:
//  - ready: fired when the runtime becomes ready.
//  - unready: fired when the runtime becomes not ready.
//
// Note: to put libp2p into debugging mode:
//  - Node: set the environment variable DEBUG="*"
//  - Browser: set localStorage.debug = '*'
export class Runtime extends EventTarget {
  // runtimeId is the ID of the Go and Web Runtime pair.
  private runtimeId: string
  // webRuntimeUuid is the unique id of this instance & attached worker.
  // this ID identifies this TypeScript Runtime class object.
  private webRuntimeUuid: string

  // isElectron indicates this is electron and we will use ipcRenderer.
  private isElectron?: boolean
  // useWasm indicates if web assembly is available.
  private useWasm?: boolean

  // releaseShutdownCallback removes the callback handler for onunload.
  private releaseShutdownCallback: DisposeCallback | null

  // leaderElect manages leader election and participant tracking.
  private leaderElect: LeaderElect
  // webRuntimeNotify is a broadcast channel for Runtimes to send notifications.
  private readonly webRuntimeNotify: BroadcastChannel
  // ready indicates the runtime is ready to use.
  // fires an event 'ready' when ready and 'unready' when unready.
  private ready: boolean

  // webViews contains the list of associated web views by ID.
  private webViews: { [id: string]: RuntimeWebView }

  // _webStatusUpdates is a stream of web status updates.
  public readonly webStatusStream: AsyncIterable<WebStatus>
  // _webStatusUpdates contains push + end for webStatusUpdates
  private readonly _webStatusStream: {
    push: (val: WebStatus) => void
    end: (err?: Error) => void
  }

  // workerRunning indicates we should run the worker.
  // controlled by leader election
  private workerRunning: boolean
  // worker is the loaded runtime worker
  // unset until this is the leader tab
  private worker?: Worker
  // serviceWorker is the loaded runtime service worker
  // unset until this is the leader tab
  private serviceWorker?: Workbox
  // serviceWorkerPort is the MessagePort to talk to the ServiceWorker.
  // unset until this is the leader tab
  private serviceWorkerPort?: MessagePort

  // runtimeConn is the multiplexed connection to the Runtime.
  // not set until the runtime is initialized (and we are leader).
  private runtimeConn?: MessagePortConn
  // runtimeStreamNonce is incremented to generate a new broadcast channel id.
  private runtimeStreamNonce: number

  // server is the RPC server for the WebRuntime.
  private readonly server: Server
  // client is the RPC client for the WebRuntime.
  private readonly client: Client
  // hostRuntime is the RPC interface to the host runtime.
  private readonly hostRuntime: HostRuntime

  constructor(runtimeId?: string, createWebViewCb?: CreateWebViewCallback) {
    super()

    if (!runtimeId) {
      runtimeId = 'default'
    }
    this.runtimeId = runtimeId
    this.webRuntimeUuid = Math.random().toString(36).substring(2, 9)
    this.workerRunning = false
    if (isElectron) {
      this.isElectron = true
    }
    this.ready = false
    this.runtimeStreamNonce = 0
    this.webViews = {}

    // Detect if we can use WebAssembly.
    this.useWasm = detectWasmSupported()
    if (!this.useWasm) {
      console.log('WebAssembly is not supported in this browser')
    }

    // Setup the leader election
    const electionUuid = 'r/' + this.runtimeId
    this.leaderElect = new LeaderElect(
      electionUuid,
      this.webRuntimeUuid,
      this.onLeaderChanged.bind(this),
      this.onWorkerAnnounce.bind(this)
    )
    this.webRuntimeNotify = new BroadcastChannel('b/notify/' + this.runtimeId)
    this.webRuntimeNotify.onmessage = this.onWebRuntimeNotify.bind(this)

    // Setup the status stream.
    const webStatusStream = pushable<WebStatus>({ objectMode: true })
    this.webStatusStream = webStatusStream
    this._webStatusStream = webStatusStream

    // Setup the RPC server for this WebRuntime.
    const mux = createMux()
    const webRuntime: WebRuntime = new RuntimeServer(
      this.runtimeId,
      this,
      createWebViewCb || null
    )
    mux.register(createHandler(WebRuntimeDefinition, webRuntime))
    this.server = new Server(mux)
    this.client = new Client()
    this.hostRuntime = new HostRuntimeClientImpl(this.client)

    // add a global shutdown callback to terminate this
    this.releaseShutdownCallback = addShutdownCallback(this.close.bind(this))
  }

  // postNotifyMessage posts a message to the webRuntimeNotify channel.
  private postNotifyMessage(msg: Partial<WebRuntimeNotifyMessage>) {
    this.webRuntimeNotify.postMessage(<WebRuntimeNotifyMessage>{
      ...msg,
      from: this.webRuntimeUuid,
    })
  }

  // postServiceWorkerMessage posts a message to the ServiceWorker.
  private postServiceWorkerMessage(
    msg: Partial<ServiceWorkerMessage>,
    xfer?: Transferable[]
  ) {
    if (!this.serviceWorkerPort) {
      throw new Error('service worker: not initialized')
    }
    if (xfer) {
      this.serviceWorkerPort.postMessage(msg, xfer)
    } else {
      this.serviceWorkerPort.postMessage(msg)
    }
  }

  // onServiceWorkerMessage handles an incoming service worker message.
  private onServiceWorkerMessage(event: MessageEvent<ServiceWorkerMessage>) {
    const data = event.data
    if (!data) {
      return
    }
    if (data.openRpcStream && event.ports?.length) {
      this.handleServiceWorkerOpenRpcStream(event.ports[0])
    }
  }

  // handleServiceWorkerOpenRpcStream handles a ServiceWorker requesting to open a rpc stream.
  private async handleServiceWorkerOpenRpcStream(port: MessagePort) {
    await this.handleOpenPortRpcStream(
      port,
      this.buildServiceWorkerOpenStream()
    )
  }

  // handleOpenPortRpcStream handles a component requesting to open a rpc stream over a BroadcastChannel.
  private async handleOpenPortRpcStream(
    port: MessagePort,
    openStreamFn: OpenStreamFunc
  ) {
    // build the stream. we know they already have opened + acked the stream.
    const remoteOpen = true
    // this will ack the stream to the remote.
    const conn = new ChannelStream(this.webRuntimeUuid, port, remoteOpen)
    // start the rpc call
    let stream
    try {
      stream = await openStreamFn()
    } catch (err) {
      conn.close(err as Error)
      throw err
    }
    // connect the conn to the stream
    pipe(stream, conn, stream)
  }

  // registerWebView registers a web-view with the runtime.
  public registerWebView(webView: WebView): WebViewRegistration {
    const webViewId = webView.getWebViewUuid()
    const notifyChannel = new BroadcastChannel(
      this.buildWebViewNotifyChannelID(webViewId)
    )
    const view = new RuntimeWebView(notifyChannel, webView)
    this.webViews[webViewId] = view
    console.log('runtime: registered web view with id ' + webViewId)
    this.storeWebStatusSnapshot().finally(() => {
      this.notifyWebViewUpdated(webViewId, webView)
      this.postNotifyMessage({
        webStatus: {
          snapshot: false,
          webViews: [buildWebViewStatus(webViewId, webView)],
        },
      })
    })

    // openStream opens a stream to the RPC service for WebViews.
    const openStream = this.buildWebViewOpenStream(webViewId)
    const rpcClient = new Client(openStream)
    const webViewHost = new WebViewHostClientImpl(rpcClient)
    return <WebViewRegistration>{
      rpcClient,
      webViewHost,
      release: () => {
        this.unregisterWebView(webView)
      },
    }
  }

  // isLeader checks if the local worker is leader.
  public get isLeader(): boolean {
    return this.leaderElect.isLeader
  }

  // isReady checks if the runtime is ready to use.
  public get isReady(): boolean {
    return this.ready
  }

  // buildWebRuntimeOpenStream builds the OpenStreamFunc for a WebRuntime.
  public buildWebRuntimeOpenStream(webRuntimeId: string): OpenStreamFunc {
    return buildRpcStreamOpenStream(
      webRuntimeId,
      this.hostRuntime.WebRuntimeRpc.bind(this.hostRuntime)
    )
  }

  // buildServiceWorkerOpenStream builds the OpenStreamFunc for a ServiceWorker.
  public buildServiceWorkerOpenStream(): OpenStreamFunc {
    return buildRpcStreamOpenStream(
      'sw',
      this.hostRuntime.ServiceWorkerRpc.bind(this.hostRuntime)
    )
  }

  // buildWebViewOpenStream builds the OpenStreamFunc for a WebView.
  public buildWebViewOpenStream(webViewId: string): OpenStreamFunc {
    return buildRpcStreamOpenStream(
      webViewId,
      this.hostRuntime.WebViewRpc.bind(this.hostRuntime)
    )
  }

  // buildWebViewRpcGetter builds the RpcGetter for a WebView.
  public buildWebViewRpcGetter(from: string): RpcStreamGetter {
    return (webViewId: string) => {
      return this.getWebViewRpcHandler(from, webViewId)
    }
  }

  // getWebViewRpcHandler looks up the handler for the given WebView ID.
  public async getWebViewRpcHandler(
    from: string,
    webViewId: string
  ): Promise<RpcStreamHandler | null> {
    // if a local web view
    const webView = this.webViews[webViewId]
    if (webView) {
      const server = await webView.getRpcServer()
      return server.rpcStreamHandler
    }

    // forward to remote web view
    const stream = await this.openStreamViaRemoteWebView(from, webViewId)
    // return pipe handler
    return (rpcDataStream: Duplex<Uint8Array>) => {
      pipe(rpcDataStream, stream, rpcDataStream)
    }
  }

  // openStreamViaRemoteWebView attempts to open a stream with a WebView.
  //
  // times out if WebView does not ack within 3 seconds.
  private async openStreamViaRemoteWebView(
    from: string,
    webViewId: string
  ): Promise<Stream> {
    const [stream, streamNonce] =
      this.buildWebViewBroadcastChannelStream<Uint8Array>(from, webViewId)
    const webViewNotifyChannelID = this.buildWebViewNotifyChannelID(webViewId)
    const webViewNotifyChannel = new BroadcastChannel(webViewNotifyChannelID)
    webViewNotifyChannel.postMessage(<WebViewNotifyMessage>{
      from,
      openRpcStream: streamNonce.toString(),
    })
    webViewNotifyChannel.close()
    // wait for ack or timeout
    await Promise.race([stream.waitRemoteAck, timeoutPromise(3000)])
    if (!stream.isAcked) {
      stream.close()
      throw new Error('timed out waiting for ack')
    }
    // wait for the stream to be fully opened
    await stream.waitRemoteOpen
    // return the stream
    return stream
  }

  // buildBroadcastChannelStream builds a new outgoing BroadcastChannelStream.
  private buildBroadcastChannelStream<T>(): [ChannelStream<T>, number] {
    // unique id for the stream
    const streamNonce = ++this.runtimeStreamNonce
    // broadcast channel id prefix (/r /w)
    const baseChannelID = this.buildWebRuntimeRpcStreamChannelID(
      this.webRuntimeUuid,
      streamNonce
    )
    // notify the leader until the stream is acked
    // read channel
    const readChannel = baseChannelID + '/r'
    // write channel
    const writeChannel = baseChannelID + '/w'
    // construct the broadcast channel backed stream.
    return [
      newBroadcastChannelStream<T>(
        this.webRuntimeUuid,
        readChannel,
        writeChannel,
        false
      ),
      streamNonce,
    ]
  }

  // buildWebViewBroadcastChannelStream builds a new outgoing BroadcastChannelStream.
  private buildWebViewBroadcastChannelStream<T>(
    from: string,
    webViewId: string
  ): [ChannelStream<T>, number] {
    // unique id for the stream
    const streamNonce = ++this.runtimeStreamNonce
    // broadcast channel id prefix (/r /w)
    const baseChannelID = buildWebViewRpcStreamChannelID(
      from,
      webViewId,
      streamNonce.toString()
    )
    // read channel
    const readChannel = baseChannelID + '/w'
    // write channel
    const writeChannel = baseChannelID + '/r'
    // construct the broadcast channel backed stream.
    return [
      newBroadcastChannelStream<T>(
        this.webRuntimeUuid,
        readChannel,
        writeChannel,
        false
      ),
      streamNonce,
    ]
  }

  // buildWebStatusSnapshot builds a snapshot of the status.
  // if allWorkers is set, includes web views from other active workers.
  // prevents duplicate web view entries
  public async buildWebStatusSnapshot(allWorkers: boolean): Promise<WebStatus> {
    const webViews: WebViewStatus[] = []
    const webViewIdxs: { [id: string]: number } = {}
    for (const webViewId in this.webViews) {
      const webView = this.webViews[webViewId]
      if (webViewId && webView) {
        webViewIdxs[webViewId] = webViews.length
        webViews.push(webView.buildWebViewStatus())
      }
    }
    if (allWorkers) {
      const workers = await this.leaderElect.getWorkerList()
      for (const worker of workers) {
        if (worker.id === this.webRuntimeUuid) {
          continue
        }
        const statusSnapshot = await this.loadWebStatusSnapshot(worker.id)
        if (!statusSnapshot || !statusSnapshot.webViews) {
          continue
        }
        for (const webView of statusSnapshot.webViews) {
          if (webView.id in webViewIdxs) {
            continue
          }
          webViewIdxs[webView.id] = webViews.length
          webViews.push(webView)
        }
      }
    }
    webViews.sort((a, b) => {
      if (a < b) {
        return -1
      }
      return 1
    })
    return {
      snapshot: true,
      webViews,
    }
  }

  // close shuts down the runtime.
  public close() {
    this.setReady(false)
    if (this.leaderElect) {
      this.leaderElect.close()
    }
    if (this.workerRunning) {
      this.shutdownWorker()
    }
    if (this.releaseShutdownCallback) {
      this.releaseShutdownCallback()
    }
    if (this._webStatusStream) {
      this._webStatusStream.end()
    }
    for (const viewId of Object.keys(this.webViews)) {
      const view = this.webViews[viewId]
      if (view) {
        view.close()
      }
      delete this.webViews[viewId]
    }
  }

  // setReady updates the ready field.
  private setReady(isReady: boolean) {
    isReady = !!isReady
    if (isReady === this.ready) {
      return
    }

    this.ready = isReady
    if (!isReady) {
      this.client.setOpenStreamFn(undefined)
      if (this.workerRunning) {
        this.shutdownWorker()
      }
    }
    this.onReadyChanged(isReady)
  }

  // onReadyChanged indicates the ready state changed.
  private onReadyChanged(isReady: boolean) {
    if (isReady) {
      this.dispatchEvent(new Event('ready'))
    } else {
      this.dispatchEvent(new Event('unready'))
    }
  }

  // onLeaderChanged indicates the current leader-tab changed.
  // we run one WebWorker with the main Runtime in the leader tab.
  private async onLeaderChanged(leaderID: string, isUs: boolean) {
    if (!leaderID) {
      // no leader: set ready = false
      this.setReady(false)
      return
    }

    if (isUs) {
      if (!this.workerRunning) {
        // will call setReady(true) when done.
        this.launchWorker()
      }
    } else {
      if (this.workerRunning) {
        this.shutdownWorker()
      }

      // forward all rpc calls to the leader
      this.client.setOpenStreamFn(this.openStreamViaLeader.bind(this))
      // set ready
      this.setReady(true)
    }
  }

  // openStreamViaLeader opens a RPC stream via the leader.
  private async openStreamViaLeader(): Promise<Stream> {
    const [stream, streamNonce] = this.buildBroadcastChannelStream<Uint8Array>()
    this.postNotifyMessage({
      openRpcStream: { streamNonce },
    })
    // wait for the stream to be fully opened
    await stream.waitRemoteOpen
    // return the stream
    return stream
  }

  // onWebRuntimeNotify handles an incoming worker notification message.
  private onWebRuntimeNotify(event: MessageEvent<WebRuntimeNotifyMessage>) {
    const data = event.data
    const from = data.from
    if (!data || !from || from === this.webRuntimeUuid) {
      return
    }
    if (data.to && data.to !== this.webRuntimeUuid) {
      return
    }
    if (data.webStatus) {
      this._webStatusStream.push({
        ...data.webStatus,
        snapshot: false,
      })
    }
    if (this.isLeader && data.openRpcStream && data.openRpcStream.streamNonce) {
      this.handleWebRuntimeOpenRpcStream(from, data.openRpcStream.streamNonce)
    }
  }

  // buildWebRuntimeRpcStreamChannelID builds the channel id for the stream.
  private buildWebRuntimeRpcStreamChannelID(
    webRuntimeUuid: string,
    streamNonce: number
  ) {
    return `b/r/${this.runtimeId}/rpc/wr/${webRuntimeUuid}/${streamNonce}`
  }

  // buildWebViewNotifyChannelID builds the channel id for initing streams with a WebView.
  private buildWebViewNotifyChannelID(webViewId: string) {
    return `b/r/${this.runtimeId}/rpc/wv/${webViewId}`
  }

  // handleWebRuntimeOpenRpcStream handles a WebRuntime requesting to open a rpc stream.
  private async handleWebRuntimeOpenRpcStream(
    webRuntimeUuid: string,
    streamNonce: number
  ) {
    // if we aren't the leader, ignore.
    if (!this.isLeader) {
      return
    }

    const channelID = this.buildWebRuntimeRpcStreamChannelID(
      webRuntimeUuid,
      streamNonce
    )
    await this.handleOpenBroadcastRpcStream(
      channelID,
      this.buildWebRuntimeOpenStream(webRuntimeUuid)
    )
  }

  // handleOpenBroadcastRpcStream handles a request to open a RPC stream.
  private async handleOpenBroadcastRpcStream(
    baseChannelID: string,
    openStreamFn: OpenStreamFunc
  ) {
    // read channel
    const readChannel = baseChannelID + '/w'
    // write channel
    const writeChannel = baseChannelID + '/r'
    // build the stream. we know they already have opened + acked the stream.
    const remoteOpen = true
    // this will ack the stream to the remote.
    const conn = newBroadcastChannelStream<Uint8Array>(
      this.webRuntimeUuid,
      readChannel,
      writeChannel,
      remoteOpen
    )
    // start the rpc call
    let stream
    try {
      stream = await openStreamFn()
    } catch (err) {
      conn.close(err as Error)
      throw err
    }
    // connect the conn to the stream
    pipe(stream, conn, stream)
  }

  // onWorkerAnnounce is called when a remote worker is added or removed.
  private async onWorkerAnnounce(webRuntimeUuid: string, removed: boolean) {
    if (removed) {
      await this.onWorkerRemoved(webRuntimeUuid)
    }
  }

  // onWorkerRemoved is called when a remote worker is removed.
  private async onWorkerRemoved(webRuntimeUuid: string) {
    // load the final worker web status snapshot
    const workerWebStatus = await this.loadWebStatusSnapshot(webRuntimeUuid)
    if (!workerWebStatus) {
      return
    }

    // broadcast removal of web views for worker
    for (const webView of workerWebStatus.webViews) {
      this.notifyWebViewUpdated(webView.id, undefined)
    }

    // if we are the leader, schedule deletion of the key
    setTimeout(() => {
      this.leaderElect.deleteWorkerKey(webRuntimeUuid, workerWebStatusKey)
    }, 100)
  }

  // notifyWebViewUpdated notifies all subscribers that the web view was updated.
  // if the web view is null, sends a message indicating the view was removed.
  private notifyWebViewUpdated(webViewId: string, webView?: WebView) {
    if (!webViewId) {
      return
    }

    const webStatus: WebStatus = {
      snapshot: false,
      webViews: [buildWebViewStatus(webViewId, webView)],
    }
    this._webStatusStream.push(webStatus)
  }

  // unregisterWebView removes the web-view and notifies the runtime if necessary.
  private unregisterWebView(webView: WebView) {
    const webViewId = webView?.getWebViewUuid()
    if (!webViewId) {
      return
    }
    const view = this.webViews[webViewId]
    if (view?.webView === webView) {
      view.close()
      delete this.webViews[webViewId]
      this.notifyWebViewUpdated(webViewId, undefined)
    }
  }

  // initServiceWorkerPort initializes & sends the ServiceWorker proxy.
  private initServiceWorkerPort(sw: ServiceWorker) {
    const swMessageChannel = new MessageChannel()
    const ourSwPort = swMessageChannel.port1
    const swPort = swMessageChannel.port2
    if (this.serviceWorkerPort) {
      this.serviceWorkerPort.onmessage = null
      this.serviceWorkerPort.onmessageerror = null
      this.serviceWorkerPort.close()
    }
    this.serviceWorkerPort = ourSwPort
    this.serviceWorkerPort.onmessage = this.onServiceWorkerMessage.bind(this)
    sw.postMessage('BLDR_INIT', [swPort])
  }

  // launchWorker loads and launches the webworker.
  private async launchWorker() {
    this.workerRunning = true
    if (this.worker) {
      // already running
      return
    }
    if (!('serviceWorker' in navigator)) {
      console.error(
        'Service worker not supported, bldr cannot start.',
        'chromium: chrome://flags/#unsafely-treat-insecure-origin-as-secure'
      )
      console.error('Requires a https and/or localhost URL.')
      throw new Error('service worker not supported')
    }

    // setup the service worker RPC proxy
    // NOTE: if the script isn't in /, requires the Service-Worker-Allowed: '/' header
    // NOTE: scope controls which /pages/ are covered by the worker
    // NOTE: scope can only be narrower than paths below the script path.
    // NOTE: leader controls all the pages in this browsing context.
    const swUrl = '/sw.js'
    console.log('runtime: registering service worker', swUrl)
    const wb = new Workbox(swUrl) // Not supported in Firefox: {type: 'module'}
    this.serviceWorker = wb
    let wasActivated = false
    wb.addEventListener('activated', async (event) => {
      wasActivated = true
      let sw = event.sw
      if (!sw) {
        sw = await wb.active
      }
      this.initServiceWorkerPort(sw)
    })
    const wbReg = await wb.register({ immediate: true })

    // workaround for ctrl + shift + r disabling service workers
    // https://web.dev/service-worker-lifecycle/#shift-reload
    if (wbReg && navigator.serviceWorker.controller === null) {
      console.error('runtime: detected ctrl+shift+r: reloading page')
      location.reload()
      throw new Error('page loaded with cache disabled: ctrl+shift+r')
    }

    console.log('runtime: service worker registered')

    // setup the web workers
    let ourPort: MessagePort
    if (this.isElectron) {
      // eslint-disable-next-line
      console.log('starting electron webview')
      // setup the forwarding to ipc
      ourPort = await buildElectronPort(this.webRuntimeUuid)
    } else {
      // eslint-disable-next-line
      console.log('starting runtime worker')

      // build the message channel
      const workerMessageChannel = new MessageChannel()
      ourPort = workerMessageChannel.port1
      const workerPort = workerMessageChannel.port2

      // setup the webworkers
      if (this.useWasm) {
        this.worker = new Worker(
          // eslint-disable-next-line
          new URL('/runtime/runtime-wasm.js', import.meta.url)
        )
      } else {
        this.worker = new Worker(
          // eslint-disable-next-line
          new URL('/runtime/runtime-js.js', import.meta.url)
        )
      }

      // postMessage -> init message (worker sleeps until it receives this)
      const initMsg = this.buildInitMsg()
      this.worker.postMessage(initMsg, [workerPort])
    }

    // setup the Conn to the runtime.
    this.runtimeConn = new MessagePortConn(ourPort, this.server)

    // start the flow of incoming messages
    ourPort.start()

    // set the conn on the client
    this.client.setOpenStreamFn(this.runtimeConn.buildOpenStreamFunc())

    // wait for the service worker to finish startup
    await wb.update()

    // make sure we pass the message port to the worker
    const sw = await wb.active
    if (!wasActivated) {
      this.initServiceWorkerPort(sw)
    }

    // indicate this runtime is ready to use.
    this.setReady(true)
  }

  // shutdownWorker shuts down the webworker and remote runtime conns.
  private async shutdownWorker() {
    this.workerRunning = false
    if (this.worker) {
      this.worker.terminate()
      this.worker = undefined
    }
    if (this.serviceWorker) {
      this.serviceWorker = undefined
    }
    if (this.runtimeConn) {
      this.runtimeConn = undefined
      this.client.setOpenStreamFn(undefined)
    }
  }

  // storeWebStatusSnapshot stores a web status snapshot in indexeddb.
  private async storeWebStatusSnapshot() {
    await this.leaderElect.setWorkerKey(
      this.webRuntimeUuid,
      workerWebStatusKey,
      await this.buildWebStatusSnapshot(false)
    )
  }

  // loadWebStatusSnapshot loads a web status snapshot from indexeddb.
  // if the worker id is unset, uses the local id
  private async loadWebStatusSnapshot(
    webRuntimeUuid: string
  ): Promise<WebStatus | undefined> {
    return this.leaderElect.getWorkerKey<WebStatus>(
      webRuntimeUuid,
      workerWebStatusKey
    )
  }

  // buildInitMsg builds the worker init message
  private buildInitMsg(): Uint8Array {
    return WebInitRuntime.encode({
      runtimeId: this.runtimeId,
      webRuntimeUuid: this.webRuntimeUuid,
    }).finish()
  }
}

// RuntimeServer implements the WebRuntime service.
class RuntimeServer implements WebRuntime {
  // from is the ID to attribute to incoming calls.
  public readonly from: string

  constructor(
    from: string,
    private runtime: Runtime,
    private createWebViewCb: CreateWebViewCallback | null
  ) {
    this.from = from
  }

  // CreateWebView creates a new WebView at the root level.
  public async CreateWebView(
    request: CreateWebViewRequest
  ): Promise<CreateWebViewResponse> {
    const webViewID = request.id
    if (!webViewID) {
      throw new Error('empty web view id')
    }
    const createWebView = this.createWebViewCb
    const created = !!createWebView
    if (created) {
      await createWebView(webViewID)
    }
    return { created }
  }

  // WatchWebStatus returns an initial snapshot of web views followed by updates.
  public WatchWebStatus(
    _request: WatchWebStatusRequest
  ): AsyncIterable<WebStatus> {
    return this.runtime.webStatusStream
  }

  // WebViewRpc opens a stream for a RPC call for a WebView.
  public WebViewRpc(
    request: AsyncIterable<RpcStreamPacket>
  ): AsyncIterable<RpcStreamPacket> {
    return handleRpcStream(
      request[Symbol.asyncIterator](),
      this.runtime.buildWebViewRpcGetter(this.from)
    )
  }
}
