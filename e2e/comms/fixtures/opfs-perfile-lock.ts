// opfs-perfile-lock.ts - Per-file WebLock + OPFS contention fixture.
//
// Validates the locking protocol used by hydra's OPFS volume stores:
// - Per-file WebLock acquisition (exclusive locks per file, not per store)
// - Concurrent writes to different files proceed in parallel
// - Concurrent writes to the same file serialize correctly
// - Block store pattern: content-addressed put with shard directories
// - Object store pattern: readers-writer WebLock + per-file write locks

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      perFileLock: boolean
      parallelDistinct: boolean
      serialSameFile: boolean
      blockStorePattern: boolean
      objStoreReadWrite: boolean
      objStoreAcid: boolean
    }
  }
}

function hexEncode(data: Uint8Array): string {
  return Array.from(data)
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
}

function arraysEqual(a: Uint8Array, b: Uint8Array): boolean {
  if (a.length !== b.length) return false
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) return false
  }
  return true
}

// acquireFileExclusive mimics the Go filelock.AcquireFile protocol:
// acquire exclusive WebLock for the file, then open a writable handle.
async function acquireFileExclusive(
  dir: FileSystemDirectoryHandle,
  name: string,
  lockName: string,
): Promise<{ writable: FileSystemWritableFileStream; release: () => void }> {
  let releaseLock: (() => void) | null = null

  await new Promise<void>((resolve) => {
    navigator.locks.request(lockName, { mode: 'exclusive' }, () => {
      return new Promise<void>((relFn) => {
        releaseLock = relFn
        resolve()
      })
    })
  })

  const fh = await dir.getFileHandle(name, { create: true })
  const writable = await fh.createWritable()

  return {
    writable,
    release: () => {
      releaseLock?.()
    },
  }
}

