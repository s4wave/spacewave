import {
  RuntimeToWeb,
  WebInitRuntime,
  RuntimeToWebType,
  WebToRuntime,
  WebToRuntimeType,
  QueryWebStatus,
  WebStatus,
  WebViewStatus,
} from '../runtime/runtime'
import { decodeUint32Le, prependPacketLen } from './binary'
import { Channel } from './channel'
import { isElectron, forwardElectronIPC } from './electron'
import { LeaderElect } from './leader-elect'
import { detectWasmSupported } from './wasm-detect'
import { WebView, WebViewRegistration, buildWebViewStatus } from './web-view'

// Runtime tracks all WebView associated with Runtime instances with the same ID
// and browser context (usually based on the URL).
//
// Attaches to or mounts the root Go runtime and provides an API to interact
// with it over IPC (usually BroadcastChannel).
//
// There can be multiple Runtime in a page, although it best to have 1 Runtime
// per HTML Document.
export class Runtime {
  // runtimeId is the ID of the Go Runtime, same across all tabs.
  // there can be a single Go runtime with multiple TS Runtimes.
  private runtimeId: string
  // workerUuid is the unique id of this instance & attached worker.
  // this ID identifies this TypeScript Runtime class object.
  private workerUuid: string
  // leaderElect manages leader election and participant tracking.
  private leaderElect: LeaderElect
  // useWasm indicates if web assembly is available.
  private useWasm?: boolean
  // runtimeCh is the two-way channel to the runtime worker(s).
  private runtimeCh?: Channel
  // isElectron indicates this is electron and we will use ipcRenderer.
  private isElectron?: boolean
  // workerRunning indicates we should run the worker.
  // controlled by leader election
  private workerRunning: boolean
  // worker is the loaded runtime worker
  // unset until this is the leader tab
  private worker?: Worker
  // webViews contains the list of associated web views by ID.
  private webViews: { [id: string]: WebView }

  constructor(runtimeId?: string) {
    if (!runtimeId) {
      runtimeId = 'default'
    }
    this.runtimeId = runtimeId
    this.workerUuid = Math.random().toString(36).substr(2, 9)
    this.workerRunning = false
    if (isElectron) {
      this.isElectron = true
    }
    this.webViews = {}

    // Setup the leader election
    const electionUuid = 'r/' + this.runtimeId
    this.leaderElect = new LeaderElect(
      electionUuid,
      this.workerUuid,
      this.onLeaderChanged.bind(this)
    )
    if (window && window.addEventListener) {
      window.addEventListener('beforeunload', (e: BeforeUnloadEvent) => {
        this.dispose()
        delete e['returnValue']
      })
    }

    // Detect if we can use WebAssembly.
    this.useWasm = detectWasmSupported()
    if (!this.useWasm) {
      console.log('WebAssembly is not supported in this browser')
    }

    // Build the runtime communication channels.
    const topicPrefix = '@aperturerobotics/bldr/' + runtimeId
    const txID = `${topicPrefix}/r`
    const rxID = `${topicPrefix}/w`
    this.runtimeCh = new Channel(txID, rxID, this.handleMessage.bind(this))
  }

  // registerWebView registers a web-view with the runtime.
  public registerWebView(webView: WebView): WebViewRegistration {
    const webViewId = webView.getWebViewUuid()
    console.log('register web view with id ' + webViewId)
    this.webViews[webViewId] = webView
    this.notifyWebViewUpdated(webViewId, webView)

    return {
      release: () => {
        this.unregisterWebView(webView)
      },
    } as WebViewRegistration
  }

  // dispose shuts down the runtime.
  public dispose() {
    if (this.leaderElect) {
      this.leaderElect.close()
    }
    if (this.workerRunning) {
      this.shutdownWorker()
    }
    if (this.runtimeCh) {
      this.runtimeCh.close()
      this.runtimeCh = undefined
    }
  }

