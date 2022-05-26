import {
  RuntimeToWeb,
  WebInitRuntime,
  RuntimeToWebType,
  WebToRuntime,
  WebToRuntimeType,
  QueryWebStatus,
  WebStatus,
  WebViewStatus,
  RemoveView,
  CreateView,
} from '../runtime/runtime'
import { decodeUint32Le, prependPacketLen } from './binary'
import { Channel } from './channel'
import { isElectron, forwardElectronIPC } from './electron'
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
// with it over IPC (usually BroadcastChannel).
//
// There can be multiple Runtime in a page, although it best to have 1 Runtime
// per HTML Document.
//
// Events:
//  - ready: fired when the runtime becomes ready.
//  - unready: fired when the runtime becomes not ready.
export class Runtime extends EventTarget {
  // runtimeId is the ID of the Go Runtime, same across all tabs.
  // there can be a single Go runtime with multiple TS Runtimes.
  private runtimeId: string
  // createWebViewCb is called when the runtime requests to create a new web
  // view at the root level.
  private createWebViewCb?: CreateWebViewCallback
  // workerUuid is the unique id of this instance & attached worker.
  // this ID identifies this TypeScript Runtime class object.
  private workerUuid: string
  // leaderElect manages leader election and participant tracking.
  private leaderElect: LeaderElect
  // useWasm indicates if web assembly is available.
  private useWasm?: boolean
  // isElectron indicates this is electron and we will use ipcRenderer.
  private isElectron?: boolean
  // releaseShutdownCallback removes the callback handler for onunload.
  private releaseShutdownCallback: DisposeCallback
  // ready indicates the runtime is ready to use.
  // fires an event 'ready' when ready and 'unready' when unready.
  private ready: boolean
  // workerRunning indicates we should run the worker.
  // controlled by leader election
  private workerRunning: boolean
  // worker is the loaded runtime worker
  // unset until this is the leader tab
  private worker?: Worker
  // topicPrefix is the prefix to use for broadcast channels.
  private topicPrefix: string
  // runtimeCh is the two-way channel to the runtime worker(s).
  private runtimeCh?: Channel
  // webViews contains the list of associated web views by ID.
  private webViews: { [id: string]: WebView }

