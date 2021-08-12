import { detectWasmSupported } from './wasm-detect'
import { Channel } from './channel'

// gopherJS has some incompatibility issues, force using wasm for now.
const forceUseWasm = true

// BLDR_ELECTRON is declared if this is Electron.
declare var BLDR_ELECTRON: {
  // forwardElectronIPC forwards the tx and rx broadcast channels to electron ipc.
  forwardElectronIPC(tx: BroadcastChannel, rx: BroadcastChannel): void
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
  // webViewUuid is the uuid of this WebView
  private webViewUuid: string
  // useWasm indicates if web assembly is available.
  private useWasm?: boolean
  // worker is the loaded runtime worker
  private worker?: Worker
  // runtimeCh is the two-way channel to the runtime worker(s).
  private runtimeCh?: Channel
  // isElectron indicates this is electron.
  private isElectron?: boolean

  constructor(placeholder?: boolean) {
    this.placeholder = placeholder
    if (this.placeholder) {
      this.webViewUuid = '<placeholder>'
      return
    }

    if (isElectron) {
      this.isElectron = true
    }

    this.webViewUuid = Math.random().toString(36).substr(2, 9)

    // Detect if we can use WebAssembly
    this.useWasm = forceUseWasm || detectWasmSupported()
    if (!this.useWasm) {
      console.log('WebAssembly is not supported in this browser')
    }

    const txID = '@aperturerobotics/bldr/runtime'
    // const rxID =`@aperturerobotics/bldr/webview/${this.webViewUuid}`
    const rxID = `@aperturerobotics/bldr/webview/id`

    const dec = new TextDecoder()
    this.runtimeCh = new Channel(txID, rxID, (msg) => {
      // placeholder
      console.log('bldr: webview: got message: ' + dec.decode(msg))
    })
    // this.runtimeCh.write(new TextEncoder().encode('hello world'))

    // setup the web worker
    // new Worker(new URL('/runtime/runtime-wasm.js', import.meta.url))
    if (this.isElectron) {
      console.log('starting electron webview')
      // setup the service worker
      navigator.serviceWorker.register("./service-worker.js")
      // setup the forwarding to ipc
      BLDR_ELECTRON.forwardElectronIPC(this.runtimeCh.getTxCh(), this.runtimeCh.getRxCh())
    } else {
      console.log('starting runtime worker')
      // setup the service worker
      navigator.serviceWorker.register(
        new URL('./service-worker.js', import.meta.url || "")
      )
      // setup the webworkers
      if (this.useWasm) {
        this.worker = new Worker(
          new URL('/runtime/runtime-wasm.js', import.meta.url)
        )
        // postMessage -> init message (worker sleeps until it receives this)
        this.worker.postMessage(`init:${this.webViewUuid}`)
      } else {
        this.worker = new Worker(
          new URL('/runtime/runtime-js.js', import.meta.url)
        )
      }
    }
  }

  // registerWebView registers a web-view with the runtime.

  // dispose shuts down the runtime.
  public dispose() {
    if (this.worker) {
      this.worker.terminate()
    }
    if (this.runtimeCh) {
      this.runtimeCh.close()
    }
  }
}
