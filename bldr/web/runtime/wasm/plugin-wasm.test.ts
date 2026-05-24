import { beforeEach, describe, expect, it, vi } from 'vitest'
import { HandleStreamCtr, PacketStream } from 'starpc'
import { BackendAPI } from '@aptre/bldr-sdk'

const goProcessState = vi.hoisted(() => ({
  start: vi.fn<() => Promise<void>>(),
  constructor: vi.fn(),
}))

const pipeState = vi.hoisted(() => ({
  pipe: vi.fn<() => Promise<void>>(),
}))

async function waitForGoCallbackQueue(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0))
  await new Promise((resolve) => setTimeout(resolve, 0))
}

vi.mock('it-pipe', () => ({
  pipe: pipeState.pipe,
}))

vi.mock('./go-process.js', () => ({
  GoWasmProcess: vi.fn().mockImplementation(function (source, opts) {
    goProcessState.constructor(source, opts)
    return {
      start: goProcessState.start,
      stop: vi.fn(),
    }
  }),
}))

describe('plugin-wasm generation lifecycle', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    delete (globalThis as { BLDR_PLUGIN_START_INFO?: string })
      .BLDR_PLUGIN_START_INFO
    goProcessState.start.mockReset()
    goProcessState.constructor.mockReset()
    pipeState.pipe.mockReset()
    pipeState.pipe.mockResolvedValue(undefined)
    vi.stubGlobal('BLDR_PLUGIN_ENTRYPOINT', 'plugin.wasm')
    vi.stubGlobal('BLDR_PLUGIN_REPORT_RUNTIME_FAILURE', vi.fn())
  })

  it('does not retry Go WASM inside the failed worker generation', async () => {
    let rejectProcess!: (err: unknown) => void
    goProcessState.start.mockReturnValue(
      new Promise<void>((_resolve, reject) => {
        rejectProcess = reject
      }),
    )

    const { default: main } = await import('./plugin-wasm.js')
    await main(buildBackendAPI())

    expect(goProcessState.constructor).toHaveBeenCalledWith(
      expect.stringContaining('/plugin.wasm'),
      expect.objectContaining({ retry: false }),
    )
    expect(
      (globalThis as { BLDR_PLUGIN_START_INFO?: string })
        .BLDR_PLUGIN_START_INFO,
    ).toBe(btoa('{}'))

    const err = new Error('fatal heap pointer')
    rejectProcess(err)
    await Promise.resolve()
    await Promise.resolve()

    expect(
      (
        globalThis as {
          BLDR_PLUGIN_REPORT_RUNTIME_FAILURE?: (err: unknown) => void
        }
      ).BLDR_PLUGIN_REPORT_RUNTIME_FAILURE,
    ).toHaveBeenCalledWith(err)
  })

  it('turns accept-stream into a terminal error after Go WASM exits', async () => {
    let rejectProcess!: (err: unknown) => void
    goProcessState.start.mockReturnValue(
      new Promise<void>((_resolve, reject) => {
        rejectProcess = reject
      }),
    )

    const api = buildBackendAPI()
    const { default: main } = await import('./plugin-wasm.js')
    await main(api)

    const err = new Error('runtime exited')
    rejectProcess(err)
    await Promise.resolve()
    await Promise.resolve()

    await expect(
      api.handleStreamCtr.handleStreamFunc(buildPacketStream()),
    ).rejects.toThrow('runtime exited')
  })

  it('closes active accepted streams when the Go WASM generation exits', async () => {
    let rejectProcess!: (err: unknown) => void
    goProcessState.start.mockReturnValue(
      new Promise<void>((_resolve, reject) => {
        rejectProcess = reject
      }),
    )
    pipeState.pipe.mockReturnValue(new Promise<void>(() => {}))

    const api = buildBackendAPI()
    const { default: main } = await import('./plugin-wasm.js')
    await main(api)

    const acceptedChannel = buildMessageChannel()
    vi.stubGlobal(
      'MessageChannel',
      vi.fn(function () {
        return acceptedChannel
      }),
    )
    const setAcceptStream = (
      globalThis as {
        BLDR_PLUGIN_SET_ACCEPT_STREAM?: (
          acceptStream: (localPort: MessagePort) => void,
        ) => void
      }
    ).BLDR_PLUGIN_SET_ACCEPT_STREAM
    expect(setAcceptStream).toBeTypeOf('function')
    const acceptStream = vi.fn()
    setAcceptStream!(acceptStream)

    void api.handleStreamCtr.handleStreamFunc(buildPacketStream())
    await Promise.resolve()

    expect(acceptStream).toHaveBeenCalledWith(acceptedChannel.port1)
    expect(pipeState.pipe).toHaveBeenCalledTimes(1)
    expect(acceptedChannel.port2.postMessage).not.toHaveBeenCalled()
    expect(acceptedChannel.port2.close).not.toHaveBeenCalled()

    const err = new Error('fatal heap pointer')
    rejectProcess(err)
    await Promise.resolve()
    await Promise.resolve()

    expect(acceptedChannel.port2.postMessage).toHaveBeenCalledWith(null)
    expect(acceptedChannel.port2.close).toHaveBeenCalledTimes(1)
    await expect(
      api.handleStreamCtr.handleStreamFunc(buildPacketStream()),
    ).rejects.toThrow('fatal heap pointer')
  })

  it('keeps old generations terminal while a restarted worker publishes a fresh accept handler', async () => {
    let rejectFirstProcess!: (err: unknown) => void
    goProcessState.start
      .mockReturnValueOnce(
        new Promise<void>((_resolve, reject) => {
          rejectFirstProcess = reject
        }),
      )
      .mockReturnValueOnce(new Promise<void>(() => {}))
    pipeState.pipe.mockReturnValue(new Promise<void>(() => {}))

    const firstApi = buildBackendAPI()
    const { default: main } = await import('./plugin-wasm.js')
    await main(firstApi)

    const err = new Error('old generation failed')
    rejectFirstProcess(err)
    await Promise.resolve()
    await Promise.resolve()

    await expect(
      firstApi.handleStreamCtr.handleStreamFunc(buildPacketStream()),
    ).rejects.toThrow('old generation failed')

    const secondApi = buildBackendAPI()
    await main(secondApi)

    const freshChannel = buildMessageChannel()
    vi.stubGlobal(
      'MessageChannel',
      vi.fn(function () {
        return freshChannel
      }),
    )
    const freshAccept = vi.fn()
    const setAcceptStream = (
      globalThis as {
        BLDR_PLUGIN_SET_ACCEPT_STREAM?: (
          acceptStream: (localPort: MessagePort) => void,
        ) => void
      }
    ).BLDR_PLUGIN_SET_ACCEPT_STREAM
    expect(setAcceptStream).toBeTypeOf('function')
    setAcceptStream!(freshAccept)

    void secondApi.handleStreamCtr.handleStreamFunc(buildPacketStream())
    await Promise.resolve()

    expect(freshAccept).toHaveBeenCalledTimes(1)
    expect(freshAccept).toHaveBeenCalledWith(freshChannel.port1)
    expect(freshChannel.port2.postMessage).not.toHaveBeenCalled()
    expect(
      (
        globalThis as {
          BLDR_PLUGIN_REPORT_RUNTIME_FAILURE?: (err: unknown) => void
        }
      ).BLDR_PLUGIN_REPORT_RUNTIME_FAILURE,
    ).toHaveBeenCalledTimes(1)
  })

  it('stops packet delivery after Go stream callbacks are released', async () => {
    goProcessState.start.mockReturnValue(new Promise<void>(() => {}))
    let sinkCompleted = false
    const api = buildBackendAPI(
      vi.fn(async () => ({
        source: (async function* () {
          yield new Uint8Array([1])
          yield new Uint8Array([2])
        })(),
        sink: vi.fn(async (packets: Parameters<PacketStream['sink']>[0]) => {
          for await (const packet of packets) {
            void packet
          }
          sinkCompleted = true
        }),
      })),
    )

    const { default: main } = await import('./plugin-wasm.js')
    await main(api)
    const openStream = (
      globalThis as {
        BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME?: (
          onMessage: (message: Uint8Array) => void,
          onClose: (errMsg?: string) => void,
          onResolve: (sink: {
            push: (message: Uint8Array) => void
            end: () => void
          }) => void,
          onReject: (errMsg: string) => void,
        ) => void
      }
    ).BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME
    expect(openStream).toBeTypeOf('function')

    const onMessage = vi.fn(() => {
      console.error('call to released function')
    })
    const onClose = vi.fn(() => {
      console.error('call to released function')
    })
    const onResolve = vi.fn()
    const onReject = vi.fn()
    const consoleError = vi.spyOn(console, 'error')
    openStream!(onMessage, onClose, onResolve, onReject)
    await waitForGoCallbackQueue()

    expect(onResolve).toHaveBeenCalledTimes(1)
    expect(onReject).not.toHaveBeenCalled()
    expect(onMessage).toHaveBeenCalledTimes(1)
    expect(onClose).not.toHaveBeenCalled()
    expect(consoleError).not.toHaveBeenCalled()
    expect(sinkCompleted).toBe(true)
  })

  it('does not classify substring callback errors as Go released callbacks', async () => {
    goProcessState.start.mockReturnValue(new Promise<void>(() => {}))
    const api = buildBackendAPI(
      vi.fn(async () => ({
        source: (async function* () {
          yield new Uint8Array([1])
        })(),
        sink: vi.fn(async () => {}),
      })),
    )

    const { default: main } = await import('./plugin-wasm.js')
    await main(api)
    const openStream = (
      globalThis as {
        BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME?: (
          onMessage: (message: Uint8Array) => void,
          onClose: (errMsg?: string) => void,
          onResolve: (sink: {
            push: (message: Uint8Array) => void
            end: () => void
          }) => void,
          onReject: (errMsg: string) => void,
        ) => void
      }
    ).BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME
    expect(openStream).toBeTypeOf('function')

    const onMessage = vi.fn(() => {
      throw new Error('not the call to released function sentinel')
    })
    const onClose = vi.fn()
    const onResolve = vi.fn()
    const onReject = vi.fn()
    openStream!(onMessage, onClose, onResolve, onReject)
    await waitForGoCallbackQueue()

    expect(onMessage).toHaveBeenCalledTimes(1)
    expect(onClose).toHaveBeenCalledWith(
      'Error: not the call to released function sentinel',
    )
  })

  it('reports open stream failures through the reject callback', async () => {
    goProcessState.start.mockReturnValue(new Promise<void>(() => {}))
    const api = buildBackendAPI(
      vi.fn(async () => {
        throw new Error('stream unavailable')
      }),
    )

    const { default: main } = await import('./plugin-wasm.js')
    await main(api)
    const openStream = (
      globalThis as {
        BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME?: (
          onMessage: (message: Uint8Array) => void,
          onClose: (errMsg?: string) => void,
          onResolve: (sink: {
            push: (message: Uint8Array) => void
            end: () => void
          }) => void,
          onReject: (errMsg: string) => void,
        ) => void
      }
    ).BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME
    expect(openStream).toBeTypeOf('function')

    const onResolve = vi.fn()
    const onReject = vi.fn()
    openStream!(vi.fn(), vi.fn(), onResolve, onReject)
    await waitForGoCallbackQueue()

    expect(onResolve).not.toHaveBeenCalled()
    expect(onReject).toHaveBeenCalledWith('Error: stream unavailable')
  })

  it('does not globally filter Go released-callback logs inside the plugin worker', async () => {
    goProcessState.start.mockReturnValue(new Promise<void>(() => {}))
    const consoleError = vi.spyOn(console, 'error')

    const { default: main } = await import('./plugin-wasm.js')
    await main(buildBackendAPI())

    console.error('call to released function')
    console.error('other failure')

    expect(consoleError).toHaveBeenCalledTimes(2)
    expect(consoleError).toHaveBeenNthCalledWith(1, 'call to released function')
    expect(consoleError).toHaveBeenNthCalledWith(2, 'other failure')
  })
})

function buildBackendAPI(
  openStream: BackendAPI['openStream'] = vi.fn(),
): BackendAPI {
  return {
    startInfo: {},
    openStream,
    handleStreamCtr: new HandleStreamCtr(),
  } as unknown as BackendAPI
}

function buildPacketStream(): PacketStream {
  return {
    source: (async function* () {})(),
    sink: vi.fn(async () => {}),
  }
}

function buildMessagePort(): MessagePort {
  return {
    postMessage: vi.fn(),
    close: vi.fn(),
  } as unknown as MessagePort
}

function buildMessageChannel(): MessageChannel {
  return {
    port1: buildMessagePort(),
    port2: buildMessagePort(),
  }
}
