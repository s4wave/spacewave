import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import {
  Resource,
  type ResourceDebugInfo,
} from '@aptre/bldr-sdk/resource/resource.js'
import {
  SharedObjectResourceService,
  SharedObjectResourceServiceClient,
} from './sobject_srpc.pb.js'
import {
  MountSharedObjectBodyRequest,
  WatchSharedObjectHealthRequest,
  WatchSharedObjectHealthResponse,
} from './sobject.pb.js'
import { MountSharedObjectResponse } from '../session/session.pb.js'

// SharedObject contains state for an object managed by a Session or other parent resource.
//
// The MountSharedObject directive will remain active until this resource is released.
export class SharedObject extends Resource {
  private service: SharedObjectResourceService

  constructor(
    resourceRef: ClientResourceRef,
    public readonly meta: MountSharedObjectResponse,
  ) {
    super(resourceRef)
    this.service = new SharedObjectResourceServiceClient(resourceRef.client)
  }

  // Mounts the body of a shared object
  public async mountSharedObjectBody(
    req?: MountSharedObjectBodyRequest,
    abortSignal?: AbortSignal,
  ): Promise<SharedObjectBody> {
    const resp = await this.service.MountSharedObjectBody(
      req ?? {},
      abortSignal,
    )
    return this.resourceRef.createResource(
      resp.resourceId ?? 0,
      SharedObjectBody,
    )
  }

  // watchSharedObjectHealth streams health for the mounted shared object.
  public watchSharedObjectHealth(
    req?: WatchSharedObjectHealthRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WatchSharedObjectHealthResponse> {
    return this.service.WatchSharedObjectHealth(req ?? {}, abortSignal)
  }

  // getDebugInfo returns debug information for devtools.
  public getDebugInfo(): ResourceDebugInfo {
    const bodyType = this.meta.sharedObjectMeta?.bodyType
    return {
      label: this.meta.sharedObjectId || undefined,
      details: bodyType ? { bodyType } : undefined,
    }
  }
}

// SharedObjectBody represents the mounted body of a shared object.
// The available services on this resource depends on the body_type of the shared object.
//
// The MountSharedObjectBody directive will remain active until this resource is released.
export class SharedObjectBody extends Resource {
  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
  }
}
