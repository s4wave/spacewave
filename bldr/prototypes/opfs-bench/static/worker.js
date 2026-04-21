import {
  PAGE_SIZE,
  average,
  buildBloom,
  bloomMayContain,
  bloomParameters,
  buildSSTable,
  fileExists,
  getDirectory,
  loadTableMetadata,
  makeEntries,
  makeKey,
  makeManifestTable,
  makePageStoreLayout,
  makeValue,
  nextManifestName,
  pickIndices,
  readLatestManifest,
  resetPath,
  runLookupSeries,
  stats,
  writeJsonFile,
} from './bench-lib.js'

function errString(err) {
  if (!err) {
    return 'unknown error'
  }
  return err.stack || err.message || String(err)
}

function uniqueName(prefix, version) {
  return `${prefix}-${version.toString().padStart(6, '0')}.sst`
}

function makeEntriesFromIndices(indices, valueSize, seedOffset = 0, tombstones = new Set()) {
  return indices.map((keyIndex) => ({
    key: makeKey(keyIndex),
    value: makeValue(valueSize, keyIndex + seedOffset),
    tombstone: tombstones.has(keyIndex),
  }))
}

async function writeSyncBytes(dir, name, bytes) {
  const fh = await dir.getFileHandle(name, { create: true })
  const handle = await fh.createSyncAccessHandle()
  const data = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes)
  const t0 = performance.now()
  try {
    handle.truncate(data.byteLength)
    let off = 0
    while (off < data.byteLength) {
      const written = handle.write(data.subarray(off), { at: off })
      if (!written) {
        throw new Error(`short sync write at offset ${off}`)
      }
      off += written
    }
    handle.flush()
  } finally {
    handle.close()
  }
  return performance.now() - t0
}

async function publishImmutableSegment(path, shard, entries, opts = {}) {
  const dir = await getDirectory(path, true)
  const build0 = performance.now()
  const built = buildSSTable(entries, opts)
  const buildMs = performance.now() - build0
  let writeMs = 0
  let manifestMs = 0
  let lockHoldMs = 0
  let version = 0
  let name = ''
  await navigator.locks.request(`opfs-bench/${path}/${shard}`, { mode: 'exclusive' }, async () => {
    const lock0 = performance.now()
    const current = await readLatestManifest(dir)
    version = current.version + 1
    name = uniqueName(shard, version)
    writeMs = await writeSyncBytes(dir, name, built.buffer)
    const next = {
      version,
      tables: [
        makeManifestTable(name, built.entryCount, built.bytes, {
          keySize: opts.keySize ?? 16,
          valueSize: opts.valueSize ?? 4096,
          falsePositiveRate: opts.falsePositiveRate ?? 0.01,
        }),
        ...current.tables,
      ],
    }
    const m0 = performance.now()
    await writeJsonFile(dir, nextManifestName(version), next)
    manifestMs = performance.now() - m0
    lockHoldMs = performance.now() - lock0
  })
  return {
    buildMs,
    writeMs,
    manifestMs,
    lockHoldMs,
    totalMs: buildMs + lockHoldMs,
    version,
    name,
    bytes: built.bytes,
    entryCount: built.entryCount,
  }
}

async function readWholeTableEntries(dir, name) {
  const meta = await loadTableMetadata(dir, name)
  const fh = await dir.getFileHandle(name)
  const file = await fh.getFile()
  const buf = await file
    .slice(meta.dataOffset, meta.dataOffset + meta.entryCount * meta.recordSize)
    .arrayBuffer()
  const bytes = new Uint8Array(buf)
  const out = []
  for (let i = 0; i < meta.entryCount; i++) {
    const off = i * meta.recordSize
    let end = off + meta.keySize
    while (end > off && bytes[end - 1] === 0) {
      end--
    }
    const key = new TextDecoder().decode(bytes.subarray(off, end))
    out.push({
      key,
      tombstone: bytes[off + meta.keySize] === 1,
      value: bytes.slice(
        off + meta.keySize + 1,
        off + meta.keySize + 1 + meta.valueSize,
      ),
    })
  }
  return { meta, entries: out }
}

