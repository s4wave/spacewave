import { detectWasmSupported } from './wasm-detect'
import { Channel } from './channel'
import { RuntimeToWeb, WebInitRuntime } from '../../runtime/web/web'
import { isElectron, forwardElectronIPC } from './electron'

// gopherJS has some incompatibility issues, force using wasm for now.
const forceUseWasm = true

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
  // runtimeUuid is the unique id of the runtime worker.
  private runtimeUuid: string
  // useWasm indicates if web assembly is available.
  private useWasm?: boolean
  // worker is the loaded runtime worker
  private worker?: Worker
  // runtimeCh is the two-way channel to the runtime worker(s).
  private runtimeCh?: Channel
  // isElectron indicates this is electron.
  private isElectron?: boolean

  constructor(placeholder?: boolean) {
    this.runtimeUuid = Math.random().toString(36).substr(2, 9)
    this.placeholder = placeholder || false
    if (isElectron) {
      this.isElectron = true
    }
    if (this.placeholder) {
      return
    }

    // Detect if we can use WebAssembly
    this.useWasm = forceUseWasm || detectWasmSupported()
    if (!this.useWasm) {
      console.log('WebAssembly is not supported in this browser')
    }

    const topicPrefix = '@aperturerobotics/bldr'
    const txID = `${topicPrefix}/r/${this.runtimeUuid}`
    // const rxID =`@aperturerobotics/bldr/webview/${this.webViewUuid}`
    const rxID = `${topicPrefix}/w/${this.runtimeUuid}`

    this.runtimeCh = new Channel(txID, rxID, this.handleMessage.bind(this))

    // setup the workers
    if (this.isElectron) {
      console.log('starting electron webview')
      // setup the service worker
      navigator.serviceWorker.register('./service-worker.js')
      // setup the forwarding to ipc
      forwardElectronIPC(this.runtimeCh.getTxCh(), this.runtimeCh.getRxCh())
    } else {
      console.log('starting runtime worker')
      // setup the service worker
      navigator.serviceWorker.register(
        new URL('./service-worker.js', import.meta.url || '')
      )

      // setup the webworkers
      if (this.useWasm) {
        this.worker = new Worker(
          new URL('/runtime/runtime-wasm.js', import.meta.url)
        )

        // postMessage -> init message (worker sleeps until it receives this)
        const initMsg = this.buildInitMsg()
        this.worker.postMessage(initMsg)
      } else {
        // TODO: pass init message to gopherjs variant
        this.worker = new Worker(
          new URL('/runtime/runtime-js.js', import.meta.url)
        )
      }
    }
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
    if (this.worker) {
      this.worker.terminate()
    }
    if (this.runtimeCh) {
      this.runtimeCh.close()
    }
  }

  // handleMessage handles an incoming message from the runtime.
  private handleMessage(msg: Uint8Array) {
    // placeholder
    console.log('bldr: webview: decode message: ', msg)
    const dmsg = RuntimeToWeb.decode(msg)
    console.log('bldr: webview: got message: ', dmsg)
  }

  // unregisterWebView removes the web-view and notifies the runtime if necessary.
  private unregisterWebView(webView: WebView) {
    // todo
  }

  // buildInitMsg builds the worker init message
  private buildInitMsg(): Uint8Array {
    return WebInitRuntime.encode({
      runtimeId: this.runtimeUuid,
    }).finish()
  }
}
