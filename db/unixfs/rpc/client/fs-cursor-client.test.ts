import { describe, it, expect, vi } from 'vitest'
import type { FSCursorService } from '../rpc_srpc.pb.js'
import type { FSCursorClientResponse } from '../rpc.pb.js'
import { NodeType } from '../../block/fstree.pb.js'
import { FSCursorClient } from './fs-cursor-client.js'
import { RemoteFSCursor } from './fs-cursor-remote.js'
import { RemoteFSCursorOps } from './fs-cursor-ops-remote.js'

// createAsyncIterable builds an AsyncIterable from an array of values plus an
// optional "hold open" promise that keeps the stream alive after all values are
// yielded. This simulates a server-streaming RPC.
function createAsyncIterable<T>(
  values: T[],
  hold?: Promise<void>,
): AsyncIterable<T> {
  return {
    [Symbol.asyncIterator]() {
      let idx = 0
      return {
        async next(): Promise<IteratorResult<T>> {
          if (idx < values.length) {
            return { done: false, value: values[idx++] }
          }
          if (hold) {
            await hold
          }
          return { done: true, value: undefined }
        },
      }
    },
  }
}

// createControllableIterable builds an AsyncIterable where values can be
// pushed dynamically. Useful for simulating streaming RPCs where messages
// arrive after initial setup.
function createControllableIterable<T>(): {
  iterable: AsyncIterable<T>
  push: (value: T) => void
  end: () => void
} {
  const queue: T[] = []
  let done = false
  let waiting: ((result: IteratorResult<T>) => void) | null = null

  const push = (value: T) => {
    if (waiting) {
      const resolve = waiting
      waiting = null
      resolve({ done: false, value })
    } else {
      queue.push(value)
    }
  }

  const end = () => {
    done = true
    if (waiting) {
      const resolve = waiting
      waiting = null
      resolve({ done: true, value: undefined })
    }
  }

  const iterable: AsyncIterable<T> = {
    [Symbol.asyncIterator]() {
      return {
        next(): Promise<IteratorResult<T>> {
          if (queue.length > 0) {
            return Promise.resolve({ done: false, value: queue.shift()! })
          }
          if (done) {
            return Promise.resolve({ done: true, value: undefined })
          }
          return new Promise((resolve) => {
            waiting = resolve
          })
        },
      }
    },
  }

  return { iterable, push, end }
}

// buildMockService builds a mock FSCursorService with configurable behavior.
function buildMockService(opts: {
  clientHandleId: bigint
  rootCursorHandleId: bigint
  cursorChanges?: FSCursorClientResponse[]
  proxyCursorId?: bigint
  opsHandleId?: bigint
  opsName?: string
  opsNodeType?: NodeType
  readData?: Uint8Array
  fileSize?: bigint
}): { service: FSCursorService; holdResolve: () => void } {
  let holdResolve!: () => void
  const holdPromise = new Promise<void>((r) => {
    holdResolve = r
  })

  const initMsg: FSCursorClientResponse = {
    body: {
      case: 'init',
      value: {
        clientHandleId: opts.clientHandleId,
        cursorHandleId: opts.rootCursorHandleId,
      },
    },
  }

  const streamMsgs: FSCursorClientResponse[] = [
    initMsg,
    ...(opts.cursorChanges ?? []),
  ]

  const service: FSCursorService = {
    FSCursorClient: vi.fn(
      (_req: unknown, _signal?: AbortSignal) =>
        createAsyncIterable(streamMsgs, holdPromise) as ReturnType<
          FSCursorService['FSCursorClient']
        >,
    ),
    GetProxyCursor: vi.fn(async () => ({
      cursorHandleId: opts.proxyCursorId ?? 0n,
    })),
    GetCursorOps: vi.fn(async () => ({
      opsHandleId: opts.opsHandleId ?? 10n,
      name: opts.opsName ?? 'root',
      nodeType: opts.opsNodeType ?? NodeType.NodeType_DIRECTORY,
    })),
    ReleaseFSCursor: vi.fn(async () => ({})),
    OpsReadAt: vi.fn(async () => ({
      data: opts.readData ?? new Uint8Array([1, 2, 3]),
    })),
    OpsGetSize: vi.fn(async () => ({
      size: opts.fileSize ?? 100n,
    })),
    OpsGetPermissions: vi.fn(async () => ({ fileMode: 0o644 })),
    OpsSetPermissions: vi.fn(async () => ({})),
    OpsGetModTimestamp: vi.fn(async () => ({ modTimestamp: new Date() })),
    OpsSetModTimestamp: vi.fn(async () => ({})),
    OpsGetOptimalWriteSize: vi.fn(async () => ({ optimalWriteSize: 4096n })),
    OpsWriteAt: vi.fn(async () => ({})),
    OpsTruncate: vi.fn(async () => ({})),
    OpsLookup: vi.fn(async () => ({ cursorHandleId: 0n })),
    OpsReaddirAll: vi.fn(
      () =>
        createAsyncIterable([]) as ReturnType<FSCursorService['OpsReaddirAll']>,
    ),
    OpsMknod: vi.fn(async () => ({})),
    OpsSymlink: vi.fn(async () => ({})),
    OpsReadlink: vi.fn(async () => ({ symlink: { targetPath: {} } })),
    OpsCopyTo: vi.fn(async () => ({ done: false })),
    OpsCopyFrom: vi.fn(async () => ({ done: false })),
    OpsMoveTo: vi.fn(async () => ({ done: false })),
    OpsMoveFrom: vi.fn(async () => ({ done: false })),
    OpsRemove: vi.fn(async () => ({})),
  }

  return { service, holdResolve }
}

