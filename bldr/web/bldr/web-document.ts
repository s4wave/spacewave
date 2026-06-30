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
  WebWorkerGenerationState,
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
import { SabPairBroker } from './sab-pair-broker.js'
import { shouldUseWebDocumentLivenessLock } from './web-document-lock.js'
import { WebView, WebViewRegistration, buildWebViewStatus } from './web-view.js'
import {
  buildWebDocumentLockName,
  ClientToWebDocument,
  ConnectWebRtcBridgeAck,
  ConnectWebRuntimeAck,
  OpenOpfsWorkerAck,
  OpenSabPairAck,
  SabPairEndpointDescriptor,
  ServiceWorkerToWebDocument,
  WebDocumentToClient,
  WebDocumentToWebRuntime,
  WebDocumentToWorker,
} from '../runtime/runtime.js'

import { ItState } from './it-state.js'
import { randomId } from './random-id.js'
import {
  SAB_PAIR_DIRECTION_MTU_BYTES,
  createSabPair,
} from './sab-ring-stream.js'
import { SimpleEventEmitter } from './simple-event-emitter.js'
import {
  WebRuntimeClient,
  type RuntimeClientStreamOpenGateResult,
} from './web-runtime-client.js'
import { markStartupBoundary } from './startup-marks.js'

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
const opfsWorkerStartupTimeoutMs = 5000
// sharedWorkerControlFallbackMs bounds how long shared-worker mode waits for the
// ServiceWorker to reach controlling state before starting the runtime anyway.
// A browser with a broken or unavailable SW then still loads (degraded cold
// load) instead of hanging on /b/* imports that the SW never serves.
const sharedWorkerControlFallbackMs = 8000

function isFirefoxBrowserRuntime(): boolean {
  return /\bFirefox\//.test(globalThis.navigator?.userAgent ?? '')
}

// shouldForceDedicatedWorkers reports whether this browser document should
// avoid SharedWorker-backed runtime and plugin workers.
export function shouldForceDedicatedWorkers(
  forceDedicatedWorkers?: boolean,
): boolean {
  return (
    !!forceDedicatedWorkers ||
    typeof SharedWorker === 'undefined' ||
    isFirefoxBrowserRuntime()
  )
}

// buildWorkerURL builds the shw.mjs wrapper URL with the user script path,
// worker type, and plugin marker encoded into the query string.
function buildWorkerParams(
  scriptPath: string,
  workerType: WebWorkerType,
  hasInitData: boolean,
): string {
  const encodedPath = encodeURIComponent(scriptPath).replace(/%2F/g, '/')
  const workerTypeParam =
    workerType === WebWorkerType.QUICKJS ? '&t=quickjs' : ''
  const pluginParam = hasInitData ? '&p=1' : ''
  return `s=${encodedPath}${workerTypeParam}${pluginParam}`
}

function buildWorkerURL(sharedWorkerPath: string, params: string): URL {
  const url = new URL(sharedWorkerPath, baseURL)
  url.search = params
  return url
}

function buildOpfsWorkerURL(opfsWorkerPath: string): URL {
  return new URL(opfsWorkerPath, baseURL)
}

