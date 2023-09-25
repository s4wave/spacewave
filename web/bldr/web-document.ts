import {
  Client,
  RpcStreamHandler,
  Server,
  OpenStreamFunc,
  createMux,
  createHandler,
  StaticMux,
  RpcStreamPacket,
  handleRpcStream,
  buildRpcStreamOpenStream,
  RpcStreamGetter,
  Stream,
} from 'starpc'
import { Workbox } from 'workbox-window'

import {
  WebViewStatus,
  WebDocumentDefinition,
  WebDocument as WebDocumentService,
  WebDocumentStatus,
  CreateWebViewRequest,
  CreateWebViewResponse,
  WebDocumentHostClientImpl,
} from '../document/document.pb.js'
import {
  WebRuntimeClientInit,
  WebRuntimeClientType,
} from '../runtime/runtime.pb.js'
import {
  WebViewHostClientImpl,
  WebView as WebViewService,
  WebViewDefinition,
  SetRenderModeRequest,
  SetRenderModeResponse,
  RemoveWebViewResponse,
  SetHtmlLinksRequest,
  SetHtmlLinksResponse,
} from '../view/view.pb.js'
import { isElectron, handleElectronWorkerPort } from '../electron/electron.js'
import { addShutdownCallback, DisposeCallback } from './shutdown.js'
import { detectWasmSupported } from './wasm-detect.js'
import { WebView, WebViewRegistration, buildWebViewStatus } from './web-view.js'
import {
  ClientToWebRuntime,
  ConnectWebRuntimeAck,
  ServiceWorkerToWebDocument,
  WebDocumentToServiceWorker,
  WebRuntimeToClient,
} from '../runtime/runtime.js'
import { ItState } from './it-state.js'
import { ChannelStream } from './channel.js'
import { timeoutPromise } from './timeout.js'
import { randomId } from './random-id.js'

// CreateWebViewFunc is a function to create a WebView.
export type CreateWebViewFunc = (
  req: CreateWebViewRequest,
) => Promise<CreateWebViewResponse>

// RemoveWebViewFunc is a function to remove a WebView.
// Returns if the view was removed.
export type RemoveWebViewFunc = (id: string) => Promise<boolean>

// BLDR_RUNTIME_JS is an injected variable with the path to the runtime.js
declare const BLDR_RUNTIME_JS: string | undefined

// baseURL is the base URL to use for paths.
const baseURL = import.meta?.url || window.location.origin

// runtimeJsURL is the path to the bldr runtime js that we will use.
const runtimeJsURL = new URL(
  (typeof BLDR_RUNTIME_JS === 'string' ? BLDR_RUNTIME_JS : false) ||
    '/runtime/runtime-wasm.js',
  baseURL,
)

// WebDocumentWebView tracks a WebView associated with a WebDocument.
class WebDocumentWebView implements WebViewService {
  // id is the web view id
  public readonly id: string
  // parent is the parent web view id
  public readonly parent?: string
  // webView is the underlying web view object.
  public readonly webView: WebView
  // mux is the RPC Mux containing the WebViewService service.
  // contains other services if WebView implements them.
  private readonly mux: StaticMux
  // server is the RPC Server callable by the Go runtime.
  private readonly server: Server

  constructor(webView: WebView) {
    this.id = webView.getUuid()
    this.parent = webView.getParentUuid()
    this.webView = webView

    this.mux = createMux()
    this.mux.register(createHandler(WebViewDefinition, <WebViewService>this))
    if (webView.lookupMethod) {
      this.mux.registerLookupMethod(webView.lookupMethod.bind(webView))
    }
    this.server = new Server(this.mux.lookupMethodFunc)
  }

  // buildWebViewStatus returns the WebViewStatus for the WebView.
  public buildWebViewStatus(): WebViewStatus {
    return buildWebViewStatus(this.id, this.webView)
  }

  // getRpcServer returns the Server implementing the WebView rpc.
  public getRpcServer(): Server {
    return this.server
  }

