import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import {
  BucketLookupCursorResourceService,
  BucketLookupCursorResourceServiceClient,
} from './lookup_srpc.pb.js'
import {
  GetRefResponse,
  FollowRefRequest,
  GetBlockRequest,
  GetBlockResponse,
  PutBlockRequest,
  PutBlockResponse,
  PutBlockBatchRequest,
  PutBlockBatchResponse,
  GetBlockExistsBatchRequest,
  GetBlockExistsBatchResponse,
  BuildTransactionRequest,
  BuildTransactionAtRefRequest,
  CloneRequest,
  UnmarshalRequest,
  UnmarshalResponse,
} from './lookup.pb.js'
import type { BlockRef } from '../../../db/block/block.pb.js'
import { BlockCursor } from '../../block/cursor/cursor.js'
import { BlockTransaction } from '../../block/transaction/transaction.js'

function hasBlockRef(ref: BlockRef | undefined): boolean {
  return !!ref?.hash?.hash?.length
}

// BucketLookupCursor provides access to bucket lookup operations.
export class BucketLookupCursor extends Resource {
  private service: BucketLookupCursorResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new BucketLookupCursorResourceServiceClient(
      resourceRef.client,
    )
  }

  // getRef returns the current object reference.
  public async getRef(abortSignal?: AbortSignal): Promise<GetRefResponse> {
    return await this.service.GetRef({}, abortSignal)
  }

  // followRef follows an object reference and returns a new cursor.
  public async followRef(
    req: FollowRefRequest,
    abortSignal?: AbortSignal,
  ): Promise<BucketLookupCursor> {
    const resp = await this.service.FollowRef(req, abortSignal)
    return this.resourceRef.createResource(
      resp.resourceId ?? 0,
      BucketLookupCursor,
    )
  }

  // getBlock gets a block by reference.
  public async getBlock(
    req: GetBlockRequest,
    abortSignal?: AbortSignal,
  ): Promise<GetBlockResponse> {
    if (!hasBlockRef(req.ref)) {
      const refResp = await this.getRef(abortSignal)
      return await this.service.GetBlock(
        { ...req, ref: refResp.ref?.rootRef },
        abortSignal,
      )
    }
    return await this.service.GetBlock(req, abortSignal)
  }

  // putBlock puts a block.
  public async putBlock(
    req: PutBlockRequest,
    abortSignal?: AbortSignal,
  ): Promise<PutBlockResponse> {
    return await this.service.PutBlock(req, abortSignal)
  }

  // putBlockBatch puts a batch of blocks.
  public async putBlockBatch(
    req: PutBlockBatchRequest,
    abortSignal?: AbortSignal,
  ): Promise<PutBlockBatchResponse> {
    return await this.service.PutBlockBatch(req, abortSignal)
  }

  // getBlockExistsBatch checks whether each block exists.
  public async getBlockExistsBatch(
    req: GetBlockExistsBatchRequest,
    abortSignal?: AbortSignal,
  ): Promise<GetBlockExistsBatchResponse> {
    return await this.service.GetBlockExistsBatch(req, abortSignal)
  }

  // buildTransaction builds a transaction at the current position.
  public async buildTransaction(
    req: BuildTransactionRequest,
    abortSignal?: AbortSignal,
  ): Promise<{ transaction: BlockTransaction; cursor: BlockCursor }> {
    const resp = await this.service.BuildTransaction(req, abortSignal)
    return {
      transaction: this.resourceRef.createResource(
        resp.transactionResourceId ?? 0,
        BlockTransaction,
      ),
      cursor: this.resourceRef.createResource(
        resp.cursorResourceId ?? 0,
        BlockCursor,
      ),
    }
  }

  // buildTransactionAtRef builds a transaction at a specific block reference.
  public async buildTransactionAtRef(
    req: BuildTransactionAtRefRequest,
    abortSignal?: AbortSignal,
  ): Promise<{ transaction: BlockTransaction; cursor: BlockCursor }> {
    const resp = await this.service.BuildTransactionAtRef(req, abortSignal)
    return {
      transaction: this.resourceRef.createResource(
        resp.transactionResourceId ?? 0,
        BlockTransaction,
      ),
      cursor: this.resourceRef.createResource(
        resp.cursorResourceId ?? 0,
        BlockCursor,
      ),
    }
  }

  // clone clones the cursor.
  public async clone(
    req: CloneRequest,
    abortSignal?: AbortSignal,
  ): Promise<BucketLookupCursor> {
    const resp = await this.service.Clone(req, abortSignal)
    return this.resourceRef.createResource(
      resp.resourceId ?? 0,
      BucketLookupCursor,
    )
  }

  // release releases the cursor resources.
  public release(abortSignal?: AbortSignal): void {
    // Best-effort notify the server; don't await (Resource.release must be synchronous)
    void this.service.Release({}, abortSignal).catch(() => {})
    super.release()
  }

  // unmarshal fetches and unmarshals a block at the given reference.
  public async unmarshal(
    req: UnmarshalRequest,
    abortSignal?: AbortSignal,
  ): Promise<UnmarshalResponse> {
    return await this.service.Unmarshal(req, abortSignal)
  }
}
