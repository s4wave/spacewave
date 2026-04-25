import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import {
  SpaceContentsResourceService,
  SpaceContentsResourceServiceClient,
} from './space_srpc.pb.js'
import {
  WatchSpaceContentsStateRequest,
  SpaceContentsState,
  SetPluginApprovalResponse,
  SetProcessBindingResponse,
} from './space.pb.js'

// SpaceContents provides streaming plugin status for a mounted space.
export class SpaceContents extends Resource {
  private service: SpaceContentsResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new SpaceContentsResourceServiceClient(resourceRef.client)
  }

  // watchState streams the current plugin approval states for the space.
  public watchState(
    req?: WatchSpaceContentsStateRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<SpaceContentsState> {
    return this.service.WatchState(req ?? {}, abortSignal)
  }

  // setPluginApproval sets the approval state for a plugin.
  public async setPluginApproval(
    pluginId: string,
    approved: boolean,
    abortSignal?: AbortSignal,
  ): Promise<SetPluginApprovalResponse> {
    return this.service.SetPluginApproval({ pluginId, approved }, abortSignal)
  }

  // setProcessBinding sets the approval state for a process binding.
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
