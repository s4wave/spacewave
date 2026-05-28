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
  ) => void
  BLDR_OPFS_READ_AT?: (
    handle: FileSystemFileHandle,
    dst: Uint8Array,
    off: number,
    opID: number,
  ) => void
  BLDR_OPFS_LIST_DIRECTORY?: (
    dir: FileSystemDirectoryHandle,
    opID: number,
  ) => void
  BLDR_OPFS_WRITE_AT?: (
    handle: FileSystemFileHandle,
    data: Uint8Array,
    off: number,
    keepExisting: boolean,
    opID: number,
  ) => void
  BLDR_OPFS_WRITE_FILE?: (
    dir: FileSystemDirectoryHandle,
    name: string,
    data: Uint8Array,
    opID: number,
  ) => void
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
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

function newTinyGoRuntime(
  onOPFSResolve?: (
    opID: number,
    count: number,
    value0: number,
    value1: number,
  ) => void,
  onOPFSReject?: (opID: number, code: number) => void,
): TinyGoRuntime {
  return {
    importObject: {
      gojs: {},
    },
    _inst: {
      exports: {
        memory: new WebAssembly.Memory({ initial: 1 }),
        BLDR_OPFS_HELPER_RESOLVE: (
          opID: number,
          count: number,
          value0: number,
          value1: number,
        ) => {
          onOPFSResolve?.(opID, count, value0, value1)
        },
        BLDR_OPFS_HELPER_REJECT: (opID: number, code: number) => {
          onOPFSReject?.(opID, code)
        },
      },
    },
    _resume: vi.fn(),
  }
}

function tinyGoObjectRef(id: number): bigint {
  return tinyGoRef(id, 1n)
}

function tinyGoFunctionRef(id: number): bigint {
  return tinyGoRef(id, 4n)
}

function tinyGoRef(id: number, typeFlag: bigint): bigint {
  return BigInt(id) | ((0x7ff80000n | typeFlag) << 32n)
}

function tinyGoRefID(ref: bigint): number {
  return Number(ref & 0xffffffffn)
}

function installTinyGoValues(go: TinyGoRuntime, values: unknown[]): void {
  go._values = [NaN, 0, null, true, false, globalThis, go, ...values]
  go._goRefCounts = go._values.map(() => 0)
  go._ids = new Map<unknown, bigint>()
  go._idPool = []
  for (const [idx, value] of go._values.entries()) {
    if (
      typeof value === 'object' ||
      typeof value === 'function' ||
      typeof value === 'string'
    ) {
      go._ids.set(value, BigInt(idx))
    }
  }
}

function writeTinyGoString(
  go: TinyGoRuntime,
  value: string,
  ptr: number,
): [number, number] {
  const memory = go._inst?.exports.memory
  if (!(memory instanceof WebAssembly.Memory)) {
    throw new Error('TinyGo runtime memory is not initialized')
  }
  const bytes = new TextEncoder().encode(value)
  new Uint8Array(memory.buffer, ptr, bytes.byteLength).set(bytes)
  return [ptr, bytes.byteLength]
}

function callTinyGoImport(
  go: TinyGoRuntime,
  name: string,
  ...args: unknown[]
): unknown {
  const gojs = go.importObject.gojs
  if (!gojs || typeof gojs !== 'object') {
    throw new Error('TinyGo gojs import table is not initialized')
  }
  const fn = Reflect.get(gojs, name)
  if (typeof fn !== 'function') {
    throw new Error(`${name} import is not callable`)
  }
  return Reflect.apply(fn, undefined, args)
}

function callTinyGoBigIntImport(
  go: TinyGoRuntime,
  name: string,
  ...args: unknown[]
): bigint {
  const result = callTinyGoImport(go, name, ...args)
  if (typeof result !== 'bigint') {
    throw new Error(`${name} import did not return a bigint`)
  }
  return result
}