async function compactTables(path, shard, takeCount, opts = {}) {
  const dir = await getDirectory(path, true)
  const current = await readLatestManifest(dir)
  const selected = current.tables.slice(0, takeCount)
  if (selected.length <= 1) {
    return {
      compacted: 0,
      totalMs: 0,
      lockHoldMs: 0,
      deleteMs: 0,
      outputEntries: 0,
    }
  }
  const read0 = performance.now()
  const latest = new Map()
  for (const table of selected) {
    const loaded = await readWholeTableEntries(dir, table.name)
    for (const entry of loaded.entries) {
      if (!latest.has(entry.key)) {
        latest.set(entry.key, entry)
      }
    }
  }
  const merged = [...latest.values()].sort((a, b) => (a.key < b.key ? -1 : a.key > b.key ? 1 : 0))
  const readMs = performance.now() - read0
  const built = buildSSTable(merged, opts)
  let lockHoldMs = 0
  let deleteMs = 0
  let writeMs = 0
  let manifestMs = 0
  let version = 0
  let outName = ''
  await navigator.locks.request(`opfs-bench/${path}/${shard}`, { mode: 'exclusive' }, async () => {
    const lock0 = performance.now()
    const now = await readLatestManifest(dir)
    const compactNames = new Set(selected.map((table) => table.name))
    version = now.version + 1
    outName = uniqueName(`${shard}-compact`, version)
    writeMs = await writeSyncBytes(dir, outName, built.buffer)
    const next = {
      version,
      tables: [
        makeManifestTable(outName, built.entryCount, built.bytes, {
          keySize: opts.keySize ?? 16,
          valueSize: opts.valueSize ?? 4096,
          falsePositiveRate: opts.falsePositiveRate ?? 0.01,
          compactedFrom: selected.length,
        }),
        ...now.tables.filter((table) => !compactNames.has(table.name)),
      ],
    }
    const m0 = performance.now()
    await writeJsonFile(dir, nextManifestName(version), next)
    manifestMs = performance.now() - m0
    lockHoldMs = performance.now() - lock0
  })
  const del0 = performance.now()
  for (const table of selected) {
    try {
      await dir.removeEntry(table.name)
    } catch (err) {
      if (err?.name !== 'NotFoundError') {
        throw err
      }
    }
  }
  deleteMs = performance.now() - del0
  return {
    compacted: selected.length,
    outputEntries: merged.length,
    readMs,
    writeMs,
    manifestMs,
    lockHoldMs,
    deleteMs,
    totalMs: readMs + writeMs + manifestMs + deleteMs,
    version,
    name: outName,
  }
}

async function cmdPublishBatchSeries(params) {
  const {
    path,
    batchSize,
    repeats,
    valueSize,
    indexEvery,
    falsePositiveRate,
  } = params
  const runs = []
  for (let i = 0; i < repeats; i++) {
    const sub = `${path}/batch-${batchSize}/run-${i}`
    await resetPath(sub)
    const entries = makeEntries(batchSize, valueSize, 0)
    runs.push(
      await publishImmutableSegment(sub, 'batch', entries, {
        valueSize,
        indexEvery,
        falsePositiveRate,
      }),
    )
  }
  return {
    batchSize,
    repeats,
    bytesPerRun: runs[0]?.bytes ?? 0,
    build: stats(runs.map((run) => run.buildMs)),
    write: stats(runs.map((run) => run.writeMs)),
    manifest: stats(runs.map((run) => run.manifestMs)),
    lockHold: stats(runs.map((run) => run.lockHoldMs)),
    total: stats(runs.map((run) => run.totalMs)),
    throughputMBps:
      ((runs[0]?.bytes ?? 0) / 1024 / 1024) / Math.max(0.0001, average(runs.map((run) => run.totalMs)) / 1000),
  }
}

async function cmdPublishOnce(params) {
  const {
    path,
    entryCount,
    valueSize,
    keyBase,
    indexEvery,
    falsePositiveRate,
  } = params
  const entries = makeEntries(entryCount, valueSize, 0, { keyBase })
  const result = await publishImmutableSegment(path, 'publish', entries, {
    valueSize,
    indexEvery,
    falsePositiveRate,
  })
  return {
    ...result,
    expectedKey: entries[Math.floor(entries.length / 2)].key,
  }
}

