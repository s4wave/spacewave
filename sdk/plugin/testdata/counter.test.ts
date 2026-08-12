import { describe, expect, it } from 'vitest'

import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { CounterResourceServiceDefinition } from './counter_srpc.pb.js'
import {
  GetCounterResponse,
  InitializeRequest,
  InitializeResponse,
} from './counter.pb.js'
import { Counter } from './counter.js'

class CounterRef implements ClientResourceRef {
  readonly resourceId = 1
  readonly released = false
  value: bigint | undefined
  readonly client = {
    request: (
      _service: string,
      method: string,
      data: Uint8Array,
    ): Promise<Uint8Array> => {
      if (method === CounterResourceServiceDefinition.methods.Initialize.name) {
        if (this.value !== undefined) {
          return Promise.reject(new Error('counter is already initialized'))
        }
        this.value = InitializeRequest.fromBinary(data).value ?? 0n
        return Promise.resolve(InitializeResponse.toBinary({}))
      }
      if (method === CounterResourceServiceDefinition.methods.GetCounter.name) {
        if (this.value === undefined) {
          return Promise.reject(new Error('counter is not initialized'))
        }
        return Promise.resolve(
          GetCounterResponse.toBinary({ value: this.value }),
        )
      }
      return Promise.reject(new Error('unknown counter method: ' + method))
    },
  } as ClientResourceRef['client']

  createRef(): ClientResourceRef {
    throw new Error('not used')
  }

  createResource<T>(): T {
    throw new Error('not used')
  }

  release(): void {}

  [Symbol.dispose](): void {}
}

describe('Counter Resource', () => {
  it('initializes once and reads the same generated-service value back', async () => {
    const ref = new CounterRef()
    const counter = new Counter(ref)

    await counter.initialize(7n)
    await expect(counter.getCounter()).resolves.toBe(7n)
    await expect(counter.initialize(8n)).rejects.toThrow(
      'counter is already initialized',
    )
  })
})
