import {
  average,
  cleanupBenchRoot,
  fmtStats,
  getDirectory,
  loadTableMetadata,
  rangeReadManySlices,
  rangeReadWholeSlice,
  resetPath,
  runLookupSeries,
  stats,
} from './bench-lib.js'

const log = document.getElementById('log')
const runBtn = document.getElementById('runAll')

function print(msg) {
  log.textContent += msg + '\n'
  log.scrollTop = log.scrollHeight
}

function section(title) {
  print(`\n=== ${title} ===`)
}

function repeatKeys(keys, count) {
  const out = []
  for (let i = 0; i < count; i++) {
    out.push(keys[i % keys.length])
  }
  return out
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

class BenchWorker {
  constructor() {
    this.worker = new Worker('worker.js', { type: 'module' })
    this.seq = 1
    this.pending = new Map()
    this.worker.onmessage = (ev) => {
      const pending = this.pending.get(ev.data.id)
      if (!pending) {
        return
      }
      this.pending.delete(ev.data.id)
      if (ev.data.ok) {
        pending.resolve(ev.data.result)
        return
      }
      pending.reject(new Error(ev.data.error))
    }
  }

  call(cmd, params) {
    const id = this.seq++
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject })
      this.worker.postMessage({ id, cmd, params })
    })
  }

  close() {
    this.worker.terminate()
  }
}

async function withWorker(fn) {
  const worker = new BenchWorker()
  try {
    return await fn(worker)
  } finally {
    worker.close()
  }
}

async function benchIntegrityPublishVisibility() {
  section('Integrity: publish visibility')
  const path = 'integrity/publish-visibility'
  await resetPath(path)
  const writer = new BenchWorker()
  const reader = new BenchWorker()
  const failures = []
  const latencies = []
  try {
    for (let i = 0; i < 40; i++) {
      const published = await writer.call('publish-once', {
        path,
        entryCount: 64,
        valueSize: 4096,
        keyBase: i * 1000,
        indexEvery: 16,
        falsePositiveRate: 0.01,
      })
      const validated = await reader.call('validate-latest', {
        path,
        expectedVersion: published.version,
        expectedName: published.name,
        expectedKey: published.expectedKey,
        expectedBytes: published.bytes,
      })
      if (!validated.ok) {
        failures.push(validated.reason)
      } else {
        latencies.push(validated.latencyMs)
      }
    }
  } finally {
    writer.close()
    reader.close()
  }
  const result = {
    iterations: 40,
    failures,
    readerLatency: stats(latencies),
  }
  print(`iterations=40 failures=${failures.length}`)
  print(`reader latency ${fmtStats(result.readerLatency)}`)
  if (failures.length) {
    print(`first failure: ${failures[0]}`)
  }
  return result
}

async function benchB1() {
  section('B-1 SSTable write throughput vs batch size')
  const batchSizes = [1, 10, 50, 100, 500]
  return withWorker(async (worker) => {
    const scenarios = []
    for (const batchSize of batchSizes) {
      const result = await worker.call('publish-batch-series', {
        path: 'b1',
        batchSize,
        repeats: 6,
        valueSize: 4096,
        indexEvery: 16,
        falsePositiveRate: 0.01,
      })
      scenarios.push(result)
      print(`batch=${batchSize} total ${fmtStats(result.total)} throughput=${result.throughputMBps.toFixed(2)}MB/s`)
    }
    return { valueSize: 4096, scenarios }
  })
}

