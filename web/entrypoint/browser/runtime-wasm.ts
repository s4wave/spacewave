import { MessagePortConn, OpenStreamCtr } from 'starpc'

import {
  WebRuntimeClientInit,
  WebRuntimeHostInit,
} from '../../runtime/runtime.pb.js'
import {
  CreateWebDocumentFunc,
  RemoveWebDocumentFunc,
  WebRuntime,
} from '../../bldr/web-runtime.js'

// https://github.com/microsoft/TypeScript/issues/14877
declare let self: SharedWorkerGlobalScope
interface Global extends SharedWorkerGlobalScope {
  BLDR_INIT?: Uint8Array
  BLDR_WEB_RUNTIME_CLIENT_OPEN?: MessagePort
}
const global: Global = self

// See wasm_exec.js
declare class Go {
  importObject: WebAssembly.Imports
  env: Record<string, string>
  argv?: string[]
  exit?(code: number): void
  run(inst: WebAssembly.Module): Promise<void>
}

// openStreamCtr will contain the runtime open stream func.
const openStreamCtr = new OpenStreamCtr(undefined)
// openStreamFunc is a function that waits for OpenStreamFunc, then calls it.
const openStreamFunc = openStreamCtr.openStreamFunc

// TODO: how to create a new tab / window?
const createDocCb: CreateWebDocumentFunc | null = null
const removeDocCb: RemoveWebDocumentFunc | null = null
const workerHost = new WebRuntime(
  // TODO: should this runtime be from the init message instead?
  `shared-worker:${self.location.host}`,
  openStreamFunc,
  createDocCb,
  removeDocCb,
)

async function startWasmRuntime(msg: WebRuntimeHostInit) {
  // clear any existing open stream func
  openStreamCtr.set(undefined)

  console.log(`bldr: starting wasm runtime: ${msg.webRuntimeId}`)
  const go = new Go()

  let mod: WebAssembly.Module
  let inst: WebAssembly.Instance
  async function run() {
    await go.run(inst)
    inst = await WebAssembly.instantiate(mod, go.importObject) // reset instance
  }
  const payload = await fetch('/runtime/runtime.wasm')
  if (!payload.ok) {
    throw new Error(payload.statusText)
  }
  WebAssembly.instantiateStreaming(payload, go.importObject)
    .then((result) => {
      mod = result.module
      inst = result.instance

      // Setup the connection to the Go runtime.
      const workerChannel = new MessageChannel()
      const workerPort = workerChannel.port1
      const runtimeConn = new MessagePortConn(
        workerPort,
        workerHost.getWebRuntimeServer(),
        {
          direction: 'inbound',
        },
      )
      const runtimePort = workerChannel.port2
      const openStream = runtimeConn.buildOpenStreamFunc()

      // pass via global, use syscall/js to retrieve
      global.BLDR_INIT = WebRuntimeHostInit.encode(msg).finish()
      global.BLDR_WEB_RUNTIME_CLIENT_OPEN = runtimePort

      // start the runtime
      run()

      // start sending rpc requests
      openStreamCtr.set(openStream)
    })
    .catch((err) => {
      console.error(err)
    })
}

async function startWasmRuntimeWithRetry(msg: WebRuntimeHostInit) {
  startWasmRuntime(msg).catch((e) => {
    console.error('start runtime failed, will retry', e)
    // clear any existing open stream func
    openStreamCtr.set(undefined)
    setTimeout(() => {
      startWasmRuntimeWithRetry(msg)
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
      startWasmRuntimeWithRetry({
        webRuntimeId: initMsg.webRuntimeId,
      })
    }
  }
  port.start()
})
