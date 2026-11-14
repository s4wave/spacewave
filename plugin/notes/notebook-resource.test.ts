import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  Notebook,
  NotebookEditOp,
  NotebookSource,
} from './proto/notebook.pb.js'
import { NotebookEditOutcome } from './sdk/notebook.pb.js'
import { PluginOpTimeoutError } from '@s4wave/sdk/world/errors.js'
import { WorldErrorCode } from '@s4wave/sdk/world/world.pb.js'

// Mock cursor returned by objectState.accessWorldState().
function createMockCursor(blockData?: Uint8Array) {
  const release = vi.fn()
  return {
    getBlock: vi.fn(async () => ({ found: !!blockData, data: blockData })),
    putBlock: vi.fn(
      async (_request: { data: Uint8Array }, _signal?: AbortSignal) => ({
        ref: { hash: { hash: new Uint8Array([1, 2, 3]) } },
      }),
    ),
    getRef: vi.fn(async () => ({ ref: { bucketId: 'test' } })),
    release,
    [Symbol.dispose]: release,
  }
}

// Mock ObjectState returned by tx.getObject().
function createMockObjectState(
  cursor?:
    | ReturnType<typeof createMockCursor>
    | ReturnType<typeof createMockCursor>[],
) {
  const cursors = Array.isArray(cursor)
    ? cursor
    : [cursor ?? createMockCursor()]
  let idx = 0
  return {
    getRootRef: vi.fn(() =>
      Promise.resolve({
        rootRef: {
          bucketId: 'test',
          rootRef: { hash: { hash: new Uint8Array([1]) } },
        },
      }),
    ),
    accessWorldState: vi.fn(() => {
      const current = cursors[Math.min(idx, cursors.length - 1)]
      idx++
      return Promise.resolve(current)
    }),
    setRootRef: vi.fn(() => Promise.resolve({ rev: 1n })),
    release: vi.fn(),
    [Symbol.dispose]() {
      this.release()
    },
  }
}

// Mock read snapshot yielded by engine.watchWorldState().
function createMockSnapshot(
  objectState?: ReturnType<typeof createMockObjectState> | null,
  seqno = 1n,
) {
  const release = vi.fn()
  return {
    getSeqno: vi.fn(() => Promise.resolve({ seqno })),
    getObject: vi.fn(() => Promise.resolve(objectState ?? null)),
    release,
    [Symbol.dispose]: release,
  }
}

type MockSnapshot = ReturnType<typeof createMockSnapshot>

// abortError is the rejection of an aborted wait.
function abortError() {
  return new DOMException('Aborted', 'AbortError')
}

// Mock Engine created from engineRef. Its watch yields each queued snapshot,
// then waits for the next push until the signal aborts.
function createMockEngine(snapshots: MockSnapshot[] = []) {
  const queue = [...snapshots]
  let wake: (() => void) | undefined
  return {
    newTransaction: vi.fn(),
    executeWorldOp: vi.fn(
      (_opTypeId: string, _opData: Uint8Array, _signal?: AbortSignal) =>
        Promise.resolve<{
          seqno?: bigint
          sysErr?: boolean
          rejectionCode?: string
          rejectionMessage?: string
          errorCode?: WorldErrorCode
        }>({ seqno: 2n }),
    ),
    watchWorldState: vi.fn(async function* (signal?: AbortSignal) {
      for (;;) {
        const next = queue.shift()
        if (next) {
          yield next
          continue
        }
        await new Promise<void>((resolve, reject) => {
          if (signal?.aborted) {
            reject(abortError())
            return
          }
          wake = resolve
          signal?.addEventListener('abort', () => reject(abortError()), {
            once: true,
          })
        })
      }
    }),
    push(snapshot: MockSnapshot) {
      queue.push(snapshot)
      wake?.()
    },
    resourceRef: createMockEngineRef(),
    id: 1,
    release: vi.fn(),
    [Symbol.dispose]() {
      this.release()
    },
  }
}

