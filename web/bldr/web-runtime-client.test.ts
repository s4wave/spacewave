import { afterEach, describe, expect, it, vi } from 'vitest'

import { WebRuntimeClientType } from '../runtime/runtime.pb.js'
import { WebRuntimeClient } from './web-runtime-client.js'

describe('WebRuntimeClient', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('retries waitConn when the runtime connected ack times out', async () => {
    vi.useFakeTimers()
    const client = new WebRuntimeClient(
      'runtime',
      'client',
      WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      vi.fn(),
      null,
      null,
    )
    const { port1 } = new MessageChannel()
    const openClientChannel = vi
      .fn()
      .mockRejectedValueOnce(
        new Error(
          'WebRuntimeClient: client: timeout waiting for runtime connected ack',
        ),
      )
      .mockResolvedValue(port1)
    Reflect.set(client, 'openClientChannel', openClientChannel)

    const waitPromise = client.waitConn()
    await vi.advanceTimersByTimeAsync(100)
    await expect(waitPromise).resolves.toBeUndefined()
    expect(openClientChannel).toHaveBeenCalledTimes(2)

    client.close()
  })
})
