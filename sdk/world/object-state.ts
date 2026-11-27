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
import { throwOperationError } from './errors.js'
import { KNOWN_ERROR_CODE } from './known-error-code.js'
import type {
  GetRootRefResponse,
  SetRootRefResponse,
  ApplyObjectOpResponse,
  IncrementRevResponse,
  WaitRevResponse,
} from './world.pb.js'

// IObjectState is the object state interface.
// The interface represents a handle to an object in the store.
export interface IObjectState {
  // getKey returns the key this state object is for.
  getKey(): string

  // getRootRef returns the root reference.
  // getRootRef returns the revision number.
  getRootRef(abortSignal?: AbortSignal): Promise<GetRootRefResponse>

  // setRootRef changes the root reference of the object.
  // setRootRef increments the revision of the object if the root ref changed.
  // setRootRef returns the revision just after the change was applied.
  setRootRef(
    rootRef: ObjectRef,
    abortSignal?: AbortSignal,
  ): Promise<SetRootRefResponse>

  // accessWorldState builds a bucket lookup cursor with an optional ref.
  // If the ref is empty, accessWorldState defaults to the object RootRef.
  // If the ref Bucket ID is empty, accessWorldState uses the same bucket +
  // volume as the world.
  // The lookup cursor will be released after cb returns.
  accessWorldState(
    ref?: ObjectRef,
    abortSignal?: AbortSignal,
  ): Promise<BucketLookupCursor>

  // applyObjectOp applies a batch operation at the object level.
  // The operation is handled according to its type.
  // applyObjectOp returns the revision after the operation executes.
  // applyObjectOp throws WorldOperationRejection when the operation refuses
  // the change.
  // If sysErr is set, the error is treated as a transient system error.
  applyObjectOp(
    opTypeId: string,
    opData: Uint8Array,
    opSender: string,
    abortSignal?: AbortSignal,
  ): Promise<ApplyObjectOpResponse>

  // incrementRev increments the revision of the object.
  // incrementRev returns the revision just after the change was applied.
  incrementRev(abortSignal?: AbortSignal): Promise<IncrementRevResponse>

  // waitRev waits until the object rev is >= the specified revision.
  // waitRev returns ErrObjectNotFound if the object is deleted.
  // If ignoreNotFound is set, waitRev waits for the object to exist.
  // waitRev returns the new rev.
  waitRev(
    rev: bigint,
    ignoreNotFound?: boolean,
    abortSignal?: AbortSignal,
  ): Promise<WaitRevResponse>

  // release releases the resource if it's a Resource-backed implementation.
  release(): void

  // Symbol.dispose supports use with the 'using' statement.
  [Symbol.dispose](): void
}

// ObjectState represents a single object within a World.
// It provides access to the object's key, root reference, and operations.
//
// Provides:
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

  // getKey returns the key this state object is for.
  // getKey returns stored metadata without an RPC call.
  public getKey(): string {
    return this.objectKey
  }

  // getRootRef returns the root reference.
  // getRootRef returns the revision number.
  public async getRootRef(abortSignal?: AbortSignal) {
    return await this.service.GetRootRef({}, abortSignal)
  }

  // setRootRef changes the root reference of the object.
  // setRootRef increments the revision of the object if the root ref changed.
  // setRootRef returns the revision just after the change was applied.
  public async setRootRef(rootRef: ObjectRef, abortSignal?: AbortSignal) {
    const response = await this.service.SetRootRef(
      { rootRef, knownErrorCode: KNOWN_ERROR_CODE },
      abortSignal,
    )
    throwOperationError(response)
    return response
  }

  // accessWorldState builds a bucket lookup cursor with an optional ref.
  // If the ref is empty, accessWorldState defaults to the object RootRef.
  // If the ref Bucket ID is empty, accessWorldState uses the same bucket +
  // volume as the world.
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

  // applyObjectOp applies a batch operation at the object level.
  // The operation is handled according to its type.
  // applyObjectOp returns the revision after the operation executes.
  // applyObjectOp throws WorldOperationRejection when the operation refuses
  // the change.
  // If sysErr is set, the error is treated as a transient system error.
  public async applyObjectOp(
    opTypeId: string,
    opData: Uint8Array,
    opSender: string,
    abortSignal?: AbortSignal,
  ): Promise<ApplyObjectOpResponse> {
    const response = await this.service.ApplyObjectOp(
      { opTypeId, opData, opSender, knownErrorCode: KNOWN_ERROR_CODE },
      abortSignal,
    )
    throwOperationError(response)
    return response
  }

  // incrementRev increments the revision of the object.
  // incrementRev returns the revision just after the change was applied.
  public async incrementRev(abortSignal?: AbortSignal) {
    const response = await this.service.IncrementRev(
      { knownErrorCode: KNOWN_ERROR_CODE },
      abortSignal,
    )
    throwOperationError(response)
    return response
  }

  // waitRev waits until the object rev is >= the specified revision.
  // waitRev returns ErrObjectNotFound if the object is deleted.
  // If ignoreNotFound is set, waitRev waits for the object to exist.
  // waitRev returns the new rev.
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
