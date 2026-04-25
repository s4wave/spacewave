import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import { OrgResourceServiceClient } from './org_srpc.pb.js'
import type { OrgState, WatchOrgStateResponse } from './org.pb.js'

// OrganizationTypeID is the type identifier for organization objects.
export const OrganizationTypeID = 'spacewave/organization'

// IOrgHandle contains the OrgHandle interface.
export interface IOrgHandle {
  // watchOrgState streams organization state changes.
  watchOrgState(abortSignal?: AbortSignal): AsyncIterable<OrgState | undefined>

  // release releases the resource.
  release(): void

  // Symbol.dispose for using with 'using' statement.
  [Symbol.dispose](): void
}

// OrgHandle represents a handle to an organization resource.
export class OrgHandle extends Resource implements IOrgHandle {
  private service: OrgResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new OrgResourceServiceClient(resourceRef.client)
  }

  // watchOrgState streams organization state changes.
  public async *watchOrgState(
    abortSignal?: AbortSignal,
  ): AsyncIterable<OrgState | undefined> {
    const stream = this.service.WatchOrgState({}, abortSignal)
    for await (const resp of stream as AsyncIterable<WatchOrgStateResponse>) {
      yield resp.state
    }
  }
}
