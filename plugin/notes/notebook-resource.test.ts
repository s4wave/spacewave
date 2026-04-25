import { describe, it, expect, vi, beforeEach } from 'vitest'
import { Notebook, NotebookSource } from './proto/notebook.pb.js'

// Mock cursor returned by objectState.accessWorldState().
function createMockCursor(
  blockData?: Uint8Array,
  options?: {
    buildTransaction?: () => Promise<{
      transaction: { write: ReturnType<typeof vi.fn>; release: ReturnType<typeof vi.fn> }
      cursor: { setBlock: ReturnType<typeof vi.fn>; release: ReturnType<typeof vi.fn> }
    }>
    getRef?: () => Promise<{ ref?: string }>
  },
) {
  const release = vi.fn()
  return {
    getBlock: vi.fn(() => Promise.resolve({
      found: !!blockData,
      data: blockData,
    })),
    buildTransaction:
      options?.buildTransaction ??
      vi.fn(() =>
        Promise.resolve({
          transaction: {
            write: vi.fn(() => Promise.resolve({})),
            release: vi.fn(),
          },
          cursor: {
            setBlock: vi.fn(() => Promise.resolve()),
            markDirty: vi.fn(() => Promise.resolve()),
            release: vi.fn(),
          },
        }),
      ),
    getRef: options?.getRef ?? vi.fn(() => Promise.resolve({ ref: 'next-ref' })),
    release,
    [Symbol.dispose]: () => release(),
  }
}

// Mock ObjectState returned by tx.getObject().
function createMockObjectState(
  cursor?:
    | ReturnType<typeof createMockCursor>
    | ReturnType<typeof createMockCursor>[],
) {
  const cursors = Array.isArray(cursor) ? cursor : [cursor ?? createMockCursor()]
  let idx = 0
  return {
    accessWorldState: vi.fn(() => {
      const current = cursors[Math.min(idx, cursors.length - 1)]!
      idx++
      return Promise.resolve(current)
    }),
    setRootRef: vi.fn(() => Promise.resolve({ rev: 1n })),
    release: vi.fn(),
  }
}

// Mock Tx returned by engine.newTransaction().
function createMockTx(
  objectState?: ReturnType<typeof createMockObjectState> | null,
) {
  return {
    getObject: vi.fn(() => Promise.resolve(objectState ?? null)),
    commit: vi.fn(() => Promise.resolve()),
    discard: vi.fn(() => Promise.resolve()),
    release: vi.fn(),
  }
}

// Mock Engine created from engineRef.
function createMockEngine(tx?: ReturnType<typeof createMockTx>) {
  return {
    newTransaction: vi.fn(() => Promise.resolve(tx ?? createMockTx())),
    getSeqno: vi.fn(() => Promise.resolve({ seqno: 1n })),
    waitSeqno: vi.fn(
      (_seqno: bigint, signal?: AbortSignal) =>
        new Promise<{ seqno: bigint }>((resolve, reject) => {
          if (signal?.aborted) {
            reject(new DOMException('Aborted', 'AbortError'))
            return
          }
          const onAbort = () =>
            reject(new DOMException('Aborted', 'AbortError'))
          signal?.addEventListener('abort', onAbort, { once: true })
        }),
    ),
    release: vi.fn(),
  }
}

let mockEngineInstance: ReturnType<typeof createMockEngine>

vi.mock('@s4wave/sdk/world/engine.js', () => ({
  Engine: vi.fn(function () {
    return mockEngineInstance
  }),
}))

// Import after mocking.
const { NotebookResource } = await import('./notebook-resource.js')

