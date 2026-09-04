import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import {
  Client,
  HandleStreamCtr,
  Server,
  createHandler,
  createMux,
} from 'starpc'
import { EchoerClient, EchoerDefinition, EchoerServer } from 'starpc/echo'
import type { PacketStream } from 'starpc'
import { pushable } from 'it-pushable'
import type { BackendAPI } from '@aptre/bldr-sdk'

import {
  castToError,
  packetStreamToOpenStreamCallbacks,
  type GoPushableSink,
  type OpenStreamFunc,
} from './open-stream-contract.js'
import main from './plugin-goscript-cloudflare.js'

// openStreamCallbacksToPacketStream drives the Go callback contract in tests.
function openStreamCallbacksToPacketStream(
  open: OpenStreamFunc,
): Promise<PacketStream> {
  return new Promise<PacketStream>((resolve, reject) => {
    const incoming = pushable<Uint8Array>({ objectMode: true })
    open(
      (message) => incoming.push(message),
      (errMsg) => incoming.end(errMsg ? new Error(errMsg) : undefined),
      (sink: GoPushableSink) =>
        resolve({
          close: async () => {
            incoming.end()
          },
          abort: (err) => incoming.end(err),
          source: incoming,
          sink: async (source) => {
            try {
              for await (const message of source) sink.push(message)
              sink.end()
            } catch (err) {
              sink.end()
              incoming.end(castToError(err))
            }
          },
        }),
      (errMsg) => reject(new Error(errMsg)),
    )
  })
}

// buildBackendAPI creates a mock BackendAPI for testing.
function buildBackendAPI(
  openStream: BackendAPI['openStream'] = vi.fn(),
): BackendAPI {
  return {
    startInfo: {
      instanceId: 'inst1',
      pluginId: 'goscript-cloudflare-runtime-proof',
      instanceKey: 'default',
    },
    openStream,
    handleStreamCtr: new HandleStreamCtr(),
  } as unknown as BackendAPI
}

// buildPacketStreamPair creates an in-memory connected PacketStream pair.
function buildPacketStreamPair(): [PacketStream, PacketStream] {
  const aSource = pushable<Uint8Array>({ objectMode: true })
  const bSource = pushable<Uint8Array>({ objectMode: true })
  const close = async () => {
    aSource.end()
    bSource.end()
  }
  const abort = (err: Error) => {
    aSource.end(err)
    bSource.end(err)
  }
  const a: PacketStream = {
    close,
    abort,
    source: aSource,
    sink: async (source) => {
      for await (const msg of source) {
        bSource.push(msg)
      }
      bSource.end()
    },
  }
  const b: PacketStream = {
    close,
    abort,
    source: bSource,
    sink: async (source) => {
      for await (const msg of source) {
        aSource.push(msg)
      }
      aSource.end()
    },
  }
  return [a, b]
}

