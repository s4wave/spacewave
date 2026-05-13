import { afterEach, describe, expect, it, vi } from 'vitest'

import { WebRuntimeClientType } from '../runtime/runtime.pb.js'
import { WebRuntimeClient } from './web-runtime-client.js'

function startStreamOpenTimeout(client: WebRuntimeClient): Promise<void> {
  const streamOpenTimeout = Reflect.get(client, 'streamOpenTimeoutPromise')
  if (typeof streamOpenTimeout !== 'function') {
    throw new Error('streamOpenTimeoutPromise is not callable')
  }
  return streamOpenTimeout.call(client)
}

describe('WebRuntimeClient', () => {
  afterEach(() => {
    vi.restoreAllMocks()
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

  it('defers the stream-open timeout until the resume-ready gate resolves', async () => {
    vi.useFakeTimers()
    let resolveResumeReady: (() => void) | undefined
    const client = new WebRuntimeClient(
      'runtime',
      'client',
      WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      vi.fn(),
      null,
      null,
      undefined,
      undefined,
      () =>
        new Promise<void>((resolve) => {
          resolveResumeReady = resolve
        }),
    )
    const streamOpenTimeoutPromise = startStreamOpenTimeout(client)
    const timedOut = vi.fn()
    streamOpenTimeoutPromise.then(timedOut)

    await vi.advanceTimersByTimeAsync(2999)
    expect(timedOut).not.toHaveBeenCalled()

    resolveResumeReady?.()
    await vi.advanceTimersByTimeAsync(1499)
    expect(timedOut).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(1)
    expect(timedOut).toHaveBeenCalledTimes(1)
    await expect(streamOpenTimeoutPromise).resolves.toBeUndefined()
  })

  it('bounds the resume-ready gate wait before applying the stream-open timeout', async () => {
    vi.useFakeTimers()
    const client = new WebRuntimeClient(
      'runtime',
      'client',
      WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      vi.fn(),
      null,
      null,
      undefined,
      undefined,
      () => new Promise<void>(() => {}),
    )

    const streamOpenTimeoutPromise = startStreamOpenTimeout(client)
    const timedOut = vi.fn()
    streamOpenTimeoutPromise.then(timedOut)

    await vi.advanceTimersByTimeAsync(4499)
    expect(timedOut).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(1)
    expect(timedOut).toHaveBeenCalledTimes(1)
    await expect(streamOpenTimeoutPromise).resolves.toBeUndefined()
  })

  it('applies the stream-open timeout after the resume-ready gate resolves', async () => {
    vi.useFakeTimers()
    const client = new WebRuntimeClient(
      'runtime',
      'client',
      WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      vi.fn(),
      null,
      null,
      undefined,
      undefined,
      vi.fn().mockResolvedValue(undefined),
    )

    const streamOpenTimeoutPromise = startStreamOpenTimeout(client)
    const timedOut = vi.fn()
    streamOpenTimeoutPromise.then(timedOut)

    await vi.advanceTimersByTimeAsync(1499)
    expect(timedOut).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(1)
    expect(timedOut).toHaveBeenCalledTimes(1)
    await expect(streamOpenTimeoutPromise).resolves.toBeUndefined()
  })
})