function readTinyGoStoredBytes(
  go: TinyGoRuntime,
  bytesID: number,
  len: number,
  ptr: number,
): Uint8Array {
  const take = callTinyGoImport(
    go,
    'bldr.tinygo.takeStoredBytes',
    bytesID,
    ptr,
    len,
  )
  expect(take).toBe(1)
  const memory = go._inst?.exports.memory
  if (!(memory instanceof WebAssembly.Memory)) {
    throw new Error('TinyGo runtime memory is not initialized')
  }
  return new Uint8Array(memory.buffer, ptr, len).slice()
}

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
    expect(go.importObject['gojs']?.['bldr.opfs.acquireWebLock']).toBeTypeOf(
      'function',
    )
  })

  it('adds TinyGo WebLock imports backed by wasm-export callbacks', async () => {
    const memory = new WebAssembly.Memory({ initial: 1 })
    const name = new TextEncoder().encode('spacewave-lock')
    new Uint8Array(memory.buffer, 32, name.byteLength).set(name)
    const resolved: Array<{
      opID: number
      releaseID: number
      acquired: number
    }> = []
    const rejected: Array<{ opID: number; code: number }> = []
    const callbackOrder: string[] = []
    let exportMicrotask!: () => void
    let schedulerCallback!: () => void
    const exportMicrotaskDone = new Promise<void>((resolve) => {
      exportMicrotask = resolve
    })
    const schedulerDone = new Promise<void>((resolve) => {
      schedulerCallback = resolve
    })
    const request = vi.fn(
      (
        lockName: string,
        _options: LockOptions,
        callback: (lock: Lock | null) => unknown,
      ) => {
        callback({ name: lockName, mode: 'exclusive' })
        return Promise.resolve()
      },
    )
    vi.stubGlobal('navigator', { locks: { request } })

    const go: TinyGoRuntime = {
      importObject: {
        gojs: {},
      },
      _inst: {
        exports: {
          memory,
          BLDR_OPFS_WEB_LOCK_RESOLVE: (
            opID: number,
            releaseID: number,
            acquired: number,
          ) => {
            callbackOrder.push('export')
            queueMicrotask(() => {
              callbackOrder.push('microtask-after-export')
              exportMicrotask()
            })
            resolved.push({ opID, releaseID, acquired })
          },
          BLDR_OPFS_WEB_LOCK_REJECT: (opID: number, code: number) => {
            rejected.push({ opID, code })
          },
          go_scheduler: () => {
            callbackOrder.push('scheduler')
            schedulerCallback()
          },
        },
      },
      _resume: () => {
        callbackOrder.push('resume')
      },
    }

    patchTinyGoRuntimeImports(go)
    const acquire = go.importObject['gojs']?.['bldr.opfs.acquireWebLock'] as
      | ((
          opID: number,
          namePtr: number,
          nameLen: number,
          exclusive: number,
          ifAvailable: number,
        ) => void)
      | undefined
    const release = go.importObject['gojs']?.['bldr.opfs.releaseWebLock'] as
      | ((releaseID: number) => number)
      | undefined
    if (!acquire || !release) {
      throw new Error('WebLock imports were not installed')
    }

    acquire(17, 32, name.byteLength, 1, 0)
    await exportMicrotaskDone
    expect(callbackOrder).toEqual(['export', 'microtask-after-export'])
    await schedulerDone

    expect(request).toHaveBeenCalledWith(
      'spacewave-lock',
      expect.objectContaining({ mode: 'exclusive' }),
      expect.any(Function),
    )
    expect(rejected).toEqual([])
    expect(resolved).toEqual([{ opID: 17, releaseID: 1, acquired: 1 }])
    expect(callbackOrder).toEqual([
      'export',
      'microtask-after-export',
      'scheduler',
    ])
    expect(release(1)).toBe(1)
    expect(release(1)).toBe(0)
  })

  it('adds TinyGo OPFS imports backed by wasm memory and js.Value refs', async () => {
    const resolved = new Map<number, (values: number[]) => void>()
    const rejected = new Map<number, (code: number) => void>()
    const waitOPFS = <T>(
      opID: number,
      invoke: () => void,
      map: (values: number[]) => T,
    ) =>
      new Promise<T>((resolve) => {
        resolved.set(opID, (values) => resolve(map(values)))
        rejected.set(opID, (code) => resolve(map([-code])))
        invoke()
      })
    const go = newTinyGoRuntime(
      (opID, count, value0, value1) => {
        resolved.get(opID)?.([value0, value1].slice(0, count))
        resolved.delete(opID)
        rejected.delete(opID)
      },
      (opID, code) => {
        rejected.get(opID)?.(code)
        resolved.delete(opID)
        rejected.delete(opID)
      },
    )
    const memory = go._inst?.exports.memory as WebAssembly.Memory
    const mem = new Uint8Array(memory.buffer)
    const writeString = (ptr: number, value: string) => {
      const bytes = new TextEncoder().encode(value)
      mem.set(bytes, ptr)
      return bytes.byteLength
    }
    const writes: number[][] = []
    const abortState: {
      count: number
      resolved: boolean
      release?: () => void
    } = {
      count: 0,
      resolved: false,
    }
    const abortWait = new Promise<void>((resolve) => {
      abortState.release = resolve
    })
    const writable = {
      abort: async () => {
        abortState.count++
        await abortWait
      },
      close: async () => undefined,
      write: async (data: BufferSource | Blob | string) => {
        if (!(data instanceof Uint8Array)) {
          throw new Error('expected Uint8Array write')
        }
        writes.push(Array.from(data))
      },
    }
    let failedAborts = 0
    let failedCloses = 0
    const failingWritable = {
      abort: async () => {
        failedAborts++
      },
      close: async () => {
        failedCloses++
      },
      write: async () => {
        const err = new Error('write failure')
        err.name = 'NoModificationAllowedError'
        throw err
      },
    }
    const cleanupFailWritable = {
      abort: async () => {
        const err = new Error('cleanup failure')
        err.name = 'NoModificationAllowedError'
        throw err
      },
      close: async () => undefined,
      write: async () => {
        throw new Error('primary write failure')
      },
    }
    const abortFailWritable = {
      abort: async () => {
        const err = new Error('abort failure')
        err.name = 'NoModificationAllowedError'
        throw err
      },
      close: async () => undefined,
      write: async () => undefined,
    }
    const dir = {
      getFileHandle: async (name: string, opts?: { create?: boolean }) => {
        if (name === 'read.bin') {
          return {
            getFile: async () => ({
              arrayBuffer: async () => new Uint8Array([4, 5, 6]).buffer,
            }),
          }
        }
        if (name === 'single.bin' && opts?.create === true) {
          return {
            createWritable: async () => writable,
          }
        }
        if (name === 'stream.bin' && opts?.create === true) {
          return {
            createWritable: async () => writable,
          }
        }
        if (name === 'abort.bin' && opts?.create === true) {
          return {
            createWritable: async () => writable,
          }
        }
        if (name === 'abort-fail.bin' && opts?.create === true) {
          return {
            createWritable: async () => abortFailWritable,
          }
        }
        if (name === 'fail.bin' && opts?.create === true) {
          return {
            createWritable: async () => failingWritable,
          }
        }
        if (name === 'cleanup-fail.bin' && opts?.create === true) {
          return {
            createWritable: async () => cleanupFailWritable,
          }
        }
        throw new Error('unexpected file handle request')
      },
    }
    go._values = [NaN, 0, null, true, false, globalThis, go, dir]

    patchTinyGoRuntimeImports(go)
    const gojs = go.importObject.gojs as Record<string, unknown>
    const readFile = gojs['bldr.opfs.readFileRef'] as
      | ((
          opID: number,
          dirRef: bigint,
          namePtr: number,
          nameLen: number,
        ) => void)
      | undefined
    const takeStoredBytes = gojs['bldr.opfs.takeStoredBytes'] as
      | ((bytesID: number, ptr: number, len: number) => number)
      | undefined
    const writeFile = gojs['bldr.opfs.writeFileRef'] as
      | ((
          opID: number,
          dirRef: bigint,
          namePtr: number,
          nameLen: number,
          dataPtr: number,
          dataLen: number,
        ) => void)
      | undefined
    const openWriteStream = gojs['bldr.opfs.openWriteStreamRef'] as
      | ((
          opID: number,
          dirRef: bigint,
          namePtr: number,
          nameLen: number,
        ) => void)
      | undefined
    const writeStream = gojs['bldr.opfs.writeStreamRef'] as
      | ((
          opID: number,
          streamID: number,
          dataPtr: number,
          dataLen: number,
        ) => void)
      | undefined
    const closeWriteStream = gojs['bldr.opfs.closeWriteStreamRef'] as
      | ((opID: number, streamID: number) => void)
      | undefined
    const abortWriteStream = gojs['bldr.opfs.abortWriteStreamRef'] as
      | ((opID: number, streamID: number) => void)
      | undefined
    if (
      !readFile ||
      !takeStoredBytes ||
      !writeFile ||
      !openWriteStream ||
      !writeStream ||
      !closeWriteStream ||
      !abortWriteStream
    ) {
      throw new Error('OPFS import bridge was not installed')
    }

    const readNameLen = writeString(32, 'read.bin')
    const read = await waitOPFS(
      301,
      () => readFile(301, tinyGoObjectRef(7), 32, readNameLen),
      ([id = 0, len = 0]) => ({ id, len }),
    )
    expect(read.len).toBe(3)
    expect(takeStoredBytes(read.id, 80, read.len)).toBe(1)
    expect(Array.from(mem.subarray(80, 83))).toEqual([4, 5, 6])
    expect(takeStoredBytes(read.id, 80, read.len)).toBe(0)

    const singleNameLen = writeString(96, 'single.bin')
    mem.set([7, 8, 9], 128)
    const singleWritten = await waitOPFS(
      302,
      () => {
        writeFile(302, tinyGoObjectRef(7), 96, singleNameLen, 128, 3)
        mem[128] = 99
      },
      ([n = 0]) => n,
    )
    expect(singleWritten).toBe(3)

    const failNameLen = writeString(96, 'fail.bin')
    mem.set([11, 12, 13], 128)
    const failureCode = await waitOPFS(
      303,
      () => {
        writeFile(303, tinyGoObjectRef(7), 96, failNameLen, 128, 3)
        mem[128] = 99
      },
      ([code = 0]) => -code,
    )

    expect(failureCode).toBe(2)
    expect(failedAborts).toBe(1)
    expect(failedCloses).toBe(0)

    const cleanupFailNameLen = writeString(96, 'cleanup-fail.bin')
    mem.set([14, 15, 16], 128)
    const cleanupFailureCode = await waitOPFS(
      304,
      () => {
        writeFile(304, tinyGoObjectRef(7), 96, cleanupFailNameLen, 128, 3)
      },
      ([code = 0]) => -code,
    )
    expect(cleanupFailureCode).toBe(2)

    const streamNameLen = writeString(96, 'stream.bin')
    const streamID = await waitOPFS(
      305,
      () => openWriteStream(305, tinyGoObjectRef(7), 96, streamNameLen),
      ([id = 0]) => id,
    )
    mem.set([1, 2], 160)
    const firstStreamWrite = await waitOPFS(
      306,
      () => {
        writeStream(306, streamID, 160, 2)
        mem[160] = 99
      },
      ([n = 0]) => n,
    )
    expect(firstStreamWrite).toBe(2)
    mem.set([3, 4, 5], 168)
    const secondStreamWrite = await waitOPFS(
      307,
      () => writeStream(307, streamID, 168, 3),
      ([n = 0]) => n,
    )
    expect(secondStreamWrite).toBe(3)
    const closed = await waitOPFS(
      308,
      () => closeWriteStream(308, streamID),
      ([n = 0]) => n,
    )
    expect(closed).toBe(1)

    const abortNameLen = writeString(96, 'abort.bin')
    const abortStreamID = await waitOPFS(
      309,
      () => openWriteStream(309, tinyGoObjectRef(7), 96, abortNameLen),
      ([id = 0]) => id,
    )
    const abortResult = waitOPFS(
      310,
      () => abortWriteStream(310, abortStreamID),
      ([n = 0]) => n,
    ).then((n) => {
      abortState.resolved = true
      return n
    })
    await Promise.resolve()
    expect(abortState.count).toBe(1)
    expect(abortState.resolved).toBe(false)
    abortState.release?.()
    expect(await abortResult).toBe(1)
    expect(abortState.resolved).toBe(true)

    const abortFailNameLen = writeString(96, 'abort-fail.bin')
    const abortFailStreamID = await waitOPFS(
      311,
      () => openWriteStream(311, tinyGoObjectRef(7), 96, abortFailNameLen),
      ([id = 0]) => id,
    )
    const abortFailureCode = await waitOPFS(
      312,
      () => abortWriteStream(312, abortFailStreamID),
      ([code = 0]) => -code,
    )
    expect(abortFailureCode).toBe(2)
    const abortFailureRetryCode = await waitOPFS(
      313,
      () => abortWriteStream(313, abortFailStreamID),
      ([code = 0]) => -code,
    )
    expect(abortFailureRetryCode).toBe(2)

    expect(writes).toEqual([[7, 8, 9], [1, 2], [3, 4, 5]])
  })
})

