// opfs-volume.ts - OPFS volume lifecycle integration fixture.
//
// Exercises the full volume lifecycle: create directory tree, write/read
// entries with sharding, verify persistence across "reopen" (clear JS
// references and re-navigate the OPFS tree), and cleanup via directory removal.

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      createVolume: boolean
      writeEntries: boolean
      readEntries: boolean
      persistence: boolean
      deleteVolume: boolean
      webLockIsolation: boolean
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

async function run() {
  const log = document.getElementById('log')!
  const errors: string[] = []

  const results: Window['__results'] = {
    pass: false,
    detail: '',
    createVolume: false,
    writeEntries: false,
    readEntries: false,
    persistence: false,
    deleteVolume: false,
    webLockIsolation: false,
  }

  try {
    const opfsRoot = await navigator.storage.getDirectory()
    const volId = `vol-test-${Date.now()}`

    // --- Integration 1: Create volume directory tree ---

    const volDir = await opfsRoot.getDirectoryHandle(volId, { create: true })
    results.createVolume = true

    // --- Integration 1+2: Write entries via kvtx pattern ---

    // Simulate writing several entries with 2-char hex sharding.
    const entries = [
      {
        key: new Uint8Array([0x01, 0x23]),
        value: new Uint8Array([0x41, 0x42, 0x43]),
      },
      {
        key: new Uint8Array([0x01, 0x45]),
        value: new Uint8Array([0x44, 0x45]),
      },
      {
        key: new Uint8Array([0xab, 0xcd]),
        value: new Uint8Array([0x46, 0x47, 0x48, 0x49]),
      },
      {
        key: new Uint8Array([0xab, 0xef]),
        value: new Uint8Array([0x50]),
      },
      {
        key: new Uint8Array([0xff, 0x01]),
        value: new Uint8Array([0x51, 0x52, 0x53, 0x54, 0x55]),
      },
    ]

    // Write under exclusive WebLock (simulating write transaction).
    await navigator.locks.request(`${volId}|kvtx`, { mode: 'exclusive' }, async () => {
      for (const { key, value } of entries) {
        const hex = hexEncode(key)
        const shard = hex.substring(0, 2)
        const shardDir = await volDir.getDirectoryHandle(shard, { create: true })
        const fh = await shardDir.getFileHandle(hex, { create: true })
        const w = await fh.createWritable()
        await w.write(value)
        await w.close()
      }
    })
    results.writeEntries = true

    // --- Integration 1: Read back under shared lock ---

    let readOk = true
    await navigator.locks.request(`${volId}|kvtx`, { mode: 'shared' }, async () => {
      for (const { key, value } of entries) {
        const hex = hexEncode(key)
        const shard = hex.substring(0, 2)
        const shardDir = await volDir.getDirectoryHandle(shard, { create: false })
        const fh = await shardDir.getFileHandle(hex)
        const file = await fh.getFile()
        const ab = await file.arrayBuffer()
        const readValue = new Uint8Array(ab)
        if (!arraysEqual(readValue, value)) {
          errors.push(`read mismatch for key ${hex}`)
          readOk = false
        }
      }
    })
    results.readEntries = readOk

    // --- Integration 1: Persistence across "reopen" ---

    // Clear all JS references and re-navigate the OPFS tree.
    // This simulates a volume reopen (new Go runtime, same OPFS data).
    const volDir2 = await opfsRoot.getDirectoryHandle(volId, { create: false })

    let persistOk = true
    for (const { key, value } of entries) {
      const hex = hexEncode(key)
      const shard = hex.substring(0, 2)
      try {
        const shardDir = await volDir2.getDirectoryHandle(shard, { create: false })
        const fh = await shardDir.getFileHandle(hex)
        const file = await fh.getFile()
        const ab = await file.arrayBuffer()
        const readValue = new Uint8Array(ab)
        if (!arraysEqual(readValue, value)) {
          errors.push(`persistence mismatch for key ${hex}`)
          persistOk = false
        }
      } catch (e: any) {
        errors.push(`persistence error for key ${hex}: ${e.message}`)
        persistOk = false
      }
    }
    results.persistence = persistOk

    // --- Integration 2: WebLock isolation ---

    // Verify shared locks don't block each other but exclusive blocks shared.
    let lockOk = true

    // Two shared locks should both acquire.
    const sharedResults: boolean[] = []
    await Promise.all([
      navigator.locks.request(
        `${volId}|lock-test`,
        { mode: 'shared' },
        async () => {
          sharedResults.push(true)
          // Hold the lock briefly.
          await new Promise((r) => setTimeout(r, 50))
        },
      ),
      navigator.locks.request(
        `${volId}|lock-test`,
        { mode: 'shared' },
        async () => {
          sharedResults.push(true)
          await new Promise((r) => setTimeout(r, 50))
        },
      ),
    ])
    if (sharedResults.length !== 2) {
      errors.push('expected 2 shared locks to acquire concurrently')
      lockOk = false
    }

    // Exclusive lock blocks shared.
    let exclusiveHeld = false
    let sharedAfterExclusive = false
    const exclusiveDone = navigator.locks.request(
      `${volId}|lock-test2`,
      { mode: 'exclusive' },
      async () => {
        exclusiveHeld = true
        await new Promise((r) => setTimeout(r, 100))
        exclusiveHeld = false
      },
    )

    // Give the exclusive lock a moment to acquire.
    await new Promise((r) => setTimeout(r, 20))

    // This shared lock should wait until exclusive releases.
    await Promise.all([
      exclusiveDone,
      navigator.locks.request(
        `${volId}|lock-test2`,
        { mode: 'shared' },
        async () => {
          sharedAfterExclusive = !exclusiveHeld
        },
      ),
    ])
    if (!sharedAfterExclusive) {
      errors.push('shared lock acquired while exclusive was held')
      lockOk = false
    }
    results.webLockIsolation = lockOk

    // --- Cleanup: Delete volume ---

    await opfsRoot.removeEntry(volId, { recursive: true })

    // Verify deletion
    let deleteOk = false
    try {
      await opfsRoot.getDirectoryHandle(volId, { create: false })
    } catch (e: any) {
      if (e.name === 'NotFoundError') deleteOk = true
    }
    results.deleteVolume = deleteOk

    results.pass = errors.length === 0
    results.detail =
      errors.length > 0 ? errors.join('; ') : 'all volume integration tests passed'
  } catch (err) {
    results.pass = false
    results.detail = `error: ${err}`
  }

  window.__results = results
  log.textContent = 'DONE'
}

run()
