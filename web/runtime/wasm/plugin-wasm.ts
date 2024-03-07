import { Pushable, pushable } from 'it-pushable'
import {
  MessagePortDuplex,
  OpenStreamCtr,
  PacketStream,
  castToError,
} from 'starpc'
import { WebDocumentTracker } from '../../bldr/web-document-tracker.js'
import { GoWasmProcess } from '../../runtime/wasm/go-process.js'
import { WebDocumentToWorker } from '../runtime.js'
import { WebRuntimeClientType } from '../runtime.pb.js'

// https://github.com/microsoft/TypeScript/issues/14877
declare let self: SharedWorkerGlobalScope | DedicatedWorkerGlobalScope
interface Global {
  BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME?: (
    onMessage: (message: Uint8Array) => void,
    onClose: (errMsg?: string) => void,
  ) => Promise<Pushable<Uint8Array>>
  BLDR_PLUGIN_SET_ACCEPT_STREAM?: (acceptStream: () => MessagePort) => void
}
const global: Global & (SharedWorkerGlobalScope | DedicatedWorkerGlobalScope) =
  self

function checkSharedWorker(
  scope: SharedWorkerGlobalScope | DedicatedWorkerGlobalScope,
): scope is SharedWorkerGlobalScope {
  return (
    typeof SharedWorkerGlobalScope !== undefined &&
    scope instanceof SharedWorkerGlobalScope
  )
}

// baseURL is the base URL to use for paths.
const baseURL = import.meta?.url

// workerId is the id to use for the worker.
const workerId = self.name

// BLDR_PLUGIN_ENTRYPOINT is declared at build time by the plugin compiler.
declare const BLDR_PLUGIN_ENTRYPOINT: string
const pluginEntrypointPath = BLDR_PLUGIN_ENTRYPOINT!

// onWebDocumentsExhausted handles when no WebDocument can be contacted anymore.
const onWebDocumentsExhausted = async () => {
  // Unlike the ServiceWorker, the WebWorker / SharedWorker has no way to
  // contact a WebDocument proactively. (client.postMessage). If there are no
  // available connections to WebDocument, then we should exit.
  console.log(`PluginWorker: ${workerId}: no WebDocument available, exiting!`)
  self.close()
}

// webDocumentTracker tracks the set of connected remote WebDocument.
const webDocumentTracker = new WebDocumentTracker(
  workerId,
  WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
  onWebDocumentsExhausted,
  /*
  (webDocumentId, msgPort) => {
      const streamConn = new ChannelStream(
        workerId,
        msgPort,
        WebRuntimeClientChannelStreamOpts,
      )
      call => goOpenStream ?
      performance issue with calling WebDocument every time?
      better to pass a MessagePort to the WebRuntime and open that way?
  },
  */
)

// webRuntimeClient manages the connection to the WebRuntime.
const webRuntimeClient = webDocumentTracker.webRuntimeClient

// goOpenStreamCtr contains the function to open a stream with the Go program.
const goOpenStreamCtr = new OpenStreamCtr(undefined)
// goOpenStream is a function that waits for goOpenStreamCtr & calls it.
// const goOpenStream = goOpenStreamCtr.openStreamFunc

// The Go runtime will call this function to open outgoing streams.
global.BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME = async (
  onMessage,
  onClose,
): Promise<Pushable<Uint8Array>> => {
  const packetStream = await webRuntimeClient.openStream()
  const packetSource = packetStream.source
  queueMicrotask(async () => {
    try {
      for await (const msg of packetSource) {
        onMessage(msg)
      }
      onClose()
    } catch (err) {
      const e = castToError(err)
      onClose(e.toString())
    }
  })

  const push = pushable<Uint8Array>({ objectMode: true })
  queueMicrotask(() => packetStream.sink(push))
  return push
}

// The Go runtime will call this function to set a callback for incoming streams.
global.BLDR_PLUGIN_SET_ACCEPT_STREAM = (acceptStrm?: () => MessagePort) => {
  if (!acceptStrm) {
    goOpenStreamCtr.set(undefined)
    return
  }
  goOpenStreamCtr.set(async (): Promise<PacketStream> => {
    return new MessagePortDuplex<Uint8Array>(acceptStrm())
  })
}

let goStarted = false
function startGoPlugin(startInfo: Uint8Array) {
  if (goStarted) {
    return
  }
  goStarted = true

  // startInfo is b58 encoded with utf8
  const startInfoB58 = new TextDecoder().decode(startInfo)

  // construct the go wasm process
  const goProcess = new GoWasmProcess(
    new URL(pluginEntrypointPath, baseURL).toString(),
    {
      argv: ['plugin.wasm'],
      env: {
        BLDR_PLUGIN_START_INFO: startInfoB58,
      },
      retryOpts: {
        errorCb: (err) => {
          console.warn('plugin-wasm: error executing wasm', err)
          goOpenStreamCtr.set(undefined)
          // TODO notify error to the plugin host & ask if we should retry or terminate
          // webRuntimeClient.notifyError(...)
          self.close() // terminate the shared worker
        },
      },
    },
  )

  // start the Go process
  goProcess.start()
}

const handleWorkerMessage = (msgEvent: MessageEvent<WebDocumentToWorker>) => {
  // Expect the WebDocument to send a WebDocumentToWorker.
  const data: WebDocumentToWorker = msgEvent.data
  webDocumentTracker.handleWebDocumentMessage(data)

  if (data.initData && !goStarted) {
    startGoPlugin(data.initData)
  }
}

if (checkSharedWorker(self)) {
  // If this is a SharedWorker, handle the "connect" event when a WebDocument connects.
  self.addEventListener('connect', (ev) => {
    // With a shared worker, "connect" is fired when "new SharedWorker" is called.
    // The port passed with the event is connected to the sharedWorker.port on the WebDocument.
    const ports = ev.ports
    if (!ports || !ports.length) {
      return
    }

    const port = ev.ports[0]
    if (!port) {
      return
    }

    port.onmessage = (ev) => handleWorkerMessage(ev)
    port.start()
  })
} else {
  // Otherwise this must be a DedicatedWorker.
  self.addEventListener('message', (ev) => handleWorkerMessage(ev))
}
