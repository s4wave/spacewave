import { detectWasmSupported } from './wasm-detect'
import { Channel } from './channel'
import { RuntimeToWebView } from '../../runtime/ipc/webview/webview'

// gopherJS has some incompatibility issues, force using wasm for now.
const forceUseWasm = true

// BLDR_ELECTRON is declared if this is Electron.
declare var BLDR_ELECTRON: {
  // txMessage transmits a message to the host runtime.
  txMessage(msg: Uint8Array): void
  // setMessageHandler sets the ipc message handler.
  setMessageHandler(cb: (data: Uint8Array) => void): void
}

// forwardElectronIPC forwards the tx and rx broadcast channels to electron ipc.
function forwardElectronIPC(tx: BroadcastChannel, rx: BroadcastChannel) {
  tx.onmessage = (ev: MessageEvent<Uint8Array>) => {
    BLDR_ELECTRON.txMessage(ev.data)
  }
  BLDR_ELECTRON.setMessageHandler((data: Uint8Array) => {
    rx.postMessage(data)
  })
}

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

// isElectron indicates this is electron.
const isElectron = typeof BLDR_ELECTRON !== 'undefined'


// Runtime attaches to or mounts the root Go runtime and provides an API to
// interact with it over IPC (usually BroadcastChannel).
//
// There should be a single Runtime constructed per WebView.
// The Runtime can be controlled by Go to display content and load assets.
export class Runtime {
  // placeholder indicates that this is a placeholder runtime.
  private placeholder?: boolean
  // useWasm indicates if web assembly is available.
  private useWasm?: boolean
  // worker is the loaded runtime worker
  private worker?: Worker
  // runtimeCh is the two-way channel to the runtime worker(s).
  private runtimeCh?: Channel
  // isElectron indicates this is electron.
  private isElectron?: boolean

  constructor(placeholder?: boolean) {
    this.placeholder = placeholder || false
    if (isElectron) {
      this.isElectron = true
    }
    if (placeholder) {
      return
    }

    // Detect if we can use WebAssembly
    this.useWasm = forceUseWasm || detectWasmSupported()
    if (!this.useWasm) {
      console.log('WebAssembly is not supported in this browser')
    }

    const txID = '@aperturerobotics/bldr/runtime'
    // const rxID =`@aperturerobotics/bldr/webview/${this.webViewUuid}`
    const rxID = `@aperturerobotics/bldr/webview/id`

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
        this.worker.postMessage(`init:`)
      } else {
        this.worker = new Worker(
          new URL('/runtime/runtime-js.js', import.meta.url)
        )
      }
    }
  }

  // registerWebView registers a web-view with the runtime.
  public registerWebView(webView: WebView): WebViewRegistration {
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
    const dmsg = RuntimeToWebView.decode(msg)
    console.log('bldr: webview: got message: ', dmsg)
  }

  // unregisterWebView removes the web-view and notifies the runtime if necessary.
  private unregisterWebView(webView: WebView) {
    // todo
  }
}