  // SetRenderMode sets the rendering mode of the view.
  public async SetRenderMode(
    request: SetRenderModeRequest,
  ): Promise<SetRenderModeResponse> {
    const resp = await this.webView.setRenderMode(request)
    return resp || {}
  }

  // SetHtmlLinks sets the list of html links for the view.
  public async SetHtmlLinks(
    request: SetHtmlLinksRequest,
  ): Promise<SetHtmlLinksResponse> {
    const resp = await this.webView.setHtmlLinks(request)
    return resp || {}
  }

  // RemoveWebView requests to remove a WebView from the root level.
  public async RemoveWebView(): Promise<RemoveWebViewResponse> {
    const removed = await this.webView.remove()
    return { removed }
  }
}

// WebDocumentImpl implements the WebDocumentService.
class WebDocumentImpl implements WebDocumentService {
  // from is the ID to attribute to incoming calls.
  public readonly from: string

  constructor(
    from: string,
    private webDocument: WebDocument,
    public readonly createViewCb: CreateWebViewFunc | null,
  ) {
    this.from = from
  }

  // CreateWebView creates a new WebView at the root level.
  public async CreateWebView(
    request: CreateWebViewRequest,
  ): Promise<CreateWebViewResponse> {
    const webViewID = request.id
    if (!webViewID) {
      throw new Error('empty web view id')
    }
    const createWebView = this.createViewCb
    if (!createWebView) {
      return { created: false }
    }
    return await createWebView(request)
  }

  // WatchWebDocumentStatus returns an initial snapshot of web views followed by updates.
  public WatchWebDocumentStatus(): AsyncIterable<WebDocumentStatus> {
    return this.webDocument.webStatusStream.getIterable()
  }

  // WebViewRpc opens a stream for a RPC call for a WebView.
  public WebViewRpc(
    request: AsyncIterable<RpcStreamPacket>,
  ): AsyncIterable<RpcStreamPacket> {
    return handleRpcStream(
      request[Symbol.asyncIterator](),
      this.webDocument.buildWebViewRpcGetter(),
    )
  }
}

// WebDocumentOptions are optional parameters to WebDocument.
export interface WebDocumentOptions {
  // webRuntimeId sets the ID to use for the web runtime.
  // If unset, defaults to "default"
  webRuntimeId?: string
  // createWebViewCb is used to create web views (usually new tabs or windows).
  // if unset, the Go runtime will not be able to create new WebViews.
  createWebViewCb?: CreateWebViewFunc
  // disableStoragePersist disables requesting persistent storage permission
  // from the user on startup. This is useful if you want to call
  // navigator.storage.persist() later after displaying a message to the user
  // explaining why you are requesting the permission & requesting they approve.
  disableStoragePersist?: boolean
}

// WebDocument tracks a tree of WebView associated with a WebRuntime.
//
// Attaches to or mounts the root WebRuntime and provides an RPC API.
//
// There can be multiple WebDocument in a page, although it best to have one per
// HTML Document or Window.
//
// Note: to put libp2p into debugging mode:
//  - Node: set the environment variable DEBUG="*"
//  - Browser: set localStorage.debug = '*'
export class WebDocument {
  // webRuntimeId is the ID of the WebRuntime.
  public readonly webRuntimeId: string
  // webDocumentUuid is the unique id of this instance & attached worker.
  // this ID identifies this TypeScript WebDocument class object.
  public readonly webDocumentUuid: string

  // isElectron indicates this is electron and we will use ipcRenderer.
  private isElectron?: boolean

  // disableStoragePersist disables requesting persistent storage permission
  private disableStoragePersist?: boolean

  // releaseShutdownCallback removes the callback handler for onunload.
  private releaseShutdownCallback: DisposeCallback | null

  // webViews contains the list of associated web views by ID.
  private webViews: { [id: string]: WebDocumentWebView }

