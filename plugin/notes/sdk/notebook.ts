import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import { NotebookResourceServiceClient } from './notebook_srpc.pb.js'
import type { Notebook, NotebookSource } from '../proto/notebook.pb.js'
import type {
  GetSavedViewsAppResponse,
  NotebookEditOutcome,
  WatchNotebookResponse,
} from './notebook.pb.js'

// NotebookTypeID is the type identifier for notebook objects.
export const NotebookTypeID = 'notes/notebook'

// INotebookHandle contains the NotebookHandle interface.
export interface INotebookHandle {
  /** getSavedViewsApp identifies the immutable module for an explicit first save. */
  getSavedViewsApp(signal?: AbortSignal): Promise<GetSavedViewsAppResponse>

  // watchNotebook streams the current notebook state.
  watchNotebook(abortSignal?: AbortSignal): AsyncIterable<Notebook>

  // addSource adds a source to the notebook.
  addSource(source: NotebookSource, abortSignal?: AbortSignal): Promise<void>

  // removeSource removes the source at index while it is still source.
  removeSource(
    index: number,
    source: NotebookSource,
    abortSignal?: AbortSignal,
  ): Promise<NotebookEditOutcome>

  // reorderSources reorders sources by order while the list is still sources.
  reorderSources(
    order: number[],
    sources: NotebookSource[],
    abortSignal?: AbortSignal,
  ): Promise<NotebookEditOutcome>

  // release releases the resource.
  release(): void

  // Symbol.dispose for using with 'using' statement.
  [Symbol.dispose](): void
}

// NotebookHandle represents a handle to a notebook resource.
// Each instance maps 1:1 to a Go NotebookResource on the backend.
export class NotebookHandle extends Resource implements INotebookHandle {
  private service: NotebookResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new NotebookResourceServiceClient(resourceRef.client)
  }

  /** getSavedViewsApp reads dataset metadata without creating shared state. */
  public getSavedViewsApp(
    signal?: AbortSignal,
  ): Promise<GetSavedViewsAppResponse> {
    return this.service.GetSavedViewsApp({}, signal)
  }

  // watchNotebook streams the current notebook state.
  public async *watchNotebook(
    abortSignal?: AbortSignal,
  ): AsyncIterable<Notebook> {
    const stream = this.service.WatchNotebook({}, abortSignal)
    for await (const resp of stream as AsyncIterable<WatchNotebookResponse>) {
      if (resp.notebook) {
        yield resp.notebook
      }
    }
  }

  // addSource adds a source to the notebook.
  public async addSource(
    source: NotebookSource,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.AddSource({ source }, abortSignal)
  }

  // removeSource removes the source at index while it is still source.
  public async removeSource(
    index: number,
    source: NotebookSource,
    abortSignal?: AbortSignal,
  ): Promise<NotebookEditOutcome> {
    const resp = await this.service.RemoveSource({ index, source }, abortSignal)
    return resp.outcome ?? 0
  }

  // reorderSources reorders sources by order while the list is still sources.
  public async reorderSources(
    order: number[],
    sources: NotebookSource[],
    abortSignal?: AbortSignal,
  ): Promise<NotebookEditOutcome> {
    const resp = await this.service.ReorderSources(
      { order, sources },
      abortSignal,
    )
    return resp.outcome ?? 0
  }
}
