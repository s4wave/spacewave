import { MessagePortDuplex, OpenStreamCtr, PacketStream } from 'starpc'
import {
  WebRuntimeClientInit,
  WebRuntimeHostInit,
} from '../../runtime/runtime.pb.js'
import { WebDocumentToWebRuntime } from '../../runtime/runtime.js'
import {
  CreateWebDocumentFunc,
  RemoveWebDocumentFunc,
  WebRuntime,
} from '../../bldr/web-runtime.js'
import { GoWasmProcess, loadWebAssemblyModule } from '../../runtime/wasm/go-process.js'

// https://github.com/microsoft/TypeScript/issues/14877
declare let self: SharedWorkerGlobalScope
interface Global extends SharedWorkerGlobalScope {
  BLDR_INIT?: Uint8Array
  BLDR_WEB_RUNTIME_CLIENT_OPEN?: MessagePort
  BLDR_SQLITE_WORKER_URL?: string
}
const global: Global = self

// TODO: add/remove new windows via WebDocumentTracker
const createDocCb: CreateWebDocumentFunc | null = null
const removeDocCb: RemoveWebDocumentFunc | null = null

// goOpenStreamCtr contains the function to open a stream with the Go runtime.
const goOpenStreamCtr = new OpenStreamCtr(undefined)
// goOpenStream is a function that waits for goOpenStreamCtr & calls it.
const goOpenStream = goOpenStreamCtr.openStreamFunc

// construct the WebRuntime
const webRuntime = new WebRuntime(
  self.name,
  goOpenStream,
  createDocCb,
  removeDocCb,
)

// baseURL is the base URL to use for paths.
const baseURL = import.meta?.url

// Set the sqlite worker URL for Go to read via syscall/js.
// sqlite-worker.mjs is built to the same directory as runtime-wasm.mjs.
global.BLDR_SQLITE_WORKER_URL = new URL('./sqlite-worker.mjs', baseURL).toString()

// BLDR_RUNTIME_WASM is an injected variable with the path to the runtime.wasm
declare const BLDR_RUNTIME_WASM: string | undefined

// runtimeWasmURL is the path to the bldr runtime wasm that we will use.
const runtimeWasmURL = new URL(
  typeof BLDR_RUNTIME_WASM === 'string' && !!BLDR_RUNTIME_WASM ?
    BLDR_RUNTIME_WASM
  : './runtime.wasm',
  baseURL,
)

// Start prefetching the Go WASM module immediately.
const goWasmModule = loadWebAssemblyModule(runtimeWasmURL.toString())

// construct the go wasm process using the prefetched module
const goProcess = new GoWasmProcess(() => goWasmModule, {
  argv: ['runtime.wasm'],
  retryOpts: {
    errorCb: (err) => {
      console.warn('runtime-wasm: error running web runtime', err)
    },
  },
})

// the Go process will open streams with the WebRuntime via this channel and vise-versa.
const goOpenStreamChannel = new MessageChannel()
global.BLDR_WEB_RUNTIME_CLIENT_OPEN = goOpenStreamChannel.port2
goOpenStreamChannel.port1.onmessage = (msg) => {
  const data = msg.data
  if (data !== 'open-stream') {
    console.warn('runtime-wasm: unexpected web runtime open msg', data)
    return
  }

  const port = msg.ports[0]
  const portDuplex = new MessagePortDuplex<Uint8Array>(port)
  webRuntime
    .getWebRuntimeServer()
    .rpcStreamHandler(portDuplex)
    .catch(() => {})
}
goOpenStreamChannel.port1.start()
function startGoRpcStreams() {
  goOpenStreamCtr.set(async (): Promise<PacketStream> => {
    const streamChannel = new MessageChannel()
    goOpenStreamChannel.port1.postMessage('open-stream', [streamChannel.port2])
    return new MessagePortDuplex<Uint8Array>(streamChannel.port1)
  })
}

let goStarted = false
async function startGoRuntime(webRuntimeId: string) {
  if (goStarted) {
    return
  }
  goStarted = true

  // Configure the BLDR_INIT global
  global.BLDR_INIT = WebRuntimeHostInit.toBinary({
    webRuntimeId,
  })

  // Start the Go process
  goProcess.start()

  // start the RPC streams
  startGoRpcStreams()
}

// wait for startup / init command
const runtimeStarted = false
self.addEventListener('connect', (ev) => {
  console.log('runtime-wasm: connect event received, ports:', ev.ports?.length)
  const ports = ev.ports
  if (!ports || !ports.length) {
    return
  }

  const port = ev.ports[0]
  if (!port) {
    return
  }

  // Handle an incoming client for the WebRuntime and/or start the worker.
  port.onmessage = (msgEvent) => {
    console.log('runtime-wasm: onmessage received:', msgEvent.data)
    if (msgEvent.data === 'close') {
      port.close()
      return
    }

    const msg: WebDocumentToWebRuntime = msgEvent.data
    if (typeof msg !== 'object' || !msg.from) {
      console.log(
        'runtime-wasm: dropped invalid document to web runtime message',
        msg,
      )
      return
    }

    console.log('runtime-wasm: valid message from:', msg.from, 'keys:', Object.keys(msg))

    if (msg.initWebRuntime?.webRuntimeId && !runtimeStarted) {
      startGoRuntime(msg.initWebRuntime.webRuntimeId)
    }

    const clientPort = msg.connectWebRuntime?.port ?? msgEvent.ports?.[0]
    if (msg.connectWebRuntime && clientPort) {
      // handle the incoming client
      webRuntime.handleClient(
        WebRuntimeClientInit.fromBinary(msg.connectWebRuntime.init),
        clientPort,
      )
    }
  }

  port.start()
})
