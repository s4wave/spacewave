import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

// These tests drive the OPFS worker dispatcher with the exact { op, args } shape
// the Go RemoteDriver sends (a flat object keyed by one canonical field name per
// argument) and assert the decoded calls reach the File System Access API.

type DirCall = { method: string; name: string; opts: unknown }

function handleID(value: unknown): number {
  if (typeof value === 'object' && value !== null && 'id' in value) {
    const id = value.id
    if (typeof id === 'number') {
      return id
    }
  }
  throw new Error(`expected a handle result, got ${JSON.stringify(value)}`)
}

function makeWritable(writes: Uint8Array[]) {
  return {
    async write(data: Uint8Array) {
      writes.push(data)
    },
    async seek(_position: number) {},
    async truncate(_size: number) {},
    async close() {},
    async abort() {},
  }
}

function makeFile(name: string, bytes: Uint8Array, writes: Uint8Array[]) {
  return {
    name,
    async getFile() {
      return {
        size: bytes.byteLength,
        async arrayBuffer() {
          return bytes.buffer
        },
        slice(start: number, end: number) {
          const part = bytes.slice(start, end)
          return {
            async arrayBuffer() {
              return part.buffer
            },
          }
        },
      }
    },
    async createWritable(_opts?: unknown) {
      return makeWritable(writes)
    },
  }
}

function makeDir(
  name: string,
  calls: DirCall[],
  writes: Uint8Array[],
  entryNames: string[],
  fileBytes: Uint8Array,
) {
  return {
    name,
    async getDirectoryHandle(n: string, opts: unknown) {
      calls.push({ method: 'getDirectoryHandle', name: n, opts })
      return makeDir(n, calls, writes, entryNames, fileBytes)
    },
    async getFileHandle(n: string, opts: unknown) {
      calls.push({ method: 'getFileHandle', name: n, opts })
      return makeFile(n, fileBytes, writes)
    },
    async removeEntry(n: string, opts: unknown) {
      calls.push({ method: 'removeEntry', name: n, opts })
    },
    async *entries() {
      for (const entry of entryNames) {
        yield [entry, {}]
      }
    },
  }
}

describe('opfs-worker dispatchOp', () => {
  let calls: DirCall[]
  let writes: Uint8Array[]
  let dispatchOp: (op: string, args: unknown) => Promise<unknown>

  beforeEach(async () => {
    vi.resetModules()
    calls = []
    writes = []
    const root = makeDir('', calls, writes, ['a.txt', 'b.txt'], new Uint8Array([1, 2, 3, 4]))
    vi.stubGlobal('navigator', { storage: { getDirectory: async () => root } })
    ;({ dispatchOp } = await import('./opfs-worker.js'))
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  test('getRoot returns the first handle and caches it', async () => {
    expect(await dispatchOp('getRoot', {})).toEqual({ id: 1 })
    expect(await dispatchOp('getRoot', {})).toEqual({ id: 1 })
  })

  test('getDirectory decodes dir, name, create', async () => {
    await dispatchOp('getRoot', {})
    expect(await dispatchOp('getDirectory', { dir: 1, name: 'sub', create: true })).toEqual({
      id: 2,
    })
    expect(calls).toContainEqual({
      method: 'getDirectoryHandle',
      name: 'sub',
      opts: { create: true },
    })
  })

  test('openFile resolves dir + name without creating', async () => {
    await dispatchOp('getRoot', {})
    expect(await dispatchOp('openFile', { dir: 1, name: 'db' })).toEqual({ id: 2 })
    expect(calls).toContainEqual({
      method: 'getFileHandle',
      name: 'db',
      opts: { create: false },
    })
  })

  test('writeFile writes the transferred bytes and returns the length', async () => {
    await dispatchOp('getRoot', {})
    const data = new Uint8Array([9, 8, 7]).buffer
    expect(await dispatchOp('writeFile', { dir: 1, name: 'f', data })).toBe(3)
    expect(calls).toContainEqual({
      method: 'getFileHandle',
      name: 'f',
      opts: { create: true },
    })
    expect(writes).toHaveLength(1)
    expect(Array.from(writes[0])).toEqual([9, 8, 7])
  })

  test('readAt clamps the slice to the file size', async () => {
    await dispatchOp('getRoot', {})
    const file = handleID(await dispatchOp('openFile', { dir: 1, name: 'db' }))
    const buf = await dispatchOp('readAt', { file, offset: 0, length: 8 })
    expect(buf).toBeInstanceOf(ArrayBuffer)
    if (buf instanceof ArrayBuffer) {
      expect(Array.from(new Uint8Array(buf))).toEqual([1, 2, 3, 4])
    }
  })

  test('listDirectory returns the entry names', async () => {
    await dispatchOp('getRoot', {})
    expect(await dispatchOp('listDirectory', { dir: 1 })).toEqual(['a.txt', 'b.txt'])
  })

  test('createWriteStream then streamWrite decode the stream handle and bytes', async () => {
    await dispatchOp('getRoot', {})
    const stream = handleID(await dispatchOp('createWriteStream', { dir: 1, name: 's' }))
    expect(await dispatchOp('streamWrite', { stream, data: new Uint8Array([5, 6]).buffer })).toBe(2)
    expect(Array.from(writes[writes.length - 1])).toEqual([5, 6])
  })

  test('a missing required field rejects with a TypeError', async () => {
    await dispatchOp('getRoot', {})
    await expect(dispatchOp('getDirectory', { dir: 1, create: true })).rejects.toThrow(
      /expected string field name/,
    )
  })

  test('an unknown op rejects', async () => {
    await expect(dispatchOp('frobnicate', {})).rejects.toThrow(/unsupported OPFS op/)
  })
})
