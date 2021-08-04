import { detectWasmSupported } from './wasm-detect'

// gopherJS has some incompatibility issues, force using wasm for now.
const forceUseWasm = true

// Runtime attaches to or mounts the root Go runtime and provides an API to
// interact with it over IPC (usually BroadcastChannel).
export class Runtime {
  // useWasm indicates if web assembly is available.
  private useWasm: boolean
  // worker is the loaded runtime worker
  private worker: Worker
  // postInterval is the post setInterval
  private postInterval?: number
  // webViewUuid is the uuid of this WebView
  private webViewUuid: string
  // runtimeCh is the channel to talk to the runtime(s)
  private runtimeCh: BroadcastChannel
  // webViewCh is the channel to talk to this WebView
  private webViewCh: BroadcastChannel

  constructor() {
    this.webViewUuid = Math.random().toString(36).substr(2, 9)

    // Detect if we can use WebAssembly
    this.useWasm = forceUseWasm || detectWasmSupported()
    if (!this.useWasm) {
      console.log('WebAssembly is not supported in this browser')
    }

    // setup the service worker
    navigator.serviceWorker.register(
      new URL('./service-worker.js', import.meta.url)
    )

    // open channel to tx message -> the worker(s)
    this.runtimeCh = new BroadcastChannel('@aperturerobotics/bldr/runtime')
    this.postInterval = setInterval(() => {
      console.log('bldr: webview: send message to runtime channel')
      const enc = new TextEncoder()
      const msg = 'message from webview ' + this.webViewUuid
      this.runtimeCh.postMessage(enc.encode(msg))
    }, 10000)

    // open channel to rx message from the worker(s)
    // this.webViewCh = new BroadcastChannel(`@aperturerobotics/bldr/webview/${this.webViewUuid}`)
    this.webViewCh = new BroadcastChannel(`@aperturerobotics/bldr/webview/id`)
    const dec = new TextDecoder()
    this.webViewCh.onmessage = msg => {
      console.log('bldr: webview: got message: ' + dec.decode(msg.data))
    }

    // setup the web worker
    // new Worker(new URL('/runtime/runtime-wasm.js', import.meta.url))
    console.log('starting runtime worker')
    if (this.useWasm) {
      this.worker = new Worker(new URL('/runtime/runtime-wasm.js', import.meta.url))
      // postMessage -> init message (worker sleeps until it receives this)
      this.worker.postMessage(`init:${this.webViewUuid}`)
    } else {
      this.worker = new Worker(new URL('/runtime/runtime-js.js', import.meta.url))
    }
  }

  // dispose shuts down the runtime.
  public dispose() {
    if (this.worker) {
      this.worker.terminate()
    }
    if (this.postInterval) {
      clearInterval(this.postInterval)
      delete this.postInterval
    }
    if (this.runtimeCh) {
      this.runtimeCh.close()
    }
    if (this.webViewCh) {
      this.webViewCh.close()
    }
  }
}
