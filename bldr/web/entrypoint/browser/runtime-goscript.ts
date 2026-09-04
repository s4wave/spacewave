import { ChannelStream, OpenStreamCtr, PacketStream } from 'starpc'

import { channelPacketStream } from '../../bldr/channel-packet-stream.js'

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
import { RuntimeOpfsBridge } from './runtime-opfs-bridge.js'

export type GoScriptRuntimeMain = () => void | Promise<void>
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
  const portDuplex = channelPacketStream(
    new ChannelStream('runtime-goscript', port, { remoteOpen: true }),
  )
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
    return channelPacketStream(
      new ChannelStream('runtime-goscript', streamChannel.port1, {
        remoteOpen: true,
      }),
    )
  })
}

let goStarted = false
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
export default function runGoScriptRuntime(
  loadDistMain: GoScriptRuntimeMainLoader,
) {
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
        // Install the OPFS bridge before the Go process starts so
        // RemoteDriver.GetRoot() finds the global port during volume mount. In a
        // SharedWorker the runtime cannot call getDirectory() itself, so it must
        // not start Go without the bridge (that crashes on the original OPFS
        // SecurityError). ensureBridge() retries across live documents; if none
        // can host yet, defer startup (leave runtimeStarted false) so a later
        // document's init drives it instead of wedging on a transient first-tab
        // failure. startGoScriptRuntime is itself idempotent, and runtimeStarted
        // flips only after the bridge precondition holds.
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
