// SAB ring buffer reader using pure Atomics.wait on value change (no timeout).
// Tests whether WebKit's Atomics.wait pathology is specific to timeout mode.
onmessage = (e) => {
  const { sab, msgSize, slots, count } = e.data
  const ctrl = new Int32Array(sab, 0, 3)
  const ring = new Uint8Array(sab, 12)
  const buf = new Uint8Array(msgSize)
  postMessage('ready')

  let read = 0
  while (read < count) {
    // Wait for write index to advance past our read position.
    // Uses Atomics.wait on ctrl[0] (write index) with expected value = read.
    // No timeout - wakes only on Atomics.notify from writer.
    while (Atomics.load(ctrl, 0) <= read) {
      Atomics.wait(ctrl, 0, read)
    }
    const rIdx = read % slots
    buf.set(ring.subarray(rIdx * msgSize, (rIdx + 1) * msgSize))
    read++
    Atomics.store(ctrl, 1, read)
  }
  postMessage('done')
}
