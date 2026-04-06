// Snapshot worker: simulates a plugin with WASM memory.
// Responds to 'alloc', 'snapshot-now' (OPFS), 'snapshot-idb' commands.

let memory = null
let memChecksum = 0

postMessage('ready')

onmessage = async (e) => {
  const { type } = e.data

  if (type === 'alloc') {
    const bytes = e.data.sizeMB * 1024 * 1024
    memory = new ArrayBuffer(bytes)
    const view = new Uint8Array(memory)
    for (let off = 0; off < view.byteLength; off += 65536) {
      crypto.getRandomValues(view.subarray(off, Math.min(off + 65536, view.byteLength)))
    }
    memChecksum = 0
    for (let i = 0; i < view.byteLength; i++) {
      memChecksum = (memChecksum + view[i]) | 0
    }
    postMessage({ type: 'allocated', checksum: memChecksum, sizeBytes: bytes })
  }

  if (type === 'snapshot-now') {
    // Urgent snapshot to OPFS.
    const start = performance.now()
    const snap = new Uint8Array(memory)
    try {
      const root = await navigator.storage.getDirectory()
      const fh = await root.getFileHandle('urgent-snapshot.bin', { create: true })
      const writable = await fh.createWritable()
      await writable.write(snap)
      await writable.close()
    } catch (err) {
      postMessage({ type: 'snapshot-error', error: err.message })
      return
    }
    const ms = performance.now() - start
    postMessage({ type: 'snapshot-done', snapshotMs: ms, sizeBytes: snap.byteLength })
  }

  if (type === 'snapshot-idb') {
    const snap = new Uint8Array(memory)
    const start = performance.now()
    await new Promise((resolve, reject) => {
      const req = indexedDB.open('snapshot-bench', 1)
      req.onupgradeneeded = () => {
        req.result.createObjectStore('snaps')
      }
      req.onsuccess = () => {
        const db = req.result
        const tx = db.transaction('snaps', 'readwrite')
        const store = tx.objectStore('snaps')
        const putReq = store.put(snap.buffer, 'urgent')
        putReq.onsuccess = () => {
          db.close()
          indexedDB.deleteDatabase('snapshot-bench')
          resolve(undefined)
        }
        putReq.onerror = () => reject(putReq.error)
      }
    })
    const ms = performance.now() - start
    postMessage({ type: 'idb-done', idbWriteMs: ms })
  }
}
