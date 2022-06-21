import type { Source } from 'it-stream-types'
import {
  MessagePortConn,
  Client,
  Server,
  createMux,
  createHandler,
} from 'starpc'
import { Observable } from 'rxjs'
import { pushable } from 'it-pushable'
import { abortableSource } from 'abortable-iterator'

import {
  WebInitRuntime,
  WebViewStatus,
  WebRuntimeDefinition,
  WebRuntime,
  WebStatus,
  WatchWebStatusRequest,
  CreateWebViewRequest,
  CreateWebViewResponse,
  WebViewRpcPacket,
} from '../runtime/runtime'
import { isElectron, setElectronPort } from './electron'
import { LeaderElect } from './leader-elect'
import { addShutdownCallback, DisposeCallback } from './shutdown'
import { detectWasmSupported } from './wasm-detect'
import { WebView, WebViewRegistration, buildWebViewStatus } from './web-view'

// workerWebStatusKey is the key used to store the worker WebStatus snapshot.
const workerWebStatusKey = 'web-status'

// CreateWebViewCallback is a callback to create a new web view when requested.
// Throws an error if unable to create the web view.
export type CreateWebViewCallback = (webViewID: string) => Promise<void>

// ReadyCallback is a callback indicating the runtime ready state changed.
export type ReadyCallback = (runtimeReady: boolean) => void

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
  private releaseShutdownCallback: DisposeCallback

  // leaderElect manages leader election and participant tracking.
  private leaderElect: LeaderElect
  // ready indicates the runtime is ready to use.
  // fires an event 'ready' when ready and 'unready' when unready.
  private ready: boolean

  // webViews contains the list of associated web views by ID.
  private webViews: { [id: string]: WebView }
  // webStatusStream contains the stream of WebStatus events.
  // emits an initial snapshot followed by update messages.
  public readonly webStatusStream: Observable<WebStatus>
  // _webStatusUpdates is a stream of web status updates.
  private readonly webStatusUpdates: Source<WebStatus>
  // _webStatusUpdates contains push + end for webStatusUpdates
  private readonly _webStatusUpdates: {
    push: (val: WebStatus) => void
    end: (err?: Error) => void
  }

  // workerRunning indicates we should run the worker.
  // controlled by leader election
  private workerRunning: boolean
  // worker is the loaded runtime worker
  // unset until this is the leader tab
  private worker?: Worker

  // client is the RPC client for the WebRuntime.
  private client: Client
  // server is the RPC server for the WebRuntime.
  private server: Server
  // runtimeConn is the multiplexed connection to the Runtime.
  // not set until the runtime is initialized (and we are leader).
  private runtimeConn?: MessagePortConn

  constructor(runtimeId?: string, createWebViewCb?: CreateWebViewCallback) {
    super()

    if (!runtimeId) {
      runtimeId = 'default'
    }
    this.runtimeId = runtimeId
    this.webRuntimeUuid = Math.random().toString(36).substr(2, 9)
    this.workerRunning = false
    if (isElectron) {
      this.isElectron = true
    }
    this.ready = false
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

    // Setup the status observable.
    const webStatusUpdates = pushable<WebStatus>({ objectMode: true })
    this.webStatusUpdates = webStatusUpdates
    this._webStatusUpdates = webStatusUpdates
    this.webStatusStream = this._buildWebStatusStream()

    // Setup the RPC server for this WebRuntime.
    const mux = createMux()
    const webRuntime: WebRuntime = new RuntimeServer(
      this,
      createWebViewCb || null
    )
    mux.register(createHandler(WebRuntimeDefinition, webRuntime))
    this.server = new Server(mux)
    this.client = new Client()

    // add a global shutdown callback to terminate this
    this.releaseShutdownCallback = addShutdownCallback(this.close.bind(this))
  }

  // registerWebView registers a web-view with the runtime.
  public registerWebView(webView: WebView): WebViewRegistration {
    const webViewId = webView.getWebViewUuid()
    console.log('runtime: register web view with id ' + webViewId)
    this.webViews[webViewId] = webView
    this.storeWebStatusSnapshot()
    this.notifyWebViewUpdated(webViewId, webView)

    return {
      release: () => {
        this.unregisterWebView(webView)
      },
    } as WebViewRegistration
  }

  // isLeader checks if the local worker is leader.
  public get isLeader(): boolean {
    return this.leaderElect.isLeader
  }

  // isReady checks if the runtime is ready to use.
  public get isReady(): boolean {
    return this.ready
  }

  // buildWebStatusSnapshot builds a snapshot of the status.
  // if allWorkers is set, includes web views from other active workers.
  // prevents duplicate web view entries
  public async buildWebStatusSnapshot(allWorkers: boolean): Promise<WebStatus> {
    let webViews: WebViewStatus[] = []
    let webViewIdxs: { [id: string]: Number } = {}
    for (const webViewId in this.webViews) {
      const webView = this.webViews[webViewId]
      if (webViewId && webView) {
        webViewIdxs[webViewId] = webViews.length
        webViews.push(buildWebViewStatus(webViewId, webView))
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
    this.ready = false
    if (this.leaderElect) {
      this.leaderElect.close()
    }
    if (this.workerRunning) {
      this.shutdownWorker()
    }
    if (this.releaseShutdownCallback) {
      this.releaseShutdownCallback()
    }
    if (this._webStatusUpdates) {
      this._webStatusUpdates.end()
    }
  }

  // setReady updates the ready field.
  private setReady(isReady: boolean) {
    isReady = !!isReady
    if (isReady == this.ready) {
      return
    }

    this.ready = isReady
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
    const leaderReady = !!leaderID
    if (!leaderReady) {
      this.setReady(false)
    }
    if (isUs) {
      if (!this.workerRunning) {
        this.launchWorker()
      }
    } else {
      if (this.workerRunning) {
        this.shutdownWorker()
      }
      if (leaderReady) {
        // ???: possible race condition: service worker may not yet be ready
        // possible fix: query service worker ready via BroadcastChannel
        this.setReady(true)
      }
    }
  }

  // onWorkerAnnounce is called when a remote worker is added.
  private async onWorkerAnnounce(webRuntimeUuid: string, removed: boolean) {
    if (removed) {
      await this.onWorkerRemoved(webRuntimeUuid)
      return
    }
    if (!this.isLeader) {
      return
    }

    // write the initial worker web views status to the runtime
    const workerWebStatus = await this.loadWebStatusSnapshot(webRuntimeUuid)
    if (
      workerWebStatus &&
      workerWebStatus.webViews &&
      workerWebStatus.webViews.length
    ) {
      /* TODO
      this.writeRuntimeMessage({
        messageType: WebToRuntimeType.WebToRuntimeType_WEB_STATUS,
        webStatus: {
          snapshot: false,
          webViews: workerWebStatus.webViews,
        },
      })
      */
    }
  }

  // onWorkerRemoved is called when a remote worker is removed.
  private async onWorkerRemoved(webRuntimeUuid: string) {
    if (!this.isLeader) {
      return
    }

    // load the final worker web status snapshot
    const workerWebStatus = await this.loadWebStatusSnapshot(webRuntimeUuid)
    if (!workerWebStatus) {
      return
    }

    // broadcast removal of web views for worker
    for (const webView of workerWebStatus.webViews) {
      this.notifyWebViewUpdated(webView.id, undefined)
    }
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
    this._webStatusUpdates.push(webStatus)
  }

  // unregisterWebView removes the web-view and notifies the runtime if necessary.
  private unregisterWebView(webView: WebView) {
    const webViewId = webView?.getWebViewUuid()
    if (webViewId && this.webViews[webViewId] == webView) {
      delete this.webViews[webViewId]
      /* TODO
      this.writeRuntimeMessage({
        messageType: WebToRuntimeType.WebToRuntimeType_WEB_STATUS,
        webStatus: {
          snapshot: false,
          webViews: [buildWebViewStatus(webViewId, undefined)],
        },
      })
      */
    }
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

    // build the message channel
    const messageChannel = new MessageChannel()
    const ourPort = messageChannel.port1
    const workerPort = messageChannel.port2

    // setup the service worker
    // NOTE: if not in /, requires the Service-Worker-Allowed: '/' header
    // NOTE: scope controls which /pages/ are covered by the worker
    const serviceWorker = await navigator.serviceWorker.register('/sw.js', {
      scope: '/',
    })
    await serviceWorker.update()

    // setup the Conn to the runtime.
    this.runtimeConn = new MessagePortConn(ourPort, this.server)

    // start the flow of incoming messages
    ourPort.start()

    // setup the workers
    if (this.isElectron) {
      // eslint-disable-next-line
      console.log('starting electron webview')
      // setup the forwarding to ipc
      setElectronPort(workerPort)
    } else {
      // eslint-disable-next-line
      console.log('starting runtime worker')

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

    // set the conn on the client
    this.client.setOpenConnFn(this.runtimeConn.buildOpenStreamFunc())

    // indicate this runtime is ready to use.
    this.setReady(true)
  }

  // shutdownWorker shuts down the webworker.
  private async shutdownWorker() {
    this.workerRunning = false
    if (this.worker) {
      this.worker.terminate()
      this.worker = undefined
    }
    if (this.runtimeConn) {
      this.runtimeConn = undefined
      this.client.setOpenConnFn(undefined)
    }
  }

  // storeWebStatusSnapshot stores a web status snapshot in indexeddb.
  private async storeWebStatusSnapshot() {
    this.leaderElect.setWorkerKey(
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

  // _buildWebStatusStream builds the Observable for web status events.
  private _buildWebStatusStream(): Observable<WebStatus> {
    return new Observable<WebStatus>((subscriber) => {
      if (!this.isLeader) {
        subscriber.error(new Error('web runtime is not leader'))
        return
      }

      const abortController = new AbortController()
      ;(async () => {
        const snapshot = await this.buildWebStatusSnapshot(true)
        subscriber.next({
          snapshot: true,
          webViews: snapshot.webViews,
        })

        const webStatusUpdatesSource = abortableSource(
          this.webStatusUpdates,
          abortController.signal
        )
        for await (const webStatus of webStatusUpdatesSource) {
          subscriber.next(webStatus)
        }
      })().catch(subscriber.error.bind(subscriber))

      // return teardown logic
      return () => {
        abortController.abort()
      }
    })
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
  constructor(
    private runtime: Runtime,
    private createWebViewCb: CreateWebViewCallback | null
  ) {}

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
  ): Observable<WebStatus> {
    return this.runtime.webStatusStream
  }

  // WebViewRpc opens a stream for a RPC call for a WebView.
  public WebViewRpc(
    _request: Observable<WebViewRpcPacket>
  ): Observable<WebViewRpcPacket> {
    // TODO
    throw new Error('Method not implemented.')
  }
}
