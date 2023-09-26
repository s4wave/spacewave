import { yamux } from '@chainsafe/libp2p-yamux'
import { OpenStreamCtr, Conn } from 'starpc'

import {
  WebRuntimeClientInit,
  WebRuntimeHostInit,
} from '../../runtime/runtime.pb.js'
import {
  CreateWebDocumentFunc,
  RemoveWebDocumentFunc,
  WebRuntime,
} from '../../bldr/web-runtime.js'

import { duplex } from 'it-ws'
import WebSocket from 'it-ws/dist/src/web-socket.js'

import { pipe } from 'it-pipe'
// import { MessagePortIterable } from 'starpc'

// https://github.com/microsoft/TypeScript/issues/14877
declare let self: SharedWorkerGlobalScope

const connAddr = `ws://${self.location.host}/bldr-dev/web-runtime.ws`

// openStreamCtr will contain the runtime open stream func.
const openStreamCtr = new OpenStreamCtr(undefined)
// openStreamFunc is a function that waits for OpenStreamFunc, then calls it.
const openStreamFunc = openStreamCtr.openStreamFunc

// TODO: create a new tab / window?
const createDocCb: CreateWebDocumentFunc | null = null
const removeDocCb: RemoveWebDocumentFunc | null = null
const workerHost = new WebRuntime(
  `shared-worker:${self.location.host}`,
  openStreamFunc,
  createDocCb,
  removeDocCb,
)

// const runtimePort = workerHost.goRuntimePort
// const runtimePortIterable = new MessagePortIterable<Uint8Array>(runtimePort)

async function connectWebsocket(address: string): Promise<WebSocket> {
  const ws = new WebSocket(address)
  return new Promise<WebSocket>((resolve, reject) => {
    ws.onclose = (ev) => {
      reject(new Error(ev.reason))
    }
    ws.onopen = (_) => {
      resolve(ws)
    }
  })
}

async function startWsRuntime(msg: WebRuntimeHostInit) {
  // clear any existing open stream func
  openStreamCtr.set(undefined)
  console.log(
    `bldr: connecting to ${connAddr} as WebRuntime: ${msg.webRuntimeId}`,
  )
  const ws = await connectWebsocket(connAddr)
  ws.onclose = (_) => {
    // re-start after close
    console.warn('bldr: websocket closed, restarting')
    openStreamCtr.set(undefined)
    startWsRuntimeWithRetry(msg)
  }

  // Setup the connection to the Go runtime.
  const wsDuplex = duplex(ws)
  const runtimeConn = new Conn(workerHost.getWebRuntimeServer(), {
    direction: 'inbound',
    muxerFactory: yamux({
      // server side does keep-alive at 5000ms
      enableKeepAlive: true,
      keepAliveInterval: 2500,
      maxMessageSize: 32 * 1024,
    })(),
  })
  const openStream = runtimeConn.buildOpenStreamFunc()
  pipe(wsDuplex, runtimeConn, wsDuplex)
  openStreamCtr.set(openStream)
}

async function startWsRuntimeWithRetry(msg: WebRuntimeHostInit) {
  startWsRuntime(msg).catch((e) => {
    openStreamCtr.set(undefined)
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
        msg,
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