  // _webStatusUpdates is a stream of web status updates.
  public readonly webStatusStream: ItState<WebDocumentStatus>

  // serviceWorker is the loaded runtime service worker
  private serviceWorker?: Workbox

  // worker is the runtime shared worker
  // electron: not used
  private worker?: SharedWorker
  // workerPort is the Port connected to the Shared Worker or Electron Main.
  private workerPort: MessagePort
  // clientPort is the Port connected to the WebRuntime.
  private clientPort: MessagePort
  // serviceWorkerPort is the Port connected to the ServiceWorker.
  private serviceWorkerPort?: MessagePort

  // server is the RPC server for the WebDocument.
  private readonly server: Server
  // client is the RPC client for the WebDocument.
  private readonly client: Client
  // webDocumentHost is the RPC interface to the host runtime.
  private readonly webDocumentHost: WebDocumentHostClientImpl

  constructor(opts?: WebDocumentOptions) {
    this.webRuntimeId = opts?.webRuntimeId || 'default'
    this.webDocumentUuid = randomId()
    if (isElectron) {
      this.isElectron = true
    }
    this.webViews = {}
    if (opts?.disableStoragePersist) {
      this.disableStoragePersist = true
    }

    // Detect if we can use WebAssembly.
    const useWasm = detectWasmSupported()
    if (!useWasm) {
      throw new Error('WebAssembly is not supported in this browser')
    }

    // Setup the status stream.
    const webStatusStream = new ItState<WebDocumentStatus>(
      this.buildWebDocumentStatusSnapshot.bind(this),
    )
    this.webStatusStream = webStatusStream

    // Setup the RPC server for this WebDocument.
    const mux = createMux()
    const webDocument: WebDocumentService = new WebDocumentImpl(
      this.webRuntimeId,
      this,
      opts?.createWebViewCb || null,
    )
    mux.register(createHandler(WebDocumentDefinition, webDocument))
    this.server = new Server(mux.lookupMethodFunc)
    this.client = new Client()
    this.webDocumentHost = new WebDocumentHostClientImpl(this.client)

    // add a global shutdown callback to terminate this
    this.releaseShutdownCallback = addShutdownCallback(this.close.bind(this))

    // startup
    if (!('serviceWorker' in navigator)) {
      console.error(
        'Service worker not supported, bldr cannot start.',
        'chromium: chrome://flags/#unsafely-treat-insecure-origin-as-secure',
      )
      console.error('Requires a https and/or localhost URL.')
      throw new Error('service worker not supported')
    }

    if (typeof SharedWorker === 'undefined') {
      // Note: a workaround can be implemented using a WebWorker.
      // This is not currently implemented here; all major browsers support SharedWorker.
      console.error(
        'Shared worker not supported, bldr cannot start.',
        'See: https://caniuse.com/sharedworkers',
      )
      throw new Error('shared worker not supported')
    }

    // setup the shared worker
    if (this.isElectron) {
      // eslint-disable-next-line
      console.log('starting electron connection')
      const workerChannel = new MessageChannel()
      this.workerPort = workerChannel.port2
      const electronChannel = workerChannel.port1
      handleElectronWorkerPort(electronChannel)
    } else {
      // eslint-disable-next-line
      console.log('starting runtime worker')

      // request persistent storage
      if (
        !this.disableStoragePersist &&
        'storage' in navigator &&
        'persist' in navigator.storage
      ) {
        navigator.storage.persist().then((persistent) => {
          if (persistent) {
            console.log(
              'WebDocument: user approved persist, storage will not be cleared except by explicit user action.',
            )
          } else {
            console.log(
              'WebDocument: user declined to persist, storage may be cleared by the UA under pressure!',
            )
          }
        })
      }

      // setup the Go runtime
      const workerOptions = <WorkerOptions>{
        name: 'bldr:' + this.webRuntimeId,
      }
      this.worker = new SharedWorker(
        // eslint-disable-next-line
        runtimeJsURL,
        workerOptions,
      )
      this.workerPort = this.worker!.port!
    }

    // we don't expect any messages directly from the main worker port.
    this.workerPort.start()

    // setup the service worker
    // NOTE: if the script isn't in /, requires the Service-Worker-Allowed: '/' header
    // NOTE: scope controls which /pages/ are covered by the worker
    // NOTE: scope can only be narrower than paths below the script path.
    const swUrl = '/sw.mjs'
    console.log('WebDocument: registering service worker', swUrl)
    const wb = new Workbox(swUrl) // Not supported in Firefox: {type: 'module'}
    this.serviceWorker = wb
    this.initServiceWorker(wb)

    // initialize the conn with the WebRuntime for this WebDocument.
    const runtimeChannel = new MessageChannel()
    const localWebDocumentPort = runtimeChannel.port1
    const remoteWebDocumentPort = runtimeChannel.port2
    const initMsg = WebRuntimeClientInit.encode({
      webRuntimeId: this.webRuntimeId,
      clientUuid: this.webDocumentUuid,
      clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
    }).finish()
    this.clientPort = localWebDocumentPort
    this.clientPort.onmessage = this.onWebRuntimeMessage.bind(this)
    this.clientPort.start()
    this.openHostWebDocumentClient(initMsg, remoteWebDocumentPort)

    // set the conn on the client
    this.client.setOpenStreamFn(this.openWebDocumentHostStream.bind(this))
  }

