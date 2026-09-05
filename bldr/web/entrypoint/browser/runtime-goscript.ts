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
import { messagePortPacketStream } from './message-port-packet-stream.js'
import { RuntimeOpfsBridge } from './runtime-opfs-bridge.js'

// GoScriptRuntimeMain runs the compiled process until it exits.
export type GoScriptRuntimeMain = () => void | Promise<void>
// GoScriptRuntimeMainLoader resolves the compiled process entrypoint.
export type GoScriptRuntimeMainLoader = () => Promise<GoScriptRuntimeMain>

const isSharedWorker =
  typeof SharedWorkerGlobalScope !== 'undefined' &&
  self instanceof SharedWorkerGlobalScope

declare let self: SharedWorkerGlobalScope & DedicatedWorkerGlobalScope

interface Global {
  BLDR_INIT?: Uint8Array
  BLDR_WEB_RUNTIME_CLIENT_OPEN?: MessagePort
  BLDR_NOTIFY_STARTUP_MARK?: (
    label: string,
    phase: string,
    pluginId: string,
    blocksSeen: number,
    blocksCopied: number,
    blocksExisting: number,
    blocksWritten: number,
    blocksDeduped: number,
    subtreesSkipped: number,
    logicalSourceBytes: number,
    destinationDurableBytes: number,
    destinationDurableBytesKnown: boolean,
    demandReadCount: number,
    demandReadBytes: number,
  ) => void
  BLDR_NOTIFY_PLUGIN_MANIFEST_ROOT?: (
    pluginId: string,
    rootHash: string,
  ) => void
  BLDR_NOTIFY_DURABLE_MUTATION?: () => void
}

const globalScope = self as unknown as Global
const goOpenStreamCtr = new OpenStreamCtr(undefined)
const goOpenStream = goOpenStreamCtr.openStreamFunc
const createDocCb: CreateWebDocumentFunc | null = null
const removeDocCb: RemoveWebDocumentFunc | null = null
const webRuntime = new WebRuntime(
  self.name,
  goOpenStream,
  createDocCb,
  removeDocCb,
)
globalScope.BLDR_NOTIFY_STARTUP_MARK = (
  label,
  phase,
  pluginId,
  blocksSeen,
  blocksCopied,
  blocksExisting,
  blocksWritten,
  blocksDeduped,
  subtreesSkipped,
  logicalSourceBytes,
  destinationDurableBytes,
  destinationDurableBytesKnown,
  demandReadCount,
  demandReadBytes,
) => {
  webRuntime.broadcastStartupMark(label, {
    source: 'scheduler',
    manifestCopyPhase: phase,
    pluginId,
    blocksSeen,
    blocksCopied,
    blocksExisting,
    blocksWritten,
    blocksDeduped,
    subtreesSkipped,
    logicalSourceBytes,
    destinationDurableBytes,
    destinationDurableBytesKnown,
    demandReadCount,
    demandReadBytes,
  })
}
globalScope.BLDR_NOTIFY_PLUGIN_MANIFEST_ROOT = (pluginId, rootHash) => {
  webRuntime.broadcastPluginManifestRoot(pluginId, rootHash)
}
globalScope.BLDR_NOTIFY_DURABLE_MUTATION = () => {
  webRuntime.broadcastDurableMutation()
}

const goOpenStreamChannel = new MessageChannel()
globalScope.BLDR_WEB_RUNTIME_CLIENT_OPEN = goOpenStreamChannel.port2
goOpenStreamChannel.port1.onmessage = (msg) => {
  const data = msg.data
  if (data !== 'open-stream') {
    console.warn('runtime-goscript: unexpected web runtime open msg', data)
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

// startGoScriptRuntime starts the process once, after its host configuration is installed.
async function startGoScriptRuntime(
  loadDistMain: GoScriptRuntimeMainLoader,
  webRuntimeId: string,
) {
  if (goStarted) {
    return
  }
  goStarted = true

  globalScope.BLDR_INIT = WebRuntimeHostInit.toBinary({
    webRuntimeId,
  })
  startGoRpcStreams()

  const distMain = await loadDistMain()
  await Promise.resolve()
    .then(() => distMain())
    .then(
      () => {
        throw new Error('GoScript browser runtime exited')
      },
      (err) => {
        throw err
      },
    )
}

// runtimeOpfsBridge brokers a DedicatedWorker OPFS bridge when the runtime runs
// in a SharedWorker, which cannot call navigator.storage.getDirectory(). A
// dedicated Worker reaches OPFS directly and needs no bridge.
const runtimeOpfsBridge = isSharedWorker
  ? new RuntimeOpfsBridge(self.name)
  : null

let runtimeStarted = false
// runGoScriptRuntime binds document ports and starts Go after OPFS is available.
export default function runGoScriptRuntime(
  loadDistMain: GoScriptRuntimeMainLoader,
) {
  // handlePortMessage connects documents and establishes the process storage bridge.
  function handlePortMessage(msgEvent: MessageEvent) {
    if (msgEvent.data === 'close') {
      return
    }

    const msg: WebDocumentToWebRuntime = msgEvent.data
    if (typeof msg !== 'object' || !msg.from) {
      console.log(
        'runtime-goscript: dropped invalid document to web runtime message',
        msg,
      )
      return
    }

    if (runtimeOpfsBridge && msg.opfsBrokerPort) {
      runtimeOpfsBridge.addWebDocument(msg.from, msg.opfsBrokerPort)
    }

    if (msg.initWebRuntime?.webRuntimeId && !runtimeStarted) {
      const webRuntimeId = msg.initWebRuntime.webRuntimeId
      void (async () => {
        // SharedWorker cannot open OPFS directly. A document must provide its
        // bridge before Go mounts volumes; retry when another document connects.
        if (runtimeOpfsBridge && !(await runtimeOpfsBridge.ensureBridge())) {
          console.warn(
            'runtime-goscript: OPFS bridge unavailable; deferring Go start until a document can host OPFS',
          )
          return
        }
        if (runtimeStarted) {
          return
        }
        runtimeStarted = true
        await startGoScriptRuntime(loadDistMain, webRuntimeId)
      })().catch((err) => {
        console.warn('runtime-goscript: error running web runtime', err)
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
    self.addEventListener('connect', (ev) => {
      const port = ev.ports?.[0]
      if (!port) {
        return
      }
      port.onmessage = handlePortMessage
      port.start()
    })
    return
  }

  self.onmessage = (msgEvent: MessageEvent) => {
    const port = msgEvent.ports?.[0]
    if (port) {
      port.onmessage = handlePortMessage
      port.start()
    }
    if (
      msgEvent.data &&
      typeof msgEvent.data === 'object' &&
      msgEvent.data.from
    ) {
      handlePortMessage(msgEvent)
    }
  }
}
