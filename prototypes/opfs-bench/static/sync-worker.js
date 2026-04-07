// DedicatedWorker for sync OPFS API benchmarks.
// createSyncAccessHandle() is only available in Worker contexts.

// Navigate an OPFS path like ".opfs-bench/sync-write" segment by segment.
async function getNestedDir(path) {
  let dir = await navigator.storage.getDirectory()
  for (const seg of path.split('/')) {
    if (!seg) continue
    dir = await dir.getDirectoryHandle(seg, { create: true })
  }
  return dir
}

self.onmessage = async (ev) => {
  const { cmd, dir: dirPath, count, sizeBytes } = ev.data
  try {
    const dir = await getNestedDir(dirPath)

    if (cmd === 'sync-write') {
      // Write N files using createSyncAccessHandle (sync API).
      const data = new Uint8Array(sizeBytes)
      crypto.getRandomValues(data)
      const times = []
      for (let i = 0; i < count; i++) {
        const name = 'f' + i.toString(36).padStart(4, '0')
        const fh = await dir.getFileHandle(name, { create: true })
        const t0 = performance.now()
        const sh = await fh.createSyncAccessHandle()
        sh.write(data, { at: 0 })
        sh.flush()
        sh.close()
        times.push(performance.now() - t0)
      }
      self.postMessage({ ok: true, times })
    } else if (cmd === 'sync-read') {
      // Read N files using createSyncAccessHandle (sync API).
      const times = []
      for (let i = 0; i < count; i++) {
        const name = 'f' + i.toString(36).padStart(4, '0')
        const fh = await dir.getFileHandle(name)
        const t0 = performance.now()
        const sh = await fh.createSyncAccessHandle()
        const size = sh.getSize()
        const buf = new Uint8Array(size)
        sh.read(buf, { at: 0 })
        sh.close()
        times.push(performance.now() - t0)
      }
      self.postMessage({ ok: true, times })
    } else if (cmd === 'contended-lock-write') {
      // Acquire exclusive WebLock then write files.
      const lockName = ev.data.lockName
      const t0 = performance.now()
      await navigator.locks.request(lockName, { mode: 'exclusive' }, async () => {
        const lockAcquired = performance.now() - t0
        const data = new Uint8Array(sizeBytes)
        crypto.getRandomValues(data)
        const writeTimes = []
        for (let i = 0; i < count; i++) {
          const name = 'f' + i.toString(36).padStart(4, '0')
          const fh = await dir.getFileHandle(name, { create: true })
          const tw0 = performance.now()
          const sh = await fh.createSyncAccessHandle()
          sh.write(data, { at: 0 })
          sh.flush()
          sh.close()
          writeTimes.push(performance.now() - tw0)
        }
        self.postMessage({ ok: true, lockAcquired, writeTimes })
      })
    } else {
      self.postMessage({ ok: false, error: 'unknown cmd: ' + cmd })
    }
  } catch (err) {
    self.postMessage({ ok: false, error: err.message })
  }
}
