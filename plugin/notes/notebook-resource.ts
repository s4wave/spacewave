import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import type { MessageStream } from 'starpc'

import { Engine } from '@s4wave/sdk/world/engine.js'
import type { IObjectState } from '@s4wave/sdk/world/object-state.js'
import { accessObjectRootWorldState } from '@s4wave/sdk/world/utils.js'
import { watchObjectQuery } from '@s4wave/sdk/sync/object-query.js'
import type { AppRevision } from '@s4wave/sdk/sync/instance.js'

import { Notebook } from './proto/notebook.pb.js'
import type {
  WatchNotebookRequest,
  WatchNotebookResponse,
  AddSourceRequest,
  AddSourceResponse,
  RemoveSourceRequest,
  RemoveSourceResponse,
  ReorderSourcesRequest,
  ReorderSourcesResponse,
  GetSavedViewsAppRequest,
  GetSavedViewsAppResponse,
} from './sdk/notebook.pb.js'
import type { NotebookResourceService } from './sdk/notebook_srpc.pb.js'
import { setObjectBlockData } from './object-block.js'
import { notebookViewsKey } from './saved-views.js'

/** NotebookResource reads and mutates the canonical Notebook block in its World. */
class NotebookResource implements NotebookResourceService {
  private objectKey: string
  private engineRef: ClientResourceRef | undefined

  constructor(
    objectKey: string,
    engineRef: ClientResourceRef | undefined,
    private readonly revision?: Promise<AppRevision>,
  ) {
    this.objectKey = objectKey
    this.engineRef = engineRef
  }

  /** GetSavedViewsApp identifies the exact module to use for an explicit first save. */
  async GetSavedViewsApp(
    _request: GetSavedViewsAppRequest,
    signal?: AbortSignal,
  ): Promise<GetSavedViewsAppResponse> {
    const revision = await this.revision
    signal?.throwIfAborted()
    if (!revision)
      throw new Error('Saved views are not available in this Notebook runtime')
    return { objectKey: notebookViewsKey(this.objectKey), ...revision }
  }

  /** WatchNotebook streams changed Notebook roots from revision-bound snapshots. */
  async *WatchNotebook(
    _request: WatchNotebookRequest,
    abortSignal?: AbortSignal,
  ): MessageStream<WatchNotebookResponse> {
    if (!this.engineRef) {
      return
    }

    // The query owns its Engine reference independently of the Notebook resource.
    try {
      for await (const snapshot of watchObjectQuery(
        new Engine(this.engineRef),
        this.objectKey,
        this.readNotebookObject,
        abortSignal,
      )) {
        yield { notebook: snapshot.value ?? undefined }
      }
    } catch (err) {
      if (abortSignal?.aborted) {
        return
      }
      throw err
    }
  }

  /** mutateNotebook commits a source edit and releases the transaction on every exit. */
  private async mutateNotebook(
    mutate: (notebook: Notebook) => Notebook,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    // Retain the Engine before opening the writer so failures release both owners.
    if (!this.engineRef) {
      throw new Error('Notebook engine is not available')
    }
    using engine = new Engine(
      this.engineRef.createRef(this.engineRef.resourceId),
    )
    const tx = await engine.newTransaction(true, abortSignal)
    try {
      // Keep Notebook protobuf and UnixFS data in their existing storage.
      using objectState = await tx.getObject(this.objectKey, abortSignal)
      if (!objectState) {
        throw new Error('Notebook object was not found')
      }
      const current = await this.readNotebookObject(objectState, abortSignal)
      if (!current) {
        throw new Error('Notebook block was not found')
      }
      const nextData = Notebook.toBinary(mutate(current))
      await setObjectBlockData(objectState, nextData, abortSignal)

      // Commit only after the complete mutation succeeds.
      await tx.commit(abortSignal)
    } finally {
      try {
        await tx.discard()
      } finally {
        tx.release()
      }
    }
  }

  /** readNotebookObject decodes the canonical block from a pinned object snapshot. */
  private async readNotebookObject(
    objectState: IObjectState,
    abortSignal?: AbortSignal,
  ): Promise<Notebook | null> {
    using cursor = await accessObjectRootWorldState(objectState, abortSignal)
    const blockResp = await cursor.getBlock({}, abortSignal)
    if (!blockResp.found || !blockResp.data) {
      return null
    }
    return Notebook.fromBinary(blockResp.data)
  }

  /** AddSource appends a source to the Notebook. */
  async AddSource(
    request: AddSourceRequest,
    abortSignal?: AbortSignal,
  ): Promise<AddSourceResponse> {
    // Validate user input before taking the writer.
    const source = request.source
    const name = source?.name?.trim() ?? ''
    const ref = source?.ref?.trim() ?? ''
    if (!ref) {
      throw new Error('source ref is required')
    }

    // Preserve source order and all unrelated Notebook fields.
    await this.mutateNotebook(
      (notebook) => ({
        ...notebook,
        sources: [...(notebook.sources ?? []), { name, ref }],
      }),
      abortSignal,
    )
    return {}
  }

  /** RemoveSource removes a source by index. */
  async RemoveSource(
    request: RemoveSourceRequest,
    abortSignal?: AbortSignal,
  ): Promise<RemoveSourceResponse> {
    // Reject a stale index within the same transaction that removes its source.
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

  /** ReorderSources accepts a permutation of the current source list. */
  async ReorderSources(
    request: ReorderSourcesRequest,
    abortSignal?: AbortSignal,
  ): Promise<ReorderSourcesResponse> {
    // Validate against the transaction's source list, never a stale viewer copy.
    const order = (request.order ?? []).map(Number)
    await this.mutateNotebook((notebook) => {
      const sources = [...(notebook.sources ?? [])]
      if (order.length !== sources.length) {
        throw new Error('source order length does not match source count')
      }
      // Require every index once before constructing the replacement list.
      const seen = new Set<number>()
      for (const idx of order) {
        if (idx < 0 || idx >= sources.length || seen.has(idx)) {
          throw new Error('source order is invalid')
        }
        seen.add(idx)
      }
      // Persist only the requested order.
      return {
        ...notebook,
        sources: order.map((idx) => sources[idx]),
      }
    }, abortSignal)
    return {}
  }

  /** dispose releases the resource's Engine reference. Active calls retain their own. */
  dispose(): void {
    this.engineRef?.release()
    this.engineRef = undefined
  }
}

export { NotebookResource }
