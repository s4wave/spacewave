// Note: this is replaced with Go from wasm_exec.js.
declare class Go {
  importObject: any
  run(inst: WebAssembly.Module): Promise<void>
}

async function startRuntime() {
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
      run()
    })
    .catch((err) => {
      console.error(err)
    })
}

async function startRuntimeWithRetry() {
  startRuntime().catch((e) => {
    console.error('start runtime failed, will retry', e)
    setTimeout(() => {
      startRuntimeWithRetry()
    }, 1000)
  })
}

// wait for startup / init command
var runtimeStarted = false
onmessage = ev => {
  const msg = ev.data
  if (typeof msg !== 'string') {
    return
  }
  if (!msg.startsWith('init:')) {
    return
  }
  if (!runtimeStarted) {
    onmessage = undefined
    runtimeStarted = true
    startRuntimeWithRetry()
  }
}
