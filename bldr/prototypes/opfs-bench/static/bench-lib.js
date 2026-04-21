export const BENCH_DIR = '.opfs-bench'
export const HEADER_SIZE = 64
export const INDEX_ENTRY_SIZE = 20
export const PAGE_SIZE = 4096

const enc = new TextEncoder()
const dec = new TextDecoder()

export function percentile(sorted, q) {
  if (!sorted.length) {
    return 0
  }
  const idx = Math.min(sorted.length - 1, Math.floor((sorted.length - 1) * q))
  return sorted[idx]
}

export function stats(values) {
  const sorted = [...values].sort((a, b) => a - b)
  const n = sorted.length
  const sum = sorted.reduce((a, b) => a + b, 0)
  return {
    n,
    mean: n ? sum / n : 0,
    median: percentile(sorted, 0.5),
    p95: percentile(sorted, 0.95),
    p99: percentile(sorted, 0.99),
    min: sorted[0] ?? 0,
    max: sorted[n - 1] ?? 0,
  }
}

export function average(values) {
  if (!values.length) {
    return 0
  }
  return values.reduce((a, b) => a + b, 0) / values.length
}

export function fmtStats(s) {
  const f = (v) => v.toFixed(3)
  return `n=${s.n} mean=${f(s.mean)}ms median=${f(s.median)}ms p95=${f(s.p95)}ms p99=${f(s.p99)}ms min=${f(s.min)}ms max=${f(s.max)}ms`
}

export async function getBenchDirectory() {
  const root = await navigator.storage.getDirectory()
  return root.getDirectoryHandle(BENCH_DIR, { create: true })
}

export async function getDirectory(path, create = true) {
  const base = await getBenchDirectory()
  let dir = base
  if (!path) {
    return dir
  }
  for (const part of path.split('/')) {
    if (!part) {
      continue
    }
    dir = await dir.getDirectoryHandle(part, { create })
  }
  return dir
}

export async function resetPath(path) {
  const base = await getBenchDirectory()
  if (!path) {
    await clearDirectory(base)
    return base
  }
  const parts = path.split('/').filter(Boolean)
  const name = parts.pop()
  let dir = base
  for (const part of parts) {
    dir = await dir.getDirectoryHandle(part, { create: true })
  }
  if (name) {
    try {
      await dir.removeEntry(name, { recursive: true })
    } catch (err) {
      if (err?.name !== 'NotFoundError') {
        throw err
      }
    }
  }
  return getDirectory(path, true)
}

export async function clearDirectory(dir) {
  for await (const [name, handle] of dir.entries()) {
    await dir.removeEntry(name, { recursive: handle.kind === 'directory' })
  }
}

export async function cleanupBenchRoot() {
  const root = await navigator.storage.getDirectory()
  try {
    await root.removeEntry(BENCH_DIR, { recursive: true })
  } catch (err) {
    if (err?.name !== 'NotFoundError') {
      throw err
    }
  }
}

export async function writeJsonFile(dir, name, value) {
  const fh = await dir.getFileHandle(name, { create: true })
  const w = await fh.createWritable()
  await w.write(JSON.stringify(value))
  await w.close()
}

export async function readJsonFile(dir, name) {
  const fh = await dir.getFileHandle(name)
  const file = await fh.getFile()
  return JSON.parse(await file.text())
}

export async function fileExists(dir, name) {
  try {
    await dir.getFileHandle(name)
    return true
  } catch (err) {
    if (err?.name === 'NotFoundError') {
      return false
    }
    throw err
  }
}

export function makeKey(index, width = 15) {
  return `k${index.toString().padStart(width, '0')}`
}

export function makeValue(size, seed) {
  const value = new Uint8Array(size)
  let x = (seed + 1) >>> 0
  for (let i = 0; i < size; i++) {
    x = (x * 1664525 + 1013904223) >>> 0
    value[i] = x & 0xff
  }
  return value
}

export function makeEntries(count, valueSize, startIndex = 0, opts = {}) {
  const entries = []
  const keyBase = opts.keyBase ?? 0
  const tombstoneSet = opts.tombstones ?? new Set()
  for (let i = 0; i < count; i++) {
    const keyIndex = keyBase + startIndex + i
    entries.push({
      key: makeKey(keyIndex),
      value: makeValue(valueSize, keyIndex),
      tombstone: tombstoneSet.has(keyIndex),
    })
  }
  return entries
}