async function cmdValidateLatest(params) {
  const { path, expectedVersion, expectedName, expectedKey, expectedBytes } = params
  const dir = await getDirectory(path, true)
  const manifest = await readLatestManifest(dir)
  if (manifest.version < expectedVersion) {
    return {
      ok: false,
      reason: `manifest version ${manifest.version} < expected ${expectedVersion}`,
    }
  }
  const newest = manifest.tables[0]
  if (!newest || newest.name !== expectedName) {
    return {
      ok: false,
      reason: `manifest head ${newest?.name ?? '<none>'} != ${expectedName}`,
    }
  }
  if (!(await fileExists(dir, expectedName))) {
    return {
      ok: false,
      reason: `missing table ${expectedName}`,
    }
  }
  const meta = await loadTableMetadata(dir, expectedName)
  if (meta.bytes !== expectedBytes) {
    return {
      ok: false,
      reason: `table bytes ${meta.bytes} != ${expectedBytes}`,
    }
  }
  const fh = await dir.getFileHandle(expectedName)
  const file = await fh.getFile()
  const t0 = performance.now()
  await file.slice(0, expectedBytes).arrayBuffer()
  const latencyMs = performance.now() - t0
  return {
    ok: true,
    expectedKey,
    latencyMs,
  }
}

async function cmdSetupDataset(params) {
  const {
    path,
    tableCount,
    entriesPerTable,
    valueSize,
    keySpace,
    falsePositiveRate,
    indexEvery,
  } = params
  await resetPath(path)
  const positiveKeys = []
  for (let table = 0; table < tableCount; table++) {
    const indices = pickIndices(keySpace, entriesPerTable, 1000 + table)
    const entries = makeEntriesFromIndices(indices, valueSize, table * 100000)
    const published = await publishImmutableSegment(path, 'dataset', entries, {
      valueSize,
      falsePositiveRate,
      indexEvery,
    })
    if (table === tableCount - 1) {
      positiveKeys.push(entries[Math.floor(entries.length / 3)].key)
    }
    if (table === 0) {
      positiveKeys.push(entries[Math.floor(entries.length / 2)].key)
    }
    if (table === Math.floor(tableCount / 2)) {
      positiveKeys.push(entries[Math.floor(entries.length * 2 / 3)].key)
    }
    if (!published.version) {
      throw new Error('publish failed')
    }
  }
  const dir = await getDirectory(path, true)
  const manifest = await readLatestManifest(dir)
  return {
    manifest,
    positiveKeys,
    negativeKeys: [
      makeKey(keySpace + 1000),
      makeKey(keySpace + 2000),
      makeKey(keySpace + 3000),
      makeKey(keySpace + 4000),
      makeKey(keySpace + 5000),
    ],
    bloomBitsPerKey: bloomParameters(entriesPerTable, falsePositiveRate).bitCount / Math.max(1, entriesPerTable),
  }
}

async function cmdReadFileSlices(params) {
  const { path, name, start, length, iterations } = params
  const dir = await getDirectory(path, true)
  const fh = await dir.getFileHandle(name)
  const times = []
  for (let i = 0; i < iterations; i++) {
    const t0 = performance.now()
    const file = await fh.getFile()
    await file.slice(start, start + length).arrayBuffer()
    times.push(performance.now() - t0)
  }
  return {
    iterations,
    latency: stats(times),
    throughputMBps:
      ((length * iterations) / 1024 / 1024) / Math.max(0.0001, average(times) * iterations / 1000),
  }
}

function chooseShard(shards, skew, seed) {
  const hotProbability = skew ?? 0
  const hotShard = 0
  let x = (seed + 1) >>> 0
  x = (x * 1664525 + 1013904223) >>> 0
  if (hotProbability > 0 && (x % 1000) / 1000 < hotProbability) {
    return hotShard
  }
  return x % shards
}