async function benchB2(selectedScenarios = null) {
  section('B-2 SSTable read latency (cold and warm)')
  const scenarios = selectedScenarios ?? [
    { totalEntries: 1000, tableCount: 1 },
    { totalEntries: 1000, tableCount: 5 },
    { totalEntries: 1000, tableCount: 10 },
    { totalEntries: 1000, tableCount: 20 },
    { totalEntries: 10000, tableCount: 1 },
    { totalEntries: 10000, tableCount: 5 },
    { totalEntries: 10000, tableCount: 10 },
    { totalEntries: 10000, tableCount: 20 },
    { totalEntries: 50000, tableCount: 1 },
    { totalEntries: 50000, tableCount: 5 },
    { totalEntries: 50000, tableCount: 10 },
    { totalEntries: 50000, tableCount: 20 },
  ]
  const worker = new BenchWorker()
  try {
    const rows = []
    for (const scenario of scenarios) {
      const path = `b2/e${scenario.totalEntries}-t${scenario.tableCount}`
      const setup = await worker.call('setup-dataset', {
        path,
        tableCount: scenario.tableCount,
        entriesPerTable: Math.max(1, Math.floor(scenario.totalEntries / scenario.tableCount)),
        valueSize: 1024,
        keySpace: scenario.totalEntries * 8,
        falsePositiveRate: 0.01,
        indexEvery: 32,
      })
      const dir = await getDirectory(path, true)
      const keys = repeatKeys(setup.positiveKeys, 30)
      const warm = await runLookupSeries(dir, setup.manifest, keys, {
        useCache: true,
        cache: new Map(),
        skipBloom: true,
      })
      const coldNoBloom = await runLookupSeries(dir, setup.manifest, keys, {
        useCache: false,
        skipBloom: true,
      })
      const row = {
        ...scenario,
        valueSize: 1024,
        cold: coldNoBloom,
        warm,
      }
      rows.push(row)
      print(
        `entries=${scenario.totalEntries} tables=${scenario.tableCount} cold=${coldNoBloom.latency.p95.toFixed(3)}ms warm=${warm.latency.p95.toFixed(3)}ms`,
      )
    }
    return { scenarios: rows }
  } finally {
    worker.close()
  }
}

async function benchB3() {
  section('B-3 Bloom filter effectiveness')
  const worker = new BenchWorker()
  try {
    const scenarios = []
    for (const falsePositiveRate of [0.01, 0.001]) {
      const row = await worker.call('measure-bloom', {
        tableCount: 10,
        entriesPerTable: 3000,
        keySpace: 120000,
        falsePositiveRate,
        negatives: 200,
      })
      scenarios.push(row)
      print(
        `fpr=${falsePositiveRate} observedRate=${row.observedRate.toFixed(4)} estDataReads/lookup=${row.estimatedDataReadsPerLookup.toFixed(3)} p95=${row.queryLatency.p95.toFixed(3)}ms`,
      )
    }
    return { scenarios }
  } finally {
    worker.close()
  }
}

function aggregateLoadResults(results, totalMs) {
  const batchTimes = results.flatMap((row) => row.batchTimes)
  const publishLockMs = results.flatMap((row) => row.publishLockMs)
  const totalBlocks = results.reduce((sum, row) => sum + row.totalBlocks, 0)
  return {
    totalMs,
    throughputBlocksPerSec: totalBlocks / Math.max(0.0001, totalMs / 1000),
    batchLatency: stats(batchTimes),
    publishLock: stats(publishLockMs),
    meanTouchedShards: average(results.map((row) => row.meanTouchedShards)),
  }
}

async function runShardLoadScenario(path, withCompaction) {
  await withWorker((worker) =>
    worker.call('seed-shard', {
      path,
      shard: 0,
      tables: 8,
      entriesPerTable: 32,
      valueSize: 4096,
      falsePositiveRate: 0.01,
      indexEvery: 16,
      reset: true,
    }),
  )
  const workers = Array.from({ length: 4 }, () => new BenchWorker())
  const t0 = performance.now()
  const loadPromise = Promise.all(
    workers.map((worker, workerId) =>
      worker.call('shard-write-load', {
        path,
        workerId,
        batches: 20,
        blocksPerBatch: 50,
        shards: 8,
        valueSize: 4096,
        skew: 0.7,
        falsePositiveRate: 0.01,
        indexEvery: 16,
      }),
    ),
  )
  let compaction = null
  if (withCompaction) {
    await sleep(25)
    compaction = await withWorker((worker) =>
      worker.call('compact-existing-shard', {
        path,
        shard: 0,
        takeCount: 8,
        valueSize: 4096,
        falsePositiveRate: 0.01,
        indexEvery: 16,
      }),
    )
  }
  const loadRows = await loadPromise
  const totalMs = performance.now() - t0
  for (const worker of workers) {
    worker.close()
  }
  return {
    ...aggregateLoadResults(loadRows, totalMs),
    compaction,
  }
}

async function benchB4() {
  section('B-4 Compaction cost under load')
  const baseline = await runShardLoadScenario('b4/baseline', false)
  const withCompaction = await runShardLoadScenario('b4/with-compaction', true)
  print(`baseline throughput=${baseline.throughputBlocksPerSec.toFixed(1)} blocks/s p95=${baseline.batchLatency.p95.toFixed(3)}ms`)
  print(
    `with compaction throughput=${withCompaction.throughputBlocksPerSec.toFixed(1)} blocks/s p95=${withCompaction.batchLatency.p95.toFixed(3)}ms`,
  )
  if (withCompaction.compaction) {
    print(
      `compaction total=${withCompaction.compaction.totalMs.toFixed(3)}ms lockHold=${withCompaction.compaction.lockHoldMs.toFixed(3)}ms`,
    )
  }
  return { baseline, withCompaction }
}

