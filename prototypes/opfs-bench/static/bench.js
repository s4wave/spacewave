// OPFS Benchmark Suite - OQ-9 Validation
// Measures: async write, sync write, block read, prefix scan, WebLock contention.

const log = document.getElementById('log')
const runBtn = document.getElementById('runAll')

function print(msg) {
  log.textContent += msg + '\n'
  log.scrollTop = log.scrollHeight
}

function stats(times) {
  const sorted = [...times].sort((a, b) => a - b)
  const n = sorted.length
  const sum = sorted.reduce((a, b) => a + b, 0)
  return {
    n,
    mean: sum / n,
    median: sorted[Math.floor(n / 2)],
    p95: sorted[Math.floor(n * 0.95)],
    p99: sorted[Math.floor(n * 0.99)],
    min: sorted[0],
    max: sorted[n - 1],
  }
}

function fmtStats(s) {
  const f = (v) => v.toFixed(3)
  return `  n=${s.n}  mean=${f(s.mean)}ms  median=${f(s.median)}ms  p95=${f(s.p95)}ms  p99=${f(s.p99)}ms  min=${f(s.min)}ms  max=${f(s.max)}ms`
}

const BENCH_DIR = '.opfs-bench'

async function getDir(sub) {
  const root = await navigator.storage.getDirectory()
  const benchDir = await root.getDirectoryHandle(BENCH_DIR, { create: true })
  return benchDir.getDirectoryHandle(sub, { create: true })
}

async function rmDir(dirHandle) {
  for await (const name of dirHandle.keys()) {
    await dirHandle.removeEntry(name, { recursive: true })
  }
}

// B1: Async write via createWritable()
async function benchAsyncWrite(count, sizeBytes) {
  print(`\n--- B1: Async Write (createWritable) ---`)
  print(`  files=${count}  size=${sizeBytes}B`)
  const dir = await getDir('async-write')
  await rmDir(dir)

  const data = new Uint8Array(sizeBytes)
  crypto.getRandomValues(data)
  const blob = new Blob([data])
  const times = []

  for (let i = 0; i < count; i++) {
    const name = 'f' + i.toString(36).padStart(4, '0')
    const fh = await dir.getFileHandle(name, { create: true })
    const t0 = performance.now()
    const w = await fh.createWritable()
    await w.write(blob)
    await w.close()
    times.push(performance.now() - t0)
  }

  const s = stats(times)
  print(fmtStats(s))
  return s
}

// B2: Sync write via createSyncAccessHandle (DedicatedWorker)
async function benchSyncWrite(count, sizeBytes) {
  print(`\n--- B2: Sync Write (createSyncAccessHandle) ---`)
  print(`  files=${count}  size=${sizeBytes}B`)
  const dir = await getDir('sync-write')
  await rmDir(dir)

  const worker = new Worker('sync-worker.js')
  const result = await new Promise((resolve) => {
    worker.onmessage = (ev) => resolve(ev.data)
    worker.postMessage({ cmd: 'sync-write', dir: BENCH_DIR + '/sync-write', count, sizeBytes })
  })
  worker.terminate()

  if (!result.ok) {
    print(`  ERROR: ${result.error}`)
    return null
  }

  const s = stats(result.times)
  print(fmtStats(s))
  return s
}

// B3: Async read via getFile() + arrayBuffer()
async function benchAsyncRead(count, sizeBytes) {
  print(`\n--- B3: Async Read (getFile + arrayBuffer) ---`)
  print(`  files=${count}  size=${sizeBytes}B`)

  // Ensure files exist (write first).
  const dir = await getDir('async-read')
  await rmDir(dir)
  const data = new Uint8Array(sizeBytes)
  crypto.getRandomValues(data)
  const blob = new Blob([data])
  for (let i = 0; i < count; i++) {
    const name = 'f' + i.toString(36).padStart(4, '0')
    const fh = await dir.getFileHandle(name, { create: true })
    const w = await fh.createWritable()
    await w.write(blob)
    await w.close()
  }

  // Benchmark reads.
  const times = []
  for (let i = 0; i < count; i++) {
    const name = 'f' + i.toString(36).padStart(4, '0')
    const fh = await dir.getFileHandle(name)
    const t0 = performance.now()
    const file = await fh.getFile()
    const buf = await file.arrayBuffer()
    void buf // ensure read completes
    times.push(performance.now() - t0)
  }

  const s = stats(times)
  print(fmtStats(s))
  return s
}

// B3b: Sync read via createSyncAccessHandle (DedicatedWorker)
async function benchSyncRead(count, sizeBytes) {
  print(`\n--- B3b: Sync Read (createSyncAccessHandle) ---`)
  print(`  files=${count}  size=${sizeBytes}B`)

  // Ensure files exist in the sync-write dir (reuse from B2 or write new).
  const dir = await getDir('sync-read')
  await rmDir(dir)
  const writeWorker = new Worker('sync-worker.js')
  await new Promise((resolve) => {
    writeWorker.onmessage = (ev) => resolve(ev.data)
    writeWorker.postMessage({ cmd: 'sync-write', dir: BENCH_DIR + '/sync-read', count, sizeBytes })
  })
  writeWorker.terminate()

  // Benchmark sync reads.
  const worker = new Worker('sync-worker.js')
  const result = await new Promise((resolve) => {
    worker.onmessage = (ev) => resolve(ev.data)
    worker.postMessage({ cmd: 'sync-read', dir: BENCH_DIR + '/sync-read', count, sizeBytes })
  })
  worker.terminate()

  if (!result.ok) {
    print(`  ERROR: ${result.error}`)
    return null
  }

  const s = stats(result.times)
  print(fmtStats(s))
  return s
}

