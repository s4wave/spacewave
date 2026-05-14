import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import {
  SpaceContentsResourceService,
  SpaceContentsResourceServiceClient,
} from './space_srpc.pb.js'
import {
  WatchSpaceContentsStateRequest,
  SpaceContentsState,
  SetProcessBindingResponse,
} from './space.pb.js'

// SpaceContents provides streaming plugin status for a mounted space.
export class SpaceContents extends Resource {
  private service: SpaceContentsResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new SpaceContentsResourceServiceClient(resourceRef.client)
  }

  // watchState streams the current plugin and process-binding state for the space.
  public watchState(
    req?: WatchSpaceContentsStateRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<SpaceContentsState> {
    return this.service.WatchState(req ?? {}, abortSignal)
  }

  // setProcessBinding sets the state for a process binding.
  public async setProcessBinding(
    objectKey: string,
    typeId: string,
    approved: boolean,
    abortSignal?: AbortSignal,
  ): Promise<SetProcessBindingResponse> {
    return this.service.SetProcessBinding(
      { objectKey, typeId, approved },
      abortSignal,
    )
  }
}