async function benchB5() {
  section('B-5 Overwrite pattern on SSTables')
  return withWorker(async (worker) => {
    const result = await worker.call('overwrite-sstable-meta', {
      path: 'b5/meta-sstable',
      keyCount: 1000,
      rounds: 10,
      overwriteFraction: 0.5,
      batchSize: 50,
      valueSize: 256,
      falsePositiveRate: 0.01,
      indexEvery: 16,
      compactAt: 8,
    })
    print(
      `final tables=${result.finalTableCount} max tables=${result.maxTableCount} compactions=${result.compactions}`,
    )
    print(`publish ${fmtStats(result.publishLatency)} obsoleteVersions=${result.obsoleteVersions}`)
    return result
  })
}

async function benchB6() {
  section('B-6 Page store prototype')
  return withWorker(async (worker) => {
    const result = await worker.call('page-store-benchmark', {
      path: 'b6/page-store',
      keyCount: 1000,
      rounds: 10,
      overwriteFraction: 0.5,
      batchSize: 50,
      valueSize: 256,
    })
    print(`commit ${fmtStats(result.commitLatency)} meanPages=${result.meanPagesPerCommit.toFixed(2)}`)
    print(`read ${fmtStats(result.readLatency)}`)
    return result
  })
}

async function benchB7() {
  section('B-7 Shard count vs contention')
  const scenarios = []
  for (const skew of [0, 0.7]) {
    for (const shards of [4, 8, 16, 32]) {
      const workers = Array.from({ length: 4 }, () => new BenchWorker())
      const t0 = performance.now()
      const rows = await Promise.all(
        workers.map((worker, workerId) =>
          worker.call('shard-write-load', {
            path: `b7/${skew === 0 ? 'uniform' : 'hot'}/shards-${shards}`,
            workerId,
            batches: 20,
            blocksPerBatch: 50,
            shards,
            valueSize: 4096,
            skew,
            falsePositiveRate: 0.01,
            indexEvery: 16,
          }),
        ),
      )
      const totalMs = performance.now() - t0
      for (const worker of workers) {
        worker.close()
      }
      const aggregate = aggregateLoadResults(rows, totalMs)
      const row = {
        skew,
        shards,
        ...aggregate,
      }
      scenarios.push(row)
      print(
        `${skew === 0 ? 'uniform' : 'hot70'} shards=${shards} throughput=${aggregate.throughputBlocksPerSec.toFixed(1)} blocks/s p95=${aggregate.batchLatency.p95.toFixed(3)}ms`,
      )
    }
  }
  return { scenarios }
}

async function benchB8() {
  section('B-8 Async read concurrency')
  const path = 'b8/async-read'
  const publish = await withWorker((worker) =>
    worker.call('publish-once', {
      path,
      entryCount: 500,
      valueSize: 4096,
      keyBase: 0,
      indexEvery: 16,
      falsePositiveRate: 0.01,
    }),
  )
  const workers = Array.from({ length: 4 }, () => new BenchWorker())
  try {
    const rows = await Promise.all(
      workers.map((worker) =>
        worker.call('read-file-slices', {
          path,
          name: publish.name,
          start: 0,
          length: publish.bytes,
          iterations: 40,
        }),
      ),
    )
    const latencies = rows.map((row) => row.latency)
    const throughputMBps = rows.reduce((sum, row) => sum + row.throughputMBps, 0)
    for (const [index, row] of rows.entries()) {
      print(`worker=${index} ${fmtStats(row.latency)} throughput=${row.throughputMBps.toFixed(2)}MB/s`)
    }
    return {
      fileBytes: publish.bytes,
      perWorker: rows,
      aggregateThroughputMBps: throughputMBps,
      meanP95Ms: average(latencies.map((row) => row.p95)),
    }
  } finally {
    for (const worker of workers) {
      worker.close()
    }
  }
}