let mockEngineInstance: ReturnType<typeof createMockEngine>

vi.mock('@s4wave/sdk/world/engine.js', () => ({
  Engine: vi.fn(function (ref) {
    mockEngineInstance.resourceRef = ref
    return mockEngineInstance
  }),
}))

// Import after mocking.
const { NotebookResource } = await import('./notebook-resource.js')
const { NOTEBOOK_CONFLICT_CODE, NOTEBOOK_EDIT_OP_ID } =
  await import('./notebook-edit.js')

function createMockEngineRef() {
  return {
    resourceId: 1,
    released: false,
    client: {},
    createRef: vi.fn(() => createMockEngineRef()),
    createResource: vi.fn(),
    release: vi.fn(),
    [Symbol.dispose]: vi.fn(),
  }
}

function makeNotebookBytes(name: string, sources: NotebookSource[]) {
  return Notebook.toBinary(Notebook.create({ name, sources }))
}

describe('NotebookResource', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('WatchNotebook', () => {
    it('yields nothing when engineRef is undefined', async () => {
      const resource = new NotebookResource('obj-key', undefined)
      const results: unknown[] = []
      for await (const msg of resource.WatchNotebook({})) {
        results.push(msg)
      }
      expect(results).toHaveLength(0)
    })

    it('yields the Notebook block from the world', async () => {
      const notebookData = makeNotebookBytes('Test Notebook', [
        NotebookSource.create({ name: 'Notes', ref: 'fs/-/notes' }),
      ])
      const cursor = createMockCursor(notebookData)
      const objectState = createMockObjectState(cursor)
      const snapshot = createMockSnapshot(objectState)
      mockEngineInstance = createMockEngine([snapshot])

      const engineRef = createMockEngineRef()
      const resource = new NotebookResource('my-notebook', engineRef as never)

      const ac = new AbortController()
      const results: unknown[] = []
      for await (const msg of resource.WatchNotebook({}, ac.signal)) {
        results.push(msg)
        // Abort after first yield to exit the loop.
        ac.abort()
      }

      expect(results).toHaveLength(1)
      const notebook = (results[0] as { notebook: Notebook }).notebook
      expect(notebook.name).toBe('Test Notebook')
      expect(notebook.sources).toHaveLength(1)
      expect(notebook.sources![0].ref).toBe('fs/-/notes')

      // Verify resource chain was called correctly.
      expect(mockEngineInstance.watchWorldState).toHaveBeenCalledWith(ac.signal)
      expect(snapshot.getObject).toHaveBeenCalledWith('my-notebook', ac.signal)
      expect(objectState.accessWorldState).toHaveBeenCalledWith(
        { bucketId: 'test', rootRef: { hash: { hash: new Uint8Array([1]) } } },
        ac.signal,
      )
      expect(cursor.getBlock).toHaveBeenCalledWith({}, ac.signal)
    })

    it('releases all resources in the read chain', async () => {
      const notebookData = makeNotebookBytes('Cleanup Test', [])
      const cursor = createMockCursor(notebookData)
      const objectState = createMockObjectState(cursor)
      const snapshot = createMockSnapshot(objectState)
      mockEngineInstance = createMockEngine([snapshot])

      const engineRef = createMockEngineRef()
      const resource = new NotebookResource('key', engineRef as never)

      const ac = new AbortController()
      for await (const _ of resource.WatchNotebook({}, ac.signal)) {
        ac.abort()
      }

      expect(cursor.release).toHaveBeenCalled()
      expect(objectState.release).toHaveBeenCalled()
      expect(snapshot.release).toHaveBeenCalledOnce()
      expect(mockEngineInstance.release).toHaveBeenCalled()
    })

    it('yields no Notebook when the object is not found', async () => {
      const snapshot = createMockSnapshot(null)
      mockEngineInstance = createMockEngine([snapshot])

      const engineRef = createMockEngineRef()
      const resource = new NotebookResource('missing', engineRef as never)

      const ac = new AbortController()
      const iter = resource.WatchNotebook({}, ac.signal)[Symbol.asyncIterator]()
      const result = await iter.next()
      expect(result.done).toBe(false)
      expect(result.value?.notebook).toBeUndefined()
      expect(snapshot.getObject).toHaveBeenCalledWith('missing', ac.signal)
      expect(snapshot.release).toHaveBeenCalledOnce()
      ac.abort()
      await iter.return?.(undefined)
    })

    it('yields nothing when block data is missing', async () => {
      const cursor = createMockCursor(undefined)
      const objectState = createMockObjectState(cursor)
      mockEngineInstance = createMockEngine([createMockSnapshot(objectState)])

      const engineRef = createMockEngineRef()
      const resource = new NotebookResource('no-block', engineRef as never)

      const ac = new AbortController()
      const iter = resource.WatchNotebook({}, ac.signal)[Symbol.asyncIterator]()
      const resultPromise = iter.next()
      ac.abort()
      const result = await resultPromise
      expect(result.done).toBe(true)
    })

    it('re-reads the Notebook from the snapshot after a change', async () => {
      // The first snapshot holds the first version of the Notebook.
      const first = createMockObjectState(
        createMockCursor(makeNotebookBytes('Version 1', [])),
      )
      const second = createMockObjectState(
        createMockCursor(
          makeNotebookBytes('Version 2', [
            NotebookSource.create({ name: 'Added', ref: 'fs/-/' }),
          ]),
        ),
      )
      second.getRootRef.mockResolvedValue({
        rootRef: {
          bucketId: 'test',
          rootRef: { hash: { hash: new Uint8Array([2]) } },
        },
      })
      mockEngineInstance = createMockEngine([createMockSnapshot(first)])
      const ac = new AbortController()
      const resource = new NotebookResource(
        'key',
        createMockEngineRef() as never,
      )
      const iter = resource.WatchNotebook({}, ac.signal)[Symbol.asyncIterator]()
      expect((await iter.next()).value?.notebook?.name).toBe('Version 1')

      // The server sends the next snapshot after the Notebook changes.
      const secondPromise = iter.next()
      mockEngineInstance.push(createMockSnapshot(second, 2n))
      const notebook = (await secondPromise).value?.notebook
      expect(notebook?.name).toBe('Version 2')
      expect(notebook?.sources).toHaveLength(1)

      ac.abort()
      await iter.return?.(undefined)
    })

    it('skips decoding on unrelated World changes and retains the resource Engine', async () => {
      // Two World snapshots refer to the same Notebook root.
      const cursor = createMockCursor(makeNotebookBytes('Notebook', []))
      const object = createMockObjectState(cursor)
      const first = createMockSnapshot(object)
      const second = createMockSnapshot(object, 2n)
      mockEngineInstance = createMockEngine([first])
      const engineRef = createMockEngineRef()
      const resource = new NotebookResource('key', engineRef as never)
      const controller = new AbortController()
      const iter = resource
        .WatchNotebook({}, controller.signal)
        [Symbol.asyncIterator]()
      await iter.next()

      // Deliver the unchanged root, then cancel at the next wait.
      const next = iter.next()
      mockEngineInstance.push(second)
      await vi.waitFor(() => expect(second.release).toHaveBeenCalled())
      controller.abort()
      expect((await next).done).toBe(true)
      expect(cursor.getBlock).toHaveBeenCalledTimes(1)
      expect(first.release).toHaveBeenCalledOnce()
      expect(engineRef.createRef).toHaveBeenCalledWith(engineRef.resourceId)
      expect(engineRef.release).not.toHaveBeenCalled()
      resource.dispose()
      expect(engineRef.release).toHaveBeenCalledOnce()
    })
  })

  describe('mutations', () => {
    // sentOp decodes the one edit op the resource executed.
    function sentOp() {
      expect(mockEngineInstance.executeWorldOp).toHaveBeenCalledOnce()
      const [opTypeId, opData] = mockEngineInstance.executeWorldOp.mock
        .calls[0] as [string, Uint8Array]
      expect(opTypeId).toBe(NOTEBOOK_EDIT_OP_ID)
      return NotebookEditOp.fromBinary(opData)
    }

    it('AddSource executes one add op', async () => {
      mockEngineInstance = createMockEngine()
      const resource = new NotebookResource(
        'key',
        createMockEngineRef() as never,
      )
      const source = { name: 'Archive', ref: 'fs/-/archive' }
      await resource.AddSource({ source })
      expect(sentOp()).toEqual({
        objectKey: 'key',
        edit: { case: 'add', value: { source } },
      })
      expect(mockEngineInstance.newTransaction).not.toHaveBeenCalled()
    })

    it('AddSource throws the op rejection', async () => {
      mockEngineInstance = createMockEngine()
      mockEngineInstance.executeWorldOp.mockResolvedValueOnce({
        rejectionCode: 'notes/notebook/source',
        rejectionMessage: 'source ref is required',
      })
      const resource = new NotebookResource(
        'key',
        createMockEngineRef() as never,
      )
      await expect(resource.AddSource({ source: undefined })).rejects.toThrow(
        'source ref is required',
      )
    })

    it('ReorderSources reports a plugin op timeout as nothing changed', async () => {
      mockEngineInstance = createMockEngine()
      mockEngineInstance.executeWorldOp.mockResolvedValueOnce({
        errorCode: WorldErrorCode.PLUGIN_OP_TIMEOUT,
      })
      const resource = new NotebookResource(
        'key',
        createMockEngineRef() as never,
      )
      await expect(
        resource.ReorderSources({ order: [1, 0], sources: [] }),
      ).rejects.toThrow(PluginOpTimeoutError)
    })

    it('RemoveSource carries the source the caller saw', async () => {
      mockEngineInstance = createMockEngine()
      const resource = new NotebookResource(
        'key',
        createMockEngineRef() as never,
      )
      const source = { name: 'One', ref: 'fs/-/one' }
      const resp = await resource.RemoveSource({ index: 1, source })
      expect(resp.outcome).toBe(NotebookEditOutcome.APPLIED)
      expect(sentOp().edit).toEqual({
        case: 'remove',
        value: { index: 1, source },
      })
    })

    it('ReorderSources reports a conflict as the changed outcome', async () => {
      mockEngineInstance = createMockEngine()
      mockEngineInstance.executeWorldOp.mockResolvedValueOnce({
        rejectionCode: NOTEBOOK_CONFLICT_CODE,
        rejectionMessage: 'the notebook changed',
      })
      const resource = new NotebookResource(
        'key',
        createMockEngineRef() as never,
      )
      const sources = [
        { name: 'One', ref: 'fs/-/one' },
        { name: 'Two', ref: 'fs/-/two' },
      ]
      const resp = await resource.ReorderSources({ order: [1, 0], sources })
      expect(resp.outcome).toBe(NotebookEditOutcome.CHANGED)
      expect(sentOp().edit).toEqual({
        case: 'reorder',
        value: { order: [1, 0], sources },
      })
    })

    it('RemoveSource needs the engine', async () => {
      const resource = new NotebookResource('key', undefined)
      await expect(resource.RemoveSource({ index: 0 })).rejects.toThrow(
        'Notebook engine is not available',
      )
    })
  })

  describe('dispose', () => {
    it('releases the engineRef', () => {
      const engineRef = createMockEngineRef()
      const resource = new NotebookResource('key', engineRef as never)
      resource.dispose()
      expect(engineRef.release).toHaveBeenCalled()
    })

    it('is safe to call twice', () => {
      const engineRef = createMockEngineRef()
      const resource = new NotebookResource('key', engineRef as never)
      resource.dispose()
      resource.dispose()
      expect(engineRef.release).toHaveBeenCalledTimes(1)
    })

    it('handles undefined engineRef', () => {
      const resource = new NotebookResource('key', undefined)
      expect(() => resource.dispose()).not.toThrow()
    })
  })
})
