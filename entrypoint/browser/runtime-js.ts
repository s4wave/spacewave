async function startRuntime(msg: Uint8Array) {
  console.log('bldr: starting js runtime')

  // pass via global, use syscall/js to retrieve
  var global = self
  global.BLDR_INIT = msg

  importScripts('./runtime-gopherjs.js')
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
