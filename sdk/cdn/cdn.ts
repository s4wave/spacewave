import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'

import { Space } from '../space/space.js'
import {
  CdnResourceService,
  CdnResourceServiceClient,
} from './cdn-resource_srpc.pb.js'

// Cdn wraps a server-side CdnResource. Obtain via root.getCdn(cdnId). Each
// instance is scoped to one CDN (identified by cdn_id in the enclosing
// GetCdn call; empty cdn_id selects the default CDN). RPC methods on the
// resource do not re-take cdn_id because it is implicit in the bound
// resource.
export class Cdn extends Resource {
  private service: CdnResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new CdnResourceServiceClient(resourceRef.client)
  }

  // getCdnSpaceId returns the Space ULID for this CDN instance.
  public async getCdnSpaceId(abortSignal?: AbortSignal): Promise<string> {
    const resp = await this.service.GetCdnSpaceId({}, abortSignal)
    return resp.spaceId ?? ''
  }

  // mountCdnSpace mounts the CDN SharedObject as a read-only Space resource.
  // The returned Space exposes the standard WorldStateResource surface
  // (listObjectsWithType, getObject, lookupGraphQuads) against the CDN world.
  public async mountCdnSpace(abortSignal?: AbortSignal): Promise<Space> {
    const resp = await this.service.MountCdnSpace({}, abortSignal)
    return this.resourceRef.createResource(resp.resourceId ?? 0, Space)
  }

  // copyVmImageToSpace copies a VmImage (metadata block plus the five asset
  // edges) from this CDN Space into a user-owned destination Space. The
  // caller supplies session_idx for the session-aware destination resolve
  // plus the destination space ULID and source/destination object keys.
  // Asset UnixFS blocks are content-addressed; the destination block store
  // dedupes them against the CDN block store without re-upload.
  public async copyVmImageToSpace(
    sessionIdx: number,
    dstSpaceId: string,
    srcObjectKey: string,
    dstObjectKey: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.CopyVmImageToSpace(
      { sessionIdx, dstSpaceId, srcObjectKey, dstObjectKey },
      abortSignal,
    )
  }
}