  // onLeaderChanged indicates the current leader-tab changed.
  // we run one WebWorker with the main Runtime in the leader tab.
  private async onLeaderChanged(_: string, isUs: boolean) {
    if (isUs) {
      if (!this.workerRunning) {
        this.launchWorker()
      }
      // send initial web status snapshot
      this.writeWebStatusSnapshot()
    } else {
      if (this.workerRunning) {
        this.shutdownWorker()
      }
    }
  }

  // notifyWebViewUpdated sends a message to the runtime with the WebView update.
  // if the web view is null, sends a message indicating the view was removed.
  private notifyWebViewUpdated(webViewId: string, webView?: WebView) {
    const runtimeCh = this.runtimeCh
    if (!runtimeCh || !webViewId) {
      return
    }

    this.writeMessage({
      messageType: WebToRuntimeType.WebToRuntimeType_WEB_STATUS,
      webStatus: {
        snapshot: false,
        webViews: [buildWebViewStatus(webViewId, webView)],
      },
    })
  }

  // unregisterWebView removes the web-view and notifies the runtime if necessary.
  private unregisterWebView(webView: WebView) {
    const webViewId = webView?.getWebViewUuid()
    if (webViewId && this.webViews[webViewId] == webView) {
      delete this.webViews[webViewId]
      this.writeMessage({
        messageType: WebToRuntimeType.WebToRuntimeType_WEB_STATUS,
        webStatus: {
          snapshot: false,
          webViews: [buildWebViewStatus(webViewId, undefined)],
        },
      })
    }
  }

  // launchWorker loads and launches the webworker.
  private launchWorker() {
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
    // setup the workers
    if (this.isElectron) {
      // eslint-disable-next-line
      console.log('starting electron webview')
      // setup the service worker
      navigator.serviceWorker.register('./service-worker.js')
      // setup the forwarding to ipc
      if (this.runtimeCh) {
        forwardElectronIPC(this.runtimeCh.getTxCh(), this.runtimeCh.getRxCh())
      }
    } else {
      // eslint-disable-next-line
      console.log('starting runtime worker')
      // setup the service worker
      navigator.serviceWorker.register(
        // eslint-disable-next-line
        new URL('./service-worker.js', import.meta.url || '')
      )

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
  }

  // shutdownWorker shuts down the webworker.
  private async shutdownWorker() {
    this.workerRunning = false
    if (this.worker) {
      this.worker.terminate()
      this.worker = undefined
    }
  }

  // handleMessage handles an incoming message from the runtime.
  private handleMessage(msg: Uint8Array) {
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
      default:
        console.warn('bldr: webview: unhandled message', dmsg)
        break
    }
  }

  // writeMessage writes a message to the connected runtime.
  private writeMessage(msg: WebToRuntime) {
    const msgData = WebToRuntime.encode(msg).finish()
    const merged = prependPacketLen(msgData)
    this.runtimeCh?.write(merged)
  }

  // handleQueryStatus handles a query status request.
  private handleQueryStatus(_: QueryWebStatus) {
    this.writeWebStatusSnapshot()
  }

  // writeWebStatusSnapshot writes a full web status snapshot.
  private writeWebStatusSnapshot() {
    const msg: WebToRuntime = {
      messageType: WebToRuntimeType.WebToRuntimeType_WEB_STATUS,
      webStatus: this.buildWebStatusSnapshot(),
    }
    console.log('bldr: writing web status snapshot', msg)
    this.writeMessage(msg)
  }

  // buildInitMsg builds the worker init message
  private buildInitMsg(): Uint8Array {
    return WebInitRuntime.encode({
      runtimeId: this.runtimeId,
      workerUuid: this.workerUuid,
    }).finish()
  }

  // buildWebStatusSnapshot builds a snapshot of the status.
  private buildWebStatusSnapshot(): WebStatus {
    let webViews: WebViewStatus[] = []
    for (const webViewId in this.webViews) {
      const webView = this.webViews[webViewId]
      if (webViewId && webView) {
        webViews.push(buildWebViewStatus(webViewId, webView))
      }
    }
    return {
      snapshot: true,
      webViews: webViews,
    }
  }
}
