import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import type { MessageStream } from 'starpc'

import { Engine } from '@s4wave/sdk/world/engine.js'
import { throwOperationError } from '@s4wave/sdk/world/errors.js'
import type { IObjectState } from '@s4wave/sdk/world/object-state.js'
import { watchObjectQuery } from '@s4wave/sdk/sync/object-query.js'
import type { AppRevision } from '@s4wave/sdk/sync/instance.js'

import { Notebook, NotebookEditOp } from './proto/notebook.pb.js'
import {
  NotebookEditOutcome,
  type WatchNotebookRequest,
  type WatchNotebookResponse,
  type AddSourceRequest,
  type AddSourceResponse,
  type RemoveSourceRequest,
  type RemoveSourceResponse,
  type ReorderSourcesRequest,
  type ReorderSourcesResponse,
  type GetSavedViewsAppRequest,
  type GetSavedViewsAppResponse,
} from './sdk/notebook.pb.js'
import type { NotebookResourceService } from './sdk/notebook_srpc.pb.js'
import { readObjectBlockData } from './object-block.js'
import { NOTEBOOK_CONFLICT_CODE, NOTEBOOK_EDIT_OP_ID } from './notebook-edit.js'
import { notebookViewsKey } from './saved-views.js'

/** NotebookResource watches the canonical Notebook block and edits it through World ops. */
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

  /**
   * editNotebook applies one edit as a single notes/notebook/edit World op,
   * which reads, checks and writes the Notebook under the World writer. An
   * edit made against sources the Notebook no longer holds comes back as the
   * changed outcome; the caller refreshes from its watch and does not resend.
   */
  private async editNotebook(
    edit: NotebookEditOp['edit'],
    abortSignal?: AbortSignal,
  ): Promise<NotebookEditOutcome> {
    if (!this.engineRef) {
      throw new Error('Notebook engine is not available')
    }
    using engine = new Engine(
      this.engineRef.createRef(this.engineRef.resourceId),
    )
    const response = await engine.executeWorldOp(
      NOTEBOOK_EDIT_OP_ID,
      NotebookEditOp.toBinary({ objectKey: this.objectKey, edit }),
      abortSignal,
    )
    if (response.rejectionCode === NOTEBOOK_CONFLICT_CODE) {
      return NotebookEditOutcome.CHANGED
    }
    throwOperationError(response)
    if (response.sysErr) {
      throw new Error('the notebook edit could not be confirmed')
    }
    return NotebookEditOutcome.APPLIED
  }

  /** readNotebookObject decodes the canonical block from a pinned object snapshot. */
  private async readNotebookObject(
    objectState: IObjectState,
    abortSignal?: AbortSignal,
  ): Promise<Notebook | null> {
    const data = await readObjectBlockData(objectState, abortSignal)
    return data ? Notebook.fromBinary(data) : null
  }

  /** AddSource appends a source to the Notebook. */
  async AddSource(
    request: AddSourceRequest,
    abortSignal?: AbortSignal,
  ): Promise<AddSourceResponse> {
    await this.editNotebook(
      { case: 'add', value: { source: request.source } },
      abortSignal,
    )
    return {}
  }

  /** RemoveSource removes the source at an index while it is the one the caller saw. */
  async RemoveSource(
    request: RemoveSourceRequest,
    abortSignal?: AbortSignal,
  ): Promise<RemoveSourceResponse> {
    const outcome = await this.editNotebook(
      {
        case: 'remove',
        value: { index: request.index, source: request.source },
      },
      abortSignal,
    )
    return { outcome }
  }

  /** ReorderSources permutes the sources while they are the ones the caller saw. */
  async ReorderSources(
    request: ReorderSourcesRequest,
    abortSignal?: AbortSignal,
  ): Promise<ReorderSourcesResponse> {
    const outcome = await this.editNotebook(
      {
        case: 'reorder',
        value: { order: request.order, sources: request.sources },
      },
      abortSignal,
    )
    return { outcome }
  }

  /** dispose releases the resource's Engine reference. Active calls retain their own. */
  dispose(): void {
    this.engineRef?.release()
    this.engineRef = undefined
  }
}

export { NotebookResource }
