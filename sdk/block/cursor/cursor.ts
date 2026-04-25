import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import {
  BlockCursorResourceService,
  BlockCursorResourceServiceClient,
} from './cursor_srpc.pb.js'
import {
  FetchResponse,
  SetBlockRequest,
  FollowRefRequest,
  GetRefResponse,
  IsDirtyResponse,
  GetBlockResponse,
  UnmarshalResponse,
  IsSubBlockResponse,
  FollowSubBlockRequest,
  SetAsSubBlockRequest,
  ClearRefRequest,
  SetRefRequest,
  GetExistingRefRequest,
  GetAllRefsRequest,
  GetAllRefsResponse,
  DetachRequest,
  DetachRecursiveRequest,
  ParentsResponse,
} from './cursor.pb.js'

// BlockCursor provides access to block cursor operations.
export class BlockCursor extends Resource {
  private service: BlockCursorResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new BlockCursorResourceServiceClient(resourceRef.client)
  }

  // fetch fetches the raw block data at the current position.
  public async fetch(abortSignal?: AbortSignal): Promise<FetchResponse> {
    return await this.service.Fetch({}, abortSignal)
  }

  // setBlock sets the block at the current position.
  public async setBlock(
    req: SetBlockRequest,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.SetBlock(req, abortSignal)
  }

  // followRef follows a reference field and returns a new cursor.
  public async followRef(
    req: FollowRefRequest,
    abortSignal?: AbortSignal,
  ): Promise<BlockCursor> {
    const resp = await this.service.FollowRef(req, abortSignal)
    return this.resourceRef.createResource(resp.resourceId ?? 0, BlockCursor)
  }

  // getRef gets the current block reference.
  public async getRef(abortSignal?: AbortSignal): Promise<GetRefResponse> {
    return await this.service.GetRef({}, abortSignal)
  }

  // isDirty checks if the cursor has uncommitted changes.
  public async isDirty(abortSignal?: AbortSignal): Promise<IsDirtyResponse> {
    return await this.service.IsDirty({}, abortSignal)
  }

  // markDirty marks the cursor location dirty for re-writing.
  public async markDirty(abortSignal?: AbortSignal): Promise<void> {
    await this.service.MarkDirty({}, abortSignal)
  }

  // getBlock returns the current loaded block at the position.
  public async getBlock(abortSignal?: AbortSignal): Promise<GetBlockResponse> {
    return await this.service.GetBlock({}, abortSignal)
  }

  // unmarshal fetches and unmarshals the data to a block.
  public async unmarshal(
    abortSignal?: AbortSignal,
  ): Promise<UnmarshalResponse> {
    return await this.service.Unmarshal({}, abortSignal)
  }

  // isSubBlock indicates if the cursor is at a sub-block position.
  public async isSubBlock(
    abortSignal?: AbortSignal,
  ): Promise<IsSubBlockResponse> {
    return await this.service.IsSubBlock({}, abortSignal)
  }

  // followSubBlock follows a sub-block reference and returns a new cursor.
  public async followSubBlock(
    req: FollowSubBlockRequest,
    abortSignal?: AbortSignal,
  ): Promise<BlockCursor> {
    const resp = await this.service.FollowSubBlock(req, abortSignal)
    return this.resourceRef.createResource(resp.resourceId ?? 0, BlockCursor)
  }

  // setAsSubBlock sets the cursor position as a sub-block of another block.
  public async setAsSubBlock(
    req: SetAsSubBlockRequest,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.SetAsSubBlock(req, abortSignal)
  }

  // clearRef removes the reference handle to the given ref ID.
  public async clearRef(
    req: ClearRefRequest,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.ClearRef(req, abortSignal)
  }

  // clearAllRefs clears all references.
  public async clearAllRefs(abortSignal?: AbortSignal): Promise<void> {
    await this.service.ClearAllRefs({}, abortSignal)
  }

  // setRef sets a block reference to the handle at the cursor.
  public async setRef(
    req: SetRefRequest,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.SetRef(req, abortSignal)
  }

  // getExistingRef checks if the reference has been traversed already.
  public async getExistingRef(
    req: GetExistingRefRequest,
    abortSignal?: AbortSignal,
  ): Promise<BlockCursor | null> {
    const resp = await this.service.GetExistingRef(req, abortSignal)
    if (!resp.resourceId) return null
    return this.resourceRef.createResource(resp.resourceId, BlockCursor)
  }

  // getAllRefs returns cursors to all references.
  public async getAllRefs(
    req: GetAllRefsRequest,
    abortSignal?: AbortSignal,
  ): Promise<GetAllRefsResponse> {
    return await this.service.GetAllRefs(req, abortSignal)
  }

  // detach clones the cursor position.
  public async detach(
    req: DetachRequest,
    abortSignal?: AbortSignal,
  ): Promise<BlockCursor> {
    const resp = await this.service.Detach(req, abortSignal)
    return this.resourceRef.createResource(resp.resourceId ?? 0, BlockCursor)
  }

  // detachTransaction creates a new ephemeral transaction rooted at the cursor.
  public async detachTransaction(
    abortSignal?: AbortSignal,
  ): Promise<BlockCursor> {
    const resp = await this.service.DetachTransaction({}, abortSignal)
    return this.resourceRef.createResource(resp.resourceId ?? 0, BlockCursor)
  }

  // detachRecursive clones the cursor position and all referenced positions.
  public async detachRecursive(
    req: DetachRecursiveRequest,
    abortSignal?: AbortSignal,
  ): Promise<BlockCursor> {
    const resp = await this.service.DetachRecursive(req, abortSignal)
    return this.resourceRef.createResource(resp.resourceId ?? 0, BlockCursor)
  }

  // parents returns new cursors pointing to the parent blocks.
  public async parents(abortSignal?: AbortSignal): Promise<ParentsResponse> {
    return await this.service.Parents({}, abortSignal)
  }
}
