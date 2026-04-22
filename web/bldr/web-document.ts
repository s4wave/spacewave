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
  WebWorkerMode,
  WebWorkerStatus,
  WebWorkerType,
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
import { isSaucer, SaucerRuntimeClient } from '../saucer/saucer.js'
import { addShutdownCallback, DisposeCallback } from './shutdown.js'
import { detectWasmSupported } from './wasm-detect.js'
import {
  detectWorkerCommsConfig,
  configDescription,
  type WorkerCommsDetectResult,
} from './worker-comms-detect.js'
import { CrossTabManager } from './cross-tab-manager.js'
import { WebRTCBridgeEndpoint } from './webrtc-bridge-endpoint.js'
import { createBusSab } from './sab-bus.js'
import { shouldUseWebDocumentLivenessLock } from './web-document-lock.js'
import { WebView, WebViewRegistration, buildWebViewStatus } from './web-view.js'
import {
  buildWebDocumentLockName,
  ClientToWebDocument,
  ConnectWebRtcBridgeAck,
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
const dedicatedWorkerShutdownGraceMs = 1000

// WebDocumentWebWorker tracks a WebWorker associated with a WebDocument.
class WebDocumentWebWorker {
  // worker is the instance of the worker if !shared
  public readonly worker?: Worker
  // sharedWorker is the instance of the worker if shared
  public readonly sharedWorker?: SharedWorker
  // port is the MessagePort passed to the Worker on startup
  public readonly port: MessagePort
  // workerType is the type of worker
  public readonly workerType: WebWorkerType
  // ready indicates the worker finished startup and runtime registration.
  public ready = false
  private closed = false

  public get isShared() {
    return !!this.sharedWorker
  }

  constructor(
    public readonly id: string,
    // path is the path to the user's worker script.
    public readonly path: string,
    // sharedWorkerPath is the path to the bldr shared worker script (shw.mjs).
    sharedWorkerPath: string,
    public readonly webDocumentUuid: string,
    initData: Uint8Array | undefined,
    workerType: WebWorkerType,
    // shared controls whether to use SharedWorker (true) or DedicatedWorker
    // (false). When false, path is used directly as the Worker script URL
    // without the shw.mjs wrapper.
    shared: boolean,
    onWebWorkerMessage: (e: MessageEvent<ClientToWebDocument>) => void,
    // busSab is the SAB bus for intra-tab plugin IPC (config B/C).
    busSab?: SharedArrayBuffer,
    // busPluginId is the numeric ID for this worker on the bus.
    busPluginId?: number,
    // workerCommsDetect is the main-thread detection result.
    workerCommsDetect?: WorkerCommsDetectResult,
  ) {
    if (!id) {
      throw new Error('empty web worker id')
    }
    if (!path) {
      throw new Error('web worker path must be set')
    }

    this.workerType = workerType

    const { port1: localPort, port2: workerPort } = new MessageChannel()
    const init: WebDocumentToWorker = {
      from: webDocumentUuid,
      initData,
      initPort: workerPort,
      busSab,
      busPluginId,
      workerCommsDetect,
    }

    if (shared) {
      if (!sharedWorkerPath) {
        throw new Error('shared worker path must be set')
      }

      // Build the worker URL with script path and worker type in hash
      const workerURL = new URL(sharedWorkerPath, baseURL)

      // Use the hash to pass parameters to avoid potential conflicts with
      // query parameters used by the script itself.
      // Format: #s=<scriptPath>&t=<workerType>
      // Encode necessary characters using encodeURIComponent, but then
      // replace encoded forward slashes (%2F) back to literal slashes (/),
      // as slashes are permitted characters within URL fragments (RFC 3986).
      const encodedPath = encodeURIComponent(path).replace(/%2F/g, '/')
      const workerTypeParam =
        workerType === WebWorkerType.QUICKJS ? '&t=quickjs' : ''
      const pluginParam = initData ? '&p=1' : ''
      workerURL.hash = `s=${encodedPath}${workerTypeParam}${pluginParam}`

      if (typeof SharedWorker !== 'undefined') {
        this.sharedWorker = new SharedWorker(workerURL.toString(), {
          name: id,
          type: 'module',
        })
        this.sharedWorker.port.postMessage(init, [workerPort])
      } else {
        this.worker = new Worker(workerURL.toString(), {
          name: id,
          type: 'module',
        })
        this.worker.postMessage(init, [workerPort])
      }
    } else {
      // Dedicated mode: use the same shw.mjs wrapper as SharedWorker mode
      // but with a dedicated Worker. The wrapper handles init messages,
      // dynamically imports the plugin script, and calls main(api).
      // Without the wrapper, the plugin script is loaded directly and
      // its exported main() is never called.
      if (!sharedWorkerPath) {
        throw new Error('shared worker path must be set for dedicated mode')
      }
      const workerURL = new URL(sharedWorkerPath, baseURL)
      const encodedPath = encodeURIComponent(path).replace(/%2F/g, '/')
      const workerTypeParam =
        workerType === WebWorkerType.QUICKJS ? '&t=quickjs' : ''
      const pluginParam = initData ? '&p=1' : ''
      workerURL.hash = `s=${encodedPath}${workerTypeParam}${pluginParam}`
      this.worker = new Worker(workerURL.toString(), {
        name: id,
        type: 'module',
      })
      this.worker.postMessage(init, [workerPort])
    }

    // Capture worker errors (module load failures, uncaught exceptions).
    // Without this, dedicated workers that fail during module loading
    // produce no console output and silently disappear.
    const w = this.worker
    if (w) {
      w.onerror = (ev: ErrorEvent) => {
        console.error(
          `worker ${id}: error: ${ev.message} at ${ev.filename}:${ev.lineno}:${ev.colno}`,
        )
      }
    }
    if (this.sharedWorker) {
      this.sharedWorker.onerror = (ev: Event) => {
        console.error(
          `shared worker ${id}: error: ${(ev as ErrorEvent).message}`,
        )
      }
    }

    this.port = localPort
    this.port.addEventListener('message', onWebWorkerMessage)
    this.port.start()
  }

  // close closes our connection to the worker.
  public async close() {
    if (this.closed) {
      return
    }
    this.closed = true

    // send a message to the worker to shutdown cleanly.
    const msg: WebDocumentToClient = {
      from: this.webDocumentUuid,
      close: true,
    }
    try {
      this.port.postMessage(msg)
    } catch {
      // ignored
    }

    if (this.worker) {
      await new Promise<void>((resolve) => {
        globalThis.setTimeout(() => {
          try {
            this.worker?.terminate()
          } finally {
            resolve()
          }
        }, dedicatedWorkerShutdownGraceMs)
      })
    }

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
  // webDocumentId sets the ID to use for this WebDocument.
  // If unset, a random ID will be generated.
  webDocumentId?: string
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
  // this is the .mjs file that loads the main Go program (the plugin host)
  runtimeWorkerPath?: string
  // serviceWorkerPath is the path to the bldr sw.mjs
  // NOTE: ServiceWorker controls the URL space below the script address!
  // NOTE: You MUST include sw.mjs next to your index.html.
  // if unset, defaults to /sw.mjs
  serviceWorkerPath?: string
  // sharedWorkerPath is the path to the bldr shw.mjs
  // if unset, defaults to /shw.mjs
  // This unified worker handles both native and QuickJS plugins.
  sharedWorkerPath?: string
  // forceDedicatedWorkers forces the runtime to use a dedicated Worker instead
  // of a SharedWorker. Useful for testing with Playwright which can capture
  // console output from dedicated workers but not shared. Also used as the
  // automatic fallback when SharedWorker is not supported (e.g. Chrome Android).
  forceDedicatedWorkers?: boolean
  // watchVisibility watches the page visibility API.
  // the callback should be called when the visibility changes.
  // call the callback with the initial visibility before returning.
  // return a function to use to unregister the callback.
  watchVisibility?: (cb: (hidden: boolean) => void) => DisposeCallback | null
}

// WebDocumentEvents is the set of events that WebDocument can emit.
type WebDocumentEvents = {
  visibilitychange: (hidden: boolean) => void
  webdocumentstatuschange: (snapshot: WebDocumentStatus) => void
  runtimeconnected: () => void
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
  // webRuntimeClientId is the runtime-client identity for this page incarnation.
  public readonly webRuntimeClientId: string

  // isElectron indicates this is electron and we will use ipcRenderer.
  private isElectron?: boolean
  // isSaucer indicates this is saucer and we will use HTTP endpoints.
  private isSaucer?: boolean
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

  // worker is the shared worker containing the WebRuntime (SharedWorker mode).
  // electron: not used
  private worker?: SharedWorker
  // runtimeWorker is the dedicated worker containing the WebRuntime.
  // Used when forceDedicatedWorkers is set or SharedWorker is unavailable.
  private runtimeWorker?: Worker
  // forceDedicatedWorkers forces dedicated Worker mode for the runtime.
  private forceDedicatedWorkers?: boolean
  // webRuntimePort is the Port connected to the WebRuntime (Shared Worker or Electron Main).
  // Not used in saucer mode (uses HTTP-based communication instead).
  private webRuntimePort?: MessagePort
  // webrtcBridgeEndpoints tracks active WebRTC bridge connections keyed by worker ID.
  private webrtcBridgeEndpoints = new Map<string, WebRTCBridgeEndpoint>()
  // webRuntimeClient is the client for the WebRuntime.
  private readonly webRuntimeClient: WebRuntimeClient | SaucerRuntimeClient
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
  // sharedWorkerPath is the path to the bldr shared worker script (shw.mjs).
  // This unified worker handles both native and QuickJS plugins via URL params.
  private readonly sharedWorkerPath: string
  // workerCommsDetect resolves to the detected worker communication config.
  private readonly workerCommsDetect: Promise<WorkerCommsDetectResult>
  // busSab is the shared bus SAB for intra-tab plugin IPC (config B/C).
  private busSab?: SharedArrayBuffer
  // nextBusPluginId is the next numeric plugin ID to assign on the bus.
  private nextBusPluginId = 1
  // crossTabManager manages brokered cross-tab MessagePort channels.
  public readonly crossTabManager: CrossTabManager
  // abortController aborts the Web Lock request on close.
  private abortController?: AbortController
  // pluginSingletonReady resolves when this tab can create plugin workers.
  // In DedicatedWorker runtime mode (no SharedWorker), a Web Lock ensures only
  // one tab creates plugin workers at a time (single-instance invariant).
  private pluginSingletonReady: Promise<void> = Promise.resolve()
  // singletonAbort aborts the singleton lock request on close.
  private singletonAbort?: AbortController

  // isClosed checks if the web document is closed
  public get isClosed(): boolean | Error {
    return this.closed ?? false
  }

  // isHidden checks if the web document is hidden
  public get isHidden(): boolean {
    return this.hidden
  }

  // waitConn waits for the WebRuntime connection to become ready.
  public async waitConn(): Promise<void> {
    try {
      await this.webRuntimeClient.waitConn()
      return
    } catch {
      // fall through and wait for the runtimeconnected event below
    }

    await new Promise<void>((resolve, reject) => {
      const onConnected = () => {
        this.removeListener('runtimeconnected', onConnected)
        resolve()
      }
      this.once('runtimeconnected', onConnected)
      if (this.closed) {
        this.removeListener('runtimeconnected', onConnected)
        reject(new Error('web document is closed'))
      }
    })
  }

  constructor(opts?: WebDocumentOptions) {
    super()
    this.webRuntimeId = opts?.webRuntimeId || 'default'
    this.webDocumentUuid = opts?.webDocumentId || randomId()
    this.hidden = false
    if (isElectron) {
      this.isElectron = true
    }
    if (isSaucer) {
      this.isSaucer = true
    }
    this.webRuntimeClientId =
      this.isElectron ? `${this.webDocumentUuid}-${randomId()}` : this.webDocumentUuid
    this.webViews = {}
    this.webWorkers = {}
    if (opts?.disableStoragePersist) {
      this.disableStoragePersist = true
    }
    if (opts?.closedCallback) {
      this.closedCallback = opts.closedCallback
    }
    if (opts?.forceDedicatedWorkers) {
      this.forceDedicatedWorkers = true
    }

    // Detect if we can use WebAssembly (not needed for saucer - Go runtime is native).
    if (!this.isSaucer) {
      const useWasm = detectWasmSupported()
      if (!useWasm) {
        throw new Error('WebAssembly is not supported in this browser')
      }
    }

    // Detect worker communication capabilities (SAB, OPFS, etc.).
    this.workerCommsDetect = detectWorkerCommsConfig()
    this.workerCommsDetect.then((result) => {
      const desc = configDescription(result.config)
      console.log(
        '%cbldr%c ' +
          this.webDocumentUuid +
          ' config ' +
          result.config +
          ' ' +
          desc,
        'color:#ff3838;font-weight:bold',
        'color:inherit',
      )
    })

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
    this.sharedWorkerPath = opts?.sharedWorkerPath ?? '/shw.mjs'
    this.crossTabManager = new CrossTabManager(this.webDocumentUuid)

    // Create the appropriate runtime client based on the environment.
    if (this.isSaucer) {
      this.webRuntimeClient = new SaucerRuntimeClient(
        this.webRuntimeId,
        this.webDocumentUuid,
        WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
        this.handleWebRuntimeOpenStream.bind(this),
      )
    } else {
      this.webRuntimeClient = new WebRuntimeClient(
        this.webRuntimeId,
        this.webRuntimeClientId,
        WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
        this.openWebRuntimeClient.bind(this),
        this.handleWebRuntimeOpenStream.bind(this),
        this.handleWebRuntimeClientDisconnected.bind(this),
        this.isElectron,
        this.webDocumentUuid,
      )
    }

    // add a global shutdown callback to terminate this
    // Before closing, send snapshotNow to all plugin DedicatedWorkers.
    this.releaseShutdownCallback = addShutdownCallback(() => {
      this.sendSnapshotNow()
      this.close()
    })

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

    // set the conn on the client to start accepting rpcs
    this.client.setOpenStreamFn(this.openWebDocumentHostStream.bind(this))

    // Saucer mode: Go runtime runs natively, no SharedWorker/ServiceWorker needed.
    if (this.isSaucer) {
      console.log('WebDocument: saucer mode - using HTTP-based communication')
      this.taskEnsureWebRuntimeConn()
      return
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

    // Determine whether to use a dedicated Worker instead of SharedWorker.
    // Forced by option, or when SharedWorker is unavailable (e.g. Chrome Android).
    const useDedicatedRuntime =
      this.forceDedicatedWorkers || typeof SharedWorker === 'undefined'

    // setup the runtime worker
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

      const initMsg: WebDocumentToWebRuntime = {
        from: this.webDocumentUuid,
        initWebRuntime: {
          webRuntimeId: this.webRuntimeId,
        },
      }

      if (useDedicatedRuntime) {
        // Dedicated Worker mode: create a Worker and a MessageChannel.
        // Transfer one port to the Worker for communication (same pattern
        // as SharedWorker's built-in port). Each tab gets its own Worker.
        console.log('WebDocument: using dedicated Worker for runtime')
        this.runtimeWorker = new Worker(runtimeJsURL, workerOptions)
        const { port1, port2 } = new MessageChannel()
        this.webRuntimePort = port1
        this.runtimeWorker.postMessage(initMsg, [port2])
      } else {
        // SharedWorker mode: all tabs share a single Worker.
        this.worker = new SharedWorker(runtimeJsURL, workerOptions)
        this.webRuntimePort = this.worker!.port!
        this.webRuntimePort.postMessage(initMsg)
      }
    }

    // In DedicatedWorker runtime mode, acquire a Web Lock to ensure only one
    // tab creates plugin workers at a time. SharedWorker mode doesn't need this
    // because the Go runtime's singletonWorkerDoc handles it within the shared
    // process. The lock is held until this document closes.
    if (useDedicatedRuntime && !this.isElectron && 'locks' in navigator) {
      this.singletonAbort = new AbortController()
      this.pluginSingletonReady = new Promise<void>((resolve, reject) => {
        navigator.locks
          .request(
            `bldr-plugin-singleton-${this.webRuntimeId}`,
            { signal: this.singletonAbort!.signal },
            () => {
              console.log('WebDocument: acquired plugin singleton lock')
              resolve()
              return new Promise<void>(() => {})
            },
          )
          .catch((err: unknown) => {
            reject(err)
          })
      })
      // Suppress unhandled rejection when abort fires without an active awaiter.
      this.pluginSingletonReady.catch(() => {})
    }

    // we don't expect any messages directly from the main worker port.
    this.webRuntimePort!.start()

    // setup the service worker
    // NOTE: if the script isn't in /, requires the Service-Worker-Allowed: '/' header
    // NOTE: scope controls which pages are covered by the worker.
    // NOTE: scope must only be narrower than paths below the script.
    // NOTE: for example /my/sw.mjs can only manage paths under /my/...
    const swUrl =
      opts?.serviceWorkerPath ?
        new URL(opts.serviceWorkerPath, baseURL).toString()
      : '/sw.mjs'
    console.log('WebDocument: registering service worker', swUrl)
    const wb = new Workbox(swUrl) // Not supported in Firefox: {type: 'module'}
    this.serviceWorker = wb
    this.initServiceWorker(wb)

    // Acquire a Web Lock to enable reliable disconnect detection.
    // The WebRuntime (SharedWorker) will try to acquire the same lock.
    // When this page closes (or crashes), the lock is released and the
    // WebRuntime can detect the disconnect without relying on timeouts.
    //
    // IMPORTANT: We must acquire the lock BEFORE connecting to the WebRuntime,
    // then send an armWebLock message to tell the WebRuntime to start watching.
    // This avoids a race where the WebRuntime acquires the lock first.
    if (shouldUseWebDocumentLivenessLock()) {
      this.abortController = new AbortController()
      const lockName = buildWebDocumentLockName(this.webDocumentUuid)
      navigator.locks
        .request(lockName, { signal: this.abortController.signal }, () => {
          // Lock acquired - now safe to connect to WebRuntime.
          // The WebRuntime will wait for this lock when we send armWebLock.
          this.taskEnsureWebRuntimeConn()
          // Hold the lock until the page closes or abort is called.
          // This promise never resolves while the page is open.
          return new Promise<void>(() => {})
        })
        .catch(() => {
          // Lock request was aborted (during close) - this is expected.
        })
    } else {
      // No Web Locks support - connect immediately.
      this.taskEnsureWebRuntimeConn()
    }
  }

  // openWebDocumentHostStream opens an RPC stream with the WebDocumentHost.
  // In Saucer mode, wraps the stream in WebRuntimeHost.WebDocumentRpc rpcstream
  // so Go can route to the per-document mux.
  public async openWebDocumentHostStream(): Promise<PacketStream> {
    if (this.isSaucer) {
      const src = this.webRuntimeClient as SaucerRuntimeClient
      return src.openWebDocumentHostStream(this.webDocumentUuid)
    }
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

  // buildWebViewHostClient builds the Client for a WebViewHost.
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
      throw new Error(`unknown web view: ${webViewId}`)
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
        ready: this.webWorkers[id].ready,
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
  public async createWebWorker(
    request: CreateWebWorkerRequest,
  ): Promise<CreateWebWorkerResponse> {
    if (this.closed) {
      throw new Error('web document is closed')
    }
    if (!request.id) {
      throw new Error('web worker id is required')
    }
    if (!request.path) {
      throw new Error('web worker path is required')
    }

    if (request.initData) {
      try {
        console.log('WebDocument: waiting for plugin singleton lock')
        await this.pluginSingletonReady
      } catch {
        return { created: false, shared: false }
      }
      if (this.closed) {
        return { created: false, shared: false }
      }
    }

    const old = this.webWorkers[request.id]
    if (old) {
      this.closeWorkerBridgeEndpoint(request.id)
      delete this.webWorkers[request.id]
      await old.close()
    }

    // All workers use the same sharedWorkerPath, with workerType passed in URL
    const workerType = request.workerType ?? WebWorkerType.NATIVE
    const detect = await this.workerCommsDetect

    const workerMode = request.workerMode ?? WebWorkerMode.WORKER_MODE_DEFAULT
    let shared: boolean
    if (workerMode === WebWorkerMode.WORKER_MODE_DEDICATED) {
      shared = false
    } else if (workerMode === WebWorkerMode.WORKER_MODE_SHARED) {
      shared = true
    } else {
      // WORKER_MODE_DEFAULT: for plugin workers on Config B/C (SAB available),
      // use DedicatedWorker so the SAB bus can be wired for intra-tab IPC.
      // Non-plugin workers and Config A/F keep SharedWorker.
      const isPlugin = !!request.initData
      if (isPlugin) {
        shared = detect.config !== 'B' && detect.config !== 'C'
      } else {
        shared = true
      }
    }

    // For DedicatedWorker plugins on config B/C, set up the SAB bus.
    // Falls back gracefully if SAB allocation fails.
    let busSab: SharedArrayBuffer | undefined
    let busPluginId: number | undefined
    if (!shared && request.initData) {
      if (detect.config === 'B' || detect.config === 'C') {
        try {
          if (!this.busSab) {
            this.busSab = createBusSab()
            console.log('WebDocument: created SAB bus for intra-tab plugin IPC')
          }
          busSab = this.busSab
          busPluginId = this.nextBusPluginId++
        } catch (err) {
          console.warn(
            'WebDocument: SAB bus allocation failed, using MessagePort only',
            err,
          )
        }
      }
    }

    const worker = new WebDocumentWebWorker(
      request.id,
      request.path,
      this.sharedWorkerPath,
      this.webDocumentUuid,
      request.initData,
      workerType,
      shared,
      this.onWebWorkerMessage.bind(this, request.id),
      busSab,
      busPluginId,
      detect,
    )
    this.webWorkers[request.id] = worker

    const createdShared = worker.isShared
    this.notifyWebWorkerUpdated(request.id, false, createdShared, worker.ready)
    return { created: true, shared: createdShared }
  }

  // removeWebWorker removes a web worker per request of the web runtime.
  public async removeWebWorker(
    request: RemoveWebWorkerRequest,
  ): Promise<RemoveWebWorkerResponse> {
    if (this.closed) return { removed: true }
    if (!request.id) {
      throw new Error('web worker id is required')
    }
    const old = this.webWorkers[request.id]
    if (old) {
      this.closeWorkerBridgeEndpoint(request.id)
      delete this.webWorkers[request.id]
      await old.close()
      this.notifyWebWorkerUpdated(request.id, true, old.isShared, old.ready)
    }
    return { removed: !!old }
  }

  // sendSnapshotNow sends a snapshotNow message to all plugin DedicatedWorkers.
  // Called from beforeunload to trigger urgent WASM memory snapshots.
  private sendSnapshotNow(): void {
    const msg: WebDocumentToWorker = {
      from: this.webDocumentUuid,
      snapshotNow: true,
    }
    for (const workerId in this.webWorkers) {
      const ww = this.webWorkers[workerId]
      if (ww.worker && !ww.isShared) {
        try {
          ww.worker.postMessage(msg)
        } catch {
          // Worker may already be terminated.
        }
      }
    }
  }

  public close(err?: Error) {
    if (this.closed) {
      return
    }
    this.closed = err ?? true

    // Close all WebRTC bridge endpoints.
    for (const [, endpoint] of this.webrtcBridgeEndpoints) {
      endpoint.close()
    }
    this.webrtcBridgeEndpoints.clear()

    // Notify the cross-tab broker that this tab is closing.
    navigator.serviceWorker?.controller?.postMessage({ crossTab: 'goodbye' })
    this.crossTabManager.close()

    this.client.setOpenStreamFn(undefined)
    this.webRuntimeClient.close()
    for (const viewId in this.webViews) {
      delete this.webViews[viewId]
    }
    for (const workerId in this.webWorkers) {
      void this.webWorkers[workerId].close()
      delete this.webWorkers[workerId]
    }
    if (this.worker) {
      try {
        this.worker.port.postMessage('close')
      } finally {
        this.worker.port.close()
      }
    }
    if (this.runtimeWorker) {
      this.runtimeWorker.terminate()
      this.runtimeWorker = undefined
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

    // Release Web Locks last, after all cleanup is done.
    if (this.singletonAbort) {
      this.singletonAbort.abort()
      this.singletonAbort = undefined
    }
    if (this.abortController) {
      this.abortController.abort()
      this.abortController = undefined
    }
  }

  // initServiceWorker asynchronously initializes the service worker.
  // called in the constructor
  private async initServiceWorker(wb: Workbox) {
    if (this.closed) return

    const swMessageCallback = (ev: MessageEvent) => {
      // Cross-tab broker messages (direct-port, peer-gone).
      if (this.crossTabManager.handleMessage(ev.data, ev.ports ?? [])) {
        return
      }

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
    // Skip this in Electron - it causes spurious reloads that orphan in-flight requests.
    if (!this.isElectron && wbReg && !navigator.serviceWorker.controller) {
      console.error('WebDocument: detected ctrl+shift+r: reloading page')
      location.reload()
      throw new Error('page loaded with cache disabled: ctrl+shift+r')
    }

    console.log('WebDocument: service worker registered')
    const sw = await wb.controlling

    console.log('WebDocument: service worker is controlling this page', sw)
    navigator.serviceWorker.addEventListener('message', swMessageCallback)
    this.initServiceWorkerPort(sw)

    // Send "hello" to the ServiceWorker cross-tab broker.
    // The SW creates direct MessagePort channels to every other tab.
    sw.postMessage({ crossTab: 'hello' })
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
    ready: boolean,
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
          ready,
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
      this.webDocumentUuid,
      WebRuntimeClientInit.toBinary(init),
      remotePort,
    )
    return localPort
  }

  // sendWebRuntimeOpenClient sends the message to the web runtime to open a client.
  // Only used in non-saucer mode (Electron/SharedWorker).
  private sendWebRuntimeOpenClient(
    from: string,
    init: Uint8Array,
    remotePort: MessagePort,
  ) {
    if (!this.webRuntimePort) {
      throw new Error('webRuntimePort not initialized')
    }
    const msg: WebDocumentToWebRuntime = {
      from,
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
      this.closeWorkerBridgeEndpoint(workerID)
      worker.port.close()
      delete this.webWorkers[workerID]
      this.notifyWebWorkerUpdated(workerID, true, worker.isShared, worker.ready)
      return
    }

    if (data.ready && !worker.ready) {
      worker.ready = true
      this.notifyWebWorkerUpdated(workerID, false, worker.isShared, true)
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
    const connectWebRuntime = data.connectWebRuntime
    const port = connectWebRuntime?.port ?? event.ports?.[0]
    if (connectWebRuntime?.init && port) {
      this.handleClientConnectWebRuntime(
        data.from,
        connectWebRuntime.init,
        port,
      )
    }

    if (data.connectWebRtcBridge) {
      this.handleConnectWebRtcBridge(data.from)
    }
  }

  // closeWorkerBridgeEndpoint closes and removes the WebRTC bridge endpoint
  // associated with the given worker ID, if any.
  private closeWorkerBridgeEndpoint(workerId: string) {
    const endpoint = this.webrtcBridgeEndpoints.get(workerId)
    if (endpoint) {
      endpoint.close()
      this.webrtcBridgeEndpoints.delete(workerId)
    }
  }

  // handleConnectWebRtcBridge creates a bridge MessageChannel and sends one
  // port back to the requesting worker. The other port drives a WebRTCBridgeEndpoint.
  private handleConnectWebRtcBridge(from: string) {
    // Look up the requesting worker by its id (the `from` field).
    const worker = this.webWorkers[from]
    if (!worker?.port) {
      console.warn(
        `WebDocument: WebRTC bridge request from unknown worker: ${from}`,
      )
      return
    }

    // Close any existing bridge endpoint for this worker (e.g. after restart).
    const prev = this.webrtcBridgeEndpoints.get(from)
    if (prev) {
      prev.close()
    }

    const { port1: endpointPort, port2: clientPort } = new MessageChannel()
    const endpoint = new WebRTCBridgeEndpoint(endpointPort)
    this.webrtcBridgeEndpoints.set(from, endpoint)
    console.log(`WebDocument: WebRTC bridge opened for ${from}`)

    const ack: ConnectWebRtcBridgeAck = {
      from: this.webDocumentUuid,
      bridgePort: clientPort,
    }
    worker.port.postMessage(ack, [clientPort])
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
    this.sendWebRuntimeOpenClient(from, init, webRuntimePort)
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
      }).then(() => {
        if (this.closed) {
          return
        }
        this.emit('runtimeconnected')
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
