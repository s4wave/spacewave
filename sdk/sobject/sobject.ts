import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import {
  Resource,
  type ResourceDebugInfo,
} from '@aptre/bldr-sdk/resource/resource.js'
import type { SharedObjectHealth } from '@s4wave/core/sobject/sobject.pb.js'
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

// SharedObjectHealthError carries backend-owned SharedObject health through SDK
// call sites that already model failed resource mounts as thrown errors.
export class SharedObjectHealthError extends Error {
  public readonly health: SharedObjectHealth

  constructor(health: SharedObjectHealth) {
    super(health.error || 'shared object health closed')
    this.name = 'SharedObjectHealthError'
    this.health = health
  }
}

export function getSharedObjectHealthFromError(
  err: unknown,
): SharedObjectHealth | null {
  if (err instanceof SharedObjectHealthError) {
    return err.health
  }
  return null
}

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
    const result = resp.result
    if (result?.case === 'health') {
      throw new SharedObjectHealthError(result.value)
    }
    if (result?.case !== 'resourceId') {
      throw new Error('mount shared object body response missing result')
    }
    return this.resourceRef.createResource(result.value, SharedObjectBody)
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
