var global: any = self

async function startRuntime(msg: Uint8Array, port: MessagePort) {
  console.log('bldr: starting js runtime')

  // pass via global, use syscall/js to retrieve
  global.BLDR_INIT = msg
  global.BLDR_PORT = port

  importScripts('./runtime-gopherjs.js')
}

async function startRuntimeWithRetry(msg: Uint8Array, port: MessagePort) {
  startRuntime(msg, port).catch((e) => {
    console.error('start runtime failed, will retry', e)
    setTimeout(() => {
      startRuntimeWithRetry(msg, port)
    }, 1000)
  })
}

// wait for startup / init command
var runtimeStarted = false
onmessage = (ev: MessageEvent) => {
  const msg = ev.data
  if (typeof msg !== 'object' || !(msg instanceof Uint8Array)) {
    return
  }
  const ports = ev.ports
  if (!ports || !ports.length) {
    return
  }
  const port = ports[0]
  if (!runtimeStarted) {
    onmessage = undefined
    runtimeStarted = true
    startRuntimeWithRetry(msg, port)
  }
}