async function benchB9() {
  section('B-9 Large slice vs multiple small slices')
  const path = 'b9/range'
  const publish = await withWorker((worker) =>
    worker.call('publish-once', {
      path,
      entryCount: 500,
      valueSize: 1024,
      keyBase: 0,
      indexEvery: 16,
      falsePositiveRate: 0.01,
    }),
  )
  const dir = await getDirectory(path, true)
  const meta = await loadTableMetadata(dir, publish.name)
  const whole = []
  const split = []
  for (let i = 0; i < 50; i++) {
    whole.push(await rangeReadWholeSlice(dir, publish.name, meta, 100, 100))
    split.push(await rangeReadManySlices(dir, publish.name, meta, 100, 10, 10))
  }
  const result = {
    wholeSlice: stats(whole),
    tenSlices: stats(split),
  }
  print(`whole slice ${fmtStats(result.wholeSlice)}`)
  print(`ten slices ${fmtStats(result.tenSlices)}`)
  return result
}

async function benchB10(selectedScenarios = null) {
  section('B-10 Memtable flush threshold')
  const scenarios = selectedScenarios ?? [
    { threshold: 1, timerMs: 10 },
    { threshold: 1, timerMs: 50 },
    { threshold: 1, timerMs: 100 },
    { threshold: 4, timerMs: 10 },
    { threshold: 4, timerMs: 50 },
    { threshold: 4, timerMs: 100 },
    { threshold: 16, timerMs: 10 },
    { threshold: 16, timerMs: 50 },
    { threshold: 16, timerMs: 100 },
    { threshold: 64, timerMs: 10 },
    { threshold: 64, timerMs: 50 },
    { threshold: 64, timerMs: 100 },
  ]
  const worker = new BenchWorker()
  try {
    const rows = []
    for (const scenario of scenarios) {
      const result = await worker.call('memtable-sim', {
        path: `b10/t${scenario.threshold}-m${scenario.timerMs}`,
        threshold: scenario.threshold,
        timerMs: scenario.timerMs,
        writes: 128,
        interWriteMs: 2,
        valueSize: 4096,
        falsePositiveRate: 0.01,
        indexEvery: 16,
      })
      rows.push(result)
      print(
        `threshold=${scenario.threshold} timer=${scenario.timerMs}ms flushes=${result.flushCount} p95=${result.ackLatency.p95.toFixed(3)}ms visibilityP95=${result.visibilityLatency.p95.toFixed(3)}ms`,
      )
    }
    return { scenarios: rows }
  } finally {
    worker.close()
  }
}

async function runFullSuite() {
  const results = {
    integrity: await benchIntegrityPublishVisibility(),
    benchmarks: {},
  }
  results.benchmarks.B1 = await benchB1()
  results.benchmarks.B2 = await benchB2()
  results.benchmarks.B3 = await benchB3()
  results.benchmarks.B4 = await benchB4()
  results.benchmarks.B5 = await benchB5()
  results.benchmarks.B6 = await benchB6()
  results.benchmarks.B7 = await benchB7()
  results.benchmarks.B8 = await benchB8()
  results.benchmarks.B9 = await benchB9()
  results.benchmarks.B10 = await benchB10()
  return results
}

async function runSmokeSuite() {
  const results = {
    integrity: await benchIntegrityPublishVisibility(),
    benchmarks: {},
  }
  results.benchmarks.B2 = await benchB2([
    { totalEntries: 10000, tableCount: 10 },
  ])
  results.benchmarks.B4 = await benchB4()
  results.benchmarks.B8 = await benchB8()
  results.benchmarks.B10 = await benchB10([
    { threshold: 16, timerMs: 50 },
    { threshold: 64, timerMs: 100 },
  ])
  return results
}

window.runAll = async function runAll(suite = 'full') {
  runBtn.disabled = true
  log.textContent = ''
  print('Browser OPFS benchmark suite')
  print(`suite: ${suite}`)
  print(`crossOriginIsolated: ${self.crossOriginIsolated}`)
  print(`userAgent: ${navigator.userAgent}`)
  print(`date: ${new Date().toISOString()}`)
  await cleanupBenchRoot()
  const startedAt = performance.now()
  const results = suite === 'smoke' ? await runSmokeSuite() : await runFullSuite()
  results.meta = {
    suite,
    userAgent: navigator.userAgent,
    crossOriginIsolated: self.crossOriginIsolated,
    startedAt: new Date().toISOString(),
    elapsedMs: performance.now() - startedAt,
  }
  section('Summary')
  print(`elapsed=${results.meta.elapsedMs.toFixed(1)}ms`)
  if (results.integrity.failures.length) {
    print(`integrity failures=${results.integrity.failures.length}`)
  } else {
    print('integrity failures=0')
  }
  window.__benchResults = results
  runBtn.disabled = false
  return results
}

window.cleanup = async function cleanup() {
  await cleanupBenchRoot()
  print('\nCleanup complete.')
}
