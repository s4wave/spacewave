import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  GoWasmProcess,
  installOPFSBroadcastHelpers,
  installTinyGoJSHelpers,
  patchTinyGoRuntimeImports,
  type TinyGoRuntime,
} from './go-process.js'

type TinyGoHelperGlobal = typeof globalThis & {
  BLDR_TINYGO_JS_CALL?: (
    target: Record<PropertyKey, unknown>,
    method: PropertyKey,
    ...args: unknown[]
  ) => unknown
  BLDR_TINYGO_JS_NEW?: <TArgs extends unknown[]>(
    ctor: new (...args: TArgs) => unknown,
    ...args: TArgs
  ) => unknown
  BLDR_TINYGO_PROMISE_AWAIT?: <TValue>(
    promise: Promise<TValue>,
    resolve: (value: TValue) => void,
    reject: (reason: number) => void,
  ) => void
  BLDR_TINYGO_NEW_BYTES?: (len: number) => Uint8Array
  BLDR_TINYGO_TAKE_STORED_BYTES?: (id: number) => Uint8Array | undefined
  BLDR_OPFS_ACQUIRE_WEB_LOCK?: (
    name: string,
    mode: LockMode,
    ifAvailable: boolean,
    resolve: (release: () => void, acquired: boolean) => void,
    reject: (code: number) => void,
  ) => void
  BLDR_TINYGO_PUSH_BYTES?: (
    sink: { push: (message: Uint8Array) => void },
    bytes: Uint8Array,
  ) => boolean
  BLDR_TINYGO_POST_BYTES?: (
    port: { postMessage: (message: Uint8Array) => void },
    bytes: Uint8Array,
  ) => boolean
  BLDR_OPFS_READ_FILE?: (
    dir: FileSystemDirectoryHandle,
    name: string,
    opID: number,
    resolve: (opID: number, id: number, len: number) => void,
    reject: (opID: number, code: number) => void,
  ) => void
  BLDR_OPFS_READ_AT?: (
    handle: FileSystemFileHandle,
    dst: Uint8Array,
    off: number,
    opID: number,
    resolve: (opID: number, read: number) => void,
    reject: (opID: number, code: number) => void,
  ) => void
  BLDR_OPFS_LIST_DIRECTORY?: (
    dir: FileSystemDirectoryHandle,
    opID: number,
    resolve: (opID: number, id: number, len: number) => void,
    reject: (opID: number, code: number) => void,
  ) => void
  BLDR_OPFS_WRITE_AT?: (
    handle: FileSystemFileHandle,
    data: Uint8Array,
    off: number,
    keepExisting: boolean,
    opID: number,
    resolve: (opID: number, written: number) => void,
    reject: (opID: number, code: number) => void,
  ) => void
  BLDR_OPFS_WRITE_FILE?: (
    dir: FileSystemDirectoryHandle,
    name: string,
    data: Uint8Array,
    opID: number,
    resolve: (opID: number, written: number) => void,
    reject: (opID: number, code: number) => void,
  ) => void
  BLDR_OPFS_WRITE_FILE_BEGIN?: (
    dir: FileSystemDirectoryHandle,
    name: string,
    opID: number,
    resolve: (opID: number, sessionID: number) => void,
    reject: (opID: number, code: number) => void,
  ) => void
  BLDR_OPFS_WRITE_FILE_CHUNK?: (
    sessionID: number,
    data: Uint8Array,
    opID: number,
    resolve: (opID: number, written: number) => void,
    reject: (opID: number, code: number) => void,
  ) => void
  BLDR_OPFS_WRITE_FILE_CLOSE?: (
    sessionID: number,
    opID: number,
    resolve: (opID: number, written: number) => void,
    reject: (opID: number, code: number) => void,
  ) => void
  BLDR_OPFS_WRITE_FILE_ABORT?: (sessionID: number) => boolean
}

