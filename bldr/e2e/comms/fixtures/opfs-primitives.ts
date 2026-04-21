// opfs-primitives.ts - OPFS primitives verification fixture.
//
// Tests all OPFS operations that the Go hydra/opfs package wraps:
// GetRoot, GetDirectory, WriteFile, ReadFile, DeleteFile, ListDirectory,
// and error handling (not-found errors).

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      getRoot: boolean
      createDir: boolean
      nestedDir: boolean
      writeRead: boolean
      overwrite: boolean
      deleteFile: boolean
      listDir: boolean
      notFoundFile: boolean
      notFoundDir: boolean
      deleteNotFound: boolean
    }
  }
}

async function run() {
  const log = document.getElementById('log')!
  const errors: string[] = []

  const results = {
    pass: false,
    detail: '',
    getRoot: false,
    createDir: false,
    nestedDir: false,
    writeRead: false,
    overwrite: false,
    deleteFile: false,
    listDir: false,
    notFoundFile: false,
    notFoundDir: false,
    deleteNotFound: false,
  }

  try {
    // Use a unique test prefix to avoid collisions with other tests.
    const prefix = `test-${Date.now()}`

    // --- Iteration 1: GetRoot, GetDirectory ---

    // GetRoot
    const root = await navigator.storage.getDirectory()
    results.getRoot = true

    // GetDirectory (create)
    const testDir = await root.getDirectoryHandle(prefix, { create: true })
    results.createDir = true

    // Nested directory tree
    const a = await testDir.getDirectoryHandle('a', { create: true })
    const b = await a.getDirectoryHandle('b', { create: true })
    const c = await b.getDirectoryHandle('c', { create: true })
    // Verify we can re-open the nested path
    const aAgain = await testDir.getDirectoryHandle('a', { create: false })
    const bAgain = await aAgain.getDirectoryHandle('b', { create: false })
    await bAgain.getDirectoryHandle('c', { create: false })
    results.nestedDir = true

    // --- Iteration 2: WriteFile, ReadFile, DeleteFile ---

    // Write and read
    const testData = new Uint8Array([72, 101, 108, 108, 111]) // "Hello"
    const fileDir = await testDir.getDirectoryHandle('files', { create: true })

    const fh = await fileDir.getFileHandle('test.bin', { create: true })
    const writable = await fh.createWritable()
    await writable.write(testData)
    await writable.close()

    const fhRead = await fileDir.getFileHandle('test.bin')
    const file = await fhRead.getFile()
    const ab = await file.arrayBuffer()
    const readData = new Uint8Array(ab)
    if (readData.length !== testData.length) {
      errors.push(`read length mismatch: got ${readData.length}, want ${testData.length}`)
    } else {
      let match = true
      for (let i = 0; i < testData.length; i++) {
        if (readData[i] !== testData[i]) {
          match = false
          break
        }
      }
      if (!match) {
        errors.push('read data mismatch')
      }
    }
    results.writeRead = errors.length === 0

    // Overwrite
    const newData = new Uint8Array([87, 111, 114, 108, 100]) // "World"
    const fhOw = await fileDir.getFileHandle('test.bin', { create: true })
    const wOw = await fhOw.createWritable()
    await wOw.write(newData)
    await wOw.close()

    const fhOr = await fileDir.getFileHandle('test.bin')
    const fOr = await fhOr.getFile()
    const abOr = await fOr.arrayBuffer()
    const orData = new Uint8Array(abOr)
    let owMatch = orData.length === newData.length
    if (owMatch) {
      for (let i = 0; i < newData.length; i++) {
        if (orData[i] !== newData[i]) {
          owMatch = false
          break
        }
      }
    }
    if (!owMatch) {
      errors.push('overwrite data mismatch')
    }
    results.overwrite = owMatch

    // Delete
    await fileDir.removeEntry('test.bin')
    let deletedGone = false
    try {
      await fileDir.getFileHandle('test.bin')
    } catch (e: any) {
      if (e.name === 'NotFoundError') {
        deletedGone = true
      }
    }
    if (!deletedGone) {
      errors.push('file still exists after delete')
    }
    results.deleteFile = deletedGone

    // --- Iteration 3: ListDirectory ---

    const listDir = await testDir.getDirectoryHandle('listtest', { create: true })
    const fileNames = ['charlie.txt', 'alpha.txt', 'bravo.txt']
    for (const name of fileNames) {
      const lfh = await listDir.getFileHandle(name, { create: true })
      const lw = await lfh.createWritable()
      await lw.write(new Uint8Array([0]))
      await lw.close()
    }

    // Also create a subdirectory to verify it appears in listing
    await listDir.getDirectoryHandle('delta-dir', { create: true })

    const entries: string[] = []
    for await (const [name] of (listDir as any).entries()) {
      entries.push(name)
    }

    const sorted = [...entries].sort()
    const expected = ['alpha.txt', 'bravo.txt', 'charlie.txt', 'delta-dir']
    if (sorted.length !== expected.length) {
      errors.push(`list length: got ${sorted.length}, want ${expected.length}`)
    } else {
      for (let i = 0; i < expected.length; i++) {
        if (sorted[i] !== expected[i]) {
          errors.push(`list[${i}]: got ${sorted[i]}, want ${expected[i]}`)
        }
      }
    }
    results.listDir = errors.filter((e) => e.startsWith('list')).length === 0

    // --- Iteration 4: Error handling ---

    // ReadFile on missing file returns NotFoundError
    let notFoundFile = false
    try {
      await fileDir.getFileHandle('nonexistent.bin')
    } catch (e: any) {
      if (e.name === 'NotFoundError') {
        notFoundFile = true
      } else {
        errors.push(`missing file error name: ${e.name}`)
      }
    }
    results.notFoundFile = notFoundFile

    // GetDirectory with create=false on missing dir returns NotFoundError
    let notFoundDir = false
    try {
      await testDir.getDirectoryHandle('nonexistent-dir', { create: false })
    } catch (e: any) {
      if (e.name === 'NotFoundError') {
        notFoundDir = true
      } else {
        errors.push(`missing dir error name: ${e.name}`)
      }
    }
    results.notFoundDir = notFoundDir

    // DeleteFile on missing file returns NotFoundError
    let deleteNotFound = false
    try {
      await fileDir.removeEntry('nonexistent.bin')
    } catch (e: any) {
      if (e.name === 'NotFoundError') {
        deleteNotFound = true
      } else {
        errors.push(`delete missing error name: ${e.name}`)
      }
    }
    results.deleteNotFound = deleteNotFound

    // Cleanup: remove test directory tree
    await root.removeEntry(prefix, { recursive: true })

    results.pass = errors.length === 0
    results.detail =
      errors.length > 0 ? errors.join('; ') : 'all opfs primitives tests passed'
  } catch (err) {
    results.pass = false
    results.detail = `error: ${err}`
  }

  window.__results = results
  log.textContent = 'DONE'
}

run()