  constructor(runtimeId?: string, createWebViewCb?: CreateWebViewCallback) {
    super()
    if (!runtimeId) {
      runtimeId = 'default'
    }
    this.runtimeId = runtimeId
    this.createWebViewCb = createWebViewCb
    this.workerUuid = Math.random().toString(36).substr(2, 9)
    this.workerRunning = false
    if (isElectron) {
      this.isElectron = true
    }
    this.ready = false
    this.webViews = {}

    // Setup the leader election
    const electionUuid = 'r/' + this.runtimeId
    this.leaderElect = new LeaderElect(
      electionUuid,
      this.workerUuid,
      this.onLeaderChanged.bind(this),
      this.onWorkerAnnounce.bind(this)
    )

    // add a global shutdown callback to terminate this
    this.releaseShutdownCallback = addShutdownCallback(this.dispose.bind(this))

    // Detect if we can use WebAssembly.
    this.useWasm = detectWasmSupported()
    if (!this.useWasm) {
      console.log('WebAssembly is not supported in this browser')
    }

    // Build the runtime communication channels.
    this.topicPrefix = 'bldr/' + runtimeId
    const txID = `${this.topicPrefix}/r`
    const rxID = `${this.topicPrefix}/w`
    this.runtimeCh = new Channel(
      txID,
      rxID,
      this.handleRuntimeMessage.bind(this)
    )
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

  // dispose shuts down the runtime.
  public dispose() {
    this.ready = false
    if (this.leaderElect) {
      this.leaderElect.close()
    }
    if (this.workerRunning) {
      this.shutdownWorker()
    }
    if (this.runtimeCh) {
      this.runtimeCh.close()
      delete this.runtimeCh
    }
    if (this.releaseShutdownCallback) {
      this.releaseShutdownCallback()
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
  private async onLeaderChanged(_leaderID: string, isUs: boolean) {
    if (isUs) {
      if (!this.workerRunning) {
        this.launchWorker()
      }
    } else {
      if (this.workerRunning) {
        this.shutdownWorker()
      }
    }
  }

  // onWorkerAnnounce is called when a remote worker is added.
  private async onWorkerAnnounce(workerUuid: string, removed: boolean) {
    if (removed) {
      await this.onWorkerRemoved(workerUuid)
      return
    }
    if (!this.isLeader) {
      return
    }

    // write the initial worker web views status to the runtime
    const workerWebStatus = await this.loadWebStatusSnapshot(workerUuid)
    if (
      workerWebStatus &&
      workerWebStatus.webViews &&
      workerWebStatus.webViews.length
    ) {
      this.writeRuntimeMessage({
        messageType: WebToRuntimeType.WebToRuntimeType_WEB_STATUS,
        webStatus: {
          snapshot: false,
          webViews: workerWebStatus.webViews,
        },
      })
    }
  }

  // onWorkerRemoved is called when a remote worker is removed.
  private async onWorkerRemoved(workerUuid: string) {
    if (!this.isLeader) {
      return
    }

    // load the final worker web status snapshot
    const workerWebStatus = await this.loadWebStatusSnapshot(workerUuid)
    if (!workerWebStatus) {
      return
    }

    // broadcast removal of web views for worker
    for (const webView of workerWebStatus.webViews) {
      this.notifyWebViewUpdated(webView.id, undefined)
    }
  }

  // notifyWebViewUpdated sends a message to the runtime with the WebView update.
  // if the web view is null, sends a message indicating the view was removed.
  private notifyWebViewUpdated(webViewId: string, webView?: WebView) {
    const runtimeCh = this.runtimeCh
    if (!runtimeCh || !webViewId) {
      return
    }

    const msg = {
      messageType: WebToRuntimeType.WebToRuntimeType_WEB_STATUS,
      webStatus: {
        snapshot: false,
        webViews: [buildWebViewStatus(webViewId, webView)],
      },
    }
    this.writeRuntimeMessage(msg)
  }

  // unregisterWebView removes the web-view and notifies the runtime if necessary.
  private unregisterWebView(webView: WebView) {
    const webViewId = webView?.getWebViewUuid()
    if (webViewId && this.webViews[webViewId] == webView) {
      delete this.webViews[webViewId]
      this.writeRuntimeMessage({
        messageType: WebToRuntimeType.WebToRuntimeType_WEB_STATUS,
        webStatus: {
          snapshot: false,
          webViews: [buildWebViewStatus(webViewId, undefined)],
        },
      })
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

    // setup the service worker
    // NOTE: if not in /, requires the Service-Worker-Allowed: '/' header
    // NOTE: scope controls which /pages/ are covered by the worker
    const serviceWorker = await navigator.serviceWorker.register('/sw.js', {
      scope: '/',
    })
    await serviceWorker.update()

    // setup the workers
    if (this.isElectron) {
      // eslint-disable-next-line
      console.log('starting electron webview')
      // setup the forwarding to ipc
      if (this.runtimeCh) {
        forwardElectronIPC(this.runtimeCh.getTxCh(), this.runtimeCh.getRxCh())
      }
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
      this.worker.postMessage(initMsg)
    }

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
  }

  // handleRuntimeMessage handles an incoming message from the runtime.
  private async handleRuntimeMessage(msg: Uint8Array) {
    // parse 4 byte message prefix & check
    // TODO: buffer data so that fragmented packets work correctly
    const msgLen = decodeUint32Le(msg.slice(0, 4))
    if (msgLen != msg.length - 4) {
      console.error(
        'message len #%d does not match prefix #%d',
        msg.length - 4,
        msgLen
      )
      return
    }

    // remove the 4 byte message len prefix & parse
    const dmsg = RuntimeToWeb.decode(msg.slice(4))
    switch (dmsg.messageType) {
      case RuntimeToWebType.RuntimeToWebType_QUERY_STATUS:
        this.handleQueryStatus(dmsg.queryViewStatus || {})
        break
      case RuntimeToWebType.RuntimeToWebType_CREATE_VIEW:
        // TODO
        await this.handleCreateView(dmsg.createView || {})
        break
      case RuntimeToWebType.RuntimeToWebType_REMOVE_VIEW:
        this.handleRemoveView(dmsg.removeView || {})
        break
      default:
        console.warn('bldr: webview: unhandled message', dmsg)
        break
    }
  }

  // writeRuntimeMessage writes a message to the connected runtime.
  private writeRuntimeMessage(msg: WebToRuntime) {
    const msgData = WebToRuntime.encode(msg).finish()
    const merged = prependPacketLen(msgData)
    this.runtimeCh?.write(merged)
  }

  // handleQueryStatus handles a query status request.
  private handleQueryStatus(_: Partial<QueryWebStatus>) {
    if (this.isLeader) {
      this.writeWebStatusSnapshot(true)
    }
  }

  // handleCreateView handles a create web view request.
  private async handleCreateView(createView: Partial<CreateView>) {
    const webViewID = createView.id
    if (!webViewID) {
      return
    }
    let err: any
    if (!this.createWebViewCb) {
      err = new Error('cannot create web view')
    } else {
      try {
        await this.createWebViewCb(webViewID)
      } catch (e: any) {
        err = e
      }
    }
    if (err) {
      // TODO this.writeRuntimeMessage()... to inform the Go runtime of the error.
      console.error('bldr: unable to create web view', err)
    }
  }

  // handleRemoveView handles a remove web view request.
  private handleRemoveView(removeView: Partial<RemoveView>) {
    const webViewID = removeView.id
    if (!webViewID) {
      return
    }
    const webView = this.webViews[webViewID]
    if (webView) {
      webView.remove()
    }
  }

  // writeWebStatusSnapshot writes a full web status snapshot.
  // if allWorkers is set, includes web views from indexeddb.
  private async writeWebStatusSnapshot(allWorkers: boolean) {
    const msg: WebToRuntime = {
      messageType: WebToRuntimeType.WebToRuntimeType_WEB_STATUS,
      webStatus: await this.buildWebStatusSnapshot(allWorkers),
    }
    console.log('bldr: writing web status snapshot', msg)
    this.writeRuntimeMessage(msg)
  }

  // storeWebStatusSnapshot stores a web status snapshot in indexeddb.
  private async storeWebStatusSnapshot() {
    this.leaderElect.setWorkerKey(
      this.workerUuid,
      workerWebStatusKey,
      await this.buildWebStatusSnapshot(false)
    )
  }

  // loadWebStatusSnapshot loads a web status snapshot from indexeddb.
  // if the worker id is unset, uses the local id
  private async loadWebStatusSnapshot(
    workerUuid: string
  ): Promise<WebStatus | undefined> {
    return this.leaderElect.getWorkerKey<WebStatus>(
      workerUuid,
      workerWebStatusKey
    )
  }

  // buildInitMsg builds the worker init message
  private buildInitMsg(): Uint8Array {
    return WebInitRuntime.encode({
      runtimeId: this.runtimeId,
      workerUuid: this.workerUuid,
    }).finish()
  }

  // buildWebStatusSnapshot builds a snapshot of the status.
  // if allWorkers is set, includes web views from other active workers.
  // prevents duplicate web view entries
  private async buildWebStatusSnapshot(
    allWorkers: boolean
  ): Promise<WebStatus> {
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
        if (worker.id === this.workerUuid) {
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
      webViews: webViews,
    }
  }
}
