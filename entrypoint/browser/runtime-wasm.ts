// Note: this is replaced with Go from wasm_exec.js.
declare class Go {
  importObject: any
  env: Object
  argv?: string[]
  exit?(code: number): void
  run(inst: WebAssembly.Module): Promise<void>
}

var global: any = self
async function startWasmRuntime(msg: Uint8Array, port: MessagePort) {
  console.log('bldr: starting wasm runtime')
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
      global.BLDR_INIT = msg
      global.BLDR_PORT = port
      run()
    })
    .catch((err) => {
      console.error(err)
    })
}

async function startWasmRuntimeWithRetry(msg: Uint8Array, port: MessagePort) {
  startWasmRuntime(msg, port).catch((e) => {
    console.error('start runtime failed, will retry', e)
    setTimeout(() => {
      startWasmRuntimeWithRetry(msg, port)
    }, 1000)
  })
}

// wait for startup / init command
var runtimeStarted = false
onmessage = (ev) => {
  const msg = ev.data
  if (typeof msg !== 'object' || !(msg instanceof Uint8Array)) {
    return
  }
  const ports = ev.ports
  if (!ports || !ports.length) {
    return
  }
  const port = ev.ports[0]
  if (!port) {
    return
  }
  if (!runtimeStarted) {
    onmessage = null
    runtimeStarted = true
    startWasmRuntimeWithRetry(msg, port)
  }
}