afterEach(() => {
  const g = globalThis as TinyGoHelperGlobal &
    typeof globalThis & {
      BLDR_OPFS_BROADCAST_CHANNEL_NEW?: unknown
      BLDR_OPFS_BROADCAST_SEND?: unknown
      BLDR_OPFS_BROADCAST_CLOSE?: unknown
    }
  delete g.BLDR_OPFS_BROADCAST_CHANNEL_NEW
  delete g.BLDR_OPFS_BROADCAST_SEND
  delete g.BLDR_OPFS_BROADCAST_CLOSE
  delete g.BLDR_TINYGO_JS_CALL
  delete g.BLDR_TINYGO_JS_NEW
  delete g.BLDR_TINYGO_PROMISE_AWAIT
  delete g.BLDR_TINYGO_NEW_BYTES
  delete g.BLDR_TINYGO_TAKE_STORED_BYTES
  delete g.BLDR_OPFS_ACQUIRE_WEB_LOCK
  delete g.BLDR_TINYGO_PUSH_BYTES
  delete g.BLDR_TINYGO_POST_BYTES
  delete g.BLDR_OPFS_READ_FILE
  delete g.BLDR_OPFS_READ_AT
  delete g.BLDR_OPFS_LIST_DIRECTORY
  delete g.BLDR_OPFS_WRITE_AT
  delete g.BLDR_OPFS_WRITE_FILE
  delete g.BLDR_OPFS_WRITE_FILE_BEGIN
  delete g.BLDR_OPFS_WRITE_FILE_CHUNK
  delete g.BLDR_OPFS_WRITE_FILE_CLOSE
  delete g.BLDR_OPFS_WRITE_FILE_ABORT
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('patchTinyGoRuntimeImports', () => {
  it('adds TinyGo random data import backed by wasm memory', () => {
    const memory = new WebAssembly.Memory({ initial: 1 })
    const getRandomValues = vi.fn((view: Uint8Array) => {
      view.fill(7)
      return view
    })
    vi.stubGlobal('crypto', { getRandomValues })

    const go: TinyGoRuntime = {
      importObject: {
        gojs: {},
      },
      _inst: {
        exports: {
          memory,
        },
      },
    }

    patchTinyGoRuntimeImports(go)
    const getRandomData = go.importObject['gojs']?.['runtime.getRandomData']
    if (typeof getRandomData !== 'function') {
      throw new Error('runtime.getRandomData was not installed')
    }

    getRandomData(12, 4, 16)

    expect(getRandomValues).toHaveBeenCalledWith(expect.any(Uint8Array))
    expect(Array.from(new Uint8Array(memory.buffer, 12, 4))).toEqual([
      7, 7, 7, 7,
    ])
  })

  it('keeps an existing random data import', () => {
    const getRandomData = vi.fn()
    const go: TinyGoRuntime = {
      importObject: {
        gojs: {
          'runtime.getRandomData': getRandomData,
        },
      },
    }

    patchTinyGoRuntimeImports(go)

    expect(go.importObject['gojs']?.['runtime.getRandomData']).toBe(
      getRandomData,
    )
  })
})

describe('installTinyGoJSHelpers', () => {
  it('calls methods, constructs values, and attaches promise callbacks from JS', async () => {
    installTinyGoJSHelpers()

    const g = globalThis as TinyGoHelperGlobal

    const target = {
      prefix: 'opfs',
      join(value: string) {
        return `${this.prefix}:${value}`
      },
    }

    class Box {
      public constructor(public readonly value: string) {}
    }

    expect(g.BLDR_TINYGO_JS_CALL?.(target, 'join', 'manifest')).toBe(
      'opfs:manifest',
    )
    expect(g.BLDR_TINYGO_JS_NEW?.(Box, 'sync')).toEqual(new Box('sync'))

    const resolved = new Promise<string>((resolve) => {
      g.BLDR_TINYGO_PROMISE_AWAIT?.(Promise.resolve('ready'), resolve, () =>
        resolve('rejected'),
      )
    })
    await expect(resolved).resolves.toBe('ready')

    const notFound = new Promise<number>((resolve) => {
      g.BLDR_TINYGO_PROMISE_AWAIT?.(
        Promise.reject({ name: 'NotFoundError' }),
        () => resolve(0),
        (reason) => resolve(reason),
      )
    })
    await expect(notFound).resolves.toBe(1)

    const noModificationAllowed = new Promise<number>((resolve) => {
      g.BLDR_TINYGO_PROMISE_AWAIT?.(
        Promise.reject({ name: 'NoModificationAllowedError' }),
        () => resolve(0),
        (reason) => resolve(reason),
      )
    })
    await expect(noModificationAllowed).resolves.toBe(2)

    const stringNotFound = new Promise<number>((resolve) => {
      g.BLDR_TINYGO_PROMISE_AWAIT?.(
        Promise.reject(
          'playwright: A requested file or directory could not be found at the time an operation was processed. NotFoundError',
        ),
        () => resolve(0),
        (reason) => resolve(reason),
      )
    })
    await expect(stringNotFound).resolves.toBe(1)

    const unknown = new Promise<number>((resolve) => {
      g.BLDR_TINYGO_PROMISE_AWAIT?.(
        Promise.reject(new TypeError('bad call')),
        () => resolve(1),
        (reason) => resolve(reason),
      )
    })
    await expect(unknown).resolves.toBe(0)

    expect(g.BLDR_TINYGO_NEW_BYTES).toBeTypeOf('function')
    expect(g.BLDR_TINYGO_TAKE_STORED_BYTES).toBeTypeOf('function')
    expect(g.BLDR_OPFS_ACQUIRE_WEB_LOCK).toBeTypeOf('function')
    expect(g.BLDR_TINYGO_PUSH_BYTES).toBeTypeOf('function')
    expect(g.BLDR_TINYGO_POST_BYTES).toBeTypeOf('function')
    expect(g.BLDR_OPFS_READ_FILE).toBeTypeOf('function')
    expect(g.BLDR_OPFS_READ_AT).toBeTypeOf('function')
    expect(g.BLDR_OPFS_LIST_DIRECTORY).toBeTypeOf('function')
    expect(g.BLDR_OPFS_WRITE_AT).toBeTypeOf('function')
    expect(g.BLDR_OPFS_WRITE_FILE).toBeTypeOf('function')
    expect(g.BLDR_OPFS_WRITE_FILE_BEGIN).toBeTypeOf('function')
    expect(g.BLDR_OPFS_WRITE_FILE_CHUNK).toBeTypeOf('function')
    expect(g.BLDR_OPFS_WRITE_FILE_CLOSE).toBeTypeOf('function')
    expect(g.BLDR_OPFS_WRITE_FILE_ABORT).toBeTypeOf('function')
  })

  it('rejects OPFS Web Locks requests when the runtime lacks Web Locks', async () => {
    vi.stubGlobal('navigator', {})
    installTinyGoJSHelpers()
    const g = globalThis as TinyGoHelperGlobal

    const rejected = new Promise<number>((resolve) => {
      g.BLDR_OPFS_ACQUIRE_WEB_LOCK?.(
        'spacewave-opfs',
        'exclusive',
        false,
        () => resolve(-1),
        resolve,
      )
    })

    await expect(rejected).resolves.toBe(0)
  })
})

describe('installOPFSBroadcastHelpers', () => {
  it('posts encoded invalidations without a Go-side method call', () => {
    const channels: FakeBroadcastChannel[] = []

    class FakeBroadcastChannel {
      public readonly name: string
      public readonly messages: Uint8Array[] = []
      public closed = false

      constructor(name: string) {
        this.name = name
        channels.push(this)
      }

      public postMessage(msg: Uint8Array) {
        this.messages.push(msg)
      }

      public close() {
        this.closed = true
      }
    }

    vi.stubGlobal('BroadcastChannel', FakeBroadcastChannel)

    installOPFSBroadcastHelpers()

    const g = globalThis as typeof globalThis & {
      BLDR_OPFS_BROADCAST_CHANNEL_NEW?: (name: string) => BroadcastChannel
      BLDR_OPFS_BROADCAST_SEND?: (
        channel: BroadcastChannel,
        shardID: number,
        generationHi: number,
        generationLo: number,
      ) => void
      BLDR_OPFS_BROADCAST_CLOSE?: (channel: BroadcastChannel) => void
    }

    const channel = g.BLDR_OPFS_BROADCAST_CHANNEL_NEW?.('hydra-blockshard-gen')
    if (!channel) {
      throw new Error('broadcast channel helper was not installed')
    }

    g.BLDR_OPFS_BROADCAST_SEND?.(channel, 0x8003, 0xfedcba98, 0x76543210)
    g.BLDR_OPFS_BROADCAST_CLOSE?.(channel)

    expect(channels).toHaveLength(1)
    expect(channels[0].name).toBe('hydra-blockshard-gen')
    expect(channels[0].messages).toHaveLength(1)
    expect(Array.from(channels[0].messages[0])).toEqual([
      0x80, 0x03, 0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10,
    ])
    expect(channels[0].closed).toBe(true)
  })
})

describe('GoWasmProcess', () => {
  it('scopes Go released-callback console errors and exposes TinyGo byte helpers', async () => {
    const pushed: Uint8Array[] = []
    const run = vi.fn(async () => {
      const g = globalThis as TinyGoHelperGlobal

      const outbound = g.BLDR_TINYGO_NEW_BYTES?.(3)
      if (!outbound) {
        throw new Error('TinyGo byte allocator was not installed')
      }
      outbound.set([1, 2, 3])
      expect(
        g.BLDR_TINYGO_PUSH_BYTES?.(
          { push: (message: Uint8Array) => pushed.push(message) },
          outbound,
        ),
      ).toBe(true)
      outbound[0] = 99

      const postBytes = g.BLDR_TINYGO_NEW_BYTES?.(3)
      if (!postBytes) {
        throw new Error('TinyGo byte allocator was not installed')
      }
      postBytes.set([1, 2, 3])
      const posted: Uint8Array[] = []
      expect(
        g.BLDR_TINYGO_POST_BYTES?.(
          { postMessage: (message: Uint8Array) => posted.push(message) },
          postBytes,
        ),
      ).toBe(true)
      postBytes[0] = 88

      expect(
        g.BLDR_TINYGO_PUSH_BYTES?.(
          {
            push: () => {
              throw new Error('closed')
            },
          },
          new Uint8Array([1]),
        ),
      ).toBe(false)
      expect(
        g.BLDR_TINYGO_POST_BYTES?.(
          {
            postMessage: () => {
              throw new Error('closed')
            },
          },
          new Uint8Array([1]),
        ),
      ).toBe(false)

      expect(Array.from(pushed[0])).toEqual([1, 2, 3])
      expect(Array.from(posted[0])).toEqual([1, 2, 3])

      await expect(
        Promise.resolve().then(() =>
          g.BLDR_TINYGO_PUSH_BYTES?.(
            { push: (message: Uint8Array) => pushed.push(message) },
            1 as unknown as Uint8Array,
          ),
        ),
      ).resolves.toBe(false)
      await expect(
        Promise.resolve().then(() =>
          g.BLDR_TINYGO_POST_BYTES?.(
            { postMessage: (message: Uint8Array) => posted.push(message) },
            1 as unknown as Uint8Array,
          ),
        ),
      ).resolves.toBe(false)

      const read = await new Promise<{ id: number; len: number }>((resolve) => {
        g.BLDR_OPFS_READ_FILE?.(
          {
            getFileHandle: async () => ({
              getFile: async () => ({
                arrayBuffer: async () => new Uint8Array([4, 5, 6]).buffer,
              }),
            }),
          } as unknown as FileSystemDirectoryHandle,
          'manifest-a',
          101,
          (_opID, id, len) => resolve({ id, len }),
          () => resolve({ id: 0, len: 0 }),
        )
      })
      const stored = g.BLDR_TINYGO_TAKE_STORED_BYTES?.(read.id)

      expect(read.len).toBe(3)
      expect(stored && Array.from(stored)).toEqual([4, 5, 6])
      expect(g.BLDR_TINYGO_TAKE_STORED_BYTES?.(read.id)).toBeUndefined()

      const listed = await new Promise<{ id: number; len: number }>(
        (resolve) => {
          g.BLDR_OPFS_LIST_DIRECTORY?.(
            {
              entries: async function* () {
                yield ['manifest-a'] as unknown as [string, FileSystemHandle]
                yield ['wal-a'] as unknown as [string, FileSystemHandle]
              },
            } as unknown as FileSystemDirectoryHandle,
            102,
            (_opID, id, len) => resolve({ id, len }),
            () => resolve({ id: 0, len: 0 }),
          )
        },
      )
      const listBytes = g.BLDR_TINYGO_TAKE_STORED_BYTES?.(listed.id)

      expect(listBytes?.byteLength).toBe(listed.len)
      const decodeNameList = (bytes: Uint8Array): string[] => {
        const decoder = new TextDecoder()
        const readUint32 = (off: number) =>
          ((bytes[off] << 24) |
            (bytes[off + 1] << 16) |
            (bytes[off + 2] << 8) |
            bytes[off + 3]) >>>
          0
        const count = readUint32(0)
        const names: string[] = []
        let off = 4
        for (let i = 0; i < count; i++) {
          const len = readUint32(off)
          off += 4
          names.push(decoder.decode(bytes.subarray(off, off + len)))
          off += len
        }
        return names
      }
      if (!listBytes) {
        throw new Error('directory list bytes were not stored')
      }
      expect(decodeNameList(listBytes)).toEqual(['manifest-a', 'wal-a'])

      const readAtSlices: number[][] = []
      const readAtBytes = g.BLDR_TINYGO_NEW_BYTES?.(4)
      if (!readAtBytes) {
        throw new Error('TinyGo byte allocator was not installed')
      }
      const readAt = await new Promise<number>((resolve) => {
        g.BLDR_OPFS_READ_AT?.(
          {
            getFile: async () => ({
              size: 9,
              slice: (start: number, end: number) => {
                readAtSlices.push([start, end])
                return {
                  arrayBuffer: async () =>
                    new Uint8Array([11, 12, 13, 14]).buffer,
                }
              },
            }),
          } as unknown as FileSystemFileHandle,
          readAtBytes,
          3,
          103,
          (_opID, read) => resolve(read),
          () => resolve(-1),
        )
      })

      expect(readAt).toBe(4)
      expect(readAtSlices).toEqual([[3, 7]])
      expect(Array.from(readAtBytes)).toEqual([11, 12, 13, 14])

      const writtenChunks: Uint8Array[] = []
      const seeks: number[] = []
      const writeAtBytes = new Uint8Array([7, 8, 9, 10])
      const written = await new Promise<number>((resolve) => {
        g.BLDR_OPFS_WRITE_AT?.(
          {
            createWritable: async () => ({
              close: async () => undefined,
              seek: async (offset: number) => {
                seeks.push(offset)
              },
              write: async (data: BufferSource | Blob | string) => {
                if (!(data instanceof Uint8Array)) {
                  throw new Error('expected Uint8Array write')
                }
                writtenChunks.push(new Uint8Array(data))
              },
            }),
          } as unknown as FileSystemFileHandle,
          writeAtBytes,
          12,
          true,
          104,
          (_opID, written) => resolve(written),
          () => resolve(-1),
        )
      })

      expect(written).toBe(4)
      expect(seeks).toEqual([12])
      expect(writtenChunks.map((chunk) => Array.from(chunk))).toEqual([
        [7, 8, 9, 10],
      ])

      const wholeFileWrites: Uint8Array[] = []
      delete (
        globalThis as typeof globalThis & { BLDR_OPFS_WRITE_AT?: unknown }
      ).BLDR_OPFS_WRITE_AT
      const wholeFileBytes = new Uint8Array([21, 22, 23])
      const wholeFileWritten = await new Promise<number>((resolve) => {
        g.BLDR_OPFS_WRITE_FILE?.(
          {
            getFileHandle: async (
              name: string,
              opts?: { create?: boolean },
            ) => {
              if (name !== 'wal-a' || opts?.create !== true) {
                throw new Error('unexpected file handle request')
              }
              return {
                createWritable: async () => ({
                  close: async () => undefined,
                  write: async (data: BufferSource | Blob | string) => {
                    if (!(data instanceof Uint8Array)) {
                      throw new Error('expected Uint8Array write')
                    }
                    wholeFileWrites.push(new Uint8Array(data))
                  },
                }),
              }
            },
          } as unknown as FileSystemDirectoryHandle,
          'wal-a',
          wholeFileBytes,
          105,
          (_opID, written) => resolve(written),
          () => resolve(-1),
        )
      })

      expect(wholeFileWritten).toBe(3)
      expect(wholeFileWrites.map((chunk) => Array.from(chunk))).toEqual([
        [21, 22, 23],
      ])

      const sessionWrites: Uint8Array[] = []
      const sessionID = await new Promise<number>((resolve) => {
        g.BLDR_OPFS_WRITE_FILE_BEGIN?.(
          {
            getFileHandle: async (
              name: string,
              opts?: { create?: boolean },
            ) => {
              if (name !== 'seg-a' || opts?.create !== true) {
                throw new Error('unexpected session file handle request')
              }
              return {
                createWritable: async () => ({
                  close: async () => undefined,
                  write: async (data: BufferSource | Blob | string) => {
                    if (!(data instanceof Uint8Array)) {
                      throw new Error('expected Uint8Array write')
                    }
                    sessionWrites.push(new Uint8Array(data))
                  },
                }),
              }
            },
          } as unknown as FileSystemDirectoryHandle,
          'seg-a',
          106,
          (_opID, id) => resolve(id),
          () => resolve(0),
        )
      })
      expect(sessionID).toBeGreaterThan(0)

      const firstChunk = new Uint8Array([31, 32])
      const firstChunkWritten = await new Promise<number>((resolve) => {
        g.BLDR_OPFS_WRITE_FILE_CHUNK?.(
          sessionID,
          firstChunk,
          107,
          (_opID, written) => resolve(written),
          () => resolve(-1),
        )
      })
      firstChunk[0] = 99
      const secondChunkWritten = await new Promise<number>((resolve) => {
        g.BLDR_OPFS_WRITE_FILE_CHUNK?.(
          sessionID,
          new Uint8Array([33, 34, 35]),
          108,
          (_opID, written) => resolve(written),
          () => resolve(-1),
        )
      })
      const sessionTotal = await new Promise<number>((resolve) => {
        g.BLDR_OPFS_WRITE_FILE_CLOSE?.(
          sessionID,
          109,
          (_opID, written) => resolve(written),
          () => resolve(-1),
        )
      })

      expect(firstChunkWritten).toBe(2)
      expect(secondChunkWritten).toBe(3)
      expect(sessionTotal).toBe(5)
      expect(sessionWrites.map((chunk) => Array.from(chunk))).toEqual([
        [31, 32],
        [33, 34, 35],
      ])
      expect(g.BLDR_OPFS_WRITE_FILE_ABORT?.(sessionID)).toBe(false)

      await new Promise<void>((resolve) => {
        g.BLDR_TINYGO_PROMISE_AWAIT?.(
          Promise.resolve(undefined),
          () => {
            console.error('call to released function')
            resolve()
          },
          () => resolve(),
        )
      })
      console.error('other failure')
    })
    class FakeGo {
      public readonly importObject = {}
      public env: Record<string, string> = {}
      public argv: string[] = []
      public run = run
    }

    vi.stubGlobal('Go', FakeGo)
    vi.spyOn(WebAssembly, 'instantiate').mockResolvedValue({
      exports: {},
    })
    const consoleError = vi.spyOn(console, 'error')

    const process = new GoWasmProcess(
      {},
      {
        retry: false,
      },
    )
    await process.start()

    expect(consoleError).toHaveBeenCalledTimes(1)
    expect(consoleError).toHaveBeenCalledWith('other failure')
  })
})
