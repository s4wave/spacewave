import { ObjectRef } from '@go/github.com/s4wave/spacewave/db/bucket/bucket.pb.js'
import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import {
  Resource,
  type ResourceDebugInfo,
} from '@aptre/bldr-sdk/resource/resource.js'
import {
  ObjectStateResourceService,
  ObjectStateResourceServiceClient,
} from './world_srpc.pb.js'
import { BucketLookupCursor } from '../bucket/lookup/lookup.js'
import type {
  GetRootRefResponse,
  SetRootRefResponse,
  ApplyObjectOpResponse,
  IncrementRevResponse,
  WaitRevResponse,
} from './world.pb.js'

// IObjectState contains the object state interface.
// Represents a handle a object in the store.
export interface IObjectState {
  // GetKey returns the key this state object is for.
  getKey(): string

  // GetRootRef returns the root reference.
  // Returns the revision number.
  getRootRef(abortSignal?: AbortSignal): Promise<GetRootRefResponse>

  // SetRootRef changes the root reference of the object.
  // Increments the revision of the object if changed.
  // Returns revision just after the change was applied.
  setRootRef(
    rootRef: ObjectRef,
    abortSignal?: AbortSignal,
  ): Promise<SetRootRefResponse>

  // AccessWorldState builds a bucket lookup cursor with an optional ref.
  // If the ref is empty, will default to the object RootRef.
  // If the ref Bucket ID is empty, uses the same bucket + volume as the world.
  // The lookup cursor will be released after cb returns.
  accessWorldState(
    ref?: ObjectRef,
    abortSignal?: AbortSignal,
  ): Promise<BucketLookupCursor>

  // ApplyObjectOp applies a batch operation at the object level.
  // The handling of the operation is operation-type specific.
  // Returns the revision following the operation execution.
  // If nil is returned for the error, implies success.
  // If sysErr is set, the error is treated as a transient system error.
  // Returns rev, sysErr, err
  applyObjectOp(
    opTypeId: string,
    opData: Uint8Array,
    opSender: string,
    abortSignal?: AbortSignal,
  ): Promise<ApplyObjectOpResponse>

  // IncrementRev increments the revision of the object.
  // Returns revision just after the change was applied.
  incrementRev(abortSignal?: AbortSignal): Promise<IncrementRevResponse>

  // WaitRev waits until the object rev is >= the specified.
  // Returns ErrObjectNotFound if the object is deleted.
  // If ignoreNotFound is set, waits for the object to exist.
  // Returns the new rev.
  waitRev(
    rev: bigint,
    ignoreNotFound?: boolean,
    abortSignal?: AbortSignal,
  ): Promise<WaitRevResponse>

  // release releases the resource if it's a Resource-backed implementation
  release(): void

  // Symbol.dispose for using with 'using' statement
  [Symbol.dispose](): void
}

// ObjectState represents a single object within a World.
// It provides access to the object's key, root reference, and operations.
//
// In the Go implementation (hydra/world/object-state.go), ObjectState provides:
// - GetKey() string
// - GetRootRef(ctx) (*bucket.ObjectRef, uint64, error)
// - SetRootRef(ctx, *bucket.ObjectRef) (uint64, error)
// - AccessWorldState(ctx, *bucket.ObjectRef, callback) error
// - ApplyObjectOp(ctx, Operation, peer.ID) (rev, sysErr, error)
// - IncrementRev(ctx) (uint64, error)
// - WaitRev(ctx, uint64, ignoreNotFound bool) (uint64, error)
//
// This is the RPC-based Resource implementation.
export class ObjectState extends Resource implements IObjectState {
  private service: ObjectStateResourceService
  private objectKey: string

  constructor(resourceRef: ClientResourceRef, meta?: { objectKey?: string }) {
    super(resourceRef)
    this.service = new ObjectStateResourceServiceClient(resourceRef.client)
    this.objectKey = meta?.objectKey ?? ''
  }

  // GetKey returns the key this state object is for.
  // Returns stored metadata without RPC call.
  public getKey(): string {
    return this.objectKey
  }

  // GetRootRef returns the root reference.
  // Returns the revision number.
  public async getRootRef(abortSignal?: AbortSignal) {
    return await this.service.GetRootRef({}, abortSignal)
  }

  // SetRootRef changes the root reference of the object.
  // Increments the revision of the object if changed.
  // Returns revision just after the change was applied.
  public async setRootRef(rootRef: ObjectRef, abortSignal?: AbortSignal) {
    return await this.service.SetRootRef({ rootRef }, abortSignal)
  }

  // AccessWorldState builds a bucket lookup cursor with an optional ref.
  // If the ref is empty, will default to the object RootRef.
  // If the ref Bucket ID is empty, uses the same bucket + volume as the world.
  // The lookup cursor will be released after cb returns.
  public async accessWorldState(
    ref?: ObjectRef,
    abortSignal?: AbortSignal,
  ): Promise<BucketLookupCursor> {
    const response = await this.service.AccessWorldState({ ref }, abortSignal)
    return this.resourceRef.createResource(
      response.resourceId ?? 0,
      BucketLookupCursor,
    )
  }

  // ApplyObjectOp applies a batch operation at the object level.
  // The handling of the operation is operation-type specific.
  // Returns the revision following the operation execution.
  // If nil is returned for the error, implies success.
  // If sysErr is set, the error is treated as a transient system error.
  // Returns rev, sysErr, err
  public async applyObjectOp(
    opTypeId: string,
    opData: Uint8Array,
    opSender: string,
    abortSignal?: AbortSignal,
  ) {
    return await this.service.ApplyObjectOp(
      { opTypeId, opData, opSender },
      abortSignal,
    )
  }

  // IncrementRev increments the revision of the object.
  // Returns revision just after the change was applied.
  public async incrementRev(abortSignal?: AbortSignal) {
    return await this.service.IncrementRev({}, abortSignal)
  }

  // WaitRev waits until the object rev is >= the specified.
  // Returns ErrObjectNotFound if the object is deleted.
  // If ignoreNotFound is set, waits for the object to exist.
  // Returns the new rev.
  public async waitRev(
    rev: bigint,
    ignoreNotFound?: boolean,
    abortSignal?: AbortSignal,
  ) {
    return await this.service.WaitRev({ rev, ignoreNotFound }, abortSignal)
  }

  // getDebugInfo returns debug information for devtools.
  public getDebugInfo(): ResourceDebugInfo {
    return { label: this.objectKey || undefined }
  }
}
