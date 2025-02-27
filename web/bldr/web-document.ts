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
  PacketStream,
  MessageStream,
} from 'starpc'
import { Workbox } from 'workbox-window'

import {
  WebViewStatus,
  WebDocumentStatus,
  CreateWebViewRequest,
  CreateWebViewResponse,
  CreateWebWorkerRequest,
  CreateWebWorkerResponse,
  RemoveWebWorkerRequest,
  RemoveWebWorkerResponse,
  WebWorkerStatus,
} from '../document/document.pb.js'
import {
  WebDocumentDefinition,
  WebDocument as WebDocumentService,
  WebDocumentHostClient,
} from '../document/document_srpc.pb.js'
import {
  WebRuntimeClientInit,
  WebRuntimeClientType,
} from '../runtime/runtime.pb.js'
import {
  SetRenderModeRequest,
  SetRenderModeResponse,
  RemoveWebViewResponse,
  SetHtmlLinksRequest,
  SetHtmlLinksResponse,
  ResetWebViewResponse,
} from '../view/view.pb.js'
import {
  WebView as WebViewService,
  WebViewDefinition,
} from '../view/view_srpc.pb.js'
import { isElectron, handleElectronWorkerPort } from '../electron/electron.js'
import { addShutdownCallback, DisposeCallback } from './shutdown.js'
import { detectWasmSupported } from './wasm-detect.js'
import { WebView, WebViewRegistration, buildWebViewStatus } from './web-view.js'
import {
  ClientToWebDocument,
  ConnectWebRuntimeAck,
  ServiceWorkerToWebDocument,
  WebDocumentToClient,
  WebDocumentToWebRuntime,
  WebDocumentToWorker,
} from '../runtime/runtime.js'

import { ItState } from './it-state.js'
import { randomId } from './random-id.js'
import { SimpleEventEmitter } from './simple-event-emitter.js'
import { WebRuntimeClient } from './web-runtime-client.js'

// CreateWebViewFunc is a function to create a WebView.
export type CreateWebViewFunc = (
  req: CreateWebViewRequest,
) => Promise<CreateWebViewResponse>

// RemoveWebViewFunc is a function to remove a WebView.
// Returns if the view was removed.
export type RemoveWebViewFunc = (id: string) => Promise<boolean>

// baseURL is the base URL to use for paths.
const baseURL = import.meta?.url || window.location.origin

// WebDocumentWebWorker tracks a WebWorker associated with a WebDocument.
class WebDocumentWebWorker {
  // worker is the instance of the worker if !shared
  public readonly worker?: Worker
  // sharedWorker is the instance of the worker if shared
  public readonly sharedWorker?: SharedWorker
  // port is the MessagePort passed to the Worker on startup
  public readonly port: MessagePort

  public get isShared() {
    return !!this.sharedWorker
  }

  constructor(
    public readonly id: string,
    public readonly url: string,
    public readonly webDocumentUuid: string,
    initData: Uint8Array | undefined,
    onWebWorkerMessage: (e: MessageEvent<ClientToWebDocument>) => void,
  ) {
    if (!id) {
      throw new Error('empty web worker id')
    }
    if (!url) {
      throw new Error('web worker url must be set')
    }

    const { port1: localPort, port2: workerPort } = new MessageChannel()
    const init: WebDocumentToWorker = {
      from: webDocumentUuid,
      initData,
      initPort: workerPort,
    }

    const workerURL = new URL(url, baseURL).toString()
    if (typeof SharedWorker !== 'undefined') {
      this.sharedWorker = new SharedWorker(workerURL, {
        name: id,
        type: 'module',
      })
      this.sharedWorker.port.postMessage(init, [workerPort])
    } else {
      this.worker = new Worker(workerURL, { name: id, type: 'module' })
      this.worker.postMessage(init, [workerPort])
    }

    this.port = localPort
    this.port.addEventListener('message', onWebWorkerMessage)
    this.port.start()
  }

