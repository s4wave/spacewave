import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import { DocsResourceServiceClient } from './docs_srpc.pb.js'
import type { Documentation } from '../proto/docs.pb.js'
import type { WatchDocsResponse } from './docs.pb.js'

// DocsTypeID is the type identifier for documentation objects.
export const DocsTypeID = 'spacewave-notes/docs'

// IDocsHandle contains the DocsHandle interface.
export interface IDocsHandle {
  // watchDocs streams the current documentation state.
  watchDocs(abortSignal?: AbortSignal): AsyncIterable<Documentation>

  // release releases the resource.
  release(): void

  // Symbol.dispose for using with 'using' statement.
  [Symbol.dispose](): void
}

// DocsHandle represents a handle to a documentation resource.
// Each instance maps 1:1 to a DocsResource on the backend.
export class DocsHandle extends Resource implements IDocsHandle {
  private service: DocsResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new DocsResourceServiceClient(resourceRef.client)
  }

  // watchDocs streams the current documentation state.
  public async *watchDocs(
    abortSignal?: AbortSignal,
  ): AsyncIterable<Documentation> {
    const stream = this.service.WatchDocs({}, abortSignal)
    for await (const resp of stream as AsyncIterable<WatchDocsResponse>) {
      if (resp.documentation) {
        yield resp.documentation
      }
    }
  }
}
