// SAB Atomics ping-pong worker.
// Main thread writes to ctrl[0] and notifies, worker responds via ctrl[1].
onmessage = (e) => {
  const { sab, count } = e.data
  const ctrl = new Int32Array(sab)
  postMessage('ready')

  for (let i = 0; i < count; i++) {
    // Wait for main thread to write i+1 to ctrl[0].
    Atomics.wait(ctrl, 0, i)
    // Respond.
    Atomics.store(ctrl, 1, i + 1)
    Atomics.notify(ctrl, 1)
  }
}