function createMockEngineRef() {
  return {
    resourceId: 1,
    released: false,
    client: {},
    createRef: vi.fn(),
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
      const tx = createMockTx(objectState)
      mockEngineInstance = createMockEngine(tx)

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
      expect(mockEngineInstance.newTransaction).toHaveBeenCalledWith(
        false,
        ac.signal,
      )
      expect(tx.getObject).toHaveBeenCalledWith('my-notebook', ac.signal)
      expect(objectState.accessWorldState).toHaveBeenCalledWith(
        undefined,
        ac.signal,
      )
      expect(cursor.getBlock).toHaveBeenCalledWith({}, ac.signal)
    })

    it('releases all resources in the read chain', async () => {
      const notebookData = makeNotebookBytes('Cleanup Test', [])
      const cursor = createMockCursor(notebookData)
      const objectState = createMockObjectState(cursor)
      const tx = createMockTx(objectState)
      mockEngineInstance = createMockEngine(tx)

      const engineRef = createMockEngineRef()
      const resource = new NotebookResource('key', engineRef as never)

      const ac = new AbortController()
      for await (const _ of resource.WatchNotebook({}, ac.signal)) {
        ac.abort()
      }

      expect(cursor.release).toHaveBeenCalled()
      expect(objectState.release).toHaveBeenCalled()
      expect(tx.release).toHaveBeenCalled()
      expect(mockEngineInstance.release).toHaveBeenCalled()
    })

    it('yields nothing when object is not found', async () => {
      const tx = createMockTx(null)
      mockEngineInstance = createMockEngine(tx)

      const engineRef = createMockEngineRef()
      const resource = new NotebookResource('missing', engineRef as never)

      // Pre-abort: generator checks abortSignal.aborted on loop entry after
      // the first read returns null and waitSeqno rejects with AbortError.
      const ac = new AbortController()
      const stream = resource.WatchNotebook({}, ac.signal)
      const iter = stream[Symbol.asyncIterator]()

      // Start iteration. Generator reads null, calls getSeqno, then waitSeqno.
      // Schedule abort so waitSeqno rejects.
      const resultPromise = iter.next()
      ac.abort()
      const result = await resultPromise
      expect(result.done).toBe(true)
      expect(tx.getObject).toHaveBeenCalledWith('missing', ac.signal)
    })

    it('yields nothing when block data is missing', async () => {
      const cursor = createMockCursor(undefined)
      const objectState = createMockObjectState(cursor)
      const tx = createMockTx(objectState)
      mockEngineInstance = createMockEngine(tx)

      const engineRef = createMockEngineRef()
      const resource = new NotebookResource('no-block', engineRef as never)

      const ac = new AbortController()
      const stream = resource.WatchNotebook({}, ac.signal)
      const iter = stream[Symbol.asyncIterator]()
      const resultPromise = iter.next()
      ac.abort()
      const result = await resultPromise
      expect(result.done).toBe(true)
    })

    it('re-reads after waitSeqno resolves', async () => {
      const nb1 = makeNotebookBytes('Version 1', [])
      const nb2 = makeNotebookBytes('Version 2', [
        NotebookSource.create({ name: 'Added', ref: 'fs/-/' }),
      ])

      let readCount = 0
      const cursor1 = createMockCursor(nb1)
      const cursor2 = createMockCursor(nb2)
      const objectState1 = createMockObjectState(cursor1)
      const objectState2 = createMockObjectState(cursor2)

      const tx = {
        getObject: vi.fn(() => {
          readCount++
          return Promise.resolve(
            readCount === 1 ? objectState1 : objectState2,
          )
        }),
        commit: vi.fn(() => Promise.resolve()),
        discard: vi.fn(() => Promise.resolve()),
        release: vi.fn(),
      }

      let waitResolve: (() => void) | undefined
      mockEngineInstance = {
        newTransaction: vi.fn(() => Promise.resolve(tx)),
        getSeqno: vi.fn(() => Promise.resolve({ seqno: BigInt(readCount) })),
        waitSeqno: vi.fn(
          (_seqno: bigint, signal?: AbortSignal) =>
            new Promise<{ seqno: bigint }>((resolve, reject) => {
              if (signal?.aborted) {
                reject(new DOMException('Aborted', 'AbortError'))
                return
              }
              waitResolve = () => resolve({ seqno: _seqno })
              signal?.addEventListener(
                'abort',
                () => reject(new DOMException('Aborted', 'AbortError')),
                { once: true },
              )
            }),
        ),
        release: vi.fn(),
      }

      const engineRef = createMockEngineRef()
      const resource = new NotebookResource('key', engineRef as never)
      const ac = new AbortController()
      const stream = resource.WatchNotebook({}, ac.signal)
      const iter = stream[Symbol.asyncIterator]()

      // First yield: Version 1.
      const first = await iter.next()
      expect(first.done).toBe(false)
      const nb1Result = (first.value as { notebook: Notebook }).notebook
      expect(nb1Result.name).toBe('Version 1')

      // Generator is now suspended at the yield. Resume it: it will call
      // getSeqno then waitSeqno (which blocks). Run concurrently.
      const secondPromise = iter.next()
      // Let the microtask queue drain so waitSeqno is called.
      await new Promise((r) => setTimeout(r, 0))
      // Simulate world change.
      waitResolve!()
      const second = await secondPromise

      expect(second.done).toBe(false)
      const nb2Result = (second.value as { notebook: Notebook }).notebook
      expect(nb2Result.name).toBe('Version 2')
      expect(nb2Result.sources).toHaveLength(1)

      ac.abort()
    })
  })

  describe('mutations', () => {
    it('AddSource validates the source ref', async () => {
      const resource = new NotebookResource('key', undefined)
      await expect(resource.AddSource({ source: undefined })).rejects.toThrow(
        'source ref is required',
      )
    })

    it('AddSource appends a source and commits the transaction', async () => {
      const notebookData = makeNotebookBytes('Notebook', [
        NotebookSource.create({ name: 'Docs', ref: 'fs/-/docs' }),
      ])
      const readCursor = createMockCursor(notebookData)
      const blockCursor = {
        setBlock: vi.fn(
          (_req: { data: Uint8Array; markDirty?: boolean }, _signal?: AbortSignal) =>
            Promise.resolve(),
        ),
        markDirty: vi.fn(() => Promise.resolve()),
        release: vi.fn(),
      }
      const writtenRootRef = { hash: { hash: new Uint8Array([1, 2, 3]) } }
      const blockTx = {
        write: vi.fn(() => Promise.resolve({ rootRef: writtenRootRef })),
        release: vi.fn(),
      }
      const writeCursor = createMockCursor(undefined, {
        buildTransaction: vi.fn(() =>
          Promise.resolve({ transaction: blockTx, cursor: blockCursor }),
        ),
        getRef: vi.fn(() => Promise.resolve({ ref: undefined })),
      })
      const objectState = createMockObjectState([readCursor, writeCursor])
      const tx = createMockTx(objectState)
      mockEngineInstance = createMockEngine(tx)

      const engineRef = createMockEngineRef()
      const resource = new NotebookResource('key', engineRef as never)
      await resource.AddSource({
        source: { name: 'Archive', ref: 'fs/-/archive' },
      })

      expect(blockCursor.setBlock).toHaveBeenCalledTimes(1)
      const req = blockCursor.setBlock.mock.calls[0]?.[0] as {
        data: Uint8Array
      }
      const nextNotebook = Notebook.fromBinary(req.data)
      expect(nextNotebook.sources).toHaveLength(2)
      expect(nextNotebook.sources?.[1]?.ref).toBe('fs/-/archive')
      expect(blockTx.write).toHaveBeenCalled()
      expect(objectState.setRootRef).toHaveBeenCalledWith(
        { bucketId: '', rootRef: writtenRootRef, transformConf: undefined },
        undefined,
      )
      expect(tx.commit).toHaveBeenCalled()
    })

    it('RemoveSource rejects an out-of-range index', async () => {
      const resource = new NotebookResource('key', undefined)
      await expect(resource.RemoveSource({ index: 0 })).rejects.toThrow(
        'Notebook engine is not available',
      )
    })

    it('ReorderSources rewrites the notebook source order', async () => {
      const notebookData = makeNotebookBytes('Notebook', [
        NotebookSource.create({ name: 'One', ref: 'fs/-/one' }),
        NotebookSource.create({ name: 'Two', ref: 'fs/-/two' }),
      ])
      const readCursor = createMockCursor(notebookData)
      const blockCursor = {
        setBlock: vi.fn(
          (_req: { data: Uint8Array; markDirty?: boolean }, _signal?: AbortSignal) =>
            Promise.resolve(),
        ),
        markDirty: vi.fn(() => Promise.resolve()),
        release: vi.fn(),
      }
      const blockTx = {
        write: vi.fn(() =>
          Promise.resolve({
            rootRef: { hash: { hash: new Uint8Array([1, 2, 3]) } },
          }),
        ),
        release: vi.fn(),
      }
      const writeCursor = createMockCursor(undefined, {
        buildTransaction: vi.fn(() =>
          Promise.resolve({ transaction: blockTx, cursor: blockCursor }),
        ),
        getRef: vi.fn(() => Promise.resolve({ ref: undefined })),
      })
      const objectState = createMockObjectState([readCursor, writeCursor])
      const tx = createMockTx(objectState)
      mockEngineInstance = createMockEngine(tx)

      const engineRef = createMockEngineRef()
      const resource = new NotebookResource('key', engineRef as never)
      await resource.ReorderSources({ order: [1, 0] })

      const req = blockCursor.setBlock.mock.calls[0]?.[0] as {
        data: Uint8Array
      }
      const nextNotebook = Notebook.fromBinary(req.data)
      expect(nextNotebook.sources?.[0]?.ref).toBe('fs/-/two')
      expect(nextNotebook.sources?.[1]?.ref).toBe('fs/-/one')
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
