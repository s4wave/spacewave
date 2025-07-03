import { yamux } from '@chainsafe/libp2p-yamux'
import {
  OpenStreamCtr,
  StreamConn,
  combineUint8ArrayListTransform,
} from 'starpc'
import { pipe } from 'it-pipe'

import { duplex } from '@aptre/it-ws'

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
const webRuntime = new WebRuntime(
  `shared-worker:${self.location.host}`,
  openStreamFunc,
  createDocCb,
  removeDocCb,
)

async function connectWebsocket(address: string): Promise<WebSocket> {
  const ws = new WebSocket(address)
  return new Promise<WebSocket>((resolve, reject) => {
    ws.onclose = (ev) => {
      reject(new Error(ev.reason))
    }
    ws.onopen = () => {
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
  ws.onclose = () => {
    // re-start after close
    console.warn('bldr: websocket closed, restarting')
    openStreamCtr.set(undefined)
    startWsRuntimeWithRetry(msg)
  }

  // Setup the connection to the Go runtime.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const wsDuplex = duplex(ws as any)
  const runtimeConn = new StreamConn(webRuntime.getWebRuntimeServer(), {
    direction: 'inbound',
    yamuxParams: {
      enableKeepAlive: false,
      maxMessageSize: 32 * 1024,
    },
  })
  const openStream = runtimeConn.buildOpenStreamFunc()
  pipe(wsDuplex, runtimeConn, combineUint8ArrayListTransform(), wsDuplex)
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
const runtimeStarted = false
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
    if (msgEvent.data === 'close') {
      port.close()
      return
    }

    const msg: WebDocumentToWebRuntime = msgEvent.data
    if (typeof msg !== 'object' || !msg.from) {
      console.log(
        'runtime-ws: dropped invalid document to web runtime message',
        msg,
      )
      return
    }

    if (msg.initWebRuntime?.webRuntimeId && !runtimeStarted) {
      startWsRuntime(msg.initWebRuntime!)
    }

    if (msg.connectWebRuntime && ev.ports.length) {
      // handle the incoming client
      webRuntime.handleClient(
        WebRuntimeClientInit.fromBinary(msg.connectWebRuntime.init),
        msg.connectWebRuntime.port ?? ev.ports[0],
      )
    }
  }

  port.start()
})