describe('plugin-goscript-cloudflare runtime', () => {
  beforeEach(() => {
    delete globalThis.BLDR_PLUGIN_START_INFO
    delete globalThis.BLDR_PLUGIN_SET_ACCEPT_STREAM_WORKERS
    delete globalThis.BLDR_PLUGIN_MARK_READY
    delete globalThis.BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME
  })

  afterEach(() => {
    delete globalThis.BLDR_PLUGIN_SET_ACCEPT_STREAM_WORKERS
    delete globalThis.BLDR_PLUGIN_MARK_READY
    delete globalThis.BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME
    delete globalThis.BLDR_PLUGIN_START_INFO
  })

  it('makes a real typed StarPC call through the host runtime', async () => {
    // Host side: an Echoer server reached through an in-memory PacketStream.
    const mux = createMux()
    mux.register(createHandler(EchoerDefinition, new EchoerServer()))
    const server = new Server(mux.lookupMethod)
    const [clientSide, serverSide] = buildPacketStreamPair()
    server.handlePacketStream(serverSide)

    // Install the host runtime so it registers the globals and hands the
    // plugin a real PacketStream through api.openStream.
    const api = buildBackendAPI(vi.fn(async () => clientSide))
    main(api, async () => async () => {
      await new Promise<void>(() => {})
    })

    // Simulate the Go plugin: it calls the runtime global with its four
    // callbacks. The runtime adapts api.openStream() to that contract.
    const packetStream = await openStreamCallbacksToPacketStream(
      (onMessage, onClose, onResolve, onReject) => {
        const fn = globalThis.BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME
        if (!fn) {
          throw new Error('missing BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME')
        }
        fn(onMessage, onClose, onResolve, onReject)
      },
    )

    // Build a typed StarPC client over the adapted packet stream.
    const client = new EchoerClient(new Client(async () => packetStream))

    // Make a real typed StarPC call.
    const result = await client.Echo({ body: 'Hello from Cloudflare!' })
    expect(result.body).toBe('Hello from Cloudflare!')
  })

  it('registers accept-stream and marks ready', async () => {
    const api = buildBackendAPI()
    const lifecycle = main(api, async () => async () => {
      await new Promise<void>(() => {})
    })

    const acceptStream = vi.fn()
    const setAcceptStream = globalThis.BLDR_PLUGIN_SET_ACCEPT_STREAM_WORKERS
    expect(setAcceptStream).toBeTypeOf('function')
    setAcceptStream!(acceptStream)
    await lifecycle.startup

    // handleStreamCtr must be set to a handler function.
    const handler = api.handleStreamCtr.value
    expect(handler).toBeTypeOf('function')

    // Accepting a stream must call the registered acceptStream with an
    // openStreamFunc using the shared callback contract.
    const [clientSide] = buildPacketStreamPair()
    void handler!(clientSide)
    await Promise.resolve()
    expect(acceptStream).toHaveBeenCalledTimes(1)

    // The openStreamFunc must receive the four callbacks.
    const openStreamFunc = acceptStream.mock.calls[0][0] as OpenStreamFunc
    expect(typeof openStreamFunc).toBe('function')
  })

  it('unregisters the accept-stream handler after plugin exit', async () => {
    let rejectPluginMain!: (err: unknown) => void
    const pluginMainExited = new Promise<void>((_resolve, reject) => {
      rejectPluginMain = reject
    })
    const api = buildBackendAPI()
    void main(api, async () => () => pluginMainExited)
    await Promise.resolve()
    await Promise.resolve()

    const err = new Error('runtime exited')
    rejectPluginMain(err)
    await Promise.resolve()
    await Promise.resolve()

    const [clientSide] = buildPacketStreamPair()
    await expect(
      api.handleStreamCtr.handleStreamFunc(clientSide),
    ).rejects.toThrow('runtime exited')
  })
  it('closes the PacketStream when its peer closes', async () => {
    const source = pushable<Uint8Array>({ objectMode: true })
    const channel = {
      close: vi.fn(async () => {}),
      abort: vi.fn(),
      source,
      sink: vi.fn(async () => {}),
    } satisfies PacketStream
    packetStreamToOpenStreamCallbacks(channel).open(
      () => {},
      () => {},
      () => {},
      () => {},
    )

    source.end()
    await vi.waitFor(() => expect(channel.close).toHaveBeenCalledOnce())
    expect(channel.abort).not.toHaveBeenCalled()
  })

  it('aborts accepted PacketStreams when the plugin exits', async () => {
    let rejectPluginMain!: (err: unknown) => void
    const pluginMainExited = new Promise<void>((_resolve, reject) => {
      rejectPluginMain = reject
    })
    const api = buildBackendAPI()
    void main(api, async () => () => pluginMainExited)
    const setAcceptStream = globalThis.BLDR_PLUGIN_SET_ACCEPT_STREAM_WORKERS
    setAcceptStream!((openStream) => {
      openStream(
        () => {},
        () => {},
        () => {},
        () => {},
      )
    })
    const source = pushable<Uint8Array>({ objectMode: true })
    const channel = {
      close: vi.fn(async () => {}),
      abort: vi.fn(),
      source,
      sink: vi.fn(async () => {}),
    } satisfies PacketStream
    void api.handleStreamCtr.handleStreamFunc(channel)
    await vi.waitFor(() => expect(channel.sink).toHaveBeenCalledOnce())

    const err = new Error('runtime exited')
    rejectPluginMain(err)
    await vi.waitFor(() => expect(channel.abort).toHaveBeenCalledWith(err))
    expect(channel.close).not.toHaveBeenCalled()
  })
})