async function run() {
  const log = document.getElementById('log')!
  const errors: string[] = []

  const results: Window['__results'] = {
    pass: false,
    detail: '',
    perFileLock: false,
    parallelDistinct: false,
    serialSameFile: false,
    blockStorePattern: false,
    objStoreReadWrite: false,
    objStoreAcid: false,
  }

  try {
    const opfsRoot = await navigator.storage.getDirectory()
    const testId = `pfl-test-${Date.now()}`
    const testDir = await opfsRoot.getDirectoryHandle(testId, { create: true })

    // --- Test 1: Per-file exclusive lock ---
    {
      const lockName = `${testId}/file-a`
      const { writable, release } = await acquireFileExclusive(testDir, 'file-a', lockName)
      const enc = new TextEncoder()
      await writable.write(enc.encode('locked-write'))
      await writable.close()
      release()

      // Read back.
      const fh = await testDir.getFileHandle('file-a')
      const file = await fh.getFile()
      const text = await file.text()
      if (text !== 'locked-write') {
        errors.push(`per-file lock: got ${JSON.stringify(text)}`)
      } else {
        results.perFileLock = true
      }
    }

    // --- Test 2: Parallel writes to distinct files ---
    {
      const n = 5
      const promises: Promise<void>[] = []
      for (let i = 0; i < n; i++) {
        const fname = `par-${i}`
        const lockName = `${testId}/par-${i}`
        promises.push(
          (async () => {
            const { writable, release } = await acquireFileExclusive(testDir, fname, lockName)
            const enc = new TextEncoder()
            await writable.write(enc.encode(`data-${i}`))
            await writable.close()
            release()
          })(),
        )
      }
      await Promise.all(promises)

      // Verify all files.
      let allOk = true
      for (let i = 0; i < n; i++) {
        const fh = await testDir.getFileHandle(`par-${i}`)
        const file = await fh.getFile()
        const text = await file.text()
        if (text !== `data-${i}`) {
          errors.push(`parallel distinct ${i}: got ${JSON.stringify(text)}`)
          allOk = false
        }
      }
      results.parallelDistinct = allOk
    }

    // --- Test 3: Serial writes to the same file (contention) ---
    {
      // Create the file first.
      const fh0 = await testDir.getFileHandle('counter', { create: true })
      const w0 = await fh0.createWritable()
      await w0.write('0')
      await w0.close()

      const n = 10
      const lockName = `${testId}/counter`
      const promises: Promise<void>[] = []
      for (let i = 0; i < n; i++) {
        promises.push(
          (async () => {
            // Acquire exclusive lock.
            let releaseLock: (() => void) | null = null
            await new Promise<void>((resolve) => {
              navigator.locks.request(lockName, { mode: 'exclusive' }, () => {
                return new Promise<void>((relFn) => {
                  releaseLock = relFn
                  resolve()
                })
              })
            })

            // Read current value.
            const fh = await testDir.getFileHandle('counter')
            const file = await fh.getFile()
            const val = parseInt(await file.text(), 10)

            // Write incremented value.
            const w = await fh.createWritable()
            await w.write(String(val + 1))
            await w.close()

            releaseLock?.()
          })(),
        )
      }
      await Promise.all(promises)

      // Verify final value.
      const fhFinal = await testDir.getFileHandle('counter')
      const fileFinal = await fhFinal.getFile()
      const finalVal = parseInt(await fileFinal.text(), 10)
      if (finalVal !== n) {
        errors.push(`serial same file: counter=${finalVal}, want ${n}`)
      } else {
        results.serialSameFile = true
      }
    }

    // --- Test 4: Block store pattern (content-addressed + shard dirs) ---
    {
      const blocksDir = await testDir.getDirectoryHandle('blocks', { create: true })

      // Simulate 3 block puts with 2-char shard prefix.
      const blocks = [
        { key: 'ab1234', data: new Uint8Array([1, 2, 3]) },
        { key: 'ab5678', data: new Uint8Array([4, 5, 6]) },
        { key: 'cd9012', data: new Uint8Array([7, 8, 9]) },
      ]

      for (const b of blocks) {
        const shard = b.key.substring(0, 2)
        const shardDir = await blocksDir.getDirectoryHandle(shard, { create: true })
        const lockName = `${testId}/blocks/${shard}/${b.key}`

        const { writable, release } = await acquireFileExclusive(shardDir, b.key, lockName)
        await writable.write(b.data)
        await writable.close()
        release()
      }

      // Read back and verify.
      let allOk = true
      for (const b of blocks) {
        const shard = b.key.substring(0, 2)
        const shardDir = await blocksDir.getDirectoryHandle(shard)
        const fh = await shardDir.getFileHandle(b.key)
        const file = await fh.getFile()
        const ab = await file.arrayBuffer()
        if (!arraysEqual(new Uint8Array(ab), b.data)) {
          errors.push(`block ${b.key}: data mismatch`)
          allOk = false
        }
      }

      // Idempotent put: writing same key again should not error.
      const shard0 = blocks[0].key.substring(0, 2)
      const shardDir0 = await blocksDir.getDirectoryHandle(shard0)
      const lockName0 = `${testId}/blocks/${shard0}/${blocks[0].key}`
      const { writable: w2, release: r2 } = await acquireFileExclusive(
        shardDir0,
        blocks[0].key,
        lockName0,
      )
      await w2.write(blocks[0].data)
      await w2.close()
      r2()

      results.blockStorePattern = allOk
    }

    // --- Test 5: Object store read/write with readers-writer WebLock ---
    {
      const objDir = await testDir.getDirectoryHandle('objects', { create: true })
      const objLock = `${testId}|objstore`

      // Write under exclusive lock.
      await navigator.locks.request(objLock, { mode: 'exclusive' }, async () => {
        const entries = [
          { key: new Uint8Array([0x01, 0x02]), value: new Uint8Array([0x41, 0x42]) },
          { key: new Uint8Array([0x03, 0x04]), value: new Uint8Array([0x43, 0x44, 0x45]) },
        ]
        for (const { key, value } of entries) {
          const hex = hexEncode(key)
          const shard = hex.substring(0, 2)
          const shardDir = await objDir.getDirectoryHandle(shard, { create: true })
          const perFileLock = `${testId}/obj/${shard}/${hex}`

          // Per-file lock within the exclusive WebLock.
          await navigator.locks.request(perFileLock, { mode: 'exclusive' }, async () => {
            const fh = await shardDir.getFileHandle(hex, { create: true })
            const w = await fh.createWritable()
            await w.write(value)
            await w.close()
          })
        }
      })

      // Read under shared lock.
      let readOk = true
      await navigator.locks.request(objLock, { mode: 'shared' }, async () => {
        const hex1 = hexEncode(new Uint8Array([0x01, 0x02]))
        const shard1 = hex1.substring(0, 2)
        const shardDir1 = await objDir.getDirectoryHandle(shard1)
        const fh1 = await shardDir1.getFileHandle(hex1)
        const file1 = await fh1.getFile()
        const ab1 = await file1.arrayBuffer()
        if (!arraysEqual(new Uint8Array(ab1), new Uint8Array([0x41, 0x42]))) {
          errors.push('objstore read key 0102: data mismatch')
          readOk = false
        }
      })
      results.objStoreReadWrite = readOk
    }

    // --- Test 6: Object store ACID (exclusive blocks shared) ---
    {
      const acidLock = `${testId}|acid`
      const events: string[] = []

      // Start exclusive write.
      const writeDone = navigator.locks.request(acidLock, { mode: 'exclusive' }, async () => {
        events.push('write-start')
        await new Promise((r) => setTimeout(r, 100))
        events.push('write-end')
      })

      // Give write a moment to acquire.
      await new Promise((r) => setTimeout(r, 20))

      // Start shared read (should wait for write to finish).
      const readDone = navigator.locks.request(acidLock, { mode: 'shared' }, async () => {
        events.push('read-start')
      })

      await Promise.all([writeDone, readDone])

      // write-start should come before write-end before read-start.
      const writeStartIdx = events.indexOf('write-start')
      const writeEndIdx = events.indexOf('write-end')
      const readStartIdx = events.indexOf('read-start')

      if (writeStartIdx < writeEndIdx && writeEndIdx < readStartIdx) {
        results.objStoreAcid = true
      } else {
        errors.push(`ACID ordering: ${events.join(', ')}`)
      }
    }

    // --- Cleanup ---
    await opfsRoot.removeEntry(testId, { recursive: true })

    results.pass = errors.length === 0
    results.detail =
      errors.length > 0 ? errors.join('; ') : 'all per-file lock tests passed'
  } catch (err) {
    results.pass = false
    results.detail = `error: ${err}`
  }

  window.__results = results
  log.textContent = 'DONE'
}

run()