async function cmdShardWriteLoad(params) {
  const {
    path,
    workerId,
    batches,
    blocksPerBatch,
    shards,
    valueSize,
    skew,
    falsePositiveRate,
    indexEvery,
  } = params
  const batchTimes = []
  const publishLockMs = []
  const touchedShards = []
  for (let batch = 0; batch < batches; batch++) {
    const shardMap = new Map()
    for (let i = 0; i < blocksPerBatch; i++) {
      const shard = chooseShard(shards, skew, workerId * 100000 + batch * 1000 + i)
      const entries = shardMap.get(shard) ?? []
      const global = workerId * 10000000 + batch * 1000 + i
      entries.push({
        key: makeKey(global),
        value: makeValue(valueSize, global),
        tombstone: false,
      })
      shardMap.set(shard, entries)
    }
    const t0 = performance.now()
    for (const [shard, entries] of shardMap.entries()) {
      const published = await publishImmutableSegment(
        `${path}/shard-${shard}`,
        `shard-${shard}`,
        entries,
        {
          valueSize,
          falsePositiveRate,
          indexEvery,
        },
      )
      publishLockMs.push(published.lockHoldMs)
    }
    batchTimes.push(performance.now() - t0)
    touchedShards.push(shardMap.size)
  }
  return {
    workerId,
    batchTimes,
    publishLockMs,
    batchLatency: stats(batchTimes),
    publishLock: stats(publishLockMs),
    meanTouchedShards: average(touchedShards),
    totalBlocks: batches * blocksPerBatch,
    totalBatches: batches,
  }
}

async function cmdCompactShard(params) {
  const {
    path,
    shard,
    initialTables,
    entriesPerTable,
    valueSize,
    falsePositiveRate,
    indexEvery,
    takeCount,
  } = params
  const shardPath = `${path}/shard-${shard}`
  await resetPath(shardPath)
  for (let i = 0; i < initialTables; i++) {
    const entries = makeEntries(entriesPerTable, valueSize, i * entriesPerTable)
    await publishImmutableSegment(shardPath, `shard-${shard}`, entries, {
      valueSize,
      falsePositiveRate,
      indexEvery,
    })
  }
  return compactTables(shardPath, `shard-${shard}`, takeCount ?? initialTables, {
    valueSize,
    falsePositiveRate,
    indexEvery,
  })
}

async function cmdSeedShard(params) {
  const {
    path,
    shard,
    tables,
    entriesPerTable,
    valueSize,
    falsePositiveRate,
    indexEvery,
    reset,
  } = params
  const shardPath = `${path}/shard-${shard}`
  if (reset) {
    await resetPath(shardPath)
  }
  for (let i = 0; i < tables; i++) {
    const entries = makeEntries(entriesPerTable, valueSize, i * entriesPerTable)
    await publishImmutableSegment(shardPath, `shard-${shard}`, entries, {
      valueSize,
      falsePositiveRate,
      indexEvery,
    })
  }
  const dir = await getDirectory(shardPath, true)
  const manifest = await readLatestManifest(dir)
  return {
    shard,
    tableCount: manifest.tables.length,
  }
}

async function cmdCompactExistingShard(params) {
  const {
    path,
    shard,
    takeCount,
    valueSize,
    falsePositiveRate,
    indexEvery,
  } = params
  return compactTables(`${path}/shard-${shard}`, `shard-${shard}`, takeCount, {
    valueSize,
    falsePositiveRate,
    indexEvery,
  })
}

async function cmdOverwriteSSTableMeta(params) {
  const {
    path,
    keyCount,
    rounds,
    overwriteFraction,
    batchSize,
    valueSize,
    falsePositiveRate,
    indexEvery,
    compactAt,
  } = params
  await resetPath(path)
  const allKeys = Array.from({ length: keyCount }, (_, i) => i)
  await publishImmutableSegment(
    path,
    'meta',
    makeEntriesFromIndices(allKeys, valueSize, 0),
    { valueSize, falsePositiveRate, indexEvery },
  )
  let compactions = 0
  let maxTables = 1
  const publishTimes = []
  const compactTimes = []
  for (let round = 0; round < rounds; round++) {
    const picked = pickIndices(keyCount, Math.floor(keyCount * overwriteFraction), round + 1)
    for (let i = 0; i < picked.length; i += batchSize) {
      const slice = picked.slice(i, i + batchSize)
      const entries = makeEntriesFromIndices(slice, valueSize, 1000000 + round * 10000 + i)
      const published = await publishImmutableSegment(path, 'meta', entries, {
        valueSize,
        falsePositiveRate,
        indexEvery,
      })
      publishTimes.push(published.totalMs)
      const manifest = await readLatestManifest(await getDirectory(path, true))
      if (manifest.tables.length > maxTables) {
        maxTables = manifest.tables.length
      }
      if (manifest.tables.length >= compactAt) {
        const compacted = await compactTables(path, 'meta', Math.min(compactAt, manifest.tables.length), {
          valueSize,
          falsePositiveRate,
          indexEvery,
        })
        if (compacted.compacted > 1) {
          compactions++
          compactTimes.push(compacted.totalMs)
        }
      }
    }
  }
  const dir = await getDirectory(path, true)
  const manifest = await readLatestManifest(dir)
  const lookupKeys = pickIndices(keyCount, 100, 9999).map((index) => makeKey(index))
  const warm = await runLookupSeries(dir, manifest, lookupKeys, { useCache: true, cache: new Map() })
  const totalEntries = manifest.tables.reduce((sum, table) => sum + table.entryCount, 0)
  return {
    finalTableCount: manifest.tables.length,
    maxTableCount: maxTables,
    compactions,
    publishLatency: stats(publishTimes),
    compactionLatency: stats(compactTimes),
    storedEntries: totalEntries,
    obsoleteVersions: totalEntries - keyCount,
    lookupWarm: warm,
  }
}