  // close closes our connection to the worker.
  public close() {
    // send a message to the worker to shutdown cleanly.
    const msg: WebDocumentToClient = {
      from: this.webDocumentUuid,
      close: true,
    }
    this.port.postMessage(msg)
    this.port.close()
  }
}

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
    this.server = new Server(this.mux.lookupMethod)
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

  // ResetWebView resets the contents of the web view.
  public async ResetWebView(): Promise<ResetWebViewResponse> {
    await this.webView.resetView()
    return {}
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

  // CreateWebWorker creates a new WebWorker.
  public async CreateWebWorker(
    request: CreateWebWorkerRequest,
  ): Promise<CreateWebWorkerResponse> {
    return this.webDocument.createWebWorker(request)
  }

  // RemoveWebWorker removes the WebWorker.
  public async RemoveWebWorker(
    request: RemoveWebWorkerRequest,
  ): Promise<RemoveWebWorkerResponse> {
    return this.webDocument.removeWebWorker(request)
  }

  // WatchWebDocumentStatus returns an initial snapshot of web views followed by updates.
  public WatchWebDocumentStatus(): MessageStream<WebDocumentStatus> {
    return this.webDocument.webStatusStream.getIterable()
  }

  // WebViewRpc opens a stream for a RPC call for a WebView.
  public WebViewRpc(
    request: MessageStream<RpcStreamPacket>,
  ): MessageStream<RpcStreamPacket> {
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
  // closedCallback is a callback to call during close() on WebDocument.
  closedCallback?: (err?: Error) => void
  // runtimeWorkerPath is the path to the runtime-wasm.mjs
  // if unset, defaults to ./runtime-wasm.mjs
  runtimeWorkerPath?: string
  // serviceWorkerPath is the path to the bldr sw.mjs
  // NOTE: ServiceWorker controls the URL space below the script address!
  // NOTE: You MUST include sw.mjs next to your index.html.
  // if unset, defaults to /sw.mjs
  serviceWorkerPath?: string
  // watchVisibility watches the page visibility API.
  // the callback should be called when the visibility changes.
  // call the callback with the initial visibility before returning.
  // return a function to use to unregister the callback.
  // if unset, defaults to using the document.visibilityState API
  watchVisibility?: (cb: (hidden: boolean) => void) => DisposeCallback | null
}

// WebDocumentEvents is the set of events that WebDocument can emit.
type WebDocumentEvents = {
  visibilitychange: (hidden: boolean) => void
  webdocumentstatuschange: (snapshot: WebDocumentStatus) => void
}

// WebDocument tracks a tree of WebView associated with a WebRuntime.
//
// Attaches to or mounts the root WebRuntime and provides an RPC API.
// It's best to have a single WebDocument per browser tab/window (HTML body).
//
// Browsers throttle background tabs, and timers / callbacks can be delayed by
// up to a minute. WebDocument watches the Page Visibility API and marks the
// document as hidden, increasing the ping/pong timings and timeouts. This
// allows the WebDocument to respond to RPC calls and pings while operating in a
// low-CPU-usage suspended state. In Electron, we can disable background
// throttling in the BrowserWindow.
//
// Note: to put libp2p into debugging mode:
//  - Node: set the environment variable DEBUG="*"
//  - Browser: set localStorage.debug = '*'
export class WebDocument extends SimpleEventEmitter<WebDocumentEvents> {
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
  // releaseVisibilityCallback removes the callback handler for visibility changes.
  private releaseVisibilityCallback: DisposeCallback | null
  // closedCallback is a callback to be called when the web document is closed.
  private closedCallback?: (err?: Error) => void

  // webViews contains the list of associated web views by ID.
  private webViews: { [id: string]: WebDocumentWebView }
  // webWorkers contains the list of running web workers by id.
  private webWorkers: { [id: string]: WebDocumentWebWorker }
  // webStatusStream is a stream of web status updates.
  public readonly webStatusStream: ItState<WebDocumentStatus>

  // serviceWorker is the loaded runtime service worker
  private serviceWorker?: Workbox
  // serviceWorkerPort is the Port connected to the ServiceWorker.
  private serviceWorkerPort?: MessagePort

  // worker is the shared worker containing the WebRuntime.
  // electron: not used
  private worker?: SharedWorker
  // webRuntimePort is the Port connected to the WebRuntime (Shared Worker or Electron Main).
  private webRuntimePort: MessagePort
  // webRuntimeClient is the client for the WebRuntime.
  private readonly webRuntimeClient: WebRuntimeClient
  // webDocumentHost is the RPC interface to the WebDocumentHost via the WebRuntime.
  private readonly webDocumentHost: WebDocumentHostClient

  // server is the RPC server for the WebDocument.
  private readonly server: Server
  // client is the RPC client for the WebDocument.
  private readonly client: Client

  // hidden indicates the web document is hidden
  private hidden: boolean
  // closed indicates the web document is closed with an optional error
  private closed?: true | Error

  // isClosed checks if the web document is closed
  public get isClosed(): boolean | Error {
    return this.closed ?? false
  }

  // isHidden checks if the web document is hidden
  public get isHidden(): boolean {
    return this.hidden
  }

  constructor(opts?: WebDocumentOptions) {
    super()
    this.webRuntimeId = opts?.webRuntimeId || 'default'
    this.webDocumentUuid = randomId()
    this.hidden = false
    if (isElectron) {
      this.isElectron = true
    }
    this.webViews = {}
    this.webWorkers = {}
    if (opts?.disableStoragePersist) {
      this.disableStoragePersist = true
    }
    if (opts?.closedCallback) {
      this.closedCallback = opts.closedCallback
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
      opts?.createWebViewCb ?? null,
    )
    mux.register(createHandler(WebDocumentDefinition, webDocument))
    this.server = new Server(mux.lookupMethod)
    this.client = new Client()
    this.webDocumentHost = new WebDocumentHostClient(this.client)

    this.webRuntimeClient = new WebRuntimeClient(
      this.webRuntimeId,
      this.webDocumentUuid,
      WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      this.openWebRuntimeClient.bind(this),
      this.handleWebRuntimeOpenStream.bind(this),
      this.handleWebRuntimeClientDisconnected.bind(this),
    )

    // add a global shutdown callback to terminate this
    this.releaseShutdownCallback = addShutdownCallback(this.close.bind(this))

    // watch the page visibility api
    if (opts?.watchVisibility) {
      this.releaseVisibilityCallback = opts.watchVisibility(
        this.onVisibilityChange.bind(this),
      )
    } else {
      const listener = () => this.onVisibilityChange(document.hidden)
      listener()
      document.addEventListener('visibilitychange', listener)
      this.releaseVisibilityCallback = () =>
        document.removeEventListener('visibilitychange', listener)
    }

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
      // TODO implement workaround using a WebWorker and leader election.
      // This is not currently implemented here.
      // Chrome for Android is the only major browser that does not support this:
      // https://groups.google.com/a/chromium.org/g/blink-dev/c/H73tticuudc/m/NunrjwcBBwAJ
      // https://issues.chromium.org/issues/40290702
      // https://caniuse.com/sharedworkers
      console.error(
        'Shared worker not supported, bldr cannot start!',
        'See: https://caniuse.com/sharedworkers',
      )
      // NOTE: the WebWorker (plugins) in WebRuntime automatically fallback to Worker.
      throw new Error('shared worker not supported')
    }

    // setup the shared worker
    if (this.isElectron) {
      const workerChannel = new MessageChannel()
      this.webRuntimePort = workerChannel.port2
      handleElectronWorkerPort(workerChannel.port1)
    } else {
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
      const runtimeJsURL = opts?.runtimeWorkerPath ?? './runtime-wasm.mjs'
      const workerOptions: WorkerOptions = {
        name: this.webRuntimeId,
        type: 'module',
      }
      this.worker = new SharedWorker(
        // eslint-disable-next-line
        runtimeJsURL,
        workerOptions,
      )

      this.webRuntimePort = this.worker!.port!
      const msg: WebDocumentToWebRuntime = {
        from: this.webDocumentUuid,
        initWebRuntime: {
          webRuntimeId: this.webRuntimeId,
        },
      }
      this.webRuntimePort.postMessage(msg)
    }

    // we don't expect any messages directly from the main worker port.
    this.webRuntimePort.start()

    // setup the service worker
    // NOTE: if the script isn't in /, requires the Service-Worker-Allowed: '/' header
    // NOTE: scope controls which /pages/ are covered by the worker
    // NOTE: scope can only be narrower than paths below the script path.
    const swUrl =
      opts?.serviceWorkerPath ?
        new URL(opts.serviceWorkerPath, baseURL).toString()
      : '/sw.mjs'
    console.log('WebDocument: registering service worker', swUrl)
    const wb = new Workbox(swUrl) // Not supported in Firefox: {type: 'module'}
    this.serviceWorker = wb
    this.initServiceWorker(wb)

    // set the conn on the client to start accepting rpcs
    this.client.setOpenStreamFn(this.openWebDocumentHostStream.bind(this))

    // trigger starting the connection to the WebRuntime
    this.taskEnsureWebRuntimeConn()
  }

  // openWebDocumentHostStream opens an RPC stream with the WebDocumentHost.
  public async openWebDocumentHostStream(): Promise<PacketStream> {
    return this.webRuntimeClient.openStream()
  }

  // registerWebView registers a web-view with the runtime.
  public registerWebView(webView: WebView): WebViewRegistration {
    if (this.closed) {
      throw new Error('web document is closed')
    }

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
    const reg: WebViewRegistration = {
      rpcClient,
      release: () => {
        this.unregisterWebView(webView)
      },
    }
    return reg
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
  public async buildWebDocumentStatusSnapshot(): Promise<WebDocumentStatus> {
    if (this.closed) {
      return {
        snapshot: true,
        closed: true,
        hidden: false,
        webViews: [],
        webWorkers: [],
      }
    }

    const webViews: WebViewStatus[] = []
    for (const webViewId of Object.keys(this.webViews)) {
      const webView = this.webViews[webViewId]
      if (webViewId && webView) {
        webViews.push(webView.buildWebViewStatus())
      }
    }
    webViews.sort((a, b) => ((a.id ?? '') < (b.id ?? '') ? -1 : 1))

    const webWorkers: WebWorkerStatus[] = Object.keys(this.webWorkers).map(
      (id) => ({
        id,
        deleted: false,
        shared: this.webWorkers[id].isShared,
      }),
    )

    return {
      snapshot: true,
      closed: false,
      hidden: this.hidden,
      webViews,
      webWorkers,
    }
  }

  // createWebWorker spawns a web worker per request of the web runtime.
  public createWebWorker(
    request: CreateWebWorkerRequest,
  ): CreateWebWorkerResponse {
    if (this.closed) {
      throw new Error('web document is closed')
    }
    if (!request.id) {
      throw new Error('web worker id is required')
    }
    if (!request.url) {
      throw new Error('web worker url is required')
    }

    const old = this.webWorkers[request.id]
    if (old) {
      old.close()
    }

    const worker = new WebDocumentWebWorker(
      request.id,
      request.url,
      this.webDocumentUuid,
      request.initData,
      this.onWebWorkerMessage.bind(this, request.id),
    )
    this.webWorkers[request.id] = worker

    const shared = worker.isShared
    this.notifyWebWorkerUpdated(request.id, false, shared)
    return { created: true, shared }
  }

  // removeWebWorker removes a web worker per request of the web runtime.
  public removeWebWorker(
    request: RemoveWebWorkerRequest,
  ): RemoveWebWorkerResponse {
    if (this.closed) return { removed: true }
    if (!request.id) {
      throw new Error('web worker id is required')
    }
    const old = this.webWorkers[request.id]
    if (old) {
      old.close()
      delete this.webWorkers[request.id]
      this.notifyWebWorkerUpdated(request.id, true, old.isShared)
    }
    return { removed: !!old }
  }

  // close shuts down the WebDocument with an optional error.
  public close(err?: Error) {
    if (this.closed) {
      return
    }
    this.closed = err ?? true
    this.client.setOpenStreamFn(undefined)
    this.webRuntimeClient.close()
    for (const viewId in this.webViews) {
      delete this.webViews[viewId]
    }
    for (const workerId in this.webWorkers) {
      this.webWorkers[workerId].close()
      delete this.webWorkers[workerId]
    }
    if (this.worker) {
      try {
        this.worker.port.postMessage('close')
      } finally {
        this.worker.port.close()
      }
    }
    if (this.serviceWorkerPort) {
      try {
        this.serviceWorkerPort.postMessage({
          close: true,
        } as WebDocumentToClient)
      } finally {
        this.serviceWorkerPort.close()
        this.serviceWorkerPort = undefined
      }
    }
    if (this.serviceWorker) {
      this.serviceWorker = undefined
    }
    this.pushChangeEvent({
      snapshot: true,
      closed: true,
      hidden: false,
      webViews: [],
      webWorkers: [],
    })
    if (this.releaseShutdownCallback) {
      this.releaseShutdownCallback()
    }
    if (this.releaseVisibilityCallback) {
      this.releaseVisibilityCallback()
    }
    if (this.closedCallback) {
      this.closedCallback(err)
    }
  }

  // initServiceWorker asynchronously initializes the service worker.
  // called in the constructor
  private async initServiceWorker(wb: Workbox) {
    if (this.closed) return

    const swMessageCallback = (ev: MessageEvent) => {
      console.log('WebDocument: got message from ServiceWorker', ev.data)
      const data: ServiceWorkerToWebDocument = ev.data
      if (typeof data !== 'object' || !data.from || !data.init) {
        return
      }
      const currSw = navigator.serviceWorker.controller || sw
      // the service worker wants a new message port for requests
      this.initServiceWorkerPort(currSw)
    }

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
    if (wbReg && !navigator.serviceWorker.controller) {
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
    if (!webViewId || this.closed) {
      return
    }

    const webStatus: WebDocumentStatus = {
      snapshot: false,
      closed: false,
      hidden: this.hidden,
      webWorkers: [],
      webViews: [buildWebViewStatus(webViewId, webView)],
    }
    this.pushChangeEvent(webStatus)
  }

  // notifyWebWorkerUpdated notifies all subscribers that the web worker was updated.
  private notifyWebWorkerUpdated(
    webWorkerId: string,
    deleted: boolean,
    shared: boolean,
  ) {
    if (this.closed) {
      return
    }
    const webStatus: WebDocumentStatus = {
      snapshot: false,
      closed: false,
      hidden: this.hidden,
      webViews: [],
      webWorkers: [
        {
          id: webWorkerId,
          deleted,
          shared,
        },
      ],
    }
    this.pushChangeEvent(webStatus)
  }

  // unregisterWebView removes the web-view and notifies the runtime if necessary.
  private unregisterWebView(webView: WebView) {
    if (this.closed) {
      return
    }
    const webViewId = webView?.getUuid()
    if (!webViewId) {
      return
    }
    const view = this.webViews[webViewId]
    if (view?.webView === webView) {
      console.log(`WebDocument: removed web view with id ${webViewId}`)
      delete this.webViews[webViewId]
      this.notifyWebViewUpdated(webViewId, undefined)
    }
  }

  // initServiceWorkerPort initializes & sends the ServiceWorker connection port.
  private initServiceWorkerPort(sw: ServiceWorker) {
    const { port1: localPort, port2: clientPort } = new MessageChannel()
    localPort.onmessage = this.onWebDocumentClientMessage.bind(this)
    localPort.start()
    this.serviceWorkerPort = localPort
    const msg: WebDocumentToWorker = {
      from: this.webDocumentUuid,
      initPort: clientPort,
    }
    sw.postMessage(msg, [clientPort])
  }

  // openWebRuntimeClient attempts to open a message port with the WebRuntime.
  // this is the function passed to the WebRuntimeClient for the WebDocument
  private async openWebRuntimeClient(
    init: WebRuntimeClientInit,
  ): Promise<MessagePort> {
    const { port1: localPort, port2: remotePort } = new MessageChannel()
    this.sendWebRuntimeOpenClient(
      WebRuntimeClientInit.toBinary(init),
      remotePort,
    )
    return localPort
  }

  // sendWebRuntimeOpenClient sends the message to the web runtime to open a client.
  private sendWebRuntimeOpenClient(init: Uint8Array, remotePort: MessagePort) {
    const msg: WebDocumentToWebRuntime = {
      from: this.webDocumentUuid,
      connectWebRuntime: {
        init,
        port: remotePort,
      },
    }
    this.webRuntimePort.postMessage(msg, [remotePort])
  }

  // handleWebRuntimeOpenStream handles the web runtime opening a rpc stream.
  // resolves once the stream has been passed off to be handled
  private async handleWebRuntimeOpenStream(ch: PacketStream) {
    this.server.handlePacketStream(ch)
  }

  // pushChangeEvent pushes a change event to the webStatusStream
  private async pushChangeEvent(status: WebDocumentStatus) {
    this.webStatusStream.pushChangeEvent(status)
    if (this.hasListener('webdocumentstatuschange')) {
      const snap = await this.webStatusStream.snapshot
      if (snap != null) {
        this.emit('webdocumentstatuschange', snap)
      }
    }
  }

  // onVisibilityChange handles page visibility changing
  private onVisibilityChange(hidden: boolean) {
    hidden = hidden || false
    if (hidden === this.hidden) {
      return
    }

    this.hidden = hidden
    if (hidden) {
      console.log('WebDocument: document is hidden')
    } else {
      console.log('WebDocument: document is visible')
    }
    if (this.closed) {
      return
    }

    this.pushChangeEvent({
      snapshot: false,
      closed: false,
      hidden,
      webViews: [],
      webWorkers: [],
    })

    // Emit the visibilitychange event
    this.emit('visibilitychange', hidden)
  }

  // onWebWorkerMessage handles an incoming web worker message.
  private onWebWorkerMessage(
    workerID: string,
    event: MessageEvent<ClientToWebDocument>,
  ) {
    const data = event.data
    if (!data || !data.from) {
      return
    }
    const worker = this.webWorkers[workerID]
    if (!worker) {
      return
    }
    if (data.close) {
      // Web worker was closed / removed.
      worker.close()
      delete this.webWorkers[workerID]
      this.notifyWebWorkerUpdated(workerID, true, worker.isShared)
      return
    }

    this.onWebDocumentClientMessage(event)
  }

  // onWebDocumentClientMessage handles an incoming client message.
  private onWebDocumentClientMessage(event: MessageEvent<ClientToWebDocument>) {
    const data = event.data
    if (!data || !data.from) {
      return
    }
    if (
      data.connectWebRuntime &&
      data.connectWebRuntime.init &&
      data.connectWebRuntime.port &&
      event.ports?.length
    ) {
      this.handleClientConnectWebRuntime(
        data.from,
        data.connectWebRuntime.init,
        data.connectWebRuntime.port,
      )
    }
  }

  // handleClientConnectWebRuntime handles a request to connect with the WebRuntime.
  private async handleClientConnectWebRuntime(
    from: string,
    init: Uint8Array,
    port: MessagePort,
  ) {
    console.log(`WebDocument: connecting client to WebRuntime: ${from}`)
    port.start()

    // Ack we are opening the channel and pass the MessagePort to use.
    const { port1: clientPort, port2: webRuntimePort } = new MessageChannel()
    const ack: ConnectWebRuntimeAck = {
      from: this.webDocumentUuid,
      webRuntimePort: clientPort,
    }
    port.postMessage(ack, [clientPort])
    port.close()

    // Send the MessagePort to the WebRuntime to complete the connection.
    this.sendWebRuntimeOpenClient(init, webRuntimePort)
  }

  // taskEnsureWebRuntimeConn ensures an active connection with the WebRuntime.
  private taskEnsureWebRuntimeConn() {
    queueMicrotask(() => {
      if (this.closed) {
        return
      }
      this.webRuntimeClient.waitConn().catch((err) => {
        if (this.closed) return
        console.warn('WebDocument: failed to connect to WebRuntime', err)
        setTimeout(() => this.taskEnsureWebRuntimeConn(), 100)
      })
    })
  }

  // handleWebRuntimeClientDisconnected handles if the WebRuntimeClient disconnects.
  private async handleWebRuntimeClientDisconnected() {
    if (this.closed) {
      return
    }
    this.taskEnsureWebRuntimeConn()
  }
}
