import { detectWasmSupported } from './wasm-detect'
import { Channel } from './channel'
import {
  RuntimeToWeb,
  WebInitRuntime,
  RuntimeToWebType,
  WebToRuntime,
  WebToRuntimeType,
  QueryWebStatus,
  WebStatus,
} from '../runtime/runtime'
import { isElectron, forwardElectronIPC } from './electron'
import { LeaderElect } from './leader-elect'

// WebView implements the web-view with pluggable logic.
export interface WebView {
  // getWebViewUuid returns the web-view unique identifier.
  getWebViewUuid(): string
}

// WebViewRegistration is returned when registering a web-view.
export interface WebViewRegistration {
  // release indicates that the web view has been shutdown.
  release(): void
}

// Runtime attaches to or mounts the root Go runtime and provides an API to
// interact with it over IPC (usually BroadcastChannel).
//
// There should be a single Runtime constructed for each Window.
// WebView should be attached to the Runtime to display web content.
export class Runtime {
  // placeholder indicates that this is a placeholder runtime.
  private placeholder?: boolean
  // runtimeId identifies the runtime across multiple tabs.
  private runtimeId: string
  // webviewUuid is the unique id of the webview & attached worker.
  private webviewUuid: string
  // leaderElect manages leader election of the worker.
  private leaderElect: LeaderElect
  // useWasm indicates if web assembly is available.
  private useWasm?: boolean
  // runtimeCh is the two-way channel to the runtime worker(s).
  private runtimeCh?: Channel
  // isElectron indicates this is electron.
  private isElectron?: boolean
  // workerRunning indicates we should run the worker.
  // controlled by leader election
  private workerRunning: boolean
  // worker is the loaded runtime worker
  // unset until this is the leader tab
  private worker?: Worker

  constructor(runtimeId?: string, placeholder?: boolean) {
    if (!runtimeId) {
      runtimeId = 'default'
    }
    this.runtimeId = runtimeId
    this.webviewUuid = Math.random().toString(36).substr(2, 9)
    this.placeholder = placeholder || false
    this.workerRunning = false
    if (isElectron) {
      this.isElectron = true
    }
    if (this.placeholder) {
      return
    }

    // Setup the leader election
    const electionUuid = "bldr/runtime/" + this.runtimeId
    this.leaderElect = new LeaderElect(
      electionUuid, this.webviewUuid,
      this.onLeaderChanged.bind(this),
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

    const topicPrefix = '@aperturerobotics/bldr'
    const txID = `${topicPrefix}/r/${this.runtimeId}`
    // const rxID =`@aperturerobotics/bldr/webview/${this.webViewUuid}`
    const rxID = `${topicPrefix}/w/${this.runtimeId}`

    this.runtimeCh = new Channel(txID, rxID, this.handleMessage.bind(this))
  }

  // registerWebView registers a web-view with the runtime.
  public registerWebView(webView: WebView): WebViewRegistration {
    if (this.placeholder) {
      // no-op placeholder
      console.warn('register web view with placeholder runtime (no-op)')
      return { release: () => {} } as WebViewRegistration
    }

    console.log('register web view with id ' + webView.getWebViewUuid())
    // TODO
    return {
      release: () => {
        this.unregisterWebView(webView)
      },
    } as WebViewRegistration
  }

  // dispose shuts down the runtime.
  public dispose() {
    if (this.placeholder) {
      return
    }
    if (this.leaderElect) {
      this.leaderElect.close()
      this.leaderElect = undefined
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
    } else {
      if (this.workerRunning) {
        this.shutdownWorker()
      }
    }
  }

  // launchWorker loads and launches the webworker.
  private launchWorker() {
    this.workerRunning = true
    if (this.worker) {
      // already running
      return
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
    // placeholder
    if (this.placeholder) {
      return
    }

    const dmsg = RuntimeToWeb.decode(msg)

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
    this.runtimeCh?.write(WebToRuntime.encode(msg).finish())
  }

  // handleQueryStatus handles a query status request.
  private handleQueryStatus(queryStatus: QueryWebStatus) {
    console.log('bldr: replying to query status request')
    this.writeMessage({
      messageType: WebToRuntimeType.WebToRuntimeType_STATUS,
      webStatus: this.buildWebStatus(),
    })
  }

  // buildWebStatus builds a snapshot of the status.
  private buildWebStatus(): WebStatus {
      // TODO
    return {
      webViews: [{
        id: 'test-webview-todo',
        permanent: true,
      }],
    }
  }

  // unregisterWebView removes the web-view and notifies the runtime if necessary.
  private unregisterWebView(webView: WebView) {
    // todo
  }

  // buildInitMsg builds the worker init message
  private buildInitMsg(): Uint8Array {
    return WebInitRuntime.encode({
      runtimeId: this.webviewUuid,
    }).finish()
  }
}
