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

  it('shares a single reconnect across concurrent waiters', async () => {
    const client = new WebRuntimeClient(
      'runtime',
      'client',
      WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      vi.fn(),
      null,
      null,
    )

    const { port1 } = new MessageChannel()
    let resolveConnect: ((port: MessagePort) => void) | undefined
    const reconnect = vi.fn().mockImplementation(
      () =>
        new Promise<MessagePort>((resolve) => {
          resolveConnect = resolve
        }),
    )
    Reflect.set(client, 'openClientChannelWithRetryImpl', reconnect)

    const a = client.waitConn()
    const b = client.waitConn()
    expect(reconnect).toHaveBeenCalledTimes(1)

    resolveConnect?.(port1)
    await expect(Promise.all([a, b])).resolves.toEqual([undefined, undefined])
  })
})
