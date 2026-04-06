// Main-side ping-pong worker: sends ping via Atomics, waits for pong.
// This runs in a DedicatedWorker because Atomics.wait is blocked on the main thread.
onmessage = (e) => {
  const { sab, count } = e.data
  const ctrl = new Int32Array(sab)
  postMessage('ready')

  const start = performance.now()
  for (let i = 0; i < count; i++) {
    Atomics.store(ctrl, 0, i + 1)
    Atomics.notify(ctrl, 0)
    Atomics.wait(ctrl, 1, i)
  }
  const elapsed = performance.now() - start
  postMessage({ elapsed, latencyUs: (elapsed / count) * 1000 })
}
