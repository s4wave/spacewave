import { accessObjectRootWorldState } from '@s4wave/sdk/world/utils.js'
import type { Engine } from '@s4wave/sdk/world/engine.js'

import {
  CounterState,
  type GetCounterRequest,
  type GetCounterResponse,
  type InitializeRequest,
  type InitializeResponse,
} from './counter.pb.js'
import type { CounterResourceServiceHandler } from './counter_srpc.pb.js'

// CounterHandler persists one counter value in its World object block.
export class CounterHandler implements CounterResourceServiceHandler {
  constructor(
    private readonly engine: Engine,
    private readonly objectKey: string,
  ) {}

  async Initialize(
    request: InitializeRequest,
    signal: AbortSignal,
  ): Promise<InitializeResponse> {
    using tx = await this.engine.newTransaction(true, signal)
    try {
      using object = await tx.getObject(this.objectKey, signal)
      if (!object) throw new Error('counter object not found')
      using cursor = await accessObjectRootWorldState(object, signal)
      const current = await cursor.getBlock({}, signal)
      if (current.found && current.data?.length) {
        throw new Error('counter is already initialized')
      }
      const { transaction, cursor: blockCursor } =
        await cursor.buildTransaction({}, signal)
      try {
        await blockCursor.markDirty(signal)
        await blockCursor.setBlock(
          {
            data: CounterState.toBinary({ value: request.value }),
            markDirty: true,
          },
          signal,
        )
        const currentRef = (await cursor.getRef(signal)).ref
        const written = await transaction.write({ clearTree: true }, signal)
        if (!written.rootRef) throw new Error('counter write returned no root')
        await object.setRootRef(
          {
            bucketId: currentRef?.bucketId ?? '',
            rootRef: written.rootRef,
            transformConf: currentRef?.transformConf,
          },
          signal,
        )
      } finally {
        blockCursor.release()
        transaction.release()
      }
      await tx.commit(signal)
      return {}
    } finally {
      await tx.discard(signal)
    }
  }

  async GetCounter(
    _request: GetCounterRequest,
    signal: AbortSignal,
  ): Promise<GetCounterResponse> {
    using tx = await this.engine.newTransaction(false, signal)
    using object = await tx.getObject(this.objectKey, signal)
    if (!object) throw new Error('counter object not found')
    using cursor = await accessObjectRootWorldState(object, signal)
    const block = await cursor.getBlock({}, signal)
    if (!block.found || !block.data?.length) {
      throw new Error('counter is not initialized')
    }
    return { value: CounterState.fromBinary(block.data).value }
  }
}
