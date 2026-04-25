import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import { NotebookResourceServiceClient } from './notebook_srpc.pb.js'
import type {
  Notebook,
  NotebookSource,
} from '../proto/notebook.pb.js'
import type { WatchNotebookResponse } from './notebook.pb.js'

// NotebookTypeID is the type identifier for notebook objects.
export const NotebookTypeID = 'spacewave-notes/notebook'

// INotebookHandle contains the NotebookHandle interface.
export interface INotebookHandle {
  // watchNotebook streams the current notebook state.
  watchNotebook(abortSignal?: AbortSignal): AsyncIterable<Notebook>

  // addSource adds a source to the notebook.
  addSource(source: NotebookSource, abortSignal?: AbortSignal): Promise<void>

  // removeSource removes a source by index.
  removeSource(index: number, abortSignal?: AbortSignal): Promise<void>

  // reorderSources reorders the source list.
  reorderSources(order: number[], abortSignal?: AbortSignal): Promise<void>

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

  // removeSource removes a source by index.
  public async removeSource(
    index: number,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.RemoveSource({ index }, abortSignal)
  }

  // reorderSources reorders the source list.
  public async reorderSources(
    order: number[],
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.ReorderSources({ order }, abortSignal)
  }
}
