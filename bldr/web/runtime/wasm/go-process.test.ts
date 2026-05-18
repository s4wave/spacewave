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
  BLDR_TINYGO_COPY_STORED_BYTES?: (
    id: number,
    ptr: number,
    len: number,
  ) => number
  BLDR_TINYGO_PUSH_BYTES?: (
    sink: { push: (message: Uint8Array) => void },
    ptr: number,
    len: number,
  ) => void
  BLDR_TINYGO_POST_BYTES?: (
    port: { postMessage: (message: Uint8Array) => void },
    ptr: number,
    len: number,
  ) => void
  BLDR_OPFS_READ_FILE?: (
    dir: FileSystemDirectoryHandle,
    name: string,
    opID: number,
    resolve: (opID: number, id: number, len: number) => void,
    reject: (opID: number, code: number) => void,
  ) => void
  BLDR_OPFS_READ_AT?: (
    handle: FileSystemFileHandle,
    ptr: number,
    len: number,
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
    ptr: number,
    len: number,
    off: number,
    keepExisting: boolean,
    opID: number,
    resolve: (opID: number, written: number) => void,
    reject: (opID: number, code: number) => void,
  ) => void
  BLDR_OPFS_WRITE_FILE?: (
    dir: FileSystemDirectoryHandle,
    name: string,
    ptr: number,
    len: number,
    opID: number,
    resolve: (opID: number, written: number) => void,
    reject: (opID: number, code: number) => void,
  ) => void
}

