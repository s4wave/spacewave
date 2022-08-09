import {
  WebRuntimeClientInit,
  WebRuntimeHostInit,
} from '../../web/runtime/runtime.pb.js'
import {
  CreateWebDocumentFunc,
  RemoveWebDocumentFunc,
  WebRuntime,
} from '../../web/bldr/web-runtime.js'

// https://github.com/microsoft/TypeScript/issues/14877
declare let self: SharedWorkerGlobalScope
const global: any = self

// TODO: create a new tab / window?
const createDocCb: CreateWebDocumentFunc | null = null
const removeDocCb: RemoveWebDocumentFunc | null = null
const workerHost = new WebRuntime(
  `shared-worker:${self.location.host}`,
  createDocCb,
  removeDocCb
)
const runtimePort = workerHost.goRuntimePort

// See wasm_exec.js
declare class Go {
  importObject: WebAssembly.Imports
  env: Record<string, string>
  argv?: string[]
  exit?(code: number): void
  run(inst: WebAssembly.Module): Promise<void>
}

async function startWasmRuntime(msg: WebRuntimeHostInit) {
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

      // pass via global, use syscall/js to retrieve
      global.BLDR_INIT = WebRuntimeHostInit.encode(msg).finish()
      global.BLDR_PORT = runtimePort
      run()
    })
    .catch((err) => {
      console.error(err)
    })
}

async function startWasmRuntimeWithRetry(msg: WebRuntimeHostInit) {
  startWasmRuntime(msg).catch((e) => {
    console.error('start runtime failed, will retry', e)
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
      startWasmRuntimeWithRetry({
        webRuntimeId: initMsg.webRuntimeId,
      })
    }
  }
  port.start()
})
