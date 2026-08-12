import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'

import { CounterResourceServiceClient } from './counter_srpc.pb.js'

// CounterTypeID is the custom ObjectType used by the TypeScript plugin fixture.
export const CounterTypeID = 'example/counter'

// Counter is the generated-service Resource wrapper for the counter fixture.
export class Counter extends Resource {
  private readonly service: CounterResourceServiceClient

  constructor(ref: ClientResourceRef) {
    super(ref)
    this.service = new CounterResourceServiceClient(ref.client)
  }

  // initialize stores the counter's first value.
  async initialize(value: bigint, signal?: AbortSignal): Promise<void> {
    await this.service.Initialize({ value }, signal)
  }

  // getCounter returns the stored counter value.
  async getCounter(signal?: AbortSignal): Promise<bigint> {
    const response = await this.service.GetCounter({}, signal)
    return response.value ?? 0n
  }
}