afterEach(() => {
  delete (
    globalThis as typeof globalThis & {
      BLDR_OPFS_BROADCAST_CHANNEL_NEW?: unknown
      BLDR_OPFS_BROADCAST_SEND?: unknown
      BLDR_OPFS_BROADCAST_CLOSE?: unknown
      BLDR_TINYGO_JS_CALL?: unknown
      BLDR_TINYGO_JS_NEW?: unknown
      BLDR_TINYGO_PROMISE_AWAIT?: unknown
      BLDR_TINYGO_COPY_STORED_BYTES?: unknown
      BLDR_TINYGO_PUSH_BYTES?: unknown
      BLDR_OPFS_READ_FILE?: unknown
    }
  ).BLDR_OPFS_BROADCAST_CHANNEL_NEW
  delete (
    globalThis as typeof globalThis & {
      BLDR_OPFS_BROADCAST_CHANNEL_NEW?: unknown
      BLDR_OPFS_BROADCAST_SEND?: unknown
      BLDR_OPFS_BROADCAST_CLOSE?: unknown
      BLDR_TINYGO_JS_CALL?: unknown
      BLDR_TINYGO_JS_NEW?: unknown
      BLDR_TINYGO_PROMISE_AWAIT?: unknown
      BLDR_TINYGO_COPY_STORED_BYTES?: unknown
      BLDR_TINYGO_PUSH_BYTES?: unknown
      BLDR_OPFS_READ_FILE?: unknown
    }
  ).BLDR_OPFS_BROADCAST_SEND
  delete (
    globalThis as typeof globalThis & {
      BLDR_OPFS_BROADCAST_CHANNEL_NEW?: unknown
      BLDR_OPFS_BROADCAST_SEND?: unknown
      BLDR_OPFS_BROADCAST_CLOSE?: unknown
      BLDR_TINYGO_JS_CALL?: unknown
      BLDR_TINYGO_JS_NEW?: unknown
      BLDR_TINYGO_PROMISE_AWAIT?: unknown
      BLDR_TINYGO_COPY_STORED_BYTES?: unknown
      BLDR_TINYGO_PUSH_BYTES?: unknown
      BLDR_OPFS_READ_FILE?: unknown
    }
  ).BLDR_OPFS_BROADCAST_CLOSE
  delete (
    globalThis as typeof globalThis & {
      BLDR_TINYGO_JS_CALL?: unknown
      BLDR_TINYGO_JS_NEW?: unknown
      BLDR_TINYGO_PROMISE_AWAIT?: unknown
      BLDR_TINYGO_COPY_STORED_BYTES?: unknown
      BLDR_TINYGO_PUSH_BYTES?: unknown
      BLDR_OPFS_READ_FILE?: unknown
    }
  ).BLDR_TINYGO_JS_CALL
  delete (
    globalThis as typeof globalThis & {
      BLDR_TINYGO_JS_CALL?: unknown
      BLDR_TINYGO_JS_NEW?: unknown
      BLDR_TINYGO_PROMISE_AWAIT?: unknown
      BLDR_TINYGO_COPY_STORED_BYTES?: unknown
      BLDR_TINYGO_PUSH_BYTES?: unknown
      BLDR_OPFS_READ_FILE?: unknown
    }
  ).BLDR_TINYGO_JS_NEW
  delete (
    globalThis as typeof globalThis & {
      BLDR_TINYGO_JS_CALL?: unknown
      BLDR_TINYGO_JS_NEW?: unknown
      BLDR_TINYGO_PROMISE_AWAIT?: unknown
      BLDR_TINYGO_COPY_STORED_BYTES?: unknown
      BLDR_TINYGO_PUSH_BYTES?: unknown
      BLDR_OPFS_READ_FILE?: unknown
    }
  ).BLDR_TINYGO_PROMISE_AWAIT
  delete (
    globalThis as typeof globalThis & {
      BLDR_TINYGO_COPY_STORED_BYTES?: unknown
      BLDR_TINYGO_PUSH_BYTES?: unknown
      BLDR_OPFS_READ_FILE?: unknown
    }
  ).BLDR_TINYGO_COPY_STORED_BYTES
  delete (
    globalThis as typeof globalThis & {
      BLDR_TINYGO_COPY_STORED_BYTES?: unknown
      BLDR_TINYGO_PUSH_BYTES?: unknown
      BLDR_OPFS_READ_FILE?: unknown
    }
  ).BLDR_TINYGO_PUSH_BYTES
  delete (globalThis as TinyGoHelperGlobal).BLDR_TINYGO_POST_BYTES
  delete (
    globalThis as typeof globalThis & {
      BLDR_TINYGO_COPY_STORED_BYTES?: unknown
      BLDR_TINYGO_PUSH_BYTES?: unknown
      BLDR_OPFS_READ_FILE?: unknown
    }
  ).BLDR_OPFS_READ_FILE
  delete (
    globalThis as typeof globalThis & {
      BLDR_OPFS_READ_AT?: unknown
    }
  ).BLDR_OPFS_READ_AT
  delete (globalThis as TinyGoHelperGlobal).BLDR_OPFS_LIST_DIRECTORY
  delete (
    globalThis as typeof globalThis & {
      BLDR_OPFS_WRITE_AT?: unknown
      BLDR_OPFS_WRITE_FILE?: unknown
    }
  ).BLDR_OPFS_WRITE_AT
  delete (
    globalThis as typeof globalThis & {
      BLDR_OPFS_WRITE_AT?: unknown
      BLDR_OPFS_WRITE_FILE?: unknown
    }
  ).BLDR_OPFS_WRITE_FILE
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

    expect(g.BLDR_TINYGO_COPY_STORED_BYTES).toBeTypeOf('function')
    expect(g.BLDR_TINYGO_PUSH_BYTES).toBeTypeOf('function')
    expect(g.BLDR_TINYGO_POST_BYTES).toBeTypeOf('function')
    expect(g.BLDR_OPFS_READ_FILE).toBeTypeOf('function')
    expect(g.BLDR_OPFS_READ_AT).toBeTypeOf('function')
    expect(g.BLDR_OPFS_LIST_DIRECTORY).toBeTypeOf('function')
    expect(g.BLDR_OPFS_WRITE_AT).toBeTypeOf('function')
    expect(g.BLDR_OPFS_WRITE_FILE).toBeTypeOf('function')
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
  it('suppresses Go released-callback console errors and exposes TinyGo memory helpers', async () => {
    const memory = new WebAssembly.Memory({ initial: 1 })
    const pushed: Uint8Array[] = []
    const run = vi.fn(async () => {
      const g = globalThis as TinyGoHelperGlobal

      new Uint8Array(memory.buffer, 8, 3).set([1, 2, 3])
      g.BLDR_TINYGO_PUSH_BYTES?.(
        { push: (message: Uint8Array) => pushed.push(message) },
        8,
        3,
      )
      const posted: Uint8Array[] = []
      g.BLDR_TINYGO_POST_BYTES?.(
        { postMessage: (message: Uint8Array) => posted.push(message) },
        8,
        3,
      )

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
      const copied = g.BLDR_TINYGO_COPY_STORED_BYTES?.(read.id, 16, read.len)

      expect(Array.from(pushed[0])).toEqual([1, 2, 3])
      expect(Array.from(posted[0])).toEqual([1, 2, 3])
      expect(copied).toBe(3)
      expect(Array.from(new Uint8Array(memory.buffer, 16, 3))).toEqual([
        4, 5, 6,
      ])

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
      const copiedList = g.BLDR_TINYGO_COPY_STORED_BYTES?.(
        listed.id,
        48,
        listed.len,
      )

      expect(copiedList).toBe(listed.len)
      const listBytes = new Uint8Array(memory.buffer, 48, listed.len)
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
      expect(decodeNameList(listBytes)).toEqual(['manifest-a', 'wal-a'])

      const readAtSlices: number[][] = []
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
          32,
          4,
          3,
          103,
          (_opID, read) => resolve(read),
          () => resolve(-1),
        )
      })

      expect(readAt).toBe(4)
      expect(readAtSlices).toEqual([[3, 7]])
      expect(Array.from(new Uint8Array(memory.buffer, 32, 4))).toEqual([
        11, 12, 13, 14,
      ])

      const writtenChunks: Uint8Array[] = []
      const seeks: number[] = []
      new Uint8Array(memory.buffer, 24, 4).set([7, 8, 9, 10])
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
          24,
          4,
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
      new Uint8Array(memory.buffer, 40, 3).set([21, 22, 23])
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
          40,
          3,
          105,
          (_opID, written) => resolve(written),
          () => resolve(-1),
        )
      })

      expect(wholeFileWritten).toBe(3)
      expect(wholeFileWrites.map((chunk) => Array.from(chunk))).toEqual([
        [21, 22, 23],
      ])

      console.error('call to released function')
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
      exports: { memory },
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