describe('installTinyGoJSHelpers', () => {
  it('replaces OPFS globals that close over a TinyGo runtime', () => {
    const g = globalThis as TinyGoHelperGlobal

    installTinyGoJSHelpers(newTinyGoRuntime())
    const firstWriteFile = g.BLDR_OPFS_WRITE_FILE
    const firstWriteAt = g.BLDR_OPFS_WRITE_AT

    installTinyGoJSHelpers(newTinyGoRuntime())

    expect(g.BLDR_OPFS_WRITE_FILE).not.toBe(firstWriteFile)
    expect(g.BLDR_OPFS_WRITE_AT).not.toBe(firstWriteAt)
  })

  it('runs deferred TinyGo callbacks with a task boundary between callbacks', async () => {
    installTinyGoJSHelpers(newTinyGoRuntime())

    const g = globalThis as TinyGoHelperGlobal
    const calls: string[] = []
    let firstMicrotask!: () => void
    let secondCallback!: () => void
    const firstDone = new Promise<void>((resolve) => {
      firstMicrotask = resolve
    })
    const secondDone = new Promise<void>((resolve) => {
      secondCallback = resolve
    })

    g.BLDR_TINYGO_PROMISE_AWAIT?.(
      Promise.resolve('first'),
      () => {
        calls.push('first')
        queueMicrotask(() => {
          calls.push('microtask-after-first')
          firstMicrotask()
        })
      },
      () => undefined,
    )
    g.BLDR_TINYGO_PROMISE_AWAIT?.(
      Promise.resolve('second'),
      () => {
        calls.push('second')
        secondCallback()
      },
      () => undefined,
    )

    await firstDone
    expect(calls).toEqual(['first', 'microtask-after-first'])

    await secondDone
    expect(calls).toEqual(['first', 'microtask-after-first', 'second'])
  })

  it('calls methods, constructs values, and attaches promise callbacks from JS', async () => {
    installTinyGoJSHelpers(newTinyGoRuntime())

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
  })

  it('installs direct TinyGo gojs imports for Bldr helper calls', async () => {
    const go = newTinyGoRuntime()
    const opts = { cache: 'no-store' }
    const target = {
      prefix: 'opfs',
      zero() {
        return this.prefix
      },
      one(value: unknown) {
        return value
      },
      fetch(url: string, init: unknown) {
        return { url, init, prefix: this.prefix }
      },
    }
    class Box {
      public constructor(public readonly value?: number) {}
    }
    const promise = Promise.resolve('ready')
    const promiseResult = new Promise<unknown>((resolve) => {
      installTinyGoValues(go, [
        target,
        Box,
        opts,
        promise,
        (value: unknown) => resolve(value),
        (code: number) => resolve(code),
      ])
    })

    installTinyGoJSHelpers(go)

    const [zeroPtr, zeroLen] = writeTinyGoString(go, 'zero', 16)
    const zeroRef = callTinyGoBigIntImport(
      go,
      'bldr.tinygo.jsCall0',
      tinyGoObjectRef(7),
      zeroPtr,
      zeroLen,
    )
    expect(go._values?.[tinyGoRefID(zeroRef)]).toBe('opfs')

    const [onePtr, oneLen] = writeTinyGoString(go, 'one', 32)
    const oneRef = callTinyGoBigIntImport(
      go,
      'bldr.tinygo.jsCall1Value',
      tinyGoObjectRef(7),
      onePtr,
      oneLen,
      tinyGoObjectRef(9),
    )
    expect(oneRef).toBe(tinyGoObjectRef(9))

    const [fetchPtr, fetchLen] = writeTinyGoString(go, 'fetch', 48)
    const [urlPtr, urlLen] = writeTinyGoString(go, '/root.packedmsg', 64)
    const fetchRef = callTinyGoBigIntImport(
      go,
      'bldr.tinygo.jsCall2StringValue',
      tinyGoObjectRef(7),
      fetchPtr,
      fetchLen,
      urlPtr,
      urlLen,
      tinyGoObjectRef(9),
    )
    expect(go._values?.[tinyGoRefID(fetchRef)]).toEqual({
      url: '/root.packedmsg',
      init: opts,
      prefix: 'opfs',
    })

    const box0Ref = callTinyGoBigIntImport(
      go,
      'bldr.tinygo.jsNew0',
      tinyGoFunctionRef(8),
    )
    expect(go._values?.[tinyGoRefID(box0Ref)]).toEqual(new Box())

    const box1Ref = callTinyGoBigIntImport(
      go,
      'bldr.tinygo.jsNew1Int',
      tinyGoFunctionRef(8),
      13,
    )
    expect(go._values?.[tinyGoRefID(box1Ref)]).toEqual(new Box(13))

    callTinyGoImport(
      go,
      'bldr.tinygo.promiseAwait',
      tinyGoObjectRef(10),
      tinyGoFunctionRef(11),
      tinyGoFunctionRef(12),
    )
    await expect(promiseResult).resolves.toBe('ready')
  })

  it('installs direct TinyGo fetch imports with stored-byte handoff', async () => {
    const memory = new WebAssembly.Memory({ initial: 1 })
    type FetchResult = {
      opID: number
      meta: unknown
      body: number[]
    }
    const resolveResult: { fn?: (value: FetchResult) => void } = {}
    const result = new Promise<FetchResult>((resolve) => {
      resolveResult.fn = resolve
    })
    const resolveExport = vi.fn(
      (
        opID: number,
        metaID: number,
        metaLen: number,
        bodyID: number,
        bodyLen: number,
      ) => {
        const metaBytes = readTinyGoStoredBytes(go, metaID, metaLen, 256)
        const bodyBytes = readTinyGoStoredBytes(go, bodyID, bodyLen, 512)
        resolveResult.fn?.({
          opID,
          meta: JSON.parse(new TextDecoder().decode(metaBytes)),
          body: Array.from(bodyBytes),
        })
      },
    )
    const rejectExport = vi.fn()
    const go: TinyGoRuntime = {
      importObject: { gojs: {} },
      _inst: {
        exports: {
          memory,
          BLDR_TINYGO_FETCH_RESOLVE: resolveExport,
          BLDR_TINYGO_FETCH_REJECT: rejectExport,
          go_scheduler: vi.fn(),
        },
      },
    }
    const fetchMock = vi.fn(async (url: string, init: RequestInit) => {
      expect(url).toBe('/api/auth/config')
      expect(init.method).toBe('POST')
      expect(init.cache).toBe('no-store')
      expect(init.signal).toBeInstanceOf(AbortSignal)
      expect(init.headers).toBeInstanceOf(Headers)
      if (!(init.headers instanceof Headers)) {
        throw new Error('expected fetch headers')
      }
      expect(init.headers.get('x-test')).toBe('ok')
      expect(init.body).toBeInstanceOf(ArrayBuffer)
      if (!(init.body instanceof ArrayBuffer)) {
        throw new Error('expected fetch body')
      }
      expect(Array.from(new Uint8Array(init.body))).toEqual([1, 2, 3])
      return new Response(new Uint8Array([4, 5]), {
        status: 201,
        statusText: 'Created',
        headers: { 'x-resp': 'yes' },
      })
    })
    vi.stubGlobal('fetch', fetchMock)
    installTinyGoJSHelpers(go)

    const [urlPtr, urlLen] = writeTinyGoString(go, '/api/auth/config', 16)
    const [reqPtr, reqLen] = writeTinyGoString(
      go,
      JSON.stringify({
        method: 'POST',
        header: { 'X-Test': ['ok'] },
        cache: 'no-store',
        signal: true,
      }),
      64,
    )
    new Uint8Array(memory.buffer, 192, 3).set([1, 2, 3])
    callTinyGoImport(
      go,
      'bldr.tinygo.fetch',
      77,
      urlPtr,
      urlLen,
      reqPtr,
      reqLen,
      192,
      3,
    )

    await expect(result).resolves.toEqual({
      opID: 77,
      meta: expect.objectContaining({
        ok: true,
        statusCode: 201,
        statusText: 'Created',
      }),
      body: [4, 5],
    })
    expect(resolveExport).toHaveBeenCalledTimes(1)
    expect(rejectExport).not.toHaveBeenCalled()
    expect(callTinyGoImport(go, 'bldr.tinygo.dropStoredBytes', 999)).toBe(0)
  })

  it('does not reserve TinyGo fetch bytes when resolve export is unavailable', async () => {
    const memory = new WebAssembly.Memory({ initial: 1 })
    const resolvedIDs: Array<{ metaID: number; bodyID: number }> = []
    const resolved = new Map<number, () => void>()
    const rejected = new Map<number, () => void>()
    const resolveExport = vi.fn(
      (
        opID: number,
        metaID: number,
        metaLen: number,
        bodyID: number,
        bodyLen: number,
      ) => {
        resolvedIDs.push({ metaID, bodyID })
        readTinyGoStoredBytes(go, metaID, metaLen, 4096 + opID * 128)
        readTinyGoStoredBytes(go, bodyID, bodyLen, 8192 + opID * 128)
        resolved.get(opID)?.()
        resolved.delete(opID)
      },
    )
    const rejectExport = vi.fn((opID: number) => {
      rejected.get(opID)?.()
      rejected.delete(opID)
    })
    const exportsObject = {
      memory,
      BLDR_TINYGO_FETCH_RESOLVE: resolveExport,
      BLDR_TINYGO_FETCH_REJECT: rejectExport,
      go_scheduler: vi.fn(),
    }
    const go: TinyGoRuntime = {
      importObject: { gojs: {} },
      _inst: { exports: exportsObject },
    }
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(new Uint8Array([9]), { status: 200 })),
    )
    installTinyGoJSHelpers(go)

    const invokeFetch = (opID: number, waitForReject: boolean) =>
      new Promise<void>((resolve) => {
        if (waitForReject) {
          rejected.set(opID, resolve)
        } else {
          resolved.set(opID, resolve)
        }
        const [urlPtr, urlLen] = writeTinyGoString(
          go,
          `/fetch-${opID}`,
          256 + opID * 128,
        )
        const [reqPtr, reqLen] = writeTinyGoString(go, '{}', 320 + opID * 128)
        callTinyGoImport(
          go,
          'bldr.tinygo.fetch',
          opID,
          urlPtr,
          urlLen,
          reqPtr,
          reqLen,
          0,
          0,
        )
      })

    await invokeFetch(1, false)
    const first = resolvedIDs[0]
    if (!first) {
      throw new Error('first fetch did not resolve')
    }
    Reflect.deleteProperty(exportsObject, 'BLDR_TINYGO_FETCH_RESOLVE')

    await invokeFetch(2, true)
    expect(rejectExport).toHaveBeenCalledWith(2, 0)

    Reflect.set(exportsObject, 'BLDR_TINYGO_FETCH_RESOLVE', resolveExport)
    await invokeFetch(3, false)
    const third = resolvedIDs[1]
    if (!third) {
      throw new Error('third fetch did not resolve')
    }

    expect(third.metaID).toBe(first.bodyID + 1)
    expect(third.bodyID).toBe(first.bodyID + 2)
  })

  it('rejects OPFS Web Locks requests when the runtime lacks Web Locks', async () => {
    vi.stubGlobal('navigator', {})
    installTinyGoJSHelpers(newTinyGoRuntime())
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
    const opfsResolves = new Map<number, (values: number[]) => void>()
    const opfsRejects = new Map<number, (code: number) => void>()
    const waitOPFS = <T>(
      opID: number,
      invoke: () => void,
      map: (values: number[]) => T,
    ) =>
      new Promise<T>((resolve) => {
        opfsResolves.set(opID, (values) => resolve(map(values)))
        opfsRejects.set(opID, (code) => resolve(map([-code])))
        invoke()
      })
    const opfsResolve = (
      opID: number,
      count: number,
      value0: number,
      value1: number,
    ) => {
      const values = [value0, value1].slice(0, count)
      opfsResolves.get(opID)?.(values)
      opfsResolves.delete(opID)
      opfsRejects.delete(opID)
    }
    const opfsReject = (opID: number, code: number) => {
      opfsRejects.get(opID)?.(code)
      opfsResolves.delete(opID)
      opfsRejects.delete(opID)
    }
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

      const read = await waitOPFS(
        101,
        () => {
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
          )
        },
        ([id = 0, len = 0]) => ({ id, len }),
      )
      const stored = g.BLDR_TINYGO_TAKE_STORED_BYTES?.(read.id)

      expect(read.len).toBe(3)
      expect(stored && Array.from(stored)).toEqual([4, 5, 6])
      expect(g.BLDR_TINYGO_TAKE_STORED_BYTES?.(read.id)).toBeUndefined()

      const listed = await waitOPFS(
        102,
        () => {
          g.BLDR_OPFS_LIST_DIRECTORY?.(
            {
              entries: async function* () {
                yield ['manifest-a'] as unknown as [string, FileSystemHandle]
                yield ['wal-a'] as unknown as [string, FileSystemHandle]
              },
            } as unknown as FileSystemDirectoryHandle,
            102,
          )
        },
        ([id = 0, len = 0]) => ({ id, len }),
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
      const readAt = await waitOPFS(
        103,
        () => {
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
          )
        },
        ([read = -1]) => read,
      )

      expect(readAt).toBe(4)
      expect(readAtSlices).toEqual([[3, 7]])
      expect(Array.from(readAtBytes)).toEqual([11, 12, 13, 14])

      const writtenChunks: Uint8Array[] = []
      const seeks: number[] = []
      const writeAtBytes = new Uint8Array([7, 8, 9, 10])
      const written = await waitOPFS(
        104,
        () => {
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
          )
        },
        ([written = -1]) => written,
      )

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
      const wholeFileWritten = await waitOPFS(
        105,
        () => {
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
                    abort: async () => undefined,
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
          )
        },
        ([written = -1]) => written,
      )

      expect(wholeFileWritten).toBe(3)
      expect(wholeFileWrites.map((chunk) => Array.from(chunk))).toEqual([
        [21, 22, 23],
      ])

      let wholeFileFailedAborts = 0
      let wholeFileFailedCloses = 0
      const wholeFileFailureCode = await waitOPFS(
        106,
        () => {
          g.BLDR_OPFS_WRITE_FILE?.(
            {
              getFileHandle: async (
                name: string,
                opts?: { create?: boolean },
              ) => {
                if (name !== 'wal-fail' || opts?.create !== true) {
                  throw new Error('unexpected failing file handle request')
                }
                return {
                  createWritable: async () => ({
                    abort: async () => {
                      wholeFileFailedAborts++
                    },
                    close: async () => {
                      wholeFileFailedCloses++
                    },
                    write: async () => {
                      const err = new Error('whole file write failure')
                      err.name = 'NoModificationAllowedError'
                      throw err
                    },
                  }),
                }
              },
            } as unknown as FileSystemDirectoryHandle,
            'wal-fail',
            new Uint8Array([24, 25]),
            106,
          )
        },
        ([code = 0]) => -code,
      )

      expect(wholeFileFailureCode).toBe(2)
      expect(wholeFileFailedAborts).toBe(1)
      expect(wholeFileFailedCloses).toBe(0)

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
      public readonly importObject = { gojs: {} }
      public env: Record<string, string> = {}
      public argv: string[] = []
      public _inst?: WebAssembly.Instance
      public _resume = vi.fn()

      public async run() {
        this._inst = {
          exports: {
            memory: new WebAssembly.Memory({ initial: 1 }),
            BLDR_OPFS_HELPER_RESOLVE: opfsResolve,
            BLDR_OPFS_HELPER_REJECT: opfsReject,
          },
        }
        await run()
      }
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

  it('aborts TinyGo OPFS write streams when the runtime exits', async () => {
    const opfsResolves = new Map<number, (values: number[]) => void>()
    const opfsRejects = new Map<number, (code: number) => void>()
    const waitOPFS = <T>(
      opID: number,
      invoke: () => void,
      map: (values: number[]) => T,
    ) =>
      new Promise<T>((resolve) => {
        opfsResolves.set(opID, (values) => resolve(map(values)))
        opfsRejects.set(opID, (code) => resolve(map([-code])))
        invoke()
      })
    const state: {
      lateCallbacks: number
      aborts: number
      createWritableAfterExit: number
      startRejected: boolean
      resolveAbortCleanup?: () => void
      rejectPendingWrite?: (reason?: unknown) => void
      resolveLateAbort?: () => void
      resolvePendingHandle?: (handle: FileSystemFileHandle) => void
      resolvePendingOpen?: (writable: FileSystemWritableFileStream) => void
      pendingOpenReady?: () => void
    } = {
      lateCallbacks: 0,
      aborts: 0,
      createWritableAfterExit: 0,
      startRejected: false,
    }
    const opfsResolve = (
      opID: number,
      count: number,
      value0: number,
      value1: number,
    ) => {
      if (opID === 202 || opID === 203 || opID === 204) {
        state.lateCallbacks++
      }
      const values = [value0, value1].slice(0, count)
      opfsResolves.get(opID)?.(values)
      opfsResolves.delete(opID)
      opfsRejects.delete(opID)
    }
    const opfsReject = (opID: number, code: number) => {
      if (opID === 202 || opID === 203 || opID === 204) {
        state.lateCallbacks++
      }
      opfsRejects.get(opID)?.(code)
      opfsResolves.delete(opID)
      opfsRejects.delete(opID)
    }
    const pendingOpen = new Promise<void>((resolve) => {
      state.pendingOpenReady = resolve
    })
    const lateAbort = new Promise<void>((resolve) => {
      state.resolveLateAbort = resolve
    })
    const abortCleanup = new Promise<void>((resolve) => {
      state.resolveAbortCleanup = resolve
    })
    const dir = {
      getFileHandle: async (
        name: string,
        opts?: { create?: boolean },
      ) => {
        if (opts?.create !== true) {
          throw new Error('unexpected file handle request')
        }
        if (name === 'late.bin') {
          return {
            createWritable: async () =>
              new Promise<FileSystemWritableFileStream>((resolve) => {
                state.resolvePendingOpen = resolve
                state.pendingOpenReady?.()
              }),
          }
        }
        if (name === 'after.bin') {
          return new Promise<FileSystemFileHandle>((resolve) => {
            state.resolvePendingHandle = resolve
          })
        }
        if (name !== 'stream.bin') {
          throw new Error('unexpected file handle request')
        }
        return {
          createWritable: async () => ({
            abort: async () => {
              state.aborts++
              state.rejectPendingWrite?.(new Error('aborted'))
              await abortCleanup
            },
            close: async () => undefined,
            write: async () =>
              new Promise<void>((_resolve, reject) => {
                state.rejectPendingWrite = reject
              }),
          }),
        }
      },
    } as unknown as FileSystemDirectoryHandle

    class FakeGo {
      public readonly importObject = { gojs: {} }
      public env: Record<string, string> = {}
      public argv: string[] = []
      public _inst?: WebAssembly.Instance
      public _resume = vi.fn()
      public _values: unknown[] = [
        NaN,
        0,
        null,
        true,
        false,
        globalThis,
        this,
        dir,
      ]

      public async run() {
        const memory = new WebAssembly.Memory({ initial: 1 })
        this._inst = {
          exports: {
            memory,
            BLDR_OPFS_HELPER_RESOLVE: opfsResolve,
            BLDR_OPFS_HELPER_REJECT: opfsReject,
          },
        }
        new TextEncoder().encodeInto(
          'stream.bin',
          new Uint8Array(memory.buffer, 32, 10),
        )
        new TextEncoder().encodeInto(
          'late.bin',
          new Uint8Array(memory.buffer, 64, 8),
        )
        new TextEncoder().encodeInto(
          'after.bin',
          new Uint8Array(memory.buffer, 80, 9),
        )
        const gojs = this.importObject.gojs as Record<string, unknown>
        const openWriteStream = gojs['bldr.opfs.openWriteStreamRef'] as
          | ((
              opID: number,
              dirRef: bigint,
              namePtr: number,
              nameLen: number,
            ) => void)
          | undefined
        const writeStream = gojs['bldr.opfs.writeStreamRef'] as
          | ((
              opID: number,
              streamID: number,
              dataPtr: number,
              dataLen: number,
            ) => void)
          | undefined
        if (!openWriteStream || !writeStream) {
          throw new Error('OPFS write stream import bridge was not installed')
        }

        const streamID = await waitOPFS(
          201,
          () => openWriteStream(201, tinyGoObjectRef(7), 32, 10),
          ([id = 0]) => id,
        )
        expect(streamID).toBeGreaterThan(0)
        new Uint8Array(memory.buffer).set([1, 2, 3], 48)
        writeStream(202, streamID, 48, 3)
        openWriteStream(203, tinyGoObjectRef(7), 64, 8)
        openWriteStream(204, tinyGoObjectRef(7), 80, 9)
        throw new Error('runtime trap')
      }
    }

    vi.stubGlobal('Go', FakeGo)
    vi.spyOn(WebAssembly, 'instantiate').mockResolvedValue({
      exports: {},
    })

    const process = new GoWasmProcess(
      {},
      {
        retry: false,
      },
    )

    const started = process.start()
    started.catch(() => {
      state.startRejected = true
    })
    await pendingOpen
    expect(state.aborts).toBe(1)
    await Promise.resolve()
    expect(state.startRejected).toBe(false)
    state.resolvePendingOpen?.({
      abort: async () => {
        state.aborts++
        state.resolveLateAbort?.()
      },
      close: async () => undefined,
      write: async () => undefined,
    } as unknown as FileSystemWritableFileStream)
    state.resolvePendingHandle?.({
      createWritable: async () => {
        state.createWritableAfterExit++
        return {
          abort: async () => {
            state.aborts++
          },
          close: async () => undefined,
          write: async () => undefined,
        } as unknown as FileSystemWritableFileStream
      },
    } as unknown as FileSystemFileHandle)
    state.rejectPendingWrite?.(new Error('aborted'))
    await lateAbort
    state.resolveAbortCleanup?.()
    await expect(started).rejects.toThrow('runtime trap')
    expect(state.aborts).toBe(2)
    expect(state.createWritableAfterExit).toBe(0)
    expect(state.lateCallbacks).toBe(0)
  })
})