export function pickIndices(total, count, seed) {
  const seen = new Set()
  const out = []
  let x = (seed + 1) >>> 0
  while (out.length < count && seen.size < total) {
    x = (x * 1664525 + 1013904223) >>> 0
    const idx = x % total
    if (seen.has(idx)) {
      continue
    }
    seen.add(idx)
    out.push(idx)
  }
  out.sort((a, b) => a - b)
  return out
}

function encodeKey(key, keySize) {
  const out = new Uint8Array(keySize)
  const data = enc.encode(key)
  out.set(data.subarray(0, keySize))
  return out
}

function decodeKey(bytes) {
  let end = bytes.length
  while (end > 0 && bytes[end - 1] === 0) {
    end--
  }
  return dec.decode(bytes.subarray(0, end))
}

function hashStringA(str) {
  let h = 2166136261
  for (let i = 0; i < str.length; i++) {
    h ^= str.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return h >>> 0
}

function hashStringB(str) {
  let h = 5381
  for (let i = 0; i < str.length; i++) {
    h = ((h << 5) + h + str.charCodeAt(i)) >>> 0
  }
  return h >>> 0
}

export function bloomParameters(entryCount, falsePositiveRate) {
  const rate = Math.max(0.0001, Math.min(0.5, falsePositiveRate))
  const bits = Math.max(64, Math.ceil((-entryCount * Math.log(rate)) / (Math.log(2) ** 2)))
  const hashCount = Math.max(1, Math.round((bits / Math.max(1, entryCount)) * Math.log(2)))
  return {
    bitCount: bits,
    hashCount,
    byteCount: Math.ceil(bits / 8),
    falsePositiveRate: rate,
  }
}

function setBloomBit(bytes, bit) {
  bytes[bit >> 3] |= 1 << (bit & 7)
}

function getBloomBit(bytes, bit) {
  return (bytes[bit >> 3] & (1 << (bit & 7))) !== 0
}

export function buildBloom(keys, falsePositiveRate) {
  const params = bloomParameters(keys.length || 1, falsePositiveRate)
  const bytes = new Uint8Array(params.byteCount)
  for (const key of keys) {
    const a = hashStringA(key)
    const b = hashStringB(key)
    for (let i = 0; i < params.hashCount; i++) {
      const bit = ((a + Math.imul(i, b || 1)) >>> 0) % params.bitCount
      setBloomBit(bytes, bit)
    }
  }
  return { bytes, params }
}

export function bloomMayContain(bytes, params, key) {
  const a = hashStringA(key)
  const b = hashStringB(key)
  for (let i = 0; i < params.hashCount; i++) {
    const bit = ((a + Math.imul(i, b || 1)) >>> 0) % params.bitCount
    if (!getBloomBit(bytes, bit)) {
      return false
    }
  }
  return true
}

function getHeader(view) {
  const magic = view.getUint32(0, true)
  if (magic !== 0x314c4253) {
    throw new Error(`bad sstable magic: ${magic}`)
  }
  return {
    entryCount: view.getUint32(8, true),
    keySize: view.getUint32(12, true),
    valueSize: view.getUint32(16, true),
    recordSize: view.getUint32(20, true),
    indexEvery: view.getUint32(24, true),
    indexCount: view.getUint32(28, true),
    dataOffset: view.getUint32(32, true),
    indexOffset: view.getUint32(36, true),
    bloomOffset: view.getUint32(40, true),
    bloomBytes: view.getUint32(44, true),
    bloomHashCount: view.getUint32(48, true),
    bytes: view.getUint32(52, true),
  }
}

export function buildSSTable(entries, opts = {}) {
  const keySize = opts.keySize ?? 16
  const valueSize = opts.valueSize ?? 4096
  const indexEvery = opts.indexEvery ?? 32
  const falsePositiveRate = opts.falsePositiveRate ?? 0.01
  const sorted = [...entries].sort((a, b) => (a.key < b.key ? -1 : a.key > b.key ? 1 : 0))
  const bloom = buildBloom(sorted.map((entry) => entry.key), falsePositiveRate)
  const recordSize = keySize + 1 + valueSize
  const indexCount = Math.ceil(sorted.length / indexEvery)
  const dataOffset = HEADER_SIZE
  const indexOffset = dataOffset + sorted.length * recordSize
  const bloomOffset = indexOffset + indexCount * INDEX_ENTRY_SIZE
  const totalBytes = bloomOffset + bloom.bytes.byteLength
  const buf = new ArrayBuffer(totalBytes)
  const bytes = new Uint8Array(buf)
  const view = new DataView(buf)
  view.setUint32(0, 0x314c4253, true)
  view.setUint32(4, 1, true)
  view.setUint32(8, sorted.length, true)
  view.setUint32(12, keySize, true)
  view.setUint32(16, valueSize, true)
  view.setUint32(20, recordSize, true)
  view.setUint32(24, indexEvery, true)
  view.setUint32(28, indexCount, true)
  view.setUint32(32, dataOffset, true)
  view.setUint32(36, indexOffset, true)
  view.setUint32(40, bloomOffset, true)
  view.setUint32(44, bloom.bytes.byteLength, true)
  view.setUint32(48, bloom.params.hashCount, true)
  view.setUint32(52, totalBytes, true)
  for (let i = 0; i < sorted.length; i++) {
    const entry = sorted[i]
    const off = dataOffset + i * recordSize
    bytes.set(encodeKey(entry.key, keySize), off)
    bytes[off + keySize] = entry.tombstone ? 1 : 0
    if (entry.value) {
      bytes.set(entry.value.subarray(0, valueSize), off + keySize + 1)
    }
    if (i % indexEvery === 0) {
      const indexOff = indexOffset + Math.floor(i / indexEvery) * INDEX_ENTRY_SIZE
      bytes.set(encodeKey(entry.key, 16), indexOff)
      view.setUint32(indexOff + 16, i, true)
    }
  }
  bytes.set(bloom.bytes, bloomOffset)
  return {
    buffer: buf,
    bytes: totalBytes,
    entryCount: sorted.length,
    bloomFalsePositiveRate: falsePositiveRate,
  }
}

export async function loadTableMetadata(dir, name) {
  const fh = await dir.getFileHandle(name)
  const file = await fh.getFile()
  const headerBuf = await file.slice(0, HEADER_SIZE).arrayBuffer()
  const header = getHeader(new DataView(headerBuf))
  const indexBuf = await file
    .slice(header.indexOffset, header.indexOffset + header.indexCount * INDEX_ENTRY_SIZE)
    .arrayBuffer()
  const indexBytes = new Uint8Array(indexBuf)
  const keys = []
  const rows = []
  for (let i = 0; i < header.indexCount; i++) {
    const off = i * INDEX_ENTRY_SIZE
    keys.push(decodeKey(indexBytes.subarray(off, off + 16)))
    rows.push(new DataView(indexBuf, off + 16, 4).getUint32(0, true))
  }
  const bloomBuf = await file
    .slice(header.bloomOffset, header.bloomOffset + header.bloomBytes)
    .arrayBuffer()
  return {
    ...header,
    name,
    indexKeys: keys,
    indexRows: rows,
    bloomBytesData: new Uint8Array(bloomBuf),
  }
}

function lowerBound(keys, key) {
  let lo = 0
  let hi = keys.length
  while (lo < hi) {
    const mid = (lo + hi) >> 1
    if (keys[mid] < key) {
      lo = mid + 1
    } else {
      hi = mid
    }
  }
  return lo
}

export async function lookupInTable(dir, name, key, meta, metrics, opts = {}) {
  metrics.tablesVisited++
  if (!opts.skipBloom) {
    if (!bloomMayContain(
      meta.bloomBytesData,
      {
        bitCount: meta.bloomBytesData.byteLength * 8,
        hashCount: meta.bloomHashCount,
      },
      key,
    )) {
      metrics.bloomRejects++
      return null
    }
  }
  const pos = Math.max(0, lowerBound(meta.indexKeys, key) - 1)
  const rowStart = meta.indexRows[pos] ?? 0
  const rowEnd = Math.min(meta.entryCount, rowStart + meta.indexEvery)
  const sliceStart = meta.dataOffset + rowStart * meta.recordSize
  const sliceEnd = meta.dataOffset + rowEnd * meta.recordSize
  const fh = await dir.getFileHandle(name)
  const file = await fh.getFile()
  const buf = await file.slice(sliceStart, sliceEnd).arrayBuffer()
  metrics.dataReads++
  metrics.dataBytes += sliceEnd - sliceStart
  const bytes = new Uint8Array(buf)
  for (let row = rowStart; row < rowEnd; row++) {
    const rel = (row - rowStart) * meta.recordSize
    const rowKey = decodeKey(bytes.subarray(rel, rel + meta.keySize))
    if (rowKey === key) {
      const tombstone = bytes[rel + meta.keySize] === 1
      return {
        key: rowKey,
        tombstone,
        value: bytes.slice(
          rel + meta.keySize + 1,
          rel + meta.keySize + 1 + meta.valueSize,
        ),
      }
    }
    if (rowKey > key) {
      return null
    }
  }
  return null
}

export async function readLatestManifest(dir) {
  const manifests = []
  for (const name of ['manifest-a.json', 'manifest-b.json']) {
    try {
      const parsed = await readJsonFile(dir, name)
      manifests.push(parsed)
    } catch (err) {
      if (err?.name !== 'NotFoundError') {
        throw err
      }
    }
  }
  manifests.sort((a, b) => b.version - a.version)
  return manifests[0] ?? { version: 0, tables: [] }
}

export function nextManifestName(version) {
  return version % 2 === 0 ? 'manifest-a.json' : 'manifest-b.json'
}

export function makeManifestTable(name, entryCount, bytes, extra = {}) {
  return {
    name,
    entryCount,
    bytes,
    ...extra,
  }
}

export async function runLookupSeries(dir, manifest, keys, opts = {}) {
  const cache = opts.cache ?? new Map()
  const useCache = opts.useCache ?? false
  const skipBloom = opts.skipBloom ?? false
  const times = []
  const metrics = []
  for (const key of keys) {
    const m = {
      tablesVisited: 0,
      bloomRejects: 0,
      dataReads: 0,
      dataBytes: 0,
      found: false,
    }
    const t0 = performance.now()
    for (const table of manifest.tables) {
      let meta = useCache ? cache.get(table.name) : null
      if (!meta) {
        meta = await loadTableMetadata(dir, table.name)
        if (useCache) {
          cache.set(table.name, meta)
        }
      }
      const row = await lookupInTable(dir, table.name, key, meta, m, {
        skipBloom,
      })
      if (row && !row.tombstone) {
        m.found = true
        break
      }
      if (row && row.tombstone) {
        break
      }
    }
    times.push(performance.now() - t0)
    metrics.push(m)
  }
  return {
    latency: stats(times),
    meanTablesVisited: average(metrics.map((m) => m.tablesVisited)),
    meanBloomRejects: average(metrics.map((m) => m.bloomRejects)),
    meanDataReads: average(metrics.map((m) => m.dataReads)),
    meanDataBytes: average(metrics.map((m) => m.dataBytes)),
    foundRate: average(metrics.map((m) => (m.found ? 1 : 0))),
  }
}

export async function rangeReadWholeSlice(dir, name, meta, startRow, rowCount) {
  const fh = await dir.getFileHandle(name)
  const file = await fh.getFile()
  const sliceStart = meta.dataOffset + startRow * meta.recordSize
  const sliceEnd = sliceStart + rowCount * meta.recordSize
  const t0 = performance.now()
  await file.slice(sliceStart, sliceEnd).arrayBuffer()
  return performance.now() - t0
}

export async function rangeReadManySlices(dir, name, meta, startRow, slices, rowsPerSlice) {
  const fh = await dir.getFileHandle(name)
  const file = await fh.getFile()
  const t0 = performance.now()
  for (let i = 0; i < slices; i++) {
    const row = startRow + i * rowsPerSlice
    const sliceStart = meta.dataOffset + row * meta.recordSize
    const sliceEnd = sliceStart + rowsPerSlice * meta.recordSize
    await file.slice(sliceStart, sliceEnd).arrayBuffer()
  }
  return performance.now() - t0
}

export function makePageStoreLayout(keyCount, valueSize, keysPerLeaf = 12) {
  const leafCount = Math.ceil(keyCount / keysPerLeaf)
  return {
    keyCount,
    valueSize,
    keysPerLeaf,
    leafCount,
    rootOffset: PAGE_SIZE,
    leafOffset(index) {
      return PAGE_SIZE * (2 + index)
    },
    totalBytes: PAGE_SIZE * (2 + leafCount + 2),
  }
}

export function makePageBuffers(layout, keyIndices) {
  const pages = []
  for (const keyIndex of keyIndices) {
    const leaf = Math.floor(keyIndex / layout.keysPerLeaf)
    const buf = new Uint8Array(PAGE_SIZE)
    buf.set(makeValue(Math.min(PAGE_SIZE, layout.valueSize), keyIndex))
    pages.push({
      offset: layout.leafOffset(leaf),
      bytes: buf,
    })
  }
  pages.push({
    offset: layout.rootOffset,
    bytes: new Uint8Array(PAGE_SIZE),
  })
  pages.push({
    offset: 0,
    bytes: new Uint8Array(PAGE_SIZE),
  })
  return pages
}