function isOpfsWorkerReadyMessage(data: unknown): boolean {
  return (
    typeof data === 'object' &&
    data !== null &&
    'opfsWorkerReady' in data &&
    data.opfsWorkerReady === true
  )
}

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
  public readonly plugin: boolean
  // ready indicates the worker finished startup and runtime registration.
  public ready = false
  public generationState = WebWorkerGenerationState.WORKER_CREATED
  public failureReason?: string
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
    // workerCommsDetect is the main-thread detection result.
    workerCommsDetect?: WorkerCommsDetectResult,
    runtimeWasmEnv?: Record<string, string>,
  ) {
    if (!id) {
      throw new Error('empty web worker id')
    }
    if (!path) {
      throw new Error('web worker path must be set')
    }

    this.workerType = workerType
    this.plugin = !!initData
    markStartupBoundary('worker.construct-start', {
      source: 'browser',
      documentId: webDocumentUuid,
      workerId: id,
      path,
      shared,
      workerType,
      plugin: this.plugin,
    })

    const { port1: localPort, port2: workerPort } = new MessageChannel()
    const init: WebDocumentToWorker = {
      from: webDocumentUuid,
      initData,
      initPort: workerPort,
      workerCommsDetect,
      runtimeWasmEnv,
    }

    if (shared) {
      if (!sharedWorkerPath) {
        throw new Error('shared worker path must be set')
      }

      const workerParams = buildWorkerParams(path, workerType, !!initData)
      const workerURL = buildWorkerURL(sharedWorkerPath, workerParams)
      const workerName = `${id}?${workerParams}`

      if (typeof SharedWorker !== 'undefined') {
        this.sharedWorker = new SharedWorker(workerURL.toString(), {
          name: workerName,
          type: 'module',
        })
        markStartupBoundary('worker.shared-created', {
          source: 'browser',
          documentId: webDocumentUuid,
          workerId: id,
          path,
          shared: true,
          workerType,
          plugin: this.plugin,
        })
        this.sharedWorker.port.postMessage(init, [workerPort])
      } else {
        this.worker = new Worker(workerURL.toString(), {
          name: workerName,
          type: 'module',
        })
        markStartupBoundary('worker.shared-fallback-created', {
          source: 'browser',
          documentId: webDocumentUuid,
          workerId: id,
          path,
          shared: false,
          workerType,
          plugin: this.plugin,
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
      const workerParams = buildWorkerParams(path, workerType, !!initData)
      const workerURL = buildWorkerURL(sharedWorkerPath, workerParams)
      this.worker = new Worker(workerURL.toString(), {
        name: `${id}?${workerParams}`,
        type: 'module',
      })
      markStartupBoundary('worker.dedicated-created', {
        source: 'browser',
        documentId: webDocumentUuid,
        workerId: id,
        path,
        shared: false,
        workerType,
        plugin: this.plugin,
      })
      this.worker.postMessage(init, [workerPort])
    }

    // Capture worker errors (module load failures, uncaught exceptions).
    // Without this, dedicated workers that fail during module loading
    // produce no console output and silently disappear.
    const w = this.worker
    if (w) {
      w.onerror = (ev: ErrorEvent) => {
        const stack =
          ev.error instanceof Error && ev.error.stack
            ? `\n${ev.error.stack}`
            : ''
        console.error(
          `worker ${id}: error: ${ev.message} at ${ev.filename}:${ev.lineno}:${ev.colno}${stack}`,
        )
      }
    }
    if (this.sharedWorker) {
      this.sharedWorker.onerror = (ev: Event) => {
        const err = ev as ErrorEvent
        const stack =
          err.error instanceof Error && err.error.stack
            ? `\n${err.error.stack}`
            : ''
        console.error(`shared worker ${id}: error: ${err.message}${stack}`)
      }
    }

    this.port = localPort
    this.port.addEventListener('message', onWebWorkerMessage)
    this.port.start()
    markStartupBoundary('worker.port-started', {
      source: 'browser',
      documentId: webDocumentUuid,
      workerId: id,
      shared: this.isShared,
      workerType,
      plugin: this.plugin,
    })
  }

  public setGenerationState(
    generationState: WebWorkerGenerationState,
    failureReason?: string,
  ) {
    this.generationState = generationState
    if (failureReason) {
      this.failureReason = failureReason
    }
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

    if (this.worker && !this.ready) {
      this.worker.terminate()
    } else if (this.worker) {
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

function isFailedWorkerGenerationState(
  generationState: WebWorkerGenerationState,
): boolean {
  return (
    generationState === WebWorkerGenerationState.TERMINAL_FAILURE ||
    generationState === WebWorkerGenerationState.STARTUP_TIMEOUT
  )
}

function advanceWorkerGenerationState(
  worker: WebDocumentWebWorker,
  generationState: WebWorkerGenerationState,
): boolean {
  if (worker.generationState >= generationState) {
    return false
  }
  worker.setGenerationState(generationState)
  return true
}

function classifyWorkerCloseGenerationState(
  failureReason?: string,
): WebWorkerGenerationState {
  if (!failureReason) {
    return WebWorkerGenerationState.NORMAL_STOP
  }
  const reason = failureReason.toLowerCase()
  if (reason.includes('stream reset') || reason.includes('err_rpc_abort')) {
    return WebWorkerGenerationState.CONTROLLED_STREAM_RESET
  }
  if (reason.includes('startup timeout') || reason.includes('timed out')) {
    return WebWorkerGenerationState.STARTUP_TIMEOUT
  }
  return WebWorkerGenerationState.TERMINAL_FAILURE
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
  // opfsWorkerPath sets the path to the OPFS protocol worker.
  // If unset, defaults to "/opfs-worker.mjs".
  opfsWorkerPath?: string
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
  // forceMessagePortWorkerComms forces Config A worker communication even when
  // SAB and OPFS are available.
  forceMessagePortWorkerComms?: boolean
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
  resumeready: () => void
  closed: (err?: Error) => void
}

interface RuntimeEnvGlobal {
  BLDR_RUNTIME_WASM_ENV?: Record<string, string>
}

function readRuntimeWasmEnv(): Record<string, string> | undefined {
  const raw = (globalThis as RuntimeEnvGlobal).BLDR_RUNTIME_WASM_ENV
  if (!raw || typeof raw !== 'object') {
    return undefined
  }
  const env: Record<string, string> = {}
  for (const [key, value] of Object.entries(raw)) {
    if (typeof key === 'string' && typeof value === 'string') {
      env[key] = value
    }
  }
  return Object.keys(env).length > 0 ? env : undefined
}

export interface WebDocumentResumeReadyState {
  ready: true
  documentId: string
  runtimeId: string
  hidden: false
  sequence: number
  focused?: boolean
  visibilityState?: string
  timestampMs?: number
}

declare global {
  var __swServiceWorker: string | undefined
  var __swWebDocumentResumeReady: WebDocumentResumeReadyState | undefined
}

// registerUpdatedServiceWorker registers the boot manifest's newer SW URL.
export async function registerUpdatedServiceWorker(
  currentSwUrl: string,
  currentRegistration: ServiceWorkerRegistration | null | undefined,
  register: (
    scriptURL: string,
    options?: RegistrationOptions,
  ) => Promise<ServiceWorkerRegistration> = navigator.serviceWorker.register.bind(
    navigator.serviceWorker,
  ),
  manifestSwPath = globalThis.__swServiceWorker,
): Promise<ServiceWorkerRegistration | null> {
  if (!manifestSwPath) {
    return null
  }
  const nextSwUrl = new URL(manifestSwPath, location.href).toString()
  const registeredSwUrl = new URL(currentSwUrl, location.href).toString()
  if (nextSwUrl === registeredSwUrl) {
    return null
  }
  return register(nextSwUrl, {
    scope: currentRegistration?.scope,
  })
}

// WebDocument tracks a tree of WebView associated with a WebRuntime.
//
// Attaches to or mounts the root WebRuntime and provides an RPC API.
// It's best to have a single WebDocument per browser tab/window (HTML body).
//
// Browsers throttle background tabs, and timers / callbacks can be delayed by
// up to a minute. WebDocument treats Page Visibility as state for runtime
// handoff, but liveness is owned by ports and Web Locks rather than timeouts.
// In Electron, we can disable background throttling in the BrowserWindow.
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
  // forceMessagePortWorkerComms forces MessagePort-only worker communication.
  private forceMessagePortWorkerComms?: boolean
  // webRuntimePort is the Port connected to the WebRuntime (Shared Worker or Electron Main).
  // Not used in saucer mode (uses HTTP-based communication instead).
  private webRuntimePort?: MessagePort
  // webrtcBridgeEndpoints tracks active WebRTC bridge connections keyed by worker ID.
  private webrtcBridgeEndpoints = new Map<string, WebRTCBridgeEndpoint>()
  // sabPairBroker tracks active same-tab SAB pair metadata.
  private readonly sabPairBroker = new SabPairBroker()
  // opfsWorkers tracks active OPFS protocol workers keyed by requester worker ID.
  private opfsWorkers = new Map<string, Worker>()
  // runtimeOpfsBrokerPort is this document's broker channel to the engine runtime
  // SharedWorker for OPFS bridge requests. Only set in shared-runtime mode.
  private runtimeOpfsBrokerPort?: MessagePort
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
  // opfsWorkerPath is the path to the dedicated OPFS protocol worker script.
  private readonly opfsWorkerPath: string
  // workerCommsDetect resolves to the detected worker communication config.
  private readonly workerCommsDetect: Promise<WorkerCommsDetectResult>
  // crossTabManager manages brokered cross-tab MessagePort channels.
  public readonly crossTabManager: CrossTabManager
  // abortController aborts the Web Lock request on close.
  private abortController?: AbortController
  // pluginSingletonReady resolves when this tab can create plugin workers.
  // A Web Lock ensures only one tab creates runtime-scoped dedicated plugin
  // workers at a time. The holder keeps the lock until close so other tabs use
  // the same worker instance through the WebRuntime instead of forcing handoff.
  private pluginSingletonReady: Promise<void> = Promise.resolve()
  // pluginSingletonLockEnabled records whether this document participates in
  // ownership of the dedicated plugin worker singleton.
  private pluginSingletonLockEnabled = false
  // singletonAbort aborts the singleton lock request on close.
  private singletonAbort?: AbortController
  // firstWorkerCreationMarked records the first worker boundary once per document.
  private firstWorkerCreationMarked = false
  // firstWorkerReadyMarked records the first ready boundary once per document.
  private firstWorkerReadyMarked = false
  // runtimeConnected records that this document has a live runtime channel.
  private runtimeConnected = false
  // resumeReady records that this foreground document has reached a stable point
  // where resume-sensitive startup collectors can treat it as usable.
  private resumeReady = false
  // resumeReadyPending records an in-flight foreground-frame stability check.
  private resumeReadyPending = false
  // resumeReadySequence increments each time this document reaches the
  // foreground resume-ready gate.
  private resumeReadySequence = 0

  // isClosed checks if the web document is closed
  public get isClosed(): boolean | Error {
    return this.closed ?? false
  }

  // isHidden checks if the web document is hidden
  public get isHidden(): boolean {
    return this.hidden
  }

  public getResumeReadyState(): WebDocumentResumeReadyState | null {
    const state = globalThis.__swWebDocumentResumeReady
    if (state?.documentId !== this.webDocumentUuid) {
      return null
    }
    return state
  }

  // waitConn waits for the WebRuntime connection to become ready.
  public async waitConn(): Promise<void> {
    try {
      markStartupBoundary('runtime.wait-conn-start', {
        source: 'browser',
        documentId: this.webDocumentUuid,
        runtimeId: this.webRuntimeId,
      })
      await this.webRuntimeClient.waitConn()
      markStartupBoundary('runtime.wait-conn-ready', {
        source: 'browser',
        documentId: this.webDocumentUuid,
        runtimeId: this.webRuntimeId,
      })
      return
    } catch {
      // fall through and wait for the runtimeconnected event below
    }

    await new Promise<void>((resolve, reject) => {
      const onConnected = () => {
        this.removeListener('runtimeconnected', onConnected)
        markStartupBoundary('runtime.event-connected', {
          source: 'browser',
          documentId: this.webDocumentUuid,
          runtimeId: this.webRuntimeId,
        })
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
    markStartupBoundary('web-document.construct-start', {
      source: 'browser',
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
    })
    this.hidden = false
    if (isElectron) {
      this.isElectron = true
    }
    if (isSaucer) {
      this.isSaucer = true
    }
    this.webRuntimeClientId = this.isElectron
      ? `${this.webDocumentUuid}-${randomId()}`
      : this.webDocumentUuid
    this.webViews = {}
    this.webWorkers = {}
    if (opts?.disableStoragePersist) {
      this.disableStoragePersist = true
    }
    if (opts?.closedCallback) {
      this.closedCallback = opts.closedCallback
    }
    this.forceDedicatedWorkers = shouldForceDedicatedWorkers(
      opts?.forceDedicatedWorkers,
    )
    this.forceMessagePortWorkerComms = !!opts?.forceMessagePortWorkerComms

    // Detect if we can use WebAssembly (not needed for saucer - Go runtime is native).
    if (!this.isSaucer) {
      const useWasm = detectWasmSupported()
      if (!useWasm) {
        throw new Error('WebAssembly is not supported in this browser')
      }
    }

    // Detect worker communication capabilities (SAB, OPFS, etc.).
    this.workerCommsDetect = detectWorkerCommsConfig().then((result) =>
      this.forceMessagePortWorkerComms ? { ...result, config: 'A' } : result,
    )
    this.workerCommsDetect.then((result) => {
      const desc = configDescription(result.config)
      markStartupBoundary('worker-comms.detected', {
        source: 'browser',
        documentId: this.webDocumentUuid,
        runtimeId: this.webRuntimeId,
        config: result.config,
        ...result.caps,
      })
      console.log(
        '%cbldr%c %s config %s %s',
        'color:#ff3838;font-weight:bold',
        'color:inherit',
        this.webDocumentUuid,
        result.config,
        desc,
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
    this.opfsWorkerPath = opts?.opfsWorkerPath ?? '/opfs-worker.mjs'
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
        this.waitForResumeReady.bind(this),
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
    const useDedicatedRuntime = this.forceDedicatedWorkers
    markStartupBoundary('runtime.mode-selected', {
      source: 'browser',
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
      mode: useDedicatedRuntime ? 'dedicated-worker' : 'shared-worker',
    })

    const startWebRuntimeWorker = () => {
      if (this.webRuntimePort || this.closed) {
        return
      }
      // setup the runtime worker
      if (this.isElectron) {
        const workerChannel = new MessageChannel()
        this.webRuntimePort = workerChannel.port2
        handleElectronWorkerPort(workerChannel.port1)
        this.webRuntimePort.start()
        return
      }

      // request persistent storage
      markStartupBoundary('storage.mode-detected', {
        source: 'browser',
        documentId: this.webDocumentUuid,
        runtimeId: this.webRuntimeId,
        mode:
          typeof navigator.storage?.getDirectory === 'function'
            ? 'browser-opfs-indexeddb'
            : 'browser-indexeddb',
        persistSupported: typeof navigator.storage?.persist === 'function',
        persistedSupported: typeof navigator.storage?.persisted === 'function',
      })
      if (
        !this.disableStoragePersist &&
        'storage' in navigator &&
        'persist' in navigator.storage
      ) {
        markStartupBoundary('storage.persist-request-start', {
          source: 'browser',
          documentId: this.webDocumentUuid,
          runtimeId: this.webRuntimeId,
        })
        navigator.storage.persist().then((persistent) => {
          markStartupBoundary('storage.persist-ready', {
            source: 'browser',
            documentId: this.webDocumentUuid,
            runtimeId: this.webRuntimeId,
            persistent,
          })
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

      const runtimeWasmEnv = readRuntimeWasmEnv()
      const initMsg: WebDocumentToWebRuntime = {
        from: this.webDocumentUuid,
        initWebRuntime: {
          webRuntimeId: this.webRuntimeId,
          env: runtimeWasmEnv,
        },
      }

      if (useDedicatedRuntime) {
        // Dedicated Worker mode: create a Worker and a MessageChannel.
        // Transfer one port to the Worker for communication (same pattern
        // as SharedWorker's built-in port). Each tab gets its own Worker.
        console.log('WebDocument: using dedicated Worker for runtime')
        markStartupBoundary('runtime.worker-create-start', {
          source: 'browser',
          documentId: this.webDocumentUuid,
          runtimeId: this.webRuntimeId,
          mode: 'dedicated-worker',
        })
        this.runtimeWorker = new Worker(runtimeJsURL, workerOptions)
        const { port1, port2 } = new MessageChannel()
        this.webRuntimePort = port1
        this.runtimeWorker.postMessage(initMsg, [port2])
        markStartupBoundary('runtime.worker-created', {
          source: 'browser',
          documentId: this.webDocumentUuid,
          runtimeId: this.webRuntimeId,
          mode: 'dedicated-worker',
        })
        markStartupBoundary('runtime.opfs-bridge-ready', {
          source: 'browser',
          documentId: this.webDocumentUuid,
          runtimeId: this.webRuntimeId,
          workerId: this.webRuntimeId,
          mode: 'dedicated-worker',
          enabled: false,
        })
      } else {
        // SharedWorker mode: all tabs share a single Worker.
        markStartupBoundary('runtime.worker-create-start', {
          source: 'browser',
          documentId: this.webDocumentUuid,
          runtimeId: this.webRuntimeId,
          mode: 'shared-worker',
        })
        this.worker = new SharedWorker(runtimeJsURL, workerOptions)
        this.webRuntimePort = this.worker.port!
        // A SharedWorker runtime cannot call navigator.storage.getDirectory()
        // (Chrome SecurityError). Hand it a broker port so it can request a
        // DedicatedWorker OPFS bridge from this document before starting Go.
        const opfsBrokerChannel = new MessageChannel()
        this.runtimeOpfsBrokerPort = opfsBrokerChannel.port1
        this.runtimeOpfsBrokerPort.onmessage = (ev) => {
          this.onRuntimeOpfsBrokerMessage(opfsBrokerChannel.port1, ev)
        }
        this.runtimeOpfsBrokerPort.start()
        initMsg.opfsBrokerPort = opfsBrokerChannel.port2
        this.webRuntimePort.postMessage(initMsg, [opfsBrokerChannel.port2])
        markStartupBoundary('runtime.worker-created', {
          source: 'browser',
          documentId: this.webDocumentUuid,
          runtimeId: this.webRuntimeId,
          mode: 'shared-worker',
        })
      }

      // In DedicatedWorker runtime mode, acquire a Web Lock to ensure only one
      // foreground tab creates plugin workers at a time. SharedWorker mode
      // doesn't need this because the shared Go runtime owns the singleton in
      // one process.
      const usePluginSingletonLock = useDedicatedRuntime && !this.isElectron
      if (usePluginSingletonLock) {
        this.enablePluginSingletonLock()
      }

      // we don't expect any messages directly from the main worker port.
      this.webRuntimePort.start()
    }

    const startWebRuntimeConnection = () => {
      if (this.closed) {
        return
      }
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

    // setup the service worker
    // NOTE: if the script isn't in /, requires the Service-Worker-Allowed: '/' header
    // NOTE: scope controls which pages are covered by the worker.
    // NOTE: scope must only be narrower than paths below the script.
    // NOTE: for example /my/sw.mjs can only manage paths under /my/...
    const swUrl = opts?.serviceWorkerPath
      ? new URL(opts.serviceWorkerPath, baseURL).toString()
      : '/sw.mjs'
    console.log('WebDocument: registering service worker', swUrl)
    markStartupBoundary('service-worker.register-start', {
      source: 'browser',
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
    })
    const wb = new Workbox(swUrl) // Not supported in Firefox: {type: 'module'}
    this.serviceWorker = wb

    if (useDedicatedRuntime) {
      void this.initServiceWorker(wb, swUrl, () => {
        startWebRuntimeWorker()
        startWebRuntimeConnection()
      })
    } else {
      // Shared-worker mode: the shared worker imports its plugin bundle and the
      // QuickJS wasm from /b/* paths, which only resolve once the ServiceWorker
      // is controlling this page. Starting the worker before control means those
      // imports hit the origin, which serves the SPA index.html for unmatched
      // paths, so the module/wasm loads fail and the runtime never comes up. Gate
      // the start on SW control like the dedicated branch, with a bounded fallback
      // so a browser whose SW never reaches controlling state still loads
      // (degraded) instead of hanging forever.
      let runtimeStarted = false
      const startRuntimeOnce = () => {
        if (runtimeStarted || this.closed) {
          return
        }
        runtimeStarted = true
        startWebRuntimeWorker()
        startWebRuntimeConnection()
      }
      const controlFallback = globalThis.setTimeout(
        startRuntimeOnce,
        sharedWorkerControlFallbackMs,
      )
      void this.initServiceWorker(wb, swUrl, () => {
        globalThis.clearTimeout(controlFallback)
        startRuntimeOnce()
      })
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
        generationState: this.webWorkers[id].generationState,
        failed: isFailedWorkerGenerationState(
          this.webWorkers[id].generationState,
        ),
        failureReason: this.webWorkers[id].failureReason,
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
    const plugin = !!request.initData
    const workerType = request.workerType ?? WebWorkerType.NATIVE
    const activeWorkerCountBefore = Object.keys(this.webWorkers).length
    const replacedWorker = !!this.webWorkers[request.id]
    markStartupBoundary('worker.create-request-received', {
      source: 'browser',
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
      workerId: request.id,
      workerType,
      plugin,
      path: request.path,
      activeWorkerCountBefore,
      replacedWorker,
    })

    if (plugin) {
      try {
        console.log('WebDocument: waiting for plugin singleton lock')
        markStartupBoundary('singleton-lock.wait-start', {
          source: 'browser',
          documentId: this.webDocumentUuid,
          runtimeId: this.webRuntimeId,
          workerId: request.id,
        })
        await this.pluginSingletonReady
        markStartupBoundary('singleton-lock.wait-ready', {
          source: 'browser',
          documentId: this.webDocumentUuid,
          runtimeId: this.webRuntimeId,
          workerId: request.id,
        })
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
      this.closeOpfsWorker(request.id)
      this.closeSabPairsForWorker(request.id, 'worker replaced')
      old.setGenerationState(WebWorkerGenerationState.NORMAL_STOP)
      delete this.webWorkers[request.id]
      await old.close()
    }

    // All workers use the same sharedWorkerPath, with workerType passed in URL
    markStartupBoundary('worker.create-request-accepted', {
      source: 'browser',
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
      workerId: request.id,
      workerType,
      plugin,
      path: request.path,
      activeWorkerCountBefore,
      replacedWorker,
    })
    const detect = await this.workerCommsDetect
    if (!this.firstWorkerCreationMarked) {
      this.firstWorkerCreationMarked = true
      markStartupBoundary('worker.first-create-start', {
        source: 'browser',
        documentId: this.webDocumentUuid,
        runtimeId: this.webRuntimeId,
        workerId: request.id,
        workerType,
        plugin: !!request.initData,
      })
    }

    const workerMode = request.workerMode ?? WebWorkerMode.WORKER_MODE_DEFAULT
    let shared: boolean
    if (this.forceDedicatedWorkers) {
      shared = false
    } else if (workerMode === WebWorkerMode.WORKER_MODE_DEDICATED) {
      shared = false
    } else if (workerMode === WebWorkerMode.WORKER_MODE_SHARED) {
      shared = true
    } else {
      // WORKER_MODE_DEFAULT: for plugin workers on Config B/C (SAB available),
      // use DedicatedWorker so the SAB bus can be wired for intra-tab IPC.
      // Non-plugin workers and Config A/F keep SharedWorker.
      if (plugin) {
        shared = detect.config !== 'B' && detect.config !== 'C'
      } else {
        shared = true
      }
    }

    this.notifyWebWorkerUpdated(
      request.id,
      false,
      shared,
      false,
      undefined,
      WebWorkerGenerationState.WORKER_REQUESTED,
    )

    markStartupBoundary('worker.create-dispatch-start', {
      source: 'browser',
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
      workerId: request.id,
      workerType,
      workerMode,
      shared,
      plugin,
      path: request.path,
      detectConfig: detect.config,
      activeWorkerCountBefore,
      replacedWorker,
    })
    const worker = new WebDocumentWebWorker(
      request.id,
      request.path,
      this.sharedWorkerPath,
      this.webDocumentUuid,
      request.initData,
      workerType,
      shared,
      this.onWebWorkerMessage.bind(this, request.id),
      detect,
      readRuntimeWasmEnv(),
    )
    this.webWorkers[request.id] = worker
    this.notifyResumeReadyClient(worker.port)

    const createdShared = worker.isShared
    this.notifyWebWorkerUpdated(
      request.id,
      false,
      createdShared,
      false,
      undefined,
      worker.generationState,
    )
    worker.setGenerationState(WebWorkerGenerationState.STARTUP_RUNNING)
    this.notifyWebWorkerUpdated(
      request.id,
      false,
      createdShared,
      false,
      undefined,
      worker.generationState,
    )
    markStartupBoundary('worker.create-ready', {
      source: 'browser',
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
      workerId: request.id,
      shared: createdShared,
      workerType,
      plugin: !!request.initData,
      activeWorkerCountBefore,
      activeWorkerCountAfter: Object.keys(this.webWorkers).length,
      replacedWorker,
    })
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
    const activeWorkerCountBefore = Object.keys(this.webWorkers).length
    markStartupBoundary('worker.remove-request-received', {
      source: 'browser',
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
      workerId: request.id,
      activeWorkerCountBefore,
      removed: !!old,
      shared: old?.isShared,
      ready: old?.ready,
    })
    if (old) {
      this.closeWorkerBridgeEndpoint(request.id)
      this.closeOpfsWorker(request.id)
      this.closeSabPairsForWorker(request.id, 'worker removed')
      old.setGenerationState(WebWorkerGenerationState.NORMAL_STOP)
      delete this.webWorkers[request.id]
      await old.close()
      this.notifyWebWorkerUpdated(
        request.id,
        true,
        old.isShared,
        old.ready,
        undefined,
        old.generationState,
      )
    }
    markStartupBoundary('worker.remove-ready', {
      source: 'browser',
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
      workerId: request.id,
      activeWorkerCountBefore,
      activeWorkerCountAfter: Object.keys(this.webWorkers).length,
      removed: !!old,
      shared: old?.isShared,
      ready: old?.ready,
    })
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
    this.sabPairBroker.closeAll()
    for (const [, worker] of this.opfsWorkers) {
      worker.terminate()
    }
    this.opfsWorkers.clear()
    if (this.runtimeOpfsBrokerPort) {
      try {
        this.runtimeOpfsBrokerPort.postMessage({
          from: this.webDocumentUuid,
          close: true,
        } satisfies WebDocumentToClient)
      } catch {
        // The runtime worker is already gone.
      }
      this.runtimeOpfsBrokerPort.onmessage = null
      this.runtimeOpfsBrokerPort.close()
      this.runtimeOpfsBrokerPort = undefined
    }

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
        })
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
    this.emit('closed', err)

    // Release Web Locks last, after all cleanup is done.
    this.releasePluginSingletonLock()
    if (this.abortController) {
      this.abortController.abort()
      this.abortController = undefined
    }
  }

  // initServiceWorker asynchronously initializes the service worker.
  // called in the constructor
  private async initServiceWorker(
    wb: Workbox,
    swUrl: string,
    onControlReady?: () => void,
  ) {
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
      if (!this.runtimeConnected) {
        this.taskEnsureWebRuntimeConn()
      }
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
    markStartupBoundary('service-worker.register-ready', {
      source: 'browser',
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
    })

    // wait for the service worker to finish startup
    // await wb.active()
    await wb.update()
    markStartupBoundary('service-worker.update-ready', {
      source: 'browser',
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
    })
    await registerUpdatedServiceWorker(swUrl, wbReg)

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
    markStartupBoundary('service-worker.control-ready', {
      source: 'browser',
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
    })
    onControlReady?.()
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
    failureReason?: string,
    generationState = WebWorkerGenerationState.UNKNOWN,
  ) {
    if (this.closed) {
      return
    }
    const failed = isFailedWorkerGenerationState(generationState)
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
          failed,
          failureReason,
          generationState,
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
    markStartupBoundary('service-worker.port-started', {
      source: 'browser',
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
    })
    const msg: WebDocumentToWorker = {
      from: this.webDocumentUuid,
      initPort: clientPort,
    }
    sw.postMessage(msg, [clientPort])
    this.notifyResumeReadyClient(localPort)
    markStartupBoundary('service-worker.port-sent', {
      source: 'browser',
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
    })
  }

  // openWebRuntimeClient attempts to open a message port with the WebRuntime.
  // this is the function passed to the WebRuntimeClient for the WebDocument
  private async openWebRuntimeClient(
    init: WebRuntimeClientInit,
  ): Promise<MessagePort> {
    const { port1: localPort, port2: remotePort } = new MessageChannel()
    markStartupBoundary('runtime.client-open-start', {
      source: 'browser',
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
      clientType: init.clientType,
    })
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
    markStartupBoundary('runtime.client-open-sent', {
      source: 'browser',
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
      from,
    })
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
      this.clearResumeReadyState('hidden')
    } else {
      console.log('WebDocument: document is visible')
      this.refreshPluginSingletonLock()
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
    if (!hidden) {
      if (!this.runtimeConnected) {
        this.taskEnsureWebRuntimeConn()
      }
      this.scheduleResumeReadySeed()
    }
  }

  private enablePluginSingletonLock() {
    this.pluginSingletonLockEnabled = true
    if (!shouldUseWebDocumentLivenessLock()) {
      // There is no timer fallback here: backgrounded tabs throttle timers, and
      // plugin startup needs a real cross-tab singleton owner.
      this.pluginSingletonReady = Promise.reject(
        new Error('Web Locks unavailable for dedicated plugin workers'),
      )
      this.pluginSingletonReady.catch(() => {})
      markStartupBoundary('singleton-lock.unavailable', {
        source: 'browser',
        documentId: this.webDocumentUuid,
        runtimeId: this.webRuntimeId,
      })
      return
    }
    this.refreshPluginSingletonLock()
  }

  private releasePluginSingletonLock() {
    if (this.singletonAbort) {
      this.singletonAbort.abort()
      this.singletonAbort = undefined
    }
    if (this.pluginSingletonLockEnabled) {
      this.pluginSingletonReady = new Promise<void>(() => {})
    }
  }

  private refreshPluginSingletonLock() {
    if (!this.pluginSingletonLockEnabled || this.closed) {
      return
    }
    if (this.singletonAbort) {
      return
    }

    this.singletonAbort = new AbortController()
    markStartupBoundary('singleton-lock.request-start', {
      source: 'browser',
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
    })
    this.pluginSingletonReady = new Promise<void>((resolve, reject) => {
      navigator.locks
        .request(
          `bldr-plugin-singleton-${this.webRuntimeId}`,
          { signal: this.singletonAbort!.signal },
          () => {
            console.log('WebDocument: acquired plugin singleton lock')
            markStartupBoundary('singleton-lock.acquired', {
              source: 'browser',
              documentId: this.webDocumentUuid,
              runtimeId: this.webRuntimeId,
            })
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

  private markPluginStartupBoundary(
    workerID: string,
    worker: WebDocumentWebWorker,
    label: string,
    reason?: string,
  ) {
    if (!worker.plugin) {
      return
    }
    markStartupBoundary(label, {
      source: 'browser',
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
      workerId: workerID,
      shared: worker.isShared,
      workerType: worker.workerType,
      plugin: true,
      reason,
    })
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
    if (data.startupMark) {
      markStartupBoundary(data.startupMark.label, {
        source: 'worker',
        documentId: this.webDocumentUuid,
        runtimeId: this.webRuntimeId,
        workerId: workerID,
        shared: worker.isShared,
        workerType: worker.workerType,
        plugin: worker.plugin,
        ...(data.startupMark.detail ?? {}),
        workerStartTimeMs: data.startupMark.startTimeMs ?? null,
      })
    }
    if (data.close) {
      // Web worker was closed / removed.
      const failureReason = data.failureReason
      const generationState = classifyWorkerCloseGenerationState(failureReason)
      worker.setGenerationState(generationState, failureReason)
      if (isFailedWorkerGenerationState(generationState)) {
        this.markPluginStartupBoundary(
          workerID,
          worker,
          'plugin.terminal-failure',
          failureReason,
        )
      }
      if (generationState === WebWorkerGenerationState.NORMAL_STOP) {
        this.markPluginStartupBoundary(
          workerID,
          worker,
          'plugin.normal-stop',
          failureReason,
        )
      }
      this.closeWorkerBridgeEndpoint(workerID)
      this.closeOpfsWorker(workerID)
      this.closeSabPairsForWorker(workerID, 'worker closed')
      worker.port.close()
      delete this.webWorkers[workerID]
      this.notifyWebWorkerUpdated(
        workerID,
        true,
        worker.isShared,
        worker.ready,
        failureReason,
        worker.generationState,
      )
      return
    }

    if (
      data.frontendReady &&
      !worker.ready &&
      advanceWorkerGenerationState(
        worker,
        WebWorkerGenerationState.FRONTEND_READY,
      )
    ) {
      this.notifyWebWorkerUpdated(
        workerID,
        false,
        worker.isShared,
        false,
        undefined,
        worker.generationState,
      )
      this.markPluginStartupBoundary(workerID, worker, 'plugin.frontend-ready')
    }

    if (data.capabilityReady && !worker.ready) {
      if (
        advanceWorkerGenerationState(
          worker,
          WebWorkerGenerationState.FRONTEND_READY,
        )
      ) {
        this.notifyWebWorkerUpdated(
          workerID,
          false,
          worker.isShared,
          false,
          undefined,
          worker.generationState,
        )
        this.markPluginStartupBoundary(
          workerID,
          worker,
          'plugin.frontend-ready',
        )
      }
      if (
        advanceWorkerGenerationState(
          worker,
          WebWorkerGenerationState.CAPABILITY_READY,
        )
      ) {
        this.notifyWebWorkerUpdated(
          workerID,
          false,
          worker.isShared,
          false,
          undefined,
          worker.generationState,
        )
        this.markPluginStartupBoundary(
          workerID,
          worker,
          'plugin.capability-ready',
        )
      }
    }

    if (data.ready && !worker.ready) {
      if (
        advanceWorkerGenerationState(
          worker,
          WebWorkerGenerationState.FRONTEND_READY,
        )
      ) {
        this.notifyWebWorkerUpdated(
          workerID,
          false,
          worker.isShared,
          false,
          undefined,
          worker.generationState,
        )
        this.markPluginStartupBoundary(
          workerID,
          worker,
          'plugin.frontend-ready',
        )
      }
      if (
        advanceWorkerGenerationState(
          worker,
          WebWorkerGenerationState.CAPABILITY_READY,
        )
      ) {
        this.notifyWebWorkerUpdated(
          workerID,
          false,
          worker.isShared,
          false,
          undefined,
          worker.generationState,
        )
        this.markPluginStartupBoundary(
          workerID,
          worker,
          'plugin.capability-ready',
        )
      }
      advanceWorkerGenerationState(worker, WebWorkerGenerationState.RUNNING)
      worker.ready = true
      if (!this.firstWorkerReadyMarked) {
        this.firstWorkerReadyMarked = true
        markStartupBoundary('worker.first-ready', {
          source: 'browser',
          documentId: this.webDocumentUuid,
          runtimeId: this.webRuntimeId,
          workerId: workerID,
          shared: worker.isShared,
          workerType: worker.workerType,
          plugin: worker.plugin,
        })
      }
      markStartupBoundary('worker.ready', {
        source: 'browser',
        documentId: this.webDocumentUuid,
        runtimeId: this.webRuntimeId,
        workerId: workerID,
        shared: worker.isShared,
        workerType: worker.workerType,
        plugin: worker.plugin,
      })
      if (worker.plugin) {
        markStartupBoundary('plugin.running', {
          source: 'browser',
          documentId: this.webDocumentUuid,
          runtimeId: this.webRuntimeId,
          workerId: workerID,
          shared: worker.isShared,
          workerType: worker.workerType,
          plugin: true,
        })
      }
      this.notifyWebWorkerUpdated(
        workerID,
        false,
        worker.isShared,
        true,
        undefined,
        worker.generationState,
      )
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
      markStartupBoundary('runtime.client-connect-request', {
        source: 'browser',
        documentId: this.webDocumentUuid,
        runtimeId: this.webRuntimeId,
        from: data.from,
      })
      this.handleClientConnectWebRuntime(
        data.from,
        connectWebRuntime.init,
        port,
      )
    }

    if (data.connectWebRtcBridge) {
      this.handleConnectWebRtcBridge(data.from)
    }

    if (data.openOpfsWorker) {
      void this.handleOpenOpfsWorker(data.from)
    }

    if (data.openSabPair) {
      this.handleOpenSabPair(data.from, data.openSabPair)
    }

    if (data.closeSabPair) {
      const pair = this.sabPairBroker.closePairForWorker(
        data.from,
        data.closeSabPair.pairId,
      )
      if (pair) {
        this.notifySabPairClosed(pair, data.from, 'stream closed')
      }
    }
  }

  private sendOpenSabPairError(
    sourceWorkerId: string,
    requestId: string,
    error: string,
  ): void {
    const sourceWorker = this.webWorkers[sourceWorkerId]
    if (!sourceWorker?.port) {
      return
    }
    const ack: OpenSabPairAck = {
      from: this.webDocumentUuid,
      requestId,
      error,
    }
    try {
      sourceWorker.port.postMessage({
        from: this.webDocumentUuid,
        openSabPairAck: ack,
      } satisfies WebDocumentToClient)
    } catch {
      // The requester is already closed. Pair metadata cleanup is handled by caller.
    }
  }

  private handleOpenSabPair(
    sourceWorkerId: string,
    request: { requestId: string; targetWorkerId: string },
  ): void {
    const sourceWorker = this.webWorkers[sourceWorkerId]
    if (!sourceWorker?.port) {
      return
    }
    const targetWorker = this.webWorkers[request.targetWorkerId]
    if (!targetWorker?.port) {
      this.sendOpenSabPairError(
        sourceWorkerId,
        request.requestId,
        `target worker not found: ${request.targetWorkerId}`,
      )
      return
    }

    let pairId: string | undefined
    try {
      const pair = this.sabPairBroker.allocate(
        sourceWorkerId,
        request.targetWorkerId,
      )
      pairId = pair.pairId
      const { aSab, bSab } = createSabPair()
      const sourceEndpoint: SabPairEndpointDescriptor = {
        pairId: pair.pairId,
        localWorkerId: sourceWorkerId,
        remoteWorkerId: request.targetWorkerId,
        txSab: aSab,
        rxSab: bSab,
        mtuBytes: SAB_PAIR_DIRECTION_MTU_BYTES,
      }
      const targetEndpoint: SabPairEndpointDescriptor = {
        pairId: pair.pairId,
        localWorkerId: request.targetWorkerId,
        remoteWorkerId: sourceWorkerId,
        txSab: bSab,
        rxSab: aSab,
        mtuBytes: SAB_PAIR_DIRECTION_MTU_BYTES,
      }
      targetWorker.port.postMessage({
        from: this.webDocumentUuid,
        sabPairEndpoint: targetEndpoint,
      } satisfies WebDocumentToClient)
      const ack: OpenSabPairAck = {
        from: this.webDocumentUuid,
        requestId: request.requestId,
        endpoint: sourceEndpoint,
      }
      sourceWorker.port.postMessage({
        from: this.webDocumentUuid,
        openSabPairAck: ack,
      } satisfies WebDocumentToClient)
      this.sabPairBroker.markOpen(pair.pairId)
    } catch (err) {
      if (pairId) {
        this.sabPairBroker.closePair(pairId)
      }
      this.sendOpenSabPairError(
        sourceWorkerId,
        request.requestId,
        err instanceof Error ? err.message : String(err),
      )
    }
  }

  private closeSabPairsForWorker(workerId: string, reason: string): void {
    const pairs = this.sabPairBroker.closeForWorker(workerId)
    for (const pair of pairs) {
      this.notifySabPairClosed(pair, workerId, reason)
    }
  }

  private notifySabPairClosed(
    pair: { pairId: string; workerAId: string; workerBId: string },
    closedByWorkerId: string,
    reason: string,
  ): void {
    const remoteWorkerId =
      pair.workerAId === closedByWorkerId ? pair.workerBId : pair.workerAId
    const remoteWorker = this.webWorkers[remoteWorkerId]
    if (!remoteWorker?.port) {
      return
    }
    remoteWorker.port.postMessage({
      from: this.webDocumentUuid,
      sabPairClosed: {
        pairId: pair.pairId,
        reason,
      },
    } satisfies WebDocumentToClient)
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

  // closeOpfsWorker terminates the OPFS protocol worker associated with a WebWorker.
  private closeOpfsWorker(workerId: string) {
    const worker = this.opfsWorkers.get(workerId)
    if (worker) {
      worker.terminate()
      this.opfsWorkers.delete(workerId)
    }
  }

  private sendOpenOpfsWorkerAck(
    from: string,
    ack: OpenOpfsWorkerAck,
    port?: MessagePort,
  ) {
    const worker = this.webWorkers[from]
    if (!worker?.port) {
      if (port) {
        port.close()
      }
      return
    }
    const msg = {
      from: this.webDocumentUuid,
      openOpfsWorkerAck: ack,
    } satisfies WebDocumentToClient
    if (port) {
      worker.port.postMessage(msg, [port])
    } else {
      worker.port.postMessage(msg)
    }
  }

  private sendOpfsWorkerClosed(from: string): void {
    const worker = this.webWorkers[from]
    if (!worker?.port) {
      return
    }
    worker.port.postMessage({
      from: this.webDocumentUuid,
      opfsWorkerClosed: true,
    } satisfies WebDocumentToClient)
  }

  private sendOpenOpfsWorkerError(from: string, error: string): void {
    try {
      this.sendOpenOpfsWorkerAck(from, {
        from: this.webDocumentUuid,
        error,
      })
    } catch {
      // The requester is already closed.
    }
  }

  // handleOpenOpfsWorker creates a raw DedicatedWorker OPFS bridge for a plugin
  // worker and sends its client MessagePort back over the plugin's port.
  private async handleOpenOpfsWorker(from: string) {
    const requester = this.webWorkers[from]
    if (!requester?.port) {
      console.warn(
        `WebDocument: OPFS worker request from unknown worker: ${from}`,
      )
      return
    }

    try {
      const clientPort = await this.openOpfsBridgeWorker(from, () => {
        this.sendOpfsWorkerClosed(from)
      })
      if (!clientPort) {
        return
      }
      this.sendOpenOpfsWorkerAck(
        from,
        { from: this.webDocumentUuid },
        clientPort,
      )
      console.log(`WebDocument: OPFS worker opened for ${from}`)
    } catch (err) {
      this.sendOpenOpfsWorkerError(
        from,
        err instanceof Error ? err.message : String(err),
      )
    }
  }

  // openOpfsBridgeWorker creates a DedicatedWorker OPFS bridge keyed by
  // requesterId, waits for it to report ready, and returns the client
  // MessagePort. After startup, a worker error tears the worker down and invokes
  // onWorkerDied so the requester can re-host (the dead worker leaves the
  // transferred client port with no owner, hanging in-flight bridge requests).
  // Returns null when a concurrent request already superseded this worker (the
  // superseding request owns the response). Throws when the worker fails to
  // start.
  private async openOpfsBridgeWorker(
    requesterId: string,
    onWorkerDied: () => void,
  ): Promise<MessagePort | null> {
    this.closeOpfsWorker(requesterId)

    let opfsWorker: Worker | undefined
    let clientPort: MessagePort | undefined
    try {
      const worker = new Worker(buildOpfsWorkerURL(this.opfsWorkerPath), {
        name: `${requesterId}-opfs`,
        type: 'module',
      })
      opfsWorker = worker
      const reportWorkerError = (ev: ErrorEvent) => {
        const stack =
          ev.error instanceof Error && ev.error.stack
            ? `\n${ev.error.stack}`
            : ''
        console.error(
          `opfs worker ${requesterId}: error: ${ev.message} at ${ev.filename}:${ev.lineno}:${ev.colno}${stack}`,
        )
      }
      worker.onerror = reportWorkerError
      const { port1: workerPort, port2 } = new MessageChannel()
      clientPort = port2
      const ready = new Promise<void>((resolve, reject) => {
        const timeout = globalThis.setTimeout(() => {
          cleanup()
          reject(new Error(`OPFS worker ${requesterId} did not become ready`))
        }, opfsWorkerStartupTimeoutMs)
        function cleanup() {
          globalThis.clearTimeout(timeout)
          clientPort?.removeEventListener('message', onMessage)
        }
        function onMessage(ev: MessageEvent<unknown>) {
          if (!isOpfsWorkerReadyMessage(ev.data)) {
            return
          }
          cleanup()
          resolve()
        }
        worker.onerror = (ev: ErrorEvent) => {
          reportWorkerError(ev)
          cleanup()
          reject(
            new Error(
              ev.message || `OPFS worker ${requesterId} failed to start`,
            ),
          )
        }
        clientPort?.addEventListener('message', onMessage)
        clientPort?.start()
      })
      this.opfsWorkers.set(requesterId, worker)
      worker.postMessage(
        {
          from: this.webDocumentUuid,
          initPort: workerPort,
        } satisfies WebDocumentToWorker,
        [workerPort],
      )
      await ready
      if (this.opfsWorkers.get(requesterId) !== worker) {
        clientPort.close()
        return null
      }
      // A post-startup OPFS worker error leaves the transferred client port with
      // no live owner. Tear the dead worker down and notify the requester to
      // re-host. Guard on identity so a worker already replaced by a re-host does
      // not trigger a second teardown.
      worker.onerror = (ev: ErrorEvent) => {
        reportWorkerError(ev)
        if (this.opfsWorkers.get(requesterId) !== worker) {
          return
        }
        this.closeOpfsWorker(requesterId)
        onWorkerDied()
      }
      const readyPort = clientPort
      clientPort = undefined
      return readyPort
    } catch (err) {
      if (clientPort) {
        clientPort.close()
      }
      if (opfsWorker) {
        opfsWorker.terminate()
      }
      this.opfsWorkers.delete(requesterId)
      throw err
    }
  }

  // onRuntimeOpfsBrokerMessage serves OPFS bridge requests from the engine
  // runtime SharedWorker over the broker port. Unlike the plugin path, the
  // requester is the runtime worker (not a registered WebWorker), so the ack and
  // worker-death notice ride the broker port directly.
  private onRuntimeOpfsBrokerMessage(
    brokerPort: MessagePort,
    event: MessageEvent<ClientToWebDocument>,
  ) {
    const data = event.data
    if (!data || !data.from || !data.openOpfsWorker) {
      return
    }
    const requesterId = data.from
    void this.serveRuntimeOpfsRequest(requesterId, brokerPort)
  }

  private async serveRuntimeOpfsRequest(
    requesterId: string,
    brokerPort: MessagePort,
  ) {
    const postAck = (ack: OpenOpfsWorkerAck, port?: MessagePort): boolean => {
      const msg = {
        from: this.webDocumentUuid,
        openOpfsWorkerAck: ack,
      } satisfies WebDocumentToClient
      try {
        brokerPort.postMessage(msg, port ? [port] : [])
        return true
      } catch {
        port?.close()
        return false
      }
    }
    try {
      const clientPort = await this.openOpfsBridgeWorker(requesterId, () => {
        try {
          brokerPort.postMessage({
            from: this.webDocumentUuid,
            opfsWorkerClosed: true,
          } satisfies WebDocumentToClient)
        } catch {
          // The runtime worker or this document is already gone.
        }
      })
      if (!clientPort) {
        return
      }
      if (!postAck({ from: this.webDocumentUuid }, clientPort)) {
        // The broker port closed before the runtime received the bridge port, so
        // it never got a usable OPFS worker. Tear down the worker we just created
        // instead of leaking it, and do not emit a false readiness mark.
        this.closeOpfsWorker(requesterId)
        return
      }
      markStartupBoundary('runtime.opfs-bridge-ready', {
        source: 'browser',
        documentId: this.webDocumentUuid,
        runtimeId: this.webRuntimeId,
        workerId: requesterId,
        enabled: true,
      })
      console.log(`WebDocument: runtime OPFS worker opened for ${requesterId}`)
    } catch (err) {
      postAck({
        from: this.webDocumentUuid,
        error: err instanceof Error ? err.message : String(err),
      })
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
    markStartupBoundary('runtime.client-connect-start', {
      source: 'browser',
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
      from,
    })
    port.start()

    const { port1: clientPort, port2: webRuntimePort } = new MessageChannel()
    try {
      this.sendWebRuntimeOpenClient(from, init, webRuntimePort)
    } catch (err) {
      clientPort.close()
      webRuntimePort.close()
      const message = err instanceof Error ? err.message : String(err)
      const ack: ConnectWebRuntimeAck = {
        from: this.webDocumentUuid,
        error: message,
      }
      port.postMessage(ack)
      port.close()
      return
    }

    // Ack only after forwarding the remote port. Otherwise the caller can get a
    // client port that no WebRuntime will ever open, with no timeout to rescue it.
    const ack: ConnectWebRuntimeAck = {
      from: this.webDocumentUuid,
      webRuntimePort: clientPort,
    }
    port.postMessage(ack, [clientPort])
    port.close()
    markStartupBoundary('runtime.client-connect-ack', {
      source: 'browser',
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
      from,
    })
  }

  // taskEnsureWebRuntimeConn ensures an active connection with the WebRuntime.
  private taskEnsureWebRuntimeConn() {
    queueMicrotask(() => {
      if (this.closed) {
        return
      }
      this.webRuntimeClient.waitConn().then(
        () => {
          if (this.closed) {
            return
          }
          markStartupBoundary('runtime.connected', {
            source: 'browser',
            documentId: this.webDocumentUuid,
            runtimeId: this.webRuntimeId,
          })
          this.runtimeConnected = true
          this.scheduleResumeReadySeed()
          this.emit('runtimeconnected')
        },
        (err) => {
          if (this.closed) return
          console.warn('WebDocument: failed to connect to WebRuntime', err)
          this.runtimeConnected = false
          this.clearResumeReadyState('runtime-connect-failed')
        },
      )
    })
  }

  // handleWebRuntimeClientDisconnected handles if the WebRuntimeClient disconnects.
  private async handleWebRuntimeClientDisconnected() {
    if (this.closed) {
      return
    }
    this.runtimeConnected = false
    this.clearResumeReadyState('runtime-disconnected')
    this.taskEnsureWebRuntimeConn()
  }

  private scheduleResumeReadySeed() {
    if (
      this.resumeReady ||
      this.resumeReadyPending ||
      this.closed ||
      this.hidden ||
      !this.runtimeConnected
    ) {
      return
    }

    this.resumeReadyPending = true
    this.afterForegroundFrame(() => {
      if (this.shouldAbortResumeReadySeed()) {
        this.resumeReadyPending = false
        return
      }
      this.afterForegroundFrame(() => {
        this.resumeReadyPending = false
        if (this.shouldAbortResumeReadySeed()) {
          return
        }
        this.seedResumeReadyState()
      })
    })
  }

  private shouldAbortResumeReadySeed(): boolean {
    return (
      this.resumeReady || !!this.closed || this.hidden || !this.runtimeConnected
    )
  }

  private afterForegroundFrame(cb: () => void) {
    if (typeof globalThis.requestAnimationFrame === 'function') {
      globalThis.requestAnimationFrame(() => cb())
      return
    }
    globalThis.setTimeout(cb, 0)
  }

  private seedResumeReadyState() {
    if (this.resumeReady) {
      return
    }
    this.resumeReady = true
    this.resumeReadySequence++

    const state: WebDocumentResumeReadyState = {
      ready: true,
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
      hidden: false,
      sequence: this.resumeReadySequence,
      focused:
        typeof document !== 'undefined' &&
        typeof document.hasFocus === 'function'
          ? document.hasFocus()
          : undefined,
      visibilityState:
        typeof document !== 'undefined' ? document.visibilityState : undefined,
      timestampMs:
        typeof performance !== 'undefined' &&
        typeof performance.now === 'function'
          ? performance.now()
          : undefined,
    }
    globalThis.__swWebDocumentResumeReady = state
    markStartupBoundary('web-document.resume-ready', {
      source: 'browser',
      documentId: state.documentId,
      runtimeId: state.runtimeId,
      sequence: state.sequence,
      focused: state.focused,
      visibilityState: state.visibilityState,
    })
    this.emit('resumeready')
    this.notifyResumeReadyClients()
  }

  private clearResumeReadyState(reason: string) {
    if (!this.resumeReady && !this.resumeReadyPending) {
      return
    }
    this.resumeReady = false
    this.resumeReadyPending = false
    if (
      globalThis.__swWebDocumentResumeReady?.documentId === this.webDocumentUuid
    ) {
      globalThis.__swWebDocumentResumeReady = undefined
    }
    markStartupBoundary('web-document.resume-not-ready', {
      source: 'browser',
      documentId: this.webDocumentUuid,
      runtimeId: this.webRuntimeId,
      sequence: this.resumeReadySequence,
      reason,
    })
    this.notifyResumeReadyClients()
  }

  private notifyResumeReadyClients() {
    if (this.serviceWorkerPort) {
      this.notifyResumeReadyClient(this.serviceWorkerPort)
    }
    for (const workerId in this.webWorkers) {
      this.notifyResumeReadyClient(this.webWorkers[workerId].port)
    }
  }

  private notifyResumeReadyClient(port: MessagePort) {
    const msg: WebDocumentToClient = {
      from: this.webDocumentUuid,
      resumeReady: this.resumeReady,
    }
    port.postMessage(msg)
  }

  private async waitForResumeReady(): Promise<RuntimeClientStreamOpenGateResult> {
    if (this.resumeReady) {
      return {
        state: 'ready',
        documentId: this.webDocumentUuid,
      }
    }
    return new Promise<RuntimeClientStreamOpenGateResult>((resolve) => {
      const onReady = () => {
        this.removeListener('closed', onClosed)
        this.removeListener('resumeready', onReady)
        resolve({
          state: 'ready',
          documentId: this.webDocumentUuid,
        })
      }
      const onClosed = (err?: Error) => {
        this.removeListener('resumeready', onReady)
        this.removeListener('closed', onClosed)
        resolve({
          state: 'closed',
          documentId: this.webDocumentUuid,
          reason: err?.message ?? 'web document is closed',
        })
      }
      this.on('resumeready', onReady)
      this.on('closed', onClosed)
      if (this.closed) {
        onClosed(
          this.closed instanceof Error
            ? this.closed
            : new Error('web document is closed'),
        )
      }
    })
  }
}