async function cmdPageStoreBenchmark(params) {
  const {
    path,
    keyCount,
    rounds,
    overwriteFraction,
    batchSize,
    valueSize,
  } = params
  await resetPath(path)
  const dir = await getDirectory(path, true)
  const layout = makePageStoreLayout(keyCount, valueSize)
  await writeSyncBytes(dir, 'page-store.dat', new Uint8Array(layout.totalBytes))
  const commits = []
  const pagesPerCommit = []
  for (let round = 0; round < rounds; round++) {
    const picked = pickIndices(keyCount, Math.floor(keyCount * overwriteFraction), round + 11)
    for (let i = 0; i < picked.length; i += batchSize) {
      const slice = picked.slice(i, i + batchSize)
      const leaves = new Set(slice.map((keyIndex) => Math.floor(keyIndex / layout.keysPerLeaf)))
      const pages = [
        ...[...leaves].map((leaf) => ({
          offset: layout.leafOffset(leaf),
          bytes: new Uint8Array(PAGE_SIZE),
        })),
        { offset: layout.rootOffset, bytes: new Uint8Array(PAGE_SIZE) },
        { offset: 0, bytes: new Uint8Array(PAGE_SIZE) },
      ]
      const t0 = performance.now()
      await navigator.locks.request(`opfs-bench/${path}/page-store`, { mode: 'exclusive' }, async () => {
        const fh = await dir.getFileHandle('page-store.dat')
        const handle = await fh.createSyncAccessHandle()
        try {
          for (const page of pages) {
            let off = 0
            while (off < page.bytes.byteLength) {
              const written = handle.write(page.bytes.subarray(off), {
                at: page.offset + off,
              })
              if (!written) {
                throw new Error(`short page write at offset ${page.offset + off}`)
              }
              off += written
            }
          }
          handle.flush()
        } finally {
          handle.close()
        }
      })
      commits.push(performance.now() - t0)
      pagesPerCommit.push(pages.length)
    }
  }
  const readTimes = []
  const fh = await dir.getFileHandle('page-store.dat')
  const readIndices = pickIndices(keyCount, 100, 31337)
  for (const keyIndex of readIndices) {
    const leaf = Math.floor(keyIndex / layout.keysPerLeaf)
    const t0 = performance.now()
    const file = await fh.getFile()
    await file.slice(layout.rootOffset, layout.rootOffset + PAGE_SIZE).arrayBuffer()
    await file
      .slice(layout.leafOffset(leaf), layout.leafOffset(leaf) + PAGE_SIZE)
      .arrayBuffer()
    readTimes.push(performance.now() - t0)
  }
  return {
    commitLatency: stats(commits),
    meanPagesPerCommit: average(pagesPerCommit),
    readLatency: stats(readTimes),
    totalBytes: layout.totalBytes,
  }
}

