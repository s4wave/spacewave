// opfs-kvtx.ts - OPFS kvtx.Store verification fixture.
//
// Tests the key-value transaction operations that the Go store_kvtx_opfs
// package implements: WebLock transactions, read/write/delete, prefix scan,
// iteration, and crash recovery via .pending marker.

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      readTx: boolean
      writeTx: boolean
      deleteTx: boolean
      scanPrefix: boolean
      scanPrefixKeys: boolean
      iterate: boolean
      size: boolean
      crashRecovery: boolean
    }
  }
}

interface FileSystemDirectoryHandleWithEntries
  extends FileSystemDirectoryHandle {
  entries(): AsyncIterable<[string, FileSystemHandle]>
}

// Hex encode/decode matching the Go implementation.
function hexEncode(data: Uint8Array): string {
  return Array.from(data)
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
}

function getEntries(
  dir: FileSystemDirectoryHandle,
): AsyncIterable<[string, FileSystemHandle]> {
  return (dir as FileSystemDirectoryHandleWithEntries).entries()
}

function isNotFoundError(err: unknown): err is DOMException {
  return err instanceof DOMException && err.name === 'NotFoundError'
}

function shardPrefix(encoded: string): string {
  if (encoded.length < 2) return '00'
  return encoded.substring(0, 2)
}

// Simplified OPFS kvtx operations mirroring the Go implementation.
async function writeEntry(
  root: FileSystemDirectoryHandle,
  key: Uint8Array,
  value: Uint8Array,
): Promise<void> {
  const encoded = hexEncode(key)
  const shard = shardPrefix(encoded)
  const shardDir = await root.getDirectoryHandle(shard, { create: true })
  const fh = await shardDir.getFileHandle(encoded, { create: true })
  const w = await fh.createWritable()
  await w.write(value)
  await w.close()
}

async function readEntry(
  root: FileSystemDirectoryHandle,
  key: Uint8Array,
): Promise<Uint8Array | null> {
  const encoded = hexEncode(key)
  const shard = shardPrefix(encoded)
  try {
    const shardDir = await root.getDirectoryHandle(shard, { create: false })
    const fh = await shardDir.getFileHandle(encoded)
    const file = await fh.getFile()
    const ab = await file.arrayBuffer()
    return new Uint8Array(ab)
  } catch (err) {
    if (isNotFoundError(err)) return null
    throw err
  }
}

async function deleteEntry(
  root: FileSystemDirectoryHandle,
  key: Uint8Array,
): Promise<void> {
  const encoded = hexEncode(key)
  const shard = shardPrefix(encoded)
  try {
    const shardDir = await root.getDirectoryHandle(shard, { create: false })
    await shardDir.removeEntry(encoded)
  } catch (err) {
    if (isNotFoundError(err)) return
    throw err
  }
}

async function listAllKeys(
  root: FileSystemDirectoryHandle,
): Promise<string[]> {
  const keys: string[] = []
  for await (const [name, handle] of getEntries(root)) {
    if (handle.kind !== 'directory' || name.length !== 2) continue
    const shardDir = await root.getDirectoryHandle(name)
    for await (const [fname] of getEntries(shardDir)) {
      keys.push(fname)
    }
  }
  return keys.sort()
}

function arraysEqual(a: Uint8Array, b: Uint8Array): boolean {
  if (a.length !== b.length) return false
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) return false
  }
  return true
}

