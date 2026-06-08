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

export type GoScriptRuntimeMain = () => void | Promise<void>

const isSharedWorker =
  typeof SharedWorkerGlobalScope !== 'undefined' &&
  self instanceof SharedWorkerGlobalScope

declare let self: SharedWorkerGlobalScope & DedicatedWorkerGlobalScope

interface Global {
  BLDR_INIT?: Uint8Array
  BLDR_WEB_RUNTIME_CLIENT_OPEN?: MessagePort
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

const goOpenStreamChannel = new MessageChannel()
globalScope.BLDR_WEB_RUNTIME_CLIENT_OPEN = goOpenStreamChannel.port2
goOpenStreamChannel.port1.onmessage = (msg) => {
  const data = msg.data
  if (data !== 'open-stream') {
    console.warn('runtime-goscript: unexpected web runtime open msg', data)
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
async function startGoScriptRuntime(
  distMain: GoScriptRuntimeMain,
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

let runtimeStarted = false
export default function runGoScriptRuntime(distMain: GoScriptRuntimeMain) {
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

    if (msg.initWebRuntime?.webRuntimeId && !runtimeStarted) {
      runtimeStarted = true
      startGoScriptRuntime(distMain, msg.initWebRuntime.webRuntimeId).catch(
        (err) => {
          console.warn('runtime-goscript: error running web runtime', err)
        },
      )
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
