import {
  WebRuntimeClientInit,
  WebRuntimeHostInit,
} from '../../runtime/runtime.pb.js'
import { GoWasmProcess } from '../../runtime/wasm/go-process.js'
import {
  CreateWebDocumentFunc,
  RemoveWebDocumentFunc,
  WebRuntime,
} from '../../bldr/web-runtime.js'
import { MessagePortDuplex, OpenStreamCtr, PacketStream } from 'starpc'

// https://github.com/microsoft/TypeScript/issues/14877
declare let self: SharedWorkerGlobalScope
interface Global extends SharedWorkerGlobalScope {
  BLDR_INIT?: Uint8Array
  BLDR_WEB_RUNTIME_CLIENT_OPEN?: MessagePort
}
const global: Global = self

// TODO: add/remove new windows
const createDocCb: CreateWebDocumentFunc | null = null
const removeDocCb: RemoveWebDocumentFunc | null = null

// goOpenStreamCtr contains the function to open a stream with the Go runtime.
const goOpenStreamCtr = new OpenStreamCtr(undefined)
// goOpenStream is a function that waits for goOpenStreamCtr & calls it.
const goOpenStream = goOpenStreamCtr.openStreamFunc

// construct the WebRuntime
const webRuntime = new WebRuntime(
  // TODO: should this runtime be from the init message instead?
  `shared-worker:${self.location.host}`,
  goOpenStream,
  createDocCb,
  removeDocCb,
)

// construct the go wasm process
const goProcess = new GoWasmProcess('/runtime/runtime.wasm', {
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

// wait for startup / init command
let runtimeStarted = false
self.addEventListener('connect', (ev) => {
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
    const msg = msgEvent.data
    if (msg === 'close') {
      port.close()
      return
    }

    if (typeof msg !== 'object' || !(msg instanceof Uint8Array)) {
      console.log('runtime-wasm: dropped invalid init message', msg)
      return
    }

    const initMsg = WebRuntimeClientInit.decode(msg)
    if (!msgEvent.ports.length) {
      console.error(
        'runtime-wasm: dropped invalid init message without port',
        msg,
      )
      return
    }

    // Handle the incoming client
    const connPort = msgEvent.ports[0]
    webRuntime.handleClient(initMsg, connPort)

    // Start the runtime if needed
    if (!runtimeStarted) {
      if (!initMsg.webRuntimeId) {
        throw new Error('web runtime id: must be set in init message')
      }
      runtimeStarted = true

      // Configure the BLDR_INIT global
      global.BLDR_INIT = WebRuntimeHostInit.encode({
        webRuntimeId: initMsg.webRuntimeId,
      }).finish()

      // Start the Go process
      goProcess.start()

      // Start the RPC streams
      startGoRpcStreams()
    }
  }
  port.start()
})
