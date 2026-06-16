import { afterEach, describe, expect, it, vi } from 'vitest'

import { WebRuntimeClientType } from '../runtime/runtime.pb.js'
import {
  RuntimeClientGenerationGateError,
  type RuntimeClientStreamOpenGateResult,
  WebRuntimeClient,
} from './web-runtime-client.js'

interface Deferred<T> {
  promise: Promise<T>
  resolve(value: T): void
}

function newDeferred<T>(): Deferred<T> {
  let resolve: (value: T) => void = () => {}
  const promise = new Promise<T>((r) => {
    resolve = r
  })
  return { promise, resolve }
}

async function flushPromises(count = 5): Promise<void> {
  for (let i = 0; i < count; i++) {
    await Promise.resolve()
  }
}

async function* emptyPacketSource(): AsyncGenerator<Uint8Array> {}

async function startStreamOpenGate(
  client: WebRuntimeClient,
  generationId = 1,
): Promise<void> {
  const waitForStreamOpenGate = Reflect.get(client, 'waitForStreamOpenGate')
  if (typeof waitForStreamOpenGate !== 'function') {
    throw new Error('waitForStreamOpenGate is not callable')
  }
  return waitForStreamOpenGate.call(client, generationId)
}

function seedConnectedGeneration(client: WebRuntimeClient): void {
  Reflect.set(client, 'generation', {
    id: 1,
    state: 'connected',
    webRuntimeId: 'runtime',
    clientId: 'client',
    clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
  })
  Reflect.set(client, 'generationAbortController', new AbortController())
}

function installFakeMessageChannel(): {
  channels: Array<{ port1: MessagePort; port2: MessagePort }>
  port(): MessagePort
} {
  const channels: Array<{ port1: MessagePort; port2: MessagePort }> = []
  class FakeMessagePort {
    public onmessage: ((ev: MessageEvent) => void) | null = null
    public postMessage = vi.fn()
    public start = vi.fn()
    public close = vi.fn()
  }
  class FakeMessageChannel {
    public readonly port1 = new FakeMessagePort() as unknown as MessagePort
    public readonly port2 = new FakeMessagePort() as unknown as MessagePort

    public constructor() {
      channels.push({ port1: this.port1, port2: this.port2 })
    }
  }
  vi.stubGlobal('MessagePort', FakeMessagePort)
  vi.stubGlobal('MessageChannel', FakeMessageChannel)
  return {
    channels,
    port() {
      return new FakeMessagePort() as unknown as MessagePort
    },
  }
}

async function connectClient(
  client: WebRuntimeClient,
  runtimePort: MessagePort,
): Promise<void> {
  const waitConn = client.waitConn()
  runtimePort.postMessage({ connected: true })
  await waitConn
}