  // openWebDocumentHostStream opens a stream with the WebDocumentHost.
  public async openWebDocumentHostStream(): Promise<Stream> {
    const channel = new MessageChannel()
    const localPort = channel.port1
    const channelStream = new ChannelStream<Uint8Array>(
      this.webDocumentUuid,
      localPort,
      false,
    )
    this.postWebRuntimeMessage({ openStream: true }, [channel.port2])
    await Promise.race([channelStream.waitRemoteOpen, timeoutPromise(3000)])
    if (!channelStream.isOpen) {
      throw new Error(
        'WebDocument: timeout opening stream with WebDocumentHost',
      )
    }
    return channelStream
  }

  // registerWebView registers a web-view with the runtime.
  public registerWebView(webView: WebView): WebViewRegistration {
    const webViewId = webView.getUuid()
    const parentId = webView.getParentUuid()
    const view = new WebDocumentWebView(webView)
    this.webViews[webViewId] = view
    console.log(
      `WebDocument: registered web view with id ${webViewId}` +
        (parentId ? ` parent ${parentId}` : ''),
    )
    this.notifyWebViewUpdated(webViewId, webView)

    // openStream opens a stream to the WebViewHost service.
    const rpcClient = this.buildWebViewHostClient(webViewId)
    const webViewHost = new WebViewHostClientImpl(rpcClient)
    return <WebViewRegistration>{
      rpcClient,
      webViewHost,
      release: () => {
        this.unregisterWebView(webView)
      },
    }
  }

  // buildWebViewHostOpenStream builds the OpenStreamFunc for a WebViewHost.
  public buildWebViewHostOpenStream(webViewId: string): OpenStreamFunc {
    return buildRpcStreamOpenStream(
      webViewId,
      this.webDocumentHost.WebViewRpc.bind(this.webDocumentHost),
    )
  }

  // buildWebViewHostOpenStream builds the Client for a WebViewHost.
  public buildWebViewHostClient(webViewId: string): Client {
    return new Client(this.buildWebViewHostOpenStream(webViewId))
  }

  // buildWebViewRpcGetter builds the RpcGetter for a WebView.
  public buildWebViewRpcGetter(): RpcStreamGetter {
    return (webViewId: string) => {
      return this.getWebViewRpcHandler(webViewId)
    }
  }

