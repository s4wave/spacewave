// SAB ring buffer reader worker.
// Receives SAB, reads messages from ring buffer, notifies when done.
let config = null

onmessage = (e) => {
  config = e.data
  postMessage('ready')
  runReader()
}

function runReader() {
  const { sab, msgSize, slots, count } = config
  const ctrl = new Int32Array(sab, 0, 3)
  const ring = new Uint8Array(sab, 12)
  const buf = new Uint8Array(msgSize)

  let read = 0
  while (read < count) {
    // Wait for data.
    while (Atomics.load(ctrl, 0) <= read) {
      Atomics.wait(ctrl, 2, 0, 1) // 1ms timeout
      Atomics.store(ctrl, 2, 0)
    }
    const rIdx = read % slots
    // Read into local buffer (simulates deserialization cost).
    buf.set(ring.subarray(rIdx * msgSize, (rIdx + 1) * msgSize))
    read++
    Atomics.store(ctrl, 1, read)
  }
  postMessage('done')
}