describe('WebRuntimeClient', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('keeps runtime connected ack pending until the generation closes', async () => {
    vi.useFakeTimers()
    const { port1, port2 } = new MessageChannel()
    const client = new WebRuntimeClient(
      'runtime',
      'client',
      WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      vi.fn().mockResolvedValue(port1),
      null,
      null,
    )

    let settled = false
    const waitPromise = client.waitConn()
    waitPromise.then(
      () => {
        settled = true
      },
      () => {
        settled = true
      },
    )
    await Promise.resolve()

    await vi.advanceTimersByTimeAsync(30_000)

    expect(settled).toBe(false)
    expect(client.getRuntimeGenerationSnapshot()).toMatchObject({
      id: 1,
      state: 'opening',
    })
    expect(client.getRuntimeGenerationSnapshot().closeReason).toBeUndefined()

    client.close()

    await expect(waitPromise).rejects.toThrow('normal-close')
    expect(client.getRuntimeGenerationSnapshot()).toMatchObject({
      id: 1,
      state: 'closed',
      closeReason: 'normal-close',
    })
    port2.close()
  })

  it('exposes initial generation state for every browser runtime client type', () => {
    for (const clientType of [
      WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      WebRuntimeClientType.WebRuntimeClientType_SERVICE_WORKER,
      WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
    ]) {
      const client = new WebRuntimeClient(
        'runtime',
        `client-${clientType}`,
        clientType,
        vi.fn(),
        null,
        null,
      )

      expect(client.getRuntimeGenerationSnapshot()).toMatchObject({
        id: 0,
        state: 'idle',
        webRuntimeId: 'runtime',
        clientId: `client-${clientType}`,
        clientType,
        closeReason: 'not-started',
        activeStreams: 0,
      })
    }
  })

  it('publishes connected and closed runtime client generations', async () => {
    const { port1, port2 } = new MessageChannel()
    const client = new WebRuntimeClient(
      'runtime',
      'service-worker',
      WebRuntimeClientType.WebRuntimeClientType_SERVICE_WORKER,
      vi.fn().mockResolvedValue(port1),
      null,
      null,
    )

    await connectClient(client, port2)

    const connected = client.getRuntimeGenerationSnapshot()
    expect(connected).toMatchObject({
      id: 1,
      state: 'connected',
      clientId: 'service-worker',
      clientType: WebRuntimeClientType.WebRuntimeClientType_SERVICE_WORKER,
      activeStreams: 0,
    })
    expect(connected.openedAtMs).toBeTypeOf('number')
    expect(connected.connectedAtMs).toBeTypeOf('number')

    client.close()
    expect(client.getRuntimeGenerationSnapshot()).toMatchObject({
      id: 1,
      state: 'closed',
      closeReason: 'normal-close',
      activeStreams: 0,
    })
    port2.close()
  })

  it('closes child streams when the parent runtime client generation closes', () => {
    const client = new WebRuntimeClient(
      'runtime',
      'plugin-worker',
      WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
      vi.fn(),
      null,
      null,
    )
    Reflect.set(client, 'generation', {
      id: 1,
      state: 'connected',
      webRuntimeId: 'runtime',
      clientId: 'plugin-worker',
      clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
    })
    const close = vi.fn()
    const stream = { close }
    const activeStreams = Reflect.get(client, 'activeStreams') as Set<{
      close(error?: Error): void
    }>
    activeStreams.add(stream)
    expect(client.getRuntimeGenerationSnapshot().activeStreams).toBe(1)

    client.close()

    expect(close).toHaveBeenCalledWith(
      expect.objectContaining({
        message: expect.stringContaining('normal-close'),
      }),
    )
    expect(client.getRuntimeGenerationSnapshot()).toMatchObject({
      state: 'closed',
      activeStreams: 0,
    })
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

  it('keeps the resume-ready gate pending without a timeout', async () => {
    vi.useFakeTimers()
    const resumeReady = newDeferred<RuntimeClientStreamOpenGateResult>()
    const client = new WebRuntimeClient(
      'runtime',
      'client',
      WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      vi.fn(),
      null,
      null,
      undefined,
      undefined,
      () => resumeReady.promise,
    )
    seedConnectedGeneration(client)

    let settled = false
    const gatePromise = startStreamOpenGate(client)
    gatePromise.then(
      () => {
        settled = true
      },
      () => {
        settled = true
      },
    )

    await vi.advanceTimersByTimeAsync(30_000)

    expect(settled).toBe(false)

    resumeReady.resolve({ state: 'ready', documentId: 'document-1' })

    await expect(gatePromise).resolves.toBeUndefined()
  })

  it('returns a typed resume-unavailable gate failure', async () => {
    const client = new WebRuntimeClient(
      'runtime',
      'client',
      WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      vi.fn(),
      null,
      null,
      undefined,
      undefined,
      vi.fn().mockResolvedValue({
        state: 'unavailable',
        reason: 'no active WebDocument',
      } satisfies RuntimeClientStreamOpenGateResult),
    )
    seedConnectedGeneration(client)

    await expect(startStreamOpenGate(client)).rejects.toThrow(
      RuntimeClientGenerationGateError,
    )
  })

  it('keeps stream opens pending until the parent generation closes', async () => {
    vi.useFakeTimers()
    const fake = installFakeMessageChannel()
    const clientPort = fake.port()
    const client = new WebRuntimeClient(
      'runtime',
      'client',
      WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      vi.fn(),
      null,
      null,
    )
    seedConnectedGeneration(client)
    Reflect.set(client, 'clientChannel', clientPort)

    let settled = false
    const openPromise = client.openStream()
    openPromise.then(
      () => {
        settled = true
      },
      () => {
        settled = true
      },
    )

    await vi.advanceTimersByTimeAsync(30_000)

    expect(settled).toBe(false)
    expect(client.getRuntimeGenerationSnapshot()).toMatchObject({
      state: 'connected',
      activeStreams: 1,
    })
    expect(client.getRuntimeGenerationSnapshot().closeReason).toBeUndefined()

    client.close()

    await expect(openPromise).rejects.toThrow('normal-close')
    expect(client.getRuntimeGenerationSnapshot()).toMatchObject({
      state: 'closed',
      closeReason: 'normal-close',
      activeStreams: 0,
    })
  })

  it('opens a stream after resume-ready reports ready', async () => {
    const fake = installFakeMessageChannel()
    const clientPort = fake.port()
    const resumeReady = newDeferred<RuntimeClientStreamOpenGateResult>()
    const client = new WebRuntimeClient(
      'runtime',
      'client',
      WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      vi.fn(),
      null,
      null,
      undefined,
      undefined,
      () => resumeReady.promise,
    )
    seedConnectedGeneration(client)
    Reflect.set(client, 'clientChannel', clientPort)

    const openPromise = client.openStream()
    await flushPromises()
    expect(fake.channels).toHaveLength(0)

    resumeReady.resolve({ state: 'ready', documentId: 'document-1' })
    await flushPromises()
    expect(fake.channels).toHaveLength(1)

    fake.channels[0].port1.onmessage?.({
      data: { from: 'runtime', ack: true, opened: true },
    } as MessageEvent)
    const stream = await openPromise
    expect(client.getRuntimeGenerationSnapshot().activeStreams).toBe(1)

    await stream.sink(emptyPacketSource())
    expect(client.getRuntimeGenerationSnapshot().activeStreams).toBe(1)

    const close = Reflect.get(stream, 'close')
    if (typeof close !== 'function') {
      throw new Error('runtime stream close method missing')
    }
    close.call(stream)
    expect(client.getRuntimeGenerationSnapshot().activeStreams).toBe(0)

    client.close()
  })

  it('reroutes a connected runtime client through a fresh channel without telling the runtime it closed', async () => {
    const { port1, port2 } = new MessageChannel()
    const reconnect = new MessageChannel()
    const openClientCh = vi
      .fn()
      .mockResolvedValueOnce(port1)
      .mockResolvedValueOnce(reconnect.port1)
    const client = new WebRuntimeClient(
      'runtime',
      'client',
      WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      openClientCh,
      null,
      null,
    )

    await connectClient(client, port2)
    expect(openClientCh).toHaveBeenCalledTimes(1)

    const streamClose = vi.fn()
    const activeStreams = Reflect.get(client, 'activeStreams') as Set<{
      close(error?: Error): void
    }>
    activeStreams.add({ close: streamClose })
    const postMessage = vi.spyOn(port1, 'postMessage')

    await client.rerouteChannel()

    // In-flight streams fail with a retryable relay-rerouted error so callers
    // retry onto the next generation, not a terminal normal-close.
    expect(streamClose).toHaveBeenCalledWith(
      expect.objectContaining({
        message: expect.stringContaining('relay-rerouted'),
      }),
    )
    // The runtime is not told the client is going away; no close is posted.
    expect(postMessage).not.toHaveBeenCalledWith(
      expect.objectContaining({ close: true }),
    )
    // A reconnect through a surviving document is kicked off.
    await flushPromises()
    expect(openClientCh).toHaveBeenCalledTimes(2)

    client.close()
    port2.close()
    reconnect.port1.close()
    reconnect.port2.close()
  })
})