async function cmdMeasureBloom(params) {
  const {
    tableCount,
    entriesPerTable,
    keySpace,
    falsePositiveRate,
    negatives,
  } = params
  const tables = []
  const buildTimes = []
  for (let table = 0; table < tableCount; table++) {
    const indices = pickIndices(keySpace, entriesPerTable, 2000 + table)
    const keys = indices.map((index) => makeKey(index))
    const t0 = performance.now()
    const built = buildBloom(keys, falsePositiveRate)
    buildTimes.push(performance.now() - t0)
    tables.push({
      bytes: built.bytes,
      params: built.params,
    })
  }
  const negativeKeys = Array.from({ length: negatives }, (_, i) => makeKey(keySpace + 10000 + i))
  const queryTimes = []
  let falsePositives = 0
  let checks = 0
  for (const key of negativeKeys) {
    const t0 = performance.now()
    for (const table of tables) {
      checks++
      if (bloomMayContain(table.bytes, table.params, key)) {
        falsePositives++
      }
    }
    queryTimes.push(performance.now() - t0)
  }
  return {
    falsePositiveRate,
    tableCount,
    entriesPerTable,
    negatives,
    checks,
    falsePositives,
    observedRate: falsePositives / Math.max(1, checks),
    estimatedDataReadsPerLookup: falsePositives / Math.max(1, negatives),
    buildLatency: stats(buildTimes),
    queryLatency: stats(queryTimes),
    bloomBitsPerKey: bloomParameters(entriesPerTable, falsePositiveRate).bitCount / Math.max(1, entriesPerTable),
  }
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

async function simulateMemtablePolicy(params) {
  const {
    path,
    threshold,
    timerMs,
    writes,
    interWriteMs,
    valueSize,
    falsePositiveRate,
    indexEvery,
  } = params
  await resetPath(path)
  const queue = []
  const waiters = []
  let timer = null
  let flushing = false
  let flushCount = 0
  const visibility = []

  const flush = async () => {
    if (flushing || queue.length === 0) {
      return
    }
    flushing = true
    const batch = queue.splice(0)
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
    const entries = batch.map((item) => ({
      key: item.key,
      value: item.value,
      tombstone: false,
    }))
    await publishImmutableSegment(path, 'memtable', entries, {
      valueSize,
      falsePositiveRate,
      indexEvery,
    })
    flushCount++
    const seenAt = performance.now()
    for (const item of batch) {
      const delta = seenAt - item.enqueuedAt
      visibility.push(delta)
      item.resolve(delta)
    }
    flushing = false
    if (queue.length > 0) {
      if (queue.length >= threshold) {
        await flush()
      } else if (!timer && timerMs > 0) {
        timer = setTimeout(() => {
          timer = null
          void flush()
        }, timerMs)
      }
    }
  }

  for (let i = 0; i < writes; i++) {
    waiters.push(
      new Promise((resolve) => {
        queue.push({
          key: makeKey(i),
          value: makeValue(valueSize, i),
          enqueuedAt: performance.now(),
          resolve,
        })
      }),
    )
    if (threshold <= 1 || queue.length >= threshold) {
      await flush()
    } else if (!timer && timerMs > 0) {
      timer = setTimeout(() => {
        timer = null
        void flush()
      }, timerMs)
    }
    if (interWriteMs > 0) {
      await sleep(interWriteMs)
    }
  }
  await flush()
  const ack = await Promise.all(waiters)
  return {
    threshold,
    timerMs,
    flushCount,
    visibilityLatency: stats(visibility),
    ackLatency: stats(ack),
  }
}

const commands = {
  'publish-batch-series': cmdPublishBatchSeries,
  'publish-once': cmdPublishOnce,
  'validate-latest': cmdValidateLatest,
  'setup-dataset': cmdSetupDataset,
  'read-file-slices': cmdReadFileSlices,
  'shard-write-load': cmdShardWriteLoad,
  'compact-shard': cmdCompactShard,
  'seed-shard': cmdSeedShard,
  'compact-existing-shard': cmdCompactExistingShard,
  'overwrite-sstable-meta': cmdOverwriteSSTableMeta,
  'page-store-benchmark': cmdPageStoreBenchmark,
  'measure-bloom': cmdMeasureBloom,
  'memtable-sim': simulateMemtablePolicy,
}

self.onmessage = async (ev) => {
  const { id, cmd, params } = ev.data
  const handler = commands[cmd]
  if (!handler) {
    self.postMessage({
      id,
      ok: false,
      error: `unknown command: ${cmd}`,
    })
    return
  }
  try {
    const result = await handler(params)
    self.postMessage({ id, ok: true, result })
  } catch (err) {
    self.postMessage({
      id,
      ok: false,
      error: errString(err),
    })
  }
}
