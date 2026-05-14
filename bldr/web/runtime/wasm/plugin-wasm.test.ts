import { beforeEach, describe, expect, it, vi } from 'vitest'
import { HandleStreamCtr, PacketStream } from 'starpc'
import { BackendAPI } from '@aptre/bldr-sdk'

const goProcessState = vi.hoisted(() => ({
  start: vi.fn<() => Promise<void>>(),
  constructor: vi.fn(),
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
    vi.unstubAllGlobals()
    delete (globalThis as { BLDR_PLUGIN_START_INFO?: string })
      .BLDR_PLUGIN_START_INFO
    goProcessState.start.mockReset()
    goProcessState.constructor.mockReset()
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
})

function buildBackendAPI(): BackendAPI {
  return {
    startInfo: {},
    openStream: vi.fn(),
    handleStreamCtr: new HandleStreamCtr(),
  } as unknown as BackendAPI
}

function buildPacketStream(): PacketStream {
  return {
    source: (async function* () {})(),
    sink: vi.fn(async () => {}),
  }
}