  // getWebViewRpcHandler looks up the handler for the given WebView ID.
  public async getWebViewRpcHandler(
    webViewId: string,
  ): Promise<RpcStreamHandler | null> {
    // if a local web view
    const webView = this.webViews[webViewId]
    if (!webView) {
      throw new Error('unknown web view: ${webViewId}')
    }

    const server = webView.getRpcServer()
    return server.rpcStreamHandler
  }

  // buildWebDocumentStatusSnapshot builds a snapshot of the status.
  // if allWorkers is set, includes web views from other active workers.
  // prevents duplicate web view entries
  public async buildWebDocumentStatusSnapshot(): Promise<WebDocumentStatus> {
    const webViews: WebViewStatus[] = []
    for (const webViewId of Object.keys(this.webViews)) {
      const webView = this.webViews[webViewId]
      if (webViewId && webView) {
        webViews.push(webView.buildWebViewStatus())
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
    this.client.setOpenStreamFn(undefined)
    for (const viewId of Object.keys(this.webViews)) {
      delete this.webViews[viewId]
    }
    if (this.clientPort) {
      try {
        this.clientPort.postMessage(<ClientToWebRuntime>{
          close: true,
        })
      } finally {
        this.clientPort.close()
      }
    }
    if (this.worker) {
      this.worker.port.close()
    }
    if (this.serviceWorker) {
      this.serviceWorker = undefined
    }
    if (this.serviceWorkerPort) {
      try {
        this.serviceWorkerPort.postMessage('close')
      } finally {
        this.serviceWorkerPort.close()
        this.serviceWorkerPort = undefined
      }
    }
    if (this.releaseShutdownCallback) {
      this.releaseShutdownCallback()
    }
  }

  // initServiceWorker asynchronously initializes the service worker.
  // called in the constructor
  private async initServiceWorker(wb: Workbox) {
    const swMessageCallback = (ev: MessageEvent) => {
      console.log('WebDocument: got message from ServiceWorker', ev.data)
      const data = ev.data
      if (typeof data === 'object' && data['BLDR_INIT_SW']) {
        const currSw = navigator.serviceWorker.controller || sw
        // the service worker needs a new message port for requests
        this.initServiceWorkerPort(currSw)
      }
    }
    /*
    wb.addEventListener('activated', (ev) => {
      console.log('WORKBOX: got activated event', ev)
    })
    wb.addEventListener('controlling', (ev) => {
      console.log('WORKBOX: got controlling event', ev)
    })
    wb.addEventListener('redundant', (ev) => {
      console.log('WORKBOX: got redundant event', ev)
    })
    */
    navigator.serviceWorker.addEventListener('controllerchange', (ev) => {
      // console.log('WORKBOX: got controllerchange event', ev.target)
      if (!ev.target) {
        return
      }
      const swContainer = ev.target as ServiceWorkerContainer
      swContainer.addEventListener('message', swMessageCallback)
    })

    // register the service worker
    const wbReg = await wb.register() // ({ immediate: true })

    // wait for the service worker to finish startup
    // await wb.active()
    await wb.update()

    // workaround for ctrl + shift + r disabling service workers
    // https://web.dev/service-worker-lifecycle/#shift-reload
    if (wbReg && navigator.serviceWorker.controller === null) {
      console.error('WebDocument: detected ctrl+shift+r: reloading page')
      location.reload()
      throw new Error('page loaded with cache disabled: ctrl+shift+r')
    }

    console.log('WebDocument: service worker registered')
    const sw = await wb.controlling

    console.log('WebDocument: service worker is controlling this page', sw)
    navigator.serviceWorker.addEventListener('message', swMessageCallback)
    this.initServiceWorkerPort(sw)
  }

  // notifyWebViewUpdated notifies all subscribers that the web view was updated.
  // if the web view is null, sends a message indicating the view was removed.
  private notifyWebViewUpdated(webViewId: string, webView?: WebView) {
    if (!webViewId) {
      return
    }

    const webStatus: WebDocumentStatus = {
      snapshot: false,
      webViews: [buildWebViewStatus(webViewId, webView)],
    }
    this.webStatusStream.pushChangeEvent(webStatus)
  }

  // unregisterWebView removes the web-view and notifies the runtime if necessary.
  private unregisterWebView(webView: WebView) {
    const webViewId = webView?.getUuid()
    if (!webViewId) {
      return
    }
    const view = this.webViews[webViewId]
    if (view?.webView === webView) {
      delete this.webViews[webViewId]
      this.notifyWebViewUpdated(webViewId, undefined)
    }
  }

  // initServiceWorkerPort initializes & sends the ServiceWorker connection port.
  private initServiceWorkerPort(sw: ServiceWorker) {
    const swMessageChannel = new MessageChannel()
    const ourSwPort = swMessageChannel.port1
    const swPort = swMessageChannel.port2
    ourSwPort.onmessage = this.onServiceWorkerMessage.bind(this)
    ourSwPort.start()
    this.serviceWorkerPort = ourSwPort
    sw.postMessage(
      <WebDocumentToServiceWorker>{
        from: this.webDocumentUuid,
        initPort: swPort,
      },
      [swPort],
    )
  }

  // openHostWebDocumentClient opens a new Client for the host runtime.
  // passes a MessagePort for the client to communicate over.
  // the client should send ClientToWebDocumentHost messages.
  // initMsg should be a WebRuntimeClientInit
  private openHostWebDocumentClient(initMsg: Uint8Array, port: MessagePort) {
    this.workerPort!.postMessage(initMsg, [port])
  }

  // onWebRuntimeMessage handles an incoming WebRuntime message.
  private onWebRuntimeMessage(event: MessageEvent<WebRuntimeToClient>) {
    const data = event.data
    if (!data) {
      return
    }
    if (data.openStream && event.ports?.length) {
      this.handleWebRuntimeOpenStream(event.ports[0])
    }
  }

  // handleWebRuntimeOpenStream handles the WebRuntime attempting to open a stream.
  private handleWebRuntimeOpenStream(port: MessagePort) {
    const channel = new ChannelStream<Uint8Array>(
      this.webDocumentUuid,
      port,
      true,
    )
    this.server.handlePacketStream(channel)
  }

  // onServiceWorkerMessage handles an incoming service worker message.
  private onServiceWorkerMessage(
    event: MessageEvent<ServiceWorkerToWebDocument>,
  ) {
    const data = event.data
    if (!data || !data.from) {
      return
    }
    if (data.connectWebRuntime && event.ports?.length) {
      this.handleServiceWorkerConnectWebRuntime(
        data.from,
        data.connectWebRuntime,
      )
    }
  }

  // handleServiceWorkerConnectWebRuntime handles a ServiceWorker requesting a connection with the WebRuntime.
  private async handleServiceWorkerConnectWebRuntime(
    id: string,
    port: MessagePort,
  ) {
    const commChannel = new MessageChannel()
    // we don't expect any replies
    port.start()

    console.log(`WebDocument: connecting ServiceWorker to WebRuntime: ${id}`)
    const initMsg = <WebRuntimeClientInit>{
      webRuntimeId: this.webRuntimeId,
      clientUuid: id,
      clientType: WebRuntimeClientType.WebRuntimeClientType_SERVICE_WORKER,
    }
    port.postMessage(
      <ConnectWebRuntimeAck>{
        from: this.webDocumentUuid,
        webRuntimePort: commChannel.port2,
      },
      [commChannel.port2],
    )
    this.openHostWebDocumentClient(
      WebRuntimeClientInit.encode(initMsg).finish(),
      commChannel.port1,
    )
  }

  // postWebRuntimeMessage posts a message to the WebRuntime client port.
  private postWebRuntimeMessage(
    msg: ClientToWebRuntime,
    xfer?: Transferable[],
  ) {
    if (xfer && xfer.length) {
      this.clientPort.postMessage(msg, xfer)
    } else {
      this.clientPort.postMessage(msg)
    }
  }
}
