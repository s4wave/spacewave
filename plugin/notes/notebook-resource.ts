import { Engine } from '@s4wave/sdk/world/engine.js'
import { Notebook } from './proto/notebook.pb.js'
import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import type { MessageStream } from 'starpc'
import type {
  WatchNotebookRequest,
  WatchNotebookResponse,
  AddSourceRequest,
  AddSourceResponse,
  RemoveSourceRequest,
  RemoveSourceResponse,
  ReorderSourcesRequest,
  ReorderSourcesResponse,
} from './sdk/notebook.pb.js'
import type { NotebookResourceService } from './sdk/notebook_srpc.pb.js'
import { setObjectBlockData } from './object-block.js'

// NotebookResource serves NotebookResourceService for a single notebook
// world object. Reads the Notebook block from the World via the Engine SDK.
class NotebookResource implements NotebookResourceService {
  private objectKey: string
  private engineRef: ClientResourceRef | undefined

  constructor(objectKey: string, engineRef: ClientResourceRef | undefined) {
    this.objectKey = objectKey
    this.engineRef = engineRef
  }

  // WatchNotebook streams the Notebook block, re-reading on every world change.
  // Uses engine.waitSeqno to block until the world state advances, then re-reads
  // the Notebook block via a fresh read-only transaction.
  async *WatchNotebook(
    _request: WatchNotebookRequest,
    abortSignal?: AbortSignal,
  ): MessageStream<WatchNotebookResponse> {
    if (!this.engineRef) {
      return
    }

    const engine = new Engine(this.engineRef)
    try {
      let lastSeqno = 0n
      for (;;) {
        if (abortSignal?.aborted) return

        const notebook = await this.readNotebookBlock(engine, abortSignal)
        if (notebook) {
          yield { notebook }
        }

        // Get the seqno of the state we just read.
        const resp = await engine.getSeqno(abortSignal)
        lastSeqno = resp.seqno ?? 0n

        // Block until the world state advances past the snapshot we read.
        await engine.waitSeqno(lastSeqno + 1n, abortSignal)
      }
    } catch (err) {
      if (abortSignal?.aborted) return
      throw err
    } finally {
      engine.release()
    }
  }

  // readNotebookBlock reads the Notebook block from the world via a
  // short-lived read-only transaction.
  private async readNotebookBlock(
    engine: Engine,
    abortSignal?: AbortSignal,
  ): Promise<Notebook | null> {
    const tx = await engine.newTransaction(false, abortSignal)
    try {
      const objectState = await tx.getObject(this.objectKey, abortSignal)
      if (!objectState) return null
      try {
        const cursor = await objectState.accessWorldState(
          undefined,
          abortSignal,
        )
        try {
          const blockResp = await cursor.getBlock({}, abortSignal)
          if (!blockResp.found || !blockResp.data) return null
          return Notebook.fromBinary(blockResp.data)
        } finally {
          cursor.release()
        }
      } finally {
        objectState.release()
      }
    } finally {
      tx.release()
    }
  }

  // mutateNotebook applies a notebook mutation and persists the updated block.
  private async mutateNotebook(
    mutate: (notebook: Notebook) => Notebook,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    if (!this.engineRef) {
      throw new Error('Notebook engine is not available')
    }

    const engine = new Engine(this.engineRef)
    let txCommitted = false
    const tx = await engine.newTransaction(true, abortSignal)
    try {
      const objectState = await tx.getObject(this.objectKey, abortSignal)
      if (!objectState) {
        throw new Error('Notebook object was not found')
      }
      try {
        const current = await this.readNotebookObject(objectState, abortSignal)
        if (!current) {
          throw new Error('Notebook block was not found')
        }

        const nextNotebook = mutate(current)
        const nextData = Notebook.toBinary(nextNotebook)
        await setObjectBlockData(objectState, nextData, abortSignal)
      } finally {
        objectState.release()
      }

      await tx.commit(abortSignal)
      txCommitted = true
    } finally {
      if (!txCommitted) {
        await tx.discard(abortSignal).catch(() => {})
      }
      tx.release()
      engine.release()
    }
  }

  // readNotebookObject reads the notebook block through an object state handle.
  private async readNotebookObject(
    objectState: {
      accessWorldState(
        ref?: undefined,
        abortSignal?: AbortSignal,
      ): Promise<{
        getBlock(
          req: {},
          abortSignal?: AbortSignal,
        ): Promise<{ found?: boolean; data?: Uint8Array }>
        release(): void
      }>
    },
    abortSignal?: AbortSignal,
  ): Promise<Notebook | null> {
    const cursor = await objectState.accessWorldState(undefined, abortSignal)
    try {
      const blockResp = await cursor.getBlock({}, abortSignal)
      if (!blockResp.found || !blockResp.data) return null
      return Notebook.fromBinary(blockResp.data)
    } finally {
      cursor.release()
    }
  }

  // AddSource appends a source to the notebook.
  async AddSource(
    request: AddSourceRequest,
    abortSignal?: AbortSignal,
  ): Promise<AddSourceResponse> {
    const source = request.source
    const name = source?.name?.trim() ?? ''
    const ref = source?.ref?.trim() ?? ''
    if (!ref) {
      throw new Error('source ref is required')
    }

    await this.mutateNotebook((notebook) => ({
      ...notebook,
      sources: [...(notebook.sources ?? []), { name, ref }],
    }), abortSignal)
    return {}
  }

  // RemoveSource removes a source by index.
  async RemoveSource(
    request: RemoveSourceRequest,
    abortSignal?: AbortSignal,
  ): Promise<RemoveSourceResponse> {
    const index = Number(request.index ?? 0)
    await this.mutateNotebook((notebook) => {
      const sources = [...(notebook.sources ?? [])]
      if (index < 0 || index >= sources.length) {
        throw new Error('source index is out of range')
      }
      sources.splice(index, 1)
      return { ...notebook, sources }
    }, abortSignal)
    return {}
  }

  // ReorderSources reorders the source list.
  async ReorderSources(
    request: ReorderSourcesRequest,
    abortSignal?: AbortSignal,
  ): Promise<ReorderSourcesResponse> {
    const order = (request.order ?? []).map(Number)
    await this.mutateNotebook((notebook) => {
      const sources = [...(notebook.sources ?? [])]
      if (order.length !== sources.length) {
        throw new Error('source order length does not match source count')
      }
      const seen = new Set<number>()
      for (const idx of order) {
        if (idx < 0 || idx >= sources.length || seen.has(idx)) {
          throw new Error('source order is invalid')
        }
        seen.add(idx)
      }
      return {
        ...notebook,
        sources: order.map((idx) => sources[idx]),
      }
    }, abortSignal)
    return {}
  }

  // dispose releases the engine ref if still held.
  dispose(): void {
    this.engineRef?.release()
    this.engineRef = undefined
  }
}

export { NotebookResource }
