// Note: this is replaced with Go from wasm_exec.js.
declare class Go {
  importObject: any
  env: Object
  argv?: string[]
  exit?(code: number): void
  run(inst: WebAssembly.Module): Promise<void>
}

async function startRuntime(msg: Uint8Array) {
  console.log('bldr: starting wasm runtime')
  const go = new Go()
  /*
  go.env = Object.assign(go.env, {
    BLDR_INIT: base64.encode(msg),
  })
  */
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
      run()
    })
    .catch((err) => {
      console.error(err)
    })
}

async function startRuntimeWithRetry(msg: Uint8Array) {
  startRuntime(msg).catch((e) => {
    console.error('start runtime failed, will retry', e)
    setTimeout(() => {
      startRuntimeWithRetry(msg)
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
  if (!runtimeStarted) {
    onmessage = undefined
    runtimeStarted = true
    startRuntimeWithRetry(msg)
  }
}
