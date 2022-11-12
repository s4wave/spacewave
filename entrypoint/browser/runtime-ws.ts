import {
  WebRuntimeClientInit,
  WebRuntimeHostInit,
} from '../../web/runtime/runtime.pb.js'
import {
  CreateWebDocumentFunc,
  RemoveWebDocumentFunc,
  WebRuntime,
} from '../../web/bldr/web-runtime.js'

import { duplex } from 'it-ws'
import { pipe } from 'it-pipe'
import { MessagePortIterable } from 'starpc'

// https://github.com/microsoft/TypeScript/issues/14877
declare let self: SharedWorkerGlobalScope

const connAddr = `ws://${self.location.host}/bldr-dev/web-runtime.ws`

// TODO: create a new tab / window?
const createDocCb: CreateWebDocumentFunc | null = null
const removeDocCb: RemoveWebDocumentFunc | null = null
const workerHost = new WebRuntime(
  `shared-worker:${self.location.host}`,
  createDocCb,
  removeDocCb
)
const runtimePort = workerHost.goRuntimePort
const runtimePortIterable = new MessagePortIterable<Uint8Array>(runtimePort)

async function connectWebsocket(address: string): Promise<WebSocket> {
  const ws = new WebSocket(address)
  return new Promise<WebSocket>((resolve, reject) => {
    ws.onclose = ev => {
      reject(new Error(ev.reason))
    }
    ws.onopen = _ => {
      resolve(ws)
    }
  })
}

async function startWsRuntime(msg: WebRuntimeHostInit) {
  console.log(`bldr: connecting to ${connAddr} as WebRuntime: ${msg.webRuntimeId}`)
  const ws = await connectWebsocket(connAddr)
  const wsDuplex = duplex(ws)
  pipe(wsDuplex, runtimePortIterable, wsDuplex)
}

async function startWsRuntimeWithRetry(msg: WebRuntimeHostInit) {
  startWsRuntime(msg).catch((e) => {
    console.error('start runtime failed, will retry', e)
    setTimeout(() => {
      startWsRuntimeWithRetry(msg)
    }, 1000)
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
        msg
      )
      return
    }
    const connPort = msgEvent.ports[0]
    workerHost.handleClient(initMsg, connPort)
    if (!runtimeStarted) {
      if (!initMsg.webRuntimeId) {
        throw new Error('web runtime id: must be set in init message')
      }
      runtimeStarted = true
      startWsRuntimeWithRetry({
        webRuntimeId: initMsg.webRuntimeId,
      })
    }
  }
  port.start()
})