describe('FSCursorClient', () => {
  describe('build', () => {
    it('creates client with correct clientHandleId and rootCursor', async () => {
      const { service, holdResolve } = buildMockService({
        clientHandleId: 42n,
        rootCursorHandleId: 1n,
      })

      const fsc = await FSCursorClient.build(service)
      expect(fsc.clientHandleId).toBe(42n)
      expect(fsc.rootCursor).toBeInstanceOf(RemoteFSCursor)
      expect(fsc.rootCursor.cursorHandleId).toBe(1n)
      expect(fsc.released).toBe(false)

      fsc.release()
      holdResolve()
    })

    it('throws if stream closes before init', async () => {
      const service: FSCursorService = {
        FSCursorClient: vi.fn(
          () =>
            createAsyncIterable([]) as ReturnType<
              FSCursorService['FSCursorClient']
            >,
        ),
        GetProxyCursor: vi.fn(async () => ({})),
        GetCursorOps: vi.fn(async () => ({})),
        ReleaseFSCursor: vi.fn(async () => ({})),
        OpsGetPermissions: vi.fn(async () => ({})),
        OpsSetPermissions: vi.fn(async () => ({})),
        OpsGetSize: vi.fn(async () => ({})),
        OpsGetModTimestamp: vi.fn(async () => ({})),
        OpsSetModTimestamp: vi.fn(async () => ({})),
        OpsReadAt: vi.fn(async () => ({})),
        OpsGetOptimalWriteSize: vi.fn(async () => ({})),
        OpsWriteAt: vi.fn(async () => ({})),
        OpsTruncate: vi.fn(async () => ({})),
        OpsLookup: vi.fn(async () => ({})),
        OpsReaddirAll: vi.fn(
          () =>
            createAsyncIterable([]) as ReturnType<
              FSCursorService['OpsReaddirAll']
            >,
        ),
        OpsMknod: vi.fn(async () => ({})),
        OpsSymlink: vi.fn(async () => ({})),
        OpsReadlink: vi.fn(async () => ({})),
        OpsCopyTo: vi.fn(async () => ({})),
        OpsCopyFrom: vi.fn(async () => ({})),
        OpsMoveTo: vi.fn(async () => ({})),
        OpsMoveFrom: vi.fn(async () => ({})),
        OpsRemove: vi.fn(async () => ({})),
      }

      await expect(FSCursorClient.build(service)).rejects.toThrow(
        'stream closed before init',
      )
    })

    it('throws if first message is not init', async () => {
      let holdResolve!: () => void
      const holdPromise = new Promise<void>((r) => {
        holdResolve = r
      })

      const nonInitMsg: FSCursorClientResponse = {
        body: {
          case: 'cursorChange',
          value: { cursorHandleId: 1n },
        },
      }

      const service: FSCursorService = {
        FSCursorClient: vi.fn(
          () =>
            createAsyncIterable([nonInitMsg], holdPromise) as ReturnType<
              FSCursorService['FSCursorClient']
            >,
        ),
        GetProxyCursor: vi.fn(async () => ({})),
        GetCursorOps: vi.fn(async () => ({})),
        ReleaseFSCursor: vi.fn(async () => ({})),
        OpsGetPermissions: vi.fn(async () => ({})),
        OpsSetPermissions: vi.fn(async () => ({})),
        OpsGetSize: vi.fn(async () => ({})),
        OpsGetModTimestamp: vi.fn(async () => ({})),
        OpsSetModTimestamp: vi.fn(async () => ({})),
        OpsReadAt: vi.fn(async () => ({})),
        OpsGetOptimalWriteSize: vi.fn(async () => ({})),
        OpsWriteAt: vi.fn(async () => ({})),
        OpsTruncate: vi.fn(async () => ({})),
        OpsLookup: vi.fn(async () => ({})),
        OpsReaddirAll: vi.fn(
          () =>
            createAsyncIterable([]) as ReturnType<
              FSCursorService['OpsReaddirAll']
            >,
        ),
        OpsMknod: vi.fn(async () => ({})),
        OpsSymlink: vi.fn(async () => ({})),
        OpsReadlink: vi.fn(async () => ({})),
        OpsCopyTo: vi.fn(async () => ({})),
        OpsCopyFrom: vi.fn(async () => ({})),
        OpsMoveTo: vi.fn(async () => ({})),
        OpsMoveFrom: vi.fn(async () => ({})),
        OpsRemove: vi.fn(async () => ({})),
      }

      await expect(FSCursorClient.build(service)).rejects.toThrow(
        'expected init',
      )
      holdResolve()
    })
  })

  describe('cursor change events', () => {
    it('routes cursor change to the correct cursor callbacks', async () => {
      const changeCb = vi.fn(() => true)

      const { iterable, push, end } =
        createControllableIterable<FSCursorClientResponse>()

      // Push init message immediately.
      push({
        body: {
          case: 'init',
          value: { clientHandleId: 1n, cursorHandleId: 1n },
        },
      })

      const service = buildMockService({
        clientHandleId: 1n,
        rootCursorHandleId: 1n,
      }).service
      // Override FSCursorClient to use our controllable stream.
      service.FSCursorClient = vi.fn(
        () => iterable as ReturnType<FSCursorService['FSCursorClient']>,
      )

      const fsc = await FSCursorClient.build(service)
      fsc.rootCursor.addChangeCb(changeCb)

      // Now push the cursor change after callback is registered.
      push({
        body: {
          case: 'cursorChange',
          value: {
            cursorHandleId: 1n,
            released: false,
            offset: 10n,
            size: 20n,
          },
        },
      })

      // Let the background loop process the cursor change.
      await new Promise((r) => setTimeout(r, 50))

      expect(changeCb).toHaveBeenCalledTimes(1)
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const call = (changeCb.mock.calls as any)[0][0] as {
        cursor: unknown
        released: boolean
        offset: bigint
        size: bigint
      }
      expect(call.cursor).toBe(fsc.rootCursor)
      expect(call.released).toBe(false)
      expect(call.offset).toBe(10n)
      expect(call.size).toBe(20n)

      fsc.release()
      end()
    })

    it('released cursor change removes cursor from maps', async () => {
      const { service, holdResolve } = buildMockService({
        clientHandleId: 1n,
        rootCursorHandleId: 1n,
        cursorChanges: [
          {
            body: {
              case: 'cursorChange',
              value: {
                cursorHandleId: 1n,
                released: true,
              },
            },
          },
        ],
      })

      const fsc = await FSCursorClient.build(service)
      const rootCursor = fsc.rootCursor

      // Let the background loop process the released cursor change.
      await new Promise((r) => setTimeout(r, 50))

      expect(rootCursor.released).toBe(true)

      fsc.release()
      holdResolve()
    })
  })

  describe('ingestCursor', () => {
    it('reuses existing non-released cursor', async () => {
      const { service, holdResolve } = buildMockService({
        clientHandleId: 1n,
        rootCursorHandleId: 1n,
      })

      const fsc = await FSCursorClient.build(service)
      const c1 = fsc.ingestCursor(1n)
      expect(c1).toBe(fsc.rootCursor)

      // Ingesting same ID returns same instance.
      const c2 = fsc.ingestCursor(1n)
      expect(c2).toBe(c1)

      fsc.release()
      holdResolve()
    })

    it('creates new cursor for unknown handle ID', async () => {
      const { service, holdResolve } = buildMockService({
        clientHandleId: 1n,
        rootCursorHandleId: 1n,
      })

      const fsc = await FSCursorClient.build(service)
      const newCursor = fsc.ingestCursor(99n)
      expect(newCursor).toBeInstanceOf(RemoteFSCursor)
      expect(newCursor.cursorHandleId).toBe(99n)
      expect(newCursor).not.toBe(fsc.rootCursor)

      fsc.release()
      holdResolve()
    })

    it('creates new cursor when existing one is released', async () => {
      const { service, holdResolve } = buildMockService({
        clientHandleId: 1n,
        rootCursorHandleId: 1n,
      })

      const fsc = await FSCursorClient.build(service)
      const original = fsc.rootCursor
      original.released = true

      const replacement = fsc.ingestCursor(1n)
      expect(replacement).not.toBe(original)
      expect(replacement.cursorHandleId).toBe(1n)

      fsc.release()
      holdResolve()
    })
  })

  describe('resolveOps', () => {
    it('creates ops and associates with cursor', async () => {
      const { service, holdResolve } = buildMockService({
        clientHandleId: 1n,
        rootCursorHandleId: 1n,
        opsHandleId: 10n,
        opsName: 'root',
        opsNodeType: NodeType.NodeType_DIRECTORY,
      })

      const fsc = await FSCursorClient.build(service)
      const ops = fsc.resolveOps(1n, 10n, NodeType.NodeType_DIRECTORY, 'root')
      expect(ops).toBeInstanceOf(RemoteFSCursorOps)
      expect(ops.handleId).toBe(10n)
      expect(ops.name).toBe('root')
      expect(ops.nodeType).toBe(NodeType.NodeType_DIRECTORY)

      fsc.release()
      holdResolve()
    })

    it('reuses existing ops if matching', async () => {
      const { service, holdResolve } = buildMockService({
        clientHandleId: 1n,
        rootCursorHandleId: 1n,
      })

      const fsc = await FSCursorClient.build(service)
      const ops1 = fsc.resolveOps(1n, 10n, NodeType.NodeType_DIRECTORY, 'root')
      const ops2 = fsc.resolveOps(1n, 10n, NodeType.NodeType_DIRECTORY, 'root')
      expect(ops2).toBe(ops1)

      fsc.release()
      holdResolve()
    })
  })

  describe('release', () => {
    it('marks all cursors and ops as released', async () => {
      const { service, holdResolve } = buildMockService({
        clientHandleId: 1n,
        rootCursorHandleId: 1n,
      })

      const fsc = await FSCursorClient.build(service)
      const rootCursor = fsc.rootCursor
      const ops = fsc.resolveOps(1n, 10n, NodeType.NodeType_DIRECTORY, 'root')

      expect(fsc.released).toBe(false)
      expect(rootCursor.released).toBe(false)
      expect(ops.released).toBe(false)

      fsc.release()

      expect(fsc.released).toBe(true)
      expect(rootCursor.released).toBe(true)
      expect(ops.released).toBe(true)

      holdResolve()
    })

    it('is idempotent', async () => {
      const { service, holdResolve } = buildMockService({
        clientHandleId: 1n,
        rootCursorHandleId: 1n,
      })

      const fsc = await FSCursorClient.build(service)
      fsc.release()
      fsc.release()
      expect(fsc.released).toBe(true)

      holdResolve()
    })

    it('Symbol.dispose calls release', async () => {
      const { service, holdResolve } = buildMockService({
        clientHandleId: 1n,
        rootCursorHandleId: 1n,
      })

      const fsc = await FSCursorClient.build(service)
      fsc[Symbol.dispose]()
      expect(fsc.released).toBe(true)

      holdResolve()
    })
  })

  describe('removeOps', () => {
    it('removes ops entry from map', async () => {
      const { service, holdResolve } = buildMockService({
        clientHandleId: 1n,
        rootCursorHandleId: 1n,
      })

      const fsc = await FSCursorClient.build(service)
      const ops = fsc.resolveOps(1n, 10n, NodeType.NodeType_DIRECTORY, 'root')

      fsc.removeOps(10n)

      // Creating ops with same ID now creates a new instance.
      const ops2 = fsc.resolveOps(1n, 10n, NodeType.NodeType_DIRECTORY, 'root')
      expect(ops2).not.toBe(ops)

      fsc.release()
      holdResolve()
    })
  })
})
