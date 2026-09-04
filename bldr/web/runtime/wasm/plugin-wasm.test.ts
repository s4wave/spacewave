import { beforeEach, describe, expect, it, vi } from 'vitest'
import { pushable } from 'it-pushable'
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

vi.mock('./go-process.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./go-process.js')>()
  return {
    ...actual,
    GoWasmProcess: vi.fn().mockImplementation(function (source, opts) {
      goProcessState.constructor(source, opts)
      return {
        start: goProcessState.start,
        stop: vi.fn(),
      }
    }),
  }
})

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
        close: vi.fn(async () => {}),
        abort: vi.fn(),
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
        close: vi.fn(async () => {}),
        abort: vi.fn(),
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

  it('opens TinyGo streams through primitive wasm imports', async () => {
    goProcessState.start.mockReturnValue(new Promise<void>(() => {}))
    const sinkPackets: Uint8Array[] = []
    const api = buildBackendAPI(
      vi.fn(async () => ({
        close: vi.fn(async () => {}),
        abort: vi.fn(),
        source: (async function* () {
          yield new Uint8Array([7, 8])
        })(),
        sink: vi.fn(async (packets: Parameters<PacketStream['sink']>[0]) => {
          for await (const packet of packets) {
            sinkPackets.push(packet)
          }
        }),
      })),
    )

    const { default: main } = await import('./plugin-wasm.js')
    await main(api)

    const opts = goProcessState.constructor.mock.calls[0][1] as {
      tinyGoRuntimeImports?: (go: unknown) => void
    }
    const memory = new WebAssembly.Memory({ initial: 1 })
    const exports = {
      memory,
      go_scheduler: vi.fn(),
      BLDR_PLUGIN_STREAM_OPEN_RESOLVE: vi.fn(),
      BLDR_PLUGIN_STREAM_OPEN_REJECT: vi.fn(),
      BLDR_PLUGIN_STREAM_MESSAGE: vi.fn(),
      BLDR_PLUGIN_STREAM_CLOSE: vi.fn(),
      BLDR_PLUGIN_STREAM_ACCEPT: vi.fn(),
    }
    const go = {
      importObject: { gojs: {} as Record<string, unknown> },
      _inst: { exports },
      _resume: vi.fn(),
    }
    opts.tinyGoRuntimeImports?.(go)

    const openStream = go.importObject.gojs['bldr.plugin.openStream'] as (
      opID: number,
    ) => void
    openStream(41)
    await Promise.resolve()

    expect(exports.BLDR_PLUGIN_STREAM_OPEN_RESOLVE).toHaveBeenCalledWith(41, 1)

    await waitForGoCallbackQueue()
    expect(exports.BLDR_PLUGIN_STREAM_MESSAGE).toHaveBeenCalledWith(
      1,
      expect.any(Number),
      2,
    )
    const messageBytesID = exports.BLDR_PLUGIN_STREAM_MESSAGE.mock.calls[0][1]
    const takeBytes = go.importObject.gojs['bldr.plugin.streamTakeBytes'] as (
      bytesID: number,
      ptr: number,
      len: number,
    ) => number
    expect(takeBytes(messageBytesID, 16, 2)).toBe(1)
    expect(Array.from(new Uint8Array(memory.buffer, 16, 2))).toEqual([7, 8])
    getGoImport(go.importObject.gojs, 'bldr.plugin.streamMessageHandled')(
      messageBytesID,
      1,
    )

    new Uint8Array(memory.buffer, 32, 2).set([9, 10])
    const writeStream = go.importObject.gojs['bldr.plugin.streamWrite'] as (
      streamID: number,
      ptr: number,
      len: number,
    ) => number
    expect(writeStream(1, 32, 2)).toBe(1)
    const closeStream = go.importObject.gojs['bldr.plugin.streamClose'] as (
      streamID: number,
    ) => number
    expect(closeStream(1)).toBe(1)
    await waitForGoCallbackQueue()

    expect(sinkPackets.map((packet) => Array.from(packet))).toEqual([[9, 10]])
  })

  it('keeps accepted TinyGo streams alive after handoff returns', async () => {
    goProcessState.start.mockReturnValue(new Promise<void>(() => {}))

    const api = buildBackendAPI()
    const { default: main } = await import('./plugin-wasm.js')
    await main(api)

    const acceptExport = vi.fn()
    const closeExport = vi.fn()
    const messageExport = vi.fn()
    const gojs = installTinyGoStreamImports({
      memory: new WebAssembly.Memory({ initial: 1 }),
      go_scheduler: vi.fn(),
      BLDR_PLUGIN_STREAM_ACCEPT: acceptExport,
      BLDR_PLUGIN_STREAM_CLOSE: closeExport,
      BLDR_PLUGIN_STREAM_MESSAGE: messageExport,
    })
    getGoImport(gojs, 'bldr.plugin.setAcceptStreams')(1)

    const source = pushable<Uint8Array>({ objectMode: true })
    const handled = api.handleStreamCtr.handleStreamFunc({
      close: async () => {
        source.end()
      },
      abort: (err) => source.end(err),
      source,
      sink: vi.fn(async () => {}),
    })
    await waitForGoCallbackQueue()

    expect(acceptExport).toHaveBeenCalledWith(1)
    await handled

    source.push(new Uint8Array([3, 4]))
    await waitForGoCallbackQueue()

    expect(messageExport).toHaveBeenCalledWith(1, expect.any(Number), 2)
    getGoImport(gojs, 'bldr.plugin.streamMessageHandled')(
      messageExport.mock.calls[0][1],
      1,
    )

    source.end()
    await waitForGoCallbackQueue()

    expect(closeExport).toHaveBeenCalledWith(1, 0, 0)
  })

  it('waits for TinyGo packet handling before reading the next inbound message', async () => {
    goProcessState.start.mockReturnValue(new Promise<void>(() => {}))

    const api = buildBackendAPI()
    const { default: main } = await import('./plugin-wasm.js')
    await main(api)

    const messageExport = vi.fn()
    const gojs = installTinyGoStreamImports({
      memory: new WebAssembly.Memory({ initial: 1 }),
      go_scheduler: vi.fn(),
      BLDR_PLUGIN_STREAM_ACCEPT: vi.fn(),
      BLDR_PLUGIN_STREAM_MESSAGE: messageExport,
      BLDR_PLUGIN_STREAM_CLOSE: vi.fn(),
    })
    getGoImport(gojs, 'bldr.plugin.setAcceptStreams')(1)

    const source = pushable<Uint8Array>({ objectMode: true })
    void api.handleStreamCtr.handleStreamFunc({
      close: async () => {
        source.end()
      },
      abort: (err) => source.end(err),
      source,
      sink: vi.fn(async () => {}),
    })
    await waitForGoCallbackQueue()

    source.push(new Uint8Array([1]))
    await waitForGoCallbackQueue()
    expect(messageExport).toHaveBeenCalledTimes(1)
    const firstBytesID = messageExport.mock.calls[0][1]

    source.push(new Uint8Array([2]))
    await waitForGoCallbackQueue()
    expect(messageExport).toHaveBeenCalledTimes(1)

    getGoImport(gojs, 'bldr.plugin.streamMessageHandled')(firstBytesID, 1)
    await waitForGoCallbackQueue()
    expect(messageExport).toHaveBeenCalledTimes(2)

    getGoImport(gojs, 'bldr.plugin.streamMessageHandled')(
      messageExport.mock.calls[1][1],
      1,
    )
    source.end()
  })

  it('delivers small and large TinyGo inbound payloads', async () => {
    goProcessState.start.mockReturnValue(new Promise<void>(() => {}))

    const api = buildBackendAPI()
    const { default: main } = await import('./plugin-wasm.js')
    await main(api)

    const messageExport = vi.fn()
    const memory = new WebAssembly.Memory({ initial: 4 })
    const gojs = installTinyGoStreamImports({
      memory,
      go_scheduler: vi.fn(),
      BLDR_PLUGIN_STREAM_ACCEPT: vi.fn(),
      BLDR_PLUGIN_STREAM_MESSAGE: messageExport,
      BLDR_PLUGIN_STREAM_CLOSE: vi.fn(),
    })
    getGoImport(gojs, 'bldr.plugin.setAcceptStreams')(1)

    const source = pushable<Uint8Array>({ objectMode: true })
    void api.handleStreamCtr.handleStreamFunc({
      close: async () => {
        source.end()
      },
      abort: (err) => source.end(err),
      source,
      sink: vi.fn(async () => {}),
    })
    await waitForGoCallbackQueue()

    const takeBytes = getGoNumberImport(gojs, 'bldr.plugin.streamTakeBytes')
    const messageHandled = getGoImport(gojs, 'bldr.plugin.streamMessageHandled')

    source.push(new Uint8Array([5, 6, 7]))
    await waitForGoCallbackQueue()
    expect(messageExport).toHaveBeenCalledTimes(1)
    const smallBytesID = messageExport.mock.calls[0][1]
    expect(messageExport.mock.calls[0][2]).toBe(3)
    expect(takeBytes(smallBytesID, 16, 3)).toBe(1)
    expect(Array.from(new Uint8Array(memory.buffer, 16, 3))).toEqual([5, 6, 7])
    messageHandled(smallBytesID, 1)

    const largePayload = Uint8Array.from(
      { length: 128 * 1024 },
      (_value, index) => index % 251,
    )
    source.push(largePayload)
    await waitForGoCallbackQueue()
    expect(messageExport).toHaveBeenCalledTimes(2)
    const largeBytesID = messageExport.mock.calls[1][1]
    expect(messageExport.mock.calls[1][2]).toBe(largePayload.byteLength)
    expect(takeBytes(largeBytesID, 64, largePayload.byteLength)).toBe(1)
    const copied = new Uint8Array(memory.buffer, 64, largePayload.byteLength)
    expect(Array.from(copied)).toEqual(Array.from(largePayload))
    messageHandled(largeBytesID, 1)
    source.end()
  })

  it('resolves pending TinyGo packet deliveries when a stream is released', async () => {
    goProcessState.start.mockReturnValue(new Promise<void>(() => {}))

    const api = buildBackendAPI()
    const { default: main } = await import('./plugin-wasm.js')
    await main(api)

    const messageExport = vi.fn()
    const memory = new WebAssembly.Memory({ initial: 1 })
    const gojs = installTinyGoStreamImports({
      memory,
      go_scheduler: vi.fn(),
      BLDR_PLUGIN_STREAM_ACCEPT: vi.fn(),
      BLDR_PLUGIN_STREAM_MESSAGE: messageExport,
      BLDR_PLUGIN_STREAM_CLOSE: vi.fn(),
    })
    getGoImport(gojs, 'bldr.plugin.setAcceptStreams')(1)

    const source = pushable<Uint8Array>({ objectMode: true })
    void api.handleStreamCtr.handleStreamFunc({
      close: async () => {
        source.end()
      },
      abort: (err) => source.end(err),
      source,
      sink: vi.fn(async () => {}),
    })
    await waitForGoCallbackQueue()

    source.push(new Uint8Array([1]))
    await waitForGoCallbackQueue()
    expect(messageExport).toHaveBeenCalledTimes(1)
    const firstBytesID = messageExport.mock.calls[0][1]

    getGoImport(gojs, 'bldr.plugin.streamRelease')(1)
    source.push(new Uint8Array([2]))
    await waitForGoCallbackQueue()

    expect(messageExport).toHaveBeenCalledTimes(1)
    const takeBytes = getGoNumberImport(gojs, 'bldr.plugin.streamTakeBytes')
    expect(takeBytes(firstBytesID, 16, 1)).toBe(0)
    source.end()
  })

  it('lets TinyGo drop stored stream bytes without copying them', async () => {
    goProcessState.start.mockReturnValue(new Promise<void>(() => {}))

    const api = buildBackendAPI()
    const { default: main } = await import('./plugin-wasm.js')
    await main(api)

    const messageExport = vi.fn()
    const memory = new WebAssembly.Memory({ initial: 1 })
    const gojs = installTinyGoStreamImports({
      memory,
      go_scheduler: vi.fn(),
      BLDR_PLUGIN_STREAM_ACCEPT: vi.fn(),
      BLDR_PLUGIN_STREAM_MESSAGE: messageExport,
      BLDR_PLUGIN_STREAM_CLOSE: vi.fn(),
    })
    getGoImport(gojs, 'bldr.plugin.setAcceptStreams')(1)

    const source = pushable<Uint8Array>({ objectMode: true })
    void api.handleStreamCtr.handleStreamFunc({
      close: async () => {
        source.end()
      },
      abort: (err) => source.end(err),
      source,
      sink: vi.fn(async () => {}),
    })
    await waitForGoCallbackQueue()

    source.push(new Uint8Array([9]))
    await waitForGoCallbackQueue()
    expect(messageExport).toHaveBeenCalledTimes(1)
    const bytesID = messageExport.mock.calls[0][1]

    const dropBytes = getGoNumberImport(gojs, 'bldr.plugin.streamDropBytes')
    const takeBytes = getGoNumberImport(gojs, 'bldr.plugin.streamTakeBytes')
    expect(dropBytes(bytesID)).toBe(1)
    expect(takeBytes(bytesID, 16, 1)).toBe(0)

    getGoImport(gojs, 'bldr.plugin.streamMessageHandled')(bytesID, 0)
    source.end()
  })

  it('keeps TinyGo inbound backpressure until dropped bytes are acknowledged', async () => {
    goProcessState.start.mockReturnValue(new Promise<void>(() => {}))

    const api = buildBackendAPI()
    const { default: main } = await import('./plugin-wasm.js')
    await main(api)

    const messageExport = vi.fn()
    const gojs = installTinyGoStreamImports({
      memory: new WebAssembly.Memory({ initial: 1 }),
      go_scheduler: vi.fn(),
      BLDR_PLUGIN_STREAM_ACCEPT: vi.fn(),
      BLDR_PLUGIN_STREAM_MESSAGE: messageExport,
      BLDR_PLUGIN_STREAM_CLOSE: vi.fn(),
    })
    getGoImport(gojs, 'bldr.plugin.setAcceptStreams')(1)

    const source = pushable<Uint8Array>({ objectMode: true })
    void api.handleStreamCtr.handleStreamFunc({
      close: async () => {
        source.end()
      },
      abort: (err) => source.end(err),
      source,
      sink: vi.fn(async () => {}),
    })
    await waitForGoCallbackQueue()

    source.push(new Uint8Array([1]))
    await waitForGoCallbackQueue()
    expect(messageExport).toHaveBeenCalledTimes(1)
    const firstBytesID = messageExport.mock.calls[0][1]

    const dropBytes = getGoNumberImport(gojs, 'bldr.plugin.streamDropBytes')
    expect(dropBytes(firstBytesID)).toBe(1)

    source.push(new Uint8Array([2]))
    await waitForGoCallbackQueue()
    expect(messageExport).toHaveBeenCalledTimes(1)

    getGoImport(gojs, 'bldr.plugin.streamMessageHandled')(firstBytesID, 1)
    await waitForGoCallbackQueue()
    expect(messageExport).toHaveBeenCalledTimes(2)

    getGoImport(gojs, 'bldr.plugin.streamMessageHandled')(
      messageExport.mock.calls[1][1],
      1,
    )
    source.end()
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

function installTinyGoStreamImports(
  exports: Record<string, unknown>,
): Record<string, unknown> {
  const call = goProcessState.constructor.mock.calls[0]
  const imports = getTinyGoRuntimeImports(call?.[1])
  const gojs: Record<string, unknown> = {}
  imports({
    importObject: { gojs },
    _inst: { exports },
    _resume: vi.fn(),
  })
  return gojs
}

function getTinyGoRuntimeImports(value: unknown): (go: unknown) => void {
  if (typeof value !== 'object' || value === null) {
    throw new Error('missing TinyGo runtime import options')
  }
  const imports = Reflect.get(value, 'tinyGoRuntimeImports')
  if (typeof imports !== 'function') {
    throw new Error('missing TinyGo runtime imports')
  }
  return (go: unknown) => {
    imports(go)
  }
}

function getGoImport(
  gojs: Record<string, unknown>,
  name: string,
): (...args: number[]) => void {
  const fn = gojs[name]
  if (typeof fn !== 'function') {
    throw new Error(`missing Go import ${name}`)
  }
  return (...args: number[]) => {
    fn(...args)
  }
}

function getGoNumberImport(
  gojs: Record<string, unknown>,
  name: string,
): (...args: number[]) => number {
  const fn = gojs[name]
  if (typeof fn !== 'function') {
    throw new Error(`missing Go import ${name}`)
  }
  return (...args: number[]) => {
    const result = fn(...args)
    if (typeof result !== 'number') {
      throw new Error(`Go import ${name} did not return a number`)
    }
    return result
  }
}

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
    close: vi.fn(async () => {}),
    abort: vi.fn(),
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