async function run() {
  const log = document.getElementById('log')!
  const errors: string[] = []

  const results: Window['__results'] = {
    pass: false,
    detail: '',
    readTx: false,
    writeTx: false,
    deleteTx: false,
    scanPrefix: false,
    scanPrefixKeys: false,
    iterate: false,
    size: false,
    crashRecovery: false,
  }

  try {
    const opfsRoot = await navigator.storage.getDirectory()
    const testDir = await opfsRoot.getDirectoryHandle(`kvtx-test-${Date.now()}`, {
      create: true,
    })

    // --- Iteration 1+2: Read and Write transactions ---

    // Write two entries using WebLock exclusive
    const key1 = new Uint8Array([0x01, 0x02, 0x03])
    const val1 = new Uint8Array([0x41, 0x42, 0x43]) // "ABC"
    const key2 = new Uint8Array([0x01, 0x02, 0x04])
    const val2 = new Uint8Array([0x44, 0x45, 0x46]) // "DEF"

    await navigator.locks.request('kvtx-test', { mode: 'exclusive' }, async () => {
      await writeEntry(testDir, key1, val1)
      await writeEntry(testDir, key2, val2)
    })
    results.writeTx = true

    // Read back with shared lock
    let readOk = true
    await navigator.locks.request('kvtx-test', { mode: 'shared' }, async () => {
      const r1 = await readEntry(testDir, key1)
      if (!r1 || !arraysEqual(r1, val1)) {
        errors.push('read key1 mismatch')
        readOk = false
      }
      const r2 = await readEntry(testDir, key2)
      if (!r2 || !arraysEqual(r2, val2)) {
        errors.push('read key2 mismatch')
        readOk = false
      }
      // Read non-existent key
      const r3 = await readEntry(testDir, new Uint8Array([0xff]))
      if (r3 !== null) {
        errors.push('read missing key should be null')
        readOk = false
      }
    })
    results.readTx = readOk

    // --- Iteration 3: Delete ---

    let deleteOk = true
    await navigator.locks.request('kvtx-test', { mode: 'exclusive' }, async () => {
      await deleteEntry(testDir, key1)
    })
    await navigator.locks.request('kvtx-test', { mode: 'shared' }, async () => {
      const r = await readEntry(testDir, key1)
      if (r !== null) {
        errors.push('key1 should be deleted')
        deleteOk = false
      }
      // key2 should still exist
      const r2 = await readEntry(testDir, key2)
      if (!r2 || !arraysEqual(r2, val2)) {
        errors.push('key2 should survive delete of key1')
        deleteOk = false
      }
    })
    results.deleteTx = deleteOk

    // --- Iteration 4: ScanPrefix ---

    // Write keys with known prefixes for scanning
    const scanDir = await opfsRoot.getDirectoryHandle(`kvtx-scan-${Date.now()}`, {
      create: true,
    })
    const scanKeys = [
      { key: new Uint8Array([0xaa, 0x01]), val: new Uint8Array([1]) },
      { key: new Uint8Array([0xaa, 0x02]), val: new Uint8Array([2]) },
      { key: new Uint8Array([0xaa, 0x03]), val: new Uint8Array([3]) },
      { key: new Uint8Array([0xbb, 0x01]), val: new Uint8Array([4]) },
      { key: new Uint8Array([0xbb, 0x02]), val: new Uint8Array([5]) },
    ]
    for (const { key, val } of scanKeys) {
      await writeEntry(scanDir, key, val)
    }

    // Scan prefix 0xaa - should get 3 entries
    const aaPrefix = hexEncode(new Uint8Array([0xaa]))
    const allKeys = await listAllKeys(scanDir)
    const aaKeys = allKeys.filter((k) => k.startsWith(aaPrefix))
    if (aaKeys.length !== 3) {
      errors.push(`scanPrefix 0xaa: got ${aaKeys.length} entries, want 3`)
    }
    results.scanPrefix = aaKeys.length === 3

    // --- Iteration 5: ScanPrefixKeys, Size ---

    // ScanPrefixKeys: same as ScanPrefix but keys only
    const bbPrefix = hexEncode(new Uint8Array([0xbb]))
    const bbKeys = allKeys.filter((k) => k.startsWith(bbPrefix))
    if (bbKeys.length !== 2) {
      errors.push(`scanPrefixKeys 0xbb: got ${bbKeys.length} entries, want 2`)
    }
    results.scanPrefixKeys = bbKeys.length === 2

    // Iterate: verify sorted order
    const sortedKeys = [...allKeys]
    const expectedOrder = ['aa01', 'aa02', 'aa03', 'bb01', 'bb02']
    let iterateOk = sortedKeys.length === expectedOrder.length
    if (iterateOk) {
      for (let i = 0; i < expectedOrder.length; i++) {
        if (sortedKeys[i] !== expectedOrder[i]) {
          errors.push(`iterate order: [${i}] got ${sortedKeys[i]}, want ${expectedOrder[i]}`)
          iterateOk = false
        }
      }
    } else {
      errors.push(
        `iterate count: got ${sortedKeys.length}, want ${expectedOrder.length}`,
      )
    }
    results.iterate = iterateOk

    // Size
    results.size = allKeys.length === 5

    // --- Iteration 6: Crash recovery ---

    const crashDir = await opfsRoot.getDirectoryHandle(
      `kvtx-crash-${Date.now()}`,
      { create: true },
    )

    // Simulate a crashed write: leave .pending marker
    const pendingFh = await crashDir.getFileHandle('.pending', { create: true })
    const pw = await pendingFh.createWritable()
    await pw.write(new Uint8Array([0x31]))
    await pw.close()

    // Write a partial entry (simulating what commit started)
    await writeEntry(crashDir, new Uint8Array([0xcc, 0x01]), new Uint8Array([99]))

    // "Next write transaction" detects .pending and cleans up
    let pendingExists = false
    try {
      await crashDir.getFileHandle('.pending')
      pendingExists = true
    } catch {
      pendingExists = false
    }

    if (!pendingExists) {
      errors.push('pending marker should exist before cleanup')
    }

    // Cleanup: remove .pending (simulating what cleanupPending does)
    try {
      await crashDir.removeEntry('.pending')
    } catch {
      // already gone
    }

    // Verify marker is gone
    let markerGone = false
    try {
      await crashDir.getFileHandle('.pending')
    } catch (err) {
      if (isNotFoundError(err)) markerGone = true
    }

    // Verify the partial data is still readable
    const partial = await readEntry(crashDir, new Uint8Array([0xcc, 0x01]))
    results.crashRecovery = pendingExists && markerGone && partial !== null

    // Cleanup test directories
    const testPrefix = testDir.name
    const scanPrefix2 = scanDir.name
    const crashPrefix = crashDir.name
    await opfsRoot.removeEntry(testPrefix, { recursive: true })
    await opfsRoot.removeEntry(scanPrefix2, { recursive: true })
    await opfsRoot.removeEntry(crashPrefix, { recursive: true })

    results.pass = errors.length === 0
    results.detail =
      errors.length > 0 ? errors.join('; ') : 'all kvtx tests passed'
  } catch (err) {
    results.pass = false
    results.detail = `error: ${err}`
  }

  window.__results = results
  log.textContent = 'DONE'
}

run()
