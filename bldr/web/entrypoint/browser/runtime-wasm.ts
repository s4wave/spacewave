import { OpenStreamCtr, type PacketStream } from 'starpc'

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
import {
  GoWasmProcess,
  loadWebAssemblyModule,
} from '../../runtime/wasm/go-process.js'
import { messagePortPacketStream } from './message-port-packet-stream.js'
import { RuntimeOpfsBridge } from './runtime-opfs-bridge.js'

// Detect whether we are running as a SharedWorker or a dedicated Worker.
// SharedWorker receives connections via the 'connect' event.
// Dedicated Worker receives an initial message with a communication port.
const isSharedWorker =
  typeof SharedWorkerGlobalScope !== 'undefined' &&
  self instanceof SharedWorkerGlobalScope

// https://github.com/microsoft/TypeScript/issues/14877
declare let self: SharedWorkerGlobalScope & DedicatedWorkerGlobalScope
interface Global {
  BLDR_INIT?: Uint8Array
  BLDR_WEB_RUNTIME_CLIENT_OPEN?: MessagePort
  BLDR_NOTIFY_PLUGIN_MANIFEST_ROOT?: (
    pluginId: string,
    rootHash: string,
  ) => void
  BLDR_NOTIFY_DURABLE_MUTATION?: () => void
}
const global: Global = self as unknown as Global

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
global.BLDR_NOTIFY_PLUGIN_MANIFEST_ROOT = (pluginId, rootHash) => {
  webRuntime.broadcastPluginManifestRoot(pluginId, rootHash)
}
global.BLDR_NOTIFY_DURABLE_MUTATION = () => {
  webRuntime.broadcastDurableMutation()
}

// baseURL is the base URL to use for paths.
const baseURL = import.meta?.url

// BLDR_RUNTIME_WASM is an injected variable with the path to the runtime.wasm
declare const BLDR_RUNTIME_WASM: string | undefined

// runtimeWasmURL is the path to the bldr runtime wasm that we will use.
const runtimeWasmURL = new URL(
  typeof BLDR_RUNTIME_WASM === 'string' && !!BLDR_RUNTIME_WASM
    ? BLDR_RUNTIME_WASM
    : './runtime.wasm',
  baseURL,
)

// Start prefetching the Go WASM module immediately.
const goWasmModule = loadWebAssemblyModule(runtimeWasmURL.toString())

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
  const portDuplex = messagePortPacketStream(port)
  webRuntime
    .getWebRuntimeServer()
    .rpcStreamHandler(portDuplex)
    .catch(() => {})
}
goOpenStreamChannel.port1.start()
// startGoRpcStreams installs the opener for raw Go MessagePort RPC streams.
function startGoRpcStreams() {
  goOpenStreamCtr.set(async (): Promise<PacketStream> => {
    const streamChannel = new MessageChannel()
    goOpenStreamChannel.port1.postMessage('open-stream', [streamChannel.port2])
    return messagePortPacketStream(streamChannel.port1)
  })
}

let goStarted = false

// startGoRuntime starts the process once, after its host configuration is installed.
async function startGoRuntime(
  webRuntimeId: string,
  env?: Record<string, string>,
) {
  if (goStarted) {
    return
  }
  goStarted = true

  // Configure the BLDR_INIT global
  global.BLDR_INIT = WebRuntimeHostInit.toBinary({
    webRuntimeId,
  })

  // Construct the Go WASM process after init so callers can pass runtime env.
  const goProcess = new GoWasmProcess(() => goWasmModule, {
    argv: ['runtime.wasm'],
    env,
    retryOpts: {
      errorCb: (err) => {
        console.warn('runtime-wasm: error running web runtime', err)
      },
    },
  })

  // Start the Go process
  goProcess.start()

  // start the RPC streams
  startGoRpcStreams()
}

// runtimeOpfsBridge brokers a DedicatedWorker OPFS bridge when the runtime runs
// in a SharedWorker, which cannot call navigator.storage.getDirectory(). A
// dedicated Worker reaches OPFS directly and needs no bridge.
const runtimeOpfsBridge = isSharedWorker
  ? new RuntimeOpfsBridge(self.name)
  : null

// handlePortMessage processes a message from a WebDocument on a communication port.
let runtimeStarted = false
function handlePortMessage(msgEvent: MessageEvent) {
  if (msgEvent.data === 'close') {
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

  console.log(
    'runtime-wasm: valid message from:',
    msg.from,
    'keys:',
    Object.keys(msg),
  )

  if (runtimeOpfsBridge && msg.opfsBrokerPort) {
    runtimeOpfsBridge.addWebDocument(msg.from, msg.opfsBrokerPort)
  }

  if (msg.initWebRuntime?.webRuntimeId && !runtimeStarted) {
    const { webRuntimeId, env } = msg.initWebRuntime
    void (async () => {
      // SharedWorker cannot open OPFS directly. A document must provide its
      // bridge before Go mounts volumes; retry when another document connects.
      if (runtimeOpfsBridge && !(await runtimeOpfsBridge.ensureBridge())) {
        console.warn(
          'runtime-wasm: OPFS bridge unavailable; deferring Go start until a document can host OPFS',
        )
        return
      }
      if (runtimeStarted) {
        return
      }
      runtimeStarted = true
      await startGoRuntime(webRuntimeId, env)
    })().catch((err) => {
      console.warn('runtime-wasm: error running web runtime', err)
    })
  }

  const clientPort = msg.connectWebRuntime?.port ?? msgEvent.ports?.[0]
  if (msg.connectWebRuntime && clientPort) {
    webRuntime.handleClient(
      WebRuntimeClientInit.fromBinary(msg.connectWebRuntime.init),
      clientPort,
    )
  }
}

if (isSharedWorker) {
  // SharedWorker mode: each connecting page fires a 'connect' event with a port.
  self.addEventListener('connect', (ev) => {
    console.log(
      'runtime-wasm: connect event received, ports:',
      ev.ports?.length,
    )
    const port = ev.ports?.[0]
    if (!port) {
      return
    }
    port.onmessage = handlePortMessage
    port.start()
  })
} else {
  // Dedicated Worker mode: the page transfers a MessagePort in the first message.
  // All subsequent communication uses that port (same pattern as SharedWorker).
  self.onmessage = (msgEvent: MessageEvent) => {
    const port = msgEvent.ports?.[0]
    if (port) {
      console.log('runtime-wasm: dedicated worker received communication port')
      port.onmessage = handlePortMessage
      port.start()
    }
    // The first message may also contain an init or connect request.
    if (
      msgEvent.data &&
      typeof msgEvent.data === 'object' &&
      msgEvent.data.from
    ) {
      handlePortMessage(msgEvent)
    }
  }
}
