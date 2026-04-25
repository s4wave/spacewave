import { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import { BlockCursor } from '../cursor/cursor.js'
import {
  BlockTransactionResourceService,
  BlockTransactionResourceServiceClient,
} from './transaction_srpc.pb.js'
import { WriteRequest, WriteResponse } from './transaction.pb.js'

// BlockTransaction provides access to block transaction operations.
export class BlockTransaction extends Resource {
  private service: BlockTransactionResourceService

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new BlockTransactionResourceServiceClient(resourceRef.client)
  }

  // write writes the transaction to storage and returns the root reference.
  public async write(
    req: WriteRequest,
    abortSignal?: AbortSignal,
  ): Promise<WriteResponse> {
    return await this.service.Write(req, abortSignal)
  }

  // getRootCursor returns the root cursor of the transaction.
  public async getRootCursor(abortSignal?: AbortSignal): Promise<BlockCursor> {
    const resp = await this.service.GetRootCursor({}, abortSignal)
    return this.resourceRef.createResource(resp.resourceId ?? 0, BlockCursor)
  }
}