// B4: Prefix scan (directory listing + sort)
async function benchPrefixScan(counts) {
  print(`\n--- B4: Prefix Scan (directory listing + sort) ---`)

  for (const count of counts) {
    const subDir = 'scan-' + count
    const dir = await getDir(subDir)
    await rmDir(dir)

    // Create N files with sortable names.
    const data = new Uint8Array(64)
    const blob = new Blob([data])
    for (let i = 0; i < count; i++) {
      const name = 'k' + i.toString(36).padStart(6, '0')
      const fh = await dir.getFileHandle(name, { create: true })
      const w = await fh.createWritable()
      await w.write(blob)
      await w.close()
    }

    // Benchmark: list all entries + sort.
    const iterations = Math.max(1, Math.floor(50 / Math.max(1, count / 100)))
    const times = []
    for (let r = 0; r < iterations; r++) {
      const t0 = performance.now()
      const names = []
      for await (const name of dir.keys()) {
        names.push(name)
      }
      names.sort()
      times.push(performance.now() - t0)
    }

    const s = stats(times)
    print(`  count=${count}` + fmtStats(s))
  }
}

// B5: WebLock acquisition latency under contention
async function benchWebLockContention(workers, count, sizeBytes) {
  print(`\n--- B5: WebLock Contention (${workers} workers, exclusive) ---`)
  print(`  workers=${workers}  filesPerWorker=${count}  size=${sizeBytes}B`)

  const dir = await getDir('lock-contention')
  await rmDir(dir)

  const lockName = 'opfs-bench-lock-' + Date.now()
  const promises = []

  for (let w = 0; w < workers; w++) {
    const worker = new Worker('sync-worker.js')
    promises.push(
      new Promise((resolve) => {
        worker.onmessage = (ev) => {
          worker.terminate()
          resolve(ev.data)
        }
        worker.postMessage({
          cmd: 'contended-lock-write',
          dir: BENCH_DIR + '/lock-contention',
          count,
          sizeBytes,
          lockName,
        })
      }),
    )
  }

  const results = await Promise.all(promises)
  const lockTimes = results.filter((r) => r.ok).map((r) => r.lockAcquired)

  if (lockTimes.length === 0) {
    print(`  ERROR: all workers failed`)
    for (const r of results) {
      if (!r.ok) print(`    ${r.error}`)
    }
    return
  }

  const s = stats(lockTimes)
  print(`  lock acquisition:` + fmtStats(s))

  // Also report per-file write times across all workers.
  const allWriteTimes = results.filter((r) => r.ok).flatMap((r) => r.writeTimes)
  if (allWriteTimes.length > 0) {
    const ws = stats(allWriteTimes)
    print(`  per-file write:` + fmtStats(ws))
  }
}

// Summary verdict
function verdict(results) {
  print(`\n${'='.repeat(60)}`)
  print(`SUMMARY`)
  print(`${'='.repeat(60)}`)

  const threshold = 1.0 // ms
  let pass = true

  for (const [label, s] of Object.entries(results)) {
    if (!s) continue
    const ok = s.p95 < threshold
    if (!ok) pass = false
    const tag = ok ? 'PASS' : 'FAIL'
    print(`  [${tag}] ${label}: p95=${s.p95.toFixed(3)}ms (threshold: ${threshold}ms)`)
  }

  print('')
  if (pass) {
    print('VERDICT: file-per-key OPFS design is viable. No batching needed for day one.')
  } else {
    print('VERDICT: some operations exceed 1ms p95. Consider batching or kvfile compaction.')
  }
}

window.runAll = async function () {
  runBtn.disabled = true
  log.textContent = ''
  print('OPFS Benchmark Suite - OQ-9')
  print(`crossOriginIsolated: ${self.crossOriginIsolated}`)
  print(`userAgent: ${navigator.userAgent}`)
  print(`date: ${new Date().toISOString()}`)

  const results = {}

  // Small files (typical block refs, object store entries): 64-256 bytes
  results['B1 async-write 100x256B'] = await benchAsyncWrite(100, 256)
  results['B2 sync-write 100x256B'] = await benchSyncWrite(100, 256)
  results['B3 async-read 100x256B'] = await benchAsyncRead(100, 256)
  results['B3b sync-read 100x256B'] = await benchSyncRead(100, 256)

  // Larger files (block data): 4KB, 64KB
  results['B1 async-write 50x4KB'] = await benchAsyncWrite(50, 4096)
  results['B2 sync-write 50x4KB'] = await benchSyncWrite(50, 4096)
  results['B3 async-read 50x4KB'] = await benchAsyncRead(50, 4096)

  results['B1 async-write 20x64KB'] = await benchAsyncWrite(20, 65536)
  results['B2 sync-write 20x64KB'] = await benchSyncWrite(20, 65536)
  results['B3 async-read 20x64KB'] = await benchAsyncRead(20, 65536)

  // Prefix scan at varying directory sizes
  await benchPrefixScan([100, 1000, 5000])

  // WebLock contention
  await benchWebLockContention(4, 10, 256)
  await benchWebLockContention(8, 5, 256)

  verdict(results)

  // Store for programmatic access.
  window.__benchResults = results
  runBtn.disabled = false
}

window.cleanup = async function () {
  print('\nCleaning up OPFS benchmark data...')
  const root = await navigator.storage.getDirectory()
  try {
    await root.removeEntry(BENCH_DIR, { recursive: true })
    print('Done.')
  } catch (e) {
    print('Cleanup: ' + e.message)
  }
}
